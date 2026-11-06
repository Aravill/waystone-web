package api

import (
	"context"
	"log"
	"time"
	"waystone-web/db"
	"waystone-web/models"
)

// StartSessionStartNotifier tells a game's players and followers when a confirmed session
// begins. It runs once now and then every minute until ctx is done. The notifications table
// is the dedupe (one session_started row per session), so a restart never repeats one.
// The DM is deliberately not notified. Errors are logged, never fatal.
func StartSessionStartNotifier(ctx context.Context) {
	go func() {
		notifyStartedSessions(time.Now())
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				notifyStartedSessions(now)
			}
		}
	}()
}

func notifyStartedSessions(now time.Time) {
	utc := now.UTC()
	candidates, err := db.GetUnannouncedConfirmedSessions(
		utc.AddDate(0, 0, -2).Format("2006-01-02"),
		utc.AddDate(0, 0, 1).Format("2006-01-02"),
	)
	if err != nil {
		log.Printf("session start notifier: %v", err)
		return
	}
	for _, session := range candidates {
		if session.Manual {
			continue
		}
		start, ok := sessionStart(session)
		if !ok || start.After(now) || !now.Before(start.Add(time.Duration(session.Duration)*time.Minute)) {
			continue
		}
		campaign, err := db.GetCampaignByID(session.CampaignID)
		if err != nil {
			log.Printf("session start notifier: campaign %s: %v", session.CampaignID, err)
			continue
		}
		if campaign == nil || campaign.Status == models.Finished || campaign.Status == models.Cancelled {
			continue
		}
		notify("", withoutID(campaignFollowerIDs(campaign), campaign.DM), models.NotificationSessionStarted, campaign, session.ID, map[string]string{
			"date":     session.Date,
			"time":     session.Time,
			"timezone": session.Timezone,
		})
	}
}

// withoutID returns ids minus every occurrence of drop.
func withoutID(ids []string, drop string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != drop {
			out = append(out, id)
		}
	}
	return out
}
