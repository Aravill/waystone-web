package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"
	"waystone-web/models"
)

// ---- Map pages: the image (map_images), its pins (map_pins), their change log (map_pin_events)
// and the scale (map_scales). A pin may link a page of the same codex; then it shows that page's
// title. When the link goes (page deleted or moved to another codex) the pin keeps the title. ----

const mapImageColumns = `id, page_id, file, width, height, uploaded_by, created_at`

func scanMapImage(row rowScanner) (models.MapImage, error) {
	var m models.MapImage
	err := row.Scan(&m.ID, &m.PageID, &m.File, &m.Width, &m.Height, &m.UploadedBy, &m.CreatedAt)
	return m, err
}

// target_text holds a pin's name and to_page_id the page it links: the column names of the first Codex
// version, kept so that an older build still works on the database.
const mapPinColumns = `id, map_page_id, x, y, target_text, description, to_page_id, icon, color, created_by, created_at, updated_at`

func scanMapPin(row rowScanner) (models.MapPin, error) {
	var p models.MapPin
	err := row.Scan(&p.ID, &p.MapPageID, &p.X, &p.Y, &p.Name, &p.Description, &p.LinkedPageID, &p.Icon, &p.Color, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// mapCodexTx returns the codex of a map page: ErrPageNotFound, or ErrNotMap for any other kind of page.
func mapCodexTx(q querier, id string) (string, error) {
	var codex, kind string
	err := q.QueryRow(`SELECT codex_id, kind FROM pages WHERE id = ?`, id).Scan(&codex, &kind)
	if err == sql.ErrNoRows {
		return "", ErrPageNotFound
	}
	if err != nil {
		return "", err
	}
	if kind != models.PageMap {
		return "", ErrNotMap
	}
	return codex, nil
}

// currentMapImageTx returns the map's newest image, or nil. A row with no file is an old "map
// removed" marker.
func currentMapImageTx(q querier, pageID string) (*models.MapImage, error) {
	m, err := scanMapImage(q.QueryRow(`SELECT `+mapImageColumns+` FROM map_images WHERE page_id = ? ORDER BY created_at DESC, rowid DESC LIMIT 1`, pageID))
	if err == sql.ErrNoRows || (err == nil && m.File == "") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// mapCodexWithImageTx is mapCodexTx that also requires the map to have an image (pins and a scale need one).
func mapCodexWithImageTx(q querier, id string) (string, error) {
	codex, err := mapCodexTx(q, id)
	if err != nil {
		return "", err
	}
	if img, err := currentMapImageTx(q, id); err != nil {
		return "", err
	} else if img == nil {
		return "", ErrNoMap
	}
	return codex, nil
}

// GetMapImages lists a map's images, newest (the current one) first.
func (s *SQLiteStore) GetMapImages(pageID string) ([]models.MapImage, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT `+mapImageColumns+` FROM map_images WHERE page_id = ? ORDER BY created_at DESC, rowid DESC`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MapImage{}
	for rows.Next() {
		m, err := scanMapImage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func insertMapImageTx(q querier, img models.MapImage) error {
	_, err := q.Exec(`INSERT INTO map_images (`+mapImageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		img.ID, img.PageID, img.File, img.Width, img.Height, img.UploadedBy, img.CreatedAt)
	return err
}

// setMapImageTx makes img the map's current image. The scale only survives an image of the same
// shape (within 1%); it reports whether it was cleared.
func setMapImageTx(tx *sql.Tx, img models.MapImage) (cleared bool, err error) {
	old, err := currentMapImageTx(tx, img.PageID)
	if err != nil {
		return false, err
	}
	if old != nil {
		a, b := float64(old.Width)/float64(old.Height), float64(img.Width)/float64(img.Height)
		if math.Abs(a-b)/a > 0.01 {
			res, err := tx.Exec(`DELETE FROM map_scales WHERE page_id = ?`, img.PageID)
			if err != nil {
				return false, err
			}
			n, _ := res.RowsAffected()
			cleared = n > 0
		}
	}
	return cleared, insertMapImageTx(tx, img)
}

// AddMapImage makes img the map's current image and reports whether that cleared the scale.
func (s *SQLiteStore) AddMapImage(img models.MapImage) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := mapCodexTx(tx, img.PageID); err != nil {
		return false, err
	}
	cleared, err := setMapImageTx(tx, img)
	if err != nil {
		return false, err
	}
	return cleared, tx.Commit()
}

// RestoreMapImage makes an older image current again by adding a new row that points at the same
// file. Files are removed only when the page is deleted. It reports whether the scale was cleared.
func (s *SQLiteStore) RestoreMapImage(pageID, imageID, userID string) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := mapCodexTx(tx, pageID); err != nil {
		return false, err
	}
	old, err := scanMapImage(tx.QueryRow(`SELECT `+mapImageColumns+` FROM map_images WHERE id = ? AND page_id = ? AND file <> ''`, imageID, pageID))
	if err == sql.ErrNoRows {
		return false, ErrImageNotFound
	}
	if err != nil {
		return false, err
	}
	old.ID, old.UploadedBy, old.CreatedAt = GenerateUUID(), userID, nowUTC()
	cleared, err := setMapImageTx(tx, old)
	if err != nil {
		return false, err
	}
	return cleared, tx.Commit()
}

// ---- scale ----

// GetMapScale returns the map's scale, or nil when none is set.
func (s *SQLiteStore) GetMapScale(pageID string) (*models.MapScale, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	var m models.MapScale
	err := s.db.QueryRow(`SELECT x1, y1, x2, y2, length, unit FROM map_scales WHERE page_id = ?`, pageID).Scan(&m.X1, &m.Y1, &m.X2, &m.Y2, &m.Length, &m.Unit)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &m, err
}

func setMapScaleTx(q querier, pageID string, m models.MapScale, userID string) error {
	_, err := q.Exec(`INSERT OR REPLACE INTO map_scales (page_id, x1, y1, x2, y2, length, unit, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pageID, m.X1, m.Y1, m.X2, m.Y2, m.Length, m.Unit, userID, nowUTC())
	return err
}

// SetMapScale stores the map's scale; nil clears it.
func (s *SQLiteStore) SetMapScale(pageID string, m *models.MapScale, userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, err := mapCodexWithImageTx(s.db, pageID); err != nil {
		return err
	}
	if m == nil {
		_, err := s.db.Exec(`DELETE FROM map_scales WHERE page_id = ?`, pageID)
		return err
	}
	return setMapScaleTx(s.db, pageID, *m, userID)
}

// ---- pins ----

func mapPins(q querier, where, arg string) ([]models.MapPin, error) {
	rows, err := q.Query(`SELECT `+mapPinColumns+` FROM map_pins WHERE `+where+` ORDER BY created_at, id`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MapPin{}
	for rows.Next() {
		p, err := scanMapPin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetMapPins(pageID string) ([]models.MapPin, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return mapPins(s.db, `map_page_id = ?`, pageID)
}

// GetPinsLinkingTo lists the pins (on any map) that link the page.
func (s *SQLiteStore) GetPinsLinkingTo(pageID string) ([]models.MapPin, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return mapPins(s.db, `to_page_id = ?`, pageID)
}

// unlinkMapPinsTx makes every pin linking the page a plain pin named title.
func unlinkMapPinsTx(tx *sql.Tx, pageID, title string) error {
	_, err := tx.Exec(`UPDATE map_pins SET target_text = ?, to_page_id = '', updated_at = ? WHERE to_page_id = ?`, title, nowUTC(), pageID)
	return err
}

// unlinkOwnPinsTx makes every linked pin of a map a plain pin named after its page.
func unlinkOwnPinsTx(tx *sql.Tx, mapID string) error {
	_, err := tx.Exec(`UPDATE map_pins SET target_text = COALESCE((SELECT title FROM pages WHERE id = to_page_id), target_text), to_page_id = '', updated_at = ? WHERE map_page_id = ? AND to_page_id <> ''`, nowUTC(), mapID)
	return err
}

func pinJSON(p models.MapPin) string {
	b, _ := json.Marshal(p)
	return string(b)
}

// pinFromJSON reads a pin snapshot from the log, also one written before pins had a name of their own.
func pinFromJSON(s string) (models.MapPin, error) {
	var v struct {
		models.MapPin
		TargetText string `json:"target_text"`
		ToPageID   string `json:"to_page_id"`
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return models.MapPin{}, err
	}
	p := v.MapPin
	if p.Name == "" {
		p.Name, p.LinkedPageID = v.TargetText, v.ToPageID
	}
	if p.Icon == "" {
		p.Icon = models.PinIcons[0]
	}
	if p.Color == "" {
		p.Color = models.PinColors[0]
	}
	return p, nil
}

// logPinEvent records a pin change, keeps only the newest PinEventsKeep per map and returns the event's id.
func logPinEvent(tx *sql.Tx, mapID, pinID, userID, action, before, after string) (string, error) {
	id := GenerateUUID()
	if _, err := tx.Exec(`INSERT INTO map_pin_events (id, map_page_id, pin_id, user_id, action, before, after, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, mapID, pinID, userID, action, before, after, nowUTC()); err != nil {
		return "", err
	}
	_, err := tx.Exec(`DELETE FROM map_pin_events WHERE map_page_id = ? AND id NOT IN
		(SELECT id FROM map_pin_events WHERE map_page_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?)`, mapID, mapID, PinEventsKeep)
	return id, err
}

func insertMapPinTx(q querier, p models.MapPin) error {
	_, err := q.Exec(`INSERT INTO map_pins (`+mapPinColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.MapPageID, p.X, p.Y, p.Name, p.Description, p.LinkedPageID, p.Icon, p.Color, p.CreatedBy, p.CreatedAt, p.UpdatedAt)
	return err
}

func updateMapPinTx(q querier, p models.MapPin) error {
	_, err := q.Exec(`UPDATE map_pins SET x = ?, y = ?, target_text = ?, description = ?, to_page_id = ?, icon = ?, color = ?, updated_at = ? WHERE id = ?`,
		p.X, p.Y, p.Name, p.Description, p.LinkedPageID, p.Icon, p.Color, p.UpdatedAt, p.ID)
	return err
}

func getMapPinTx(q querier, mapID, pinID string) (models.MapPin, error) {
	p, err := scanMapPin(q.QueryRow(`SELECT `+mapPinColumns+` FROM map_pins WHERE id = ? AND map_page_id = ?`, pinID, mapID))
	if err == sql.ErrNoRows {
		err = ErrPinNotFound
	}
	return p, err
}

// addMapPinTx stores a new pin. A linked pin takes its page's title and has no description.
func addMapPinTx(tx *sql.Tx, codex string, p models.MapPin) (models.MapPin, error) {
	var err error
	if p.LinkedPageID != "" {
		if p.Name, err = linkedTitleTx(tx, codex, p.MapPageID, p.LinkedPageID); err != nil {
			return p, err
		}
		p.Description = ""
	}
	p.ID, p.CreatedAt = GenerateUUID(), nowUTC()
	p.UpdatedAt = p.CreatedAt
	if err := insertMapPinTx(tx, p); err != nil {
		return p, err
	}
	_, err = logPinEvent(tx, p.MapPageID, p.ID, p.CreatedBy, "add", "", pinJSON(p))
	return p, err
}

// AddMapPin adds a pin; everything but its id and times is used.
func (s *SQLiteStore) AddMapPin(p models.MapPin) (models.MapPin, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	codex, err := mapCodexWithImageTx(tx, p.MapPageID)
	if err != nil {
		return p, err
	}
	if p, err = addMapPinTx(tx, codex, p); err != nil {
		return p, err
	}
	return p, tx.Commit()
}

// AddMapPinWithPage creates the page (an article in the map's codex and folder, with the body and
// the tags x.TagNames) and a pin linking it, in one transaction: a name clash leaves neither behind.
func (s *SQLiteStore) AddMapPinWithPage(p models.MapPin, page models.Page, body string, x models.NewPageExtras) (models.MapPin, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	codex, err := mapCodexWithImageTx(tx, p.MapPageID)
	if err != nil {
		return p, err
	}
	page.Kind, page.CodexID, page.Aliases = models.PageLore, codex, nil
	if err := tx.QueryRow(`SELECT folder_id FROM pages WHERE id = ?`, p.MapPageID).Scan(&page.FolderID); err != nil {
		return p, err
	}
	if err := createPageNoteTx(tx, page, body, x.Note); err != nil {
		return p, err
	}
	if err := addPageTags(tx, page.ID, codex, x.TagNames, page.AuthorID); err != nil {
		return p, err
	}
	p.LinkedPageID = page.ID
	if p, err = addMapPinTx(tx, codex, p); err != nil {
		return p, err
	}
	return p, tx.Commit()
}

// changeMapPin loads a pin, lets change alter it, stores it and logs the action. It returns the
// event's id, or "" when nothing changed.
func (s *SQLiteStore) changeMapPin(mapID, pinID, userID, action string, change func(tx *sql.Tx, codex string, p *models.MapPin) error) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	codex, err := mapCodexTx(tx, mapID)
	if err != nil {
		return "", err
	}
	before, err := getMapPinTx(tx, mapID, pinID)
	if err != nil {
		return "", err
	}
	after := before
	if err := change(tx, codex, &after); err != nil {
		return "", err
	}
	if after == before {
		return "", nil
	}
	after.UpdatedAt = nowUTC()
	if err := updateMapPinTx(tx, after); err != nil {
		return "", err
	}
	id, err := logPinEvent(tx, mapID, pinID, userID, action, pinJSON(before), pinJSON(after))
	if err != nil {
		return "", err
	}
	return id, tx.Commit()
}

// MoveMapPin puts a pin somewhere else on its map.
func (s *SQLiteStore) MoveMapPin(mapID, pinID string, x, y float64, userID string) (string, error) {
	return s.changeMapPin(mapID, pinID, userID, "move", func(_ *sql.Tx, _ string, p *models.MapPin) error {
		p.X, p.Y = x, y
		return nil
	})
}

// UpdateMapPin changes a pin's name, description, link, icon and colour.
func (s *SQLiteStore) UpdateMapPin(v models.MapPin, userID string) (string, error) {
	return s.changeMapPin(v.MapPageID, v.ID, userID, "edit", func(tx *sql.Tx, codex string, p *models.MapPin) error {
		p.Name, p.Description, p.LinkedPageID, p.Icon, p.Color = v.Name, v.Description, v.LinkedPageID, v.Icon, v.Color
		if p.LinkedPageID == "" {
			return nil
		}
		p.Description = ""
		var err error
		p.Name, err = linkedTitleTx(tx, codex, p.MapPageID, p.LinkedPageID)
		return err
	})
}

// DeleteMapPin removes a pin (its page, if any, stays) and returns the logged event's id.
func (s *SQLiteStore) DeleteMapPin(mapID, pinID, userID string) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := mapCodexTx(tx, mapID); err != nil {
		return "", err
	}
	id, err := deleteMapPinTx(tx, mapID, pinID, userID)
	if err != nil {
		return "", err
	}
	return id, tx.Commit()
}

func deleteMapPinTx(tx *sql.Tx, mapID, pinID, userID string) (string, error) {
	before, err := getMapPinTx(tx, mapID, pinID)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(`DELETE FROM map_pins WHERE id = ?`, pinID); err != nil {
		return "", err
	}
	return logPinEvent(tx, mapID, pinID, userID, "remove", pinJSON(before), "")
}

func (s *SQLiteStore) GetMapPinEvents(mapID string, limit int) ([]models.MapPinEvent, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT id, map_page_id, pin_id, user_id, action, before, after, undone_by, created_at
		FROM map_pin_events WHERE map_page_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?`, mapID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MapPinEvent{}
	for rows.Next() {
		var e models.MapPinEvent
		if err := rows.Scan(&e.ID, &e.MapPageID, &e.PinID, &e.UserID, &e.Action, &e.Before, &e.After, &e.UndoneBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		// The name the pin had then (a linked pin: its page's title).
		snap := e.After
		if snap == "" {
			snap = e.Before
		}
		if p, err := pinFromJSON(snap); err == nil {
			e.Name = p.Name
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// relinkTx prepares a logged pin for coming back: a page that is gone from the map's codex leaves a
// plain pin, one that was renamed gives its current title.
func relinkTx(q querier, codex string, p *models.MapPin) error {
	if p.LinkedPageID == "" {
		return nil
	}
	title, err := linkedTitleTx(q, codex, p.MapPageID, p.LinkedPageID)
	if err == ErrLinkNotInCodex {
		p.LinkedPageID = ""
		return nil
	}
	if err == nil {
		p.Name = title
	}
	return err
}

// UndoMapPinEvent reverses one pin change: an added pin is removed, a moved pin goes back, an
// edited pin gets its old details back (not its place) and a removed pin is restored as it was. The
// event stays in the log, marked undone, and the undo is logged as an event of its own, which can be
// undone in turn.
func (s *SQLiteStore) UndoMapPinEvent(mapID, eventID, userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	codex, err := mapCodexTx(tx, mapID)
	if err != nil {
		return err
	}
	var e models.MapPinEvent
	err = tx.QueryRow(`SELECT id, pin_id, action, before, undone_by FROM map_pin_events WHERE id = ? AND map_page_id = ?`, eventID, mapID).
		Scan(&e.ID, &e.PinID, &e.Action, &e.Before, &e.UndoneBy)
	if err == sql.ErrNoRows {
		return ErrPinEventNotFound
	}
	if err != nil {
		return err
	}
	if e.UndoneBy != "" {
		return ErrPinEventUndone
	}
	var before models.MapPin
	if e.Action != "add" {
		if before, err = pinFromJSON(e.Before); err != nil {
			return err
		}
		before.MapPageID = mapID
		if err := relinkTx(tx, codex, &before); err != nil {
			return err
		}
	}
	switch e.Action {
	case "add":
		if _, err := deleteMapPinTx(tx, mapID, e.PinID, userID); err != nil {
			return err
		}
	case "move", "edit":
		cur, err := getMapPinTx(tx, mapID, e.PinID)
		if err != nil {
			return err
		}
		back := cur
		if e.Action == "move" {
			back.X, back.Y = before.X, before.Y
		} else {
			back.Name, back.Description, back.LinkedPageID, back.Icon, back.Color = before.Name, before.Description, before.LinkedPageID, before.Icon, before.Color
		}
		back.UpdatedAt = nowUTC()
		if err := updateMapPinTx(tx, back); err != nil {
			return err
		}
		if _, err := logPinEvent(tx, mapID, e.PinID, userID, e.Action, pinJSON(cur), pinJSON(back)); err != nil {
			return err
		}
	case "remove":
		if _, err := getMapPinTx(tx, mapID, e.PinID); err == nil {
			return ErrPinEventUndone // already back
		} else if err != ErrPinNotFound {
			return err
		}
		before.UpdatedAt = nowUTC()
		if err := insertMapPinTx(tx, before); err != nil {
			return err
		}
		if _, err := logPinEvent(tx, mapID, e.PinID, userID, "add", "", pinJSON(before)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE map_pin_events SET undone_by = ? WHERE id = ?`, userID, eventID); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- migrations ----

// addMapPinColumns adds the columns map_pins got when maps became pages.
func (s *SQLiteStore) addMapPinColumns() error {
	if has, err := s.columnExists("map_pins", "icon"); err != nil || has {
		return err
	}
	for _, c := range []string{"description TEXT NOT NULL DEFAULT ''", "icon TEXT NOT NULL DEFAULT 'place'", "color TEXT NOT NULL DEFAULT 'red'"} {
		if _, err := s.db.Exec(`ALTER TABLE map_pins ADD COLUMN ` + c); err != nil {
			return fmt.Errorf("failed to add columns to map_pins: %w", err)
		}
	}
	return nil
}

// convertOldMaps turns the articles that carry a map from the first Codex version (hidden since
// PR 60) into map pages. A map has no text, so their text, history and pictures are deleted, as
// decided in docs/design/codex-maps.md; the image, pins, title, aliases, tags and attachments stay.
// Articles can't get a map any more, so later starts find nothing. When BACKUP_DIR is set a
// snapshot is written there first, and nothing is converted without it.
func (s *SQLiteStore) convertOldMaps() error {
	type oldMap struct{ id, title, author string }
	rows, err := s.db.Query(`SELECT id, title, author_id FROM pages WHERE kind = ? AND session_id = ''
		AND COALESCE((SELECT file FROM map_images i WHERE i.page_id = pages.id ORDER BY created_at DESC, rowid DESC LIMIT 1), '') <> ''`, models.PageLore)
	if err != nil {
		return fmt.Errorf("failed to look for old maps: %w", err)
	}
	var maps []oldMap
	for rows.Next() {
		var m oldMap
		if err := rows.Scan(&m.id, &m.title, &m.author); err != nil {
			rows.Close()
			return err
		}
		maps = append(maps, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(maps) == 0 {
		return err
	}
	if dir := os.Getenv("BACKUP_DIR"); dir != "" {
		snap := filepath.Join(dir, "before-maps-"+time.Now().UTC().Format("20060102-150405")+".db")
		err := os.MkdirAll(dir, 0o755)
		if err == nil {
			err = s.Backup(snap)
		}
		if err != nil {
			log.Printf("Old maps are not converted, the snapshot before it failed: %v", err)
			return nil
		}
		log.Printf("Snapshot before converting old maps: %s", snap)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var files []string
	for _, m := range maps {
		var chars, revisions, pins int
		if err := tx.QueryRow(`SELECT COALESCE((SELECT LENGTH(body) FROM page_revisions WHERE page_id = ?1 ORDER BY number DESC LIMIT 1), 0),
			(SELECT COUNT(*) FROM page_revisions WHERE page_id = ?1), (SELECT COUNT(*) FROM map_pins WHERE map_page_id = ?1)`, m.id).Scan(&chars, &revisions, &pins); err != nil {
			return err
		}
		pics, err := queryFiles(tx, `SELECT file FROM page_images WHERE page_id = ?`, m.id)
		if err != nil {
			return err
		}
		files = append(files, pics...)
		for _, q := range []string{
			`DELETE FROM page_revisions WHERE page_id = ?`,
			`DELETE FROM page_contributors WHERE page_id = ?`,
			`DELETE FROM page_links WHERE from_page_id = ?`,
			`DELETE FROM page_images WHERE page_id = ?`,
			`UPDATE map_pins SET to_page_id = '' WHERE map_page_id = ?1 AND (to_page_id = ?1 OR to_page_id NOT IN (SELECT id FROM pages))`,
			`UPDATE pages SET kind = '` + models.PageMap + `', cover_image_id = '' WHERE id = ?`,
		} {
			if _, err := tx.Exec(q, m.id); err != nil {
				return fmt.Errorf("failed to convert the old map %q: %w", m.title, err)
			}
		}
		if _, err := insertPageRevision(tx, m.id, models.PageRevision{AuthorID: m.author, Title: m.title, Note: "Became a map page"}, PageKeepRevisions); err != nil {
			return err
		}
		log.Printf("Old map %q (%s) is now a map page: %d pins kept, %d characters of text and %d revisions deleted, %d pictures deleted", m.title, m.id, pins, chars, revisions, len(pics))
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if dir, err := UploadDir(); err == nil {
		for _, f := range files {
			if UploadFileRe.MatchString(f) {
				os.Remove(filepath.Join(dir, f))
			}
		}
	}
	return nil
}
