package models

// JournalRevision is one saved version of a journal entry. Numbers count up per entry and are never reused.
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

// JournalScribe is one person's credit on an entry. It outlives pruning of old revisions.
type JournalScribe struct {
	UserID        string
	Revisions     int
	FirstRevision int
}

// JournalEntry is an entry with its newest revision. Scribes are ordered by first revision.
type JournalEntry struct {
	ID            string
	CampaignID    string
	SessionID     string
	AuthorID      string
	CreatedAt     string
	Latest        JournalRevision
	RevisionCount int // revisions still kept
	Scribes       []JournalScribe
}

// JournalSummary is the per-session digest shown on the game page.
type JournalSummary struct {
	EntryID       string
	SessionID     string
	Name          string
	Revisions     int // revisions still kept
	UpdatedAt     string
	ExcerptSource string // the first 600 characters of the newest body
	AuthorID      string
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
