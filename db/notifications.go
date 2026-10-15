package db

import "waystone-web/models"

func CreateNotifications(notifications []models.Notification) error {
	return GetStore().CreateNotifications(notifications)
}

func GetNotifications(userID string, limit int) ([]models.Notification, error) {
	return GetStore().GetNotifications(userID, limit)
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
