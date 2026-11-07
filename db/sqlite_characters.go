package db

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
	"waystone-web/models"
)

const characterColumns = `id, owner_id, system_id, name, description, background, status, created_at, updated_at`

func scanCharacter(row rowScanner) (models.Character, error) {
	var c models.Character
	err := row.Scan(&c.ID, &c.OwnerID, &c.SystemID, &c.Name, &c.Description, &c.Background, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

const assignmentColumns = `a.id, a.character_id, a.campaign_id, a.user_id, a.started_at, a.ended_at, COALESCE(c.name, ''), COALESCE(c.status, '')`
const assignmentFrom = `character_assignments a LEFT JOIN characters c ON c.id = a.character_id`

func scanAssignment(row rowScanner) (models.CharacterAssignment, error) {
	var a models.CharacterAssignment
	err := row.Scan(&a.ID, &a.CharacterID, &a.CampaignID, &a.UserID, &a.StartedAt, &a.EndedAt, &a.CharacterName, &a.CharacterStatus)
	return a, err
}

// endUserAssignments ends the user's active assignments in one game (campaignID != "") or in all games.
func endUserAssignments(tx *sql.Tx, campaignID, userID, at string) error {
	q := `UPDATE character_assignments SET ended_at = ? WHERE user_id = ? AND ended_at = ''`
	args := []interface{}{at, userID}
	if campaignID != "" {
		q += ` AND campaign_id = ?`
		args = append(args, campaignID)
	}
	_, err := tx.Exec(q, args...)
	return err
}

func (s *SQLiteStore) CreateCharacter(c models.Character) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, err := s.db.Exec(`INSERT INTO characters (`+characterColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.OwnerID, c.SystemID, c.Name, c.Description, c.Background, c.Status, c.CreatedAt, c.UpdatedAt)
	return err
}

// GetCharacter returns nil, nil when the character does not exist.
func (s *SQLiteStore) GetCharacter(id string) (*models.Character, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	c, err := scanCharacter(s.db.QueryRow(`SELECT `+characterColumns+` FROM characters WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *SQLiteStore) GetCharactersByOwner(ownerID string) ([]models.Character, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT `+characterColumns+` FROM characters WHERE owner_id = ? ORDER BY created_at, id`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Character{}
	for rows.Next() {
		c, err := scanCharacter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateCharacter saves the owner's edits. A system change is refused (ErrCharacterAssigned) while
// the character has an active assignment; the check and the write are one statement.
func (s *SQLiteStore) UpdateCharacter(c models.Character) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	res, err := s.db.Exec(`UPDATE characters SET system_id = ?, name = ?, description = ?, background = ?, updated_at = ?
		WHERE id = ? AND (system_id = ? OR NOT EXISTS (SELECT 1 FROM character_assignments WHERE character_id = ? AND ended_at = ''))`,
		c.SystemID, c.Name, c.Description, c.Background, c.UpdatedAt, c.ID, c.SystemID, c.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM characters WHERE id = ?`, c.ID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrCharacterNotFound
	}
	return ErrCharacterAssigned
}

func (s *SQLiteStore) SetCharacterStatus(id string, status models.CharacterStatus) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	res, err := s.db.Exec(`UPDATE characters SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

// DeleteCharacter removes a character and its (history) assignments, unless one is still active.
func (s *SQLiteStore) DeleteCharacter(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM character_assignments WHERE character_id = ? AND ended_at = ''`, id).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return ErrCharacterAssigned
	}
	if _, err := tx.Exec(`DELETE FROM character_assignments WHERE character_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM characters WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCharacterNotFound
	}
	return tx.Commit()
}

// GetAssignmentsByCharacterIDs returns every assignment (history included), oldest first, keyed by character.
func (s *SQLiteStore) GetAssignmentsByCharacterIDs(ids []string) (map[string][]models.CharacterAssignment, error) {
	out := map[string][]models.CharacterAssignment{}
	if len(ids) == 0 {
		return out, nil
	}
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.Query(`SELECT `+assignmentColumns+` FROM `+assignmentFrom+` WHERE a.character_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`) ORDER BY a.started_at, a.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out[a.CharacterID] = append(out[a.CharacterID], a)
	}
	return out, rows.Err()
}

// GetCampaignAssignments returns every assignment in a game (history included), oldest first.
func (s *SQLiteStore) GetCampaignAssignments(campaignID string) ([]models.CharacterAssignment, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT `+assignmentColumns+` FROM `+assignmentFrom+` WHERE a.campaign_id = ? ORDER BY a.started_at, a.id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CharacterAssignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetActiveAssignmentsByUser returns the user's active assignments keyed by campaign id.
func (s *SQLiteStore) GetActiveAssignmentsByUser(userID string) (map[string]models.CharacterAssignment, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT `+assignmentColumns+` FROM `+assignmentFrom+` WHERE a.user_id = ? AND a.ended_at = ''`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.CharacterAssignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out[a.CampaignID] = a
	}
	return out, rows.Err()
}

// AssignCharacter makes characterID the user's active character in the game: any other active
// assignment ends and a new one starts. It reports false when it was already the active one.
func (s *SQLiteStore) AssignCharacter(campaignID, userID, characterID, systemID, at string) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var one int
	if err := tx.QueryRow(`SELECT 1 FROM characters WHERE id = ? AND owner_id = ? AND system_id = ?`, characterID, userID, systemID).Scan(&one); err == sql.ErrNoRows {
		return false, ErrCharacterMismatch
	} else if err != nil {
		return false, err
	}
	var playersJSON, dm string
	if err := tx.QueryRow(`SELECT players, dm FROM campaigns WHERE id = ?`, campaignID).Scan(&playersJSON, &dm); err == sql.ErrNoRows {
		return false, ErrCampaignNotFound
	} else if err != nil {
		return false, err
	}
	var players []string
	if err := json.Unmarshal([]byte(playersJSON), &players); err != nil {
		return false, err
	}
	isPlayer := false
	for _, p := range players {
		if p == userID {
			isPlayer = true
		}
	}
	if !isPlayer || dm == userID {
		return false, ErrNotPlayer
	}
	var current string
	err = tx.QueryRow(`SELECT character_id FROM character_assignments WHERE campaign_id = ? AND user_id = ? AND ended_at = ''`, campaignID, userID).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if err == nil && current == characterID {
		return false, nil
	}
	if err := endUserAssignments(tx, campaignID, userID, at); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`INSERT INTO character_assignments (id, character_id, campaign_id, user_id, started_at, ended_at) VALUES (?, ?, ?, ?, ?, '')`,
		GenerateUUID(), characterID, campaignID, userID, at); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// EndCharacterAssignment ends the user's active assignment in the game and returns it, or nil if there was none.
func (s *SQLiteStore) EndCharacterAssignment(campaignID, userID, at string) (*models.CharacterAssignment, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	a, err := scanAssignment(tx.QueryRow(`SELECT `+assignmentColumns+` FROM `+assignmentFrom+` WHERE a.campaign_id = ? AND a.user_id = ? AND a.ended_at = ''`, campaignID, userID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := endUserAssignments(tx, campaignID, userID, at); err != nil {
		return nil, err
	}
	a.EndedAt = at
	return &a, tx.Commit()
}

func (s *SQLiteStore) CountCharactersBySystem(systemID string) (int, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM characters WHERE system_id = ?`, systemID).Scan(&n)
	return n, err
}
