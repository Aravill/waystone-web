package api

import (
	"bufio"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
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

var codexImageTypes = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "image/gif": ".gif"}

func codexImageURL(file string) string { return "/api/codices/files/" + file }

// currentMapImage splits a page's image rows (newest first) into its history and its current
// image. A newest row with no file is an old "map removed" marker: no current image, and markers
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

// handleCodexMapRoutes serves the map endpoints under /api/codices/pages/{id}/.
// It reports false when rest is not one of them.
func handleCodexMapRoutes(w http.ResponseWriter, r *http.Request, id string, rest []string, method func(...string) bool) bool {
	switch {
	case rest[0] == "map" && len(rest) == 1:
		if method(http.MethodGet) {
			handleGetMap(w, r, id)
		}
	case rest[0] == "map" && len(rest) == 2 && rest[1] == "scale":
		if method(http.MethodPut, http.MethodDelete) {
			handleSetMapScale(w, r, id)
		}
	case rest[0] == "images" && len(rest) == 1:
		if method(http.MethodGet, http.MethodPost) {
			if r.Method == http.MethodGet {
				handleListMapImages(w, r, id)
			} else {
				handleUploadMapImage(w, r, id)
			}
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
				handleChangePin(w, r, id, rest[1])
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
	case errors.Is(err, db.ErrNotMap):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "This page is not a map.", "code": "not_a_map"})
	case errors.Is(err, db.ErrNoMap):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "no_image"})
	case errors.Is(err, db.ErrPinNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error(), "code": "pin_deleted"})
	case errors.Is(err, db.ErrPinEventNotFound), errors.Is(err, db.ErrImageNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, db.ErrPinEventUndone):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "already_undone"})
	case errors.Is(err, db.ErrLinkNotInCodex):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "page_not_in_codex"})
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

// codexMapPayload is the map as the client shows it: {image, pins, scale}. A pin has a linked page
// (title, kind, excerpt) or null; a link to a page that is gone counts as no link.
func codexMapPayload(pageID, codexID string) (map[string]interface{}, error) {
	images, err := db.GetMapImages(pageID)
	if err != nil {
		return nil, err
	}
	pins, err := db.GetMapPins(pageID)
	if err != nil {
		return nil, err
	}
	scale, err := db.GetMapScale(pageID)
	if err != nil {
		return nil, err
	}
	users, err := journalUsers()
	if err != nil {
		return nil, err
	}
	pages := map[string]models.GraphPage{}
	for _, p := range pins {
		if p.LinkedPageID == "" {
			continue
		}
		list, _, err := db.GetCodexGraph(codexID)
		if err != nil {
			return nil, err
		}
		for _, g := range list {
			pages[g.ID] = g
		}
		break
	}
	var image interface{}
	if cur, _ := currentMapImage(images); cur != nil {
		image = codexImageObject(users, *cur, true)
	}
	list := make([]map[string]interface{}, 0, len(pins))
	for _, p := range pins {
		o := map[string]interface{}{
			"id": p.ID, "x": p.X, "y": p.Y, "name": p.Name, "description": p.Description, "icon": p.Icon, "color": p.Color,
			"linked": nil, "created_by_user": journalUser(users, p.CreatedBy), "created_at": p.CreatedAt,
		}
		if g, ok := pages[p.LinkedPageID]; ok && p.LinkedPageID != "" {
			o["name"] = g.Title
			o["linked"] = map[string]interface{}{"id": g.ID, "title": g.Title, "folder_id": g.FolderID, "excerpt": graphExcerpt(g.Body)}
		}
		list = append(list, o)
	}
	return map[string]interface{}{"image": image, "pins": list, "scale": scale}, nil
}

// replyMap answers with the map's current state (plus extra) after a change.
func replyMap(w http.ResponseWriter, status int, id string, extra map[string]interface{}) {
	codexID, err := db.GetPageCodexID(id)
	if err != nil {
		codexServerError(w, "retrieve page")
		return
	}
	payload, err := codexMapPayload(id, codexID)
	if err != nil {
		codexServerError(w, "retrieve map")
		return
	}
	out := map[string]interface{}{"map": payload}
	for k, v := range extra {
		out[k] = v
	}
	writeJSON(w, status, out)
}

