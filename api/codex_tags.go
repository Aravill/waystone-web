package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

const (
	maxTagNameRunes = 40
	maxPageTags     = 50
)

// codexTagName validates a tag name: 1-40 characters, no line breaks. Runs of spaces collapse.
func codexTagName(s string) (string, bool) {
	if strings.ContainsAny(s, "\r\n") {
		return "", false
	}
	s = strings.Join(strings.Fields(s), " ")
	if n := utf8.RuneCountInString(s); n < 1 || n > maxTagNameRunes {
		return "", false
	}
	return s, true
}

const tagNameMessage = "a tag name must be 1 to 40 characters on one line"

func codexTagObjects(tags []models.CodexTag, counts bool) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(tags))
	for _, t := range tags {
		o := map[string]interface{}{"id": t.ID, "name": t.Name}
		if counts {
			o["page_count"] = t.PageCount
		}
		out = append(out, o)
	}
	return out
}

func codexTagError(w http.ResponseWriter, err error, what string) {
	switch {
	case errors.Is(err, db.ErrTagNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, db.ErrTagNameTaken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "tag_name_taken"})
	default:
		codexStoreError(w, err, what)
	}
}

// handleCodexTags serves /api/codices/c/{id}/tags: GET lists the tags with page counts, POST adds one.
func handleCodexTags(w http.ResponseWriter, r *http.Request, codexID string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkCodex(w, codexID, r.Method == http.MethodPost) {
		return
	}
	if r.Method == http.MethodGet {
		tags, err := db.GetCodexTags(codexID)
		if err != nil {
			codexServerError(w, "retrieve tags")
			return
		}
		writeJSON(w, http.StatusOK, codexTagObjects(tags, true))
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	name, valid := codexTagName(req.Name)
	if !valid {
		writeJSONError(w, http.StatusBadRequest, tagNameMessage)
		return
	}
	tag, err := db.CreateCodexTag(codexID, name, userID)
	if err != nil {
		codexTagError(w, err, "create tag")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"tag": map[string]interface{}{"id": tag.ID, "name": tag.Name, "page_count": 0}})
}

// handleCodexTag serves /api/codices/tags/{id}: PUT renames, DELETE removes the tag from its pages.
func handleCodexTag(w http.ResponseWriter, r *http.Request, tagID string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkTag(w, tagID, true) {
		return
	}
	if r.Method == http.MethodDelete {
		n, err := db.DeleteCodexTag(tagID)
		if err != nil {
			codexTagError(w, err, "delete tag")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "success", "pages_removed": n})
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	name, valid := codexTagName(req.Name)
	if !valid {
		writeJSONError(w, http.StatusBadRequest, tagNameMessage)
		return
	}
	if err := db.RenameCodexTag(tagID, name); err != nil {
		codexTagError(w, err, "rename tag")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tag": map[string]interface{}{"id": tagID, "name": name}})
}

// handleSetPageTags sets a page's tags by name, creating the ones its codex lacks. Not a revision.
func handleSetPageTags(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req struct {
		Names []string `json:"names"`
	}
	if !codexDecode(w, r, 16<<10, &req) {
		return
	}
	if len(req.Names) > maxPageTags {
		writeJSONError(w, http.StatusBadRequest, "a page can have at most 50 tags")
		return
	}
	names, seen := []string{}, map[string]bool{}
	for _, n := range req.Names {
		n, valid := codexTagName(n)
		if !valid {
			writeJSONError(w, http.StatusBadRequest, tagNameMessage)
			return
		}
		if k := db.NormName(n); !seen[k] {
			seen[k] = true
			names = append(names, n)
		}
	}
	tags, err := db.SetPageTags(id, names, userID)
	if err != nil {
		codexTagError(w, err, "save tags")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tags": codexTagObjects(tags, false)})
}
