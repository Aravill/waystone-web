package api

import (
	"net/http"
	"strings"
	"waystone-web/db"
	"waystone-web/models"
)

// HandleAdminAccessPage serves admin-access.html (server-side admin gate).
func HandleAdminAccessPage(w http.ResponseWriter, r *http.Request) {
	caller, err := getAdminCaller(r)
	if err != nil || caller == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, "./static/admin-access.html")
}

// HandleAdminAccess serves GET /api/admin/access: the non-admin users, the codices and every grant.
func HandleAdminAccess(w http.ResponseWriter, r *http.Request) {
	if caller, err := getAdminCaller(r); err != nil || caller == nil {
		adminForbidden(w)
		return
	}
	if r.Method != http.MethodGet {
		adminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	users, err := db.GetAllUsers()
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to fetch users")
		return
	}
	rights, err := db.GetAllUserRights()
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to fetch rights")
		return
	}
	codices, err := db.GetCodices()
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to fetch codices")
		return
	}
	access, err := db.GetAllCodexAccess()
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to fetch access")
		return
	}
	userList := []map[string]interface{}{}
	for _, u := range users {
		if u.Deleted || u.IsAdmin() {
			continue
		}
		userRights := rights[u.ID]
		if userRights == nil {
			userRights = []string{}
		}
		userList = append(userList, map[string]interface{}{
			"id": u.ID, "username": u.Username, "display_name": u.DisplayName(),
			"blocked": u.Blocked, "rights": userRights,
		})
	}
	codexList := make([]map[string]string, 0, len(codices))
	for _, c := range codices {
		codexList = append(codexList, map[string]string{"id": c.ID, "name": c.Name})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": userList, "codices": codexList, "access": access})
}

// accessTarget loads the user a grant is for; it must exist and not be an admin.
func accessTarget(w http.ResponseWriter, userID string) bool {
	u, err := db.GetUserByID(userID)
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to fetch user")
		return false
	}
	if u == nil || u.Deleted {
		adminError(w, http.StatusNotFound, "user not found")
		return false
	}
	if u.IsAdmin() {
		adminError(w, http.StatusBadRequest, "admins always have every right")
		return false
	}
	return true
}

// HandleAdminAccessActions serves PUT /api/admin/access/users/{userID} ({action, on}) and
// PUT /api/admin/access/codices/{codexID}/users/{userID} ({level}: none, see or edit).
func HandleAdminAccessActions(w http.ResponseWriter, r *http.Request) {
	if caller, err := getAdminCaller(r); err != nil || caller == nil {
		adminForbidden(w)
		return
	}
	if r.Method != http.MethodPut {
		adminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/access/"), "/"), "/")
	switch {
	case len(parts) == 2 && parts[0] == "users":
		var req struct {
			Action string `json:"action"`
			On     bool   `json:"on"`
		}
		if !codexDecode(w, r, 4<<10, &req) {
			return
		}
		known := false
		for _, a := range models.UserRights {
			known = known || a == req.Action
		}
		if !known {
			adminError(w, http.StatusBadRequest, "unknown action")
			return
		}
		if !accessTarget(w, parts[1]) {
			return
		}
		if err := db.SetUserRight(parts[1], req.Action, req.On); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to save right")
			return
		}
	case len(parts) == 4 && parts[0] == "codices" && parts[2] == "users":
		var req struct {
			Level string `json:"level"`
		}
		if !codexDecode(w, r, 4<<10, &req) {
			return
		}
		if req.Level != "none" && req.Level != models.CodexLevelSee && req.Level != models.CodexLevelEdit {
			adminError(w, http.StatusBadRequest, "level must be none, see or edit")
			return
		}
		codex, err := db.GetCodex(parts[1])
		if err != nil {
			adminError(w, http.StatusInternalServerError, "failed to fetch codex")
			return
		}
		if codex == nil {
			adminError(w, http.StatusNotFound, db.ErrCodexNotFound.Error())
			return
		}
		if !accessTarget(w, parts[3]) {
			return
		}
		if err := db.SetCodexAccess(parts[1], parts[3], req.Level); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to save access")
			return
		}
	default:
		adminError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}
