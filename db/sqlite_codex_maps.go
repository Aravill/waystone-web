package db

import (
	"database/sql"
	"encoding/json"
	"waystone-web/models"
)

const mapImageColumns = `id, page_id, file, width, height, uploaded_by, created_at`

func scanMapImage(row rowScanner) (models.MapImage, error) {
	var m models.MapImage
	err := row.Scan(&m.ID, &m.PageID, &m.File, &m.Width, &m.Height, &m.UploadedBy, &m.CreatedAt)
	return m, err
}

const mapPinColumns = `id, map_page_id, x, y, target_text, to_page_id, created_by, created_at, updated_at`

func scanMapPin(row rowScanner) (models.MapPin, error) {
	var p models.MapPin
	err := row.Scan(&p.ID, &p.MapPageID, &p.X, &p.Y, &p.TargetText, &p.ToPageID, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// mapWorld returns the map page's world, or ErrPageNotFound / ErrNotAMap.
func mapWorld(q querier, id string) (string, error) {
	var kind, world string
	err := q.QueryRow(`SELECT kind, world_id FROM pages WHERE id = ?`, id).Scan(&kind, &world)
	if err == sql.ErrNoRows {
		return "", ErrPageNotFound
	}
	if err != nil {
		return "", err
	}
	if kind != models.PageMap {
		return "", ErrNotAMap
	}
	return world, nil
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

func (s *SQLiteStore) AddMapImage(img models.MapImage) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, err := mapWorld(s.db, img.PageID); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO map_images (`+mapImageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		img.ID, img.PageID, img.File, img.Width, img.Height, img.UploadedBy, img.CreatedAt)
	return err
}

// RestoreMapImage makes an older image current again by adding a new row that points at the same
// file. Files are removed only when the page is deleted.
func (s *SQLiteStore) RestoreMapImage(pageID, imageID, userID string) (models.MapImage, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, err := mapWorld(s.db, pageID); err != nil {
		return models.MapImage{}, err
	}
	old, err := scanMapImage(s.db.QueryRow(`SELECT `+mapImageColumns+` FROM map_images WHERE id = ? AND page_id = ?`, imageID, pageID))
	if err == sql.ErrNoRows {
		return models.MapImage{}, ErrImageNotFound
	}
	if err != nil {
		return models.MapImage{}, err
	}
	old.ID, old.UploadedBy, old.CreatedAt = GenerateUUID(), userID, nowUTC()
	_, err = s.db.Exec(`INSERT INTO map_images (`+mapImageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		old.ID, old.PageID, old.File, old.Width, old.Height, old.UploadedBy, old.CreatedAt)
	return old, err
}

func (s *SQLiteStore) GetMapPins(pageID string) ([]models.MapPin, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	rows, err := s.db.Query(`SELECT `+mapPinColumns+` FROM map_pins WHERE map_page_id = ? ORDER BY created_at, id`, pageID)
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

func pinJSON(p models.MapPin) string {
	b, _ := json.Marshal(p)
	return string(b)
}

// logPinEvent records a pin change and keeps only the newest PinEventsKeep per map.
func logPinEvent(tx *sql.Tx, mapID, pinID, userID, action, before, after string) error {
	if _, err := tx.Exec(`INSERT INTO map_pin_events (id, map_page_id, pin_id, user_id, action, before, after, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		GenerateUUID(), mapID, pinID, userID, action, before, after, nowUTC()); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM map_pin_events WHERE map_page_id = ? AND id NOT IN
		(SELECT id FROM map_pin_events WHERE map_page_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?)`, mapID, mapID, PinEventsKeep)
	return err
}

// insertPin resolves the pin's target in the map's world and stores it.
func insertPin(tx *sql.Tx, world string, pin models.MapPin) (models.MapPin, error) {
	idx, err := loadCodexIndex(tx, world)
	if err != nil {
		return pin, err
	}
	pin.ToPageID = idx.resolve(world, pin.TargetText)
	_, err = tx.Exec(`INSERT INTO map_pins (`+mapPinColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pin.ID, pin.MapPageID, pin.X, pin.Y, pin.TargetText, pin.ToPageID, pin.CreatedBy, pin.CreatedAt, pin.UpdatedAt)
	return pin, err
}

// AddMapPin pins target_text on a map; it points at the page of that name in the map's world, if any.
func (s *SQLiteStore) AddMapPin(pin models.MapPin) (models.MapPin, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return pin, err
	}
	defer tx.Rollback()
	if pin, err = addMapPinTx(tx, pin); err != nil {
		return pin, err
	}
	return pin, tx.Commit()
}

// AddMapPinWithNewPage creates an empty lore page titled pin.TargetText in the map's world and
// pins it, in one transaction: a name clash leaves neither behind. page.ID, Title and AuthorID
// are used; the kind and world are set here.
func (s *SQLiteStore) AddMapPinWithNewPage(page models.Page, pin models.MapPin) (models.MapPin, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return pin, err
	}
	defer tx.Rollback()
	world, err := mapWorld(tx, pin.MapPageID)
	if err != nil {
		return pin, err
	}
	page.Kind, page.WorldID, page.Aliases = models.PageLore, world, nil
	if err := createPageTx(tx, page, ""); err != nil {
		return pin, err
	}
	if pin, err = addMapPinTx(tx, pin); err != nil {
		return pin, err
	}
	return pin, tx.Commit()
}

func addMapPinTx(tx *sql.Tx, pin models.MapPin) (models.MapPin, error) {
	world, err := mapWorld(tx, pin.MapPageID)
	if err != nil {
		return pin, err
	}
	pin.ID, pin.CreatedAt = GenerateUUID(), nowUTC()
	pin.UpdatedAt = pin.CreatedAt
	if pin, err = insertPin(tx, world, pin); err != nil {
		return pin, err
	}
	err = logPinEvent(tx, pin.MapPageID, pin.ID, pin.CreatedBy, "add", "", pinJSON(pin))
	return pin, err
}

func (s *SQLiteStore) MoveMapPin(mapID, pinID string, x, y float64, userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	before, err := scanMapPin(tx.QueryRow(`SELECT `+mapPinColumns+` FROM map_pins WHERE id = ? AND map_page_id = ?`, pinID, mapID))
	if err == sql.ErrNoRows {
		return ErrPinNotFound
	}
	if err != nil {
		return err
	}
	after := before
	after.X, after.Y, after.UpdatedAt = x, y, nowUTC()
	if _, err := tx.Exec(`UPDATE map_pins SET x = ?, y = ?, updated_at = ? WHERE id = ?`, x, y, after.UpdatedAt, pinID); err != nil {
		return err
	}
	if err := logPinEvent(tx, mapID, pinID, userID, "move", pinJSON(before), pinJSON(after)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) DeleteMapPin(mapID, pinID, userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	before, err := scanMapPin(tx.QueryRow(`SELECT `+mapPinColumns+` FROM map_pins WHERE id = ? AND map_page_id = ?`, pinID, mapID))
	if err == sql.ErrNoRows {
		return ErrPinNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM map_pins WHERE id = ?`, pinID); err != nil {
		return err
	}
	if err := logPinEvent(tx, mapID, pinID, userID, "remove", pinJSON(before), ""); err != nil {
		return err
	}
	return tx.Commit()
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
		out = append(out, e)
	}
	return out, rows.Err()
}

// UndoMapPinEvent reverses one pin change: an added pin is removed, a moved pin goes back, a
// removed pin is restored as it was (named after its target page's current title, if that page
// is still in the map's world). The event stays in the log, marked undone, and the undo is logged
// as an event of its own, which can be undone in turn.
func (s *SQLiteStore) UndoMapPinEvent(mapID, eventID, userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	world, err := mapWorld(tx, mapID)
	if err != nil {
		return err
	}
	var e models.MapPinEvent
	err = tx.QueryRow(`SELECT id, pin_id, action, before, after, undone_by FROM map_pin_events WHERE id = ? AND map_page_id = ?`, eventID, mapID).
		Scan(&e.ID, &e.PinID, &e.Action, &e.Before, &e.After, &e.UndoneBy)
	if err == sql.ErrNoRows {
		return ErrPinEventNotFound
	}
	if err != nil {
		return err
	}
	if e.UndoneBy != "" {
		return ErrPinEventUndone
	}
	current := func() (models.MapPin, error) {
		p, err := scanMapPin(tx.QueryRow(`SELECT `+mapPinColumns+` FROM map_pins WHERE id = ? AND map_page_id = ?`, e.PinID, mapID))
		if err == sql.ErrNoRows {
			err = ErrPinNotFound
		}
		return p, err
	}
	switch e.Action {
	case "add":
		cur, err := current()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM map_pins WHERE id = ?`, e.PinID); err != nil {
			return err
		}
		err = logPinEvent(tx, mapID, e.PinID, userID, "remove", pinJSON(cur), "")
		if err != nil {
			return err
		}
	case "move":
		var before models.MapPin
		if err := json.Unmarshal([]byte(e.Before), &before); err != nil {
			return err
		}
		cur, err := current()
		if err != nil {
			return err
		}
		restored := cur
		restored.X, restored.Y, restored.UpdatedAt = before.X, before.Y, nowUTC()
		if _, err := tx.Exec(`UPDATE map_pins SET x = ?, y = ?, updated_at = ? WHERE id = ?`, restored.X, restored.Y, restored.UpdatedAt, e.PinID); err != nil {
			return err
		}
		if err := logPinEvent(tx, mapID, e.PinID, userID, "move", pinJSON(cur), pinJSON(restored)); err != nil {
			return err
		}
	case "remove":
		var before models.MapPin
		if err := json.Unmarshal([]byte(e.Before), &before); err != nil {
			return err
		}
		if _, err := current(); err == nil {
			return ErrPinEventUndone // already back
		} else if err != ErrPinNotFound {
			return err
		}
		if before.ToPageID != "" { // the page may have been renamed since
			var title string
			err := tx.QueryRow(`SELECT title FROM pages WHERE id = ? AND world_id = ?`, before.ToPageID, world).Scan(&title)
			if err == nil {
				before.TargetText = title
			} else if err != sql.ErrNoRows {
				return err
			}
		}
		before.UpdatedAt = nowUTC()
		restored, err := insertPin(tx, world, before)
		if err != nil {
			return err
		}
		if err := logPinEvent(tx, mapID, e.PinID, userID, "add", "", pinJSON(restored)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE map_pin_events SET undone_by = ? WHERE id = ?`, userID, eventID); err != nil {
		return err
	}
	return tx.Commit()
}
