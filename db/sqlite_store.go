package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
		username TEXT NOT NULL COLLATE NOCASE UNIQUE,
		password_hash TEXT NOT NULL DEFAULT '',
		must_change_password INTEGER NOT NULL DEFAULT 0,
		needs_profile_setup INTEGER NOT NULL DEFAULT 0,
		name TEXT,
		nickname TEXT,
		roles TEXT NOT NULL DEFAULT '[]',
		blocked INTEGER NOT NULL DEFAULT 0,
		deleted INTEGER NOT NULL DEFAULT 0,
		avatar BLOB,
		avatar_type TEXT NOT NULL DEFAULT '',
		avatar_version INTEGER NOT NULL DEFAULT 0,
		session_version INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS campaigns (
		id TEXT PRIMARY KEY,
		title TEXT,
		status TEXT,
		type TEXT NOT NULL DEFAULT 'Campaign',
		system TEXT NOT NULL DEFAULT '',
		summary TEXT,
		description TEXT,
		players TEXT NOT NULL DEFAULT '[]',
		dm TEXT,
		desired_player_count TEXT,
		sign_ups_open INTEGER DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS systems (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL COLLATE NOCASE UNIQUE,
		logo BLOB,
		logo_type TEXT NOT NULL DEFAULT '',
		logo_version INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS campaign_sessions (
		id TEXT PRIMARY KEY,
		campaign_id TEXT NOT NULL,
		date TEXT NOT NULL,
		time TEXT NOT NULL,
		duration INTEGER NOT NULL,
		status TEXT NOT NULL,
		timezone TEXT NOT NULL DEFAULT '',
		manual INTEGER NOT NULL DEFAULT 0,
		guests TEXT NOT NULL DEFAULT '[]',
		dm_absent INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS calendar_tokens (
		user_id TEXT PRIMARY KEY,
		token TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	DROP TABLE IF EXISTS guest_tokens;

	CREATE TABLE IF NOT EXISTS password_invites (
		user_id TEXT PRIMARY KEY,
		token_hash TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL
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

	CREATE TABLE IF NOT EXISTS campaign_signups (
		id TEXT PRIMARY KEY,
		campaign_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(campaign_id, user_id),
		FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS campaign_follows (
		campaign_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (campaign_id, user_id)
	);

	CREATE TABLE IF NOT EXISTS notifications (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		type TEXT NOT NULL,
		campaign_id TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		actor_id TEXT NOT NULL DEFAULT '',
		data TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL,
		read_at TEXT NOT NULL DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS journal_entries (
		id TEXT PRIMARY KEY,
		campaign_id TEXT NOT NULL,
		session_id TEXT NOT NULL UNIQUE,
		author_id TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
		FOREIGN KEY (session_id) REFERENCES campaign_sessions(id) ON DELETE CASCADE,
		FOREIGN KEY (author_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS journal_revisions (
		entry_id TEXT NOT NULL,
		number INTEGER NOT NULL,
		author_id TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		body TEXT NOT NULL,
		note TEXT NOT NULL DEFAULT '',
		restored_from INTEGER NOT NULL DEFAULT 0,
		merged_with INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		PRIMARY KEY (entry_id, number),
		FOREIGN KEY (entry_id) REFERENCES journal_entries(id) ON DELETE CASCADE,
		FOREIGN KEY (author_id) REFERENCES users(id)
	);

	-- Credit outlives pruning: one row per person per entry.
	CREATE TABLE IF NOT EXISTS journal_contributors (
		entry_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		revisions INTEGER NOT NULL,
		first_revision INTEGER NOT NULL,
		PRIMARY KEY (entry_id, user_id),
		FOREIGN KEY (entry_id) REFERENCES journal_entries(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);

	CREATE INDEX IF NOT EXISTS idx_journal_entries_campaign_id ON journal_entries(campaign_id);
	CREATE INDEX IF NOT EXISTS idx_notifications_user_created ON notifications(user_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_notifications_campaign_id ON notifications(campaign_id);
	CREATE INDEX IF NOT EXISTS idx_campaign_follows_user_id ON campaign_follows(user_id);
	CREATE INDEX IF NOT EXISTS idx_campaign_signups_campaign_id ON campaign_signups(campaign_id);
	CREATE INDEX IF NOT EXISTS idx_campaign_signups_user_id ON campaign_signups(user_id);
	CREATE INDEX IF NOT EXISTS idx_campaign_sessions_campaign_id ON campaign_sessions(campaign_id);
	CREATE INDEX IF NOT EXISTS idx_campaign_sessions_date ON campaign_sessions(date);
	CREATE INDEX IF NOT EXISTS idx_session_responses_session_id ON session_responses(session_id);
	CREATE INDEX IF NOT EXISTS idx_session_responses_user_id ON session_responses(user_id);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Add columns introduced after the first release to older databases.
	rows, err := s.db.Query(`PRAGMA table_info(campaigns)`)
	if err != nil {
		return fmt.Errorf("failed to inspect campaigns table: %w", err)
	}
	cols := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("failed to inspect campaigns table: %w", err)
		}
		cols[name] = true
	}
	rows.Close()
	if !cols["type"] {
		if _, err := s.db.Exec(`ALTER TABLE campaigns ADD COLUMN type TEXT NOT NULL DEFAULT 'Campaign'`); err != nil {
			return fmt.Errorf("failed to add campaigns.type column: %w", err)
		}
	}
	if !cols["system"] {
		if _, err := s.db.Exec(`ALTER TABLE campaigns ADD COLUMN system TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("failed to add campaigns.system column: %w", err)
		}
	}

	// Add users columns introduced after the table was first created.
	rows, err = s.db.Query(`PRAGMA table_info(users)`)
	if err != nil {
		return fmt.Errorf("failed to inspect users table: %w", err)
	}
	userCols := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("failed to inspect users table: %w", err)
		}
		userCols[name] = true
	}
	rows.Close()
	for _, col := range []struct{ name, def string }{
		{"deleted", "INTEGER NOT NULL DEFAULT 0"},
		{"avatar", "BLOB"},
		{"avatar_type", "TEXT NOT NULL DEFAULT ''"},
		{"avatar_version", "INTEGER NOT NULL DEFAULT 0"},
		{"needs_profile_setup", "INTEGER NOT NULL DEFAULT 0"},
		{"session_version", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if !userCols[col.name] {
			if _, err := s.db.Exec(`ALTER TABLE users ADD COLUMN ` + col.name + ` ` + col.def); err != nil {
				return fmt.Errorf("failed to add users.%s column: %w", col.name, err)
			}
		}
	}

	// Add campaign_sessions columns introduced after the table was first created.
	rows, err = s.db.Query(`PRAGMA table_info(campaign_sessions)`)
	if err != nil {
		return fmt.Errorf("failed to inspect campaign_sessions table: %w", err)
	}
	sessionCols := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("failed to inspect campaign_sessions table: %w", err)
		}
		sessionCols[name] = true
	}
	rows.Close()
	for _, col := range []struct{ name, def string }{
		{"timezone", "TEXT NOT NULL DEFAULT ''"},
		{"manual", "INTEGER NOT NULL DEFAULT 0"},
		{"guests", "TEXT NOT NULL DEFAULT '[]'"},
		{"dm_absent", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if !sessionCols[col.name] {
			if _, err := s.db.Exec(`ALTER TABLE campaign_sessions ADD COLUMN ` + col.name + ` ` + col.def); err != nil {
				return fmt.Errorf("failed to add campaign_sessions.%s column: %w", col.name, err)
			}
		}
	}

	return nil
}

// Backup writes a consistent snapshot of the database to path (which must not exist).
func (s *SQLiteStore) Backup(path string) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	_, err := s.db.Exec("VACUUM INTO ?", path)
	return err
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

	rows, err := s.db.Query("SELECT id, title, status, type, system, summary, description, players, dm, desired_player_count, sign_ups_open FROM campaigns ORDER BY id")
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
			&campaign.Type,
			&campaign.System,
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
	INSERT INTO campaigns (id, title, status, type, system, summary, description, players, dm, desired_player_count, sign_ups_open)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		title = excluded.title,
		status = excluded.status,
		type = excluded.type,
		system = excluded.system,
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
		campaign.Type,
		campaign.System,
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

	query := `SELECT id, title, status, type, system, summary, description, players, dm, desired_player_count, sign_ups_open FROM campaigns WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&campaign.ID,
		&campaign.Title,
		&campaign.Status,
		&campaign.Type,
		&campaign.System,
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

	blockedInt := 0
	if user.Blocked {
		blockedInt = 1
	}

	mustChangeInt := 0
	if user.MustChangePassword {
		mustChangeInt = 1
	}

	needsSetupInt := 0
	if user.NeedsProfileSetup {
		needsSetupInt = 1
	}

	deletedInt := 0
	if user.Deleted {
		deletedInt = 1
	}

	query := `
	INSERT INTO users (id, username, password_hash, must_change_password, needs_profile_setup, name, nickname, roles, blocked, deleted, session_version, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		username = excluded.username,
		password_hash = excluded.password_hash,
		must_change_password = excluded.must_change_password,
		needs_profile_setup = excluded.needs_profile_setup,
		name = excluded.name,
		nickname = excluded.nickname,
		roles = excluded.roles,
		blocked = excluded.blocked,
		deleted = excluded.deleted,
		session_version = MAX(session_version, excluded.session_version),
		updated_at = excluded.updated_at
	`

	_, err = s.db.Exec(
		query,
		user.ID,
		user.Username,
		user.PasswordHash,
		mustChangeInt,
		needsSetupInt,
		user.Name,
		user.Nickname,
		string(rolesJSON),
		blockedInt,
		deletedInt,
		user.SessionVersion,
		user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		user.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	)

	return err
}

func (s *SQLiteStore) GetAllUsers() ([]models.User, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query("SELECT id, username, password_hash, must_change_password, needs_profile_setup, name, nickname, roles, blocked, deleted, avatar_type, avatar_version, session_version, created_at, updated_at FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		var rolesJSON string
		var blockedInt, mustChangeInt, needsSetupInt, deletedInt int
		var createdAtStr, updatedAtStr string

		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.PasswordHash,
			&mustChangeInt,
			&needsSetupInt,
			&user.Name,
			&user.Nickname,
			&rolesJSON,
			&blockedInt,
			&deletedInt,
			&user.AvatarType,
			&user.AvatarVersion,
			&user.SessionVersion,
			&createdAtStr,
			&updatedAtStr,
		)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
			user.Roles = []string{}
		}

		user.Blocked = blockedInt != 0
		user.Deleted = deletedInt != 0
		user.MustChangePassword = mustChangeInt != 0
		user.NeedsProfileSetup = needsSetupInt != 0

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

func (s *SQLiteStore) GetUserByUsername(username string) (*models.User, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if username == "" {
		return nil, nil
	}

	var user models.User
	var rolesJSON string
	var blockedInt, mustChangeInt, needsSetupInt, deletedInt int
	var createdAtStr, updatedAtStr string

	query := `SELECT id, username, password_hash, must_change_password, needs_profile_setup, name, nickname, roles, blocked, deleted, avatar_type, avatar_version, session_version, created_at, updated_at FROM users WHERE username = ?`
	err := s.db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&mustChangeInt,
		&needsSetupInt,
		&user.Name,
		&user.Nickname,
		&rolesJSON,
		&blockedInt,
		&deletedInt,
		&user.AvatarType,
		&user.AvatarVersion,
		&user.SessionVersion,
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

	user.Blocked = blockedInt != 0
	user.Deleted = deletedInt != 0
	user.MustChangePassword = mustChangeInt != 0
	user.NeedsProfileSetup = needsSetupInt != 0

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
	var blockedInt, mustChangeInt, needsSetupInt, deletedInt int
	var createdAtStr, updatedAtStr string

	query := `SELECT id, username, password_hash, must_change_password, needs_profile_setup, name, nickname, roles, blocked, deleted, avatar_type, avatar_version, session_version, created_at, updated_at FROM users WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&mustChangeInt,
		&needsSetupInt,
		&user.Name,
		&user.Nickname,
		&rolesJSON,
		&blockedInt,
		&deletedInt,
		&user.AvatarType,
		&user.AvatarVersion,
		&user.SessionVersion,
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

	user.Blocked = blockedInt != 0
	user.Deleted = deletedInt != 0
	user.MustChangePassword = mustChangeInt != 0
	user.NeedsProfileSetup = needsSetupInt != 0

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

// SetUserProfile updates the provided name and/or nickname; nil fields are left unchanged.
func (s *SQLiteStore) SetUserProfile(id string, name, nickname *string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sets := []string{}
	args := []interface{}{}
	if name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *name)
	}
	if nickname != nil {
		sets = append(sets, "nickname = ?")
		args = append(args, *nickname)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().UTC().Format(time.RFC3339), id)
	res, err := s.db.Exec(`UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ? AND deleted = 0`, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// SetUserAvatar stores an avatar and bumps its version; empty data removes the avatar.
func (s *SQLiteStore) CompleteProfileSetup(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	res, err := s.db.Exec(`UPDATE users SET needs_profile_setup = 0, updated_at = ? WHERE id = ? AND deleted = 0`,
		time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *SQLiteStore) SetUserAvatar(id string, data []byte, contentType string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var avatar interface{}
	if len(data) > 0 {
		avatar = data
	} else {
		contentType = ""
	}
	res, err := s.db.Exec(`UPDATE users SET avatar = ?, avatar_type = ?, avatar_version = avatar_version + 1 WHERE id = ? AND deleted = 0`, avatar, contentType, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// GetUserAvatar returns the avatar bytes and content type; empty data means no avatar.
func (s *SQLiteStore) GetUserAvatar(id string) ([]byte, string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var data []byte
	var contentType string
	err := s.db.QueryRow(`SELECT avatar, avatar_type FROM users WHERE id = ? AND deleted = 0`, id).Scan(&data, &contentType)
	if err == sql.ErrNoRows {
		return nil, "", ErrUserNotFound
	}
	return data, contentType, err
}

// AnonymizeUser erases a user's personal data and per-user rows but keeps the user row
// (marked deleted) so session responses and DM references stay intact.
func (s *SQLiteStore) AnonymizeUser(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if id == "" {
		return fmt.Errorf("user id cannot be empty")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE users SET deleted = 1, username = ?, name = '', nickname = '', password_hash = '',
		must_change_password = 0, needs_profile_setup = 0, roles = '["user"]', blocked = 0, avatar = NULL, avatar_type = '',
		avatar_version = avatar_version + 1, updated_at = ? WHERE id = ? AND deleted = 0`,
		"deleted-"+id, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUserNotFound
	}

	for _, table := range []string{"campaign_signups", "campaign_follows", "calendar_tokens", "password_invites", "notifications"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE user_id = ?`, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) SetUserBlocked(id string, blocked bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	blockedInt := 0
	if blocked {
		blockedInt = 1
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Blocking bumps session_version so unblocking doesn't revive cookies issued before the block.
	if _, err := tx.Exec(
		`UPDATE users SET blocked=?, session_version=session_version+?, updated_at=? WHERE id=?`,
		blockedInt, blockedInt, time.Now().UTC().Format(time.RFC3339), id,
	); err != nil {
		return err
	}
	// A pending invite dies with the block and stays dead after unblocking.
	if blocked {
		if _, err := tx.Exec(`DELETE FROM password_invites WHERE user_id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateUserRoles writes only the roles, so it can't undo a concurrent password or block change.
func (s *SQLiteStore) UpdateUserRoles(id string, roles []string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE users SET roles=?, updated_at=? WHERE id=? AND deleted=0`,
		string(rolesJSON), time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *SQLiteStore) SetUserPassword(id, hash string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Bumping session_version revokes every existing session cookie for the user.
	if _, err := tx.Exec(
		`UPDATE users SET password_hash=?, must_change_password=0, session_version=session_version+1, updated_at=? WHERE id=?`,
		hash, time.Now().UTC().Format(time.RFC3339), id,
	); err != nil {
		return err
	}
	// Any password change ends a pending invite.
	if _, err := tx.Exec(`DELETE FROM password_invites WHERE user_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
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
		SELECT id, campaign_id, date, time, duration, status, timezone, manual, guests, dm_absent, created_at, updated_at
		FROM campaign_sessions
		WHERE campaign_id = ? AND manual = 0 AND date >= ? AND date < ?
		ORDER BY date ASC, time ASC
	`

	rows, err := s.db.Query(query, campaignID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.CampaignSession
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
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

	_, err := s.db.Exec(upsertSessionSQL, sessionArgs(session)...)
	return err
}

const upsertSessionSQL = `
	INSERT INTO campaign_sessions (id, campaign_id, date, time, duration, status, timezone, manual, guests, dm_absent, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		date = excluded.date,
		time = excluded.time,
		duration = excluded.duration,
		status = excluded.status,
		timezone = excluded.timezone,
		manual = excluded.manual,
		guests = excluded.guests,
		dm_absent = excluded.dm_absent,
		updated_at = excluded.updated_at
`

func sessionArgs(session models.CampaignSession) []interface{} {
	guests := session.Guests
	if guests == nil {
		guests = []string{}
	}
	guestsJSON, _ := json.Marshal(guests)
	return []interface{}{session.ID, session.CampaignID, session.Date, session.Time, session.Duration, session.Status,
		session.Timezone, session.Manual, string(guestsJSON), session.DMAbsent, session.CreatedAt, session.UpdatedAt}
}

// scanSession reads one campaign_sessions row selected in the standard column order.
func scanSession(row interface {
	Scan(dest ...interface{}) error
}) (models.CampaignSession, error) {
	var session models.CampaignSession
	var manual, dmAbsent int
	var guests string
	if err := row.Scan(&session.ID, &session.CampaignID, &session.Date, &session.Time, &session.Duration,
		&session.Status, &session.Timezone, &manual, &guests, &dmAbsent, &session.CreatedAt, &session.UpdatedAt); err != nil {
		return session, err
	}
	session.Manual, session.DMAbsent = manual != 0, dmAbsent != 0
	if guests != "" && guests != "[]" {
		_ = json.Unmarshal([]byte(guests), &session.Guests)
	}
	return session, nil
}

// SaveManualSession writes a by-hand session and replaces its Accepted response rows in one transaction.
func (s *SQLiteStore) SaveManualSession(session models.CampaignSession, attendeeIDs []string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if session.ID == "" || session.CampaignID == "" {
		return fmt.Errorf("session id and campaign id cannot be empty")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if session.CreatedAt == "" {
		session.CreatedAt = now
	}
	session.UpdatedAt = now

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(upsertSessionSQL, sessionArgs(session)...); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM session_responses WHERE session_id = ?`, session.ID); err != nil {
		return err
	}
	for _, userID := range attendeeIDs {
		if _, err := tx.Exec(`INSERT INTO session_responses (id, session_id, user_id, participation, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			GenerateUUID(), session.ID, userID, models.Accepted, now, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteManualSession removes a by-hand session and its responses. Foreign keys are off, so nothing cascades.
func (s *SQLiteStore) DeleteManualSession(sessionID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM session_responses WHERE session_id = ? AND EXISTS (SELECT 1 FROM campaign_sessions WHERE id = ? AND manual = 1)`, sessionID, sessionID); err != nil {
		return err
	}
	// A by-hand session that is deleted didn't happen, so its journal entry goes too.
	if err := deleteJournalForSessions(tx, `SELECT id FROM campaign_sessions WHERE id = ? AND manual = 1`, sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM campaign_sessions WHERE id = ? AND manual = 1`, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) GetSessionsByCampaignIDs(campaignIDs []string, fromDate string) ([]models.CampaignSession, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	args := make([]interface{}, 0, len(campaignIDs)+1)
	for _, id := range campaignIDs {
		if id != "" {
			args = append(args, id)
		}
	}
	if len(args) == 0 {
		return nil, nil
	}
	args = append(args, fromDate)

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(args)-1), ",")
	query := fmt.Sprintf(`
		SELECT id, campaign_id, date, time, duration, status, timezone, manual, guests, dm_absent, created_at, updated_at
		FROM campaign_sessions
		WHERE campaign_id IN (%s) AND date >= ?
		ORDER BY date ASC, time ASC, id ASC
	`, placeholders)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.CampaignSession
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

// GetUnannouncedConfirmedSessions returns confirmed, non-manual sessions dated within the range
// that have no session_started notification yet: the notifications table is the dedupe.
func (s *SQLiteStore) GetUnannouncedConfirmedSessions(fromDate, toDate string) ([]models.CampaignSession, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, campaign_id, date, time, duration, status, timezone, manual, guests, dm_absent, created_at, updated_at
		FROM campaign_sessions
		WHERE status = ? AND manual = 0 AND date >= ? AND date <= ?
		  AND NOT EXISTS (SELECT 1 FROM notifications n WHERE n.type = 'session_started' AND n.session_id = campaign_sessions.id)
		ORDER BY date ASC, time ASC, id ASC
	`, models.SessionConfirmed, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.CampaignSession
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

// GetAcceptedConfirmedSessions returns confirmed sessions the user responded Accepted to.
func (s *SQLiteStore) GetAcceptedConfirmedSessions(userID string) ([]models.CampaignSession, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query(`
		SELECT s.id, s.campaign_id, s.date, s.time, s.duration, s.status, s.timezone, s.manual, s.guests, s.dm_absent, s.created_at, s.updated_at
		FROM campaign_sessions s JOIN session_responses r ON r.session_id = s.id
		WHERE r.user_id = ? AND r.participation = ? AND s.status = ?
	`, userID, models.Accepted, models.SessionConfirmed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.CampaignSession
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *SQLiteStore) GetCalendarToken(userID string) (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var token string
	err := s.db.QueryRow(`SELECT token FROM calendar_tokens WHERE user_id = ?`, userID).Scan(&token)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return token, err
}

func (s *SQLiteStore) SetCalendarToken(userID, token string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if userID == "" || token == "" {
		return fmt.Errorf("user id and token cannot be empty")
	}
	_, err := s.db.Exec(`
		INSERT INTO calendar_tokens (user_id, token, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET token = excluded.token, created_at = excluded.created_at
	`, userID, token, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) GetUserIDByCalendarToken(token string) (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if token == "" {
		return "", nil
	}
	var userID string
	err := s.db.QueryRow(`SELECT user_id FROM calendar_tokens WHERE token = ?`, token).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return userID, err
}

func (s *SQLiteStore) GetSessionByID(sessionID string) (*models.CampaignSession, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if sessionID == "" {
		return nil, nil
	}

	query := `
		SELECT id, campaign_id, date, time, duration, status, timezone, manual, guests, dm_absent, created_at, updated_at
		FROM campaign_sessions
		WHERE id = ?
	`

	session, err := scanSession(s.db.QueryRow(query, sessionID))
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

func (s *SQLiteStore) GetSessionResponsesBySessionIDs(sessionIDs []string) (map[string][]models.SessionResponse, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	responsesBySession := make(map[string][]models.SessionResponse, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return responsesBySession, nil
	}

	args := make([]interface{}, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if sessionID == "" {
			continue
		}
		responsesBySession[sessionID] = []models.SessionResponse{}
		args = append(args, sessionID)
	}
	if len(args) == 0 {
		return responsesBySession, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
	query := fmt.Sprintf(`
		SELECT id, session_id, user_id, participation, created_at, updated_at
		FROM session_responses
		WHERE session_id IN (%s)
		ORDER BY created_at ASC
	`, placeholders)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
		responsesBySession[response.SessionID] = append(responsesBySession[response.SessionID], response)
	}

	return responsesBySession, rows.Err()
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
		if err == sql.ErrNoRows {
			return fmt.Errorf("session not found")
		}
		return err
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

var (
	ErrSignupNotFound       = errors.New("sign-up not found")
	ErrCampaignFull         = errors.New("game is full")
	ErrCampaignNotFound     = errors.New("game not found")
	ErrSystemNotFound       = errors.New("system not found")
	ErrUserNotFound         = errors.New("user not found")
	ErrNotificationNotFound = errors.New("notification not found")
	ErrJournalSessionGone   = errors.New("session not found")
	ErrJournalNotFound      = errors.New("journal entry not found")
)

func (s *SQLiteStore) CreateCampaignSignup(signup models.CampaignSignup) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	if signup.CreatedAt == "" {
		signup.CreatedAt = now
	}
	signup.UpdatedAt = now

	_, err := s.db.Exec(
		`INSERT INTO campaign_signups (id, campaign_id, user_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		signup.ID, signup.CampaignID, signup.UserID, signup.Status, signup.CreatedAt, signup.UpdatedAt,
	)
	return err
}

func (s *SQLiteStore) GetCampaignSignup(campaignID, userID string) (*models.CampaignSignup, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var signup models.CampaignSignup
	err := s.db.QueryRow(
		`SELECT id, campaign_id, user_id, status, created_at, updated_at FROM campaign_signups WHERE campaign_id = ? AND user_id = ?`,
		campaignID, userID,
	).Scan(&signup.ID, &signup.CampaignID, &signup.UserID, &signup.Status, &signup.CreatedAt, &signup.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &signup, nil
}

func (s *SQLiteStore) GetCampaignSignups(campaignID string) ([]models.CampaignSignup, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query(
		`SELECT id, campaign_id, user_id, status, created_at, updated_at FROM campaign_signups WHERE campaign_id = ? ORDER BY created_at ASC`,
		campaignID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	signups := []models.CampaignSignup{}
	for rows.Next() {
		var signup models.CampaignSignup
		if err := rows.Scan(&signup.ID, &signup.CampaignID, &signup.UserID, &signup.Status, &signup.CreatedAt, &signup.UpdatedAt); err != nil {
			return nil, err
		}
		signups = append(signups, signup)
	}
	return signups, rows.Err()
}

func (s *SQLiteStore) FollowCampaign(campaignID, userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO campaign_follows (campaign_id, user_id, created_at) VALUES (?, ?, ?)`,
		campaignID, userID, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (s *SQLiteStore) UnfollowCampaign(campaignID, userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, err := s.db.Exec(`DELETE FROM campaign_follows WHERE campaign_id = ? AND user_id = ?`, campaignID, userID)
	return err
}

func (s *SQLiteStore) GetCampaignFollowerIDs(campaignID string) ([]string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query(`SELECT user_id FROM campaign_follows WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *SQLiteStore) GetFollowedCampaignIDs(userID string) (map[string]bool, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query(`SELECT campaign_id FROM campaign_follows WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	followed := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		followed[id] = true
	}
	return followed, rows.Err()
}

func (s *SQLiteStore) GetCampaignSignupsByUser(userID string) (map[string]models.CampaignSignup, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query(
		`SELECT id, campaign_id, user_id, status, created_at, updated_at FROM campaign_signups WHERE user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	signups := map[string]models.CampaignSignup{}
	for rows.Next() {
		var signup models.CampaignSignup
		if err := rows.Scan(&signup.ID, &signup.CampaignID, &signup.UserID, &signup.Status, &signup.CreatedAt, &signup.UpdatedAt); err != nil {
			return nil, err
		}
		signups[signup.CampaignID] = signup
	}
	return signups, rows.Err()
}

// CountPendingSignupsForDM returns pending sign-up counts per campaign run by the given DM.
func (s *SQLiteStore) CountPendingSignupsForDM(dmID string) (map[string]int, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query(
		`SELECT campaign_id, COUNT(*) FROM campaign_signups
		WHERE status = ? AND campaign_id IN (SELECT id FROM campaigns WHERE dm = ?)
		GROUP BY campaign_id`,
		models.SignupPending, dmID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var campaignID string
		var n int
		if err := rows.Scan(&campaignID, &n); err != nil {
			return nil, err
		}
		counts[campaignID] = n
	}
	return counts, rows.Err()
}

// GetUnansweredSessions returns, per campaign, upcoming suggested sessions the user has not responded to.
func (s *SQLiteStore) GetUnansweredSessions(userID, fromDate string) (map[string]models.UnansweredSessions, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query(
		`SELECT s.campaign_id, COUNT(*), MIN(s.date) FROM campaign_sessions s
		WHERE s.status = ? AND s.date >= ?
		AND NOT EXISTS (SELECT 1 FROM session_responses r WHERE r.session_id = s.id AND r.user_id = ?)
		GROUP BY s.campaign_id`,
		models.SessionSuggested, fromDate, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]models.UnansweredSessions{}
	for rows.Next() {
		var campaignID string
		var u models.UnansweredSessions
		if err := rows.Scan(&campaignID, &u.Count, &u.FirstDate); err != nil {
			return nil, err
		}
		result[campaignID] = u
	}
	return result, rows.Err()
}

// SetCampaignSignUpsOpen manually opens or closes a campaign's sign-ups.
func (s *SQLiteStore) SetCampaignSignUpsOpen(campaignID string, open bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	res, err := s.db.Exec(`UPDATE campaigns SET sign_ups_open = ? WHERE id = ?`, open, campaignID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCampaignNotFound
	}
	return nil
}

// UpdateCampaignDetails updates a campaign's editable fields: text, type, system and desired player count.
func (s *SQLiteStore) UpdateCampaignDetails(id, title, summary, description string, campaignType models.CampaignType, system, desiredPlayerCount string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	res, err := s.db.Exec(`UPDATE campaigns SET title = ?, summary = ?, description = ?, type = ?, system = ?, desired_player_count = ? WHERE id = ?`,
		title, summary, description, campaignType, system, desiredPlayerCount, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCampaignNotFound
	}
	return nil
}

// SetCampaignStatus changes a campaign's status.
func (s *SQLiteStore) SetCampaignStatus(id string, status models.CampaignStatus) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	res, err := s.db.Exec(`UPDATE campaigns SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCampaignNotFound
	}
	return nil
}

// DeleteCampaign removes a campaign with its sessions, session responses, sign-ups and notifications.
// The foreign_keys pragma is not enabled, so ON DELETE CASCADE does not fire and the
// cascade is done by hand.
func (s *SQLiteStore) DeleteCampaign(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM session_responses WHERE session_id IN (SELECT id FROM campaign_sessions WHERE campaign_id = ?)`, id); err != nil {
		return err
	}
	if err := deleteJournalForSessions(tx, `SELECT id FROM campaign_sessions WHERE campaign_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM campaign_sessions WHERE campaign_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM campaign_signups WHERE campaign_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM campaign_follows WHERE campaign_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notifications WHERE campaign_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM campaigns WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCampaignNotFound
	}
	return tx.Commit()
}

// SetCampaignSignupStatus changes a sign-up's status and keeps campaigns.players
// (and sign_ups_open when the campaign fills up) consistent in one transaction.
func (s *SQLiteStore) SetCampaignSignupStatus(campaignID, userID string, status models.SignupStatus) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var playersJSON string
	var desired sql.NullString
	var signUpsOpenInt int
	err = tx.QueryRow(`SELECT players, desired_player_count, sign_ups_open FROM campaigns WHERE id = ?`, campaignID).
		Scan(&playersJSON, &desired, &signUpsOpenInt)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrCampaignNotFound
		}
		return err
	}

	var players []string
	if err := json.Unmarshal([]byte(playersJSON), &players); err != nil {
		players = []string{}
	}

	var existingID string
	hasRow := true
	err = tx.QueryRow(`SELECT id FROM campaign_signups WHERE campaign_id = ? AND user_id = ?`, campaignID, userID).Scan(&existingID)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		hasRow = false
	}

	max, _ := strconv.Atoi(strings.TrimSpace(desired.String))
	isPlayer := false
	for _, p := range players {
		if p == userID {
			isPlayer = true
			break
		}
	}

	signUpsOpen := signUpsOpenInt != 0
	switch status {
	case models.SignupAccepted:
		if !hasRow && !isPlayer {
			return ErrSignupNotFound
		}
		if !isPlayer {
			if max > 0 && len(players) >= max {
				return ErrCampaignFull
			}
			players = append(players, userID)
		}
		if max > 0 && len(players) >= max {
			signUpsOpen = false
		}
	case models.SignupDeclined, models.SignupKicked:
		if !hasRow && !isPlayer {
			return ErrSignupNotFound
		}
		remaining := make([]string, 0, len(players))
		for _, p := range players {
			if p != userID {
				remaining = append(remaining, p)
			}
		}
		players = remaining
	default:
		return fmt.Errorf("invalid sign-up status: %s", status)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(
		`INSERT INTO campaign_signups (id, campaign_id, user_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(campaign_id, user_id) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at`,
		GenerateUUID(), campaignID, userID, status, now, now,
	); err != nil {
		return err
	}

	newPlayersJSON, err := json.Marshal(players)
	if err != nil {
		return err
	}
	open := 0
	if signUpsOpen {
		open = 1
	}
	if _, err := tx.Exec(`UPDATE campaigns SET players = ?, sign_ups_open = ? WHERE id = ?`, string(newPlayersJSON), open, campaignID); err != nil {
		return err
	}

	return tx.Commit()
}

// WithdrawCampaignSignup deletes the user's pending sign-up, or removes a player
// from the campaign (and deletes their sign-up row). Declined/Kicked rows are kept.
func (s *SQLiteStore) WithdrawCampaignSignup(campaignID, userID string) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var playersJSON string
	err = tx.QueryRow(`SELECT players FROM campaigns WHERE id = ?`, campaignID).Scan(&playersJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, ErrCampaignNotFound
		}
		return false, err
	}

	var players []string
	if err := json.Unmarshal([]byte(playersJSON), &players); err != nil {
		players = []string{}
	}
	isPlayer := false
	remaining := make([]string, 0, len(players))
	for _, p := range players {
		if p == userID {
			isPlayer = true
		} else {
			remaining = append(remaining, p)
		}
	}

	if !isPlayer {
		var status string
		err = tx.QueryRow(`SELECT status FROM campaign_signups WHERE campaign_id = ? AND user_id = ?`, campaignID, userID).Scan(&status)
		if err != nil {
			if err == sql.ErrNoRows {
				return false, ErrSignupNotFound
			}
			return false, err
		}
		if models.SignupStatus(status) != models.SignupPending {
			return false, ErrSignupNotFound
		}
	} else {
		newPlayersJSON, err := json.Marshal(remaining)
		if err != nil {
			return false, err
		}
		if _, err := tx.Exec(`UPDATE campaigns SET players = ? WHERE id = ?`, string(newPlayersJSON), campaignID); err != nil {
			return false, err
		}
	}

	if _, err := tx.Exec(`DELETE FROM campaign_signups WHERE campaign_id = ? AND user_id = ?`, campaignID, userID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return isPlayer, nil
}

// GetAllSystems returns all known systems (without logo data), sorted by name.
func (s *SQLiteStore) GetAllSystems() ([]models.System, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query(`SELECT id, name, logo_type, logo_version FROM systems ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	systems := []models.System{}
	for rows.Next() {
		var sys models.System
		if err := rows.Scan(&sys.ID, &sys.Name, &sys.LogoType, &sys.LogoVersion); err != nil {
			return nil, err
		}
		systems = append(systems, sys)
	}
	return systems, rows.Err()
}

func (s *SQLiteStore) CreateSystem(sys models.System) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, err := s.db.Exec(`INSERT INTO systems (id, name) VALUES (?, ?)`, sys.ID, sys.Name)
	return err
}

// RenameSystem renames a system and updates campaigns that use the old name.
func (s *SQLiteStore) RenameSystem(id, name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldName string
	if err := tx.QueryRow(`SELECT name FROM systems WHERE id = ?`, id).Scan(&oldName); err != nil {
		if err == sql.ErrNoRows {
			return ErrSystemNotFound
		}
		return err
	}
	if _, err := tx.Exec(`UPDATE systems SET name = ? WHERE id = ?`, name, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE campaigns SET system = ? WHERE system = ? COLLATE NOCASE`, name, oldName); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteSystem removes the system row; campaigns keep their system text.
func (s *SQLiteStore) DeleteSystem(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	res, err := s.db.Exec(`DELETE FROM systems WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrSystemNotFound
	}
	return nil
}

// SetSystemLogo stores a logo and bumps its version; empty data removes the logo.
func (s *SQLiteStore) SetSystemLogo(id string, data []byte, contentType string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var logo interface{}
	if len(data) > 0 {
		logo = data
	} else {
		contentType = ""
	}
	res, err := s.db.Exec(`UPDATE systems SET logo = ?, logo_type = ?, logo_version = logo_version + 1 WHERE id = ?`, logo, contentType, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrSystemNotFound
	}
	return nil
}

// GetSystemLogo returns the logo bytes and content type; empty data means no logo.
func (s *SQLiteStore) GetSystemLogo(id string) ([]byte, string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var data []byte
	var contentType string
	err := s.db.QueryRow(`SELECT logo, logo_type FROM systems WHERE id = ?`, id).Scan(&data, &contentType)
	if err == sql.ErrNoRows {
		return nil, "", ErrSystemNotFound
	}
	return data, contentType, err
}

// notificationRetention is how long read notifications are kept.
const notificationRetention = 90 * 24 * time.Hour

// CreateNotifications inserts the notifications in one transaction and prunes old read ones.
func (s *SQLiteStore) CreateNotifications(notifications []models.Notification) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO notifications (id, user_id, type, campaign_id, session_id, actor_id, data, created_at, read_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, n := range notifications {
		data, err := json.Marshal(n.Data)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(n.ID, n.UserID, n.Type, n.CampaignID, n.SessionID, n.ActorID, string(data), n.CreatedAt, n.ReadAt); err != nil {
			return err
		}
	}

	cutoff := time.Now().UTC().Add(-notificationRetention).Format(time.RFC3339)
	if _, err := tx.Exec(`DELETE FROM notifications WHERE read_at != '' AND read_at < ?`, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

// GetNotifications returns a user's newest notifications first.
func (s *SQLiteStore) GetNotifications(userID string, limit int) ([]models.Notification, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	rows, err := s.db.Query(`SELECT id, user_id, type, campaign_id, session_id, actor_id, data, created_at, read_at FROM notifications WHERE user_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notifications := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		var data string
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.CampaignID, &n.SessionID, &n.ActorID, &data, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &n.Data); err != nil || n.Data == nil {
			n.Data = map[string]string{}
		}
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

// CountUnreadNotifications returns how many notifications a user has not read.
func (s *SQLiteStore) CountUnreadNotifications(userID string) (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at = ''`, userID).Scan(&count)
	return count, err
}

// MarkNotificationRead marks one of the user's notifications read (idempotent).
func (s *SQLiteStore) MarkNotificationRead(userID, id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE notifications SET read_at = CASE WHEN read_at = '' THEN ? ELSE read_at END WHERE id = ? AND user_id = ?`, now, id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

// MarkAllNotificationsRead marks all of the user's unread notifications read.
func (s *SQLiteStore) MarkAllNotificationsRead(userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at = ''`, now, userID)
	return err
}

// IssuePasswordInvite clears the user's password (so the old one stops working
// at once) and upserts their single live invite.
func (s *SQLiteStore) IssuePasswordInvite(userID, tokenHash string, expiresAt time.Time) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if userID == "" || tokenHash == "" {
		return fmt.Errorf("user id and token hash cannot be empty")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(
		`UPDATE users SET password_hash='', must_change_password=1, session_version=session_version+1, updated_at=? WHERE id=?`,
		now, userID,
	)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrUserNotFound
	}
	if _, err := tx.Exec(`
		INSERT INTO password_invites (user_id, token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET token_hash = excluded.token_hash, created_at = excluded.created_at, expires_at = excluded.expires_at
	`, userID, tokenHash, now, expiresAt.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}

// GetPasswordInvite resolves an invite by token hash; userID is empty when unknown.
func (s *SQLiteStore) GetPasswordInvite(tokenHash string) (string, time.Time, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if tokenHash == "" {
		return "", time.Time{}, nil
	}
	var userID, expires string
	err := s.db.QueryRow(`SELECT user_id, expires_at FROM password_invites WHERE token_hash = ?`, tokenHash).Scan(&userID, &expires)
	if err == sql.ErrNoRows {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		return "", time.Time{}, err
	}
	return userID, expiresAt, nil
}

// DeletePasswordInvite removes the user's pending invite, if any.
func (s *SQLiteStore) DeletePasswordInvite(userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, err := s.db.Exec(`DELETE FROM password_invites WHERE user_id = ?`, userID)
	return err
}

// RedeemPasswordInvite sets the password and deletes the invite in one transaction.
func (s *SQLiteStore) RedeemPasswordInvite(tokenHash, passwordHash string) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if tokenHash == "" {
		return "", nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var userID, expires string
	var deleted, blocked int
	err = tx.QueryRow(`
		SELECT i.user_id, i.expires_at, u.deleted, u.blocked
		FROM password_invites i JOIN users u ON u.id = i.user_id
		WHERE i.token_hash = ?`, tokenHash).Scan(&userID, &expires, &deleted, &blocked)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	expiresAt, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		return "", err
	}
	if !expiresAt.After(time.Now()) || deleted != 0 || blocked != 0 {
		return "", nil
	}
	if _, err := tx.Exec(
		`UPDATE users SET password_hash=?, must_change_password=0, session_version=session_version+1, updated_at=? WHERE id=?`,
		passwordHash, time.Now().UTC().Format(time.RFC3339), userID,
	); err != nil {
		return "", err
	}
	if _, err := tx.Exec(`DELETE FROM password_invites WHERE user_id = ?`, userID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return userID, nil
}

// deleteJournalForSessions removes the journal entries (with revisions and contributors) of the
// sessions the query selects. Foreign keys are off, so nothing cascades.
func deleteJournalForSessions(tx *sql.Tx, sessionIDsQuery string, args ...interface{}) error {
	entries := `SELECT id FROM journal_entries WHERE session_id IN (` + sessionIDsQuery + `)`
	for _, table := range []string{"journal_contributors", "journal_revisions"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE entry_id IN (`+entries+`)`, args...); err != nil {
			return err
		}
	}
	_, err := tx.Exec(`DELETE FROM journal_entries WHERE session_id IN (`+sessionIDsQuery+`)`, args...)
	return err
}

const journalRevisionColumns = `number, author_id, name, body, note, restored_from, merged_with, created_at`

type rowScanner interface{ Scan(dest ...interface{}) error }

func scanJournalRevision(row rowScanner) (models.JournalRevision, error) {
	var r models.JournalRevision
	err := row.Scan(&r.Number, &r.AuthorID, &r.Name, &r.Body, &r.Note, &r.RestoredFrom, &r.MergedWith, &r.CreatedAt)
	return r, err
}

// GetJournalSummaries returns the newest-revision digest of every entry in a campaign, keyed by session id.
func (s *SQLiteStore) GetJournalSummaries(campaignID string) (map[string]models.JournalSummary, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query(`
		SELECT e.id, e.session_id, e.author_id, r.name, substr(r.body, 1, 600), r.created_at,
		       (SELECT COUNT(*) FROM journal_revisions WHERE entry_id = e.id)
		FROM journal_entries e
		JOIN journal_revisions r ON r.entry_id = e.id
		  AND r.number = (SELECT MAX(number) FROM journal_revisions WHERE entry_id = e.id)
		WHERE e.campaign_id = ?`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]models.JournalSummary{}
	byEntry := map[string]string{} // entry id -> session id
	for rows.Next() {
		var j models.JournalSummary
		if err := rows.Scan(&j.EntryID, &j.SessionID, &j.AuthorID, &j.Name, &j.ExcerptSource, &j.UpdatedAt, &j.Revisions); err != nil {
			return nil, err
		}
		j.AmenderIDs = []string{}
		result[j.SessionID] = j
		byEntry[j.EntryID] = j.SessionID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	people, err := s.db.Query(`
		SELECT c.entry_id, c.user_id
		FROM journal_contributors c JOIN journal_entries e ON e.id = c.entry_id
		WHERE e.campaign_id = ?
		ORDER BY c.entry_id, c.first_revision`, campaignID)
	if err != nil {
		return nil, err
	}
	defer people.Close()
	for people.Next() {
		var entryID, userID string
		if err := people.Scan(&entryID, &userID); err != nil {
			return nil, err
		}
		j := result[byEntry[entryID]]
		if userID != j.AuthorID {
			j.AmenderIDs = append(j.AmenderIDs, userID)
			result[j.SessionID] = j
		}
	}
	return result, people.Err()
}

// GetJournalEntry returns a session's entry with its newest revision and credits, or nil.
func (s *SQLiteStore) GetJournalEntry(sessionID string) (*models.JournalEntry, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var e models.JournalEntry
	err := s.db.QueryRow(`SELECT id, campaign_id, session_id, author_id, created_at FROM journal_entries WHERE session_id = ?`, sessionID).
		Scan(&e.ID, &e.CampaignID, &e.SessionID, &e.AuthorID, &e.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if e.Latest, err = scanJournalRevision(s.db.QueryRow(
		`SELECT `+journalRevisionColumns+` FROM journal_revisions WHERE entry_id = ? ORDER BY number DESC LIMIT 1`, e.ID)); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM journal_revisions WHERE entry_id = ?`, e.ID).Scan(&e.RevisionCount); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT user_id, revisions, first_revision FROM journal_contributors WHERE entry_id = ? ORDER BY first_revision`, e.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c models.JournalScribe
		if err := rows.Scan(&c.UserID, &c.Revisions, &c.FirstRevision); err != nil {
			return nil, err
		}
		e.Scribes = append(e.Scribes, c)
	}
	return &e, rows.Err()
}

// GetJournalRevisions returns a session's entry id and every kept revision, oldest first.
// With no entry the id is empty.
func (s *SQLiteStore) GetJournalRevisions(sessionID string) (string, []models.JournalRevision, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var entryID string
	err := s.db.QueryRow(`SELECT id FROM journal_entries WHERE session_id = ?`, sessionID).Scan(&entryID)
	if err == sql.ErrNoRows {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	rows, err := s.db.Query(`SELECT `+journalRevisionColumns+` FROM journal_revisions WHERE entry_id = ? ORDER BY number`, entryID)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var revs []models.JournalRevision
	for rows.Next() {
		r, err := scanJournalRevision(rows)
		if err != nil {
			return "", nil, err
		}
		revs = append(revs, r)
	}
	return entryID, revs, rows.Err()
}

// SaveJournalRevision adds a revision to a session's entry in one transaction, creating the
// entry when entryID and baseRevision are both empty. A save made against anything but the
// latest revision is refused so the caller can merge. Revisions older than the newest
// keepRevisions are pruned; credit is kept in journal_contributors.
func (s *SQLiteStore) SaveJournalRevision(campaignID, sessionID, entryID string, baseRevision int, rev models.JournalRevision, keepRevisions int) (models.JournalSaveResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return models.JournalSaveResult{}, err
	}
	defer tx.Rollback()

	var one int
	err = tx.QueryRow(`SELECT 1 FROM campaign_sessions WHERE id = ? AND campaign_id = ?`, sessionID, campaignID).Scan(&one)
	if err == sql.ErrNoRows {
		return models.JournalSaveResult{}, ErrJournalSessionGone
	}
	if err != nil {
		return models.JournalSaveResult{}, err
	}

	var current string
	var latest *models.JournalRevision
	err = tx.QueryRow(`SELECT id FROM journal_entries WHERE session_id = ?`, sessionID).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return models.JournalSaveResult{}, err
	}
	if err == nil {
		l, err := scanJournalRevision(tx.QueryRow(
			`SELECT `+journalRevisionColumns+` FROM journal_revisions WHERE entry_id = ? ORDER BY number DESC LIMIT 1`, current))
		if err != nil {
			return models.JournalSaveResult{}, err
		}
		latest = &l
	}

	now := time.Now().UTC().Format(time.RFC3339)
	switch {
	case latest == nil && (entryID != "" || baseRevision != 0):
		return models.JournalSaveResult{Outcome: models.JournalDeleted}, nil
	case latest == nil:
		current = GenerateUUID()
		if _, err := tx.Exec(`INSERT INTO journal_entries (id, campaign_id, session_id, author_id, created_at) VALUES (?, ?, ?, ?, ?)`,
			current, campaignID, sessionID, rev.AuthorID, now); err != nil {
			return models.JournalSaveResult{}, err
		}
	case entryID != "" && entryID != current:
		return models.JournalSaveResult{Outcome: models.JournalReplaced, EntryID: current, Latest: latest}, nil
	case baseRevision != latest.Number: // also a first draft written against an entry that now exists
		return models.JournalSaveResult{Outcome: models.JournalStale, EntryID: current, Latest: latest}, nil
	case latest.Name == rev.Name && latest.Body == rev.Body:
		return models.JournalSaveResult{Outcome: models.JournalUnchanged, EntryID: current, Revision: *latest}, nil
	}

	outcome := models.JournalCreated
	rev.Number = 1
	if latest != nil {
		outcome, rev.Number = models.JournalSaved, latest.Number+1
	}
	rev.CreatedAt = now
	if _, err := tx.Exec(`INSERT INTO journal_revisions (entry_id, number, author_id, name, body, note, restored_from, merged_with, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		current, rev.Number, rev.AuthorID, rev.Name, rev.Body, rev.Note, rev.RestoredFrom, rev.MergedWith, rev.CreatedAt); err != nil {
		return models.JournalSaveResult{}, err
	}
	if _, err := tx.Exec(`INSERT INTO journal_contributors (entry_id, user_id, revisions, first_revision) VALUES (?, ?, 1, ?)
		ON CONFLICT(entry_id, user_id) DO UPDATE SET revisions = revisions + 1`, current, rev.AuthorID, rev.Number); err != nil {
		return models.JournalSaveResult{}, err
	}
	if _, err := tx.Exec(`DELETE FROM journal_revisions WHERE entry_id = ? AND number <= ?`, current, rev.Number-keepRevisions); err != nil {
		return models.JournalSaveResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.JournalSaveResult{}, err
	}
	return models.JournalSaveResult{Outcome: outcome, EntryID: current, Revision: rev}, nil
}

// DeleteJournalEntry removes a session's entry with all its revisions and credits.
func (s *SQLiteStore) DeleteJournalEntry(sessionID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var one int
	if err := tx.QueryRow(`SELECT 1 FROM journal_entries WHERE session_id = ?`, sessionID).Scan(&one); err == sql.ErrNoRows {
		return ErrJournalNotFound
	} else if err != nil {
		return err
	}
	if err := deleteJournalForSessions(tx, `SELECT ?`, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}
