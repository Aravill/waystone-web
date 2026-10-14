package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

func HandleCampaigns(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		HandleGetCampaigns(w, r)
	} else if r.Method == http.MethodPost {
		HandleCreateCampaign(w, r)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, `{"error": "method not allowed"}`)
	}
}

func HandleGetCampaigns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	campaigns, err := db.GetAllCampaigns()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to retrieve campaigns"}`)
		return
	}

	// Fetch all users once to avoid N+1 queries
	allUsers, err := db.GetAllUsers()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to retrieve users"}`)
		return
	}

	currentUserID, _ := r.Context().Value("user_id").(string)
	userSignups, err := db.GetCampaignSignupsByUser(currentUserID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to retrieve sign-ups"}`)
		return
	}

	knownSystems, err := systemsByName()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to retrieve systems"}`)
		return
	}

	// Create a map for O(1) user lookup
	userMap := make(map[string]*models.User)
	for i := range allUsers {
		userMap[allUsers[i].ID] = &allUsers[i]
	}

	isAdmin := false
	if currentUser, ok := userMap[currentUserID]; ok && currentUser != nil {
		isAdmin = currentUser.IsAdmin()
	}

	// Build enriched campaign objects
	enriched := []map[string]interface{}{}
	for _, campaign := range campaigns {
		campaignObj := map[string]interface{}{
			"id":                         campaign.ID,
			"title":                      campaign.Title,
			"status":                     campaign.Status,
			"type":                       campaign.Type,
			"summary":                    campaign.Summary,
			"description":                campaign.Description,
			"dm":                         campaign.DM,
			"players":                    campaign.Players,
			"sign_ups_open":              campaign.SignUpsOpen,
			"desired_player_count":       campaign.DesiredPlayerCount,
			"system":                     campaign.System,
			"system_logo_url":            knownSystems[strings.ToLower(campaign.System)].LogoURL(),
			"can_manage":                 isAdmin || (currentUserID != "" && campaign.DM == currentUserID),
			"allowed_status_transitions": campaign.Status.AllowedTransitions(),
		}
		campaignObj["current_user_signup_status"] = string(userSignups[campaign.ID].Status)

		// Add DM user display object if DM exists
		if campaign.DM != "" {
			if dmUser, ok := userMap[campaign.DM]; ok && dmUser != nil {
				campaignObj["dm_user"] = map[string]interface{}{
					"id":           dmUser.ID,
					"name":         dmUser.Name,
					"nickname":     dmUser.Nickname,
					"display_name": dmUser.DisplayName(),
					"profile_url":  fmt.Sprintf("/profile?user_id=%s", dmUser.ID),
				}
			}
		}

		// Add player user display objects
		playerUsers := []map[string]interface{}{}
		for _, playerID := range campaign.Players {
			if playerUser, ok := userMap[playerID]; ok && playerUser != nil {
				playerUsers = append(playerUsers, map[string]interface{}{
					"id":           playerUser.ID,
					"name":         playerUser.Name,
					"nickname":     playerUser.Nickname,
					"display_name": playerUser.DisplayName(),
					"profile_url":  fmt.Sprintf("/profile?user_id=%s", playerUser.ID),
				})
			}
		}
		campaignObj["player_users"] = playerUsers

		enriched = append(enriched, campaignObj)
	}

	w.WriteHeader(http.StatusOK)
	data, _ := json.Marshal(enriched)
	w.Write(data)
}

func HandleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, `{"error": "method not allowed"}`)
		return
	}

	// Get authenticated user from context
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error": "not authenticated"}`)
		return
	}

	var req campaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "invalid request body"}`)
		return
	}
	if msg := req.normalize(); msg != "" {
		w.WriteHeader(http.StatusBadRequest)
		data, _ := json.Marshal(map[string]string{"error": msg})
		w.Write(data)
		return
	}

	system, err := canonicalSystemName(req.System)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to retrieve systems"}`)
		return
	}

	// Create campaign with DM set to authenticated user
	campaign := models.Campaign{
		ID:                 db.GetNextCampaignID(),
		Title:              req.Title,
		Summary:            req.Summary,
		Description:        req.Description,
		DesiredPlayerCount: playerCountString(req.DesiredPlayerCount),
		DM:                 userID,
		Players:            []string{},
		Status:             models.Pitch,
		Type:               req.Type,
		System:             system,
		SignUpsOpen:        true,
	}

	// Save campaign to database
	if err := db.SaveCampaign(campaign); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to create campaign"}`)
		return
	}

	w.WriteHeader(http.StatusCreated)
	data, _ := json.Marshal(map[string]interface{}{
		"status":      "success",
		"message":     "campaign created successfully",
		"campaign_id": campaign.ID,
	})
	w.Write(data)
}

