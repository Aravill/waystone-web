package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

const (
	maxSessionsPerCampaign = 999
	maxGuestsPerSession    = 20
	maxGuestNameRunes      = 64
)

// playedSessionRequest is the body for recording and editing a by-hand session.
type playedSessionRequest struct {
	Date        string   `json:"date"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Timezone    string   `json:"timezone"`
	AttendeeIDs []string `json:"attendee_ids"`
	Guests      []string `json:"guests"`
}

// HandlePlayedSessions handles /api/campaigns/{id}/played-sessions[/{sid}]: the DM
// records, edits and deletes past sessions that were never on the calendar.
func HandlePlayedSessions(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) < 2 || len(parts) > 3 || parts[1] != "played-sessions" || (len(parts) == 3 && parts[2] == "") {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}
	switch {
	case len(parts) == 2 && r.Method == http.MethodPost:
		handleRecordPlayedSession(w, r, parts[0], "")
	case len(parts) == 3 && r.Method == http.MethodPut:
		handleRecordPlayedSession(w, r, parts[0], parts[2])
	case len(parts) == 3 && r.Method == http.MethodDelete:
		handleDeletePlayedSession(w, r, parts[0], parts[2])
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// loadManualSession authenticates the DM and loads the by-hand session being changed.
func loadManualSession(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) (*models.Campaign, *models.CampaignSession, bool) {
	campaign, userID, ok := loadCampaignAndUser(w, r, campaignID)
	if !ok {
		return nil, nil, false
	}
	if campaign.DM != userID {
		writeJSONError(w, http.StatusForbidden, "only the game DM can record past sessions")
		return nil, nil, false
	}
	if sessionID == "" {
		return campaign, nil, true
	}
	_, session, err := validateSessionCampaign(campaignID, sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve session")
		return nil, nil, false
	}
	if session == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return nil, nil, false
	}
	if !session.Manual {
		writeJSONError(w, http.StatusConflict, "only sessions recorded by hand can be changed here")
		return nil, nil, false
	}
	return campaign, session, true
}

func handleRecordPlayedSession(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) {
	campaign, existing, ok := loadManualSession(w, r, campaignID, sessionID)
	if !ok {
		return
	}
	var req playedSessionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// On edit, someone who has since left the game may stay on the session.
	allowed := map[string]bool{}
	if existing != nil {
		responses, err := db.GetSessionResponses(existing.ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to retrieve session")
			return
		}
		for _, resp := range responses {
			if resp.Participation == models.Accepted {
				allowed[resp.UserID] = true
			}
		}
	}
	session, attendeeIDs, msg := req.normalize(campaign, allowed)
	if msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}

	status, message := http.StatusOK, "session updated successfully"
	if existing != nil {
		session.ID, session.CreatedAt = existing.ID, existing.CreatedAt
	} else {
		sessions, err := db.GetSessionsByCampaignIDs([]string{campaignID}, "")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sessions")
			return
		}
		if len(sessions) >= maxSessionsPerCampaign {
			writeJSONError(w, http.StatusConflict, "a game can have at most 999 sessions")
			return
		}
		session.ID = db.GenerateUUID()
		status, message = http.StatusCreated, "session recorded successfully"
	}
	session.CampaignID, session.Status, session.Manual = campaignID, models.SessionConfirmed, true

	if err := db.SaveManualSession(session, attendeeIDs); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save session")
		return
	}
	writeJSON(w, status, map[string]interface{}{"status": "success", "message": message, "session": session})
}

func handleDeletePlayedSession(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) {
	if _, _, ok := loadManualSession(w, r, campaignID, sessionID); !ok {
		return
	}
	if err := db.DeleteManualSession(sessionID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "session deleted successfully"})
}

// normalize validates the request and returns the session fields, the attendee user ids
// (without the DM, who never has a response row) and a validation message, or "".
// allowedExtra lists users who may attend although they aren't players now.
func (req *playedSessionRequest) normalize(campaign *models.Campaign, allowedExtra map[string]bool) (models.CampaignSession, []string, string) {
	var session models.CampaignSession
	req.Date = strings.TrimSpace(req.Date)
	req.Start = strings.TrimSpace(req.Start)
	req.End = strings.TrimSpace(req.End)
	req.Timezone = strings.TrimSpace(req.Timezone)
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	loc, err := time.LoadLocation(req.Timezone)
	if err != nil {
		return session, nil, "invalid timezone"
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		return session, nil, "choose a date"
	}
	if req.Date > time.Now().In(loc).Format("2006-01-02") {
		return session, nil, "the date must not be in the future"
	}

	duration := 0
	if req.End != "" && req.Start == "" {
		return session, nil, "an end time needs a start time"
	}
	var start, end time.Time
	if req.Start != "" {
		if start, err = time.Parse("15:04", req.Start); err != nil {
			return session, nil, "invalid start time"
		}
		req.Start = start.Format("15:04")
		begins, err := time.ParseInLocation("2006-01-02 15:04", req.Date+" "+req.Start, loc)
		if err != nil || begins.After(time.Now()) {
			return session, nil, "the session must not be in the future"
		}
	}
	if req.End != "" {
		if end, err = time.Parse("15:04", req.End); err != nil {
			return session, nil, "invalid end time"
		}
		if end.Equal(start) {
			return session, nil, "the end time must differ from the start time"
		}
		if end.Before(start) { // past midnight
			end = end.Add(24 * time.Hour)
		}
		duration = int(end.Sub(start).Minutes())
	}

	member := map[string]bool{campaign.DM: true}
	for _, p := range campaign.Players {
		member[p] = true
	}
	var ids []string
	seen := map[string]bool{}
	dmThere := false
	for _, id := range req.AttendeeIDs {
		if !member[id] && !allowedExtra[id] {
			return session, nil, "attendees must be the game's DM and players"
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if id == campaign.DM {
			dmThere = true
			continue
		}
		ids = append(ids, id)
	}

	var guests []string
	seenGuest := map[string]bool{}
	for _, raw := range req.Guests {
		name := strings.Join(strings.Fields(raw), " ")
		if name == "" || seenGuest[strings.ToLower(name)] {
			continue
		}
		if utf8.RuneCountInString(name) > maxGuestNameRunes {
			return session, nil, "a guest name must be at most 64 characters"
		}
		seenGuest[strings.ToLower(name)] = true
		guests = append(guests, name)
	}
	if len(guests) > maxGuestsPerSession {
		return session, nil, "at most 20 guests"
	}
	if !dmThere && len(ids) == 0 && len(guests) == 0 {
		return session, nil, "choose at least one person who was there"
	}

	session.Date, session.Time, session.Duration, session.Timezone = req.Date, req.Start, duration, req.Timezone
	session.Guests, session.DMAbsent = guests, !dmThere
	return session, ids, ""
}
