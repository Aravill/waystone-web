package db

import (
	"database/sql"
	"sort"
	"strings"
	"time"
	"waystone-web/models"
)

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

const pageRevisionColumns = `number, author_id, title, body, note, restored_from, created_at`

func scanPageRevision(row rowScanner) (models.PageRevision, error) {
	var r models.PageRevision
	err := row.Scan(&r.Number, &r.AuthorID, &r.Title, &r.Body, &r.Note, &r.RestoredFrom, &r.CreatedAt)
	return r, err
}

// insertPageRevision appends the next revision, credits its author and prunes revisions older
// than the newest keep. Credit lives in page_contributors, so it survives pruning.
func insertPageRevision(tx *sql.Tx, pageID string, rev models.PageRevision, keep int) (models.PageRevision, error) {
	var last int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(number), 0) FROM page_revisions WHERE page_id = ?`, pageID).Scan(&last); err != nil {
		return rev, err
	}
	rev.Number = last + 1
	if rev.CreatedAt == "" {
		rev.CreatedAt = nowUTC()
	}
	if _, err := tx.Exec(`INSERT INTO page_revisions (page_id, `+pageRevisionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		pageID, rev.Number, rev.AuthorID, rev.Title, rev.Body, rev.Note, rev.RestoredFrom, rev.CreatedAt); err != nil {
		return rev, err
	}
	if _, err := tx.Exec(`INSERT INTO page_contributors (page_id, user_id, revisions, first_revision) VALUES (?, ?, 1, ?)
		ON CONFLICT(page_id, user_id) DO UPDATE SET revisions = revisions + 1`, pageID, rev.AuthorID, rev.Number); err != nil {
		return rev, err
	}
	_, err := tx.Exec(`DELETE FROM page_revisions WHERE page_id = ? AND number <= ?`, pageID, rev.Number-keep)
	return rev, err
}

func pageCodex(q querier, id string) (codex, title string, err error) {
	err = q.QueryRow(`SELECT codex_id, title FROM pages WHERE id = ?`, id).Scan(&codex, &title)
	if err == sql.ErrNoRows {
		err = ErrPageNotFound
	}
	return
}

// checkPageText refuses to write text into a map page (ErrMapNoText).
func checkPageText(q querier, id string) error {
	var kind string
	if err := q.QueryRow(`SELECT kind FROM pages WHERE id = ?`, id).Scan(&kind); err != nil {
		return err
	}
	if kind == models.PageMap {
		return ErrMapNoText
	}
	return nil
}

func pageAliases(q querier, id string) ([]string, error) {
	rows, err := q.Query(`SELECT alias FROM page_aliases WHERE page_id = ? ORDER BY alias`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func codexExists(q querier, id string) error {
	var one int
	if err := q.QueryRow(`SELECT 1 FROM codices WHERE id = ?`, id).Scan(&one); err == sql.ErrNoRows {
		return ErrCodexNotFound
	} else {
		return err
	}
}

// checkNewPage is what a new page needs before it is written: the codex exists, none of the names
// (title and aliases) is taken by another page in it, and the folder belongs to it.
func checkNewPage(q querier, codexID, folderID, pageID string, names []string) error {
	if err := codexExists(q, codexID); err != nil {
		return err
	}
	if conflicts, err := nameConflicts(q, codexID, pageID, names); err != nil {
		return err
	} else if len(conflicts) > 0 {
		return &NameTakenError{conflicts}
	}
	return folderInCodex(q, folderID, codexID)
}

// CreatePage adds a page (revision 1, body may be empty) and any aliases in p.Aliases.
// A title or alias already used in the codex gives *NameTakenError.
func (s *SQLiteStore) CreatePage(p models.Page, body string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := createPageTx(tx, p, body); err != nil {
		return err
	}
	return tx.Commit()
}

// createPageTx does CreatePage's work inside a caller's transaction.
func createPageTx(tx *sql.Tx, p models.Page, body string) error {
	return createPageNoteTx(tx, p, body, "")
}

// createPageNoteTx is createPageTx with a note on revision 1.
func createPageNoteTx(tx *sql.Tx, p models.Page, body, note string) error {
	// Aliases are unique by normalized name and never repeat the title.
	aliases, seen := []string{}, map[string]bool{NormName(p.Title): true}
	for _, a := range p.Aliases {
		if k := NormName(a); !seen[k] {
			seen[k] = true
			aliases = append(aliases, a)
		}
	}
	if err := checkNewPage(tx, p.CodexID, p.FolderID, p.ID, append([]string{p.Title}, aliases...)); err != nil {
		return err
	}
	now := nowUTC()
	if _, err := tx.Exec(`INSERT INTO pages (id, kind, codex_id, folder_id, session_id, title, author_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Kind, p.CodexID, p.FolderID, p.SessionID, p.Title, p.AuthorID, now, now); err != nil {
		return err
	}
	for _, a := range aliases {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO page_aliases (page_id, alias) VALUES (?, ?)`, p.ID, a); err != nil {
			return err
		}
	}
	if _, err := insertPageRevision(tx, p.ID, models.PageRevision{AuthorID: p.AuthorID, Title: p.Title, Body: body, Note: note, CreatedAt: now}, PageKeepRevisions); err != nil {
		return err
	}
	idx, err := loadCodexIndex(tx, p.CodexID)
	if err != nil {
		return err
	}
	if err := resolveLinksWith(tx, idx, p.CodexID); err != nil { // other pages' missing links may now resolve
		return err
	}
	return rebuildPageLinks(tx, idx, p.ID, p.CodexID, body)
}

// GetPage returns nil, nil when the page does not exist.
func (s *SQLiteStore) GetPage(id string) (*models.Page, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	var p models.Page
	err := s.db.QueryRow(`SELECT id, kind, codex_id, folder_id, session_id, title, author_id, created_at, updated_at FROM pages WHERE id = ?`, id).
		Scan(&p.ID, &p.Kind, &p.CodexID, &p.FolderID, &p.SessionID, &p.Title, &p.AuthorID, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT number, body, author_id FROM page_revisions WHERE page_id = ? ORDER BY number DESC LIMIT 1`, id).
		Scan(&p.CurrentRevision, &p.Body, &p.UpdatedByID); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM page_revisions WHERE page_id = ?`, id).Scan(&p.RevisionCount); err != nil {
		return nil, err
	}
	if p.Aliases, err = pageAliases(s.db, id); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT user_id, revisions, first_revision FROM page_contributors WHERE page_id = ? ORDER BY first_revision`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c models.PageContributor
		if err := rows.Scan(&c.UserID, &c.Revisions, &c.FirstRevision); err != nil {
			return nil, err
		}
		p.Contributors = append(p.Contributors, c)
	}
	return &p, rows.Err()
}

