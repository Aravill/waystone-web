package db

import "waystone-web/models"

func GetUnremindedConfirmedSessions(fromDate, toDate string) ([]models.CampaignSession, error) {
	return GetStore().GetUnremindedConfirmedSessions(fromDate, toDate)
}

func GetDiscordSettings() (models.DiscordSettings, error) { return GetStore().GetDiscordSettings() }

// SaveDiscordSettings returns how many games lost their channel because the server changed.
func SaveDiscordSettings(s models.DiscordSettings) (int, error) {
	return GetStore().SaveDiscordSettings(s)
}

func SetCampaignDiscordChannel(campaignID, channelID, channelName string) error {
	return GetStore().SetCampaignDiscordChannel(campaignID, channelID, channelName)
}

func ClearLostDiscordChannel(campaignID, channelID string) (bool, error) {
	return GetStore().ClearLostDiscordChannel(campaignID, channelID)
}

func GetDiscordLink(userID string) (*models.DiscordLink, error) {
	return GetStore().GetDiscordLink(userID)
}

func SaveDiscordLink(userID, discordUserID, username string) error {
	return GetStore().SaveDiscordLink(userID, discordUserID, username)
}

func GetDiscordLinksForUsers(userIDs []string) (map[string]models.DiscordLink, error) {
	return GetStore().GetDiscordLinksForUsers(userIDs)
}

func SetDiscordToggles(userID string, sessions, signups, characters bool) error {
	return GetStore().SetDiscordToggles(userID, sessions, signups, characters)
}

func DeleteDiscordLink(userID string) error { return GetStore().DeleteDiscordLink(userID) }

func SetDiscordDMChannel(userID, channelID string) error {
	return GetStore().SetDiscordDMChannel(userID, channelID)
}

func SetDiscordDMError(userID, msg string) error { return GetStore().SetDiscordDMError(userID, msg) }

func EnqueueDiscord(rows []models.DiscordOutboxRow) error { return GetStore().EnqueueDiscord(rows) }

func NextDiscordOutbox(now string) (*models.DiscordOutboxRow, error) {
	return GetStore().NextDiscordOutbox(now)
}

func DeleteDiscordOutbox(id int64) error { return GetStore().DeleteDiscordOutbox(id) }

func UpdateDiscordOutbox(id int64, status string, attempts int, nextAttemptAt, lastError string) error {
	return GetStore().UpdateDiscordOutbox(id, status, attempts, nextAttemptAt, lastError)
}

func PruneDiscordOutbox(failedBefore string) error {
	return GetStore().PruneDiscordOutbox(failedBefore)
}

func GetDiscordOutboxSummary(limit int) (int, []models.DiscordOutboxRow, error) {
	return GetStore().GetDiscordOutboxSummary(limit)
}
