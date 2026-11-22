package db

import "waystone-web/models"

func GetSessionsByMonth(campaignID string, year int, month int) ([]models.CampaignSession, error) {
	return GetStore().GetSessionsByMonth(campaignID, year, month)
}

// GetAcceptedConfirmedSessions returns confirmed sessions the user accepted.
func GetAcceptedConfirmedSessions(userID string) ([]models.CampaignSession, error) {
	return GetStore().GetAcceptedConfirmedSessions(userID)
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

func GetSessionResponsesBySessionIDs(sessionIDs []string) (map[string][]models.SessionResponse, error) {
	return GetStore().GetSessionResponsesBySessionIDs(sessionIDs)
}

func DeleteSession(sessionID string) error {
	return GetStore().DeleteSession(sessionID)
}

func GetUnansweredSessions(userID, fromDate string) ([]models.CampaignSession, error) {
	return GetStore().GetUnansweredSessions(userID, fromDate)
}

func SaveManualSession(session models.CampaignSession, attendeeIDs []string) error {
	return GetStore().SaveManualSession(session, attendeeIDs)
}

func DeleteManualSession(sessionID string) error {
	return GetStore().DeleteManualSession(sessionID)
}

// GetUnannouncedConfirmedSessions returns confirmed, non-manual sessions dated between
// fromDate and toDate (inclusive, YYYY-MM-DD) that have no session_started notification yet.
func GetUnannouncedConfirmedSessions(fromDate, toDate string) ([]models.CampaignSession, error) {
	return GetStore().GetUnannouncedConfirmedSessions(fromDate, toDate)
}

// CreatePollSessions inserts a poll's dates in one transaction; a *DayTakenError refuses them all.
func CreatePollSessions(sessions []models.CampaignSession) error {
	return GetStore().CreatePollSessions(sessions)
}

// GetPollSessions returns the Suggested dates of a poll, past ones included.
func GetPollSessions(campaignID, pollID string) ([]models.CampaignSession, error) {
	return GetStore().GetPollSessions(campaignID, pollID)
}

// DeletePoll removes the Suggested dates of a poll with their replies and returns how many went.
func DeletePoll(campaignID, pollID string) (int, error) {
	return GetStore().DeletePoll(campaignID, pollID)
}

// ConfirmSession confirms a Suggested date and deletes the rest of its poll.
func ConfirmSession(sessionID string) error {
	return GetStore().ConfirmSession(sessionID)
}

// GetPollReminderCandidates returns every Suggested date of the polls with a date between
// fromDate and toDate (inclusive, YYYY-MM-DD).
func GetPollReminderCandidates(fromDate, toDate string) ([]models.CampaignSession, error) {
	return GetStore().GetPollReminderCandidates(fromDate, toDate)
}
