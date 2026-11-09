package db

import (
	"database/sql"
	"strings"
	"time"
	"waystone-web/models"
)

const worldColumns = `id, name, description, created_by, created_at, updated_at`

func scanWorld(row rowScanner) (models.World, error) {
	var w models.World
	err := row.Scan(&w.ID, &w.Name, &w.Description, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

func (s *SQLiteStore) GetWorlds() ([]models.World, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT ` + worldColumns + ` FROM worlds ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.World{}
	for rows.Next() {
		w, err := scanWorld(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetWorld returns nil, nil when the world does not exist.
func (s *SQLiteStore) GetWorld(id string) (*models.World, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	w, err := scanWorld(s.db.QueryRow(`SELECT `+worldColumns+` FROM worlds WHERE id = ?`, id))
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

func (s *SQLiteStore) CreateWorld(w models.World) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, err := s.db.Exec(`INSERT INTO worlds (`+worldColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Description, w.CreatedBy, w.CreatedAt, w.UpdatedAt)
	if isUniqueViolation(err) {
		return ErrWorldNameTaken
	}
	return err
}

func (s *SQLiteStore) UpdateWorld(id, name, description string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	res, err := s.db.Exec(`UPDATE worlds SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
		name, description, time.Now().UTC().Format(time.RFC3339), id)
	if isUniqueViolation(err) {
		return ErrWorldNameTaken
	}
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrWorldNotFound
	}
	return nil
}

// DeleteWorld removes an empty world: it is refused (ErrWorldInUse) while a game points at it
// or any page still lives in it, so there is nothing else to cascade.
func (s *SQLiteStore) DeleteWorld(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var inUse int
	if err := tx.QueryRow(`SELECT (SELECT COUNT(*) FROM campaigns WHERE world_id = ?) + (SELECT COUNT(*) FROM pages WHERE world_id = ?)`, id, id).Scan(&inUse); err != nil {
		return err
	}
	if inUse > 0 {
		return ErrWorldInUse
	}
	res, err := tx.Exec(`DELETE FROM worlds WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrWorldNotFound
	}
	return tx.Commit()
}

// SetCampaignWorld points a game at a world (” clears it). It is only a label: links don't change.
func (s *SQLiteStore) SetCampaignWorld(campaignID, worldID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if worldID != "" {
		var one int
		if err := s.db.QueryRow(`SELECT 1 FROM worlds WHERE id = ?`, worldID).Scan(&one); err == sql.ErrNoRows {
			return ErrWorldNotFound
		} else if err != nil {
			return err
		}
	}
	res, err := s.db.Exec(`UPDATE campaigns SET world_id = ? WHERE id = ?`, worldID, campaignID)
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
