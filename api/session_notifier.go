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
// The DM is deliberately not notified. It also runs remindUpcomingSessions, remindOpenPolls and
// postDuePolls, and advanceDiscordEvents. Errors are logged, never fatal.
func StartSessionStartNotifier(ctx context.Context) {
	go func() {
		now := time.Now()
		notifyStartedSessions(now)
		remindUpcomingSessions(now)
		remindOpenPolls(now)
		postDuePolls(now)
		advanceDiscordEvents(now)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				notifyStartedSessions(now)
				remindUpcomingSessions(now)
				remindOpenPolls(now)
				postDuePolls(now)
				advanceDiscordEvents(now)
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

// sessionReminderLead is how long before a session's start its reminder goes out.
const sessionReminderLead = 24 * time.Hour

// remindUpcomingSessions sends a session_reminder 24 hours before each Confirmed, non-manual
// session of a running game, to its players and followers (not the DM), and queues one channel
// post. A session confirmed less than 24 hours before it starts gets none: the confirmation was
// the notice. The notifications table is the dedupe; when nobody is notified, a row with an empty
// user_id marks the session as done.
func remindUpcomingSessions(now time.Time) {
	utc := now.UTC()
	candidates, err := db.GetUnremindedConfirmedSessions(
		utc.AddDate(0, 0, -1).Format("2006-01-02"),
		utc.AddDate(0, 0, 2).Format("2006-01-02"),
	)
	if err != nil {
		log.Printf("session reminder: %v", err)
		return
	}
	campaigns := map[string]*models.Campaign{}
	for _, session := range candidates {
		if session.Manual {
			continue
		}
		start, ok := sessionStart(session)
		if !ok || now.Before(start.Add(-sessionReminderLead)) || !now.Before(start) {
			continue
		}
		if confirmed, ok := parseStoredTime(session.UpdatedAt); ok && confirmed.After(start.Add(-sessionReminderLead)) {
			continue
		}
		campaign, seen := campaigns[session.CampaignID]
		if !seen {
			if campaign, err = db.GetCampaignByID(session.CampaignID); err != nil {
				log.Printf("session reminder: campaign %s: %v", session.CampaignID, err)
			}
			campaigns[session.CampaignID] = campaign
		}
		if campaign == nil || campaign.Status == models.Finished || campaign.Status == models.Cancelled {
			continue
		}
		data := map[string]string{"date": session.Date, "time": session.Time, "timezone": session.Timezone}
		n := notify("", withoutID(campaignFollowerIDs(campaign), campaign.DM), models.NotificationSessionReminder, campaign, session.ID, data)
		if n < 0 {
			continue // try again next minute
		}
		if n == 0 {
			marker := models.Notification{
				ID:         db.GenerateUUID(),
				Type:       models.NotificationSessionReminder,
				CampaignID: campaign.ID,
				SessionID:  session.ID,
				Data:       data,
				CreatedAt:  now.UTC().Format(time.RFC3339),
			}
			if err := db.CreateNotifications([]models.Notification{marker}); err != nil {
				log.Printf("session reminder marker: %v", err)
				continue
			}
		}
		postSessionReminder(campaign, session)
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

// pollReminderWindow is how close a poll's first upcoming date must be for the reminder, and
// how long before that date the poll must have been posted for it to be sent at all.
const pollReminderWindow = 120 * time.Hour

// remindOpenPolls sends each player a poll_reminder, once per poll, when the poll's earliest
// upcoming date is within five days and the poll was posted more than five days before it (a
// late poll needs none: its "suggested" notification is enough). Only players who have not
// answered every upcoming date are reminded. The notifications table is the dedupe.
func remindOpenPolls(now time.Time) {
	utc := now.UTC()
	candidates, err := db.GetPollReminderCandidates(
		utc.AddDate(0, 0, -1).Format("2006-01-02"),
		utc.AddDate(0, 0, 6).Format("2006-01-02"),
	)
	if err != nil {
		log.Printf("poll reminder: %v", err)
		return
	}
	byPoll := map[string][]models.CampaignSession{}
	pollOrder := []string{}
	for _, s := range candidates {
		if s.Manual {
			continue
		}
		if _, ok := byPoll[s.PollID]; !ok {
			pollOrder = append(pollOrder, s.PollID)
		}
		byPoll[s.PollID] = append(byPoll[s.PollID], s)
	}

	type duePoll struct {
		campaign *models.Campaign
		upcoming []models.CampaignSession
	}
	campaigns := map[string]*models.Campaign{}
	var due []duePoll
	dueIDs := []string{}
	for _, pollID := range pollOrder {
		dates := byPoll[pollID]
		upcoming := upcomingDates(dates, now)
		if len(upcoming) == 0 {
			continue
		}
		firstStart, ok := sessionStart(upcoming[0])
		if !ok || firstStart.Sub(now) > pollReminderWindow {
			continue
		}
		posted, ok := pollPostedAt(dates)
		if !ok || firstStart.Sub(posted) <= pollReminderWindow {
			continue
		}
		campaignID := dates[0].CampaignID
		campaign, seen := campaigns[campaignID]
		if !seen {
			if campaign, err = db.GetCampaignByID(campaignID); err != nil {
				log.Printf("poll reminder: campaign %s: %v", campaignID, err)
			}
			campaigns[campaignID] = campaign
		}
		if campaign == nil || campaign.Status == models.Finished || campaign.Status == models.Cancelled {
			continue
		}
		due = append(due, duePoll{campaign, upcoming})
		dueIDs = append(dueIDs, pollID)
	}
	if len(due) == 0 {
		return
	}

	reminded, err := db.GetPollReminderUserIDs(dueIDs)
	if err != nil {
		log.Printf("poll reminder: %v", err)
		return
	}
	for _, p := range due {
		ids := make([]string, 0, len(p.upcoming))
		for _, d := range p.upcoming {
			ids = append(ids, d.ID)
		}
		responses, err := db.GetSessionResponsesBySessionIDs(ids)
		if err != nil {
			log.Printf("poll reminder: %v", err)
			continue
		}
		pollID := p.upcoming[0].PollID
		recipients := []string{}
		for _, player := range p.campaign.Players {
			if reminded[pollID][player] {
				continue
			}
			for _, d := range p.upcoming {
				replied := false
				for _, resp := range responses[d.ID] {
					if resp.UserID == player {
						replied = true
						break
					}
				}
				if !replied {
					recipients = append(recipients, player)
					break
				}
			}
		}
		notifyPoll("", recipients, models.NotificationPollReminder, p.campaign, p.upcoming[0], p.upcoming, nil)
	}
}

// pollPostedAt is when a poll was posted: the earliest created_at among its current dates.
func pollPostedAt(dates []models.CampaignSession) (time.Time, bool) {
	var earliest time.Time
	found := false
	for _, d := range dates {
		t, ok := parseStoredTime(d.CreatedAt)
		if !ok {
			continue
		}
		if !found || t.Before(earliest) {
			earliest, found = t, true
		}
	}
	return earliest, found
}

// parseStoredTime reads a stored timestamp: RFC3339 or SQLite's "2006-01-02 15:04:05" (UTC).
func parseStoredTime(v string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02 15:04:05", v); err == nil {
		return t, true
	}
	return time.Time{}, false
}
