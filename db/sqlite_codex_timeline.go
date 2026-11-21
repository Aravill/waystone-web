package db

import (
	"database/sql"
	"strings"
	"waystone-web/models"
)

// ---- Timeline pages: dated events (timeline_events) and the calendar names (timeline_calendars).
// An event may link a page of the same codex; then it shows that page's title. When the link goes
// (page deleted or moved to another codex) the event keeps the title it had. ----

const timelineEventColumns = `id, page_id, title, linked_page_id, year, month, day, created_by, created_at, updated_at`

func scanTimelineEvent(row rowScanner) (models.TimelineEvent, error) {
	var e models.TimelineEvent
	err := row.Scan(&e.ID, &e.PageID, &e.Title, &e.LinkedPageID, &e.Year, &e.Month, &e.Day, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

func timelineEvents(q querier, where string, arg string) ([]models.TimelineEvent, error) {
	rows, err := q.Query(`SELECT `+timelineEventColumns+` FROM timeline_events WHERE `+where+` ORDER BY year DESC, month DESC, day DESC, created_at DESC, id`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TimelineEvent{}
	for rows.Next() {
		e, err := scanTimelineEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// timelineCodexTx returns the codex of a timeline page: ErrPageNotFound, or ErrNotTimeline for
// any other kind of page.
func timelineCodexTx(q querier, pageID string) (string, error) {
	var codex, kind string
	err := q.QueryRow(`SELECT codex_id, kind FROM pages WHERE id = ?`, pageID).Scan(&codex, &kind)
	if err == sql.ErrNoRows {
		return "", ErrPageNotFound
	}
	if err != nil {
		return "", err
	}
	if kind != models.PageTimeline {
		return "", ErrNotTimeline
	}
	return codex, nil
}

// linkedTitleTx checks that the linked page can be linked from the timeline (it exists, is in the
// codex and is not the timeline itself) and returns its current title for the event's snapshot.
func linkedTitleTx(q querier, codexID, timelineID, linkedID string) (string, error) {
	var title string
	err := q.QueryRow(`SELECT title FROM pages WHERE id = ? AND codex_id = ? AND id <> ?`, linkedID, codexID, timelineID).Scan(&title)
	if err == sql.ErrNoRows {
		return "", ErrLinkNotInCodex
	}
	return title, err
}

// unlinkTimelineEventsTx makes every event linking the page a plain event titled title.
func unlinkTimelineEventsTx(tx *sql.Tx, pageID, title string) error {
	_, err := tx.Exec(`UPDATE timeline_events SET title = ?, linked_page_id = '', updated_at = ? WHERE linked_page_id = ?`, title, nowUTC(), pageID)
	return err
}

func insertTimelineEventTx(tx *sql.Tx, e models.TimelineEvent) error {
	if e.CreatedAt == "" {
		e.CreatedAt = nowUTC()
	}
	if e.UpdatedAt == "" {
		e.UpdatedAt = e.CreatedAt
	}
	_, err := tx.Exec(`INSERT INTO timeline_events (`+timelineEventColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.PageID, e.Title, e.LinkedPageID, e.Year, e.Month, e.Day, e.CreatedBy, e.CreatedAt, e.UpdatedAt)
	return err
}

// GetTimelineEvents lists a timeline's events, newest first.
func (s *SQLiteStore) GetTimelineEvents(pageID string) ([]models.TimelineEvent, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return timelineEvents(s.db, `page_id = ?`, pageID)
}

// GetEventsLinkingTo lists the events (on any timeline) that link the page.
func (s *SQLiteStore) GetEventsLinkingTo(pageID string) ([]models.TimelineEvent, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return timelineEvents(s.db, `linked_page_id = ?`, pageID)
}

// CreateTimelineEvent adds an event; e.PageID, Year, Month, Day, CreatedBy and Title are used. When
// e.LinkedPageID is set the event takes that page's title.
func (s *SQLiteStore) CreateTimelineEvent(e models.TimelineEvent) (models.TimelineEvent, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return e, err
	}
	defer tx.Rollback()
	codex, err := timelineCodexTx(tx, e.PageID)
	if err != nil {
		return e, err
	}
	if e.LinkedPageID != "" {
		if e.Title, err = linkedTitleTx(tx, codex, e.PageID, e.LinkedPageID); err != nil {
			return e, err
		}
	}
	e.ID, e.CreatedAt = GenerateUUID(), nowUTC()
	e.UpdatedAt = e.CreatedAt
	if err := insertTimelineEventTx(tx, e); err != nil {
		return e, err
	}
	return e, tx.Commit()
}

// CreateTimelineEventWithPage creates the page (a lore page in the timeline's codex, in p.FolderID,
// with the body, tags x.TagNames and note) and an event linking it, in one transaction: a name
// clash leaves neither behind.
func (s *SQLiteStore) CreateTimelineEventWithPage(e models.TimelineEvent, p models.Page, body string, x models.NewPageExtras) (models.TimelineEvent, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return e, err
	}
	defer tx.Rollback()
	codex, err := timelineCodexTx(tx, e.PageID)
	if err != nil {
		return e, err
	}
	p.Kind, p.CodexID, p.Aliases = models.PageLore, codex, nil
	if err := createPageNoteTx(tx, p, body, x.Note); err != nil {
		return e, err
	}
	if err := addPageTags(tx, p.ID, codex, x.TagNames, p.AuthorID); err != nil {
		return e, err
	}
	e.ID, e.CreatedAt = GenerateUUID(), nowUTC()
	e.UpdatedAt = e.CreatedAt
	e.Title, e.LinkedPageID = p.Title, p.ID
	if err := insertTimelineEventTx(tx, e); err != nil {
		return e, err
	}
	return e, tx.Commit()
}

// UpdateTimelineEvent changes an event's title, link and date. ErrEventNotFound when it is gone.
func (s *SQLiteStore) UpdateTimelineEvent(e models.TimelineEvent) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	codex, err := timelineCodexTx(tx, e.PageID)
	if err != nil {
		return err
	}
	if e.LinkedPageID != "" {
		if e.Title, err = linkedTitleTx(tx, codex, e.PageID, e.LinkedPageID); err != nil {
			return err
		}
	}
	res, err := tx.Exec(`UPDATE timeline_events SET title = ?, linked_page_id = ?, year = ?, month = ?, day = ?, updated_at = ? WHERE id = ? AND page_id = ?`,
		e.Title, e.LinkedPageID, e.Year, e.Month, e.Day, nowUTC(), e.ID, e.PageID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEventNotFound
	}
	return tx.Commit()
}

// DeleteTimelineEvent removes an event; its page, if any, stays.
func (s *SQLiteStore) DeleteTimelineEvent(pageID, eventID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	res, err := s.db.Exec(`DELETE FROM timeline_events WHERE id = ? AND page_id = ?`, eventID, pageID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEventNotFound
	}
	return nil
}

// GetTimelineCalendar returns the timeline's calendar; the zero value (no names) when none is set.
func (s *SQLiteStore) GetTimelineCalendar(pageID string) (models.TimelineCalendar, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	c := models.TimelineCalendar{Months: []string{}}
	var months string
	err := s.db.QueryRow(`SELECT months, era, before_era FROM timeline_calendars WHERE page_id = ?`, pageID).Scan(&months, &c.Era, &c.BeforeEra)
	if err == sql.ErrNoRows {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if months != "" {
		c.Months = strings.Split(months, "\n")
	}
	return c, nil
}

func setTimelineCalendarTx(q querier, pageID string, c models.TimelineCalendar, userID string) error {
	_, err := q.Exec(`INSERT INTO timeline_calendars (page_id, months, era, before_era, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(page_id) DO UPDATE SET months = excluded.months, era = excluded.era, before_era = excluded.before_era, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		pageID, strings.Join(c.Months, "\n"), c.Era, c.BeforeEra, userID, nowUTC())
	return err
}

// SetTimelineCalendar replaces the timeline's calendar (callers pass cleaned values).
func (s *SQLiteStore) SetTimelineCalendar(pageID string, c models.TimelineCalendar, userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, err := timelineCodexTx(s.db, pageID); err != nil {
		return err
	}
	return setTimelineCalendarTx(s.db, pageID, c, userID)
}
