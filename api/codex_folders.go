package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

const maxFolderNameRunes = 80

// codexFolderName validates a folder name: 1-80 characters, whitespace collapsed, no "/" or
// control characters.
func codexFolderName(s string) (string, bool) {
	s = strings.Join(strings.Fields(s), " ")
	n := utf8.RuneCountInString(s)
	if n < 1 || n > maxFolderNameRunes || strings.ContainsRune(s, '/') {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return s, true
}

const codexFolderNameMsg = "folder name must be 1 to 80 characters and can't contain / or control characters"

// codexFolderPath returns the folders from the world's top level down to folderID.
func codexFolderPath(worldID, folderID string) ([]map[string]string, error) {
	path := []map[string]string{}
	if folderID == "" {
		return path, nil
	}
	all, err := db.GetFolders(worldID)
	if err != nil {
		return nil, err
	}
	byID := map[string]models.Folder{}
	for _, f := range all {
		byID[f.ID] = f
	}
	for cur, hops := folderID, 0; cur != "" && hops <= len(all); hops++ {
		f, ok := byID[cur]
		if !ok {
			break
		}
		path = append([]map[string]string{{"id": f.ID, "name": f.Name}}, path...)
		cur = f.ParentID
	}
	return path, nil
}

// codexFolderError writes the response for the errors the folder store returns.
func codexFolderError(w http.ResponseWriter, err error, what string) {
	switch {
	case errors.Is(err, db.ErrFolderNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error(), "code": "folder_deleted"})
	case errors.Is(err, db.ErrFolderNameTaken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "folder_name_taken"})
	case errors.Is(err, db.ErrParentNotFound):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "parent_not_found"})
	case errors.Is(err, db.ErrFolderCycle), errors.Is(err, db.ErrFolderTooDeep), errors.Is(err, db.ErrNoFolders), errors.Is(err, db.ErrWorldNotFound):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	default:
		codexStoreError(w, err, what)
	}
}

// handleWorldFolders is the light folder list ({id, parent_id, name}) for the folder pickers.
func handleWorldFolders(w http.ResponseWriter, r *http.Request, apiWorldID string) {
	if _, ok := codexUserID(w, r); !ok {
		return
	}
	id := codexWorldParam(apiWorldID)
	out := []map[string]string{}
	if id != "" {
		worlds, _, err := codexWorlds()
		if err != nil {
			codexServerError(w, "retrieve worlds")
			return
		}
		if _, found := worlds[id]; !found {
			writeJSONError(w, http.StatusNotFound, db.ErrWorldNotFound.Error())
			return
		}
		folders, err := db.GetFolders(id)
		if err != nil {
			codexServerError(w, "retrieve folders")
			return
		}
		for _, f := range folders {
			out = append(out, map[string]string{"id": f.ID, "parent_id": f.ParentID, "name": f.Name})
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"folders": out})
}

func handleCreateFolder(w http.ResponseWriter, r *http.Request, apiWorldID string) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	name, valid := codexFolderName(req.Name)
	if !valid {
		writeJSONError(w, http.StatusBadRequest, codexFolderNameMsg)
		return
	}
	f := models.Folder{ID: db.GenerateUUID(), WorldID: codexWorldParam(apiWorldID), ParentID: req.ParentID, Name: name, CreatedBy: userID}
	if err := db.CreateFolder(f); err != nil {
		codexFolderError(w, err, "create folder")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"folder": f})
}

func handleCodexFolderRoutes(w http.ResponseWriter, r *http.Request, parts []string, method func(...string) bool) {
	if len(parts) != 1 || parts[0] == "" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	if !method(http.MethodPut, http.MethodDelete) {
		return
	}
	if _, ok := codexUserID(w, r); !ok {
		return
	}
	if r.Method == http.MethodDelete {
		pages, folders, err := db.DeleteFolder(parts[0])
		if err != nil {
			codexFolderError(w, err, "delete folder")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "success", "moved_pages": pages, "moved_folders": folders})
		return
	}
	// PUT renames ({name}) or moves ({parent_id}, "" = top level); one per request.
	var req struct {
		Name     *string `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	if req.Name == nil && req.ParentID == nil {
		writeJSONError(w, http.StatusBadRequest, "name or parent_id is required")
		return
	}
	if req.Name != nil && req.ParentID != nil {
		writeJSONError(w, http.StatusBadRequest, "send either name or parent_id, not both")
		return
	}
	if req.Name != nil {
		name, valid := codexFolderName(*req.Name)
		if !valid {
			writeJSONError(w, http.StatusBadRequest, codexFolderNameMsg)
			return
		}
		if err := db.RenameFolder(parts[0], name); err != nil {
			codexFolderError(w, err, "rename folder")
			return
		}
	}
	if req.ParentID != nil {
		if err := db.MoveFolder(parts[0], *req.ParentID); err != nil {
			codexFolderError(w, err, "move folder")
			return
		}
	}
	f, err := db.GetFolder(parts[0])
	if err != nil || f == nil {
		codexServerError(w, "retrieve folder")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"folder": f})
}

func handleSetPageFolder(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := codexUserID(w, r); !ok {
		return
	}
	var req struct {
		FolderID string `json:"folder_id"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	if err := db.SetPageFolder(id, req.FolderID); err != nil {
		codexStoreError(w, err, "move page to folder")
		return
	}
	page, err := db.GetPage(id)
	if err != nil || page == nil {
		codexServerError(w, "retrieve page")
		return
	}
	path, err := codexFolderPath(page.WorldID, page.FolderID)
	if err != nil {
		codexServerError(w, "retrieve folders")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"folder_id": page.FolderID, "folder_path": path})
}
