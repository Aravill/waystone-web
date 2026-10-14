package db

import "waystone-web/models"

func GetSessionsByCampaignIDs(campaignIDs []string, fromDate string) ([]models.CampaignSession, error) {
	return GetStore().GetSessionsByCampaignIDs(campaignIDs, fromDate)
}

func GetCalendarToken(userID string) (string, error) {
	return GetStore().GetCalendarToken(userID)
}

func SetCalendarToken(userID, token string) error {
	return GetStore().SetCalendarToken(userID, token)
}

func GetUserIDByCalendarToken(token string) (string, error) {
	return GetStore().GetUserIDByCalendarToken(token)
}
