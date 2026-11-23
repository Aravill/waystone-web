package api

import (
	"context"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"waystone-web/db"
	"waystone-web/discord"
	"waystone-web/models"
)

var discordGuildRe = regexp.MustCompile(`^\d{17,20}$`)

// discordChannelRef is the game's channel for the API ({id, name}), or nil when Discord is off
// or the game has none.
func discordChannelRef(c models.Campaign) interface{} {
	if !discordEnabled() || c.DiscordChannelID == "" {
		return nil
	}
	return map[string]string{"id": c.DiscordChannelID, "name": c.DiscordChannelName}
}

// HandleAdminDiscordPage serves admin-discord.html (server-side admin gate).
func HandleAdminDiscordPage(w http.ResponseWriter, r *http.Request) {
	caller, err := getAdminCaller(r)
	if err != nil || caller == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, "./static/admin-discord.html")
}

func discordMissingEnv() []string {
	c := discordConf()
	missing := []string{}
	for _, m := range []struct {
		name string
		ok   bool
	}{
		{"DISCORD_BOT_TOKEN", c.token != ""},
		{"DISCORD_CLIENT_ID", c.clientID != ""},
		{"DISCORD_CLIENT_SECRET", c.clientSecret != ""},
		{"PUBLIC_URL", c.publicURL != ""},
	} {
		if !m.ok {
			missing = append(missing, m.name)
		}
	}
	return missing
}

func discordStatusRank(s models.CampaignStatus) int {
	switch s {
	case models.Pitch:
		return 0
	case models.Ongoing:
		return 1
	case models.OnHiatus:
		return 2
	case models.Finished:
		return 3
	}
	return 4
}

// HandleAdminDiscord serves GET /api/admin/discord (setup, games and outbox) and
// PUT /api/admin/discord ({guild_id, invite_url, events}).
func HandleAdminDiscord(w http.ResponseWriter, r *http.Request) {
	if caller, err := getAdminCaller(r); err != nil || caller == nil {
		adminForbidden(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		getAdminDiscord(w, r)
	case http.MethodPut:
		putAdminDiscord(w, r)
	default:
		adminError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func getAdminDiscord(w http.ResponseWriter, r *http.Request) {
	settings, err := db.GetDiscordSettings()
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	options := make([]map[string]string, 0, len(models.DiscordEvents))
	for _, e := range models.DiscordEvents {
		options = append(options, map[string]string{"key": e.Key, "label": e.Label})
	}
	events := settings.Events
	if events == nil {
		events = []string{}
	}
	out := map[string]interface{}{
		"enabled": discordEnabled(), "missing_env": discordMissingEnv(),
		"guild_id": settings.GuildID, "guild_name": "", "invite_url": settings.InviteURL,
		"events": events, "event_options": options,
		"channels": []map[string]string{}, "channels_error": "",
		"games":  []map[string]string{},
		"outbox": map[string]interface{}{"queued": 0, "failures": []map[string]interface{}{}},
	}
	if !discordEnabled() {
		writeJSON(w, http.StatusOK, out)
		return
	}
	if settings.GuildID != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		client := discordConf().client
		channels := []map[string]string{}
		if g, err := client.GetGuild(ctx, settings.GuildID); err != nil {
			out["channels_error"] = "Could not reach the server: " + err.Error()
		} else {
			out["guild_name"] = g.Name
			if chans, err := client.PostableChannels(ctx, settings.GuildID); err != nil {
				out["channels_error"] = "Could not list channels: " + err.Error()
			} else {
				for _, c := range chans {
					channels = append(channels, map[string]string{"id": c.ID, "name": c.Name})
				}
			}
		}
		out["channels"] = channels
	}
	campaigns, err := db.GetAllCampaigns()
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to load games")
		return
	}
	sort.SliceStable(campaigns, func(i, j int) bool {
		ri, rj := discordStatusRank(campaigns[i].Status), discordStatusRank(campaigns[j].Status)
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(campaigns[i].Title) < strings.ToLower(campaigns[j].Title)
	})
	titles := map[string]string{}
	games := make([]map[string]string, 0, len(campaigns))
	for _, c := range campaigns {
		titles[c.ID] = c.Title
		games = append(games, map[string]string{
			"id": c.ID, "title": c.Title, "status": string(c.Status),
			"channel_id": c.DiscordChannelID, "channel_name": c.DiscordChannelName,
		})
	}
	out["games"] = games
	queued, failed, err := db.GetDiscordOutboxSummary(20)
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to load the outbox")
		return
	}
	failures := make([]map[string]interface{}, 0, len(failed))
	for _, f := range failed {
		label := f.Target
		if f.Kind == "dm" {
			if u, err := db.GetUserByID(f.Target); err == nil && u != nil {
				label = u.DisplayName()
			}
		}
		failures = append(failures, map[string]interface{}{
			"id": f.ID, "kind": f.Kind, "target": f.Target, "target_label": label,
			"campaign_title": titles[f.CampaignID], "error": f.LastError,
			"attempts": f.Attempts, "created_at": f.CreatedAt,
		})
	}
	out["outbox"] = map[string]interface{}{"queued": queued, "failures": failures}
	writeJSON(w, http.StatusOK, out)
}

