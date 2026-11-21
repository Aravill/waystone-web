package api

import (
	"errors"
	"fmt"
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
)

const (
	maxTemplateNameRunes  = 80
	maxTemplateDescRunes  = 160
	builtinTemplatePrefix = "builtin:"
)

// builtinTemplate is a template defined here: the same in every codex, never stored, so the server
// can apply it and the client shows the same text. Tags are names, created in the codex on use.
type builtinTemplate struct {
	ID, Name, Description string
	Tags                  []string
	Body                  string
}

var builtinTemplates = []builtinTemplate{
	{"blank", "Blank page", "Start from nothing", []string{}, ""},
	{"session", "Session", "Notes from one game session", []string{"session"},
		"**Date:** {{date}}\n**Who was there:** [[ ]], [[ ]]\n\n## Summary\n\n_What happened, in a line or two._\n\n## What happened\n\n1. \n\n## People met\n\n- [[ ]]\n\n## Loot and rewards\n\n- \n\n## Loose ends\n\n- \n\n## Next time\n\n"},
	{"character", "Character", "A person, player or not", []string{"npc"},
		"_Who {{title}} is, in one line._\n\n**Lives in:** [[ ]]\n**Belongs to:** [[ ]]\n\n## Appearance\n\n## Personality\n\n## Background\n\n## Relationships\n\n- [[ ]]: \n\n## Notes\n\n"},
	{"settlement", "Settlement", "A village, town or city", []string{"settlement"},
		"_What {{title}} is, in one line._\n\n**Population:** \n**Ruled by:** [[ ]]\n**Region:** [[ ]]\n\n## Places of note\n\n- [[ ]]\n\n## People of note\n\n- [[ ]]\n\n## History\n\n## Rumours\n\n- \n"},
	{"landmark", "Landmark", "A place worth a page of its own", []string{"landmark"},
		"_What {{title}} is and where it stands, in one line._\n\n**Near:** [[ ]]\n\n## Description\n\n## History\n\n## Dangers\n\n## Rumours\n\n- \n"},
	{"event", "Event", "Something that happened: a battle, a fire, a wedding", []string{"event"},
		"_What happened, in one line._\n\n**When:** \n**Where:** [[ ]]\n**Who:** [[ ]], [[ ]]\n\n## What happened\n\n## Why it matters\n\n## What came of it\n\n- \n"},
	{"faction", "Faction", "A guild, cult, family or army", []string{"faction"},
		"_Who {{title}} are, in one line._\n\n**Led by:** [[ ]]\n**Based in:** [[ ]]\n\n## Goals\n\n## Members\n\n- [[ ]]\n\n## Allies and enemies\n\n- \n\n## Resources\n\n"},
	{"item", "Item", "An object, relic or treasure", []string{"item"},
		"_What {{title}} is, in one line._\n\n**Held by:** [[ ]]\n\n## Appearance\n\n## What it does\n\n## History\n\n## Where it is now\n\n"},
	{"lore", "Lore", "History, legends, religion, customs", []string{"lore"},
		"_What this is about, in one line._\n\n## The story\n\n## What people believe\n\n## Sources\n\n- [[ ]]\n"},
}

// templateToday is the day {{date}} stands for, written the way the Codices mock-up shows dates.
func templateToday() string { return time.Now().UTC().Format("Jan 2, 2006") }

// fillTemplate applies the two placeholders in one pass, so a title that contains "{{date}}" stays as typed.
func fillTemplate(body, title string) string {
	return strings.NewReplacer("{{title}}", title, "{{date}}", templateToday()).Replace(body)
}

func builtinTemplateByID(id string) *builtinTemplate {
	if !strings.HasPrefix(id, builtinTemplatePrefix) {
		return nil
	}
	for i := range builtinTemplates {
		if builtinTemplates[i].ID == strings.TrimPrefix(id, builtinTemplatePrefix) {
			return &builtinTemplates[i]
		}
	}
	return nil
}

// handleBuiltinTemplates serves GET /api/codices/templates/builtin: the built-in templates and
// the date {{date}} stands for today.
func handleBuiltinTemplates(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]interface{}, 0, len(builtinTemplates))
	for _, t := range builtinTemplates {
		out = append(out, map[string]interface{}{
			"id": builtinTemplatePrefix + t.ID, "name": t.Name, "description": t.Description, "tags": t.Tags, "body": t.Body,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"templates": out, "today": templateToday()})
}

