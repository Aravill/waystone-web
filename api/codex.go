package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

const (
	maxPageTitleRunes = 120
	maxPageBodyRunes  = 100000
	maxPageNoteRunes  = 120
	maxCodexNameRunes = 80
	maxCodexDescRunes = 20000
	maxAliases        = 20
)

// codexName validates a page title or alias: 1-120 characters, no [ ] | or line breaks.
func codexName(s string) (string, bool) {
	s = strings.Join(strings.Fields(s), " ")
	n := utf8.RuneCountInString(s)
	if n < 1 || n > maxPageTitleRunes || strings.ContainsAny(s, "[]|") {
		return "", false
	}
	return s, true
}

func codexUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return "", false
	}
	return userID, true
}

func codexNameTaken(w http.ResponseWriter, e *db.NameTakenError) {
	writeJSON(w, http.StatusConflict, map[string]interface{}{"error": e.Error(), "code": "name_taken", "conflicts": e.Conflicts})
}

func codexDecode(w http.ResponseWriter, r *http.Request, max int64, v interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func codexServerError(w http.ResponseWriter, what string) {
	writeJSONError(w, http.StatusInternalServerError, "failed to "+what)
}

// codexCodices loads the codices by id.
func codexCodices() (map[string]models.Codex, []models.Codex, error) {
	list, err := db.GetCodices()
	if err != nil {
		return nil, nil, err
	}
	byID := map[string]models.Codex{}
	for _, w := range list {
		byID[w.ID] = w
	}
	return byID, list, nil
}

func codexCodexRef(codices map[string]models.Codex, storedID string) map[string]interface{} {
	return map[string]interface{}{"id": storedID, "name": codices[storedID].Name}
}

func codexRevisionObject(users map[string]*models.User, rev models.PageRevision) map[string]interface{} {
	o := codexRevisionSummary(users, rev)
	o["body"] = rev.Body
	return o
}

// codexRevisionSummary is a revision without its body, for the history list.
func codexRevisionSummary(users map[string]*models.User, rev models.PageRevision) map[string]interface{} {
	return map[string]interface{}{
		"number":        rev.Number,
		"author_id":     rev.AuthorID,
		"author_user":   journalUser(users, rev.AuthorID),
		"title":         rev.Title,
		"note":          rev.Note,
		"restored_from": rev.RestoredFrom,
		"created_at":    rev.CreatedAt,
	}
}

func codexExcerpt(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 160 {
		return strings.TrimSpace(string(r[:160])) + "…"
	}
	return s
}

// HandleCodexHome serves GET /api/codices: the codices and recent changes.
func HandleCodexHome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	_, rt, ok := codexRightsFor(w, r)
	if !ok {
		return
	}
	codices, list, err := codexCodices()
	if err != nil {
		codexServerError(w, "retrieve codices")
		return
	}
	pages, err := db.GetCodexPages()
	if err != nil {
		codexServerError(w, "retrieve pages")
		return
	}
	pageCount := map[string]int{}
	for _, p := range pages {
		pageCount[p.CodexID]++
	}
	out := make([]map[string]interface{}, 0, len(list))
	for _, wd := range list {
		if !rt.canSee(wd.ID) {
			continue
		}
		out = append(out, map[string]interface{}{
			"id": wd.ID, "name": wd.Name, "excerpt": codexExcerpt(wd.Description),
			"page_count": pageCount[wd.ID], "can_edit": rt.canEdit(wd.ID),
		})
	}
	var visible []string // nil = every codex (admins)
	if !rt.admin {
		visible = []string{}
		for _, wd := range list {
			if rt.canSee(wd.ID) {
				visible = append(visible, wd.ID)
			}
		}
	}
	recent, err := codexRecent(visible, codices)
	if err != nil {
		codexServerError(w, "retrieve recent changes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"codices":    out,
		"recent":     recent,
		"can_create": rt.canManage(),
	})
}

func codexRecent(codexIDs []string, codices map[string]models.Codex) ([]map[string]interface{}, error) {
	revs, err := db.GetRecentRevisions(codexIDs, 15)
	if err != nil {
		return nil, err
	}
	users, err := journalUsers()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(revs))
	for _, rv := range revs {
		out = append(out, map[string]interface{}{
			"page_id": rv.PageID, "title": rv.Title,
			"codex": codexCodexRef(codices, rv.CodexID), "number": rv.Number, "note": rv.Note,
			"author_user": journalUser(users, rv.AuthorID), "created_at": rv.CreatedAt,
		})
	}
	return out, nil
}

