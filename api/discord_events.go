package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"waystone-web/db"
	"waystone-web/discord"
	"waystone-web/models"
)

// Discord scheduled events (docs/design/discord-events.md). Every Discord call goes through the
// outbox, in three row kinds whose target is the session id:
//   - event_create: built at send time from the game and session as they are then; on success the
//     "session confirmed" channel post is queued with the event's link, and when it fails for good
//     (or there's nothing to create any more) the post goes out without one.
//   - event_status: {guild, event, status} moves an event to active or completed.
//   - event_delete: {guild, event, complete} deletes a scheduled event, or completes one that has
//     already started (Discord can't cancel an active event).
// discord_events.status is the state the app asked for, set when the row is queued, so the minute
// ticker never queues the same change twice.

// discordEventDefaultLength is how long an event runs when its session has no duration.
const discordEventDefaultLength = 4 * time.Hour

type discordEventChange struct {
	Guild    string `json:"guild"`
	Event    string `json:"event"`
	Status   int    `json:"status,omitempty"`
	Complete bool   `json:"complete,omitempty"`
}

func queueDiscordEventRow(kind, sessionID, campaignID string, payload interface{}) {
	body := []byte("{}")
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	row := models.DiscordOutboxRow{Kind: kind, Target: sessionID, CampaignID: campaignID, Payload: string(body), NextAttemptAt: now, CreatedAt: now}
	if err := db.EnqueueDiscord([]models.DiscordOutboxRow{row}); err != nil {
		log.Printf("discord %s %s: %v", kind, sessionID, err)
		return
	}
	wakeDiscord()
}

// discordEventsFor reports whether confirming a session of c should create an event now.
func discordEventsFor(c *models.Campaign) bool {
	if !discordEnabled() || c == nil || c.DiscordChannelID == "" {
		return false
	}
	settings, err := db.GetDiscordSettings()
	if err != nil {
		log.Printf("discord events: %v", err)
		return false
	}
	g, err := db.GetDiscordGameEvents(c.ID)
	if err != nil {
		log.Printf("discord events: %v", err)
		return false
	}
	return discordEventsReady(c, g, settings.GuildID)
}

// sessionEnd is when an event of the session ends: its start plus the duration, or plus four hours.
func sessionEnd(s models.CampaignSession, start time.Time) time.Time {
	if s.Duration > 0 {
		return start.Add(time.Duration(s.Duration) * time.Minute)
	}
	return start.Add(discordEventDefaultLength)
}

// discordEventLink is the session's live event link, or "".
func discordEventLink(sessionID string) string {
	e, err := db.GetDiscordEvent(sessionID)
	if err != nil || e == nil || (e.Status != models.DiscordEventScheduled && e.Status != models.DiscordEventActive) {
		return ""
	}
	return discord.EventURL(e.GuildID, e.EventID)
}

// cancelDiscordEvent removes a session's event: a queued creation is dropped, a scheduled event is
// deleted on Discord and one that already started is completed.
func cancelDiscordEvent(session models.CampaignSession) {
	if !discordEnabled() {
		return
	}
	if _, err := db.DropQueuedDiscordEventCreate(session.ID); err != nil {
		log.Printf("discord event cancel %s: %v", session.ID, err)
	}
	e, err := db.GetDiscordEvent(session.ID)
	if err != nil || e == nil {
		return
	}
	queueDiscordEventRemoval(*e)
}

func queueDiscordEventRemoval(e models.DiscordEvent) {
	if e.Status != models.DiscordEventScheduled && e.Status != models.DiscordEventActive {
		return
	}
	if err := db.SetDiscordEventStatus(e.SessionID, models.DiscordEventCancelled); err != nil {
		log.Printf("discord event cancel %s: %v", e.SessionID, err)
		return
	}
	queueDiscordEventRow("event_delete", e.SessionID, e.CampaignID, discordEventChange{Guild: e.GuildID, Event: e.EventID, Complete: e.Status == models.DiscordEventActive})
}

// dropUnstartedDiscordEvents deletes the game's events that haven't started (the game ended).
func dropUnstartedDiscordEvents(campaignID string) {
	if !discordEnabled() {
		return
	}
	events, err := db.GetDiscordEventsByStatus(campaignID, models.DiscordEventScheduled)
	if err != nil {
		log.Printf("discord events of %s: %v", campaignID, err)
		return
	}
	for _, e := range events {
		queueDiscordEventRemoval(e)
	}
}