func codexTemplateObject(users map[string]*models.User, t models.CodexTemplate) map[string]interface{} {
	names := make([]string, 0, len(t.Tags))
	for _, tag := range t.Tags {
		names = append(names, tag.Name)
	}
	return map[string]interface{}{
		"id": t.ID, "codex_id": t.CodexID, "name": t.Name, "description": t.Description, "body": t.Body, "tags": names,
		"created_by_user": journalUser(users, t.CreatedBy), "created_at": t.CreatedAt, "updated_at": t.UpdatedAt,
	}
}

type templateRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Body        string   `json:"body"`
	Tags        []string `json:"tags"`
}

// normalize cleans the fields and returns an error message, or "" when they are fine.
func (req *templateRequest) normalize() string {
	req.Name = strings.Join(strings.Fields(req.Name), " ")
	if n := utf8.RuneCountInString(req.Name); n < 1 || n > maxTemplateNameRunes {
		return "a template name must be 1 to 80 characters"
	}
	req.Description = strings.Join(strings.Fields(req.Description), " ")
	if utf8.RuneCountInString(req.Description) > maxTemplateDescRunes {
		return "the description can be at most 160 characters"
	}
	if utf8.RuneCountInString(req.Body) > maxPageBodyRunes {
		return "body is too long"
	}
	if len(req.Tags) > maxPageTags {
		return "a template can add at most 50 tags"
	}
	names, seen := []string{}, map[string]bool{}
	for _, n := range req.Tags {
		n, valid := codexTagName(n)
		if !valid {
			return tagNameMessage
		}
		if k := db.NormName(n); !seen[k] {
			seen[k] = true
			names = append(names, n)
		}
	}
	req.Tags = names
	return ""
}

func codexTemplateError(w http.ResponseWriter, err error, what string) {
	switch {
	case errors.Is(err, db.ErrTemplateNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, db.ErrTemplateNameTaken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "template_name_taken"})
	default:
		codexStoreError(w, err, what)
	}
}