func putAdminDiscord(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GuildID   string   `json:"guild_id"`
		InviteURL string   `json:"invite_url"`
		Events    []string `json:"events"`
	}
	if !codexDecode(w, r, 16<<10, &req) {
		return
	}
	req.GuildID = strings.TrimSpace(req.GuildID)
	req.InviteURL = strings.TrimSpace(req.InviteURL)
	if req.GuildID != "" && !discordGuildRe.MatchString(req.GuildID) {
		adminError(w, http.StatusBadRequest, "The server ID is 17 to 20 digits")
		return
	}
	if req.InviteURL != "" && (!strings.HasPrefix(req.InviteURL, "https://") || len(req.InviteURL) > 200) {
		adminError(w, http.StatusBadRequest, "The invite link must start with https:// and be at most 200 characters")
		return
	}
	known := map[string]bool{}
	for _, e := range models.DiscordEvents {
		known[e.Key] = true
	}
	events := []string{}
	seen := map[string]bool{}
	for _, e := range req.Events {
		if !known[e] {
			adminError(w, http.StatusBadRequest, "unknown event")
			return
		}
		if !seen[e] {
			seen[e] = true
			events = append(events, e)
		}
	}
	old, err := db.GetDiscordSettings()
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	if req.GuildID != "" && req.GuildID != old.GuildID {
		if !discordEnabled() {
			adminError(w, http.StatusBadRequest, "The bot isn't in that server")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if _, err := discordConf().client.GetGuild(ctx, req.GuildID); err != nil {
			adminError(w, http.StatusBadRequest, "The bot isn't in that server")
			return
		}
	}
	cleared, err := db.SaveDiscordSettings(models.DiscordSettings{GuildID: req.GuildID, InviteURL: req.InviteURL, Events: events})
	if err != nil {
		log.Printf("save discord settings: %v", err)
		adminError(w, http.StatusInternalServerError, "failed to save")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "success", "cleared": cleared})
}

// HandleAdminDiscordActions serves PUT /api/admin/discord/games/{id} ({channel_id}) and
// POST /api/admin/discord/games/{id}/test.
func HandleAdminDiscordActions(w http.ResponseWriter, r *http.Request) {
	if caller, err := getAdminCaller(r); err != nil || caller == nil {
		adminForbidden(w)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/discord/"), "/"), "/")
	if len(parts) < 2 || parts[0] != "games" || !discordEnabled() {
		adminError(w, http.StatusNotFound, "not found")
		return
	}
	campaign, err := db.GetCampaignByID(parts[1])
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to load the game")
		return
	}
	if campaign == nil {
		adminError(w, http.StatusNotFound, "game not found")
		return
	}
	switch {
	case len(parts) == 2 && r.Method == http.MethodPut:
		setGameChannel(w, r, campaign)
	case len(parts) == 3 && parts[2] == "test" && r.Method == http.MethodPost:
		testGameChannel(w, r, campaign)
	default:
		adminError(w, http.StatusNotFound, "not found")
	}
}

func setGameChannel(w http.ResponseWriter, r *http.Request, campaign *models.Campaign) {
	var req struct {
		ChannelID string `json:"channel_id"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	id, name := strings.TrimSpace(req.ChannelID), ""
	if id != "" {
		settings, err := db.GetDiscordSettings()
		if err != nil {
			adminError(w, http.StatusInternalServerError, "failed to load settings")
			return
		}
		if settings.GuildID == "" {
			adminError(w, http.StatusBadRequest, "Set the server ID first")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		channels, err := discordConf().client.PostableChannels(ctx, settings.GuildID)
		if err != nil {
			adminError(w, http.StatusBadGateway, "Could not list the channels: "+err.Error())
			return
		}
		found := false
		for _, c := range channels {
			if c.ID == id {
				found, name = true, c.Name
			}
		}
		if !found {
			adminError(w, http.StatusBadRequest, "The bot can't post in that channel")
			return
		}
	}
	if err := db.SetCampaignDiscordChannel(campaign.ID, id, name); err != nil {
		adminError(w, http.StatusInternalServerError, "failed to save the channel")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"channel_id": id, "channel_name": name})
}

func testGameChannel(w http.ResponseWriter, r *http.Request, campaign *models.Campaign) {
	if campaign.DiscordChannelID == "" {
		adminError(w, http.StatusConflict, "This game has no channel")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	payload := discordPayload("**"+mdEscape(campaign.Title)+"**: test message from Waystone", discordButtonFor("Open the game", "/games/"+campaign.ID))
	if err := discordConf().client.SendMessage(ctx, campaign.DiscordChannelID, payload); err != nil {
		msg := "Discord didn't accept the message"
		if de, ok := err.(*discord.Error); ok && de.Message != "" {
			msg += ": " + de.Message
		}
		adminError(w, http.StatusBadGateway, msg)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}
