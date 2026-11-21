package db

import "waystone-web/models"

// graphBodyChars is how much of a page's body the graph reads, enough for an excerpt.
const graphBodyChars = 1500

// GetCodexGraph returns the codex's pages (with the start of the newest body and their tags) and
// the stored links between them in three queries.
func (s *SQLiteStore) GetCodexGraph(codexID string) ([]models.GraphPage, []models.GraphLink, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT p.id, p.title, p.folder_id,
		COALESCE((SELECT substr(r.body, 1, ?) FROM page_revisions r WHERE r.page_id = p.id ORDER BY r.number DESC LIMIT 1), '')
		FROM pages p WHERE p.codex_id = ? ORDER BY p.title COLLATE NOCASE, p.id`, graphBodyChars, codexID)
	if err != nil {
		return nil, nil, err
	}
	pages := []models.GraphPage{}
	at := map[string]int{}
	for rows.Next() {
		var p models.GraphPage
		if err := rows.Scan(&p.ID, &p.Title, &p.FolderID, &p.Body); err != nil {
			rows.Close()
			return nil, nil, err
		}
		p.Tags = []string{}
		at[p.ID] = len(pages)
		pages = append(pages, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	trows, err := s.db.Query(`SELECT pt.page_id, t.name FROM page_tags pt JOIN codex_tags t ON t.id = pt.tag_id WHERE t.codex_id = ? ORDER BY t.name COLLATE NOCASE`, codexID)
	if err != nil {
		return nil, nil, err
	}
	for trows.Next() {
		var id, name string
		if err := trows.Scan(&id, &name); err != nil {
			trows.Close()
			return nil, nil, err
		}
		if i, ok := at[id]; ok {
			pages[i].Tags = append(pages[i].Tags, name)
		}
	}
	err = trows.Err()
	trows.Close()
	if err != nil {
		return nil, nil, err
	}
	lrows, err := s.db.Query(`SELECT l.from_page_id, l.target_text, l.to_page_id FROM page_links l JOIN pages p ON p.id = l.from_page_id WHERE p.codex_id = ? ORDER BY l.from_page_id, l.target_text`, codexID)
	if err != nil {
		return nil, nil, err
	}
	defer lrows.Close()
	links := []models.GraphLink{}
	for lrows.Next() {
		var l models.GraphLink
		if err := lrows.Scan(&l.FromPageID, &l.TargetText, &l.ToPageID); err != nil {
			return nil, nil, err
		}
		links = append(links, l)
	}
	return pages, links, lrows.Err()
}
