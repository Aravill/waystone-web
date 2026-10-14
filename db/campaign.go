package db

import (
	"waystone-web/models"
)

func GetAllCampaigns() ([]models.Campaign, error) {
	return GetStore().GetAllCampaigns()
}

func SaveCampaign(campaign models.Campaign) error {
	return GetStore().SaveCampaign(campaign)
}

func GetCampaignByID(id string) (*models.Campaign, error) {
	return GetStore().GetCampaignByID(id)
}

func GetNextCampaignID() string {
	return GenerateUUID()
}

func UpdateCampaignDetails(id, title, summary, description string, campaignType models.CampaignType, system, desiredPlayerCount string) error {
	return GetStore().UpdateCampaignDetails(id, title, summary, description, campaignType, system, desiredPlayerCount)
}

func SetCampaignStatus(id string, status models.CampaignStatus) error {
	return GetStore().SetCampaignStatus(id, status)
}

func DeleteCampaign(id string) error {
	return GetStore().DeleteCampaign(id)
}
