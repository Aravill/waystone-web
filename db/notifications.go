package db

import "waystone-web/models"

func CreateNotifications(notifications []models.Notification) error {
	return GetStore().CreateNotifications(notifications)
}

func GetNotifications(userID string, unreadOnly bool, limit, offset int) ([]models.Notification, error) {
	return GetStore().GetNotifications(userID, unreadOnly, limit, offset)
}

func CountUnreadNotifications(userID string) (int, error) {
	return GetStore().CountUnreadNotifications(userID)
}

func MarkNotificationRead(userID, id string) error {
	return GetStore().MarkNotificationRead(userID, id)
}

func MarkAllNotificationsRead(userID string) error {
	return GetStore().MarkAllNotificationsRead(userID)
}

// GetPollReminderUserIDs returns, per poll id, the users who already got a poll_reminder for it.
func GetPollReminderUserIDs(pollIDs []string) (map[string]map[string]bool, error) {
	return GetStore().GetPollReminderUserIDs(pollIDs)
}
