package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"unicode"
	"unicode/utf8"
	"waystone-web/models"
)

// ---- The game journal as codex pages ----
//
// A session's journal is an ordinary codex page (kind 'lore') whose pages.session_id is the
// session. The pages live in a folder "{game}: Campaign Journal" at the top level of the game's
// codex, found by campaigns.journal_folder_id. The legacy journal_* tables are only read by the
// migration below and by the seed (SaveJournalRevision); they are dropped in a later cleanup.

const (
	journalFolderTemplate = "builtin:session" // what a new journal folder starts new pages from
	journalGameMax        = 62                // runes of the game's title in a folder or page title
	journalTitleMax       = 112               // leaves room for " (n)" under the 120-rune title limit
	journalFolderMax      = 80
)

func cutRunes(s string, n int) string {
	if n < 0 {
		n = 0
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n]))
}

// journalFolderTails is what a journal folder is called after "{game}: ", by the codex's language
// (codexJournalPrefix in static/codex.js knows the same names).
var journalFolderTails = map[string]string{
	models.LangEnglish: "Campaign Journal",
	models.LangCzech:   "Deník tažení",
}

// journalFolderName is the generated folder name for a game: "{game}: Campaign Journal", the game
// part cut to 62 characters with "/" written as "-", plus " (n)" from the second try on.
func journalFolderName(game, lang string, n int) string {
	suffix := ""
	if n > 1 {
		suffix = fmt.Sprintf(" (%d)", n)
	}
	g := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ReplaceAll(game, "/", "-"))
	g = cutRunes(strings.Join(strings.Fields(g), " "), journalGameMax-utf8.RuneCountInString(suffix))
	if g == "" {
		g = "Game"
	}
	return g + ": " + journalFolderTails[lang] + suffix
}

// journalPageBase is a journal page's title before it is made unique: "{game}: {date}", plus
// " – {name}" when a migrated entry's newest revision had a name. A page title can't contain [ ] or |.
func journalPageBase(game, date, name string) string {
	clean := func(s string) string {
		s = strings.Map(func(r rune) rune {
			if r == '[' || r == ']' || r == '|' {
				return -1
			}
			return r
		}, s)
		return strings.Join(strings.Fields(s), " ")
	}
	g := cutRunes(clean(game), journalGameMax)
	if g == "" {
		g = "Game"
	}
	base := g + ": " + date
	if n := clean(name); n != "" {
		if room := journalTitleMax - utf8.RuneCountInString(base) - 3; room >= 1 {
			base += " – " + cutRunes(n, room)
		}
	}
	return base
}

// uniqueJournalTitle makes base unique among the titles and aliases of the codex with " (2)", " (3)"…
func uniqueJournalTitle(q querier, codexID, base string) (string, error) {
	for n := 1; ; n++ {
		title := base
		if n > 1 {
			title = fmt.Sprintf("%s (%d)", base, n)
		}
		taken, err := nameConflicts(q, codexID, "", []string{title})
		if err != nil {
			return "", err
		}
		if len(taken) == 0 {
			return title, nil
		}
	}
}

// ensureJournalFolderTx returns the game's journal folder in the codex, creating it at the top
// level (and remembering it in campaigns.journal_folder_id) when there is none or it is gone.
func ensureJournalFolderTx(tx *sql.Tx, campaignID, codexID, userID string) (models.Folder, error) {
	var game, folderID string
	if err := tx.QueryRow(`SELECT title, journal_folder_id FROM campaigns WHERE id = ?`, campaignID).Scan(&game, &folderID); err == sql.ErrNoRows {
		return models.Folder{}, ErrCampaignNotFound
	} else if err != nil {
		return models.Folder{}, err
	}
	if folderID != "" {
		f, err := folderByID(tx, folderID)
		if err == nil && f.CodexID == codexID {
			return f, nil
		}
		if err != nil && err != ErrFolderNotFound {
			return models.Folder{}, err
		}
	}
	var lang string
	if err := tx.QueryRow(`SELECT language FROM codices WHERE id = ?`, codexID).Scan(&lang); err == sql.ErrNoRows {
		return models.Folder{}, ErrCodexNotFound
	} else if err != nil {
		return models.Folder{}, err
	}
	name := ""
	for n := 1; ; n++ {
		name = journalFolderName(game, lang, n)
		if free, err := siblingNameFree(tx, codexID, "", name, ""); err != nil {
			return models.Folder{}, err
		} else if free {
			break
		}
	}
	now := nowUTC()
	f := models.Folder{ID: GenerateUUID(), CodexID: codexID, Name: name, DefaultTemplate: journalFolderTemplate, CreatedBy: userID, CreatedAt: now, UpdatedAt: now}
	if _, err := tx.Exec(`INSERT INTO codex_folders (`+folderColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.CodexID, f.ParentID, f.Name, f.DefaultTemplate, f.CreatedBy, f.CreatedAt, f.UpdatedAt); err != nil {
		return models.Folder{}, err
	}
	if _, err := tx.Exec(`UPDATE campaigns SET journal_folder_id = ? WHERE id = ?`, f.ID, campaignID); err != nil {
		return models.Folder{}, err
	}
	return f, nil
}

