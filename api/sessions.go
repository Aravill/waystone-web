package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"waystone-web/db"
	"waystone-web/models"
)

type sessionResponsePayload struct {
	ID            string                     `json:"id"`
	SessionID     string                     `json:"session_id"`
	UserID        string                     `json:"user_id"`
	Participation models.ParticipationStatus `json:"participation"`
	User          map[string]interface{}     `json:"user,omitempty"`
	CreatedAt     string                     `json:"created_at"`
	UpdatedAt     string                     `json:"updated_at"`
}

func HandleSessions(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) < 2 || parts[1] != "sessions" {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	campaignID := parts[0]
	switch r.Method {
	case http.MethodGet:
		handleGetSessions(w, r, campaignID)
	case http.MethodPost:
		handleCreateSession(w, r, campaignID)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func HandleSessionActions(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) < 3 || parts[1] != "sessions" {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	campaignID := parts[0]
	sessionID := parts[2]
	isResponsesEndpoint := len(parts) > 3 && parts[3] == "responses"

	if isResponsesEndpoint {
		switch r.Method {
		case http.MethodPost:
			handleSubmitSessionResponse(w, r, campaignID, sessionID)
		case http.MethodGet:
			handleGetSessionResponses(w, campaignID, sessionID)
		default:
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	switch r.Method {
	case http.MethodPut:
		handleUpdateSessionStatus(w, r, campaignID, sessionID)
	case http.MethodDelete:
		handleDeleteSession(w, r, campaignID, sessionID)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func handleGetSessions(w http.ResponseWriter, r *http.Request, campaignID string) {
	w.Header().Set("Content-Type", "application/json")

	monthStr := r.URL.Query().Get("month")
	if monthStr == "" {
		writeJSONError(w, http.StatusBadRequest, "month query parameter is required (format: YYYY-MM)")
		return
	}

	parts := strings.Split(monthStr, "-")
	if len(parts) != 2 {
		writeJSONError(w, http.StatusBadRequest, "invalid month format (expected YYYY-MM)")
		return
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid year")
		return
	}
	month, err := strconv.Atoi(parts[1])
	if err != nil || month < 1 || month > 12 {
		writeJSONError(w, http.StatusBadRequest, "invalid month")
		return
	}

	campaign, err := db.GetCampaignByID(campaignID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve campaign")
		return
	}
	if campaign == nil {
		writeJSONError(w, http.StatusNotFound, "campaign not found")
		return
	}

	sessions, err := db.GetSessionsByMonth(campaignID, year, month)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sessions")
		return
	}

	userID, _ := r.Context().Value("user_id").(string)
	for i := range sessions {
		responses, err := db.GetSessionResponses(sessions[i].ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to retrieve session responses")
			return
		}

		var accepted, declined, tentative int
		currentParticipation := ""
		respondedPlayers := map[string]bool{}
		for _, response := range responses {
			switch response.Participation {
			case models.Accepted:
				accepted++
			case models.Declined:
				declined++
			case models.Tentative:
				tentative++
			}
			if response.UserID == userID {
				currentParticipation = string(response.Participation)
			}
			respondedPlayers[response.UserID] = true
		}

		pending := 0
		for _, playerID := range campaign.Players {
			if !respondedPlayers[playerID] {
				pending++
			}
		}

		sessions[i].AcceptedCount = accepted
		sessions[i].DeclinedCount = declined
		sessions[i].TentativeCount = tentative
		sessions[i].PendingCount = pending
		sessions[i].CurrentUserParticipation = models.ParticipationStatus(currentParticipation)
	}

	w.WriteHeader(http.StatusOK)
	data, _ := json.Marshal(sessions)
	w.Write(data)
}

func handleCreateSession(w http.ResponseWriter, r *http.Request, campaignID string) {
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	campaign, err := db.GetCampaignByID(campaignID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve campaign")
		return
	}
	if campaign == nil {
		writeJSONError(w, http.StatusNotFound, "campaign not found")
		return
	}
	if campaign.DM != userID {
		writeJSONError(w, http.StatusForbidden, "only campaign DM can create sessions")
		return
	}

	type createSessionRequest struct {
		Date     string `json:"date"`
		Time     string `json:"time"`
		Duration int    `json:"duration"`
	}
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Date = strings.TrimSpace(req.Date)
	req.Time = strings.TrimSpace(req.Time)
	if req.Date == "" || req.Time == "" || req.Duration <= 0 {
		writeJSONError(w, http.StatusBadRequest, "date, time and positive duration are required")
		return
	}

	session := models.CampaignSession{
		ID:         db.GenerateUUID(),
		CampaignID: campaignID,
		Date:       req.Date,
		Time:       req.Time,
		Duration:   req.Duration,
		Status:     models.SessionSuggested,
	}
	if err := db.SaveSession(session); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	data, _ := json.Marshal(map[string]interface{}{
		"status":  "success",
		"message": "session created successfully",
		"session": session,
	})
	w.Write(data)
}

func handleUpdateSessionStatus(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) {
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	campaign, session, err := validateSessionCampaign(campaignID, sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve session")
		return
	}
	if campaign == nil {
		writeJSONError(w, http.StatusNotFound, "campaign not found")
		return
	}
	if session == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}
	if campaign.DM != userID {
		writeJSONError(w, http.StatusForbidden, "only campaign DM can update sessions")
		return
	}

	type updateSessionRequest struct {
		Status models.SessionStatus `json:"status"`
	}
	var req updateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	switch req.Status {
	case models.SessionConfirmed:
		session.Status = models.SessionConfirmed
		if err := db.SaveSession(*session); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to update session")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "success",
			"message": "session updated successfully",
			"session": session,
		})
	case models.SessionCancelled:
		if session.Status == models.SessionSuggested {
			if err := db.DeleteSession(session.ID); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to cancel session")
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":  "success",
				"message": "session cancelled and removed successfully",
			})
			return
		}
		session.Status = models.SessionCancelled
		if err := db.SaveSession(*session); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to cancel session")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "success",
			"message": "session cancelled successfully",
			"session": session,
		})
	default:
		writeJSONError(w, http.StatusBadRequest, "invalid status")
	}
}