// HandleCodexRoutes dispatches /api/codices/{c|pages|folders|tags|titles|files}/... and POST /api/codices.
func HandleCodexRoutes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/codices"), "/"), "/")
	method := func(allowed ...string) bool {
		for _, m := range allowed {
			if r.Method == m {
				return true
			}
		}
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	switch parts[0] {
	case "":
		if method(http.MethodGet, http.MethodPost) {
			if r.Method == http.MethodGet {
				HandleCodexHome(w, r)
			} else {
				handleCreateCodex(w, r)
			}
		}
	case "titles":
		if len(parts) == 1 && method(http.MethodGet) {
			handleCodexTitles(w, r)
		} else if len(parts) != 1 {
			writeJSONError(w, http.StatusNotFound, "not found")
		}
	case "c":
		switch {
		case len(parts) == 2 && method(http.MethodGet, http.MethodPut, http.MethodDelete):
			switch r.Method {
			case http.MethodGet:
				handleGetCodex(w, r, parts[1])
			case http.MethodPut:
				handleUpdateCodex(w, r, parts[1])
			default:
				handleDeleteCodex(w, r, parts[1])
			}
		case len(parts) == 3 && parts[2] == "folders":
			if method(http.MethodGet, http.MethodPost) {
				if r.Method == http.MethodGet {
					handleCodexFolders(w, r, parts[1])
				} else {
					handleCreateFolder(w, r, parts[1])
				}
			}
		case len(parts) == 3 && parts[2] == "tags":
			if method(http.MethodGet, http.MethodPost) {
				handleCodexTags(w, r, parts[1])
			}
		case len(parts) != 2:
			writeJSONError(w, http.StatusNotFound, "not found")
		}
	case "tags":
		if len(parts) == 2 && method(http.MethodPut, http.MethodDelete) {
			handleCodexTag(w, r, parts[1])
		} else if len(parts) != 2 {
			writeJSONError(w, http.StatusNotFound, "not found")
		}
	case "pages":
		handleCodexPageRoutes(w, r, parts[1:], method)
	case "folders":
		handleCodexFolderRoutes(w, r, parts[1:], method)
	case "files":
		if len(parts) == 2 && method(http.MethodGet) {
			handleCodexFile(w, r, parts[1])
		} else if len(parts) != 2 {
			writeJSONError(w, http.StatusNotFound, "not found")
		}
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

// handleCodexTitles lists the titles and aliases of every page the user can see (optionally one
// codex: ?codex=id), for search on the Codices home.
func handleCodexTitles(w http.ResponseWriter, r *http.Request) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok {
		return
	}
	pages, err := db.GetCodexPages()
	if err != nil {
		codexServerError(w, "retrieve pages")
		return
	}
	filter, only := r.URL.Query().Get("codex"), r.URL.Query().Has("codex")
	out := []map[string]interface{}{}
	for _, p := range pages {
		if (only && p.CodexID != filter) || !rt.canSee(p.CodexID) {
			continue
		}
		out = append(out, map[string]interface{}{
			"id": p.ID, "title": p.Title, "has_map": p.HasMap, "aliases": p.Aliases, "codex_id": p.CodexID,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type codexRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (req *codexRequest) normalize() string {
	req.Name = strings.Join(strings.Fields(req.Name), " ")
	if n := utf8.RuneCountInString(req.Name); n < 1 || n > maxCodexNameRunes {
		return "name must be 1 to 80 characters"
	}
	if utf8.RuneCountInString(req.Description) > maxCodexDescRunes {
		return "description is too long"
	}
	return ""
}

func handleCreateCodex(w http.ResponseWriter, r *http.Request) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkManage(w) {
		return
	}
	var req codexRequest
	if !codexDecode(w, r, 64<<10, &req) {
		return
	}
	if msg := req.normalize(); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	codex := models.Codex{ID: db.GenerateUUID(), Name: req.Name, Description: req.Description, CreatedBy: userID, CreatedAt: now, UpdatedAt: now}
	// Admins have every right anyway; a stored row would outlive a demotion unseen.
	if err := db.CreateCodex(codex, !rt.admin); err != nil {
		if errors.Is(err, db.ErrCodexNameTaken) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "codex_name_taken"})
			return
		}
		codexServerError(w, "create codex")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"codex": codex})
}

