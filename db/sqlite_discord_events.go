package db

import (
	"database/sql"
	"waystone-web/models"
)

// Discord scheduled events (docs/design/discord-events.md). A game's settings live in campaigns
// (discord_events, discord_voice_id/name, discord_event_cover), written only here, never by
// SaveCampaign. discord_events holds one row per session that got an event on Discord.

const discordGameEventCols = `id, discord_events, discord_voice_id, discord_voice_name, discord_event_cover`

func scanDiscordGameEvents(r rowScanner) (models.DiscordGameEvents, error) {
	var g models.DiscordGameEvents
	var on int
	err := r.Scan(&g.CampaignID, &on, &g.VoiceID, &g.VoiceName, &g.Cover)
	g.On = on != 0
	return g, err
}

// GetDiscordGameEvents returns a game's event settings (zero values for an unknown game).
func (s *SQLiteStore) GetDiscordGameEvents(campaignID string) (models.DiscordGameEvents, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	g, err := scanDiscordGameEvents(s.db.QueryRow(`SELECT `+discordGameEventCols+` FROM campaigns WHERE id = ?`, campaignID))
	if err == sql.ErrNoRows {
		return models.DiscordGameEvents{CampaignID: campaignID}, nil
	}
	return g, err
}

// GetAllDiscordGameEvents returns every game's event settings by game id.
func (s *SQLiteStore) GetAllDiscordGameEvents() (map[string]models.DiscordGameEvents, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT ` + discordGameEventCols + ` FROM campaigns`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.DiscordGameEvents{}
	for rows.Next() {
		g, err := scanDiscordGameEvents(rows)
		if err != nil {
			return nil, err
		}
		out[g.CampaignID] = g
	}
	return out, rows.Err()
}

// SetDiscordGameEvents saves whether a game gets events and its voice channel. Turning events off
// drops the game's queued event creations (sessions confirmed meanwhile get a plain post instead:
// the caller decides that before queueing).
func (s *SQLiteStore) SetDiscordGameEvents(campaignID string, on bool, voiceID, voiceName string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE campaigns SET discord_events = ?, discord_voice_id = ?, discord_voice_name = ? WHERE id = ?`, boolInt(on), voiceID, voiceName, campaignID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrCampaignNotFound
	}
	if !on {
		if _, err := tx.Exec(`DELETE FROM discord_outbox WHERE kind = 'event_create' AND status = 'queued' AND campaign_id = ?`, campaignID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetDiscordEventCover sets (or with "" clears) a game's event cover and returns the previous file.
func (s *SQLiteStore) SetDiscordEventCover(campaignID, file string) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var old string
	if err := tx.QueryRow(`SELECT discord_event_cover FROM campaigns WHERE id = ?`, campaignID).Scan(&old); err != nil {
		if err == sql.ErrNoRows {
			return "", ErrCampaignNotFound
		}
		return "", err
	}
	if _, err := tx.Exec(`UPDATE campaigns SET discord_event_cover = ? WHERE id = ?`, file, campaignID); err != nil {
		return "", err
	}
	return old, tx.Commit()
}

// ClearLostDiscordVoice clears the game's voice channel and turns its events off, only if the voice
// channel still is voiceID. It reports whether it cleared anything.
func (s *SQLiteStore) ClearLostDiscordVoice(campaignID, voiceID string) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE campaigns SET discord_voice_id = '', discord_voice_name = '', discord_events = 0 WHERE id = ? AND discord_voice_id = ?`, campaignID, voiceID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		if _, err := tx.Exec(`DELETE FROM discord_outbox WHERE kind = 'event_create' AND status = 'queued' AND campaign_id = ?`, campaignID); err != nil {
			return false, err
		}
	}
	return n > 0, tx.Commit()
}

const discordEventCols = `session_id, campaign_id, guild_id, event_id, status`

func scanDiscordEvent(r rowScanner) (models.DiscordEvent, error) {
	var e models.DiscordEvent
	err := r.Scan(&e.SessionID, &e.CampaignID, &e.GuildID, &e.EventID, &e.Status)
	return e, err
}

// SaveDiscordEvent stores the event a session got (replacing an older one of the same session).
func (s *SQLiteStore) SaveDiscordEvent(e models.DiscordEvent) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, err := s.db.Exec(`INSERT INTO discord_events (`+discordEventCols+`, created_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET campaign_id = excluded.campaign_id, guild_id = excluded.guild_id,
			event_id = excluded.event_id, status = excluded.status, created_at = excluded.created_at`,
		e.SessionID, e.CampaignID, e.GuildID, e.EventID, e.Status, nowUTC())
	return err
}

// GetDiscordEvent returns a session's event, or nil when it has none.
func (s *SQLiteStore) GetDiscordEvent(sessionID string) (*models.DiscordEvent, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	e, err := scanDiscordEvent(s.db.QueryRow(`SELECT `+discordEventCols+` FROM discord_events WHERE session_id = ?`, sessionID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetDiscordEventsByStatus lists the events in the given statuses (all games when campaignID is "").
func (s *SQLiteStore) GetDiscordEventsByStatus(campaignID string, statuses ...string) ([]models.DiscordEvent, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	q := `SELECT ` + discordEventCols + ` FROM discord_events WHERE status IN (` + placeholders(len(statuses)) + `)`
	args := make([]interface{}, 0, len(statuses)+1)
	for _, st := range statuses {
		args = append(args, st)
	}
	if campaignID != "" {
		q += ` AND campaign_id = ?`
		args = append(args, campaignID)
	}
	rows, err := s.db.Query(q+` ORDER BY created_at, session_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DiscordEvent{}
	for rows.Next() {
		e, err := scanDiscordEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SetDiscordEventStatus records the status an event was (or is being) moved to.
func (s *SQLiteStore) SetDiscordEventStatus(sessionID, status string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, err := s.db.Exec(`UPDATE discord_events SET status = ? WHERE session_id = ?`, status, sessionID)
	return err
}

// DropQueuedDiscordEventCreate removes a session's queued event creation and reports whether there was one.
func (s *SQLiteStore) DropQueuedDiscordEventCreate(sessionID string) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	res, err := s.db.Exec(`DELETE FROM discord_outbox WHERE kind = 'event_create' AND status = 'queued' AND target = ?`, sessionID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func placeholders(n int) string {
	if n <= 0 {
		return "''"
	}
	b := make([]byte, 0, 2*n)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '?')
	}
	return string(b)
}
