package db

import (
	"database/sql"
	"encoding/json"
	"strings"
	"waystone-web/models"
)

// GetUnremindedConfirmedSessions returns confirmed, non-manual sessions dated within the range
// that have no session_reminder notification yet: the notifications table is the dedupe.
func (s *SQLiteStore) GetUnremindedConfirmedSessions(fromDate, toDate string) ([]models.CampaignSession, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, campaign_id, date, time, duration, status, timezone, manual, guests, dm_absent, created_at, updated_at, poll_id
		FROM campaign_sessions
		WHERE status = ? AND manual = 0 AND date >= ? AND date <= ?
		  AND NOT EXISTS (SELECT 1 FROM notifications n WHERE n.type = 'session_reminder' AND n.session_id = campaign_sessions.id)
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

// ---- Settings ----

func allDiscordEventKeys() []string {
	keys := make([]string, 0, len(models.DiscordEvents))
	for _, e := range models.DiscordEvents {
		keys = append(keys, e.Key)
	}
	return keys
}

// GetDiscordSettings reads the settings; a missing events row means every event is on.
func (s *SQLiteStore) GetDiscordSettings() (models.DiscordSettings, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	out := models.DiscordSettings{Events: allDiscordEventKeys()}
	rows, err := s.db.Query(`SELECT key, value FROM app_settings WHERE key IN ('discord_guild_id', 'discord_invite_url', 'discord_channel_events')`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return out, err
		}
		switch k {
		case "discord_guild_id":
			out.GuildID = v
		case "discord_invite_url":
			out.InviteURL = v
		case "discord_channel_events":
			var events []string
			if json.Unmarshal([]byte(v), &events) == nil {
				out.Events = events
			}
		}
	}
	return out, rows.Err()
}

// SaveDiscordSettings stores the settings. Moving to another server (the old guild was set and
// differs) clears every game's channel, voice channel and event cover, turns events off and drops
// queued channel posts and event creations; it returns how many games lost their channel. The
// caller removes the cover files.
func (s *SQLiteStore) SaveDiscordSettings(st models.DiscordSettings) (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var oldGuild string
	if err := tx.QueryRow(`SELECT value FROM app_settings WHERE key = 'discord_guild_id'`).Scan(&oldGuild); err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	cleared := 0
	if oldGuild != "" && oldGuild != st.GuildID {
		res, err := tx.Exec(`UPDATE campaigns SET discord_channel_id = '', discord_channel_name = '' WHERE discord_channel_id <> ''`)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		cleared = int(n)
		if _, err := tx.Exec(`UPDATE campaigns SET discord_events = 0, discord_voice_id = '', discord_voice_name = '', discord_event_cover = ''`); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`DELETE FROM discord_outbox WHERE kind IN ('channel', 'event_create') AND status = 'queued'`); err != nil {
			return 0, err
		}
	}
	events := st.Events
	if events == nil {
		events = []string{}
	}
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return 0, err
	}
	for k, v := range map[string]string{
		"discord_guild_id":       st.GuildID,
		"discord_invite_url":     st.InviteURL,
		"discord_channel_events": string(eventsJSON),
	} {
		if _, err := tx.Exec(`INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return 0, err
		}
	}
	return cleared, tx.Commit()
}

// ---- Game channels ----

// SetCampaignDiscordChannel sets (or with "" clears) a game's channel. Changing it drops the
// game's queued channel posts, which were meant for the old channel.
func (s *SQLiteStore) SetCampaignDiscordChannel(campaignID, channelID, channelName string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var old string
	if err := tx.QueryRow(`SELECT discord_channel_id FROM campaigns WHERE id = ?`, campaignID).Scan(&old); err != nil {
		if err == sql.ErrNoRows {
			return ErrCampaignNotFound
		}
		return err
	}
	if _, err := tx.Exec(`UPDATE campaigns SET discord_channel_id = ?, discord_channel_name = ? WHERE id = ?`, channelID, channelName, campaignID); err != nil {
		return err
	}
	if old != channelID {
		if _, err := tx.Exec(`DELETE FROM discord_outbox WHERE kind = 'channel' AND status = 'queued' AND campaign_id = ?`, campaignID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClearLostDiscordChannel clears the game's channel only if it still is channelID (the admin may
// have picked another meanwhile), and turns the game's events off. It reports whether it cleared anything.
func (s *SQLiteStore) ClearLostDiscordChannel(campaignID, channelID string) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE campaigns SET discord_channel_id = '', discord_channel_name = '', discord_events = 0 WHERE id = ? AND discord_channel_id = ?`, campaignID, channelID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		if _, err := tx.Exec(`DELETE FROM discord_outbox WHERE kind IN ('channel', 'event_create') AND status = 'queued' AND campaign_id = ?`, campaignID); err != nil {
			return false, err
		}
	}
	return n > 0, tx.Commit()
}

