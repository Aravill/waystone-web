package api

import (
	"context"
	"log"
	"net/http"
	"time"
	"waystone-web/db"
	"waystone-web/discord"
)

// HandleProfileDiscord serves the signed-in user's own Discord link on Profile:
// GET (state), PUT {sessions, signups, characters} (DM toggles) and DELETE (disconnect).
func HandleProfileDiscord(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value("user_id").(string)
	switch r.Method {
	case http.MethodGet:
		getProfileDiscord(w, r, userID)
	case http.MethodPut:
		var req struct {
			Sessions   bool `json:"sessions"`
			Signups    bool `json:"signups"`
			Characters bool `json:"characters"`
		}
		if !codexDecode(w, r, 4<<10, &req) {
			return
		}
		link, err := db.GetDiscordLink(userID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load the Discord link")
			return
		}
		if link == nil {
			writeJSONError(w, http.StatusNotFound, "Discord isn't connected")
			return
		}
		if err := db.SetDiscordToggles(userID, req.Sessions, req.Signups, req.Characters); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to save")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
	case http.MethodDelete:
		if err := db.DeleteDiscordLink(userID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to disconnect")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func getProfileDiscord(w http.ResponseWriter, r *http.Request, userID string) {
	out := map[string]interface{}{
		"enabled": discordEnabled(), "can_link": discordCanLink(), "linked": false,
		"username": "", "toggles": map[string]bool{"sessions": true, "signups": true, "characters": true},
		"in_guild": nil, "invite_url": "", "dm_error": "", "dm_error_at": "",
	}
	if !discordEnabled() {
		writeJSON(w, http.StatusOK, out)
		return
	}
	settings, err := db.GetDiscordSettings()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	out["invite_url"] = settings.InviteURL
	link, err := db.GetDiscordLink(userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load the Discord link")
		return
	}
	if link == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["linked"] = true
	out["username"] = link.DiscordUsername
	out["toggles"] = map[string]bool{"sessions": link.DMSessions, "signups": link.DMSignups, "characters": link.DMCharacters}
	out["dm_error"] = link.DMError
	out["dm_error_at"] = link.DMErrorAt
	if settings.GuildID != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if m, err := discordConf().client.GuildMember(ctx, settings.GuildID, link.DiscordUserID); err == nil {
			out["in_guild"] = m != nil
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// HandleProfileDiscordTest (POST /api/profile/discord/test) sends the user a test DM right away.
func HandleProfileDiscordTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !discordEnabled() {
		writeJSONError(w, http.StatusConflict, "Discord isn't set up")
		return
	}
	userID, _ := r.Context().Value("user_id").(string)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	err := sendDM(ctx, userID, discordPayload("Test message from Waystone. DMs work.", "Waystone", discordURL("/profile")))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
	case err == errNoDiscordLink:
		writeJSONError(w, http.StatusConflict, "Discord isn't connected")
	case discord.HasCode(err, discord.CodeCannotDM):
		writeJSONError(w, http.StatusBadGateway, "Discord can't reach you: rejoin the server or allow DMs from server members")
	default:
		log.Printf("discord test dm: %v", err)
		writeJSONError(w, http.StatusBadGateway, "Discord didn't accept the message")
	}
}