// liveDiscordEvents returns the game's scheduled and started events, read before the game is deleted.
func liveDiscordEvents(campaignID string) []models.DiscordEvent {
	if !discordEnabled() {
		return nil
	}
	events, err := db.GetDiscordEventsByStatus(campaignID, models.DiscordEventScheduled, models.DiscordEventActive)
	if err != nil {
		log.Printf("discord events of %s: %v", campaignID, err)
		return nil
	}
	return events
}

// removeDeletedGameDiscordEvents queues the Discord side of a deleted game's events. DeleteCampaign
// drops the game's queued outbox rows and its discord_events rows, so this runs after it.
func removeDeletedGameDiscordEvents(events []models.DiscordEvent) {
	for _, e := range events {
		queueDiscordEventRow("event_delete", e.SessionID, e.CampaignID, discordEventChange{Guild: e.GuildID, Event: e.EventID, Complete: e.Status == models.DiscordEventActive})
	}
}

// advanceDiscordEvents (minute ticker) starts events whose session has begun and completes those
// whose session is over.
func advanceDiscordEvents(now time.Time) {
	if !discordEnabled() {
		return
	}
	events, err := db.GetDiscordEventsByStatus("", models.DiscordEventScheduled, models.DiscordEventActive)
	if err != nil {
		log.Printf("discord events: %v", err)
		return
	}
	for _, e := range events {
		session, err := db.GetSessionByID(e.SessionID)
		if err != nil {
			log.Printf("discord events: session %s: %v", e.SessionID, err)
			continue
		}
		if session == nil {
			continue
		}
		start, ok := sessionStart(*session)
		if !ok || now.Before(start) {
			continue
		}
		change := discordEventChange{Guild: e.GuildID, Event: e.EventID}
		if now.Before(sessionEnd(*session, start)) {
			if e.Status == models.DiscordEventScheduled {
				if db.SetDiscordEventStatus(e.SessionID, models.DiscordEventActive) == nil {
					change.Status = discord.EventActive
					queueDiscordEventRow("event_status", e.SessionID, e.CampaignID, change)
				}
			}
			continue
		}
		if db.SetDiscordEventStatus(e.SessionID, models.DiscordEventCompleted) != nil {
			continue
		}
		if e.Status == models.DiscordEventScheduled {
			// Discord only completes an active event.
			change.Status = discord.EventActive
			queueDiscordEventRow("event_status", e.SessionID, e.CampaignID, change)
		}
		change.Status = discord.EventCompleted
		queueDiscordEventRow("event_status", e.SessionID, e.CampaignID, change)
	}
}

// handleDiscordEventRow sends one event_* outbox row.
func handleDiscordEventRow(ctx context.Context, row models.DiscordOutboxRow) {
	client := discordConf().client
	switch row.Kind {
	case "event_create":
		createDiscordEvent(ctx, row)
	case "event_status", "event_delete":
		var c discordEventChange
		if err := json.Unmarshal([]byte(row.Payload), &c); err != nil || c.Guild == "" || c.Event == "" {
			discordFail(row, "bad event payload")
			return
		}
		var err error
		switch {
		case row.Kind == "event_status":
			err = client.SetScheduledEventStatus(ctx, c.Guild, c.Event, c.Status)
		case c.Complete:
			err = client.SetScheduledEventStatus(ctx, c.Guild, c.Event, discord.EventCompleted)
		default:
			err = client.DeleteScheduledEvent(ctx, c.Guild, c.Event)
		}
		if ctx.Err() != nil {
			return
		}
		if discord.HasCode(err, discord.CodeUnknownEvent) {
			err = nil // someone already removed it on Discord
		}
		discordSettle(row, err)
	default:
		discordFail(row, "unknown row kind")
	}
}