func handleUpdateCodex(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkCodex(w, id, false) || !rt.checkManage(w) {
		return
	}
	var req codexRequest
	if !codexDecode(w, r, 64<<10, &req) {
		return
	}
	if msg := req.normalize(); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	if err := db.UpdateCodex(id, req.Name, req.Description); err != nil {
		switch {
		case errors.Is(err, db.ErrCodexNotFound):
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, db.ErrCodexNameTaken):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "codex_name_taken"})
		default:
			codexServerError(w, "update codex")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func handleDeleteCodex(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkCodex(w, id, false) || !rt.checkManage(w) {
		return
	}
	codex, err := db.GetCodex(id)
	if err != nil {
		codexServerError(w, "retrieve codex")
		return
	}
	if codex == nil {
		writeJSONError(w, http.StatusNotFound, db.ErrCodexNotFound.Error())
		return
	}
	if err := db.DeleteCodex(id); err != nil {
		if errors.Is(err, db.ErrCodexInUse) {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error": "Move or delete its pages first.", "code": "codex_in_use",
				"page_count": codexPageCount(id),
			})
			return
		}
		codexServerError(w, "delete codex")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// codexPageCount returns how many pages a codex holds.
func codexPageCount(id string) int {
	pages := 0
	if list, err := db.GetCodexPages(); err == nil {
		for _, p := range list {
			if p.CodexID == id {
				pages++
			}
		}
	}
	return pages
}

func handleGetCodex(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkCodex(w, id, false) {
		return
	}
	codices, _, err := codexCodices()
	if err != nil {
		codexServerError(w, "retrieve codices")
		return
	}
	codex, found := codices[id]
	if !found {
		writeJSONError(w, http.StatusNotFound, db.ErrCodexNotFound.Error())
		return
	}
	pages, err := db.GetCodexPages()
	if err != nil {
		codexServerError(w, "retrieve pages")
		return
	}
	pageTags, err := db.GetCodexPageTags(id)
	if err != nil {
		codexServerError(w, "retrieve tags")
		return
	}
	tags, err := db.GetCodexTags(id)
	if err != nil {
		codexServerError(w, "retrieve tags")
		return
	}
	list := []map[string]interface{}{}
	for _, p := range pages {
		if p.CodexID == id {
			tagIDs := pageTags[p.ID]
			if tagIDs == nil {
				tagIDs = []string{}
			}
			list = append(list, map[string]interface{}{"id": p.ID, "title": p.Title, "has_map": p.HasMap, "aliases": p.Aliases, "folder_id": p.FolderID, "tag_ids": tagIDs})
		}
	}
	folders, err := db.GetFolders(id)
	if err != nil {
		codexServerError(w, "retrieve folders")
		return
	}
	wanted, err := db.GetWantedPages(id)
	if err != nil {
		codexServerError(w, "retrieve wanted pages")
		return
	}
	recent, err := codexRecent([]string{id}, codices)
	if err != nil {
		codexServerError(w, "retrieve recent changes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"codex":      map[string]interface{}{"id": id, "name": codex.Name, "description": codex.Description},
		"pages":      list,
		"folders":    folders,
		"tags":       codexTagObjects(tags, true),
		"wanted":     wanted,
		"recent":     recent,
		"can_edit":   rt.canEdit(id),
		"can_manage": rt.canManage(),
	})
}

type pageCreateRequest struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	CodexID  string `json:"codex_id"`
	FolderID string `json:"folder_id"`
}

func handleCreatePage(w http.ResponseWriter, r *http.Request) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok {
		return
	}
	var req pageCreateRequest
	if !codexDecode(w, r, 512<<10, &req) {
		return
	}
	title, valid := codexName(req.Title)
	if !valid {
		writeJSONError(w, http.StatusBadRequest, "title must be 1 to 120 characters and can't contain [ ] or |")
		return
	}
	if utf8.RuneCountInString(req.Body) > maxPageBodyRunes {
		writeJSONError(w, http.StatusBadRequest, "body is too long")
		return
	}
	if req.CodexID == "" || !rt.canSee(req.CodexID) {
		writeJSONError(w, http.StatusBadRequest, db.ErrCodexNotFound.Error())
		return
	}
	if !rt.canEdit(req.CodexID) {
		writeJSONError(w, http.StatusForbidden, "you can't edit this codex")
		return
	}
	page := models.Page{ID: db.GenerateUUID(), Kind: models.PageLore, CodexID: req.CodexID, FolderID: req.FolderID, Title: title, AuthorID: userID}
	if err := db.CreatePage(page, req.Body); err != nil {
		codexStoreError(w, err, "create page")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"page": map[string]interface{}{
		"id": page.ID, "codex_id": page.CodexID}})
}

