package api

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"waystone-web/db"
	"waystone-web/models"
)

// maxEventCoverBytes keeps a cover inside what Discord accepts for an event image.
const maxEventCoverBytes = 8 << 20

// discordVoiceRef is the game's events voice channel for the game page ({id, name}), or nil when
// Discord is off or the game doesn't get events.
func discordVoiceRef(c models.Campaign, g models.DiscordGameEvents) interface{} {
	if !discordEnabled() || c.DiscordChannelID == "" || !g.On || g.VoiceID == "" {
		return nil
	}
	return map[string]string{"id": g.VoiceID, "name": g.VoiceName}
}

// discordEventsReady reports whether confirming a session of the game should create an event:
// events on, both channels set, a server set and the game still running.
func discordEventsReady(c *models.Campaign, g models.DiscordGameEvents, guildID string) bool {
	return discordEnabled() && c != nil && guildID != "" && c.DiscordChannelID != "" && g.On && g.VoiceID != "" &&
		c.Status != models.Finished && c.Status != models.Cancelled
}

// setGameDiscord serves PUT /api/admin/discord/games/{id}: {channel_id?, events?, voice_channel_id?}.
// Omitted fields keep their value. Events can only be on with both a text and a voice channel.
func setGameDiscord(w http.ResponseWriter, r *http.Request, campaign *models.Campaign) {
	var req struct {
		ChannelID *string `json:"channel_id"`
		Events    *bool   `json:"events"`
		VoiceID   *string `json:"voice_channel_id"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	current, err := db.GetDiscordGameEvents(campaign.ID)
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to load the game")
		return
	}
	channelID, channelName := campaign.DiscordChannelID, campaign.DiscordChannelName
	on, voiceID, voiceName := current.On, current.VoiceID, current.VoiceName
	if req.ChannelID != nil {
		channelID = strings.TrimSpace(*req.ChannelID)
	}
	if req.Events != nil {
		on = *req.Events
	}
	if req.VoiceID != nil {
		voiceID = strings.TrimSpace(*req.VoiceID)
	}
	if on && (channelID == "" || voiceID == "") {
		field := "voice_channel_id"
		if channelID == "" {
			field = "channel_id"
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Pick a text channel and a voice channel before turning events on", "field": field})
		return
	}

	needText := req.ChannelID != nil && channelID != "" && channelID != campaign.DiscordChannelID
	needVoice := req.VoiceID != nil && voiceID != "" && voiceID != current.VoiceID
	if needText || needVoice {
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
		client := discordConf().client
		if needText {
			chans, err := client.PostableChannels(ctx, settings.GuildID)
			if err != nil {
				adminError(w, http.StatusBadGateway, "Could not list the channels: "+err.Error())
				return
			}
			channelName = ""
			for _, c := range chans {
				if c.ID == channelID {
					channelName = c.Name
				}
			}
			if channelName == "" {
				adminError(w, http.StatusBadRequest, "The bot can't post in that channel")
				return
			}
		}
		if needVoice {
			chans, err := client.VoiceChannels(ctx, settings.GuildID)
			if err != nil {
				adminError(w, http.StatusBadGateway, "Could not list the voice channels: "+err.Error())
				return
			}
			voiceName = ""
			for _, c := range chans {
				if c.ID == voiceID {
					voiceName = c.Name
				}
			}
			if voiceName == "" {
				adminError(w, http.StatusBadRequest, "The bot can't see or join that voice channel")
				return
			}
		}
	}
	if channelID == "" {
		channelName = ""
	}
	if voiceID == "" {
		voiceName = ""
	}
	if req.ChannelID != nil && channelID != campaign.DiscordChannelID {
		if err := db.SetCampaignDiscordChannel(campaign.ID, channelID, channelName); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to save the channel")
			return
		}
	}
	if on != current.On || voiceID != current.VoiceID {
		if err := db.SetDiscordGameEvents(campaign.ID, on, voiceID, voiceName); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to save the events")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channel_id": channelID, "channel_name": channelName,
		"events": on, "voice_id": voiceID, "voice_name": voiceName,
	})
}

// gameEventCover serves GET (the image), POST (raw body: PNG, JPEG or WebP, 8 MB at most) and
// DELETE /api/admin/discord/games/{id}/cover.
func gameEventCover(w http.ResponseWriter, r *http.Request, campaign *models.Campaign) {
	current, err := db.GetDiscordGameEvents(campaign.ID)
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to load the game")
		return
	}
	switch r.Method {
	case http.MethodGet:
		dir, err := db.UploadDir()
		if err != nil || current.Cover == "" || !db.UploadFileRe.MatchString(current.Cover) {
			adminError(w, http.StatusNotFound, "no cover")
			return
		}
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, filepath.Join(dir, current.Cover))
	case http.MethodPost:
		file, _, ok := receiveCodexImage(w, r)
		if !ok {
			return
		}
		if strings.HasSuffix(file, ".gif") {
			removeCodexFiles([]string{file})
			adminError(w, http.StatusBadRequest, "The cover must be a PNG, JPEG or WebP image")
			return
		}
		if dir, err := db.UploadDir(); err == nil {
			if st, err := os.Stat(filepath.Join(dir, file)); err == nil && st.Size() > maxEventCoverBytes {
				removeCodexFiles([]string{file})
				writeJSONError(w, http.StatusRequestEntityTooLarge, "The cover must be 8 MB or smaller")
				return
			}
		}
		old, err := db.SetDiscordEventCover(campaign.ID, file)
		if err != nil {
			removeCodexFiles([]string{file})
			adminError(w, http.StatusInternalServerError, "failed to save the cover")
			return
		}
		if old != "" {
			removeCodexFiles([]string{old})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"cover": true})
	case http.MethodDelete:
		old, err := db.SetDiscordEventCover(campaign.ID, "")
		if err != nil {
			adminError(w, http.StatusInternalServerError, "failed to remove the cover")
			return
		}
		if old != "" {
			removeCodexFiles([]string{old})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"cover": false})
	default:
		adminError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// removeAllEventCovers deletes the given games' cover files (after a server change cleared them).
func removeAllEventCovers(games map[string]models.DiscordGameEvents) {
	var files []string
	for _, g := range games {
		if g.Cover != "" {
			files = append(files, g.Cover)
		}
	}
	if len(files) > 0 {
		removeCodexFiles(files)
		log.Printf("discord: removed %d event covers after the server changed", len(files))
	}
}
