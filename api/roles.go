package api

import (
	"encoding/json"
	"fmt"
	"log"
	"waystone-web/db"
	"waystone-web/middleware"
	"net/http"
)

type AssignRolesRequest struct {
	Email string   `json:"email"`
	Roles []string `json:"roles"`
}

type UserRolesResponse struct {
	Email string   `json:"email"`
	Roles []string `json:"roles"`
	Name  string   `json:"name"`
}

// HandleAssignRoles assigns roles to a user (admin only)
func HandleAssignRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check if requester is admin
	session, err := middleware.GetSession(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error": "not authenticated"}`)
		return
	}

	// Get current user to check if admin
	currentUser, err := db.GetUserByEmail(session["email"].(string))
	if err != nil || currentUser == nil || !currentUser.IsAdmin() {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"error": "only admins can assign roles"}`)
		return
	}

	// Parse request
	var req AssignRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "invalid request"}`)
		return
	}

	if req.Email == "" || len(req.Roles) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "email and roles are required"}`)
		return
	}

	// Find user by email
	user, err := db.GetUserByEmail(req.Email)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to find user"}`)
		return
	}

	if user == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error": "user not found"}`)
		return
	}

	// Update roles
	if err := db.UpdateUserRoles(user.ID, req.Roles); err != nil {
		log.Printf("Failed to update user roles: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to update roles"}`)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Roles updated for %s", req.Email),
		"roles":   req.Roles,
	})
}

// HandleGetUserRoles gets a user's roles (admin or self)
func HandleGetUserRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Get email from query parameter
	email := r.URL.Query().Get("email")
	if email == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "email parameter required"}`)
		return
	}

	// Check if requester is authenticated
	session, err := middleware.GetSession(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error": "not authenticated"}`)
		return
	}

	// Check if requester can view this user's roles (admin or self)
	currentUserEmail := session["email"].(string)
	if currentUserEmail != email {
		currentUser, err := db.GetUserByEmail(currentUserEmail)
		if err != nil || currentUser == nil || !currentUser.IsAdmin() {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `{"error": "can only view own roles or need admin access"}`)
			return
		}
	}

	// Get user
	user, err := db.GetUserByEmail(email)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to fetch user"}`)
		return
	}

	if user == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error": "user not found"}`)
		return
	}

	// Ensure roles slice is not nil
	roles := user.Roles
	if roles == nil {
		roles = []string{}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(UserRolesResponse{
		Email: user.Email,
		Name:  user.Name,
		Roles: roles,
	})
}

// HandleListUsers lists all users with their roles (admin only)
func HandleListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check if requester is admin
	session, err := middleware.GetSession(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error": "not authenticated"}`)
		return
	}

	currentUser, err := db.GetUserByEmail(session["email"].(string))
	if err != nil || currentUser == nil || !currentUser.IsAdmin() {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"error": "only admins can list users"}`)
		return
	}

	// Get all users
	users, err := db.GetAllUsers()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to fetch users"}`)
		return
	}

	// Convert to response format
	var responses []UserRolesResponse
	for _, user := range users {
		roles := user.Roles
		if roles == nil {
			roles = []string{}
		}
		responses = append(responses, UserRolesResponse{
			Email: user.Email,
			Name:  user.Name,
			Roles: roles,
		})
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(responses)
}
