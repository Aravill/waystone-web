package db

import "waystone-web/models"

// GetPollSchedule returns a game's repeating-poll schedule, or nil if it has none.
func GetPollSchedule(campaignID string) (*models.PollSchedule, error) {
	return GetStore().GetPollSchedule(campaignID)
}

func GetPollSchedules() ([]models.PollSchedule, error) {
	return GetStore().GetPollSchedules()
}

// SavePollSchedule inserts or replaces a game's schedule.
func SavePollSchedule(schedule models.PollSchedule) error {
	return GetStore().SavePollSchedule(schedule)
}

func DeletePollSchedule(campaignID string) error {
	return GetStore().DeletePollSchedule(campaignID)
}
