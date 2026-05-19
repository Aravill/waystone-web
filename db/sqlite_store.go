package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"waystone-web/models"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db    *sql.DB
	mutex sync.RWMutex
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Open SQLite database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	store := &SQLiteStore{db: db}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		google_id TEXT,
		email TEXT,
		name TEXT,
		nickname TEXT,
		picture TEXT,
		roles TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS campaigns (
		id TEXT PRIMARY KEY,
		title TEXT,
		status TEXT,
		summary TEXT,
		description TEXT,
		players TEXT NOT NULL DEFAULT '[]',
		dm TEXT,
		desired_player_count TEXT,
		sign_ups_open INTEGER DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS campaign_sessions (
		id TEXT PRIMARY KEY,
		campaign_id TEXT NOT NULL,
		date TEXT NOT NULL,
		time TEXT NOT NULL,
		duration INTEGER NOT NULL,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS session_responses (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		participation TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(session_id, user_id),
		FOREIGN KEY (session_id) REFERENCES campaign_sessions(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	CREATE INDEX IF NOT EXISTS idx_users_google_id ON users(google_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_unique_nonempty ON users(email) WHERE email != '';
	CREATE UNIQUE INDEX IF NOT EXISTS idx_users_google_id_unique_nonempty ON users(google_id) WHERE google_id != '';
	CREATE INDEX IF NOT EXISTS idx_campaign_sessions_campaign_id ON campaign_sessions(campaign_id);
	CREATE INDEX IF NOT EXISTS idx_campaign_sessions_date ON campaign_sessions(date);
	CREATE INDEX IF NOT EXISTS idx_session_responses_session_id ON session_responses(session_id);
	CREATE INDEX IF NOT EXISTS idx_session_responses_user_id ON session_responses(user_id);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	return nil
}

func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *SQLiteStore) GetAllCampaigns() ([]models.Campaign, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query("SELECT id, title, status, summary, description, players, dm, desired_player_count, sign_ups_open FROM campaigns ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var campaigns []models.Campaign
	for rows.Next() {
		var campaign models.Campaign
		var playersJSON string
		var signUpsOpen int

		err := rows.Scan(
			&campaign.ID,
			&campaign.Title,
			&campaign.Status,
			&campaign.Summary,
			&campaign.Description,
			&playersJSON,
			&campaign.DM,
			&campaign.DesiredPlayerCount,
			&signUpsOpen,
		)
		if err != nil {
			return nil, err
		}

		// Unmarshal players JSON
		if err := json.Unmarshal([]byte(playersJSON), &campaign.Players); err != nil {
			campaign.Players = []string{}
		}

		campaign.SignUpsOpen = signUpsOpen != 0

		campaigns = append(campaigns, campaign)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return campaigns, nil
}

func (s *SQLiteStore) SaveCampaign(campaign models.Campaign) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	playersJSON, err := json.Marshal(campaign.Players)
	if err != nil {
		return err
	}

	signUpsOpen := 0
	if campaign.SignUpsOpen {
		signUpsOpen = 1
	}

	query := `
	INSERT INTO campaigns (id, title, status, summary, description, players, dm, desired_player_count, sign_ups_open)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		title = excluded.title,
		status = excluded.status,
		summary = excluded.summary,
		description = excluded.description,
		players = excluded.players,
		dm = excluded.dm,
		desired_player_count = excluded.desired_player_count,
		sign_ups_open = excluded.sign_ups_open
	`

	_, err = s.db.Exec(
		query,
		campaign.ID,
		campaign.Title,
		campaign.Status,
		campaign.Summary,
		campaign.Description,
		string(playersJSON),
		campaign.DM,
		campaign.DesiredPlayerCount,
		signUpsOpen,
	)

	return err
}

func (s *SQLiteStore) GetCampaignByID(id string) (*models.Campaign, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if id == "" {
		return nil, nil
	}

	var campaign models.Campaign
	var playersJSON string
	var signUpsOpen int

	query := `SELECT id, title, status, summary, description, players, dm, desired_player_count, sign_ups_open FROM campaigns WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&campaign.ID,
		&campaign.Title,
		&campaign.Status,
		&campaign.Summary,
		&campaign.Description,
		&playersJSON,
		&campaign.DM,
		&campaign.DesiredPlayerCount,
		&signUpsOpen,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Unmarshal players JSON
	if err := json.Unmarshal([]byte(playersJSON), &campaign.Players); err != nil {
		campaign.Players = []string{}
	}

	campaign.SignUpsOpen = signUpsOpen != 0

	return &campaign, nil
}

func (s *SQLiteStore) SaveUser(user models.User) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	rolesJSON, err := json.Marshal(user.Roles)
	if err != nil {
		return err
	}

	query := `
	INSERT INTO users (id, google_id, email, name, nickname, picture, roles, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		google_id = excluded.google_id,
		email = excluded.email,
		name = excluded.name,
		nickname = excluded.nickname,
		picture = excluded.picture,
		roles = excluded.roles,
		updated_at = excluded.updated_at
	`

	_, err = s.db.Exec(
		query,
		user.ID,
		user.GoogleID,
		user.Email,
		user.Name,
		user.Nickname,
		user.Picture,
		string(rolesJSON),
		user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		user.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	)

	return err
}

func (s *SQLiteStore) GetUserByGoogleID(googleID string) (*models.User, error) {
	if googleID == "" {
		return nil, nil
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var user models.User
	var rolesJSON string
	var createdAtStr, updatedAtStr string

	query := `SELECT id, google_id, email, name, nickname, picture, roles, created_at, updated_at FROM users WHERE google_id = ?`
	err := s.db.QueryRow(query, googleID).Scan(
		&user.ID,
		&user.GoogleID,
		&user.Email,
		&user.Name,
		&user.Nickname,
		&user.Picture,
		&rolesJSON,
		&createdAtStr,
		&updatedAtStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
		user.Roles = []string{}
	}

	// Parse timestamps
	if createdAtStr != "" {
		if t, err := parseTime(createdAtStr); err == nil {
			user.CreatedAt = t
		}
	}
	if updatedAtStr != "" {
		if t, err := parseTime(updatedAtStr); err == nil {
			user.UpdatedAt = t
		}
	}

	return &user, nil
}

func (s *SQLiteStore) GetAllUsers() ([]models.User, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query("SELECT id, google_id, email, name, nickname, picture, roles, created_at, updated_at FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		var rolesJSON string
		var createdAtStr, updatedAtStr string

		err := rows.Scan(
			&user.ID,
			&user.GoogleID,
			&user.Email,
			&user.Name,
			&user.Nickname,
			&user.Picture,
			&rolesJSON,
			&createdAtStr,
			&updatedAtStr,
		)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
			user.Roles = []string{}
		}

		// Parse timestamps
		if createdAtStr != "" {
			if t, err := parseTime(createdAtStr); err == nil {
				user.CreatedAt = t
			}
		}
		if updatedAtStr != "" {
			if t, err := parseTime(updatedAtStr); err == nil {
				user.UpdatedAt = t
			}
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func (s *SQLiteStore) GetUserByEmail(email string) (*models.User, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if email == "" {
		return nil, nil
	}

	var user models.User
	var rolesJSON string
	var createdAtStr, updatedAtStr string

	query := `SELECT id, google_id, email, name, nickname, picture, roles, created_at, updated_at FROM users WHERE email = ?`
	err := s.db.QueryRow(query, email).Scan(
		&user.ID,
		&user.GoogleID,
		&user.Email,
		&user.Name,
		&user.Nickname,
		&user.Picture,
		&rolesJSON,
		&createdAtStr,
		&updatedAtStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
		user.Roles = []string{}
	}

	// Parse timestamps
	if createdAtStr != "" {
		if t, err := parseTime(createdAtStr); err == nil {
			user.CreatedAt = t
		}
	}
	if updatedAtStr != "" {
		if t, err := parseTime(updatedAtStr); err == nil {
			user.UpdatedAt = t
		}
	}

	return &user, nil
}

func (s *SQLiteStore) GetUserByID(id string) (*models.User, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if id == "" {
		return nil, nil
	}

	var user models.User
	var rolesJSON string
	var createdAtStr, updatedAtStr string

	query := `SELECT id, google_id, email, name, nickname, picture, roles, created_at, updated_at FROM users WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&user.ID,
		&user.GoogleID,
		&user.Email,
		&user.Name,
		&user.Nickname,
		&user.Picture,
		&rolesJSON,
		&createdAtStr,
		&updatedAtStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
		user.Roles = []string{}
	}

	// Parse timestamps
	if createdAtStr != "" {
		if t, err := parseTime(createdAtStr); err == nil {
			user.CreatedAt = t
		}
	}
	if updatedAtStr != "" {
		if t, err := parseTime(updatedAtStr); err == nil {
			user.UpdatedAt = t
		}
	}

	return &user, nil
}

func (s *SQLiteStore) DeleteUser(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if id == "" {
		return fmt.Errorf("user id cannot be empty")
	}

	// Keep session response data consistent when users are removed.
	if _, err := s.db.Exec(`DELETE FROM session_responses WHERE user_id = ?`, id); err != nil {
		return err
	}

	query := `DELETE FROM users WHERE id = ?`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

func parseTime(timeStr string) (time.Time, error) {
	// Try RFC3339 format first (includes timezone)
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t, nil
	}

	// Try RFC3339Nano format
	if t, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
		return t, nil
	}

	// Try basic format
	return time.Parse("2006-01-02T15:04:05Z07:00", timeStr)
}

func (s *SQLiteStore) GetSessionsByMonth(campaignID string, year int, month int) ([]models.CampaignSession, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Format month range: YYYY-MM-01 to YYYY-MM-31
	startDate := fmt.Sprintf("%04d-%02d-01", year, month)
	nextMonth := month + 1
	nextYear := year
	if nextMonth > 12 {
		nextMonth = 1
		nextYear++
	}
	endDate := fmt.Sprintf("%04d-%02d-01", nextYear, nextMonth)

	query := `
		SELECT id, campaign_id, date, time, duration, status, created_at, updated_at
		FROM campaign_sessions
		WHERE campaign_id = ? AND date >= ? AND date < ?
		ORDER BY date ASC, time ASC
	`

	rows, err := s.db.Query(query, campaignID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.CampaignSession
	for rows.Next() {
		var session models.CampaignSession
		if err := rows.Scan(
			&session.ID,
			&session.CampaignID,
			&session.Date,
			&session.Time,
			&session.Duration,
			&session.Status,
			&session.CreatedAt,
			&session.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

func (s *SQLiteStore) SaveSession(session models.CampaignSession) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if session.ID == "" {
		return fmt.Errorf("session id cannot be empty")
	}
	if session.CampaignID == "" {
		return fmt.Errorf("campaign id cannot be empty")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if session.CreatedAt == "" {
		session.CreatedAt = now
	}
	session.UpdatedAt = now

	query := `
		INSERT INTO campaign_sessions (id, campaign_id, date, time, duration, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			date = excluded.date,
			time = excluded.time,
			duration = excluded.duration,
			status = excluded.status,
			updated_at = excluded.updated_at
	`

	_, err := s.db.Exec(query, session.ID, session.CampaignID, session.Date, session.Time, session.Duration, session.Status, session.CreatedAt, session.UpdatedAt)
	return err
}

func (s *SQLiteStore) GetSessionByID(sessionID string) (*models.CampaignSession, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if sessionID == "" {
		return nil, nil
	}

	query := `
		SELECT id, campaign_id, date, time, duration, status, created_at, updated_at
		FROM campaign_sessions
		WHERE id = ?
	`

	var session models.CampaignSession
	err := s.db.QueryRow(query, sessionID).Scan(
		&session.ID,
		&session.CampaignID,
		&session.Date,
		&session.Time,
		&session.Duration,
		&session.Status,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &session, nil
}

func (s *SQLiteStore) UpsertSessionResponse(response models.SessionResponse) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if response.ID == "" {
		return fmt.Errorf("response id cannot be empty")
	}
	if response.SessionID == "" {
		return fmt.Errorf("session id cannot be empty")
	}
	if response.UserID == "" {
		return fmt.Errorf("user id cannot be empty")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if response.CreatedAt == "" {
		response.CreatedAt = now
	}
	response.UpdatedAt = now

	query := `
		INSERT INTO session_responses (id, session_id, user_id, participation, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, user_id) DO UPDATE SET
			participation = excluded.participation,
			updated_at = excluded.updated_at
	`

	_, err := s.db.Exec(query, response.ID, response.SessionID, response.UserID, response.Participation, response.CreatedAt, response.UpdatedAt)
	return err
}

func (s *SQLiteStore) GetSessionResponses(sessionID string) ([]models.SessionResponse, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	query := `
		SELECT id, session_id, user_id, participation, created_at, updated_at
		FROM session_responses
		WHERE session_id = ?
		ORDER BY created_at ASC
	`

	rows, err := s.db.Query(query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var responses []models.SessionResponse
	for rows.Next() {
		var response models.SessionResponse
		if err := rows.Scan(
			&response.ID,
			&response.SessionID,
			&response.UserID,
			&response.Participation,
			&response.CreatedAt,
			&response.UpdatedAt,
		); err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}

	return responses, rows.Err()
}

func (s *SQLiteStore) DeleteSession(sessionID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if sessionID == "" {
		return fmt.Errorf("session id cannot be empty")
	}

	// Check if session exists and is in Suggested status
	var status string
	query := `SELECT status FROM campaign_sessions WHERE id = ?`
	err := s.db.QueryRow(query, sessionID).Scan(&status)
	if err != nil {
		return fmt.Errorf("session not found")
	}

	if status != string(models.SessionSuggested) {
		return fmt.Errorf("only suggested sessions can be deleted")
	}

	// Delete the session (cascade will handle session_responses)
	deleteQuery := `DELETE FROM campaign_sessions WHERE id = ?`
	result, err := s.db.Exec(deleteQuery, sessionID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("session not found")
	}

	return nil
}
