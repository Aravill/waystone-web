package db

import "waystone-web/models"

func GetAllCampaigns() ([]models.Campaign, error) {
	return GetStore().GetAllCampaigns()
}

func SaveCampaign(campaign models.Campaign) error {
	return GetStore().SaveCampaign(campaign)
}

func GetCampaignByID(id int) (*models.Campaign, error) {
	return GetStore().GetCampaignByID(id)
}

func GetNextCampaignID() int {
	campaigns, err := GetAllCampaigns()
	if err != nil {
		return 1
	}

	maxID := 0
	for _, campaign := range campaigns {
		if campaign.ID > maxID {
			maxID = campaign.ID
		}
	}

	return maxID + 1
}
