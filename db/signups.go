package db

import "waystone-web/models"

func CreateCampaignSignup(signup models.CampaignSignup) error {
	return GetStore().CreateCampaignSignup(signup)
}

func GetCampaignSignup(campaignID, userID string) (*models.CampaignSignup, error) {
	return GetStore().GetCampaignSignup(campaignID, userID)
}

func GetCampaignSignups(campaignID string) ([]models.CampaignSignup, error) {
	return GetStore().GetCampaignSignups(campaignID)
}

func GetCampaignSignupsByUser(userID string) (map[string]models.CampaignSignup, error) {
	return GetStore().GetCampaignSignupsByUser(userID)
}

func SetCampaignSignupStatus(campaignID, userID string, status models.SignupStatus) error {
	return GetStore().SetCampaignSignupStatus(campaignID, userID, status)
}

func SetCampaignSignUpsOpen(campaignID string, open bool) error {
	return GetStore().SetCampaignSignUpsOpen(campaignID, open)
}
