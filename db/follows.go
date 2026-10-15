package db

func FollowCampaign(campaignID, userID string) error {
	return GetStore().FollowCampaign(campaignID, userID)
}

func UnfollowCampaign(campaignID, userID string) error {
	return GetStore().UnfollowCampaign(campaignID, userID)
}

func GetCampaignFollowerIDs(campaignID string) ([]string, error) {
	return GetStore().GetCampaignFollowerIDs(campaignID)
}

func GetFollowedCampaignIDs(userID string) (map[string]bool, error) {
	return GetStore().GetFollowedCampaignIDs(userID)
}
