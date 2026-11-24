package api

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

// builtinTemplate is a template defined here: the same in every codex of a language, never stored, so
// the server can apply it and the client shows the same text. Tags are names, created in the codex on
// use. The ids are the same in every language.
type builtinTemplate struct {
	ID, Name, Description string
	Tags                  []string
	Body                  string
}

var builtinTemplates = map[string][]builtinTemplate{models.LangEnglish: builtinEnglish, models.LangCzech: builtinCzech}

var builtinEnglish = []builtinTemplate{
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

var builtinCzech = []builtinTemplate{
	{"blank", "Prázdná stránka", "Začít od nuly", []string{}, ""},
	{"session", "Sezení", "Zápis z jednoho herního sezení", []string{"sezení"},
		"**Datum:** {{date}}\n**Přítomni:** [[ ]], [[ ]]\n\n## Shrnutí\n\n_Co se stalo, jednou nebo dvěma větami._\n\n## Co se stalo\n\n1. \n\n## Koho jsme potkali\n\n- [[ ]]\n\n## Kořist a odměny\n\n- \n\n## Nedořešené věci\n\n- \n\n## Příště\n\n"},
	{"character", "Postava", "Hráčská i nehráčská postava", []string{"npc"},
		"_Kdo je {{title}}, jednou větou._\n\n**Bydliště:** [[ ]]\n**Příslušnost:** [[ ]]\n\n## Vzhled\n\n## Povaha\n\n## Minulost\n\n## Vztahy\n\n- [[ ]]: \n\n## Poznámky\n\n"},
	{"settlement", "Sídlo", "Vesnice, městečko nebo město", []string{"sídlo"},
		"_Co je {{title}} zač, jednou větou._\n\n**Počet obyvatel:** \n**Vládne:** [[ ]]\n**Oblast:** [[ ]]\n\n## Významná místa\n\n- [[ ]]\n\n## Významné osoby\n\n- [[ ]]\n\n## Historie\n\n## Zvěsti\n\n- \n"},
	{"landmark", "Místo", "Místo, které si zaslouží vlastní stránku", []string{"místo"},
		"_Co je {{title}} zač a kde leží, jednou větou._\n\n**Poblíž:** [[ ]]\n\n## Popis\n\n## Historie\n\n## Nebezpečí\n\n## Zvěsti\n\n- \n"},
	{"event", "Událost", "Něco, co se stalo: bitva, požár, svatba", []string{"událost"},
		"_Co se stalo, jednou větou._\n\n**Kdy:** \n**Kde:** [[ ]]\n**Kdo:** [[ ]], [[ ]]\n\n## Co se stalo\n\n## Proč na tom záleží\n\n## Následky\n\n- \n"},
	{"faction", "Frakce", "Cech, kult, rod nebo vojsko", []string{"frakce"},
		"_Co je {{title}} za skupinu, jednou větou._\n\n**V čele:** [[ ]]\n**Sídlí v:** [[ ]]\n\n## Cíle\n\n## Členové\n\n- [[ ]]\n\n## Spojenci a nepřátelé\n\n- \n\n## Prostředky\n\n"},
	{"item", "Předmět", "Věc, relikvie nebo poklad", []string{"předmět"},
		"_Co je {{title}} zač, jednou větou._\n\n**Vlastník:** [[ ]]\n\n## Vzhled\n\n## Co umí\n\n## Historie\n\n## Kde je teď\n\n"},
	{"lore", "Historie", "Dějiny, legendy, náboženství, zvyky", []string{"historie"},
		"_O čem to je, jednou větou._\n\n## Příběh\n\n## Čemu lidé věří\n\n## Prameny\n\n- [[ ]]\n"},
}

// codexLanguage loads a codex's language; ok is false (with the response written) when it can't.
func codexLanguage(w http.ResponseWriter, codexID string) (string, bool) {
	codex, err := db.GetCodex(codexID)
	if err != nil {
		codexServerError(w, "retrieve codex")
		return "", false
	}
	if codex == nil {
		codexStoreError(w, db.ErrCodexNotFound, "")
		return "", false
	}
	return codex.Language, true
}

// codexDate writes a date in a codex's language. The Czech one has no-break spaces: it stays on one line,
// and at the start of a line "23. 11. 2026" isn't taken for a numbered list by the editor.
func codexDate(t time.Time, lang string) string {
	if lang == models.LangCzech {
		return t.Format("2.\u00a01.\u00a02006")
	}
	return t.Format("Jan 2, 2006")
}

// codexToday is the day {{date}} stands for.
func codexToday(lang string) string { return codexDate(time.Now().UTC(), lang) }

// fillTemplate applies the two placeholders in one pass, so a title that contains "{{date}}" stays as typed.
func fillTemplate(body, title, date string) string {
	return strings.NewReplacer("{{title}}", title, "{{date}}", date).Replace(body)
}

func builtinTemplateByID(id, lang string) *builtinTemplate {
	if !strings.HasPrefix(id, builtinTemplatePrefix) {
		return nil
	}
	for i, t := range builtinTemplates[lang] {
		if t.ID == strings.TrimPrefix(id, builtinTemplatePrefix) {
			return &builtinTemplates[lang][i]
		}
	}
	return nil
}

// handleBuiltinTemplates serves GET /api/codices/templates/builtin[?codex=id]: the built-in templates
// in the codex's language (English without one) and the date {{date}} stands for today.
func handleBuiltinTemplates(w http.ResponseWriter, r *http.Request) {
	lang := models.LangEnglish
	if codexID := r.URL.Query().Get("codex"); codexID != "" {
		_, rt, ok := codexRightsFor(w, r)
		if !ok || !rt.checkCodex(w, codexID, false) {
			return
		}
		if lang, ok = codexLanguage(w, codexID); !ok {
			return
		}
	}
	out := make([]map[string]interface{}, 0, len(builtinTemplates[lang]))
	for _, t := range builtinTemplates[lang] {
		out = append(out, map[string]interface{}{
			"id": builtinTemplatePrefix + t.ID, "name": t.Name, "description": t.Description, "tags": t.Tags, "body": t.Body,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"language": lang, "templates": out, "today": codexToday(lang)})
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

// builtinNameTaken reports whether name is a built-in template's name in any language (two cards of
// one name would look identical in the New page dialog, now or after the codex changes its language).
func builtinNameTaken(w http.ResponseWriter, name string) bool {
	for _, list := range builtinTemplates {
		for _, b := range list {
			if db.NormName(b.Name) == db.NormName(name) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "A built-in template has that name, in English or Czech.", "code": "template_name_taken"})
				return true
			}
		}
	}
	return false
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
	if builtinNameTaken(w, req.Name) {
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
	// A template saved before its name became a built-in one keeps it.
	if db.NormName(req.Name) != db.NormName(t.Name) && builtinNameTaken(w, req.Name) {
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
	lang, ok := codexLanguage(w, codexID)
	if !ok {
		return "", nil, false
	}
	return pageStartOn(w, templateID, codexID, lang, title, body, codexToday(lang))
}

// pageStartOn is pageStart with the codex's language and the date {{date}} stands for (a journal page
// uses its session's date).
func pageStartOn(w http.ResponseWriter, templateID, codexID, lang, title, body, date string) (string, []string, bool) {
	if templateID == "" {
		return body, nil, true
	}
	if b := builtinTemplateByID(templateID, lang); b != nil {
		return fillTemplate(b.Body, title, date), b.Tags, true
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
	return fillTemplate(t.Body, title, date), names, true
}

type pageCopyRequest struct {
	Title      string `json:"title"`
	CodexID    string `json:"codex_id"`
	FolderID   string `json:"folder_id"`
	CopyTags   bool   `json:"copy_tags"`
	CopyImages bool   `json:"copy_images"`

	CopyAttachments bool `json:"copy_attachments"`
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
	page := models.Page{ID: db.GenerateUUID(), Kind: src.Kind, CodexID: req.CodexID, FolderID: req.FolderID, Title: title, AuthorID: userID}
	if src.Kind == models.PageTimeline {
		// Links to pages stay only inside the same codex; elsewhere an event keeps its title.
		if err := copyTimelineInto(&x, id, src.CodexID == req.CodexID); err != nil {
			codexServerError(w, "copy the timeline")
			return
		}
	}
	// Cheap checks first, so a refused copy never duplicates a file.
	if err := db.CheckNewPage(req.CodexID, req.FolderID, title); err != nil {
		codexStoreError(w, err, "copy page")
		return
	}
	var newFiles, newAttachments []string
	if src.Kind == models.PageMap {
		if newFiles, err = copyMapInto(&x, src, src.CodexID == req.CodexID, userID); err != nil {
			log.Printf("codex copy page: %v", err)
			codexServerError(w, "copy the map")
			return
		}
	}
	if req.CopyImages && src.Kind != models.PageMap {
		if x.Images, x.CoverImageID, newFiles, err = duplicatePictures(id, src.Body, userID); err != nil {
			removeCodexFiles(newFiles)
			log.Printf("codex copy page: %v", err)
			codexServerError(w, "copy the images")
			return
		}
	}
	if req.CopyAttachments {
		// Copying up to 100 MB per file needs more than the server-wide write timeout.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(attachmentTimeout))
		if x.Attachments, newAttachments, err = duplicateAttachments(id, userID); err != nil {
			removeCodexFiles(newFiles)
			removeAttachmentFiles(newAttachments)
			log.Printf("codex copy page: %v", err)
			codexServerError(w, "copy the attachments")
			return
		}
	}
	if err := db.CreatePageWith(page, src.Body, x); err != nil {
		removeCodexFiles(newFiles)
		removeAttachmentFiles(newAttachments)
		codexStoreError(w, err, "copy page")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"page": map[string]interface{}{"id": page.ID, "codex_id": page.CodexID}})
}

var pictureRefRe = regexp.MustCompile(`!\[\[([^\[\]\n|]{1,200})(?:\|[^\[\]\n]{0,200})?\]\]`)

// duplicatePictures copies the files of a page's current pictures, and of removed ones the body
// still embeds, under new random names and returns the new rows (same picture names and removed
// state, new ids, uploaded by the copier now), the new id of the cover (or "") and the new file
// names. A picture whose file is gone is left out. On error the caller removes the files.
func duplicatePictures(pageID, body, userID string) ([]models.PageImage, string, []string, error) {
	all, coverID, err := db.GetPageImages(pageID)
	if err != nil {
		return nil, "", nil, err
	}
	embedded := map[string]bool{}
	for _, m := range pictureRefRe.FindAllStringSubmatch(body, -1) {
		embedded[db.NormName(m[1])] = true
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var out []models.PageImage
	var files []string
	cover := ""
	for _, img := range all {
		if (img.RemovedAt != "" && !embedded[db.NormName(img.Name)]) || !db.UploadFileRe.MatchString(img.File) {
			continue
		}
		file, err := copyUploadFile(img.File)
		if os.IsNotExist(err) {
			log.Printf("codex copy page: image file %s is missing, not copied", img.File)
			continue
		}
		if err != nil {
			return out, "", files, fmt.Errorf("copy %s: %w", img.File, err)
		}
		files = append(files, file)
		n := img
		n.ID, n.File, n.UploadedBy, n.CreatedAt = db.GenerateUUID(), file, userID, now
		out = append(out, n)
		if img.ID == coverID {
			cover = n.ID
		}
	}
	return out, cover, files, nil
}

// copyUploadFile stores a copy of an uploaded image under a new random name. The error of a file that
// is gone satisfies os.IsNotExist.
func copyUploadFile(name string) (string, error) {
	dir, err := db.UploadDir()
	if err != nil {
		return "", err
	}
	src, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer src.Close()
	file := db.GenerateUUID() + filepath.Ext(name)
	return file, db.WriteUpload(file, func(w io.Writer) error {
		_, err := io.Copy(w, src)
		return err
	})
}

// duplicateAttachments copies the files of a page's attachments under new random names and returns the new rows
// (same names and sizes, new ids, uploaded by the copier now) and the new file names. An attachment whose file is
// gone is left out. On error it returns the files made so far, which the caller removes.
func duplicateAttachments(pageID, userID string) ([]models.PageAttachment, []string, error) {
	all, err := db.GetPageAttachments(pageID)
	if err != nil {
		return nil, nil, err
	}
	dir, err := db.AttachmentDir()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var out []models.PageAttachment
	var files []string
	for _, a := range all {
		if !db.AttachmentFileRe.MatchString(a.File) {
			continue
		}
		src, err := os.Open(filepath.Join(dir, a.File))
		if os.IsNotExist(err) {
			log.Printf("codex copy page: attachment file %s is missing, not copied", a.File)
			continue
		}
		if err != nil {
			return out, files, err
		}
		file, err := copyAttachmentFile(src)
		src.Close()
		if err != nil {
			return out, files, fmt.Errorf("copy %s: %w", a.File, err)
		}
		files = append(files, file)
		n := a
		n.ID, n.File, n.UploadedBy, n.CreatedAt = db.GenerateUUID(), file, userID, now
		out = append(out, n)
	}
	return out, files, nil
}

// copyAttachmentFile stores a copy of src in the attachments directory under a new random name.
func copyAttachmentFile(src io.Reader) (string, error) {
	tmp, err := db.CreateAttachmentTemp()
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		db.DiscardUpload(tmp)
		return "", err
	}
	file := db.GenerateUUID()
	return file, db.CommitUpload(tmp, file)
}
