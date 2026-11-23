package api

import (
	"net/url"
	"waystone-web/models"
)

// notificationTarget is the one place that decides where a notification points: the path of the
// page that shows the event, and the label of the Discord button that opens it. The inbox, the
// Discord DMs and the channel posts all use it. It never depends on the viewer: the target page
// decides what that person can see. Everything is in the query string because a "#" part would be
// lost on the way through the login page. Design: docs/design/notification-links.md.
func notificationTarget(n models.Notification) (path, label string) {
	d := n.Data
	if d["character_id"] != "" && (n.Type == models.NotificationCharacterStatus || n.Type == models.NotificationCharacterAssigned) {
		return "/characters/" + url.PathEscape(d["character_id"]), "Open the character"
	}
	if n.Type == models.NotificationDiscordChannelLost {
		return withQuery("/admin/discord", "game", n.CampaignID), "Pick a channel"
	}
	if n.CampaignID == "" {
		return "/", "Open Waystone"
	}
	game := "/games/" + url.PathEscape(n.CampaignID)
	calendar := game + "/calendar"
	switch n.Type {
	case models.NotificationSessionSuggested, models.NotificationPollReminder, models.NotificationPollComplete, models.NotificationPollPosted:
		if d["poll_id"] == "" { // stored before polls existed
			return withQuery(calendar, "date", d["date"], "session", n.SessionID), "Open the poll"
		}
		return withQuery(calendar, "date", d["date"], "poll", d["poll_id"]), "Open the poll"
	case models.NotificationPollSkipped:
		return withQuery(calendar, "date", d["date"]), "Open the calendar"
	case models.NotificationSessionCancelled:
		if d["was"] == string(models.SessionSuggested) && d["poll_id"] != "" {
			return withQuery(calendar, "date", d["date"], "poll", d["poll_id"]), "Open the calendar"
		}
		return withQuery(calendar, "date", d["date"], "session", n.SessionID), "Open the session"
	case models.NotificationSessionConfirmed, models.NotificationSessionStarted, models.NotificationSessionReminder:
		return withQuery(calendar, "date", d["date"], "session", n.SessionID), "Open the session"
	case models.NotificationSignupCreated, models.NotificationSignupWithdrawn, models.NotificationPlayerLeft, models.NotificationCampaignFull:
		return withQuery("/campaigns", "signups", n.CampaignID), "Open sign-ups"
	case models.NotificationSignupAccepted:
		if d["characters"] != "" {
			return withQuery(game, "character", "1"), "Choose your character"
		}
	case models.NotificationCharacterAssigned, models.NotificationCharacterCleared:
		return withQuery(game, "company", "1"), "Open the game"
	}
	return game, "Open the game"
}

// notificationURL is the notification's path for its own recipient: opening it marks the
// notification read (static/notifications.js looks for notif).
func notificationURL(n models.Notification) string {
	path, _ := notificationTarget(n)
	return withQuery(path, "notif", n.ID)
}

// withQuery adds the non-empty key/value pairs, in order, to a path that may have a query already.
func withQuery(path string, pairs ...string) string {
	sep := "?"
	for i := range path {
		if path[i] == '?' {
			sep = "&"
			break
		}
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] == "" {
			continue
		}
		path += sep + pairs[i] + "=" + url.QueryEscape(pairs[i+1])
		sep = "&"
	}
	return path
}
