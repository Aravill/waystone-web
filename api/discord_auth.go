package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"time"
	"waystone-web/db"
	"waystone-web/discord"
	"waystone-web/middleware"
)

const discordStateKey = "discord_state"

func discordRedirectURI() string { return discordConf().publicURL + "/auth/discord/callback" }

func profileDiscordRedirect(w http.ResponseWriter, r *http.Request, result string) {
	http.Redirect(w, r, "/profile?discord="+result+"#integrations", http.StatusSeeOther)
}

// HandleDiscordStart (GET /auth/discord/start) sends the signed-in user to Discord to approve
// the identify scope. The state is kept in the cookie session and checked on the way back.
func HandleDiscordStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !discordCanLink() {
		profileDiscordRedirect(w, r, "off")
		return
	}
	state, err := newToken()
	if err != nil {
		profileDiscordRedirect(w, r, "failed")
		return
	}
	if err := middleware.SetSessionValue(w, r, discordStateKey, state); err != nil {
		log.Printf("discord start: %v", err)
		profileDiscordRedirect(w, r, "failed")
		return
	}
	http.Redirect(w, r, discord.AuthorizeURL(discordConf().clientID, discordRedirectURI(), state), http.StatusFound)
}

// HandleDiscordCallback (GET /auth/discord/callback) finishes the link: it checks the state,
// trades the code for the Discord user and saves the link for the signed-in user.
func HandleDiscordCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !discordCanLink() {
		profileDiscordRedirect(w, r, "off")
		return
	}
	want, _ := middleware.TakeSessionValue(w, r, discordStateKey)
	got := r.URL.Query().Get("state")
	if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		profileDiscordRedirect(w, r, "failed")
		return
	}
	if r.URL.Query().Get("error") != "" {
		profileDiscordRedirect(w, r, "cancelled")
		return
	}
	code := r.URL.Query().Get("code")
	userID, _ := r.Context().Value("user_id").(string)
	if code == "" || userID == "" {
		profileDiscordRedirect(w, r, "failed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	c := discordConf()
	token, err := discord.ExchangeCode(ctx, c.apiBase, c.clientID, c.clientSecret, code, discordRedirectURI())
	if err != nil {
		log.Printf("discord callback: %v", err)
		profileDiscordRedirect(w, r, "failed")
		return
	}
	du, err := discord.OAuthUser(ctx, c.apiBase, token)
	if err != nil {
		log.Printf("discord callback: %v", err)
		profileDiscordRedirect(w, r, "failed")
		return
	}
	name := du.GlobalName
	if name == "" {
		name = du.Username
	}
	if err := db.SaveDiscordLink(userID, du.ID, name); err != nil {
		if errors.Is(err, db.ErrDiscordAccountTaken) {
			profileDiscordRedirect(w, r, "taken")
			return
		}
		log.Printf("discord callback: %v", err)
		profileDiscordRedirect(w, r, "failed")
		return
	}
	profileDiscordRedirect(w, r, "linked")
}
