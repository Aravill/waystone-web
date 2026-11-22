package models

// JournalRevision is one saved version of a legacy journal entry (journal_revisions), kept only
// until the entry moves into a codex. Numbers count up per entry and are never reused.
type JournalRevision struct {
	Number       int    `json:"number"`
	AuthorID     string `json:"author_id"`
	Name         string `json:"name"`
	Body         string `json:"body"`
	Note         string `json:"note"`
	RestoredFrom int    `json:"restored_from"`
	MergedWith   int    `json:"merged_with"`
	CreatedAt    string `json:"created_at"`
}

// JournalPage is the digest of a session's journal page (a codex page with session_id set), shown
// on the game page.
type JournalPage struct {
	PageID        string
	Title         string
	CodexID       string
	SessionID     string
	AuthorID      string
	UpdatedAt     string // when the newest revision was saved
	ExcerptSource string // the first 600 characters of the newest body
	Revisions     int    // revisions still kept
	AmenderIDs    []string
}

type JournalSaveOutcome int

const (
	JournalCreated JournalSaveOutcome = iota
	JournalSaved
	JournalUnchanged
	JournalStale    // the base revision is no longer the latest
	JournalReplaced // the entry was deleted and written again since the client loaded it
	JournalDeleted  // the entry the client was editing no longer exists
)

// JournalSaveResult reports what SaveJournalRevision did. Revision is the new revision (or the
// current one when unchanged); Latest is the current revision when the save was refused as stale
// or replaced.
type JournalSaveResult struct {
	Outcome  JournalSaveOutcome
	EntryID  string
	Revision JournalRevision
	Latest   *JournalRevision
}
