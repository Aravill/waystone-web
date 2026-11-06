package db

import "waystone-web/models"

// JournalKeepRevisions is how many revisions of one entry are kept; older ones are pruned on save.
const JournalKeepRevisions = 200

func GetJournalSummaries(campaignID string) (map[string]models.JournalSummary, error) {
	return GetStore().GetJournalSummaries(campaignID)
}

func GetJournalEntry(sessionID string) (*models.JournalEntry, error) {
	return GetStore().GetJournalEntry(sessionID)
}

func GetJournalRevisions(sessionID string) (string, []models.JournalRevision, error) {
	return GetStore().GetJournalRevisions(sessionID)
}

func SaveJournalRevision(campaignID, sessionID, entryID string, baseRevision int, rev models.JournalRevision) (models.JournalSaveResult, error) {
	return GetStore().SaveJournalRevision(campaignID, sessionID, entryID, baseRevision, rev, JournalKeepRevisions)
}

func DeleteJournalEntry(sessionID string) error {
	return GetStore().DeleteJournalEntry(sessionID)
}
