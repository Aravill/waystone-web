package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
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

type campaignSessionSummary struct {
	next        *models.CampaignSession // closest confirmed session that hasn't ended
	nextStart   time.Time
	playedCount int                      // confirmed sessions that have ended
	played      []models.CampaignSession // those sessions, in date/time order
}

// campaignSessionSummaries summarises each campaign's confirmed sessions.
func campaignSessionSummaries(campaigns []models.Campaign) (map[string]campaignSessionSummary, error) {
	ids := make([]string, 0, len(campaigns))
	for _, c := range campaigns {
		ids = append(ids, c.ID)
	}
	sessions, err := db.GetSessionsByCampaignIDs(ids, "")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result := map[string]campaignSessionSummary{}
	for i := range sessions {
		session := sessions[i]
		if session.Status != models.SessionConfirmed {
			continue
		}
		start, ok := sessionStart(session)
		if !ok {
			continue
		}
		summary := result[session.CampaignID]
		if start.Add(time.Duration(session.Duration) * time.Minute).After(now) {
			if summary.next == nil || start.Before(summary.nextStart) {
				summary.next, summary.nextStart = &session, start
			}
		} else {
			summary.playedCount++
			summary.played = append(summary.played, session)
		}
		result[session.CampaignID] = summary
	}
	return result, nil
}

// campaignViewer holds everything needed to enrich campaigns for one viewer.
type campaignViewer struct {
	userID         string
	isAdmin        bool
	users          map[string]*models.User
	signups        map[string]models.CampaignSignup
	followed       map[string]bool
	pendingSignups map[string]int
	unanswered     map[string]models.UnansweredSessions
	sessions       map[string]campaignSessionSummary
	systems        map[string]models.System
}

// loadCampaignViewer fetches the per-viewer data for campaigns. It returns an
// error message (for a 500 response) or "".
func loadCampaignViewer(r *http.Request, campaigns []models.Campaign) (*campaignViewer, string) {
	// Fetch all users once to avoid N+1 queries
	allUsers, err := db.GetAllUsers()
	if err != nil {
		return nil, "failed to retrieve users"
	}

	v := &campaignViewer{users: make(map[string]*models.User)}
	v.userID, _ = r.Context().Value("user_id").(string)
	if v.signups, err = db.GetCampaignSignupsByUser(v.userID); err != nil {
		return nil, "failed to retrieve sign-ups"
	}
	if v.followed, err = db.GetFollowedCampaignIDs(v.userID); err != nil {
		return nil, "failed to retrieve follows"
	}
	if v.pendingSignups, err = db.CountPendingSignupsForDM(v.userID); err != nil {
		return nil, "failed to retrieve game counts"
	}
	if v.unanswered, err = db.GetUnansweredSessions(v.userID, time.Now().UTC().Format("2006-01-02")); err != nil {
		return nil, "failed to retrieve game counts"
	}
	if v.sessions, err = campaignSessionSummaries(campaigns); err != nil {
		return nil, "failed to retrieve sessions"
	}
	if v.systems, err = systemsByName(); err != nil {
		return nil, "failed to retrieve systems"
	}

	// Create a map for O(1) user lookup
	for i := range allUsers {
		v.users[allUsers[i].ID] = &allUsers[i]
	}
	if currentUser, ok := v.users[v.userID]; ok && currentUser != nil {
		v.isAdmin = currentUser.IsAdmin()
	}
	return v, ""
}

// enrich builds the API object for one campaign as seen by the viewer.
func (v *campaignViewer) enrich(campaign models.Campaign) map[string]interface{} {
	currentUserID := v.userID
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
		"system_logo_url":            v.systems[strings.ToLower(campaign.System)].LogoURL(),
		"can_manage":                 v.isAdmin || (currentUserID != "" && campaign.DM == currentUserID),
		"allowed_status_transitions": campaign.Status.AllowedTransitions(),
	}
	campaignObj["current_user_signup_status"] = string(v.signups[campaign.ID].Status)
	campaignObj["is_following"] = v.followed[campaign.ID]

	isPlayer := false
	for _, p := range campaign.Players {
		if p == currentUserID {
			isPlayer = true
			break
		}
	}
	unansweredCount, firstUnanswered, pendingCount := 0, "", 0
	if isPlayer {
		u := v.unanswered[campaign.ID]
		unansweredCount, firstUnanswered = u.Count, u.FirstDate
	}
	if currentUserID != "" && campaign.DM == currentUserID {
		pendingCount = v.pendingSignups[campaign.ID]
	}
	campaignObj["unanswered_sessions_count"] = unansweredCount
	campaignObj["first_unanswered_session_date"] = firstUnanswered
	campaignObj["pending_signups_count"] = pendingCount
	campaignObj["next_session"] = v.sessions[campaign.ID].next
	campaignObj["played_sessions_count"] = v.sessions[campaign.ID].playedCount

	// Add DM user display object if DM exists
	if campaign.DM != "" {
		if dmUser, ok := v.users[campaign.DM]; ok && dmUser != nil {
			campaignObj["dm_user"] = userDisplayObject(dmUser)
		}
	}

	// Add player user display objects
	playerUsers := []map[string]interface{}{}
	for _, playerID := range campaign.Players {
		if playerUser, ok := v.users[playerID]; ok && playerUser != nil {
			playerUsers = append(playerUsers, userDisplayObject(playerUser))
		}
	}
	campaignObj["player_users"] = playerUsers
	return campaignObj
}

