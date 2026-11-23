package api

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

// Quick notes: private to their author, one inbox per user per codex. Listing and deleting need
// See on the codex (someone demoted to See can still clear their inbox); adding, editing and
// filing need Edit. Someone else's note is a 404, never a 403.

const maxNoteRunes = 5000

func noteObject(n models.CodexNote) map[string]interface{} {
	return map[string]interface{}{"id": n.ID, "codex_id": n.CodexID, "body": n.Body, "created_at": n.CreatedAt}
}

// noteBody trims and validates a note's text; ok is false (with the response written) when it is
// empty or too long.
func noteBody(w http.ResponseWriter, s string) (string, bool) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n < 1 || n > maxNoteRunes {
		writeJSONError(w, http.StatusBadRequest, "a note must be 1 to 5000 characters")
		return "", false
	}
	return s, true
}

func noteNotFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": db.ErrNoteNotFound.Error(), "code": "note_not_found"})
}

// ownNote loads the note when it is the user's and they can still see its codex; otherwise the
// 404 is written. With edit it also needs Edit on the codex (403).
func ownNote(w http.ResponseWriter, rt *codexRights, userID, id string, edit bool) (*models.CodexNote, bool) {
	n, err := db.GetCodexNote(id)
	if err != nil {
		codexServerError(w, "retrieve note")
		return nil, false
	}
	if n == nil || n.UserID != userID || !rt.canSee(n.CodexID) {
		noteNotFound(w)
		return nil, false
	}
	if edit && !rt.canEdit(n.CodexID) {
		writeJSONError(w, http.StatusForbidden, "you can't edit this codex")
		return nil, false
	}
	return n, true
}

// codexNoteError writes the response for the errors the note store returns.
func codexNoteError(w http.ResponseWriter, err error, what string) {
	switch {
	case errors.Is(err, db.ErrNoteNotFound):
		noteNotFound(w)
	case errors.Is(err, db.ErrInboxFull):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "inbox_full"})
	case errors.Is(err, db.ErrNoteTooLong):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	default:
		codexStoreError(w, err, what)
	}
}

type noteRequest struct {
	Body   string `json:"body"`
	PageID string `json:"page_id"`
}

// handleCodexNotes serves GET (the caller's own notes, newest first) and POST (add one) on
// /api/codices/c/{id}/notes.
func handleCodexNotes(w http.ResponseWriter, r *http.Request, codexID string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkCodex(w, codexID, r.Method == http.MethodPost) {
		return
	}
	if r.Method == http.MethodGet {
		notes, err := db.GetCodexNotes(codexID, userID)
		if err != nil {
			codexServerError(w, "retrieve notes")
			return
		}
		out := make([]map[string]interface{}, 0, len(notes))
		for _, n := range notes {
			out = append(out, noteObject(n))
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"notes": out})
		return
	}
	var req noteRequest
	if !codexDecode(w, r, 64<<10, &req) {
		return
	}
	body, valid := noteBody(w, req.Body)
	if !valid {
		return
	}
	n := models.CodexNote{ID: db.GenerateUUID(), CodexID: codexID, UserID: userID, Body: body, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := db.CreateCodexNote(n); err != nil {
		codexNoteError(w, err, "save note")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"note": noteObject(n)})
}

// handleCodexNoteRoutes serves /api/codices/notes/{id} (PUT, DELETE) and .../append (POST).
func handleCodexNoteRoutes(w http.ResponseWriter, r *http.Request, parts []string, method func(...string) bool) {
	switch {
	case len(parts) == 1 && parts[0] != "":
		if method(http.MethodPut, http.MethodDelete) {
			if r.Method == http.MethodPut {
				handleUpdateNote(w, r, parts[0])
			} else {
				handleDeleteNote(w, r, parts[0])
			}
		}
	case len(parts) == 2 && parts[1] == "append":
		if method(http.MethodPost) {
			handleAppendNote(w, r, parts[0])
		}
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

func handleUpdateNote(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok {
		return
	}
	if _, ok := ownNote(w, rt, userID, id, true); !ok {
		return
	}
	var req noteRequest
	if !codexDecode(w, r, 64<<10, &req) {
		return
	}
	body, valid := noteBody(w, req.Body)
	if !valid {
		return
	}
	if err := db.UpdateCodexNote(id, body); err != nil {
		codexNoteError(w, err, "save note")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func handleDeleteNote(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok {
		return
	}
	if _, ok := ownNote(w, rt, userID, id, false); !ok {
		return
	}
	if err := db.DeleteCodexNote(id); err != nil {
		codexNoteError(w, err, "delete note")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// noteSignature is the line that opens an appended note: the date and who wrote it, in italics.
func noteSignature(name, lang string) string {
	return "_" + codexToday(lang) + " · " + name + "_"
}

// handleAppendNote adds the note to the end of a page of its codex (a revision noted "Added a
// quick note") and deletes it, in one transaction.
func handleAppendNote(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok {
		return
	}
	note, ok := ownNote(w, rt, userID, id, true)
	if !ok {
		return
	}
	var req noteRequest
	if !codexDecode(w, r, 16<<10, &req) {
		return
	}
	pageCodex, err := db.GetPageCodexID(req.PageID)
	if err != nil {
		codexServerError(w, "retrieve page")
		return
	}
	if pageCodex == "" {
		codexStoreError(w, db.ErrPageNotFound, "")
		return
	}
	if pageCodex != note.CodexID {
		writeJSONError(w, http.StatusBadRequest, "the page is in another codex")
		return
	}
	name := userID
	if u, err := db.GetUserByID(userID); err == nil && u != nil {
		name = u.DisplayName()
	}
	text, ok := linkUserMentions(w, note.Body)
	if !ok {
		return
	}
	lang, ok := codexLanguage(w, note.CodexID)
	if !ok {
		return
	}
	rev, err := db.AppendNoteToPage(id, userID, req.PageID, noteSignature(name, lang)+"\n"+text, maxPageBodyRunes)
	if err != nil {
		codexNoteError(w, err, "add note")
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"revision": codexRevisionObject(users, rev)})
}