// codexStoreError writes the response for the errors the page store returns.
func codexStoreError(w http.ResponseWriter, err error, what string) {
	var taken *db.NameTakenError
	switch {
	case errors.As(err, &taken):
		codexNameTaken(w, taken)
	case errors.Is(err, db.ErrPageNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error(), "code": "page_deleted"})
	case errors.Is(err, db.ErrCodexNotFound), errors.Is(err, db.ErrFolderNotFound):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, db.ErrRevisionNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, db.ErrNoMap):
		writeJSONError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("codex %s: %v", what, err)
		codexServerError(w, what)
	}
}

func handleCodexPageRoutes(w http.ResponseWriter, r *http.Request, parts []string, method func(...string) bool) {
	if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
		if method(http.MethodPost) {
			handleCreatePage(w, r)
		}
		return
	}
	if parts[0] == "" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	id, rest := parts[0], parts[1:]
	switch {
	case len(rest) == 0:
		if !method(http.MethodGet, http.MethodPut, http.MethodDelete) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			handleGetPage(w, r, id)
		case http.MethodPut:
			handleSavePage(w, r, id)
		default:
			handleDeletePage(w, r, id)
		}
	case rest[0] == "revisions" && len(rest) == 1:
		if method(http.MethodGet) {
			handlePageRevisions(w, r, id)
		}
	case rest[0] == "revisions" && (len(rest) == 2 || len(rest) == 3 && rest[2] == "restore"):
		n, err := strconv.Atoi(rest[1])
		if err != nil || n < 1 {
			writeJSONError(w, http.StatusNotFound, "revision not found")
			return
		}
		if len(rest) == 2 {
			if method(http.MethodGet) {
				handleGetRevision(w, r, id, n)
			}
		} else if method(http.MethodPost) {
			handleRestorePage(w, r, id, n)
		}
	case rest[0] == "title" && len(rest) == 1:
		if method(http.MethodPut) {
			handleRenamePage(w, r, id)
		}
	case rest[0] == "aliases" && len(rest) == 1:
		if method(http.MethodPut) {
			handleSetAliases(w, r, id)
		}
	case rest[0] == "folder" && len(rest) == 1:
		if method(http.MethodPut) {
			handleSetPageFolder(w, r, id)
		}
	case rest[0] == "codex" && len(rest) == 1:
		if method(http.MethodPut) {
			handleMovePage(w, r, id)
		}
	case rest[0] == "tags" && len(rest) == 1:
		if method(http.MethodPut) {
			handleSetPageTags(w, r, id)
		}
	default:
		if !handleCodexMapRoutes(w, r, id, rest, method) && !handleCodexPictureRoutes(w, r, id, rest, method) {
			writeJSONError(w, http.StatusNotFound, "not found")
		}
	}
}

func handleGetPage(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, false) {
		return
	}
	payload, status, msg := codexPagePayload(id)
	if msg != "" {
		writeJSONError(w, status, msg)
		return
	}
	codexID := payload["page"].(map[string]interface{})["codex"].(map[string]interface{})["id"].(string)
	payload["can_edit"] = rt.canEdit(codexID)
	writeJSON(w, http.StatusOK, payload)
}

