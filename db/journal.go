package db

import "waystone-web/models"

// JournalKeepRevisions is how many revisions of one legacy entry are kept (the seed writes through it).
const JournalKeepRevisions = 200

func SaveJournalRevision(campaignID, sessionID, entryID string, baseRevision int, rev models.JournalRevision) (models.JournalSaveResult, error) {
	return GetStore().SaveJournalRevision(campaignID, sessionID, entryID, baseRevision, rev, JournalKeepRevisions)
}

// GetJournalPages returns the campaign's journal pages keyed by session id.
func GetJournalPages(campaignID string) (map[string]models.JournalPage, error) {
	return GetStore().GetJournalPages(campaignID)
}

// GetJournalPageID returns the id of a session's journal page, or "".
func GetJournalPageID(sessionID string) (string, error) {
	return GetStore().GetJournalPageID(sessionID)
}

// CountLegacyJournal counts entries still waiting in the legacy tables for the game to get a codex.
func CountLegacyJournal(campaignID string) (int, error) {
	return GetStore().CountLegacyJournal(campaignID)
}

// EnsureJournalFolder finds or creates the game's journal folder in the codex and returns its id
// and the template new pages there start from.
func EnsureJournalFolder(campaignID, codexID, userID string) (string, string, error) {
	return GetStore().EnsureJournalFolder(campaignID, codexID, userID)
}

// CreateJournalPage writes a session's journal page (created false: the session already had one).
func CreateJournalPage(campaignID string, p models.Page, base, body string, x models.NewPageExtras) (string, bool, error) {
	return GetStore().CreateJournalPage(campaignID, p, base, body, x)
}

// RenameJournalFolder follows a game rename while the folder still has its generated name.
func RenameJournalFolder(campaignID, oldTitle, newTitle string) error {
	return GetStore().RenameJournalFolder(campaignID, oldTitle, newTitle)
}
