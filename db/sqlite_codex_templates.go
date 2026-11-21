package db

import (
	"database/sql"
	"sort"
	"waystone-web/models"
)

// ---- Codex templates: page templates that belong to one codex. Names are unique per codex by
// NormName. The tags a template adds are rows of codex_tags, joined by codex_template_tags. ----

// CreatePageWith adds a page like CreatePage and, in the same transaction, gives it a note on
// revision 1, tags (the codex's missing ones are created), pictures and a cover.
func (s *SQLiteStore) CreatePageWith(p models.Page, body string, x models.NewPageExtras) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := createPageNoteTx(tx, p, body, x.Note); err != nil {
		return err
	}
	if err := addPageTags(tx, p.ID, p.CodexID, x.TagNames, p.AuthorID); err != nil {
		return err
	}
	for _, img := range x.Images {
		if _, err := tx.Exec(`INSERT INTO page_images (`+pageImageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			img.ID, p.ID, img.Name, img.File, img.Width, img.Height, img.UploadedBy, img.CreatedAt, img.RemovedAt); err != nil {
			return err
		}
	}
	if x.CoverImageID != "" {
		if _, err := tx.Exec(`UPDATE pages SET cover_image_id = ? WHERE id = ?`, x.CoverImageID, p.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CheckNewPage reports what creating a page with this title in the codex/folder would fail on
// (ErrCodexNotFound, *NameTakenError, ErrFolderNotFound), without writing anything.
func (s *SQLiteStore) CheckNewPage(codexID, folderID, title string) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	if err := codexExists(s.db, codexID); err != nil {
		return err
	}
	if conflicts, err := nameConflicts(s.db, codexID, "", []string{title}); err != nil {
		return err
	} else if len(conflicts) > 0 {
		return &NameTakenError{conflicts}
	}
	return folderInCodex(s.db, folderID, codexID)
}

const templateColumns = `id, codex_id, name, description, body, created_by, created_at, updated_at`

func scanTemplate(row rowScanner) (models.CodexTemplate, error) {
	var t models.CodexTemplate
	err := row.Scan(&t.ID, &t.CodexID, &t.Name, &t.Description, &t.Body, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	t.Tags = []models.CodexTag{}
	return t, err
}

func templateTags(q querier, templateID string) ([]models.CodexTag, error) {
	rows, err := q.Query(`SELECT t.id, t.codex_id, t.name FROM codex_template_tags tt JOIN codex_tags t ON t.id = tt.tag_id WHERE tt.template_id = ?`, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CodexTag{}
	for rows.Next() {
		var t models.CodexTag
		if err := rows.Scan(&t.ID, &t.CodexID, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortTags(out)
	return out, nil
}

func codexTemplates(q querier, codexID string) ([]models.CodexTemplate, error) {
	rows, err := q.Query(`SELECT `+templateColumns+` FROM codex_templates WHERE codex_id = ?`, codexID)
	if err != nil {
		return nil, err
	}
	out := []models.CodexTemplate{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Tags, err = templateTags(q, out[i].ID); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return NormName(out[i].Name) < NormName(out[j].Name) })
	return out, nil
}

func (s *SQLiteStore) GetCodexTemplates(codexID string) ([]models.CodexTemplate, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return codexTemplates(s.db, codexID)
}

// GetCodexTemplate returns nil, nil when there is no such template.
func (s *SQLiteStore) GetCodexTemplate(id string) (*models.CodexTemplate, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	t, err := scanTemplate(s.db.QueryRow(`SELECT `+templateColumns+` FROM codex_templates WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if t.Tags, err = templateTags(s.db, id); err != nil {
		return nil, err
	}
	return &t, nil
}

func templateNameTaken(tx *sql.Tx, codexID, name, skip string) (bool, error) {
	list, err := codexTemplates(tx, codexID)
	if err != nil {
		return false, err
	}
	for _, t := range list {
		if t.ID != skip && NormName(t.Name) == NormName(name) {
			return true, nil
		}
	}
	return false, nil
}

// setTemplateTags makes names the template's tags, creating the codex's missing ones.
func setTemplateTags(tx *sql.Tx, templateID, codexID string, names []string, userID string) error {
	tags, err := ensureCodexTags(tx, codexID, names, userID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM codex_template_tags WHERE template_id = ?`, templateID); err != nil {
		return err
	}
	for _, t := range tags {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO codex_template_tags (template_id, tag_id) VALUES (?, ?)`, templateID, t.ID); err != nil {
			return err
		}
	}
	return nil
}

// CreateCodexTemplate adds a template (callers pass cleaned fields) with the tags it adds.
func (s *SQLiteStore) CreateCodexTemplate(t models.CodexTemplate, tagNames []string) (models.CodexTemplate, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return t, err
	}
	defer tx.Rollback()
	if err := codexExists(tx, t.CodexID); err != nil {
		return t, err
	}
	if taken, err := templateNameTaken(tx, t.CodexID, t.Name, ""); err != nil {
		return t, err
	} else if taken {
		return t, ErrTemplateNameTaken
	}
	t.ID, t.CreatedAt = GenerateUUID(), nowUTC()
	t.UpdatedAt = t.CreatedAt
	if _, err := tx.Exec(`INSERT INTO codex_templates (`+templateColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.CodexID, t.Name, t.Description, t.Body, t.CreatedBy, t.CreatedAt, t.UpdatedAt); err != nil {
		return t, err
	}
	if err := setTemplateTags(tx, t.ID, t.CodexID, tagNames, t.CreatedBy); err != nil {
		return t, err
	}
	if t.Tags, err = templateTags(tx, t.ID); err != nil {
		return t, err
	}
	return t, tx.Commit()
}

func (s *SQLiteStore) UpdateCodexTemplate(id, name, description, body string, tagNames []string, userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var codexID string
	if err := tx.QueryRow(`SELECT codex_id FROM codex_templates WHERE id = ?`, id).Scan(&codexID); err == sql.ErrNoRows {
		return ErrTemplateNotFound
	} else if err != nil {
		return err
	}
	if taken, err := templateNameTaken(tx, codexID, name, id); err != nil {
		return err
	} else if taken {
		return ErrTemplateNameTaken
	}
	if _, err := tx.Exec(`UPDATE codex_templates SET name = ?, description = ?, body = ?, updated_at = ? WHERE id = ?`,
		name, description, body, nowUTC(), id); err != nil {
		return err
	}
	if err := setTemplateTags(tx, id, codexID, tagNames, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) DeleteCodexTemplate(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM codex_template_tags WHERE template_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM codex_templates WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrTemplateNotFound
	}
	return tx.Commit()
}