// codexPagePayload builds the page view: the page, its links with their status, and who points at it.
func codexPagePayload(id string) (map[string]interface{}, int, string) {
	page, err := db.GetPage(id)
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve page"
	}
	if page == nil {
		return nil, http.StatusNotFound, db.ErrPageNotFound.Error()
	}
	users, err := journalUsers()
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve users"
	}
	codices, _, err := codexCodices()
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve codices"
	}
	links, err := db.GetPageLinks(id)
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve links"
	}
	all, err := db.GetCodexPages()
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve pages"
	}
	byID := map[string]models.PageListItem{}
	for _, p := range all {
		byID[p.ID] = p
	}
	linkList := make([]map[string]interface{}, 0, len(links))
	for _, l := range links {
		o := map[string]interface{}{"text": l.TargetText, "status": "missing", "page": nil}
		if t, ok := byID[l.ToPageID]; ok {
			o["status"] = "ok"
			o["page"] = map[string]interface{}{"id": t.ID, "title": t.Title, "has_map": t.HasMap}
		}
		linkList = append(linkList, o)
	}
	backs, err := db.GetBacklinks(id)
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve backlinks"
	}
	// One entry per linking page: whether it links, pins or both, and any alias used.
	backList := []map[string]interface{}{}
	at := map[string]int{}
	for _, b := range backs {
		i, seen := at[b.PageID]
		if !seen {
			i = len(backList)
			at[b.PageID] = i
			backList = append(backList, map[string]interface{}{
				"id": b.PageID, "title": b.Title,
				"via": []string{}, "aliases": []string{},
			})
		}
		via := "link"
		if b.PinID != "" {
			via = "pin"
		}
		o := backList[i]
		if !containsString(o["via"].([]string), via) {
			o["via"] = append(o["via"].([]string), via)
		}
		if db.NormName(b.TargetText) != db.NormName(page.Title) && !containsString(o["aliases"].([]string), b.TargetText) {
			o["aliases"] = append(o["aliases"].([]string), b.TargetText)
		}
	}
	folderPath, err := codexFolderPath(page.CodexID, page.FolderID)
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve folders"
	}
	amenders := []string{}
	for _, c := range page.Contributors {
		if c.UserID != page.AuthorID {
			amenders = append(amenders, c.UserID)
		}
	}
	incoming := 0 // links only: pins are hidden while maps are
	for _, b := range backs {
		if b.PinID == "" {
			incoming++
		}
	}
	payload := map[string]interface{}{
		"page": map[string]interface{}{
			"id": page.ID, "title": page.Title,
			"codex": codexCodexRef(codices, page.CodexID), "folder_id": page.FolderID, "folder_path": folderPath, "aliases": page.Aliases, "body": page.Body,
			"current_revision": page.CurrentRevision, "revision_count": page.RevisionCount,
			"created_at": page.CreatedAt, "updated_at": page.UpdatedAt,
			"author_user": journalUser(users, page.AuthorID), "amender_users": journalUserList(users, amenders),
			"updated_by_user": journalUser(users, page.UpdatedByID),
		},
		"links":          linkList,
		"backlinks":      backList,
		"incoming_count": incoming,
		"on_maps":        []interface{}{},
	}
	tags, err := db.GetPageTags(id)
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve tags"
	}
	payload["tags"] = codexTagObjects(tags, false)
	if err := codexAddPicturePayload(payload, id); err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve images"
	}
	if err := codexAddMapPayload(payload, page); err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve map"
	}
	if err := codexAddOnMaps(payload, backs); err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve maps"
	}
	return payload, http.StatusOK, ""
}

type pageSaveRequest struct {
	BaseRevision int    `json:"base_revision"`
	Body         string `json:"body"`
	Note         string `json:"note"`
}

func handleSavePage(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req pageSaveRequest
	if !codexDecode(w, r, 512<<10, &req) {
		return
	}
	req.Note = strings.TrimSpace(req.Note)
	if req.BaseRevision < 1 || utf8.RuneCountInString(req.Body) > maxPageBodyRunes || utf8.RuneCountInString(req.Note) > maxPageNoteRunes {
		writeJSONError(w, http.StatusBadRequest, "invalid page body or note")
		return
	}
	saveCodexRevision(w, id, req.BaseRevision, models.PageRevision{AuthorID: userID, Body: req.Body, Note: req.Note})
}

func saveCodexRevision(w http.ResponseWriter, id string, base int, rev models.PageRevision) {
	result, err := db.SavePageRevision(id, base, rev)
	if err != nil {
		codexStoreError(w, err, "save page")
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	switch result.Outcome {
	case models.PageStale:
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "the page changed while you were editing", "code": "stale_base",
			"latest": codexRevisionObject(users, *result.Latest)})
	case models.PageUnchanged:
		writeJSON(w, http.StatusOK, map[string]interface{}{"revision": codexRevisionObject(users, result.Revision), "unchanged": true})
	default:
		writeJSON(w, http.StatusCreated, map[string]interface{}{"revision": codexRevisionObject(users, result.Revision)})
	}
}

