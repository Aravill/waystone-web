package db

import (
	"database/sql"
	"unicode/utf8"
	"waystone-web/models"
)

// ---- Quick notes: private to their author; one inbox per user per codex. ----

// GetCodexNotes lists one user's notes in a codex, newest first.
func (s *SQLiteStore) GetCodexNotes(codexID, userID string) ([]models.CodexNote, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT id, codex_id, user_id, body, created_at FROM codex_notes WHERE codex_id = ? AND user_id = ? ORDER BY created_at DESC, rowid DESC`, codexID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CodexNote{}
	for rows.Next() {
		var n models.CodexNote
		if err := rows.Scan(&n.ID, &n.CodexID, &n.UserID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) CountCodexNotes(codexID, userID string) (int, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM codex_notes WHERE codex_id = ? AND user_id = ?`, codexID, userID).Scan(&n)
	return n, err
}

// CreateCodexNote stores the note, or ErrInboxFull when the user already has max in that codex.
func (s *SQLiteStore) CreateCodexNote(n models.CodexNote, max int) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM codex_notes WHERE codex_id = ? AND user_id = ?`, n.CodexID, n.UserID).Scan(&count); err != nil {
		return err
	}
	if count >= max {
		return ErrInboxFull
	}
	if _, err := tx.Exec(`INSERT INTO codex_notes (id, codex_id, user_id, body, created_at) VALUES (?, ?, ?, ?, ?)`,
		n.ID, n.CodexID, n.UserID, n.Body, n.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// GetCodexNote returns nil, nil when the note does not exist.
func (s *SQLiteStore) GetCodexNote(id string) (*models.CodexNote, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return getCodexNote(s.db, id)
}

func getCodexNote(q querier, id string) (*models.CodexNote, error) {
	var n models.CodexNote
	err := q.QueryRow(`SELECT id, codex_id, user_id, body, created_at FROM codex_notes WHERE id = ?`, id).
		Scan(&n.ID, &n.CodexID, &n.UserID, &n.Body, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *SQLiteStore) UpdateCodexNote(id, body string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	res, err := s.db.Exec(`UPDATE codex_notes SET body = ? WHERE id = ?`, body, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNoteNotFound
	}
	return nil
}

func (s *SQLiteStore) DeleteCodexNote(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return deleteCodexNoteTx(s.db, id)
}

func deleteCodexNoteTx(q querier, id string) error {
	res, err := q.Exec(`DELETE FROM codex_notes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNoteNotFound
	}
	return nil
}

// AppendNoteToPage appends addition to the page's latest body as a new revision (noted "Added a
// quick note", by userID) and deletes the note, in one transaction. The note must belong to userID
// and to the page's codex (else ErrNoteNotFound, ErrPageNotFound). ErrNoteTooLong when the page
// would pass maxRunes.
func (s *SQLiteStore) AppendNoteToPage(noteID, userID, pageID, addition string, maxRunes, keep int) (models.PageRevision, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return models.PageRevision{}, err
	}
	defer tx.Rollback()
	note, err := getCodexNote(tx, noteID)
	if err != nil {
		return models.PageRevision{}, err
	}
	if note == nil || note.UserID != userID {
		return models.PageRevision{}, ErrNoteNotFound
	}
	codex, title, err := pageCodex(tx, pageID)
	if err != nil {
		return models.PageRevision{}, err
	}
	if codex != note.CodexID {
		return models.PageRevision{}, ErrPageNotFound
	}
	latest, err := scanPageRevision(tx.QueryRow(`SELECT `+pageRevisionColumns+` FROM page_revisions WHERE page_id = ? ORDER BY number DESC LIMIT 1`, pageID))
	if err != nil {
		return models.PageRevision{}, err
	}
	body := latest.Body
	if body != "" {
		body += "\n\n"
	}
	body += addition
	if utf8.RuneCountInString(body) > maxRunes {
		return models.PageRevision{}, ErrNoteTooLong
	}
	rev, err := insertPageRevision(tx, pageID, models.PageRevision{AuthorID: userID, Title: title, Body: body, Note: "Added a quick note"}, keep)
	if err != nil {
		return models.PageRevision{}, err
	}
	if _, err := tx.Exec(`UPDATE pages SET updated_at = ? WHERE id = ?`, rev.CreatedAt, pageID); err != nil {
		return models.PageRevision{}, err
	}
	idx, err := loadCodexIndex(tx, codex)
	if err != nil {
		return models.PageRevision{}, err
	}
	if err := rebuildPageLinks(tx, idx, pageID, codex, body); err != nil {
		return models.PageRevision{}, err
	}
	if err := deleteCodexNoteTx(tx, noteID); err != nil {
		return models.PageRevision{}, err
	}
	return rev, tx.Commit()
}
