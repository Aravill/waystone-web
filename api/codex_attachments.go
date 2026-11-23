package api

import (
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"waystone-web/db"
	"waystone-web/models"
)

const (
	maxAttachmentBytes = 100 << 20
	// attachmentTimeout is how long the slowest accepted upload, download or copy (100 MB) may take: about 0.93 Mbit/s.
	attachmentTimeout = 15 * time.Minute
)

// attachmentInline lists the types that open in the browser and the type they are served as; every other
// allowed type downloads as application/octet-stream.
var attachmentInline = map[string]string{".pdf": "application/pdf", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".gif": "image/gif"}

// attachmentOtherTypes are the allowed extensions that download.
var attachmentOtherTypes = map[string]bool{".docx": true, ".doc": true, ".xlsx": true, ".xls": true, ".pptx": true, ".ppt": true,
	".odt": true, ".ods": true, ".odp": true, ".txt": true, ".md": true, ".csv": true, ".zip": true, ".mp3": true}

// attachmentName turns an uploaded file name into the name a page refers to it by in ![[name]]: like a picture's
// name (see cleanFileName) with the extension kept as typed, at most 80 characters in all. ext is lower case.
// ok is false when the extension is not allowed.
func attachmentName(raw string) (name, ext string, ok bool) {
	raw = strings.TrimSpace(raw)
	if i := strings.LastIndexAny(raw, `/\`); i >= 0 {
		raw = raw[i+1:]
	}
	typed := filepath.Ext(raw)
	ext = strings.ToLower(typed)
	if _, inline := attachmentInline[ext]; !inline && !attachmentOtherTypes[ext] {
		return "", ext, false
	}
	base := cleanFileName(strings.TrimSuffix(raw, typed))
	if base == "" {
		base = "file"
	}
	if room := maxPictureNameRunes - len([]rune(typed)); len([]rune(base)) > room {
		base = strings.TrimRight(string([]rune(base)[:room]), ". ")
		if base == "" {
			base = "file"
		}
	}
	return base + typed, ext, true
}

func attachmentURL(a models.PageAttachment) string {
	return "/api/codices/attachments/" + a.ID + "/" + url.PathEscape(a.Name)
}

func attachmentObject(users map[string]*models.User, a models.PageAttachment) map[string]interface{} {
	_, inline := attachmentInline[strings.ToLower(filepath.Ext(a.Name))]
	return map[string]interface{}{
		"id": a.ID, "name": a.Name, "size": a.Size, "url": attachmentURL(a), "inline": inline,
		"uploaded_by_user": journalUser(users, a.UploadedBy), "created_at": a.CreatedAt,
	}
}

// codexAddAttachmentPayload adds a page's attachments (oldest first) to its payload.
func codexAddAttachmentPayload(payload map[string]interface{}, pageID string) error {
	list, err := db.GetPageAttachments(pageID)
	if err != nil {
		return err
	}
	users, err := journalUsers()
	if err != nil {
		return err
	}
	out := []map[string]interface{}{}
	for _, a := range list {
		out = append(out, attachmentObject(users, a))
	}
	payload["attachments"] = out
	return nil
}

func attachmentCodeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

// removeAttachmentFiles deletes stored attachment files after their row or page is gone; failures are only logged.
func removeAttachmentFiles(files []string) {
	dir, err := db.AttachmentDir()
	if err != nil {
		log.Printf("cannot remove attachments: %v", err)
		return
	}
	for _, f := range files {
		if !db.AttachmentFileRe.MatchString(f) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to remove attachment %s: %v", f, err)
		}
	}
}

// receiveAttachment stores the request body in the attachments directory under a new random name (100 MB at most,
// content checked against ext) and returns that name and the size. On failure it has already written the error response.
func receiveAttachment(w http.ResponseWriter, r *http.Request, ext string) (file string, size int64, ok bool) {
	// The server-wide timeouts suit small bodies; give this one room (unsupported writers ignore it).
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(attachmentTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(attachmentTimeout))
	tooLarge := func() {
		attachmentCodeError(w, http.StatusRequestEntityTooLarge, "too_large", "the file must be 100 MB or smaller")
	}
	if r.ContentLength > maxAttachmentBytes {
		tooLarge()
		return "", 0, false
	}
	tmp, err := db.CreateAttachmentTemp()
	if err != nil {
		log.Printf("failed to store attachment: %v", err)
		codexServerError(w, "store the file")
		return "", 0, false
	}
	size, err = io.Copy(tmp, http.MaxBytesReader(w, r.Body, maxAttachmentBytes))
	if err != nil {
		db.DiscardUpload(tmp)
		var tooBig *http.MaxBytesError
		var pathErr *os.PathError
		switch {
		case errors.As(err, &tooBig):
			tooLarge()
		case errors.As(err, &pathErr):
			log.Printf("failed to store attachment: %v", err)
			codexServerError(w, "store the file")
		default:
			writeJSONError(w, http.StatusBadRequest, "failed to read the file")
		}
		return "", 0, false
	}
	if !attachmentContentOK(tmp, size, ext) {
		db.DiscardUpload(tmp)
		attachmentCodeError(w, http.StatusBadRequest, "content_mismatch", "the file is empty or is not what its extension says")
		return "", 0, false
	}
	file = db.GenerateUUID()
	if err := db.CommitUpload(tmp, file); err != nil {
		log.Printf("failed to store attachment: %v", err)
		codexServerError(w, "store the file")
		return "", 0, false
	}
	return file, size, true
}

func attachmentNotFound(w http.ResponseWriter) {
	attachmentCodeError(w, http.StatusNotFound, "attachment_not_found", db.ErrAttachmentNotFound.Error())
}

func attachmentError(w http.ResponseWriter, err error, what string) {
	if errors.Is(err, db.ErrAttachmentNotFound) {
		attachmentNotFound(w)
		return
	}
	codexStoreError(w, err, what)
}

// handleCodexAttachmentRoutes serves /api/codices/pages/{id}/attachments[/{attId}]. It reports false when rest is
// not one of them.
func handleCodexAttachmentRoutes(w http.ResponseWriter, r *http.Request, id string, rest []string, method func(...string) bool) bool {
	switch {
	case rest[0] == "attachments" && len(rest) == 1:
		if method(http.MethodPost) {
			handleUploadAttachment(w, r, id)
		}
	case rest[0] == "attachments" && len(rest) == 2:
		if method(http.MethodPut, http.MethodDelete) {
			if r.Method == http.MethodPut {
				handleReplaceAttachment(w, r, id, rest[1])
			} else {
				handleRemoveAttachment(w, r, id, rest[1])
			}
		}
	default:
		return false
	}
	return true
}

// fileNameHeader is the original file name, URL-encoded, in X-File-Name.
func fileNameHeader(r *http.Request) string {
	raw, err := url.PathUnescape(r.Header.Get("X-File-Name"))
	if err != nil {
		return ""
	}
	return raw
}

// handleUploadAttachment takes the raw file as the request body and the original file name in X-File-Name.
func handleUploadAttachment(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	name, ext, ok := attachmentName(fileNameHeader(r))
	if !ok {
		attachmentCodeError(w, http.StatusBadRequest, "type_not_allowed", "this type of file can't be attached")
		return
	}
	file, size, ok := receiveAttachment(w, r, ext)
	if !ok {
		return
	}
	a, err := db.AddPageAttachment(models.PageAttachment{ID: db.GenerateUUID(), PageID: id, Name: name, File: file, Size: size,
		UploadedBy: userID, CreatedAt: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		removeAttachmentFiles([]string{file})
		attachmentError(w, err, "save the file")
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"attachment": attachmentObject(users, a)})
}

// handleReplaceAttachment swaps the file behind an attachment (same id, name and URL; the extension must match).
func handleReplaceAttachment(w http.ResponseWriter, r *http.Request, id, attID string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	cur, _, err := db.GetAttachment(attID)
	if err != nil {
		codexServerError(w, "read the file")
		return
	}
	if cur.ID == "" || cur.PageID != id {
		attachmentNotFound(w)
		return
	}
	raw := fileNameHeader(r)
	if i := strings.LastIndexAny(raw, `/\`); i >= 0 {
		raw = raw[i+1:]
	}
	ext, want := strings.ToLower(filepath.Ext(strings.TrimSpace(raw))), strings.ToLower(filepath.Ext(cur.Name))
	if ext != want {
		attachmentCodeError(w, http.StatusBadRequest, "wrong_extension", "the new file must be a "+want+" file")
		return
	}
	file, size, ok := receiveAttachment(w, r, ext)
	if !ok {
		return
	}
	a, old, err := db.ReplacePageAttachment(id, attID, file, size, userID)
	if err != nil {
		removeAttachmentFiles([]string{file})
		attachmentError(w, err, "replace the file")
		return
	}
	// The old file goes only now that the row points at the stored new one.
	removeAttachmentFiles([]string{old})
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"attachment": attachmentObject(users, a)})
}

func handleRemoveAttachment(w http.ResponseWriter, r *http.Request, id, attID string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	file, err := db.RemovePageAttachment(id, attID)
	if err != nil {
		attachmentError(w, err, "remove the file")
		return
	}
	removeAttachmentFiles([]string{file})
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// handleAttachmentFile serves GET/HEAD /api/codices/attachments/{id}[/{name}] to whoever can see the page's codex.
// The name is ignored; it only gives browser tabs a readable title. The URL survives Replace, so the response is
// revalidated (no-cache) and the ETag is the stored file's name, which changes on Replace.
func handleAttachmentFile(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok {
		return
	}
	a, codexID, err := db.GetAttachment(id)
	if err != nil {
		codexServerError(w, "read the file")
		return
	}
	if codexID == "" || !rt.canSee(codexID) || !db.AttachmentFileRe.MatchString(a.File) {
		attachmentNotFound(w)
		return
	}
	dir, err := db.AttachmentDir()
	if err != nil {
		codexServerError(w, "read the file")
		return
	}
	f, err := os.Open(filepath.Join(dir, a.File))
	if err != nil {
		attachmentNotFound(w)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		attachmentNotFound(w)
		return
	}
	// A download of the largest file needs more than the server-wide write timeout.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(attachmentTimeout))
	ext := strings.ToLower(filepath.Ext(a.Name))
	ctype, disp := attachmentInline[ext], "inline"
	if ctype == "" {
		ctype, disp = "application/octet-stream", "attachment"
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	if cd := mime.FormatMediaType(disp, map[string]string{"filename": a.Name}); cd != "" {
		h.Set("Content-Disposition", cd)
	} else {
		h.Set("Content-Disposition", disp)
	}
	h.Set("X-Content-Type-Options", "nosniff")
	// No CSP on a PDF: browsers show it in their own viewer, and a policy can break that viewer.
	if ext != ".pdf" {
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	}
	h.Set("Cache-Control", "private, no-cache")
	h.Set("ETag", strconv.Quote(a.File))
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

// handleCodexAttachments lists every attachment on the codex's pages, newest first, with the page each is on and
// the total size.
func handleCodexAttachments(w http.ResponseWriter, r *http.Request, codexID string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkCodex(w, codexID, false) {
		return
	}
	list, err := db.GetCodexAttachments(codexID)
	if err != nil {
		codexServerError(w, "retrieve attachments")
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	out := []map[string]interface{}{}
	var total int64
	for _, a := range list {
		o := attachmentObject(users, a.PageAttachment)
		o["page"] = map[string]string{"id": a.PageID, "title": a.PageTitle}
		out = append(out, o)
		total += a.Size
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"attachments": out, "total_size": total})
}