// campaignRequest is the body for creating and updating a campaign.
type campaignRequest struct {
	Title              string              `json:"title"`
	Summary            string              `json:"summary"`
	Description        string              `json:"description"`
	Type               models.CampaignType `json:"type"`
	DesiredPlayerCount int                 `json:"desired_player_count"`
	System             string              `json:"system"`
}

// normalize trims the text fields and returns a validation error message, or "" if valid.
func (req *campaignRequest) normalize() string {
	req.Title = strings.TrimSpace(req.Title)
	req.Summary = strings.TrimSpace(req.Summary)
	req.Description = strings.TrimSpace(req.Description)
	req.System = strings.TrimSpace(req.System)
	if req.Type == "" {
		req.Type = models.LongCampaign
	}
	switch {
	case req.Title == "":
		return "title is required"
	case req.Summary == "":
		return "summary is required"
	case req.Description == "":
		return "description is required"
	case !req.Type.IsValid():
		return "invalid campaign type"
	case req.DesiredPlayerCount < 0:
		return "desired player count cannot be negative"
	case utf8.RuneCountInString(req.System) > 64:
		return "system must be at most 64 characters"
	}
	return ""
}

// playerCountString stores 0 (unlimited) as an empty string.
func playerCountString(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// loadManagedCampaign loads the campaign and allows only its DM or an admin.
func loadManagedCampaign(w http.ResponseWriter, r *http.Request, campaignID string) (*models.Campaign, bool) {
	campaign, userID, ok := loadCampaignAndUser(w, r, campaignID)
	if !ok {
		return nil, false
	}
	if campaign.DM == userID {
		return campaign, true
	}
	user, err := db.GetUserByID(userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve user")
		return nil, false
	}
	if user == nil || !user.IsAdmin() {
		writeJSONError(w, http.StatusForbidden, "only the campaign DM or an admin can manage this campaign")
		return nil, false
	}
	return campaign, true
}

// HandleCampaignActions handles /api/campaigns/{id} (PUT update, DELETE remove).
func HandleCampaignActions(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) != 1 || parts[0] == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	switch r.Method {
	case http.MethodPut:
		handleUpdateCampaign(w, r, parts[0])
	case http.MethodDelete:
		handleDeleteCampaign(w, r, parts[0])
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func handleUpdateCampaign(w http.ResponseWriter, r *http.Request, campaignID string) {
	campaign, ok := loadManagedCampaign(w, r, campaignID)
	if !ok {
		return
	}

	var req campaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := req.normalize(); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	if req.DesiredPlayerCount > 0 && req.DesiredPlayerCount < len(campaign.Players) {
		writeJSONError(w, http.StatusConflict, "desired player count is lower than the current number of players")
		return
	}

	system, err := canonicalSystemName(req.System)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve systems")
		return
	}

	if err := db.UpdateCampaignDetails(campaignID, req.Title, req.Summary, req.Description, req.Type, system, playerCountString(req.DesiredPlayerCount)); err != nil {
		if errors.Is(err, db.ErrCampaignNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to update campaign")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "campaign updated"})
}

func handleDeleteCampaign(w http.ResponseWriter, r *http.Request, campaignID string) {
	if _, ok := loadManagedCampaign(w, r, campaignID); !ok {
		return
	}
	if err := db.DeleteCampaign(campaignID); err != nil {
		if errors.Is(err, db.ErrCampaignNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to delete campaign")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "campaign deleted"})
}

// HandleCampaignStatus handles /api/campaigns/{id}/status (PUT change status).
func HandleCampaignStatus(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) != 2 || parts[1] != "status" {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if r.Method != http.MethodPut {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	campaign, ok := loadManagedCampaign(w, r, parts[0])
	if !ok {
		return
	}

	var req struct {
		Status models.CampaignStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Status == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !campaign.Status.CanTransitionTo(req.Status) {
		writeJSONError(w, http.StatusConflict, fmt.Sprintf("cannot change status from %s to %s", campaign.Status, req.Status))
		return
	}

	if err := db.SetCampaignStatus(parts[0], req.Status); err != nil {
		if errors.Is(err, db.ErrCampaignNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to update status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":                     "success",
		"message":                    "status updated",
		"campaign_status":            req.Status,
		"allowed_status_transitions": req.Status.AllowedTransitions(),
	})
}