// GetCodexPages lists every page with its aliases, ordered by title.
func (s *SQLiteStore) GetCodexPages() ([]models.PageListItem, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT id, kind, codex_id, folder_id, title, created_at, updated_at FROM pages ORDER BY title COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	out := []models.PageListItem{}
	at := map[string]int{}
	for rows.Next() {
		var p models.PageListItem
		if err := rows.Scan(&p.ID, &p.Kind, &p.CodexID, &p.FolderID, &p.Title, &p.CreatedAt, &p.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		p.Aliases = []string{}
		at[p.ID] = len(out)
		out = append(out, p)
	}
	rows.Close()
	arows, err := s.db.Query(`SELECT page_id, alias FROM page_aliases ORDER BY alias`)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var id, a string
		if err := arows.Scan(&id, &a); err != nil {
			return nil, err
		}
		if i, ok := at[id]; ok {
			out[i].Aliases = append(out[i].Aliases, a)
		}
	}
	return out, arows.Err()
}

func (s *SQLiteStore) GetPageLinks(fromID string) ([]models.PageLink, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT from_page_id, target_text, to_page_id FROM page_links WHERE from_page_id = ? ORDER BY target_text`, fromID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PageLink{}
	for rows.Next() {
		var l models.PageLink
		if err := rows.Scan(&l.FromPageID, &l.TargetText, &l.ToPageID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetBacklinks lists the pages that link to id (self-links excluded).
func (s *SQLiteStore) GetBacklinks(id string) ([]models.PageBacklink, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	out := []models.PageBacklink{}
	rows, err := s.db.Query(`SELECT p.id, p.title, p.kind, l.target_text FROM page_links l JOIN pages p ON p.id = l.from_page_id
		WHERE l.to_page_id = ? AND p.id <> ? ORDER BY p.title COLLATE NOCASE`, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b models.PageBacklink
		if err := rows.Scan(&b.PageID, &b.Title, &b.Kind, &b.TargetText); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// CountPagesByAuthor returns how many pages the user created (pages.author_id), per codex id.
func (s *SQLiteStore) CountPagesByAuthor(userID string) (map[string]int, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT codex_id, COUNT(*) FROM pages WHERE author_id = ? GROUP BY codex_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// GetCodexBodies lists the pages of a codex with the body of their newest revision, ordered by title.
func (s *SQLiteStore) GetCodexBodies(codexID string) ([]models.PageBody, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT p.id, p.title, r.number, r.body FROM pages p
		JOIN page_revisions r ON r.page_id = p.id AND r.number = (SELECT MAX(number) FROM page_revisions WHERE page_id = p.id)
		WHERE p.codex_id = ? ORDER BY p.title COLLATE NOCASE, p.id`, codexID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PageBody{}
	for rows.Next() {
		var b models.PageBody
		if err := rows.Scan(&b.ID, &b.Title, &b.CurrentRevision, &b.Body); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetWantedPages lists the titles that links in a codex point at without a page, most wanted first.
func (s *SQLiteStore) GetWantedPages(codexID string) ([]models.WantedPage, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	counts := map[string]*models.WantedPage{}
	rows, err := s.db.Query(`SELECT l.target_text FROM page_links l JOIN pages p ON p.id = l.from_page_id WHERE l.to_page_id = '' AND p.codex_id = ?`, codexID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return nil, err
		}
		k := NormName(t)
		if counts[k] == nil {
			counts[k] = &models.WantedPage{Text: t}
		}
		counts[k].Count++
	}
	rows.Close()
	out := []models.WantedPage{}
	for _, w := range counts {
		out = append(out, *w)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return NormName(out[i].Text) < NormName(out[j].Text)
	})
	return out, nil
}

