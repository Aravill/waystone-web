package api

import (
	"net/http"
	"sort"
	"strings"
	"waystone-web/db"
)

// HandleAdminIntegrationsPage serves admin-integrations.html (server-side admin gate).
func HandleAdminIntegrationsPage(w http.ResponseWriter, r *http.Request) {
	caller, err := getAdminCaller(r)
	if err != nil || caller == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, "./static/admin-integrations.html")
}

// HandleAdminIntegrations serves GET /api/admin/integrations: each user's calendar link and
// Discord link (null when they have none). Blocked users are left out: neither reaches them.
func HandleAdminIntegrations(w http.ResponseWriter, r *http.Request) {
	if caller, err := getAdminCaller(r); err != nil || caller == nil {
		adminForbidden(w)
		return
	}
	if r.Method != http.MethodGet {
		adminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	users, err := db.GetAllUsers()
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to fetch users")
		return
	}
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	calendars, err := db.GetCalendarLinks()
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to fetch calendar links")
		return
	}
	discords, err := db.GetDiscordLinksForUsers(ids)
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to fetch Discord links")
		return
	}
	list := []map[string]interface{}{}
	for _, u := range users {
		if u.Deleted || u.Blocked {
			continue
		}
		row := map[string]interface{}{
			"id": u.ID, "username": u.Username, "display_name": u.DisplayName(),
			"calendar": nil, "discord": nil,
		}
		if c, ok := calendars[u.ID]; ok {
			row["calendar"] = map[string]string{"created_at": c.CreatedAt, "last_read_at": c.LastReadAt}
		}
		if d, ok := discords[u.ID]; ok {
			topics := []string{}
			if d.DMSessions {
				topics = append(topics, "Sessions & polls")
			}
			if d.DMSignups {
				topics = append(topics, "Sign-ups & players")
			}
			if d.DMCharacters {
				topics = append(topics, "Characters")
			}
			row["discord"] = map[string]interface{}{
				"username": d.DiscordUsername, "linked_at": d.LinkedAt, "topics": topics,
				"dm_error": d.DMError, "dm_error_at": d.DMErrorAt,
			}
		}
		list = append(list, row)
	}
	sort.SliceStable(list, func(i, j int) bool {
		return strings.ToLower(list[i]["display_name"].(string)) < strings.ToLower(list[j]["display_name"].(string))
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": list, "discord_enabled": discordEnabled()})
}
