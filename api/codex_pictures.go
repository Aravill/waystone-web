package api

import (
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"waystone-web/db"
	"waystone-web/models"
)

const maxPictureNameRunes = 80

// pictureName turns an uploaded file name into the name a page refers to it by in ![[name|caption]]:
// no path, letters, digits, spaces and - _ . only (so no [ ] |), at most 80 characters.
func pictureName(raw string) string {
	if i := strings.LastIndexAny(raw, `/\`); i >= 0 {
		raw = raw[i+1:]
	}
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == ' ' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	name := strings.TrimLeft(strings.Join(strings.Fields(b.String()), " "), ". ")
	if r := []rune(name); len(r) > maxPictureNameRunes {
		ext := filepath.Ext(name)
		if len([]rune(ext)) > 8 {
			ext = ""
		}
		name = string(r[:maxPictureNameRunes-len([]rune(ext))]) + ext
	}
	if name == "" {
		return "image"
	}
	return name
}

func pictureObject(users map[string]*models.User, img models.PageImage) map[string]interface{} {
	return map[string]interface{}{
		"id": img.ID, "name": img.Name, "url": codexImageURL(img.File), "width": img.Width, "height": img.Height,
		"uploaded_by_user": journalUser(users, img.UploadedBy), "created_at": img.CreatedAt,
	}
}

// codexAddPicturePayload adds a page's pictures to its payload: images (the current ones),
// image_index (every picture ever uploaded, by name, so ![[name]] in the text and in old
// revisions resolves) and cover.
func codexAddPicturePayload(payload map[string]interface{}, pageID string) error {
	all, coverID, err := db.GetPageImages(pageID)
	if err != nil {
		return err
	}
	users, err := journalUsers()
	if err != nil {
		return err
	}
	current := []map[string]interface{}{}
	index := map[string]interface{}{}
	var cover interface{}
	for _, img := range all {
		index[img.Name] = map[string]interface{}{"name": img.Name, "url": codexImageURL(img.File), "width": img.Width, "height": img.Height}
		if img.RemovedAt != "" {
			continue
		}
		current = append(current, pictureObject(users, img))
		if img.ID == coverID {
			cover = map[string]interface{}{"id": img.ID, "name": img.Name, "url": codexImageURL(img.File), "width": img.Width, "height": img.Height}
		}
	}
	payload["images"], payload["image_index"], payload["cover"] = current, index, cover
	return nil
}

func codexPictureError(w http.ResponseWriter, err error, what string) {
	if errors.Is(err, db.ErrImageNotFound) {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	codexStoreError(w, err, what)
}

// handleCodexPictureRoutes serves /api/codices/pages/{id}/{pictures|cover}. It reports false when
// rest is not one of them. (The page's map has its own routes under "images", see codex_maps.go.)
func handleCodexPictureRoutes(w http.ResponseWriter, r *http.Request, id string, rest []string, method func(...string) bool) bool {
	switch {
	case rest[0] == "pictures" && len(rest) == 1:
		if method(http.MethodGet, http.MethodPost) {
			if r.Method == http.MethodGet {
				handleListPictures(w, r, id)
			} else {
				handleUploadPicture(w, r, id)
			}
		}
	case rest[0] == "pictures" && len(rest) == 2:
		if method(http.MethodDelete) {
			handleRemovePicture(w, r, id, rest[1])
		}
	case rest[0] == "cover" && len(rest) == 1:
		if method(http.MethodPut) {
			handleSetCover(w, r, id)
		}
	default:
		return false
	}
	return true
}

func handleListPictures(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, false) {
		return
	}
	payload := map[string]interface{}{}
	if err := codexAddPicturePayload(payload, id); err != nil {
		codexServerError(w, "retrieve images")
		return
	}
	writeJSON(w, http.StatusOK, payload["images"])
}

// handleUploadPicture takes the raw image as the request body (like a map image) and the original
// file name, URL-encoded, in X-File-Name.
func handleUploadPicture(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	raw, err := url.PathUnescape(r.Header.Get("X-File-Name"))
	if err != nil {
		raw = ""
	}
	file, cfg, ok := receiveCodexImage(w, r)
	if !ok {
		return
	}
	img := models.PageImage{ID: db.GenerateUUID(), PageID: id, Name: pictureName(raw), File: file, Width: cfg.Width, Height: cfg.Height,
		UploadedBy: userID, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if img, err = db.AddPageImage(img); err != nil {
		removeCodexFiles([]string{file})
		codexPictureError(w, err, "save the image")
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"image": pictureObject(users, img)})
}

func handleRemovePicture(w http.ResponseWriter, r *http.Request, id, imageID string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	if err := db.RemovePageImage(id, imageID); err != nil {
		codexPictureError(w, err, "remove the image")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func handleSetCover(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req struct {
		ImageID string `json:"image_id"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	if err := db.SetPageCover(id, req.ImageID); err != nil {
		codexPictureError(w, err, "set the cover")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}