func handleDeleteSession(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) {
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	campaign, session, err := validateSessionCampaign(campaignID, sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve session")
		return
	}
	if campaign == nil {
		writeJSONError(w, http.StatusNotFound, "campaign not found")
		return
	}
	if session == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}
	if campaign.DM != userID {
		writeJSONError(w, http.StatusForbidden, "only campaign DM can delete sessions")
		return
	}

	if session.Status == models.SessionSuggested {
		if err := db.DeleteSession(sessionID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to delete session")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "success",
			"message": "session deleted successfully",
		})
		return
	}

	session.Status = models.SessionCancelled
	if err := db.SaveSession(*session); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "session cancelled successfully",
		"session": session,
	})
}

func handleSubmitSessionResponse(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) {
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	campaign, session, err := validateSessionCampaign(campaignID, sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve session")
		return
	}
	if campaign == nil {
		writeJSONError(w, http.StatusNotFound, "campaign not found")
		return
	}
	if session == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}

	isPlayer := false
	for _, playerID := range campaign.Players {
		if playerID == userID {
			isPlayer = true
			break
		}
	}
	if !isPlayer {
		writeJSONError(w, http.StatusForbidden, "only registered campaign players can respond")
		return
	}

	type submitResponseRequest struct {
		Participation models.ParticipationStatus `json:"participation"`
	}
	var req submitResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Participation != models.Accepted && req.Participation != models.Declined && req.Participation != models.Tentative {
		writeJSONError(w, http.StatusBadRequest, "invalid participation status")
		return
	}

	response := models.SessionResponse{
		ID:            db.GenerateUUID(),
		SessionID:     sessionID,
		UserID:        userID,
		Participation: req.Participation,
	}
	if err := db.UpsertSessionResponse(response); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to submit response")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status":   "success",
		"message":  "response submitted successfully",
		"response": response,
	})
}

func handleGetSessionResponses(w http.ResponseWriter, campaignID, sessionID string) {
	campaign, session, err := validateSessionCampaign(campaignID, sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve session")
		return
	}
	if campaign == nil {
		writeJSONError(w, http.StatusNotFound, "campaign not found")
		return
	}
	if session == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}

	responses, err := db.GetSessionResponses(sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve responses")
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

	enriched := make([]sessionResponsePayload, 0, len(responses))
	respondedPlayers := map[string]bool{}
	for _, response := range responses {
		respObj := sessionResponsePayload{
			ID:            response.ID,
			SessionID:     response.SessionID,
			UserID:        response.UserID,
			Participation: response.Participation,
			CreatedAt:     response.CreatedAt,
			UpdatedAt:     response.UpdatedAt,
		}
		respondedPlayers[response.UserID] = true
		if user, ok := userMap[response.UserID]; ok && user != nil {
			respObj.User = map[string]interface{}{
				"id":           user.ID,
				"name":         user.Name,
				"nickname":     user.Nickname,
				"display_name": getDisplayName(*user),
				"picture":      user.Picture,
			}
		}
		enriched = append(enriched, respObj)
	}

	pending := make([]map[string]interface{}, 0)
	for _, playerID := range campaign.Players {
		if respondedPlayers[playerID] {
			continue
		}
		if user, ok := userMap[playerID]; ok && user != nil {
			pending = append(pending, map[string]interface{}{
				"id":           user.ID,
				"name":         user.Name,
				"nickname":     user.Nickname,
				"display_name": getDisplayName(*user),
				"picture":      user.Picture,
			})
			continue
		}
		pending = append(pending, map[string]interface{}{
			"id":           playerID,
			"display_name": playerID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"responses": responsesByParticipation(enriched),
		"pending":   pending,
		"all":       enriched,
	})
}

func validateSessionCampaign(campaignID, sessionID string) (*models.Campaign, *models.CampaignSession, error) {
	campaign, err := db.GetCampaignByID(campaignID)
	if err != nil {
		return nil, nil, err
	}
	if campaign == nil {
		return nil, nil, nil
	}

	session, err := db.GetSessionByID(sessionID)
	if err != nil {
		return campaign, nil, err
	}
	if session == nil || session.CampaignID != campaignID {
		return campaign, nil, nil
	}
	return campaign, session, nil
}

func responsesByParticipation(responses []sessionResponsePayload) map[string][]sessionResponsePayload {
	grouped := map[string][]sessionResponsePayload{
		"Accepted":  {},
		"Declined":  {},
		"Tentative": {},
	}
	for _, response := range responses {
		key := string(response.Participation)
		if _, ok := grouped[key]; !ok {
			grouped[key] = []sessionResponsePayload{}
		}
		grouped[key] = append(grouped[key], response)
	}
	return grouped
}

func getDisplayName(user models.User) string {
	if user.Nickname != "" {
		return user.Nickname
	}
	if user.Name != "" {
		return user.Name
	}
	return user.Email
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	data, _ := json.Marshal(payload)
	_, _ = w.Write(data)
}