// GetPageRevisions returns the kept revisions, newest first, without their bodies.
func (s *SQLiteStore) GetPageRevisions(id string) ([]models.PageRevision, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT number, author_id, title, '', note, restored_from, created_at FROM page_revisions WHERE page_id = ? ORDER BY number DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PageRevision{}
	for rows.Next() {
		r, err := scanPageRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetPageRevision returns one kept revision with its body: ErrPageNotFound when the page is gone,
// ErrRevisionNotFound when that number was never made or has been pruned.
func (s *SQLiteStore) GetPageRevision(id string, number int) (models.PageRevision, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	r, err := scanPageRevision(s.db.QueryRow(`SELECT `+pageRevisionColumns+` FROM page_revisions WHERE page_id = ? AND number = ?`, id, number))
	if err == sql.ErrNoRows {
		if _, _, perr := pageCodex(s.db, id); perr != nil {
			return r, perr
		}
		return r, ErrRevisionNotFound
	}
	return r, err
}

// GetRecentRevisions lists the newest revisions, only for pages in the given codices (nil: all).
func (s *SQLiteStore) GetRecentRevisions(codexIDs []string, limit int) ([]models.RecentRevision, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	q := `SELECT p.id, p.title, p.kind, p.codex_id, r.number, r.note, r.author_id, r.created_at
		FROM page_revisions r JOIN pages p ON p.id = r.page_id`
	args := []interface{}{}
	if codexIDs != nil {
		if len(codexIDs) == 0 {
			return []models.RecentRevision{}, nil
		}
		q += ` WHERE p.codex_id IN (?` + strings.Repeat(", ?", len(codexIDs)-1) + `)`
		for _, id := range codexIDs {
			args = append(args, id)
		}
	}
	q += ` ORDER BY r.created_at DESC, r.number DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RecentRevision{}
	for rows.Next() {
		var r models.RecentRevision
		if err := rows.Scan(&r.PageID, &r.Title, &r.Kind, &r.CodexID, &r.Number, &r.Note, &r.AuthorID, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SavePageRevision adds a revision in one transaction. A save made against anything but the
// latest revision is refused (PageStale, with the latest) so the caller can show what changed.
func (s *SQLiteStore) SavePageRevision(id string, base int, rev models.PageRevision, keep int) (models.PageSaveResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return models.PageSaveResult{}, err
	}
	defer tx.Rollback()
	codex, title, err := pageCodex(tx, id)
	if err != nil {
		return models.PageSaveResult{}, err
	}
	if err := checkPageText(tx, id); err != nil {
		return models.PageSaveResult{}, err
	}
	latest, err := scanPageRevision(tx.QueryRow(`SELECT `+pageRevisionColumns+` FROM page_revisions WHERE page_id = ? ORDER BY number DESC LIMIT 1`, id))
	if err != nil {
		return models.PageSaveResult{}, err
	}
	switch {
	case base != latest.Number:
		return models.PageSaveResult{Outcome: models.PageStale, Latest: &latest}, nil
	case latest.Body == rev.Body:
		return models.PageSaveResult{Outcome: models.PageUnchanged, Revision: latest}, nil
	}
	rev.Title = title
	if rev, err = insertPageRevision(tx, id, rev, keep); err != nil {
		return models.PageSaveResult{}, err
	}
	if _, err := tx.Exec(`UPDATE pages SET updated_at = ? WHERE id = ?`, rev.CreatedAt, id); err != nil {
		return models.PageSaveResult{}, err
	}
	idx, err := loadCodexIndex(tx, codex)
	if err != nil {
		return models.PageSaveResult{}, err
	}
	if err := rebuildPageLinks(tx, idx, id, codex, rev.Body); err != nil {
		return models.PageSaveResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.PageSaveResult{}, err
	}
	return models.PageSaveResult{Outcome: models.PageSaved, Revision: rev}, nil
}

// RenamePage retitles a page and rewrites the [[links]] to its old title in the same codex
// (each rewritten page gets a revision). Links through aliases are left alone. It returns how
// many pages were rewritten.
func (s *SQLiteStore) RenamePage(id, newTitle, userID string, keep int) (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	codex, oldTitle, err := pageCodex(tx, id)
	if err != nil {
		return 0, err
	}
	if oldTitle == newTitle {
		return 0, nil
	}
	if NormName(oldTitle) != NormName(newTitle) { // a case-only rename can't collide with itself
		if conflicts, err := nameConflicts(tx, codex, id, []string{newTitle}); err != nil {
			return 0, err
		} else if len(conflicts) > 0 {
			return 0, &NameTakenError{conflicts}
		}
	}
	note := "Renamed " + oldTitle + " → " + newTitle

	// Pages whose links resolve to this page through its old title.
	rows, err := tx.Query(`SELECT from_page_id, target_text FROM page_links WHERE to_page_id = ?`, id)
	if err != nil {
		return 0, err
	}
	sources := map[string]bool{}
	for rows.Next() {
		var from, text string
		if err := rows.Scan(&from, &text); err != nil {
			rows.Close()
			return 0, err
		}
		if NormName(text) == NormName(oldTitle) {
			sources[from] = true
		}
	}
	rows.Close()

	rewritten := 0
	changed := []string{}
	for from := range sources {
		var pt, body string
		if err := tx.QueryRow(`SELECT p.title, r.body FROM pages p JOIN page_revisions r ON r.page_id = p.id
			WHERE p.id = ? ORDER BY r.number DESC LIMIT 1`, from).Scan(&pt, &body); err != nil {
			return 0, err
		}
		nb, n := rewriteLinksTo(body, oldTitle, newTitle)
		if n == 0 {
			continue
		}
		if from == id {
			pt = newTitle
		}
		if _, err := insertPageRevision(tx, from, models.PageRevision{AuthorID: userID, Title: pt, Body: nb, Note: note}, keep); err != nil {
			return 0, err
		}
		changed = append(changed, from)
		rewritten++
	}
	now := nowUTC()
	if _, err := tx.Exec(`UPDATE pages SET title = ?, updated_at = ? WHERE id = ?`, newTitle, now, id); err != nil {
		return 0, err
	}
	for _, from := range append(changed, id) {
		if _, err := tx.Exec(`UPDATE pages SET updated_at = ? WHERE id = ?`, now, from); err != nil {
			return 0, err
		}
	}
	if !contains(changed, id) {
		var body string
		if err := tx.QueryRow(`SELECT body FROM page_revisions WHERE page_id = ? ORDER BY number DESC LIMIT 1`, id).Scan(&body); err != nil {
			return 0, err
		}
		if _, err := insertPageRevision(tx, id, models.PageRevision{AuthorID: userID, Title: newTitle, Body: body, Note: note}, keep); err != nil {
			return 0, err
		}
	}
	// An alias equal to the new title is redundant now.
	aliases, err := pageAliases(tx, id)
	if err != nil {
		return 0, err
	}
	for _, a := range aliases {
		if NormName(a) == NormName(newTitle) {
			if _, err := tx.Exec(`DELETE FROM page_aliases WHERE page_id = ? AND alias = ?`, id, a); err != nil {
				return 0, err
			}
		}
	}
	// Re-derive links from the new bodies, then every resolution in this codex.
	idx, err := loadCodexIndex(tx, codex)
	if err != nil {
		return 0, err
	}
	for _, from := range changed {
		var body string
		if err := tx.QueryRow(`SELECT body FROM page_revisions WHERE page_id = ? ORDER BY number DESC LIMIT 1`, from).Scan(&body); err != nil {
			return 0, err
		}
		fw, _, err := pageCodex(tx, from)
		if err != nil {
			return 0, err
		}
		if err := rebuildPageLinks(tx, idx, from, fw, body); err != nil {
			return 0, err
		}
	}
	if err := resolveLinksWith(tx, idx, codex); err != nil {
		return 0, err
	}
	return rewritten, tx.Commit()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// SetPageAliases replaces a page's aliases (callers dedupe and drop any equal to the title).
func (s *SQLiteStore) SetPageAliases(id string, aliases []string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	codex, _, err := pageCodex(tx, id)
	if err != nil {
		return err
	}
	if conflicts, err := nameConflicts(tx, codex, id, aliases); err != nil {
		return err
	} else if len(conflicts) > 0 {
		return &NameTakenError{conflicts}
	}
	if _, err := tx.Exec(`DELETE FROM page_aliases WHERE page_id = ?`, id); err != nil {
		return err
	}
	for _, a := range aliases {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO page_aliases (page_id, alias) VALUES (?, ?)`, id, a); err != nil {
			return err
		}
	}
	if err := resolveCodexLinks(tx, codex); err != nil {
		return err
	}
	return tx.Commit()
}

