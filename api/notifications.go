package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"waystone-web/db"
	"waystone-web/models"
)

// campaignFollowerIDs returns who follows a campaign: its players (implicit)
// plus explicit followers. On a lookup error it logs and returns players only.
func campaignFollowerIDs(c *models.Campaign) []string {
	ids := append([]string(nil), c.Players...)
	followers, err := db.GetCampaignFollowerIDs(c.ID)
	if err != nil {
		log.Printf("campaign followers %s: %v", c.ID, err)
		return ids
	}
	return append(ids, followers...)
}

// notify stores one notification per recipient, skipping "", duplicates and the actor.
// Errors are logged, never returned: notifications must not fail the main action.
// campaign may be nil for notifications that belong to no game.
// It returns how many notifications were written (0 when there was nobody to tell, -1 when the
// write failed). After the inbox rows it queues Discord DMs for recipients who linked Discord.
func notify(actorID string, recipients []string, typ models.NotificationType, campaign *models.Campaign, sessionID string, data map[string]string) int {
	now := time.Now().UTC().Format(time.RFC3339)
	seen := map[string]bool{}
	var batch []models.Notification
	for _, id := range recipients {
		if id == "" || id == actorID || seen[id] {
			continue
		}
		seen[id] = true
		d := map[string]string{}
		for k, v := range data {
			d[k] = v
		}
		campaignID := ""
		if campaign != nil {
			campaignID, d["campaign_title"] = campaign.ID, campaign.Title
		}
		batch = append(batch, models.Notification{
			ID:         db.GenerateUUID(),
			UserID:     id,
			Type:       typ,
			CampaignID: campaignID,
			SessionID:  sessionID,
			ActorID:    actorID,
			Data:       d,
			CreatedAt:  now,
		})
	}
	if len(batch) == 0 {
		return 0
	}
	if err := db.CreateNotifications(batch); err != nil {
		log.Printf("notify %s: %v", typ, err)
		return -1
	}
	queueDiscordDMs(actorID, typ, batch)
	return len(batch)
}

// notifySessionChange notifies the campaign's followers about a session event.
// was is the session's previous status, when relevant.
func notifySessionChange(actorID string, campaign *models.Campaign, session models.CampaignSession, typ models.NotificationType, was string) {
	// Suggestions are availability requests: only players ever saw them.
	recipients := campaignFollowerIDs(campaign)
	if typ == models.NotificationSessionSuggested || (typ == models.NotificationSessionCancelled && was == string(models.SessionSuggested)) {
		recipients = campaign.Players
	}
	notify(actorID, recipients, typ, campaign, session.ID, map[string]string{
		"date":     session.Date,
		"time":     session.Time,
		"timezone": session.Timezone,
		"was":      was,
	})
	switch {
	case typ == models.NotificationSessionConfirmed:
		postSessionConfirmed(campaign, session)
	case typ == models.NotificationSessionCancelled && was == string(models.SessionConfirmed):
		postSessionCancelled(campaign, session)
		cancelDiscordEvent(session)
	}
}

// HandleNotifications handles /api/notifications (GET inbox list).
func HandleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	limit := 30
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			offset = n
		}
	}
	if offset > 10000 {
		offset = 10000
	}
	unreadOnly := r.URL.Query().Get("unread") == "1"

	// One extra row tells whether another page follows.
	notifications, err := db.GetNotifications(userID, unreadOnly, limit+1, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve notifications")
		return
	}
	hasMore := len(notifications) > limit
	if hasMore {
		notifications = notifications[:limit]
	}
	unread, err := db.CountUnreadNotifications(userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to count notifications")
		return
	}
	allUsers, err := db.GetAllUsers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve users")
		return
	}
	userMap := make(map[string]*models.User, len(allUsers))
	for i := range allUsers {
		userMap[allUsers[i].ID] = &allUsers[i]
	}
	allCampaigns, err := db.GetAllCampaigns()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve games")
		return
	}
	titles := make(map[string]string, len(allCampaigns))
	for _, c := range allCampaigns {
		titles[c.ID] = c.Title
	}

	items := make([]map[string]interface{}, 0, len(notifications))
	for _, n := range notifications {
		item := map[string]interface{}{
			"id":          n.ID,
			"type":        n.Type,
			"campaign_id": n.CampaignID,
			"session_id":  n.SessionID,
			"actor_id":    n.ActorID,
			"data":        n.Data,
			"created_at":  n.CreatedAt,
			"read_at":     n.ReadAt,
			"url":         notificationURL(n),
		}
		if actor, ok := userMap[n.ActorID]; ok && actor != nil {
			item["actor_user"] = userDisplayObject(actor)
		}
		if title, ok := titles[n.CampaignID]; ok {
			item["campaign_title"] = title
		} else {
			item["campaign_title"] = n.Data["campaign_title"]
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notifications": items,
		"unread_count":  unread,
		"has_more":      hasMore,
	})
}

// HandleNotificationRoutes handles /api/notifications/unread-count (GET),
// /api/notifications/read-all (POST) and /api/notifications/{id}/read (POST).
func HandleNotificationRoutes(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/notifications/"), "/")
	switch {
	case len(parts) == 1 && parts[0] == "unread-count":
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		unread, err := db.CountUnreadNotifications(userID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to count notifications")
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"unread_count": unread})
	case len(parts) == 1 && parts[0] == "read-all":
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := db.MarkAllNotificationsRead(userID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to mark notifications read")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "all notifications marked read"})
	case len(parts) == 2 && parts[0] != "" && parts[1] == "read":
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := db.MarkNotificationRead(userID, parts[0]); err != nil {
			if err == db.ErrNotificationNotFound {
				writeJSONError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "failed to mark notification read")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "notification marked read"})
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

// pollNotificationData is the payload of a poll notification: the poll's first upcoming date
// as the notification's own date, plus every date it concerns (YYYY-MM-DD, comma-joined) and
// their count. extra adds or overrides keys (added, was).
func pollNotificationData(first models.CampaignSession, dates []models.CampaignSession, extra map[string]string) map[string]string {
	days := make([]string, 0, len(dates))
	for _, d := range dates {
		days = append(days, d.Date)
	}
	data := map[string]string{
		"date":     first.Date,
		"time":     first.Time,
		"timezone": first.Timezone,
		"poll_id":  first.PollID,
		"dates":    strings.Join(days, ","),
		"count":    strconv.Itoa(len(dates)),
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}

// notifyPoll sends one notification about a poll. first (the poll's earliest upcoming date)
// becomes the notification's session, so its link lands on the calendar at that month.
func notifyPoll(actorID string, recipients []string, typ models.NotificationType, campaign *models.Campaign, first models.CampaignSession, dates []models.CampaignSession, extra map[string]string) {
	notify(actorID, recipients, typ, campaign, first.ID, pollNotificationData(first, dates, extra))
}
