package db

import (
	"database/sql"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"waystone-web/models"
)

// ---- Tags: labels that belong to one codex. Names are unique per codex by NormName. ----

// codexTags lists a codex's tags with page counts, by name.
func codexTags(q querier, codexID string) ([]models.CodexTag, error) {
	rows, err := q.Query(`SELECT t.id, t.codex_id, t.name, t.created_by, t.created_at,
		(SELECT COUNT(*) FROM page_tags pt WHERE pt.tag_id = t.id) FROM codex_tags t WHERE t.codex_id = ?`, codexID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CodexTag{}
	for rows.Next() {
		var t models.CodexTag
		if err := rows.Scan(&t.ID, &t.CodexID, &t.Name, &t.CreatedBy, &t.CreatedAt, &t.PageCount); err != nil {
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

func sortTags(tags []models.CodexTag) {
	sort.SliceStable(tags, func(i, j int) bool { return NormName(tags[i].Name) < NormName(tags[j].Name) })
}

func (s *SQLiteStore) GetCodexTags(codexID string) ([]models.CodexTag, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return codexTags(s.db, codexID)
}

// GetCodexPageTags maps each page of the codex to the ids of its tags.
func (s *SQLiteStore) GetCodexPageTags(codexID string) (map[string][]string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT pt.page_id, pt.tag_id FROM page_tags pt JOIN codex_tags t ON t.id = pt.tag_id WHERE t.codex_id = ? ORDER BY pt.page_id, pt.tag_id`, codexID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var page, tag string
		if err := rows.Scan(&page, &tag); err != nil {
			return nil, err
		}
		out[page] = append(out[page], tag)
	}
	return out, rows.Err()
}

func pageTags(q querier, pageID string) ([]models.CodexTag, error) {
	rows, err := q.Query(`SELECT t.id, t.codex_id, t.name FROM page_tags pt JOIN codex_tags t ON t.id = pt.tag_id WHERE pt.page_id = ?`, pageID)
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

func (s *SQLiteStore) GetPageTags(pageID string) ([]models.CodexTag, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return pageTags(s.db, pageID)
}

// GetTagCodexID returns the codex a tag belongs to, or "" if there is no such tag.
func (s *SQLiteStore) GetTagCodexID(tagID string) (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	var id string
	err := s.db.QueryRow(`SELECT codex_id FROM codex_tags WHERE id = ?`, tagID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

// tagByName finds the tag with that name (compared with NormName), ignoring the tag with id skip.
func tagByName(tags []models.CodexTag, name, skip string) *models.CodexTag {
	for i := range tags {
		if tags[i].ID != skip && NormName(tags[i].Name) == NormName(name) {
			return &tags[i]
		}
	}
	return nil
}

func (s *SQLiteStore) CreateCodexTag(codexID, name, userID string) (models.CodexTag, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := codexExists(s.db, codexID); err != nil {
		return models.CodexTag{}, err
	}
	tags, err := codexTags(s.db, codexID)
	if err != nil {
		return models.CodexTag{}, err
	}
	if tagByName(tags, name, "") != nil {
		return models.CodexTag{}, ErrTagNameTaken
	}
	t := models.CodexTag{ID: GenerateUUID(), CodexID: codexID, Name: name, CreatedBy: userID, CreatedAt: nowUTC()}
	_, err = s.db.Exec(`INSERT INTO codex_tags (id, codex_id, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.CodexID, t.Name, t.CreatedBy, t.CreatedAt)
	return t, err
}

func (s *SQLiteStore) RenameCodexTag(id, name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	var codexID string
	if err := s.db.QueryRow(`SELECT codex_id FROM codex_tags WHERE id = ?`, id).Scan(&codexID); err == sql.ErrNoRows {
		return ErrTagNotFound
	} else if err != nil {
		return err
	}
	tags, err := codexTags(s.db, codexID)
	if err != nil {
		return err
	}
	if tagByName(tags, name, id) != nil {
		return ErrTagNameTaken
	}
	_, err = s.db.Exec(`UPDATE codex_tags SET name = ? WHERE id = ?`, name, id)
	return err
}

// DeleteCodexTag removes the tag from every page and returns how many pages had it.
func (s *SQLiteStore) DeleteCodexTag(id string) (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var one int
	if err := tx.QueryRow(`SELECT 1 FROM codex_tags WHERE id = ?`, id).Scan(&one); err == sql.ErrNoRows {
		return 0, ErrTagNotFound
	} else if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM codex_template_tags WHERE tag_id = ?`, id); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`DELETE FROM page_tags WHERE tag_id = ?`, id)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM codex_tags WHERE id = ?`, id); err != nil {
		return 0, err
	}
	return int(n), tx.Commit()
}

// ensureCodexTags returns the codex's tag for each name (names repeating by NormName give the same
// tag once), creating the ones it lacks.
func ensureCodexTags(tx *sql.Tx, codexID string, names []string, userID string) ([]models.CodexTag, error) {
	tags, err := codexTags(tx, codexID)
	if err != nil {
		return nil, err
	}
	out := []models.CodexTag{}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[NormName(name)] {
			continue
		}
		seen[NormName(name)] = true
		t := tagByName(tags, name, "")
		if t == nil {
			nt := models.CodexTag{ID: GenerateUUID(), CodexID: codexID, Name: name, CreatedBy: userID, CreatedAt: nowUTC()}
			if _, err := tx.Exec(`INSERT INTO codex_tags (id, codex_id, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)`,
				nt.ID, nt.CodexID, nt.Name, nt.CreatedBy, nt.CreatedAt); err != nil {
				return nil, err
			}
			tags = append(tags, nt)
			t = &tags[len(tags)-1]
		}
		out = append(out, *t)
	}
	return out, nil
}

// addPageTags gives the page the tags with these names, creating the codex's missing ones.
func addPageTags(tx *sql.Tx, pageID, codexID string, names []string, userID string) error {
	tags, err := ensureCodexTags(tx, codexID, names, userID)
	if err != nil {
		return err
	}
	for _, t := range tags {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO page_tags (page_id, tag_id) VALUES (?, ?)`, pageID, t.ID); err != nil {
			return err
		}
	}
	return nil
}

// SetPageTags makes names the page's tags, creating the ones the page's codex doesn't have yet.
// Callers pass cleaned names; repeats (by NormName) count once.
func (s *SQLiteStore) SetPageTags(pageID string, names []string, userID string) ([]models.CodexTag, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	codexID, _, err := pageCodex(tx, pageID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM page_tags WHERE page_id = ?`, pageID); err != nil {
		return nil, err
	}
	if err := addPageTags(tx, pageID, codexID, names, userID); err != nil {
		return nil, err
	}
	out, err := pageTags(tx, pageID)
	if err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

// ---- Page pictures ----

const pageImageColumns = `id, page_id, name, file, width, height, uploaded_by, created_at, removed_at`

func scanPageImage(row rowScanner) (models.PageImage, error) {
	var m models.PageImage
	err := row.Scan(&m.ID, &m.PageID, &m.Name, &m.File, &m.Width, &m.Height, &m.UploadedBy, &m.CreatedAt, &m.RemovedAt)
	return m, err
}

// GetPageImages lists every picture ever uploaded to the page, oldest first, and the cover's id.
func (s *SQLiteStore) GetPageImages(pageID string) ([]models.PageImage, string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT `+pageImageColumns+` FROM page_images WHERE page_id = ? ORDER BY created_at, rowid`, pageID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []models.PageImage{}
	for rows.Next() {
		m, err := scanPageImage(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	var cover string
	if err := s.db.QueryRow(`SELECT cover_image_id FROM pages WHERE id = ?`, pageID).Scan(&cover); err != nil && err != sql.ErrNoRows {
		return nil, "", err
	}
	return out, cover, nil
}

// uniqueImageName adds " (2)", " (3)"... before the extension until no picture of the page
// (removed ones included, since old revisions may still refer to them) has the name.
func uniqueImageName(taken map[string]bool, name string) string {
	if !taken[NormName(name)] {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for n := 2; ; n++ {
		cand := base + " (" + strconv.Itoa(n) + ")" + ext
		if !taken[NormName(cand)] {
			return cand
		}
	}
}

// AddPageImage stores the picture row; the returned image has its name made unique in the page.
func (s *SQLiteStore) AddPageImage(img models.PageImage) (models.PageImage, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, _, err := pageCodex(s.db, img.PageID); err != nil {
		return img, err
	}
	rows, err := s.db.Query(`SELECT name FROM page_images WHERE page_id = ?`, img.PageID)
	if err != nil {
		return img, err
	}
	taken := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return img, err
		}
		taken[NormName(n)] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return img, err
	}
	img.Name = uniqueImageName(taken, img.Name)
	_, err = s.db.Exec(`INSERT INTO page_images (`+pageImageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '')`,
		img.ID, img.PageID, img.Name, img.File, img.Width, img.Height, img.UploadedBy, img.CreatedAt)
	return img, err
}

// RemovePageImage takes a picture out of the page's list (soft: the row and file stay so old
// revisions still show it) and clears the cover if it was the cover.
func (s *SQLiteStore) RemovePageImage(pageID, imageID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE page_images SET removed_at = ? WHERE id = ? AND page_id = ? AND removed_at = ''`, nowUTC(), imageID, pageID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrImageNotFound
	}
	if _, err := tx.Exec(`UPDATE pages SET cover_image_id = '' WHERE id = ? AND cover_image_id = ?`, pageID, imageID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetPageCover makes a current picture of the page its cover; imageID "" clears it.
func (s *SQLiteStore) SetPageCover(pageID, imageID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, _, err := pageCodex(s.db, pageID); err != nil {
		return err
	}
	if imageID != "" {
		var one int
		if err := s.db.QueryRow(`SELECT 1 FROM page_images WHERE id = ? AND page_id = ? AND removed_at = ''`, imageID, pageID).Scan(&one); err == sql.ErrNoRows {
			return ErrImageNotFound
		} else if err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`UPDATE pages SET cover_image_id = ? WHERE id = ?`, imageID, pageID)
	return err
}
