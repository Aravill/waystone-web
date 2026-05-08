package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"waystone-web/db"
	"waystone-web/middleware"
)

func HandleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		handleGetProfile(w, r)
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
	displayName := user.Nickname
	if displayName == "" {
		displayName = user.Name
	}
	if displayName == "" {
		displayName = user.Email
	}

	// Compute initials from display name
	initials := computeInitials(displayName)

	// Check if this is the current user's profile
	sessionUserID, _ := session["user_id"].(string)
	isSelf := sessionUserID == user.ID

	// Build user object; only include email for self
	userObj := map[string]interface{}{
		"id":       user.ID,
		"name":     user.Name,
		"nickname": user.Nickname,
		"picture":  user.Picture,
	}
	if isSelf {
		userObj["email"] = user.Email
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user": userObj,
		"avatar": map[string]interface{}{
			"has_picture": user.Picture != "",
			"picture":     user.Picture,
			"initials":    initials,
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

	// Delete user and cleanup campaigns
	removedAsDM, removedAsPlayer, err := db.DeleteUserAndCleanupCampaigns(userID)
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
			"removed_as_dm":     removedAsDM,
			"removed_as_player": removedAsPlayer,
		},
	})
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