func handleGetMap(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, false) {
		return
	}
	if kind, err := pageKind(id); err != nil {
		codexServerError(w, "retrieve page")
		return
	} else if kind != models.PageMap {
		codexMapError(w, db.ErrNotMap, "")
		return
	}
	replyMap(w, http.StatusOK, id, nil)
}

// codexOnMaps lists the maps that pin the page, for its side panel.
func codexOnMaps(pageID string) ([]map[string]interface{}, error) {
	pins, err := db.GetPinsLinkingTo(pageID)
	if err != nil {
		return nil, err
	}
	out := []map[string]interface{}{}
	titles := map[string]string{}
	for _, p := range pins {
		if _, seen := titles[p.MapPageID]; !seen {
			m, err := db.GetPage(p.MapPageID)
			if err != nil {
				return nil, err
			}
			if m == nil {
				continue
			}
			titles[p.MapPageID] = m.Title
		}
		out = append(out, map[string]interface{}{"map": map[string]interface{}{"id": p.MapPageID, "title": titles[p.MapPageID]}, "pin_id": p.ID})
	}
	return out, nil
}

func handleListMapImages(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, false) {
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

// handleUploadMapImage takes the raw image as the request body (PNG, JPEG, WebP or GIF, 25 MB at most).
func handleUploadMapImage(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	// Check the page before reading the body.
	if kind, err := pageKind(id); err != nil {
		codexServerError(w, "retrieve page")
		return
	} else if kind != models.PageMap {
		codexMapError(w, db.ErrNotMap, "")
		return
	}
	file, cfg, ok := receiveCodexImage(w, r)
	if !ok {
		return
	}
	img := models.MapImage{ID: db.GenerateUUID(), PageID: id, File: file, Width: cfg.Width, Height: cfg.Height, UploadedBy: userID, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	cleared, err := db.AddMapImage(img)
	if err != nil {
		removeCodexFiles([]string{file})
		codexMapError(w, err, "save the image")
		return
	}
	replyMap(w, http.StatusCreated, id, map[string]interface{}{"scale_cleared": cleared})
}

// receiveCodexImage stores the request body as an image in the upload directory (PNG, JPEG, WebP
// or GIF, 25 MB at most, sniffed rather than trusted) and returns its stored file name and size.
// On failure it has already written the error response.
func receiveCodexImage(w http.ResponseWriter, r *http.Request) (string, image.Config, bool) {
	// The server-wide timeouts suit small bodies; give this one room (unsupported writers ignore it).
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(uploadTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(uploadTimeout))
	tmp, err := db.CreateUploadTemp()
	if err != nil {
		log.Printf("failed to store image: %v", err)
		codexServerError(w, "store the image")
		return "", image.Config{}, false
	}
	size, err := io.Copy(tmp, http.MaxBytesReader(w, r.Body, maxMapImageBytes))
	if err != nil {
		db.DiscardUpload(tmp)
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "the image must be 25 MB or smaller")
			return "", image.Config{}, false
		}
		writeJSONError(w, http.StatusBadRequest, "failed to read the image")
		return "", image.Config{}, false
	}
	head := make([]byte, 512)
	n, _ := tmp.ReadAt(head, 0)
	ext, known := codexImageTypes[http.DetectContentType(head[:n])]
	if !known {
		db.DiscardUpload(tmp)
		writeJSONError(w, http.StatusBadRequest, "the image must be a PNG, JPEG, WebP or GIF file")
		return "", image.Config{}, false
	}
	cfg, _, err := image.DecodeConfig(io.NewSectionReader(tmp, 0, size))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxMapImageSide || cfg.Height > maxMapImageSide {
		db.DiscardUpload(tmp)
		writeJSONError(w, http.StatusBadRequest, "the image could not be read, or is larger than 16384 pixels on a side")
		return "", image.Config{}, false
	}
	if ext == ".gif" && !gifFramesFit(io.NewSectionReader(tmp, 0, size)) {
		db.DiscardUpload(tmp)
		writeJSONError(w, http.StatusBadRequest, "this GIF is damaged, has more than 1000 frames, or has a frame larger than the image")
		return "", image.Config{}, false
	}
	file := db.GenerateUUID() + ext
	if err := db.CommitUpload(tmp, file); err != nil {
		log.Printf("failed to store image: %v", err)
		codexServerError(w, "store the image")
		return "", image.Config{}, false
	}
	return file, cfg, true
}

