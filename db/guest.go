package db

import (
	"time"
	"waystone-web/models"
)

func GetGuestTokens(campaignID string) (map[string]models.GuestToken, error) {
	return GetStore().GetGuestTokens(campaignID)
}

func SetGuestToken(campaignID, userID, token string, expiresAt time.Time) error {
	return GetStore().SetGuestToken(campaignID, userID, token, expiresAt)
}

func DeleteGuestToken(campaignID, userID string) error {
	return GetStore().DeleteGuestToken(campaignID, userID)
}

func GetGuestToken(token string) (string, string, time.Time, error) {
	return GetStore().GetGuestToken(token)
}