func handlePageRevisions(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, false) {
		return
	}
	revs, err := db.GetPageRevisions(id)
	if err != nil {
		codexServerError(w, "retrieve revisions")
		return
	}
	if len(revs) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": db.ErrPageNotFound.Error(), "code": "page_deleted"})
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	list := make([]map[string]interface{}, 0, len(revs))
	for _, rv := range revs {
		list = append(list, codexRevisionSummary(users, rv))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current_revision": revs[0].Number,
		"pruned":           revs[len(revs)-1].Number > 1,
		"revisions":        list,
	})
}

// handleRestorePage saves an old revision's body as a new revision. The title is never restored.
func handleRestorePage(w http.ResponseWriter, r *http.Request, id string, number int) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req struct {
		BaseRevision int `json:"base_revision"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	if req.BaseRevision < 1 {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rv, err := db.GetPageRevision(id, number)
	if err != nil {
		codexStoreError(w, err, "retrieve revision")
		return
	}
	saveCodexRevision(w, id, req.BaseRevision, models.PageRevision{AuthorID: userID, Body: rv.Body, RestoredFrom: number})
}

// handleGetRevision returns one kept revision with its body.
func handleGetRevision(w http.ResponseWriter, r *http.Request, id string, number int) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, false) {
		return
	}
	rv, err := db.GetPageRevision(id, number)
	if err != nil {
		codexStoreError(w, err, "retrieve revision")
		return
	}
	users, err := journalUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"revision": codexRevisionObject(users, rv)})
}

func handleRenamePage(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	title, valid := codexName(req.Title)
	if !valid {
		writeJSONError(w, http.StatusBadRequest, "title must be 1 to 120 characters and can't contain [ ] or |")
		return
	}
	n, err := db.RenamePage(id, title, userID)
	if err != nil {
		codexStoreError(w, err, "rename page")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"rewritten": n, "title": title})
}

func handleSetAliases(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req struct {
		Aliases []string `json:"aliases"`
	}
	if !codexDecode(w, r, 16<<10, &req) {
		return
	}
	page, err := db.GetPage(id)
	if err != nil {
		codexServerError(w, "retrieve page")
		return
	}
	if page == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": db.ErrPageNotFound.Error(), "code": "page_deleted"})
		return
	}
	if len(req.Aliases) > maxAliases {
		writeJSONError(w, http.StatusBadRequest, "at most 20 aliases")
		return
	}
	clean, seen := []string{}, map[string]bool{db.NormName(page.Title): true}
	for _, a := range req.Aliases {
		a, valid := codexName(a)
		if !valid {
			writeJSONError(w, http.StatusBadRequest, "aliases must be 1 to 120 characters and can't contain [ ] or |")
			return
		}
		if k := db.NormName(a); !seen[k] {
			seen[k] = true
			clean = append(clean, a)
		}
	}
	if err := db.SetPageAliases(id, clean); err != nil {
		codexStoreError(w, err, "save aliases")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"aliases": clean})
}

func handleMovePage(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req struct {
		CodexID string `json:"codex_id"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	// The target needs Edit too; one that doesn't exist or can't be seen is "codex not found".
	if !rt.canSee(req.CodexID) {
		writeJSONError(w, http.StatusBadRequest, db.ErrCodexNotFound.Error())
		return
	}
	if !rt.canEdit(req.CodexID) {
		writeJSONError(w, http.StatusForbidden, "you can't edit the target codex")
		return
	}
	if err := db.MovePage(id, req.CodexID); err != nil {
		codexStoreError(w, err, "move page")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func handleDeletePage(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	files, err := db.DeletePage(id)
	if err != nil {
		codexStoreError(w, err, "delete page")
		return
	}
	removeCodexFiles(files)
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// redirectOldCodexURL sends the old /codex page URLs to /codices: /codex/w/{rest} became
// /codices/c/{rest}, /codex/p/{rest} became /codices/p/{rest}, anything else goes to the home.
func redirectOldCodexURL(w http.ResponseWriter, r *http.Request) {
	target := "/codices"
	rest := strings.TrimPrefix(r.URL.EscapedPath(), "/codex")
	if strings.HasPrefix(rest, "/w/unsorted") {
		// The old Unsorted folder has no codex of its own any more.
	} else if strings.HasPrefix(rest, "/w/") {
		target = "/codices/c/" + strings.TrimPrefix(rest, "/w/")
	} else if strings.HasPrefix(rest, "/p/") {
		target = "/codices/p/" + strings.TrimPrefix(rest, "/p/")
	}
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}
