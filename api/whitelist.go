package api

import (
	"encoding/json"
	"fmt"
	"log"
	"waystone-web/db"
	"waystone-web/middleware"
	"waystone-web/models"
	"net/http"
	"time"
)

func HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	// Check authentication
	session, err := middleware.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login.html", http.StatusSeeOther)
		return
	}

	// Check admin role
	user, err := getUserFromSession(session)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "forbidden",
		})
		return
	}

	if !user.IsAdmin() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "forbidden",
		})
		return
	}

	// Parse request body
	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "invalid request body",
		})
		return
	}

	if req.Email == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "email is required",
		})
		return
	}

	// Check if user already exists
	existingUser, err := db.GetUserByEmail(req.Email)
	if err != nil {
		log.Printf("error checking existing user: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "internal server error",
		})
		return
	}

	if existingUser != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "user already exists",
		})
		return
	}

	// Create new user
	newUser := models.User{
		Email:     req.Email,
		Name:      req.Name,
		Picture:   "",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Roles:     []string{},
	}

	if err := db.SaveUser(newUser); err != nil {
		log.Printf("failed to save user: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "failed to create user",
		})
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"user": map[string]interface{}{
			"email":      newUser.Email,
			"name":       newUser.Name,
			"created_at": newUser.CreatedAt,
			"roles":      newUser.Roles,
		},
	})
}

// Helper function to get user from session
func getUserFromSession(session map[interface{}]interface{}) (*models.User, error) {
	userID, ok := session["user_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid session: missing user_id")
	}

	// Get all users and find by ID (since we store by ID now)
	users, err := db.GetAllUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %v", err)
	}

	for i := range users {
		if users[i].ID == userID {
			return &users[i], nil
		}
	}

	return nil, fmt.Errorf("user not found in database")
}
