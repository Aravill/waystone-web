package db

import (
	"waystone-web/models"

	"github.com/google/uuid"
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
	return uuid.New().String()
}
