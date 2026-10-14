package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"waystone-web/db"
	"waystone-web/models"
)

type signupPayload struct {
	ID         string                 `json:"id"`
	CampaignID string                 `json:"campaign_id"`
	UserID     string                 `json:"user_id"`
	Status     models.SignupStatus    `json:"status"`
	CreatedAt  string                 `json:"created_at"`
	UpdatedAt  string                 `json:"updated_at"`
	SignupUser map[string]interface{} `json:"signup_user"`
}

// HandleCampaignSignups handles /api/campaigns/{id}/signups (GET list, POST create).
func HandleCampaignSignups(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) != 2 || parts[1] != "signups" {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	switch r.Method {
	case http.MethodGet:
		handleListSignups(w, r, parts[0])
	case http.MethodPost:
		handleCreateSignup(w, r, parts[0])
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleCampaignSignupActions handles /api/campaigns/{id}/signups/{userID} (PUT accept/decline).
func HandleCampaignSignupActions(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) != 3 || parts[1] != "signups" {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	if r.Method != http.MethodPut {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	handleUpdateSignup(w, r, parts[0], parts[2])
}

// HandleCampaignPlayerActions handles /api/campaigns/{id}/players/{userID} (DELETE kick).
func HandleCampaignPlayerActions(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) != 3 || parts[1] != "players" {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	if r.Method != http.MethodDelete {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	handleKickPlayer(w, r, parts[0], parts[2])
}

// HandleCampaignSignupsOpen handles /api/campaigns/{id}/signups-open (PUT open/close).
func HandleCampaignSignupsOpen(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) != 2 || parts[1] != "signups-open" {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	if r.Method != http.MethodPut {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	campaign, userID, ok := loadCampaignAndUser(w, r, parts[0])
	if !ok {
		return
	}
	if campaign.DM != userID {
		writeJSONError(w, http.StatusForbidden, "only the campaign DM can open or close sign-ups")
		return
	}

	var req struct {
		Open *bool `json:"open"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Open == nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := db.SetCampaignSignUpsOpen(parts[0], *req.Open); err != nil {
		if errors.Is(err, db.ErrCampaignNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to update sign-ups")
		return
	}

	message := "sign-ups closed"
	if *req.Open {
		message = "sign-ups opened"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":        "success",
		"message":       message,
		"sign_ups_open": *req.Open,
	})
}

// loadCampaignAndUser authenticates the caller and loads the campaign, writing
// an error response and returning ok=false on failure.
func loadCampaignAndUser(w http.ResponseWriter, r *http.Request, campaignID string) (*models.Campaign, string, bool) {
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return nil, "", false
	}

	campaign, err := db.GetCampaignByID(campaignID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve campaign")
		return nil, "", false
	}
	if campaign == nil {
		writeJSONError(w, http.StatusNotFound, "campaign not found")
		return nil, "", false
	}
	return campaign, userID, true
}

func userDisplayObject(user *models.User) map[string]interface{} {
	return map[string]interface{}{
		"id":           user.ID,
		"name":         user.Name,
		"nickname":     user.Nickname,
		"display_name": user.DisplayName(),
		"profile_url":  fmt.Sprintf("/profile?user_id=%s", user.ID),
	}
}

func handleListSignups(w http.ResponseWriter, r *http.Request, campaignID string) {
	campaign, userID, ok := loadCampaignAndUser(w, r, campaignID)
	if !ok {
		return
	}
	if campaign.DM != userID {
		writeJSONError(w, http.StatusForbidden, "only the campaign DM can view sign-ups")
		return
	}

	signups, err := db.GetCampaignSignups(campaignID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sign-ups")
		return
	}

	allUsers, err := db.GetAllUsers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve users")
		return
	}
	userMap := make(map[string]*models.User, len(allUsers))
	for i := range allUsers {
		userMap[allUsers[i].ID] = &allUsers[i]
	}

	enrich := func(signup models.CampaignSignup) signupPayload {
		payload := signupPayload{
			ID:         signup.ID,
			CampaignID: signup.CampaignID,
			UserID:     signup.UserID,
			Status:     signup.Status,
			CreatedAt:  signup.CreatedAt,
			UpdatedAt:  signup.UpdatedAt,
		}
		if user, ok := userMap[signup.UserID]; ok && user != nil {
			payload.SignupUser = userDisplayObject(user)
		} else {
			payload.SignupUser = map[string]interface{}{"id": signup.UserID, "display_name": signup.UserID}
		}
		return payload
	}

	result := make([]signupPayload, 0, len(signups))
	hasRow := map[string]bool{}
	for _, signup := range signups {
		hasRow[signup.UserID] = true
		result = append(result, enrich(signup))
	}
	// Players added before sign-ups existed (e.g. seed data) have no row.
	for _, playerID := range campaign.Players {
		if !hasRow[playerID] {
			result = append(result, enrich(models.CampaignSignup{
				CampaignID: campaignID,
				UserID:     playerID,
				Status:     models.SignupAccepted,
			}))
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"signups":              result,
		"accepted_count":       len(campaign.Players),
		"desired_player_count": campaign.DesiredPlayerCount,
		"sign_ups_open":        campaign.SignUpsOpen,
	})
}

func handleCreateSignup(w http.ResponseWriter, r *http.Request, campaignID string) {
	campaign, userID, ok := loadCampaignAndUser(w, r, campaignID)
	if !ok {
		return
	}
	if campaign.DM == userID {
		writeJSONError(w, http.StatusForbidden, "the campaign DM cannot sign up")
		return
	}
	for _, playerID := range campaign.Players {
		if playerID == userID {
			writeJSONError(w, http.StatusConflict, "already a player")
			return
		}
	}

	existing, err := db.GetCampaignSignup(campaignID, userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to check sign-up")
		return
	}
	if existing != nil {
		switch existing.Status {
		case models.SignupDeclined:
			writeJSONError(w, http.StatusForbidden, "your sign-up was declined")
		case models.SignupKicked:
			writeJSONError(w, http.StatusForbidden, "you were removed from this campaign")
		default:
			writeJSONError(w, http.StatusConflict, "already signed up")
		}
		return
	}
	if !campaign.SignUpsOpen {
		writeJSONError(w, http.StatusConflict, "sign-ups are closed")
		return
	}

	signup := models.CampaignSignup{
		ID:         db.GenerateUUID(),
		CampaignID: campaignID,
		UserID:     userID,
		Status:     models.SignupPending,
	}
	signup.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	signup.UpdatedAt = signup.CreatedAt
	if err := db.CreateCampaignSignup(signup); err != nil {
		// A UNIQUE violation means a concurrent duplicate request.
		writeJSONError(w, http.StatusConflict, "could not sign up")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status":  "success",
		"message": "signed up successfully",
		"signup":  signup,
	})
}

func handleUpdateSignup(w http.ResponseWriter, r *http.Request, campaignID, targetUserID string) {
	campaign, userID, ok := loadCampaignAndUser(w, r, campaignID)
	if !ok {
		return
	}
	if campaign.DM != userID {
		writeJSONError(w, http.StatusForbidden, "only the campaign DM can manage sign-ups")
		return
	}

	var req struct {
		Status models.SignupStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Status != models.SignupAccepted && req.Status != models.SignupDeclined {
		writeJSONError(w, http.StatusBadRequest, "status must be Accepted or Declined")
		return
	}

	applySignupStatus(w, campaignID, targetUserID, req.Status, "sign-up updated")
}

func handleKickPlayer(w http.ResponseWriter, r *http.Request, campaignID, targetUserID string) {
	campaign, userID, ok := loadCampaignAndUser(w, r, campaignID)
	if !ok {
		return
	}
	if campaign.DM != userID {
		writeJSONError(w, http.StatusForbidden, "only the campaign DM can remove players")
		return
	}

	isPlayer := false
	for _, playerID := range campaign.Players {
		if playerID == targetUserID {
			isPlayer = true
			break
		}
	}
	if !isPlayer {
		writeJSONError(w, http.StatusNotFound, "player not found")
		return
	}

	applySignupStatus(w, campaignID, targetUserID, models.SignupKicked, "player removed")
}

func applySignupStatus(w http.ResponseWriter, campaignID, targetUserID string, status models.SignupStatus, message string) {
	if err := db.SetCampaignSignupStatus(campaignID, targetUserID, status); err != nil {
		switch {
		case errors.Is(err, db.ErrSignupNotFound), errors.Is(err, db.ErrCampaignNotFound):
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, db.ErrCampaignFull):
			writeJSONError(w, http.StatusConflict, err.Error())
		default:
			writeJSONError(w, http.StatusInternalServerError, "failed to update sign-up")
		}
		return
	}

	campaign, err := db.GetCampaignByID(campaignID)
	if err != nil || campaign == nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve campaign")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":        "success",
		"message":       message,
		"players":       campaign.Players,
		"sign_ups_open": campaign.SignUpsOpen,
	})
}
