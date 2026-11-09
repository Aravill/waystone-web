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
	maxWorldNameRunes = 80
	maxWorldDescRunes = 20000
	maxAliases        = 20
)

// codexWorldParam maps the API's "unsorted" (or empty) world id to the stored "".
func codexWorldParam(id string) string {
	if id == models.UnsortedWorldID {
		return ""
	}
	return id
}

// codexWorldID is the API's name for a stored world id.
func codexWorldID(id string) string {
	if id == "" {
		return models.UnsortedWorldID
	}
	return id
}

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

func codexIsAdmin(userID string) (bool, error) {
	u, err := db.GetUserByID(userID)
	return err == nil && u != nil && u.IsAdmin(), err
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

// codexWorlds loads the worlds by id plus the synthetic Unsorted folder.
func codexWorlds() (map[string]models.World, []models.World, error) {
	list, err := db.GetWorlds()
	if err != nil {
		return nil, nil, err
	}
	byID := map[string]models.World{"": {ID: models.UnsortedWorldID, Name: "Unsorted", Description: "Pages that haven't been put in a world yet."}}
	for _, w := range list {
		byID[w.ID] = w
	}
	return byID, list, nil
}

func codexWorldRef(worlds map[string]models.World, storedID string) map[string]interface{} {
	return map[string]interface{}{"id": codexWorldID(storedID), "name": worlds[storedID].Name}
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

// HandleCodexHome serves GET /api/codex: the worlds, the Unsorted folder and recent changes.
func HandleCodexHome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, ok := codexUserID(w, r); !ok {
		return
	}
	worlds, list, err := codexWorlds()
	if err != nil {
		codexServerError(w, "retrieve worlds")
		return
	}
	pages, err := db.GetCodexPages()
	if err != nil {
		codexServerError(w, "retrieve pages")
		return
	}
	campaigns, err := db.GetAllCampaigns()
	if err != nil {
		codexServerError(w, "retrieve games")
		return
	}
	pageCount, gameCount := map[string]int{}, map[string]int{}
	for _, p := range pages {
		pageCount[p.WorldID]++
	}
	for _, c := range campaigns {
		if c.WorldID != "" {
			gameCount[c.WorldID]++
		}
	}
	out := make([]map[string]interface{}, 0, len(list))
	for _, wd := range list {
		out = append(out, map[string]interface{}{
			"id": wd.ID, "name": wd.Name, "excerpt": codexExcerpt(wd.Description),
			"game_count": gameCount[wd.ID], "page_count": pageCount[wd.ID],
		})
	}
	recent, err := codexRecent(nil, worlds)
	if err != nil {
		codexServerError(w, "retrieve recent changes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"worlds":   out,
		"unsorted": map[string]interface{}{"id": models.UnsortedWorldID, "name": "Unsorted", "page_count": pageCount[""]},
		"recent":   recent,
	})
}

func codexRecent(worldID *string, worlds map[string]models.World) ([]map[string]interface{}, error) {
	revs, err := db.GetRecentRevisions(worldID, 15)
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
			"page_id": rv.PageID, "title": rv.Title, "kind": rv.Kind,
			"world": codexWorldRef(worlds, rv.WorldID), "number": rv.Number, "note": rv.Note,
			"author_user": journalUser(users, rv.AuthorID), "created_at": rv.CreatedAt,
		})
	}
	return out, nil
}

// HandleCodexRoutes dispatches /api/codex/{worlds|pages|titles|files}/...
func HandleCodexRoutes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/codex/"), "/"), "/")
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
	case "titles":
		if len(parts) == 1 && method(http.MethodGet) {
			handleCodexTitles(w, r)
		} else if len(parts) != 1 {
			writeJSONError(w, http.StatusNotFound, "not found")
		}
	case "worlds":
		switch {
		case len(parts) == 1 && method(http.MethodPost):
			handleCreateWorld(w, r)
		case len(parts) == 2 && method(http.MethodGet, http.MethodPut, http.MethodDelete):
			switch r.Method {
			case http.MethodGet:
				handleGetWorld(w, r, parts[1])
			case http.MethodPut:
				handleUpdateWorld(w, r, parts[1])
			default:
				handleDeleteWorld(w, r, parts[1])
			}
		case len(parts) == 3 && parts[2] == "folders" && method(http.MethodPost):
			handleCreateFolder(w, r, parts[1])
		case len(parts) > 2:
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

// handleCodexTitles lists page titles and aliases (optionally one world: ?world=id), for the
// editor's autocomplete, the pin picker and link status in previews.
func handleCodexTitles(w http.ResponseWriter, r *http.Request) {
	if _, ok := codexUserID(w, r); !ok {
		return
	}
	pages, err := db.GetCodexPages()
	if err != nil {
		codexServerError(w, "retrieve pages")
		return
	}
	filter, only := r.URL.Query().Get("world"), r.URL.Query().Has("world")
	out := []map[string]interface{}{}
	for _, p := range pages {
		if only && p.WorldID != codexWorldParam(filter) {
			continue
		}
		out = append(out, map[string]interface{}{
			"id": p.ID, "title": p.Title, "kind": p.Kind, "aliases": p.Aliases,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type worldRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (req *worldRequest) normalize() string {
	req.Name = strings.Join(strings.Fields(req.Name), " ")
	if n := utf8.RuneCountInString(req.Name); n < 1 || n > maxWorldNameRunes {
		return "name must be 1 to 80 characters"
	}
	if db.NormName(req.Name) == models.UnsortedWorldID {
		return "\"unsorted\" is reserved for the Unsorted folder"
	}
	if utf8.RuneCountInString(req.Description) > maxWorldDescRunes {
		return "description is too long"
	}
	return ""
}

func handleCreateWorld(w http.ResponseWriter, r *http.Request) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return
	}
	var req worldRequest
	if !codexDecode(w, r, 64<<10, &req) {
		return
	}
	if msg := req.normalize(); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	world := models.World{ID: db.GenerateUUID(), Name: req.Name, Description: req.Description, CreatedBy: userID, CreatedAt: now, UpdatedAt: now}
	if err := db.CreateWorld(world); err != nil {
		if errors.Is(err, db.ErrWorldNameTaken) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "world_name_taken"})
			return
		}
		codexServerError(w, "create world")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"world": world})
}

