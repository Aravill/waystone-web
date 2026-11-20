package db

import (
	"database/sql"
	"fmt"
	"strings"
	"waystone-web/models"
)

const codexColumns = `id, name, description, created_by, created_at, updated_at`

func scanCodex(row rowScanner) (models.Codex, error) {
	var w models.Codex
	err := row.Scan(&w.ID, &w.Name, &w.Description, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

func (s *SQLiteStore) GetCodices() ([]models.Codex, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT ` + codexColumns + ` FROM codices ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Codex{}
	for rows.Next() {
		w, err := scanCodex(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetCodex returns nil, nil when the codex does not exist.
func (s *SQLiteStore) GetCodex(id string) (*models.Codex, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	w, err := scanCodex(s.db.QueryRow(`SELECT `+codexColumns+` FROM codices WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (s *SQLiteStore) CreateCodex(w models.Codex) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, err := s.db.Exec(`INSERT INTO codices (`+codexColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Description, w.CreatedBy, w.CreatedAt, w.UpdatedAt)
	if isUniqueViolation(err) {
		return ErrCodexNameTaken
	}
	return err
}

func (s *SQLiteStore) UpdateCodex(id, name, description string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	res, err := s.db.Exec(`UPDATE codices SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
		name, description, nowUTC(), id)
	if isUniqueViolation(err) {
		return ErrCodexNameTaken
	}
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrCodexNotFound
	}
	return nil
}

// DeleteCodex removes an empty codex: it is refused (ErrCodexInUse) while any page still lives
// in it. Games that pointed at it lose their codex label.
func (s *SQLiteStore) DeleteCodex(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var inUse int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM pages WHERE codex_id = ?`, id).Scan(&inUse); err != nil {
		return err
	}
	if inUse > 0 {
		return ErrCodexInUse
	}
	if _, err := tx.Exec(`DELETE FROM codex_folders WHERE codex_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE campaigns SET codex_id = '' WHERE codex_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM codices WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrCodexNotFound
	}
	return tx.Commit()
}

// SetCampaignCodex points a game at a codex ("" clears it). It is only a label: links don't change.
func (s *SQLiteStore) SetCampaignCodex(campaignID, codexID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if codexID != "" {
		var one int
		if err := s.db.QueryRow(`SELECT 1 FROM codices WHERE id = ?`, codexID).Scan(&one); err == sql.ErrNoRows {
			return ErrCodexNotFound
		} else if err != nil {
			return err
		}
	}
	res, err := s.db.Exec(`UPDATE campaigns SET codex_id = ? WHERE id = ?`, codexID, campaignID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrCampaignNotFound
	}
	return nil
}

// tableExists and columnExists inspect the schema for the one-off renames below.
func (s *SQLiteStore) tableExists(name string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	return n > 0, err
}

func (s *SQLiteStore) columnExists(table, column string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n)
	return n > 0, err
}

// renameWorldsToCodices renames the old "world" tables and columns in place (idempotent). It
// must run before the CREATE block, which would otherwise create fresh empty codices tables
// and an index on a column that doesn't exist yet.
func (s *SQLiteStore) renameWorldsToCodices() error {
	oldTable, err := s.tableExists("worlds")
	if err != nil {
		return fmt.Errorf("failed to inspect worlds table: %w", err)
	}
	newTable, err := s.tableExists("codices")
	if err != nil {
		return fmt.Errorf("failed to inspect codices table: %w", err)
	}
	if oldTable && !newTable {
		if _, err := s.db.Exec(`ALTER TABLE worlds RENAME TO codices`); err != nil {
			return fmt.Errorf("failed to rename worlds to codices: %w", err)
		}
	}
	for _, table := range []string{"pages", "codex_folders", "campaigns"} {
		exists, err := s.tableExists(table)
		if err != nil {
			return fmt.Errorf("failed to inspect %s table: %w", table, err)
		}
		if !exists {
			continue
		}
		old, err := s.columnExists(table, "world_id")
		if err != nil {
			return fmt.Errorf("failed to inspect %s table: %w", table, err)
		}
		if old {
			if _, err := s.db.Exec(`ALTER TABLE ` + table + ` RENAME COLUMN world_id TO codex_id`); err != nil {
				return fmt.Errorf("failed to rename %s.world_id: %w", table, err)
			}
		}
	}
	for _, idx := range []string{"idx_pages_world", "idx_codex_folders_parent"} {
		if _, err := s.db.Exec(`DROP INDEX IF EXISTS ` + idx); err != nil {
			return fmt.Errorf("failed to drop %s: %w", idx, err)
		}
	}
	return nil
}

// rescueUnsortedPages moves pages that sat in the old Unsorted folder (codex_id '') into a codex
// of their own, since every page now belongs to a codex. It does nothing when there are none.
func (s *SQLiteStore) rescueUnsortedPages() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pages WHERE codex_id = ''`).Scan(&n); err != nil || n == 0 {
		return err
	}
	const name = "Unsorted pages"
	var id string
	err := s.db.QueryRow(`SELECT id FROM codices WHERE name = ?`, name).Scan(&id)
	if err == sql.ErrNoRows {
		id = GenerateUUID()
		now := nowUTC()
		if _, err := s.db.Exec(`INSERT INTO codices (id, name, description, created_by, created_at, updated_at) VALUES (?, ?, ?, '', ?, ?)`,
			id, name, "Pages that were in the old Unsorted folder.", now, now); err != nil {
			return fmt.Errorf("failed to create the rescue codex: %w", err)
		}
	} else if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE pages SET codex_id = ?, folder_id = '' WHERE codex_id = ''`, id); err != nil {
		return fmt.Errorf("failed to move unsorted pages: %w", err)
	}
	if err := resolveCodexLinks(tx, id); err != nil {
		return fmt.Errorf("failed to resolve links of unsorted pages: %w", err)
	}
	return tx.Commit()
}
