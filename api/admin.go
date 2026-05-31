package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"waystone-web/db"
	"waystone-web/middleware"
	"waystone-web/models"
)

// adminUserJSON is a view of a user for the admin API
type adminUserJSON struct {
	ID        string   `json:"id"`
	Email     string   `json:"email"`
	Name      string   `json:"name"`
	Nickname  string   `json:"nickname"`
	Roles     []string `json:"roles"`
	Blocked   bool     `json:"blocked"`
	Activated bool     `json:"activated"`
}

func toAdminUserJSON(u models.User) adminUserJSON {
	roles := u.Roles
	if roles == nil {
		roles = []string{}
	}
	return adminUserJSON{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Nickname:  u.Nickname,
		Roles:     roles,
		Blocked:   u.Blocked,
		Activated: u.GoogleID != "",
	}
}

// getAdminCaller loads the calling user from session and checks admin role.
func getAdminCaller(r *http.Request) (*models.User, error) {
	session, err := middleware.GetSession(r)
	if err != nil {
		return nil, err
	}
	userID, ok := session["user_id"].(string)
	if !ok {
		return nil, http.ErrNoCookie
	}
	u, err := db.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	if u == nil || !u.IsAdmin() {
		return nil, http.ErrNoCookie // reuse as "forbidden" sentinel
	}
	return u, nil
}

func adminForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]string{"error": "forbidden"})
}

func adminError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// countActiveAdmins returns the number of non-blocked admins.
func countActiveAdmins(users []models.User) int {
	n := 0
	for _, u := range users {
		if u.IsAdmin() && !u.Blocked {
			n++
		}
	}
	return n
}

// HandleAdminUsersPage serves admin-users.html (server-side admin gate).
func HandleAdminUsersPage(w http.ResponseWriter, r *http.Request) {
	caller, err := getAdminCaller(r)
	if err != nil || caller == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, "./static/admin-users.html")
}

// HandleAdminUsers handles GET /api/admin/users and POST /api/admin/users.
func HandleAdminUsers(w http.ResponseWriter, r *http.Request) {
	caller, err := getAdminCaller(r)
	if err != nil || caller == nil {
		adminForbidden(w)
		return
	}

	switch r.Method {
	case http.MethodGet:
		users, err := db.GetAllUsers()
		if err != nil {
			adminError(w, http.StatusInternalServerError, "failed to fetch users")
			return
		}
		result := make([]adminUserJSON, len(users))
		for i, u := range users {
			result[i] = toAdminUserJSON(u)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)

	case http.MethodPost:
		var body struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
			adminError(w, http.StatusBadRequest, "email required")
			return
		}
		existing, err := db.GetUserByEmail(body.Email)
		if err != nil {
			adminError(w, http.StatusInternalServerError, "failed to check email")
			return
		}
		if existing != nil {
			adminError(w, http.StatusConflict, "user already exists")
			return
		}
		newUser := models.User{
			Email: body.Email,
			Roles: []string{},
		}
		if err := db.SaveUser(newUser); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to create user")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "created"})

	default:
		adminError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleAdminUserActions handles sub-routes: /api/admin/users/{id} and /api/admin/users/{id}/{action}
func HandleAdminUserActions(w http.ResponseWriter, r *http.Request) {
	caller, err := getAdminCaller(r)
	if err != nil || caller == nil {
		adminForbidden(w)
		return
	}

	// Parse path: /api/admin/users/{id} or /api/admin/users/{id}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	parts := strings.SplitN(path, "/", 2)
	targetID := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	if targetID == "" {
		adminError(w, http.StatusBadRequest, "user id required")
		return
	}

	target, err := db.GetUserByID(targetID)
	if err != nil || target == nil {
		adminError(w, http.StatusNotFound, "user not found")
		return
	}

	switch {
	case r.Method == http.MethodDelete && action == "":
		// Guard: can't delete yourself
		if targetID == caller.ID {
			adminError(w, http.StatusConflict, "cannot delete your own account")
			return
		}
		// Guard: can't delete last admin
		if target.IsAdmin() {
			all, _ := db.GetAllUsers()
			if countActiveAdmins(all) <= 1 {
				adminError(w, http.StatusConflict, "cannot delete the last admin")
				return
			}
		}
		if _, _, err := db.DeleteUserAndCleanupCampaigns(targetID); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to delete user")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	case r.Method == http.MethodPost && action == "block":
		if targetID == caller.ID {
			adminError(w, http.StatusConflict, "cannot block yourself")
			return
		}
		if target.IsAdmin() {
			all, _ := db.GetAllUsers()
			if countActiveAdmins(all) <= 1 {
				adminError(w, http.StatusConflict, "cannot block the last admin")
				return
			}
		}
		if err := db.SetUserBlocked(targetID, true); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to block user")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "blocked"})

	case r.Method == http.MethodPost && action == "unblock":
		if err := db.SetUserBlocked(targetID, false); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to unblock user")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "unblocked"})

	case r.Method == http.MethodPost && action == "make-admin":
		if target.IsAdmin() {
			// idempotent no-op
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "already admin"})
			return
		}
		newRoles := append(target.Roles, "admin")
		if err := db.UpdateUserRoles(targetID, newRoles); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to update roles")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "promoted"})

	case r.Method == http.MethodPost && action == "remove-admin":
		if !target.IsAdmin() {
			// idempotent no-op
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "not admin"})
			return
		}
		if targetID == caller.ID {
			adminError(w, http.StatusConflict, "cannot remove your own admin role")
			return
		}
		all, _ := db.GetAllUsers()
		if countActiveAdmins(all) <= 1 {
			adminError(w, http.StatusConflict, "cannot remove the last admin")
			return
		}
		newRoles := []string{}
		for _, role := range target.Roles {
			if role != "admin" {
				newRoles = append(newRoles, role)
			}
		}
		if err := db.UpdateUserRoles(targetID, newRoles); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to update roles")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "demoted"})

	default:
		adminError(w, http.StatusNotFound, "unknown action")
	}
}