// MovePage puts a page in another codex. Its title and aliases must be free there.
// Links from the old codex to it turn missing; its own links re-resolve in the new codex.
func (s *SQLiteStore) MovePage(id, codexID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	codex, title, err := pageCodex(tx, id)
	if err != nil {
		return err
	}
	if codex == codexID {
		return nil
	}
	if err := codexExists(tx, codexID); err != nil {
		return err
	}
	aliases, err := pageAliases(tx, id)
	if err != nil {
		return err
	}
	if conflicts, err := nameConflicts(tx, codexID, id, append([]string{title}, aliases...)); err != nil {
		return err
	} else if len(conflicts) > 0 {
		return &NameTakenError{conflicts}
	}
	// Tags belong to a codex, so the page leaves its tags behind.
	if _, err := tx.Exec(`DELETE FROM page_tags WHERE page_id = ?`, id); err != nil {
		return err
	}
	// Timeline events never link across codices: events elsewhere that link this page keep its
	// title, and if this page is a timeline its own events keep their titles but lose their links.
	if err := unlinkTimelineEventsTx(tx, id, title); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE timeline_events SET title = COALESCE((SELECT title FROM pages WHERE id = linked_page_id), title), linked_page_id = '', updated_at = ? WHERE page_id = ? AND linked_page_id <> ''`, nowUTC(), id); err != nil {
		return err
	}
	// The same for map pins.
	if err := unlinkMapPinsTx(tx, id, title); err != nil {
		return err
	}
	if err := unlinkOwnPinsTx(tx, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE pages SET codex_id = ?, folder_id = '', updated_at = ? WHERE id = ?`, codexID, nowUTC(), id); err != nil {
		return err
	}
	if err := resolveCodexLinks(tx, codex, codexID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeletePage removes a page with everything attached to it and returns the image files (map
// and page pictures) and the attachment files to delete from disk. Links that pointed at it turn missing; events and pins that linked it become plain ones.
func (s *SQLiteStore) DeletePage(id string) (files, attachments []string, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	codex, title, err := pageCodex(tx, id)
	if err != nil {
		return nil, nil, err
	}
	if err := unlinkTimelineEventsTx(tx, id, title); err != nil {
		return nil, nil, err
	}
	if err := unlinkMapPinsTx(tx, id, title); err != nil {
		return nil, nil, err
	}
	if files, err = queryFiles(tx, `SELECT file FROM map_images WHERE page_id = ? AND file != '' UNION SELECT file FROM page_images WHERE page_id = ?`, id, id); err != nil {
		return nil, nil, err
	}
	if attachments, err = queryFiles(tx, `SELECT file FROM page_attachments WHERE page_id = ?`, id); err != nil {
		return nil, nil, err
	}
	for _, q := range []string{
		`DELETE FROM page_revisions WHERE page_id = ?`,
		`DELETE FROM page_contributors WHERE page_id = ?`,
		`DELETE FROM page_aliases WHERE page_id = ?`,
		`DELETE FROM page_links WHERE from_page_id = ?`,
		`DELETE FROM map_pins WHERE map_page_id = ?`,
		`DELETE FROM map_pin_events WHERE map_page_id = ?`,
		`DELETE FROM map_images WHERE page_id = ?`,
		`DELETE FROM map_scales WHERE page_id = ?`,
		`DELETE FROM page_images WHERE page_id = ?`,
		`DELETE FROM page_attachments WHERE page_id = ?`,
		`DELETE FROM page_tags WHERE page_id = ?`,
		`DELETE FROM timeline_events WHERE page_id = ?`,
		`DELETE FROM timeline_calendars WHERE page_id = ?`,
		`DELETE FROM pages WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return nil, nil, err
		}
	}
	if err := resolveCodexLinks(tx, codex); err != nil {
		return nil, nil, err
	}
	return files, attachments, tx.Commit()
}