// removeCodexFiles deletes stored map images and pictures after their page is gone; failures are only logged.
func removeCodexFiles(files []string) {
	dir, err := db.UploadDir()
	if err != nil {
		log.Printf("cannot remove images: %v", err)
		return
	}
	for _, f := range files {
		if !db.UploadFileRe.MatchString(f) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to remove image %s: %v", f, err)
		}
	}
}

func handleCodexFile(w http.ResponseWriter, r *http.Request, name string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok {
		return
	}
	if !db.UploadFileRe.MatchString(name) {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	if !rt.admin {
		// An image is visible to whoever can see the codex of the page it belongs to.
		codexID, err := db.GetImageCodexID(name)
		if err != nil {
			codexServerError(w, "read the image")
			return
		}
		if codexID == "" || !rt.canSee(codexID) {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
	}
	dir, err := db.UploadDir()
	if err != nil {
		codexServerError(w, "read the image")
		return
	}
	ctype := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".webp": "image/webp", ".gif": "image/gif"}[filepath.Ext(name)]
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

func handleRestoreMapImage(w http.ResponseWriter, r *http.Request, id, imageID string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	cleared, err := db.RestoreMapImage(id, imageID, userID)
	if err != nil {
		codexMapError(w, err, "restore the image")
		return
	}
	replyMap(w, http.StatusCreated, id, map[string]interface{}{"scale_cleared": cleared})
}

const (
	maxPinName  = 120
	maxPinDesc  = 1000
	maxUnitLen  = 20
	maxMapScale = 1e9
)

// handleSetMapScale takes {x1, y1, x2, y2, length, unit}: the two points are length units apart. DELETE clears it.
func handleSetMapScale(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	if r.Method == http.MethodDelete {
		if err := db.SetMapScale(id, nil, userID); err != nil {
			codexMapError(w, err, "clear the scale")
			return
		}
		replyMap(w, http.StatusOK, id, nil)
		return
	}
	var m models.MapScale
	if !codexDecode(w, r, 4<<10, &m) {
		return
	}
	m.Unit = strings.Join(strings.Fields(m.Unit), " ")
	for _, v := range []float64{m.X1, m.Y1, m.X2, m.Y2} {
		if v < 0 || v > 1 {
			writeJSONError(w, http.StatusBadRequest, "both points must be on the map")
			return
		}
	}
	if math.Hypot(m.X1-m.X2, m.Y1-m.Y2) < 0.005 {
		writeJSONError(w, http.StatusBadRequest, "the two points are too close together")
		return
	}
	if n := utf8.RuneCountInString(m.Unit); m.Length <= 0 || m.Length > maxMapScale || n < 1 || n > maxUnitLen {
		writeJSONError(w, http.StatusBadRequest, "enter a distance above 0 and a unit of up to 20 characters")
		return
	}
	if err := db.SetMapScale(id, &m, userID); err != nil {
		codexMapError(w, err, "set the scale")
		return
	}
	replyMap(w, http.StatusOK, id, nil)
}

// pinRequest is a new pin, or a change of one: either its place (x, y) or its details.
type pinRequest struct {
	X            *float64 `json:"x"`
	Y            *float64 `json:"y"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	LinkedPageID string   `json:"linked_page_id"`
	Icon         string   `json:"icon"`
	Color        string   `json:"color"`
	CreatePage   bool     `json:"create_page"` // add: make an article named after the pin first
}

// validPosition reports whether both coordinates are present and fractions of the image.
func (req *pinRequest) validPosition() bool {
	return req.X != nil && req.Y != nil && *req.X >= 0 && *req.X <= 1 && *req.Y >= 0 && *req.Y <= 1
}

// fill checks the details and puts them into p. The name of a linked pin is replaced by the store.
// It returns an error message, or "".
func (req *pinRequest) fill(p *models.MapPin) string {
	if !containsString(models.PinIcons, req.Icon) || !containsString(models.PinColors, req.Color) {
		return "unknown icon or colour"
	}
	p.Icon, p.Color, p.LinkedPageID = req.Icon, req.Color, req.LinkedPageID
	if req.LinkedPageID != "" {
		req.CreatePage = false
		return ""
	}
	p.Name = strings.Join(strings.Fields(req.Name), " ")
	p.Description = strings.TrimSpace(strings.ReplaceAll(req.Description, "\r\n", "\n"))
	if req.CreatePage {
		if _, ok := codexName(p.Name); !ok {
			return "title must be 1 to 120 characters and can't contain [ ] or |"
		}
	}
	if n := utf8.RuneCountInString(p.Name); n < 1 || n > maxPinName {
		return "name must be 1 to 120 characters"
	}
	if utf8.RuneCountInString(p.Description) > maxPinDesc {
		return "the description can be up to 1000 characters"
	}
	return ""
}

func handleAddPin(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req pinRequest
	if !codexDecode(w, r, 16<<10, &req) {
		return
	}
	if !req.validPosition() {
		writeJSONError(w, http.StatusBadRequest, "a pin needs a position on the map")
		return
	}
	pin := models.MapPin{MapPageID: id, X: *req.X, Y: *req.Y, CreatedBy: userID}
	if msg := req.fill(&pin); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	var page interface{}
	var err error
	if req.CreatePage {
		p, body, tags, ok := pinPage(w, id, pin)
		if !ok {
			return
		}
		pin, err = db.AddMapPinWithPage(pin, p, body, models.NewPageExtras{TagNames: tags})
		page = map[string]interface{}{"id": p.ID, "title": p.Title}
	} else {
		pin, err = db.AddMapPin(pin)
	}
	if err != nil {
		codexMapError(w, err, "add the pin")
		return
	}
	replyMap(w, http.StatusCreated, id, map[string]interface{}{"pin": map[string]interface{}{"id": pin.ID}, "page": page})
}

// pinPage is the article made for a new pin: the Settlement template for a settlement or city pin,
// else Landmark, under the pin's description. On failure it has already written the error response.
func pinPage(w http.ResponseWriter, mapID string, pin models.MapPin) (models.Page, string, []string, bool) {
	codexID, err := db.GetPageCodexID(mapID)
	if err != nil {
		codexServerError(w, "retrieve page")
		return models.Page{}, "", nil, false
	}
	lang, ok := codexLanguage(w, codexID)
	if !ok {
		return models.Page{}, "", nil, false
	}
	template := "builtin:landmark"
	if pin.Icon == "settlement" || pin.Icon == "city" {
		template = "builtin:settlement"
	}
	b := builtinTemplateByID(template, lang)
	body := fillTemplate(b.Body, pin.Name, codexToday(lang))
	if pin.Description != "" {
		text, ok := linkUserMentions(w, pin.Description)
		if !ok {
			return models.Page{}, "", nil, false
		}
		body = text + "\n\n" + body
	}
	return models.Page{ID: db.GenerateUUID(), Title: pin.Name, AuthorID: pin.CreatedBy}, body, b.Tags, true
}

// handleChangePin moves a pin ({x, y}) or changes its details (everything else).
func handleChangePin(w http.ResponseWriter, r *http.Request, id, pinID string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req pinRequest
	if !codexDecode(w, r, 16<<10, &req) {
		return
	}
	var event string
	var err error
	if req.X != nil || req.Y != nil {
		if !req.validPosition() {
			writeJSONError(w, http.StatusBadRequest, "a pin needs a position on the map")
			return
		}
		event, err = db.MoveMapPin(id, pinID, *req.X, *req.Y, userID)
	} else {
		req.CreatePage = false
		pin := models.MapPin{ID: pinID, MapPageID: id}
		if msg := req.fill(&pin); msg != "" {
			writeJSONError(w, http.StatusBadRequest, msg)
			return
		}
		event, err = db.UpdateMapPin(pin, userID)
	}
	if err != nil {
		codexMapError(w, err, "change the pin")
		return
	}
	replyMap(w, http.StatusOK, id, map[string]interface{}{"event_id": event})
}

func handleDeletePin(w http.ResponseWriter, r *http.Request, id, pinID string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	event, err := db.DeleteMapPin(id, pinID, userID)
	if err != nil {
		codexMapError(w, err, "remove the pin")
		return
	}
	replyMap(w, http.StatusOK, id, map[string]interface{}{"event_id": event})
}

func handlePinEvents(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, false) {
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
			"undone_by_user": undone, "name": e.Name,
		})
	}
	writeJSON(w, http.StatusOK, list)
}

func handleUndoPinEvent(w http.ResponseWriter, r *http.Request, id, eventID string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	if err := db.UndoMapPinEvent(id, eventID, userID); err != nil {
		codexMapError(w, err, "undo the pin change")
		return
	}
	replyMap(w, http.StatusOK, id, nil)
}

// copyMapInto puts the image (as a new file), scale and pins of a map into the extras of its copy and
// returns the new files; on error the caller removes them. In another codex the pins lose their links.
func copyMapInto(x *models.NewPageExtras, src *models.Page, sameCodex bool, userID string) ([]string, error) {
	images, err := db.GetMapImages(src.ID)
	if err != nil {
		return nil, err
	}
	c := &models.MapCopy{}
	if c.Scale, err = db.GetMapScale(src.ID); err != nil {
		return nil, err
	}
	if c.Pins, err = db.GetMapPins(src.ID); err != nil {
		return nil, err
	}
	if !sameCodex {
		for i, p := range c.Pins {
			if p.LinkedPageID == "" {
				continue
			}
			if page, err := db.GetPage(p.LinkedPageID); err != nil {
				return nil, err
			} else if page != nil {
				c.Pins[i].Name = page.Title
			}
			c.Pins[i].LinkedPageID = ""
		}
	}
	x.Map = c
	cur, _ := currentMapImage(images)
	if cur == nil {
		return nil, nil
	}
	file, err := copyUploadFile(cur.File)
	if os.IsNotExist(err) {
		log.Printf("codex copy page: map image %s is missing, not copied", cur.File)
		c.Scale, c.Pins = nil, nil
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cur.ID, cur.File, cur.UploadedBy, cur.CreatedAt = db.GenerateUUID(), file, userID, time.Now().UTC().Format(time.RFC3339)
	c.Image = cur
	return []string{file}, nil
}

const maxGIFFrames = 1000

// gifFramesFit walks the blocks of a GIF without decompressing anything and reports whether every
// frame sits inside the logical screen, the screen is within the side limit and there are at most
// maxGIFFrames frames. image.DecodeConfig only reads the screen size, so a frame could otherwise
// declare a huge size that a browser would try to allocate. A truncated file doesn't fit.
func gifFramesFit(src io.Reader) bool {
	r := bufio.NewReader(src)
	skip := func(n int) bool { _, err := r.Discard(n); return err == nil }
	u16 := func(b []byte) int { return int(b[0]) | int(b[1])<<8 }
	head := make([]byte, 13)
	if _, err := io.ReadFull(r, head); err != nil {
		return false
	}
	sw, sh := u16(head[6:]), u16(head[8:])
	if sw < 1 || sh < 1 || sw > maxMapImageSide || sh > maxMapImageSide {
		return false
	}
	if head[10]&0x80 != 0 && !skip(3<<((head[10]&7)+1)) {
		return false
	}
	subBlocks := func() bool {
		for {
			n, err := r.ReadByte()
			if err != nil {
				return false
			}
			if n == 0 {
				return true
			}
			if !skip(int(n)) {
				return false
			}
		}
	}
	frames := 0
	for {
		b, err := r.ReadByte()
		if err != nil {
			return false
		}
		switch b {
		case 0x3B:
			return true
		case 0x21:
			if !skip(1) || !subBlocks() {
				return false
			}
		case 0x2C:
			d := make([]byte, 9)
			if _, err := io.ReadFull(r, d); err != nil {
				return false
			}
			if frames++; frames > maxGIFFrames || u16(d[0:])+u16(d[4:]) > sw || u16(d[2:])+u16(d[6:]) > sh {
				return false
			}
			if d[8]&0x80 != 0 && !skip(3<<((d[8]&7)+1)) {
				return false
			}
			if !skip(1) || !subBlocks() {
				return false
			}
		default:
			return false
		}
	}
}