func HandleGetCampaigns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	campaigns, err := db.GetAllCampaigns()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to retrieve games"}`)
		return
	}
	v, msg := loadCampaignViewer(r, campaigns)
	if msg != "" {
		writeJSONError(w, http.StatusInternalServerError, msg)
		return
	}

	// Build enriched campaign objects
	enriched := []map[string]interface{}{}
	for _, campaign := range campaigns {
		enriched = append(enriched, v.enrich(campaign))
	}

	w.WriteHeader(http.StatusOK)
	data, _ := json.Marshal(enriched)
	w.Write(data)
}

// handleGetCampaign returns one campaign with its played sessions (newest
// first) and who attended each. Visible to every logged-in user.
func handleGetCampaign(w http.ResponseWriter, r *http.Request, campaignID string) {
	campaign, _, ok := loadCampaignAndUser(w, r, campaignID)
	if !ok {
		return
	}
	v, msg := loadCampaignViewer(r, []models.Campaign{*campaign})
	if msg != "" {
		writeJSONError(w, http.StatusInternalServerError, msg)
		return
	}
	obj := v.enrich(*campaign)

	played := v.sessions[campaign.ID].played
	ids := make([]string, 0, len(played))
	for _, s := range played {
		ids = append(ids, s.ID)
	}
	responses, err := db.GetSessionResponsesBySessionIDs(ids)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sessions")
		return
	}

	list := make([]map[string]interface{}, 0, len(played))
	for i := len(played) - 1; i >= 0; i-- {
		s := played[i]
		// The DM runs every played session, so they always attend.
		attendees := []map[string]interface{}{}
		if dm, ok := v.users[campaign.DM]; ok && dm != nil {
			attendees = append(attendees, userDisplayObject(dm))
		}
		for _, resp := range responses[s.ID] {
			if resp.UserID == campaign.DM || resp.Participation != models.Accepted {
				continue
			}
			if u, ok := v.users[resp.UserID]; ok && u != nil {
				attendees = append(attendees, userDisplayObject(u))
			}
		}
		list = append(list, map[string]interface{}{
			"id":        s.ID,
			"number":    i + 1,
			"date":      s.Date,
			"time":      s.Time,
			"duration":  s.Duration,
			"timezone":  s.Timezone,
			"attendees": attendees,
		})
	}
	obj["played_sessions"] = list
	writeJSON(w, http.StatusOK, obj)
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
		fmt.Fprintf(w, `{"error": "failed to create game"}`)
		return
	}

	w.WriteHeader(http.StatusCreated)
	data, _ := json.Marshal(map[string]interface{}{
		"status":      "success",
		"message":     "game created successfully",
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
		return "invalid game type"
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
		writeJSONError(w, http.StatusForbidden, "only the game DM or an admin can manage this game")
		return nil, false
	}
	return campaign, true
}

// HandleCampaignActions handles /api/campaigns/{id} (GET one, PUT update, DELETE remove).
func HandleCampaignActions(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) != 1 || parts[0] == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	switch r.Method {
	case http.MethodGet:
		handleGetCampaign(w, r, parts[0])
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
		writeJSONError(w, http.StatusInternalServerError, "failed to update game")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "game updated"})
}

// handleDeleteCampaign is admin-only: DMs can edit their games but not delete
// them, so a hijacked or malicious DM account can't wipe a game's history.
func handleDeleteCampaign(w http.ResponseWriter, r *http.Request, campaignID string) {
	_, userID, ok := loadCampaignAndUser(w, r, campaignID)
	if !ok {
		return
	}
	user, err := db.GetUserByID(userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve user")
		return
	}
	if user == nil || !user.IsAdmin() {
		writeJSONError(w, http.StatusForbidden, "only an admin can delete a game")
		return
	}
	if err := db.DeleteCampaign(campaignID); err != nil {
		if errors.Is(err, db.ErrCampaignNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to delete game")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "game deleted"})
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

	userID, _ := r.Context().Value("user_id").(string)
	oldStatus := campaign.Status
	if err := db.SetCampaignStatus(parts[0], req.Status); err != nil {
		if errors.Is(err, db.ErrCampaignNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to update status")
		return
	}
	if oldStatus != req.Status {
		notify(userID, append(campaignFollowerIDs(campaign), campaign.DM), models.NotificationCampaignStatus, campaign, "", map[string]string{
			"from_status": string(oldStatus),
			"to_status":   string(req.Status),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":                     "success",
		"message":                    "status updated",
		"campaign_status":            req.Status,
		"allowed_status_transitions": req.Status.AllowedTransitions(),
	})
}