func handleUpdateWorld(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := codexUserID(w, r); !ok {
		return
	}
	var req worldRequest
	if !codexDecode(w, r, 64<<10, &req) {
		return
	}
	if msg := req.normalize(); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	if err := db.UpdateWorld(id, req.Name, req.Description); err != nil {
		switch {
		case errors.Is(err, db.ErrWorldNotFound):
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, db.ErrWorldNameTaken):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "world_name_taken"})
		default:
			codexServerError(w, "update world")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func handleDeleteWorld(w http.ResponseWriter, r *http.Request, id string) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return
	}
	admin, err := codexIsAdmin(userID)
	if err != nil {
		codexServerError(w, "retrieve user")
		return
	}
	if !admin {
		writeJSONError(w, http.StatusForbidden, "only an admin can delete a world")
		return
	}
	world, err := db.GetWorld(id)
	if err != nil {
		codexServerError(w, "retrieve world")
		return
	}
	if world == nil {
		writeJSONError(w, http.StatusNotFound, db.ErrWorldNotFound.Error())
		return
	}
	if err := db.DeleteWorld(id); err != nil {
		if errors.Is(err, db.ErrWorldInUse) {
			games, pages := codexWorldUsage(id)
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error": "Move or delete its pages and unlink its games first.", "code": "world_in_use",
				"games": games, "page_count": pages,
			})
			return
		}
		codexServerError(w, "delete world")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// codexWorldUsage returns the titles of the games in a world and how many pages it holds.
func codexWorldUsage(id string) ([]string, int) {
	games := []string{}
	if campaigns, err := db.GetAllCampaigns(); err == nil {
		for _, c := range campaigns {
			if c.WorldID == id {
				games = append(games, c.Title)
			}
		}
	}
	pages := 0
	if list, err := db.GetCodexPages(); err == nil {
		for _, p := range list {
			if p.WorldID == id {
				pages++
			}
		}
	}
	return games, pages
}

func handleGetWorld(w http.ResponseWriter, r *http.Request, apiID string) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return
	}
	id := codexWorldParam(apiID)
	worlds, _, err := codexWorlds()
	if err != nil {
		codexServerError(w, "retrieve worlds")
		return
	}
	world, found := worlds[id]
	if !found {
		writeJSONError(w, http.StatusNotFound, db.ErrWorldNotFound.Error())
		return
	}
	pages, err := db.GetCodexPages()
	if err != nil {
		codexServerError(w, "retrieve pages")
		return
	}
	list := []map[string]interface{}{}
	for _, p := range pages {
		if p.WorldID == id {
			list = append(list, map[string]interface{}{"id": p.ID, "title": p.Title, "kind": p.Kind, "aliases": p.Aliases, "folder_id": p.FolderID})
		}
	}
	folders, err := db.GetFolders(id)
	if err != nil {
		codexServerError(w, "retrieve folders")
		return
	}
	games := []map[string]interface{}{}
	if id != "" {
		campaigns, err := db.GetAllCampaigns()
		if err != nil {
			codexServerError(w, "retrieve games")
			return
		}
		for _, c := range campaigns {
			if c.WorldID == id {
				games = append(games, map[string]interface{}{"id": c.ID, "title": c.Title, "status": c.Status})
			}
		}
	}
	admin, err := codexIsAdmin(userID)
	if err != nil {
		codexServerError(w, "retrieve user")
		return
	}
	wanted, err := db.GetWantedPages(id)
	if err != nil {
		codexServerError(w, "retrieve wanted pages")
		return
	}
	recent, err := codexRecent(&id, worlds)
	if err != nil {
		codexServerError(w, "retrieve recent changes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"world":      map[string]interface{}{"id": codexWorldID(id), "name": world.Name, "description": world.Description, "unsorted": id == ""},
		"games":      games,
		"pages":      list,
		"folders":    folders,
		"wanted":     wanted,
		"recent":     recent,
		"can_delete": id != "" && admin,
	})
}

