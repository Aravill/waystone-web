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

// campaignFollowerIDs returns who follows a campaign. For now: its players.
// Real follows later only change this function.
func campaignFollowerIDs(c *models.Campaign) []string {
	return append([]string(nil), c.Players...)
}

// notify stores one notification per recipient, skipping "", duplicates and the actor.
// Errors are logged, never returned: notifications must not fail the main action.
func notify(actorID string, recipients []string, typ models.NotificationType, campaign *models.Campaign, sessionID string, data map[string]string) {
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
		d["campaign_title"] = campaign.Title
		batch = append(batch, models.Notification{
			ID:         db.GenerateUUID(),
			UserID:     id,
			Type:       typ,
			CampaignID: campaign.ID,
			SessionID:  sessionID,
			ActorID:    actorID,
			Data:       d,
			CreatedAt:  now,
		})
	}
	if len(batch) == 0 {
		return
	}
	if err := db.CreateNotifications(batch); err != nil {
		log.Printf("notify %s: %v", typ, err)
	}
}

// notifySessionChange notifies the campaign's followers about a session event.
// was is the session's previous status, when relevant.
func notifySessionChange(actorID string, campaign *models.Campaign, session models.CampaignSession, typ models.NotificationType, was string) {
	notify(actorID, campaignFollowerIDs(campaign), typ, campaign, session.ID, map[string]string{
		"date":     session.Date,
		"time":     session.Time,
		"timezone": session.Timezone,
		"was":      was,
	})
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

	notifications, err := db.GetNotifications(userID, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve notifications")
		return
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
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve campaigns")
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
