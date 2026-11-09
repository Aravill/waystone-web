package api

import (
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"

	_ "golang.org/x/image/webp"
)

const (
	maxMapImageBytes = 25 << 20
	maxMapImageSide  = 16384
)

var codexImageTypes = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}

func codexImageURL(file string) string { return "/api/codex/files/" + file }

// currentMapImage splits a page's image rows (newest first) into its history and its current
// image. A newest row with no file is the "map removed" marker: no current image, and markers
// are not part of the history.
func currentMapImage(rows []models.MapImage) (current *models.MapImage, history []models.MapImage) {
	history = []models.MapImage{}
	for i, img := range rows {
		if img.File == "" {
			continue
		}
		if i == 0 {
			c := img
			current = &c
		}
		history = append(history, img)
	}
	return current, history
}

// handleCodexMapRoutes serves the map endpoints under /api/codex/pages/{id}/.
// It reports false when rest is not one of them.
func handleCodexMapRoutes(w http.ResponseWriter, r *http.Request, id string, rest []string, method func(...string) bool) bool {
	switch {
	case rest[0] == "images" && len(rest) == 1:
		if method(http.MethodGet, http.MethodPost) {
			if r.Method == http.MethodGet {
				handleListMapImages(w, r, id)
			} else {
				handleUploadMapImage(w, r, id)
			}
		}
	case rest[0] == "map" && len(rest) == 1:
		if method(http.MethodDelete) {
			handleRemoveMap(w, r, id)
		}
	case rest[0] == "images" && len(rest) == 3 && rest[2] == "restore":
		if method(http.MethodPost) {
			handleRestoreMapImage(w, r, id, rest[1])
		}
	case rest[0] == "pins" && len(rest) == 1:
		if method(http.MethodPost) {
			handleAddPin(w, r, id)
		}
	case rest[0] == "pins" && len(rest) == 2:
		if method(http.MethodPut, http.MethodDelete) {
			if r.Method == http.MethodPut {
				handleMovePin(w, r, id, rest[1])
			} else {
				handleDeletePin(w, r, id, rest[1])
			}
		}
	case rest[0] == "pin-events" && len(rest) == 1:
		if method(http.MethodGet) {
			handlePinEvents(w, r, id)
		}
	case rest[0] == "pin-events" && len(rest) == 3 && rest[2] == "undo":
		if method(http.MethodPost) {
			handleUndoPinEvent(w, r, id, rest[1])
		}
	default:
		return false
	}
	return true
}

func codexMapError(w http.ResponseWriter, err error, what string) {
	switch {
	case errors.Is(err, db.ErrPinNotFound), errors.Is(err, db.ErrPinEventNotFound), errors.Is(err, db.ErrImageNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, db.ErrPinEventUndone):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "already_undone"})
	default:
		codexStoreError(w, err, what) // logs anything unexpected
	}
}

func codexImageObject(users map[string]*models.User, img models.MapImage, current bool) map[string]interface{} {
	return map[string]interface{}{
		"id": img.ID, "url": codexImageURL(img.File), "width": img.Width, "height": img.Height,
		"uploaded_by_user": journalUser(users, img.UploadedBy), "created_at": img.CreatedAt, "current": current,
	}
}

func codexPinObject(pin models.MapPin, pages map[string]models.PageListItem) map[string]interface{} {
	o := map[string]interface{}{"id": pin.ID, "x": pin.X, "y": pin.Y, "target_text": pin.TargetText, "status": "missing", "target": nil}
	if t, ok := pages[pin.ToPageID]; ok {
		o["status"] = "ok"
		o["target"] = map[string]interface{}{"id": t.ID, "title": t.Title, "has_map": t.HasMap}
	}
	return o
}

func codexPagesByID() (map[string]models.PageListItem, error) {
	all, err := db.GetCodexPages()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]models.PageListItem, len(all))
	for _, p := range all {
		byID[p.ID] = p
	}
	return byID, nil
}

// codexAddMapPayload adds the map's current image and pins to a page payload.
func codexAddMapPayload(payload map[string]interface{}, page *models.Page) error {
	images, err := db.GetMapImages(page.ID)
	if err != nil {
		return err
	}
	users, err := journalUsers()
	if err != nil {
		return err
	}
	pins, err := db.GetMapPins(page.ID)
	if err != nil {
		return err
	}
	pages, err := codexPagesByID()
	if err != nil {
		return err
	}
	var image interface{}
	if cur, _ := currentMapImage(images); cur != nil {
		image = codexImageObject(users, *cur, true)
	}
	pinList := make([]map[string]interface{}, 0, len(pins))
	for _, p := range pins {
		pinList = append(pinList, codexPinObject(p, pages))
	}
	payload["map"] = map[string]interface{}{"image": image, "pins": pinList}
	return nil
}