// EnsureJournalFolder gives the id of the game's journal folder in the codex (created if needed)
// and the template new pages there start from: the folder's own default or the nearest ancestor's.
func (s *SQLiteStore) EnsureJournalFolder(campaignID, codexID, userID string) (string, string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	if err := codexExists(tx, codexID); err != nil {
		return "", "", err
	}
	f, err := ensureJournalFolderTx(tx, campaignID, codexID, userID)
	if err != nil {
		return "", "", err
	}
	all, err := foldersOf(tx, codexID)
	if err != nil {
		return "", "", err
	}
	byID := map[string]models.Folder{}
	for _, x := range all {
		byID[x.ID] = x
	}
	template := ""
	for cur, hops := f.ID, 0; cur != "" && hops <= len(all); hops++ {
		x, ok := byID[cur]
		if !ok {
			break
		}
		if x.DefaultTemplate != "" {
			template = x.DefaultTemplate
			break
		}
		cur = x.ParentID
	}
	return f.ID, template, tx.Commit()
}

// CreateJournalPage writes the journal page of session p.SessionID in the game's journal folder,
// in one transaction: the title is base made unique in the codex, and a session that already has a
// page gets no second one (that page's id comes back with created false).
func (s *SQLiteStore) CreateJournalPage(campaignID string, p models.Page, base, body string, x models.NewPageExtras) (string, bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	var one int
	if err := tx.QueryRow(`SELECT 1 FROM campaign_sessions WHERE id = ? AND campaign_id = ?`, p.SessionID, campaignID).Scan(&one); err == sql.ErrNoRows {
		return "", false, ErrJournalSessionGone
	} else if err != nil {
		return "", false, err
	}
	var existing string
	if err := tx.QueryRow(`SELECT id FROM pages WHERE session_id = ?`, p.SessionID).Scan(&existing); err == nil {
		return existing, false, nil
	} else if err != sql.ErrNoRows {
		return "", false, err
	}
	// The folder may have been deleted since the caller looked it up.
	f, err := ensureJournalFolderTx(tx, campaignID, p.CodexID, p.AuthorID)
	if err != nil {
		return "", false, err
	}
	p.FolderID, p.Kind = f.ID, models.PageLore
	if p.Title, err = uniqueJournalTitle(tx, p.CodexID, base); err != nil {
		return "", false, err
	}
	if err := createPageWithTx(tx, p, body, x); err != nil {
		return "", false, err
	}
	return p.ID, true, tx.Commit()
}