// createDiscordEvent creates the session's event and then queues the "session confirmed" post.
func createDiscordEvent(ctx context.Context, row models.DiscordOutboxRow) {
	sessionID := row.Target
	session, err := db.GetSessionByID(sessionID)
	if err != nil {
		discordSettle(row, err)
		return
	}
	campaign, err := db.GetCampaignByID(row.CampaignID)
	if err != nil {
		discordSettle(row, err)
		return
	}
	if session == nil || campaign == nil || session.Status != models.SessionConfirmed || session.Manual {
		discordSettle(row, nil) // cancelled or gone meanwhile: no event, no post
		return
	}
	postWithoutLink := func() {
		discordSettle(row, nil)
		postSessionConfirmedNow(campaign, *session, "")
	}
	if link := discordEventLink(sessionID); link != "" {
		discordSettle(row, nil)
		postSessionConfirmedNow(campaign, *session, link)
		return
	}
	settings, err := db.GetDiscordSettings()
	if err != nil {
		discordSettle(row, err)
		return
	}
	g, err := db.GetDiscordGameEvents(campaign.ID)
	if err != nil {
		discordSettle(row, err)
		return
	}
	start, ok := sessionStart(*session)
	if !discordEventsReady(campaign, g, settings.GuildID) || !ok || !start.After(time.Now().Add(time.Minute)) {
		postWithoutLink()
		return
	}

	desc := strings.TrimSpace(campaign.Summary)
	if url := discordURL(sessionLinkPath(campaign, *session)); url != "" {
		const room = 1000 - 2
		if r := []rune(desc); len(r)+len(url) > room {
			keep := room - len(url) - 1
			if keep < 0 {
				keep = 0
			}
			desc = strings.TrimSpace(string(r[:keep])) + "…"
		}
		desc = strings.TrimSpace(desc + "\n\n" + url)
	} else if r := []rune(desc); len(r) > 1000 {
		desc = string(r[:999]) + "…"
	}
	name := campaign.Title
	if r := []rune(name); len(r) > 100 {
		name = string(r[:100])
	}
	ev := discord.ScheduledEvent{
		ChannelID:          g.VoiceID,
		Name:               name,
		Description:        desc,
		ScheduledStartTime: start.UTC().Format(time.RFC3339),
		ScheduledEndTime:   sessionEnd(*session, start).UTC().Format(time.RFC3339),
		PrivacyLevel:       2, // guild only
		EntityType:         2, // voice
		Image:              eventCoverDataURI(g.Cover),
	}
	created, err := discordConf().client.CreateScheduledEvent(ctx, settings.GuildID, ev)
	if ctx.Err() != nil {
		return // shutting down: leave the row queued
	}
	if err != nil {
		if discord.HasCode(err, discord.CodeUnknownChannel, discord.CodeMissingAccess) {
			discordFail(row, "voice channel gone")
			discordVoiceLost(campaign, g.VoiceID)
			postSessionConfirmedNow(campaign, *session, "")
			return
		}
		if discordSettle(row, err) {
			postSessionConfirmedNow(campaign, *session, "") // failed for good: the post goes out anyway
		}
		return
	}
	guild := created.GuildID
	if guild == "" {
		guild = settings.GuildID
	}
	stored := models.DiscordEvent{SessionID: sessionID, CampaignID: campaign.ID, GuildID: guild, EventID: created.ID, Status: models.DiscordEventScheduled}
	if err := db.SaveDiscordEvent(stored); err != nil {
		log.Printf("discord event %s: %v", sessionID, err)
	}
	discordSettle(row, nil)
	// The session may have been cancelled while Discord was answering.
	if now, err := db.GetSessionByID(sessionID); err == nil && (now == nil || now.Status != models.SessionConfirmed) {
		queueDiscordEventRemoval(stored)
		return
	}
	postSessionConfirmedNow(campaign, *session, discord.EventURL(guild, created.ID))
}

// sessionLinkPath is the session's page in Waystone (where a "session confirmed" notification points).
func sessionLinkPath(c *models.Campaign, s models.CampaignSession) string {
	n := sessionNotification(models.NotificationSessionConfirmed, s)
	n.CampaignID = c.ID
	path, _ := notificationTarget(n)
	return path
}

// eventCoverDataURI reads a cover from the upload directory as a data URI, or "" without one.
func eventCoverDataURI(file string) string {
	if file == "" || !db.UploadFileRe.MatchString(file) {
		return ""
	}
	dir, err := db.UploadDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		log.Printf("discord event cover %s: %v", file, err)
		return ""
	}
	mime := http.DetectContentType(data)
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/webp" {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// discordVoiceLost clears a game's vanished voice channel, which turns its events off, and tells every admin.
func discordVoiceLost(campaign *models.Campaign, voiceID string) {
	cleared, err := db.ClearLostDiscordVoice(campaign.ID, voiceID)
	if err != nil {
		log.Printf("discord voice lost %s: %v", campaign.ID, err)
		return
	}
	if cleared {
		notifyDiscordAdmins(campaign, map[string]string{"channel": "voice"})
	}
}

// notifyDiscordAdmins sends every admin a discord_channel_lost notice about the game.
func notifyDiscordAdmins(campaign *models.Campaign, data map[string]string) {
	users, err := db.GetAllUsers()
	if err != nil {
		log.Printf("discord channel lost: %v", err)
		return
	}
	var admins []string
	for i := range users {
		if users[i].IsAdmin() && !users[i].Deleted && !users[i].Blocked {
			admins = append(admins, users[i].ID)
		}
	}
	notify("", admins, models.NotificationDiscordChannelLost, campaign, "", data)
}