// codexAddOnMaps lists the maps that pin the page, with each map's current image for a thumbnail.
func codexAddOnMaps(payload map[string]interface{}, backs []models.PageBacklink) error {
	onMaps := []map[string]interface{}{}
	type mapData struct {
		pins     []models.MapPin
		imageURL string
	}
	cache := map[string]mapData{}
	for _, b := range backs {
		if b.PinID == "" {
			continue
		}
		d, ok := cache[b.PageID]
		if !ok {
			pins, err := db.GetMapPins(b.PageID)
			if err != nil {
				return err
			}
			images, err := db.GetMapImages(b.PageID)
			if err != nil {
				return err
			}
			d = mapData{pins: pins}
			if cur, _ := currentMapImage(images); cur != nil {
				d.imageURL = codexImageURL(cur.File)
			}
			cache[b.PageID] = d
		}
		o := map[string]interface{}{"map_id": b.PageID, "map_title": b.Title, "pin_id": b.PinID, "image_url": d.imageURL, "x": 0.5, "y": 0.5}
		for _, pin := range d.pins {
			if pin.ID == b.PinID {
				o["x"], o["y"] = pin.X, pin.Y
			}
		}
		onMaps = append(onMaps, o)
	}
	payload["on_maps"] = onMaps
	return nil
}

func handleListMapImages(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := codexUserID(w, r); !ok {
		return
	}
	images, err := db.GetMapImages(id)
	if err != nil {
		codexServerError(w, "retrieve images")
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	cur, history := currentMapImage(images)
	list := make([]map[string]interface{}, 0, len(history))
	for _, img := range history {
		list = append(list, codexImageObject(users, img, cur != nil && img.ID == cur.ID))
	}
	writeJSON(w, http.StatusOK, list)
}

// uploadTimeout is how long the slowest accepted upload (25 MB) may take to arrive and be answered.
const uploadTimeout = 150 * time.Second

// handleUploadMapImage takes the raw image as the request body (PNG, JPEG or WebP, 25 MB at most).
func handleUploadMapImage(w http.ResponseWriter, r *http.Request, id string) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return
	}
	// The server-wide timeouts suit small bodies; give this one room (unsupported writers ignore it).
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(uploadTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(uploadTimeout))
	// Check the page before reading the body.
	page, err := db.GetPage(id)
	if err != nil {
		codexServerError(w, "retrieve page")
		return
	}
	if page == nil {
		codexStoreError(w, db.ErrPageNotFound, "")
		return
	}
	tmp, err := db.CreateUploadTemp()
	if err != nil {
		log.Printf("failed to store map image: %v", err)
		codexServerError(w, "store the image")
		return
	}
	size, err := io.Copy(tmp, http.MaxBytesReader(w, r.Body, maxMapImageBytes))
	if err != nil {
		db.DiscardUpload(tmp)
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "the image must be 25 MB or smaller")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "failed to read the image")
		return
	}
	head := make([]byte, 512)
	n, _ := tmp.ReadAt(head, 0)
	ext, known := codexImageTypes[http.DetectContentType(head[:n])]
	if !known {
		db.DiscardUpload(tmp)
		writeJSONError(w, http.StatusBadRequest, "the image must be a PNG, JPEG or WebP file")
		return
	}
	cfg, _, err := image.DecodeConfig(io.NewSectionReader(tmp, 0, size))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxMapImageSide || cfg.Height > maxMapImageSide {
		db.DiscardUpload(tmp)
		writeJSONError(w, http.StatusBadRequest, "the image could not be read, or is larger than 16384 pixels on a side")
		return
	}
	file := db.GenerateUUID() + ext
	if err := db.CommitUpload(tmp, file); err != nil {
		log.Printf("failed to store map image: %v", err)
		codexServerError(w, "store the image")
		return
	}
	img := models.MapImage{ID: db.GenerateUUID(), PageID: id, File: file, Width: cfg.Width, Height: cfg.Height, UploadedBy: userID, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := db.AddMapImage(img); err != nil {
		removeCodexFiles([]string{file})
		codexMapError(w, err, "save the image")
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"image": codexImageObject(users, img, true)})
}

// removeCodexFiles deletes stored map images after their page is gone; failures are only logged.
func removeCodexFiles(files []string) {
	dir, err := db.UploadDir()
	if err != nil {
		log.Printf("cannot remove map images: %v", err)
		return
	}
	for _, f := range files {
		if !db.UploadFileRe.MatchString(f) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to remove map image %s: %v", f, err)
		}
	}
}

