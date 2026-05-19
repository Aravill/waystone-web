package db

import "waystone-web/models"

func GetSessionsByMonth(campaignID string, year int, month int) ([]models.CampaignSession, error) {
	return GetStore().GetSessionsByMonth(campaignID, year, month)
}

func SaveSession(session models.CampaignSession) error {
	return GetStore().SaveSession(session)
}

func GetSessionByID(sessionID string) (*models.CampaignSession, error) {
	return GetStore().GetSessionByID(sessionID)
}

func UpsertSessionResponse(response models.SessionResponse) error {
	return GetStore().UpsertSessionResponse(response)
}

func GetSessionResponses(sessionID string) ([]models.SessionResponse, error) {
	return GetStore().GetSessionResponses(sessionID)
}

func DeleteSession(sessionID string) error {
	return GetStore().DeleteSession(sessionID)
}