// ---- User links ----

const discordLinkCols = `user_id, discord_user_id, discord_username, dm_channel_id, dm_sessions, dm_signups, dm_characters, dm_error, dm_error_at, linked_at`

func scanDiscordLink(r rowScanner) (models.DiscordLink, error) {
	var l models.DiscordLink
	var a, b, c int
	err := r.Scan(&l.UserID, &l.DiscordUserID, &l.DiscordUsername, &l.DMChannelID, &a, &b, &c, &l.DMError, &l.DMErrorAt, &l.LinkedAt)
	l.DMSessions, l.DMSignups, l.DMCharacters = a != 0, b != 0, c != 0
	return l, err
}

// GetDiscordLink returns the user's link, or nil, nil when there is none.
func (s *SQLiteStore) GetDiscordLink(userID string) (*models.DiscordLink, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	l, err := scanDiscordLink(s.db.QueryRow(`SELECT `+discordLinkCols+` FROM discord_links WHERE user_id = ?`, userID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// SaveDiscordLink links a Discord account to a user. An account already linked to another user
// is refused with ErrDiscordAccountTaken. Re-linking the same account only refreshes the
// username; linking a different one replaces the link (toggles reset, queued DMs dropped).
func (s *SQLiteStore) SaveDiscordLink(userID, discordUserID, username string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var owner string
	err = tx.QueryRow(`SELECT user_id FROM discord_links WHERE discord_user_id = ?`, discordUserID).Scan(&owner)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && owner != userID {
		return ErrDiscordAccountTaken
	}
	if err == nil {
		if _, err := tx.Exec(`UPDATE discord_links SET discord_username = ? WHERE user_id = ?`, username, userID); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err := tx.Exec(`DELETE FROM discord_links WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM discord_outbox WHERE kind = 'dm' AND target = ? AND status = 'queued'`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO discord_links (user_id, discord_user_id, discord_username, linked_at) VALUES (?, ?, ?, ?)`,
		userID, discordUserID, username, nowUTC()); err != nil {
		return err
	}
	return tx.Commit()
}

// GetDiscordLinksForUsers returns the links of the given users, leaving out blocked and deleted ones.
func (s *SQLiteStore) GetDiscordLinksForUsers(userIDs []string) (map[string]models.DiscordLink, error) {
	out := map[string]models.DiscordLink{}
	if len(userIDs) == 0 {
		return out, nil
	}
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		args[i] = id
	}
	rows, err := s.db.Query(`SELECT l.user_id, l.discord_user_id, l.discord_username, l.dm_channel_id, l.dm_sessions, l.dm_signups, l.dm_characters, l.dm_error, l.dm_error_at, l.linked_at
		FROM discord_links l JOIN users u ON u.id = l.user_id
		WHERE u.blocked = 0 AND u.deleted = 0 AND l.user_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		l, err := scanDiscordLink(rows)
		if err != nil {
			return nil, err
		}
		out[l.UserID] = l
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SetDiscordToggles(userID string, sessions, signups, characters bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	_, err := s.db.Exec(`UPDATE discord_links SET dm_sessions = ?, dm_signups = ?, dm_characters = ? WHERE user_id = ?`,
		b(sessions), b(signups), b(characters), userID)
	return err
}

// DeleteDiscordLink removes the link and the user's queued DMs.
func (s *SQLiteStore) DeleteDiscordLink(userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM discord_links WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM discord_outbox WHERE kind = 'dm' AND target = ? AND status = 'queued'`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) SetDiscordDMChannel(userID, channelID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, err := s.db.Exec(`UPDATE discord_links SET dm_channel_id = ? WHERE user_id = ?`, channelID, userID)
	return err
}

// SetDiscordDMError records the last refused DM; "" clears it.
func (s *SQLiteStore) SetDiscordDMError(userID, msg string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	at := ""
	if msg != "" {
		at = nowUTC()
	}
	_, err := s.db.Exec(`UPDATE discord_links SET dm_error = ?, dm_error_at = ? WHERE user_id = ?`, msg, at, userID)
	return err
}

// ---- Outbox ----

func (s *SQLiteStore) EnqueueDiscord(rows []models.DiscordOutboxRow) error {
	if len(rows) == 0 {
		return nil
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range rows {
		if _, err := tx.Exec(`INSERT INTO discord_outbox (kind, target, campaign_id, payload, next_attempt_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			r.Kind, r.Target, r.CampaignID, r.Payload, r.NextAttemptAt, r.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const discordOutboxCols = `id, kind, target, campaign_id, payload, attempts, next_attempt_at, last_error, status, created_at`

func scanDiscordOutbox(r rowScanner) (models.DiscordOutboxRow, error) {
	var o models.DiscordOutboxRow
	err := r.Scan(&o.ID, &o.Kind, &o.Target, &o.CampaignID, &o.Payload, &o.Attempts, &o.NextAttemptAt, &o.LastError, &o.Status, &o.CreatedAt)
	return o, err
}

// NextDiscordOutbox returns the oldest queued row that is due at now (RFC3339 UTC), or nil.
func (s *SQLiteStore) NextDiscordOutbox(now string) (*models.DiscordOutboxRow, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	o, err := scanDiscordOutbox(s.db.QueryRow(`SELECT `+discordOutboxCols+` FROM discord_outbox WHERE status = 'queued' AND next_attempt_at <= ? ORDER BY id LIMIT 1`, now))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *SQLiteStore) DeleteDiscordOutbox(id int64) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, err := s.db.Exec(`DELETE FROM discord_outbox WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) UpdateDiscordOutbox(id int64, status string, attempts int, nextAttemptAt, lastError string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, err := s.db.Exec(`UPDATE discord_outbox SET status = ?, attempts = ?, next_attempt_at = ?, last_error = ? WHERE id = ?`,
		status, attempts, nextAttemptAt, lastError, id)
	return err
}

// PruneDiscordOutbox deletes failed rows created before the given time (RFC3339 UTC).
func (s *SQLiteStore) PruneDiscordOutbox(failedBefore string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, err := s.db.Exec(`DELETE FROM discord_outbox WHERE status = 'failed' AND created_at < ?`, failedBefore)
	return err
}

// GetDiscordOutboxSummary returns the queued count and the latest failed rows, newest first.
func (s *SQLiteStore) GetDiscordOutboxSummary(limit int) (int, []models.DiscordOutboxRow, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var queued int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM discord_outbox WHERE status = 'queued'`).Scan(&queued); err != nil {
		return 0, nil, err
	}
	rows, err := s.db.Query(`SELECT `+discordOutboxCols+` FROM discord_outbox WHERE status = 'failed' ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	failures := []models.DiscordOutboxRow{}
	for rows.Next() {
		o, err := scanDiscordOutbox(rows)
		if err != nil {
			return 0, nil, err
		}
		failures = append(failures, o)
	}
	return queued, failures, rows.Err()
}