func handleCodexFile(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := codexUserID(w, r); !ok {
		return
	}
	if !db.UploadFileRe.MatchString(name) {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	dir, err := db.UploadDir()
	if err != nil {
		codexServerError(w, "read the image")
		return
	}
	ctype := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".webp": "image/webp"}[filepath.Ext(name)]
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'")
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		h.Del("Cache-Control")
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	http.ServeFile(w, r, path)
}

// handleRemoveMap clears the page's map and answers with how many pins went with it.
func handleRemoveMap(w http.ResponseWriter, r *http.Request, id string) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return
	}
	n, err := db.RemoveMap(id, userID)
	if err != nil {
		codexMapError(w, err, "remove the map")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "success", "pins_removed": n})
}

func handleRestoreMapImage(w http.ResponseWriter, r *http.Request, id, imageID string) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return
	}
	img, err := db.RestoreMapImage(id, imageID, userID)
	if err != nil {
		codexMapError(w, err, "restore the image")
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"image": codexImageObject(users, img, true)})
}

type pinRequest struct {
	X          *float64 `json:"x"`
	Y          *float64 `json:"y"`
	TargetText string   `json:"target_text"`
	CreatePage bool     `json:"create_page"` // add: create the lore page first (empty, in the map's world)
}

// validPosition reports whether both coordinates are present and fractions of the image.
func (req pinRequest) validPosition() bool {
	return req.X != nil && req.Y != nil && *req.X >= 0 && *req.X <= 1 && *req.Y >= 0 && *req.Y <= 1
}

func handleAddPin(w http.ResponseWriter, r *http.Request, id string) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return
	}
	var req pinRequest
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	text, valid := codexName(req.TargetText)
	if !valid || !req.validPosition() {
		writeJSONError(w, http.StatusBadRequest, "a pin needs a position on the map and a page title")
		return
	}
	mapPage, err := db.GetPage(id)
	if err != nil {
		codexServerError(w, "retrieve map")
		return
	}
	if mapPage == nil {
		codexStoreError(w, db.ErrPageNotFound, "")
		return
	}
	pin := models.MapPin{MapPageID: id, X: *req.X, Y: *req.Y, TargetText: text, CreatedBy: userID}
	if req.CreatePage {
		// The page and the pin are saved together; the store sets the page's world to the map's.
		pin, err = db.AddMapPinWithNewPage(models.Page{ID: db.GenerateUUID(), Title: text, AuthorID: userID}, pin)
	} else {
		pin, err = db.AddMapPin(pin)
	}
	if err != nil {
		codexMapError(w, err, "add the pin")
		return
	}
	pages, err := codexPagesByID()
	if err != nil {
		codexServerError(w, "retrieve pages")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"pin": codexPinObject(pin, pages)})
}

func handleMovePin(w http.ResponseWriter, r *http.Request, id, pinID string) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return
	}
	var req pinRequest
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	if !req.validPosition() {
		writeJSONError(w, http.StatusBadRequest, "a pin needs a position on the map")
		return
	}
	if err := db.MoveMapPin(id, pinID, *req.X, *req.Y, userID); err != nil {
		codexMapError(w, err, "move the pin")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func handleDeletePin(w http.ResponseWriter, r *http.Request, id, pinID string) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return
	}
	if err := db.DeleteMapPin(id, pinID, userID); err != nil {
		codexMapError(w, err, "remove the pin")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func handlePinEvents(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := codexUserID(w, r); !ok {
		return
	}
	events, err := db.GetMapPinEvents(id, db.PinEventsKeep)
	if err != nil {
		codexServerError(w, "retrieve pin changes")
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	list := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		var undone interface{}
		if e.UndoneBy != "" {
			undone = journalUser(users, e.UndoneBy)
		}
		list = append(list, map[string]interface{}{
			"id": e.ID, "action": e.Action, "user": journalUser(users, e.UserID), "created_at": e.CreatedAt,
			"undone_by_user": undone, "label": pinEventLabel(e),
		})
	}
	writeJSON(w, http.StatusOK, list)
}

func pinEventLabel(e models.MapPinEvent) string {
	snap := e.After
	if snap == "" {
		snap = e.Before
	}
	var pin models.MapPin
	_ = json.Unmarshal([]byte(snap), &pin)
	name := strings.TrimSpace(pin.TargetText)
	if utf8.RuneCountInString(name) > 60 {
		name = string([]rune(name)[:60]) + "…"
	}
	verb := map[string]string{"add": "Added a pin for", "move": "Moved the pin for", "remove": "Removed the pin for"}[e.Action]
	return verb + " “" + name + "”"
}

func handleUndoPinEvent(w http.ResponseWriter, r *http.Request, id, eventID string) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return
	}
	if err := db.UndoMapPinEvent(id, eventID, userID); err != nil {
		codexMapError(w, err, "undo the pin change")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}
