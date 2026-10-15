package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/middleware"
)

func HandleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		handleGetProfile(w, r)
	} else if r.Method == http.MethodPut {
		handleUpdateProfile(w, r)
	} else if r.Method == http.MethodDelete {
		handleDeleteProfile(w, r)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
	}
}

func handleGetProfile(w http.ResponseWriter, r *http.Request) {
	session, err := middleware.GetSession(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "not authenticated"})
		return
	}

	userID, ok := session["user_id"].(string)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid session"})
		return
	}

	// Check if query param user_id is provided
	queryUserID := r.URL.Query().Get("user_id")
	if queryUserID != "" {
		userID = queryUserID
	}

	// Fetch requested user
	user, err := db.GetUserByID(userID)
	if err != nil {
		log.Printf("error fetching user: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch user"})
		return
	}

	if user == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "user not found"})
		return
	}

	// Load all campaigns and filter
	campaigns, err := db.GetAllCampaigns()
	if err != nil {
		log.Printf("error fetching campaigns: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch campaigns"})
		return
	}

	dmCampaigns := []map[string]interface{}{}
	playingCampaigns := []map[string]interface{}{}

	for _, campaign := range campaigns {
		campaignObj := map[string]interface{}{
			"id":     campaign.ID,
			"title":  campaign.Title,
			"status": campaign.Status,
		}

		if campaign.DM == user.ID {
			dmCampaigns = append(dmCampaigns, campaignObj)
		}

		for _, playerID := range campaign.Players {
			if playerID == user.ID {
				playingCampaigns = append(playingCampaigns, campaignObj)
				break
			}
		}
	}

	// Compute display_name
	displayName := user.DisplayName()

	// Compute initials from display name
	initials := computeInitials(displayName)

	// Check if this is the current user's profile
	sessionUserID, _ := session["user_id"].(string)
	isSelf := sessionUserID == user.ID

	userObj := map[string]interface{}{
		"id":       user.ID,
		"username": user.Username,
		"name":     user.Name,
		"nickname": user.Nickname,
		"deleted":  false,
	}
	avatarURL := user.AvatarURL()
	if user.Deleted {
		userObj = map[string]interface{}{"id": user.ID, "deleted": true}
		avatarURL = ""
		initials = "?"
	}

	stats := map[string]interface{}{}
	if !user.Deleted {
		playedCampaigns := map[string]bool{}
		for _, c := range playingCampaigns {
			playedCampaigns[c["id"].(string)] = true
		}
		sessionsPlayed := 0
		accepted, err := db.GetAcceptedConfirmedSessions(user.ID)
		if err != nil {
			log.Printf("error fetching played sessions: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to fetch sessions")
			return
		}
		now := time.Now()
		for _, session := range accepted {
			start, ok := sessionStart(session)
			if !ok || start.Add(time.Duration(session.Duration)*time.Minute).After(now) {
				continue
			}
			sessionsPlayed++
			playedCampaigns[session.CampaignID] = true
		}
		stats = map[string]interface{}{
			"campaigns_played": len(playedCampaigns),
			"sessions_played":  sessionsPlayed,
			"member_since":     user.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stats": stats,
		"user":  userObj,
		"avatar": map[string]interface{}{
			"initials": initials,
			"url":      avatarURL,
		},
		"is_self": isSelf,
		"campaigns": map[string]interface{}{
			"dm":      dmCampaigns,
			"playing": playingCampaigns,
		},
		"display_name": displayName,
	})
}

func handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	session, err := middleware.GetSession(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "not authenticated"})
		return
	}

	userID, ok := session["user_id"].(string)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid session"})
		return
	}

	// Anonymize user and cleanup campaigns
	removedAsPlayer, err := db.AnonymizeUserAndCleanupCampaigns(userID)
	if err != nil {
		log.Printf("error deleting user: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": "failed to delete account",
		})
		return
	}

	// Clear session
	if err := middleware.ClearSession(w, r); err != nil {
		log.Printf("error clearing session: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          "success",
		"message":         "Account deleted",
		"deleted_user_id": userID,
		"campaign_cleanup": map[string]int{
			"removed_as_player": removedAsPlayer,
		},
	})
}

func handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := sessionUserID(w, r)
	if !ok {
		return
	}

	var body struct {
		Name     *string `json:"name"`
		Nickname *string `json:"nickname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Name == nil && body.Nickname == nil {
		writeJSONError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	if body.Name != nil {
		trimmed := strings.TrimSpace(*body.Name)
		if utf8.RuneCountInString(trimmed) > 64 {
			writeJSONError(w, http.StatusBadRequest, "name must be 64 characters or fewer")
			return
		}
		body.Name = &trimmed
	}
	if body.Nickname != nil {
		trimmed := strings.TrimSpace(*body.Nickname)
		if utf8.RuneCountInString(trimmed) > 32 {
			writeJSONError(w, http.StatusBadRequest, "nickname must be 32 characters or fewer")
			return
		}
		body.Nickname = &trimmed
	}

	if err := db.SetUserProfile(userID, body.Name, body.Nickname); err != nil {
		if errors.Is(err, db.ErrUserNotFound) {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		log.Printf("error updating profile: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}
	completeProfileSetup(userID)

	user, err := db.GetUserByID(userID)
	if err != nil || user == nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":       "success",
		"message":      "profile updated",
		"name":         user.Name,
		"nickname":     user.Nickname,
		"display_name": user.DisplayName(),
	})
}

// HandleProfileSetupComplete clears the session user's first-login profile setup flag.
func HandleProfileSetupComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID, ok := sessionUserID(w, r)
	if !ok {
		return
	}
	if err := db.CompleteProfileSetup(userID); err != nil {
		if errors.Is(err, db.ErrUserNotFound) {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		log.Printf("error completing profile setup: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to complete profile setup")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "profile setup complete"})
}

// completeProfileSetup clears the first-login setup flag after the user edits their profile.
// Errors are logged, never returned: the edit itself already succeeded.
func completeProfileSetup(userID string) {
	if err := db.CompleteProfileSetup(userID); err != nil && !errors.Is(err, db.ErrUserNotFound) {
		log.Printf("error completing profile setup: %v", err)
	}
}

// sessionUserID returns the session user's ID, or writes a 401 and returns false.
func sessionUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	session, err := middleware.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return "", false
	}
	userID, ok := session["user_id"].(string)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "invalid session")
		return "", false
	}
	return userID, true
}

const maxAvatarBytes = 1 << 20

// HandleProfileAvatar handles PUT (upload) and DELETE (remove) of the session user's avatar.
func HandleProfileAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID, ok := sessionUserID(w, r)
	if !ok {
		return
	}

	var data []byte
	var contentType, message string
	if r.Method == http.MethodPut {
		r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes)
		var err error
		data, err = io.ReadAll(r.Body)
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				writeJSONError(w, http.StatusRequestEntityTooLarge, "avatar must be 1 MB or smaller")
				return
			}
			writeJSONError(w, http.StatusBadRequest, "failed to read avatar")
			return
		}
		if len(data) == 0 {
			writeJSONError(w, http.StatusBadRequest, "avatar file is empty")
			return
		}
		contentType = http.DetectContentType(data)
		if !allowedLogoTypes[contentType] {
			writeJSONError(w, http.StatusBadRequest, "avatar must be a PNG, JPEG, WebP or GIF image")
			return
		}
		message = "avatar updated"
	} else {
		message = "avatar removed"
	}

	if err := db.SetUserAvatar(userID, data, contentType); err != nil {
		if errors.Is(err, db.ErrUserNotFound) {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		log.Printf("error saving avatar: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to save avatar")
		return
	}
	if len(data) > 0 {
		completeProfileSetup(userID)
	}

	avatarURL := ""
	if user, err := db.GetUserByID(userID); err == nil && user != nil {
		avatarURL = user.AvatarURL()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": message, "avatar_url": avatarURL})
}

// HandleUserRoutes handles GET /api/users/{id}/avatar.
func HandleUserRoutes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/users/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "avatar" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	data, contentType, err := db.GetUserAvatar(parts[0])
	if errors.Is(err, db.ErrUserNotFound) || (err == nil && len(data) == 0) {
		writeJSONError(w, http.StatusNotFound, "avatar not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve avatar")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func computeInitials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "?"
	}

	initials := []rune{}
	for _, part := range parts {
		runes := []rune(part)
		if len(runes) > 0 {
			initials = append(initials, runes[0])
		}
		if len(initials) == 2 {
			break
		}
	}

	if len(initials) == 0 {
		return "?"
	}

	return strings.ToUpper(string(initials))
}