type pageCreateRequest struct {
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	WorldID  string `json:"world_id"`
	FolderID string `json:"folder_id"`
}

func handleCreatePage(w http.ResponseWriter, r *http.Request) {
	userID, ok := codexUserID(w, r)
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
	if req.Kind != models.PageLore && req.Kind != models.PageMap {
		writeJSONError(w, http.StatusBadRequest, "kind must be lore or map")
		return
	}
	if utf8.RuneCountInString(req.Body) > maxPageBodyRunes {
		writeJSONError(w, http.StatusBadRequest, "body is too long")
		return
	}
	page := models.Page{ID: db.GenerateUUID(), Kind: req.Kind, WorldID: codexWorldParam(req.WorldID), FolderID: req.FolderID, Title: title, AuthorID: userID}
	if err := db.CreatePage(page, req.Body); err != nil {
		codexStoreError(w, err, "create page")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"page": map[string]interface{}{
		"id": page.ID, "world_id": codexWorldID(page.WorldID)}})
}

// codexStoreError writes the response for the errors the page store returns.
func codexStoreError(w http.ResponseWriter, err error, what string) {
	var taken *db.NameTakenError
	switch {
	case errors.As(err, &taken):
		codexNameTaken(w, taken)
	case errors.Is(err, db.ErrPageNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error(), "code": "page_deleted"})
	case errors.Is(err, db.ErrWorldNotFound), errors.Is(err, db.ErrFolderNotFound), errors.Is(err, db.ErrNoFolders):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, db.ErrRevisionNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, db.ErrNotAMap):
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
	case rest[0] == "world" && len(rest) == 1:
		if method(http.MethodPut) {
			handleMovePage(w, r, id)
		}
	default:
		if !handleCodexMapRoutes(w, r, id, rest, method) {
			writeJSONError(w, http.StatusNotFound, "not found")
		}
	}
}

func handleGetPage(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := codexUserID(w, r); !ok {
		return
	}
	payload, status, msg := codexPagePayload(id)
	if msg != "" {
		writeJSONError(w, status, msg)
		return
	}
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
	worlds, _, err := codexWorlds()
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve worlds"
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
			o["page"] = map[string]interface{}{"id": t.ID, "title": t.Title, "kind": t.Kind}
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
				"id": b.PageID, "title": b.Title, "kind": b.Kind,
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
	folderPath, err := codexFolderPath(page.WorldID, page.FolderID)
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to retrieve folders"
	}
	amenders := []string{}
	for _, c := range page.Contributors {
		if c.UserID != page.AuthorID {
			amenders = append(amenders, c.UserID)
		}
	}
	payload := map[string]interface{}{
		"page": map[string]interface{}{
			"id": page.ID, "kind": page.Kind, "title": page.Title,
			"world": codexWorldRef(worlds, page.WorldID), "folder_id": page.FolderID, "folder_path": folderPath, "aliases": page.Aliases, "body": page.Body,
			"current_revision": page.CurrentRevision, "revision_count": page.RevisionCount,
			"created_at": page.CreatedAt, "updated_at": page.UpdatedAt,
			"author_user": journalUser(users, page.AuthorID), "amender_users": journalUserList(users, amenders),
			"updated_by_user": journalUser(users, page.UpdatedByID),
		},
		"links":          linkList,
		"backlinks":      backList,
		"incoming_count": len(backs),
		"on_maps":        []interface{}{},
	}
	if page.Kind == models.PageMap {
		if err := codexAddMapPayload(payload, page); err != nil {
			return nil, http.StatusInternalServerError, "failed to retrieve map"
		}
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
	userID, ok := codexUserID(w, r)
	if !ok {
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
	if _, ok := codexUserID(w, r); !ok {
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
	userID, ok := codexUserID(w, r)
	if !ok {
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
	if _, ok := codexUserID(w, r); !ok {
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
	userID, ok := codexUserID(w, r)
	if !ok {
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
	if _, ok := codexUserID(w, r); !ok {
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
	if _, ok := codexUserID(w, r); !ok {
		return
	}
	var req struct {
		WorldID string `json:"world_id"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	if err := db.MovePage(id, codexWorldParam(req.WorldID)); err != nil {
		codexStoreError(w, err, "move page")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func handleDeletePage(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := codexUserID(w, r); !ok {
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
