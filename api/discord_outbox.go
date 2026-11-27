package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
	"waystone-web/db"
	"waystone-web/discord"
	"waystone-web/models"
)

var discordBackoff = []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour}

// discordStatus is the bot's custom status while the gateway connection is up.
const discordStatus = "Handling tavern business"

const (
	discordGiveUp     = 6 * time.Hour
	discordFailedKeep = 7 * 24 * time.Hour
)

// StartDiscordSender drains discord_outbox in id order until ctx is done. It wakes when rows are
// queued and every 30 seconds (retries), and prunes old failed rows hourly. It does nothing when
// Discord is off.
func StartDiscordSender(ctx context.Context) {
	if !discordEnabled() {
		log.Printf("discord: DISCORD_BOT_TOKEN not set, Discord notifications are off")
		return
	}
	go discord.RunGateway(ctx, discordConf().apiBase, discordConf().token, discordStatus)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		var lastPrune time.Time
		for {
			drainDiscordOutbox(ctx)
			if time.Since(lastPrune) >= time.Hour {
				lastPrune = time.Now()
				if err := db.PruneDiscordOutbox(time.Now().UTC().Add(-discordFailedKeep).Format(time.RFC3339)); err != nil {
					log.Printf("discord outbox prune: %v", err)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-discordWake:
			case <-ticker.C:
			}
		}
	}()
}

func drainDiscordOutbox(ctx context.Context) {
	for ctx.Err() == nil {
		row, err := db.NextDiscordOutbox(time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			log.Printf("discord outbox: %v", err)
			return
		}
		if row == nil {
			return
		}
		handleDiscordRow(ctx, *row)
	}
}

func handleDiscordRow(ctx context.Context, row models.DiscordOutboxRow) {
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if strings.HasPrefix(row.Kind, "event_") {
		handleDiscordEventRow(sendCtx, row)
		return
	}
	var err error
	if row.Kind == "dm" {
		err = sendDM(sendCtx, row.Target, []byte(row.Payload))
		if errors.Is(err, errNoDiscordLink) {
			// Disconnected meanwhile: nothing to send to.
			discordSettle(row, nil)
			return
		}
	} else {
		err = discordConf().client.SendMessage(sendCtx, row.Target, []byte(row.Payload))
	}
	if ctx.Err() != nil {
		return // shutting down: leave the row queued
	}
	discordSettle(row, err)
}

func discordFail(row models.DiscordOutboxRow, msg string) {
	if err := db.UpdateDiscordOutbox(row.ID, "failed", row.Attempts, row.NextAttemptAt, msg); err != nil {
		log.Printf("discord outbox: %v", err)
	}
}

// discordSettle records the outcome of one send attempt. It reports whether the row has now
// failed for good (it is retried otherwise, or was sent).
func discordSettle(row models.DiscordOutboxRow, err error) bool {
	if err == nil {
		if e := db.DeleteDiscordOutbox(row.ID); e != nil {
			log.Printf("discord outbox: %v", e)
		}
		return false
	}
	now := time.Now().UTC()
	var de *discord.Error
	if errors.As(err, &de) {
		switch {
		case de.Status == http.StatusTooManyRequests && de.RetryAfter > 0:
			// Rate limited: try again after the wait, without counting an attempt.
			if e := db.UpdateDiscordOutbox(row.ID, "queued", row.Attempts, now.Add(de.RetryAfter).Format(time.RFC3339), de.Message); e != nil {
				log.Printf("discord outbox: %v", e)
			}
			return false
		case row.Kind == "dm" && de.Code == discord.CodeCannotDM:
			discordFail(row, "Discord can't DM this user (50007)")
			return true
		case row.Kind == "channel" && (de.Code == discord.CodeUnknownChannel || de.Code == discord.CodeMissingAccess || de.Status == http.StatusNotFound):
			discordFail(row, "channel gone")
			discordChannelLost(row)
			return true
		case de.Status >= 400 && de.Status < 500 && de.Status != http.StatusUnauthorized && de.Status != http.StatusRequestTimeout:
			// Includes 50013 Missing Permissions: the row fails but the channel stays.
			discordFail(row, de.Error())
			return true
		}
	}
	// Network error, 5xx or 401: retry with backoff until the row is too old.
	attempts := row.Attempts + 1
	created, perr := time.Parse(time.RFC3339, row.CreatedAt)
	if perr == nil && now.Sub(created) >= discordGiveUp {
		if e := db.UpdateDiscordOutbox(row.ID, "failed", attempts, row.NextAttemptAt, err.Error()); e != nil {
			log.Printf("discord outbox: %v", e)
		}
		return true
	}
	idx := attempts - 1
	if idx >= len(discordBackoff) {
		idx = len(discordBackoff) - 1
	}
	if e := db.UpdateDiscordOutbox(row.ID, "queued", attempts, now.Add(discordBackoff[idx]).Format(time.RFC3339), err.Error()); e != nil {
		log.Printf("discord outbox: %v", e)
	}
	return false
}

// discordChannelLost clears a game's vanished channel (which turns its events off) and tells every admin.
func discordChannelLost(row models.DiscordOutboxRow) {
	cleared, err := db.ClearLostDiscordChannel(row.CampaignID, row.Target)
	if err != nil {
		log.Printf("discord channel lost %s: %v", row.CampaignID, err)
		return
	}
	if !cleared {
		return
	}
	campaign, err := db.GetCampaignByID(row.CampaignID)
	if err != nil || campaign == nil {
		return
	}
	notifyDiscordAdmins(campaign, nil)
}

var errNoDiscordLink = errors.New("user has no Discord link")

// sendDM sends a message to a linked user's DM channel, opening the channel on first use and
// re-opening it once when the cached one is gone. A refused DM (50007) is recorded on the link;
// a later success clears it.
func sendDM(ctx context.Context, userID string, payload []byte) error {
	client := discordConf().client
	link, err := db.GetDiscordLink(userID)
	if err != nil {
		return err
	}
	if link == nil {
		return errNoDiscordLink
	}
	channelID, fresh := link.DMChannelID, false
	open := func() error {
		id, err := client.CreateDM(ctx, link.DiscordUserID)
		if err != nil {
			return err
		}
		channelID, fresh = id, true
		return db.SetDiscordDMChannel(userID, id)
	}
	if channelID == "" {
		err = open()
	}
	if err == nil {
		err = client.SendMessage(ctx, channelID, payload)
		if err != nil && !fresh && (discord.IsStatus(err, http.StatusNotFound) || discord.HasCode(err, discord.CodeUnknownChannel)) {
			if err = open(); err == nil {
				err = client.SendMessage(ctx, channelID, payload)
			}
		}
	}
	if err != nil {
		if discord.HasCode(err, discord.CodeCannotDM) {
			if e := db.SetDiscordDMError(userID, "Discord refused the DM"); e != nil {
				log.Printf("discord dm error: %v", e)
			}
		}
		return err
	}
	if link.DMError != "" {
		if e := db.SetDiscordDMError(userID, ""); e != nil {
			log.Printf("discord dm error: %v", e)
		}
	}
	return nil
}
