package db

import (
	"database/sql"
	"waystone-web/models"
)

// ---- Attachments: files on a page, kept in the attachments directory (see AttachmentDir). The row id is
// stable across Replace, so links to /api/codices/attachments/{id}/... keep working. ----

const pageAttachmentColumns = `id, page_id, name, file, size, uploaded_by, created_at`

// pageFileNames is every name a page's text may refer to: pictures (removed ones included) and attachments.
// The keys are NormName of the names.
func pageFileNames(q querier, pageID string) (map[string]bool, error) {
	rows, err := q.Query(`SELECT name FROM page_images WHERE page_id = ? UNION ALL SELECT name FROM page_attachments WHERE page_id = ?`, pageID, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	taken := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		taken[NormName(n)] = true
	}
	return taken, rows.Err()
}

func scanAttachment(sc interface{ Scan(...interface{}) error }, a *models.PageAttachment, extra ...interface{}) error {
	return sc.Scan(append([]interface{}{&a.ID, &a.PageID, &a.Name, &a.File, &a.Size, &a.UploadedBy, &a.CreatedAt}, extra...)...)
}

// GetPageAttachments lists a page's attachments, oldest first.
func (s *SQLiteStore) GetPageAttachments(pageID string) ([]models.PageAttachment, error) {
	rows, err := s.db.Query(`SELECT `+pageAttachmentColumns+` FROM page_attachments WHERE page_id = ? ORDER BY created_at, rowid`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PageAttachment{}
	for rows.Next() {
		var a models.PageAttachment
		if err := scanAttachment(rows, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetCodexAttachments lists every attachment on the codex's pages, newest first, with the page title.
func (s *SQLiteStore) GetCodexAttachments(codexID string) ([]models.CodexAttachment, error) {
	rows, err := s.db.Query(`SELECT a.id, a.page_id, a.name, a.file, a.size, a.uploaded_by, a.created_at, p.title
		FROM page_attachments a JOIN pages p ON p.id = a.page_id WHERE p.codex_id = ?
		ORDER BY a.created_at DESC, a.rowid DESC`, codexID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CodexAttachment{}
	for rows.Next() {
		var a models.CodexAttachment
		if err := scanAttachment(rows, &a.PageAttachment, &a.PageTitle); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAttachment returns the row and the id of its page's codex; an unknown id gives an empty codex id and no error.
func (s *SQLiteStore) GetAttachment(id string) (models.PageAttachment, string, error) {
	var a models.PageAttachment
	var codexID string
	err := scanAttachment(s.db.QueryRow(`SELECT a.id, a.page_id, a.name, a.file, a.size, a.uploaded_by, a.created_at, p.codex_id
		FROM page_attachments a JOIN pages p ON p.id = a.page_id WHERE a.id = ?`, id), &a, &codexID)
	if err == sql.ErrNoRows {
		return models.PageAttachment{}, "", nil
	}
	return a, codexID, err
}

// AddPageAttachment stores the row; the returned attachment has its name made unique among the page's
// pictures (removed ones included) and attachments.
func (s *SQLiteStore) AddPageAttachment(a models.PageAttachment) (models.PageAttachment, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return a, err
	}
	defer tx.Rollback()
	if _, _, err := pageCodex(tx, a.PageID); err != nil {
		return a, err
	}
	taken, err := pageFileNames(tx, a.PageID)
	if err != nil {
		return a, err
	}
	a.Name = uniqueImageName(taken, a.Name)
	if _, err := tx.Exec(`INSERT INTO page_attachments (`+pageAttachmentColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.PageID, a.Name, a.File, a.Size, a.UploadedBy, a.CreatedAt); err != nil {
		return a, err
	}
	return a, tx.Commit()
}

// ReplacePageAttachment points the attachment at a new file (same id and name) and returns the updated
// row and the old file, which the caller deletes once this has committed.
func (s *SQLiteStore) ReplacePageAttachment(pageID, id, file string, size int64, userID string) (models.PageAttachment, string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return models.PageAttachment{}, "", err
	}
	defer tx.Rollback()
	var a models.PageAttachment
	if err := scanAttachment(tx.QueryRow(`SELECT `+pageAttachmentColumns+` FROM page_attachments WHERE id = ? AND page_id = ?`, id, pageID), &a); err == sql.ErrNoRows {
		return a, "", ErrAttachmentNotFound
	} else if err != nil {
		return a, "", err
	}
	old := a.File
	a.File, a.Size, a.UploadedBy, a.CreatedAt = file, size, userID, nowUTC()
	if _, err := tx.Exec(`UPDATE page_attachments SET file = ?, size = ?, uploaded_by = ?, created_at = ? WHERE id = ?`,
		a.File, a.Size, a.UploadedBy, a.CreatedAt, id); err != nil {
		return a, "", err
	}
	return a, old, tx.Commit()
}

// RemovePageAttachment deletes the row and returns its file for the caller to delete.
func (s *SQLiteStore) RemovePageAttachment(pageID, id string) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var file string
	if err := tx.QueryRow(`SELECT file FROM page_attachments WHERE id = ? AND page_id = ?`, id, pageID).Scan(&file); err == sql.ErrNoRows {
		return "", ErrAttachmentNotFound
	} else if err != nil {
		return "", err
	}
	if _, err := tx.Exec(`DELETE FROM page_attachments WHERE id = ? AND page_id = ?`, id, pageID); err != nil {
		return "", err
	}
	return file, tx.Commit()
}

// queryFiles runs a query that selects one file-name column.
func queryFiles(q querier, query string, args ...interface{}) ([]string, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