// handleCodexTemplates serves /api/codices/c/{id}/templates: GET lists the codex's own templates
// (See), POST adds one (Edit).
func handleCodexTemplates(w http.ResponseWriter, r *http.Request, codexID string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkCodex(w, codexID, r.Method == http.MethodPost) {
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	if r.Method == http.MethodGet {
		list, err := db.GetCodexTemplates(codexID)
		if err != nil {
			codexServerError(w, "retrieve templates")
			return
		}
		out := make([]map[string]interface{}, 0, len(list))
		for _, t := range list {
			out = append(out, codexTemplateObject(users, t))
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	var req templateRequest
	if !codexDecode(w, r, 512<<10, &req) {
		return
	}
	if msg := req.normalize(); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	t, err := db.CreateCodexTemplate(models.CodexTemplate{CodexID: codexID, Name: req.Name, Description: req.Description, Body: req.Body, CreatedBy: userID}, req.Tags)
	if err != nil {
		codexTemplateError(w, err, "create template")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"template": codexTemplateObject(users, t)})
}

// handleCodexTemplate serves /api/codices/templates/{id}: PUT updates, DELETE removes (Edit on its
// codex; a template the user can't see is a 404). Pages already made from it are not touched.
func handleCodexTemplate(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok {
		return
	}
	t, err := db.GetCodexTemplate(id)
	if err != nil {
		codexServerError(w, "retrieve template")
		return
	}
	if t == nil || !rt.canSee(t.CodexID) {
		writeJSONError(w, http.StatusNotFound, db.ErrTemplateNotFound.Error())
		return
	}
	if !rt.canEdit(t.CodexID) {
		writeJSONError(w, http.StatusForbidden, "you can't edit this codex")
		return
	}
	if r.Method == http.MethodDelete {
		if err := db.DeleteCodexTemplate(id); err != nil {
			codexTemplateError(w, err, "delete template")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
		return
	}
	var req templateRequest
	if !codexDecode(w, r, 512<<10, &req) {
		return
	}
	if msg := req.normalize(); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	if err := db.UpdateCodexTemplate(id, req.Name, req.Description, req.Body, req.Tags, userID); err != nil {
		codexTemplateError(w, err, "save template")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// pageStart is the text and tag names a new page starts with: from a template of the codex it is
// made in (built-in or the codex's own, placeholders applied), or the plain body. ok is false (with
// the response written) when the template doesn't exist in that codex.
func pageStart(w http.ResponseWriter, templateID, codexID, title, body string) (string, []string, bool) {
	if templateID == "" {
		return body, nil, true
	}
	if b := builtinTemplateByID(templateID); b != nil {
		return fillTemplate(b.Body, title), b.Tags, true
	}
	t, err := db.GetCodexTemplate(templateID)
	if err != nil {
		codexServerError(w, "retrieve template")
		return "", nil, false
	}
	if t == nil || t.CodexID != codexID {
		writeJSONError(w, http.StatusBadRequest, db.ErrTemplateNotFound.Error())
		return "", nil, false
	}
	names := make([]string, 0, len(t.Tags))
	for _, tag := range t.Tags {
		names = append(names, tag.Name)
	}
	return fillTemplate(t.Body, title), names, true
}

type pageCopyRequest struct {
	Title      string `json:"title"`
	CodexID    string `json:"codex_id"`
	FolderID   string `json:"folder_id"`
	CopyTags   bool   `json:"copy_tags"`
	CopyImages bool   `json:"copy_images"`
}

// handleCopyPage copies a page (See on its codex) into a codex the user can edit: a new page with
// its own history, revision 1 noted "Copied from <title>". Aliases aren't copied (they must be
// unique) and [[links]] stay as written, resolving in the target codex. With copy_tags the tags
// follow by name; with copy_images each current picture's file is duplicated under a new name (the
// picture names stay, so ![[name]] keeps working) and the cover is kept.
func handleCopyPage(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, false) {
		return
	}
	var req pageCopyRequest
	if !codexDecode(w, r, 16<<10, &req) {
		return
	}
	title, valid := codexName(req.Title)
	if !valid {
		writeJSONError(w, http.StatusBadRequest, "title must be 1 to 120 characters and can't contain [ ] or |")
		return
	}
	if req.CodexID == "" || !rt.canSee(req.CodexID) {
		writeJSONError(w, http.StatusBadRequest, db.ErrCodexNotFound.Error())
		return
	}
	if !rt.canEdit(req.CodexID) {
		writeJSONError(w, http.StatusForbidden, "you can't edit the target codex")
		return
	}
	src, err := db.GetPage(id)
	if err != nil {
		codexServerError(w, "retrieve page")
		return
	}
	if src == nil {
		codexStoreError(w, db.ErrPageNotFound, "")
		return
	}
	note := "Copied from " + src.Title
	if runes := []rune(note); len(runes) > maxPageNoteRunes {
		note = string(runes[:maxPageNoteRunes-1]) + "…"
	}
	x := models.NewPageExtras{Note: note, TagNames: []string{}}
	if req.CopyTags {
		tags, err := db.GetPageTags(id)
		if err != nil {
			codexServerError(w, "retrieve tags")
			return
		}
		for _, t := range tags {
			x.TagNames = append(x.TagNames, t.Name)
		}
	}
	page := models.Page{ID: db.GenerateUUID(), Kind: models.PageLore, CodexID: req.CodexID, FolderID: req.FolderID, Title: title, AuthorID: userID}
	var newFiles []string
	if req.CopyImages {
		if x.Images, x.CoverImageID, newFiles, err = duplicatePictures(id); err != nil {
			removeCodexFiles(newFiles)
			log.Printf("codex copy page: %v", err)
			codexServerError(w, "copy the images")
			return
		}
	}
	if err := db.CreatePageWith(page, src.Body, x); err != nil {
		removeCodexFiles(newFiles)
		codexStoreError(w, err, "copy page")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"page": map[string]interface{}{"id": page.ID, "codex_id": page.CodexID}})
}

// duplicatePictures copies the files of a page's current pictures under new random names and
// returns the new rows (same picture names, new ids), the new id of the cover (or "") and the new
// file names. A picture whose file is gone is left out. On error the caller removes the files.
func duplicatePictures(pageID string) ([]models.PageImage, string, []string, error) {
	all, coverID, err := db.GetPageImages(pageID)
	if err != nil {
		return nil, "", nil, err
	}
	dir, err := db.UploadDir()
	if err != nil {
		return nil, "", nil, err
	}
	var out []models.PageImage
	var files []string
	cover := ""
	for _, img := range all {
		if img.RemovedAt != "" || !db.UploadFileRe.MatchString(img.File) {
			continue
		}
		src, err := os.Open(filepath.Join(dir, img.File))
		if os.IsNotExist(err) {
			log.Printf("codex copy page: image file %s is missing, not copied", img.File)
			continue
		}
		if err != nil {
			return out, "", files, err
		}
		file := db.GenerateUUID() + filepath.Ext(img.File)
		err = db.WriteUpload(file, func(w io.Writer) error {
			_, err := io.Copy(w, src)
			return err
		})
		src.Close()
		if err != nil {
			return out, "", files, fmt.Errorf("copy %s: %w", img.File, err)
		}
		files = append(files, file)
		n := img
		n.ID, n.File = db.GenerateUUID(), file
		out = append(out, n)
		if img.ID == coverID {
			cover = n.ID
		}
	}
	return out, cover, files, nil
}