// GetJournalPageID returns the id of a session's journal page, or "".
func (s *SQLiteStore) GetJournalPageID(sessionID string) (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	var id string
	err := s.db.QueryRow(`SELECT id FROM pages WHERE session_id = ?`, sessionID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

// GetJournalPages returns the journal pages of a campaign's sessions (in any codex), keyed by
// session id, each with its newest revision's excerpt source and who amended it.
func (s *SQLiteStore) GetJournalPages(campaignID string) (map[string]models.JournalPage, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`
		SELECT p.id, p.title, p.codex_id, p.session_id, p.author_id, r.created_at, substr(r.body, 1, 600),
		       (SELECT COUNT(*) FROM page_revisions WHERE page_id = p.id)
		FROM pages p
		JOIN campaign_sessions s ON s.id = p.session_id AND s.campaign_id = ?
		JOIN page_revisions r ON r.page_id = p.id
		  AND r.number = (SELECT MAX(number) FROM page_revisions WHERE page_id = p.id)
		WHERE p.session_id <> ''`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]models.JournalPage{}
	byPage := map[string]string{} // page id -> session id
	for rows.Next() {
		var j models.JournalPage
		if err := rows.Scan(&j.PageID, &j.Title, &j.CodexID, &j.SessionID, &j.AuthorID, &j.UpdatedAt, &j.ExcerptSource, &j.Revisions); err != nil {
			return nil, err
		}
		j.AmenderIDs = []string{}
		result[j.SessionID] = j
		byPage[j.PageID] = j.SessionID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	people, err := s.db.Query(`
		SELECT c.page_id, c.user_id
		FROM page_contributors c
		JOIN pages p ON p.id = c.page_id
		JOIN campaign_sessions s ON s.id = p.session_id AND s.campaign_id = ?
		WHERE p.session_id <> ''
		ORDER BY c.page_id, c.first_revision`, campaignID)
	if err != nil {
		return nil, err
	}
	defer people.Close()
	for people.Next() {
		var pageID, userID string
		if err := people.Scan(&pageID, &userID); err != nil {
			return nil, err
		}
		j, ok := result[byPage[pageID]]
		if ok && userID != j.AuthorID {
			j.AmenderIDs = append(j.AmenderIDs, userID)
			result[j.SessionID] = j
		}
	}
	return result, people.Err()
}

// CountLegacyJournal counts a campaign's entries still in the legacy journal tables: written for
// a session that exists and waiting for the game to get a codex.
func (s *SQLiteStore) CountLegacyJournal(campaignID string) (int, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM journal_entries e JOIN campaign_sessions s ON s.id = e.session_id
		WHERE e.campaign_id = ? AND EXISTS (SELECT 1 FROM journal_revisions WHERE entry_id = e.id)`, campaignID).Scan(&n)
	return n, err
}

// RenameJournalFolder renames the game's journal folder after the game was renamed, but only while
// it still has the generated name for the old title (in either language; it keeps the one it was made
// in) and the new name is free next to it. Page titles never change.
func (s *SQLiteStore) RenameJournalFolder(campaignID, oldTitle, newTitle string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var folderID string
	if err := tx.QueryRow(`SELECT journal_folder_id FROM campaigns WHERE id = ?`, campaignID).Scan(&folderID); err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	if folderID == "" {
		return nil
	}
	f, err := folderByID(tx, folderID)
	if err == ErrFolderNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	name := ""
	for lang := range journalFolderTails {
		if f.Name == journalFolderName(oldTitle, lang, 1) {
			name = journalFolderName(newTitle, lang, 1)
		}
	}
	if name == "" || name == f.Name {
		return nil
	}
	if free, err := siblingNameFree(tx, f.CodexID, f.ParentID, name, f.ID); err != nil {
		return err
	} else if !free {
		return nil
	}
	if _, err := tx.Exec(`UPDATE codex_folders SET name = ?, updated_at = ? WHERE id = ?`, name, nowUTC(), f.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetCampaignCodex points a game at a codex ("" clears it). Links don't change and no page moves.
// Changing the codex forgets the journal folder (a new one is made in the new codex when the next
// entry is written); linking a codex moves the game's legacy journal entries into it, all in one
// transaction.
func (s *SQLiteStore) SetCampaignCodex(campaignID, codexID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if codexID != "" {
		if err := codexExists(tx, codexID); err != nil {
			return err
		}
	}
	res, err := tx.Exec(`UPDATE campaigns SET journal_folder_id = CASE WHEN codex_id <> ? THEN '' ELSE journal_folder_id END, codex_id = ? WHERE id = ?`,
		codexID, codexID, campaignID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrCampaignNotFound
	}
	if _, err := migrateCampaignJournalTx(tx, campaignID); err != nil {
		return fmt.Errorf("failed to move the journal into the codex: %w", err)
	}
	return tx.Commit()
}

type legacyEntry struct{ id, sessionID, authorID, createdAt, date string }

// migrateCampaignJournalTx moves the campaign's legacy journal entries into pages of its codex and
// returns how many moved. It does nothing when the game has no codex (or a missing one). Each
// entry whose session still exists becomes one page: titled "{game}: {date}" (plus " – {name}" from
// the newest revision, made unique), filed in the journal folder, with every kept revision under
// its old number (body, restored_from, created_at, note plus "Merged with revision N") and the old
// contributors. Its legacy rows are deleted at the end, which makes the migration idempotent;
// entries without a session, without revisions, or whose session already has a page are kept.
func migrateCampaignJournalTx(tx *sql.Tx, campaignID string) (int, error) {
	var game, codexID, dm string
	if err := tx.QueryRow(`SELECT title, codex_id, dm FROM campaigns WHERE id = ?`, campaignID).Scan(&game, &codexID, &dm); err == sql.ErrNoRows {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	if codexID == "" {
		return 0, nil
	}
	if err := codexExists(tx, codexID); err == ErrCodexNotFound {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	rows, err := tx.Query(`SELECT e.id, e.session_id, e.author_id, e.created_at, s.date
		FROM journal_entries e JOIN campaign_sessions s ON s.id = e.session_id
		WHERE e.campaign_id = ? ORDER BY s.date, e.created_at, e.id`, campaignID)
	if err != nil {
		return 0, err
	}
	var entries []legacyEntry
	for rows.Next() {
		var e legacyEntry
		if err := rows.Scan(&e.id, &e.sessionID, &e.authorID, &e.createdAt, &e.date); err != nil {
			rows.Close()
			return 0, err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}

	type moved struct{ pageID, body string }
	var pages []moved
	var folder models.Folder
	for _, e := range entries {
		var taken int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM pages WHERE session_id = ?`, e.sessionID).Scan(&taken); err != nil {
			return 0, err
		}
		if taken > 0 {
			log.Printf("journal: session %s already has a page; its legacy entry stays", e.sessionID)
			continue
		}
		revs, err := legacyRevisions(tx, e.id)
		if err != nil {
			return 0, err
		}
		if len(revs) == 0 {
			continue
		}
		newest := revs[len(revs)-1]
		if folder.ID == "" {
			if folder, err = ensureJournalFolderTx(tx, campaignID, codexID, dm); err != nil {
				return 0, err
			}
		}
		title, err := uniqueJournalTitle(tx, codexID, journalPageBase(game, e.date, newest.Name))
		if err != nil {
			return 0, err
		}
		pageID := GenerateUUID()
		if _, err := tx.Exec(`INSERT INTO pages (id, kind, codex_id, folder_id, session_id, title, author_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			pageID, models.PageLore, codexID, folder.ID, e.sessionID, title, e.authorID, e.createdAt, newest.CreatedAt); err != nil {
			return 0, err
		}
		for _, r := range revs {
			note := r.Note
			if r.MergedWith > 0 {
				merged := fmt.Sprintf("Merged with revision %d", r.MergedWith)
				if note != "" {
					note += " · " + merged
				} else {
					note = merged
				}
			}
			if _, err := tx.Exec(`INSERT INTO page_revisions (page_id, `+pageRevisionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				pageID, r.Number, r.AuthorID, title, r.Body, note, r.RestoredFrom, r.CreatedAt); err != nil {
				return 0, err
			}
		}
		if _, err := tx.Exec(`INSERT INTO page_contributors (page_id, user_id, revisions, first_revision)
			SELECT ?, user_id, revisions, first_revision FROM journal_contributors WHERE entry_id = ?`, pageID, e.id); err != nil {
			return 0, err
		}
		for _, table := range []string{"journal_contributors", "journal_revisions"} {
			if _, err := tx.Exec(`DELETE FROM `+table+` WHERE entry_id = ?`, e.id); err != nil {
				return 0, err
			}
		}
		if _, err := tx.Exec(`DELETE FROM journal_entries WHERE id = ?`, e.id); err != nil {
			return 0, err
		}
		pages = append(pages, moved{pageID, newest.Body})
	}
	if len(pages) == 0 {
		return 0, nil
	}
	idx, err := loadCodexIndex(tx, codexID)
	if err != nil {
		return 0, err
	}
	for _, p := range pages {
		if err := rebuildPageLinks(tx, idx, p.pageID, codexID, p.body); err != nil {
			return 0, err
		}
	}
	if err := resolveLinksWith(tx, idx, codexID); err != nil { // other pages' links to the new titles
		return 0, err
	}
	return len(pages), nil
}

func legacyRevisions(tx *sql.Tx, entryID string) ([]models.JournalRevision, error) {
	rows, err := tx.Query(`SELECT `+journalRevisionColumns+` FROM journal_revisions WHERE entry_id = ? ORDER BY number`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var revs []models.JournalRevision
	for rows.Next() {
		r, err := scanJournalRevision(rows)
		if err != nil {
			return nil, err
		}
		revs = append(revs, r)
	}
	return revs, rows.Err()
}

// migrateJournalToCodex runs at startup (a failing game is logged and skipped): every game that has a codex gets its legacy journal
// entries moved into it, one transaction per game. Entries of games without a codex wait until the
// DM links one (SetCampaignCodex). Nothing is left to do on later starts, because moved rows are gone.
func (s *SQLiteStore) migrateJournalToCodex() error {
	rows, err := s.db.Query(`SELECT DISTINCT e.campaign_id FROM journal_entries e JOIN campaigns c ON c.id = e.campaign_id WHERE c.codex_id <> '' ORDER BY e.campaign_id`)
	if err != nil {
		return fmt.Errorf("failed to find journal entries to move: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		n, err := migrateCampaignJournalTx(tx, id)
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			// One game's failure must not stop the server: its legacy rows stay and the next start retries.
			tx.Rollback()
			log.Printf("journal: could not move the journal of game %s into its codex (kept as it was): %v", id, err)
			continue
		}
		if n > 0 {
			log.Printf("journal: moved %d entries of game %s into its codex", n, id)
		}
	}
	return nil
}

// JournalPageBase is the title a session's journal page starts from, before it is made unique
// (the API fills a template's {{title}} with it).
func JournalPageBase(game, date string) string { return journalPageBase(game, date, "") }
