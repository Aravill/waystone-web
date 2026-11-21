package db

import (
	"database/sql"
	"waystone-web/models"
)

func (s *SQLiteStore) GetUserRights(userID string) ([]string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT action FROM user_rights WHERE user_id = ? ORDER BY action`, userID)
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

func (s *SQLiteStore) GetAllUserRights() (map[string][]string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT user_id, action FROM user_rights ORDER BY action`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var u, a string
		if err := rows.Scan(&u, &a); err != nil {
			return nil, err
		}
		out[u] = append(out[u], a)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SetUserRight(userID, action string, on bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	var err error
	if on {
		_, err = s.db.Exec(`INSERT OR IGNORE INTO user_rights (user_id, action) VALUES (?, ?)`, userID, action)
	} else {
		_, err = s.db.Exec(`DELETE FROM user_rights WHERE user_id = ? AND action = ?`, userID, action)
	}
	return err
}

func (s *SQLiteStore) GetCodexAccessForUser(userID string) (map[string]string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT codex_id, level FROM codex_access WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, level string
		if err := rows.Scan(&id, &level); err != nil {
			return nil, err
		}
		out[id] = level
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetAllCodexAccess() ([]models.CodexAccess, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT codex_id, user_id, level FROM codex_access ORDER BY codex_id, user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CodexAccess{}
	for rows.Next() {
		var a models.CodexAccess
		if err := rows.Scan(&a.CodexID, &a.UserID, &a.Level); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetCodexAccess sets a user's level on a codex ("see" or "edit"); "" or "none" removes the row.
func (s *SQLiteStore) SetCodexAccess(codexID, userID, level string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if level == "" || level == "none" {
		_, err := s.db.Exec(`DELETE FROM codex_access WHERE codex_id = ? AND user_id = ?`, codexID, userID)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO codex_access (codex_id, user_id, level) VALUES (?, ?, ?)
		ON CONFLICT(codex_id, user_id) DO UPDATE SET level = excluded.level`, codexID, userID, level)
	return err
}

// GetPageCodexID returns the codex a page lives in, or "" when the page doesn't exist.
func (s *SQLiteStore) GetPageCodexID(pageID string) (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	var id string
	err := s.db.QueryRow(`SELECT codex_id FROM pages WHERE id = ?`, pageID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

// GetImageCodexID returns the codex of the page that owns an uploaded image file (a map image or a page picture), or "" if none does.
func (s *SQLiteStore) GetImageCodexID(file string) (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	var id string
	err := s.db.QueryRow(`SELECT p.codex_id FROM map_images i JOIN pages p ON p.id = i.page_id WHERE i.file = ?
		UNION ALL SELECT p.codex_id FROM page_images i JOIN pages p ON p.id = i.page_id WHERE i.file = ? LIMIT 1`, file, file).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}
