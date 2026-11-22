package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"waystone-web/db"
	"waystone-web/models"
)

const maxPollDates = 31

type pollDateRequest struct {
	Date     string `json:"date"`
	Time     string `json:"time"`
	Duration int    `json:"duration"`
}

type pollRequest struct {
	Timezone string            `json:"timezone"`
	Dates    []pollDateRequest `json:"dates"`
}

// HandlePolls serves /api/campaigns/{id}/polls (POST), /polls/{pollId}/dates (POST) and
// /polls/{pollId} (DELETE). Only the game's DM may use them.
func HandlePolls(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) < 2 || parts[1] != "polls" || len(parts) > 4 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var handler func(campaign *models.Campaign)
	switch {
	case len(parts) == 2 && r.Method == http.MethodPost:
		handler = func(c *models.Campaign) { handleCreatePoll(w, r, c, userID) }
	case len(parts) == 4 && parts[3] == "dates" && parts[2] != "" && r.Method == http.MethodPost:
		handler = func(c *models.Campaign) { handleAddPollDates(w, r, c, userID, parts[2]) }
	case len(parts) == 3 && parts[2] != "" && r.Method == http.MethodDelete:
		handler = func(c *models.Campaign) { handleWithdrawPoll(w, c, userID, parts[2]) }
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	campaign, err := db.GetCampaignByID(parts[0])
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve game")
		return
	}
	if campaign == nil {
		writeJSONError(w, http.StatusNotFound, "game not found")
		return
	}
	if campaign.DM != userID {
		writeJSONError(w, http.StatusForbidden, "only game DM can change polls")
		return
	}
	handler(campaign)
}

// buildPollSessions validates the requested dates and turns them into Suggested sessions of
// one poll. On failure it returns the status code and message to answer with.
func buildPollSessions(campaignID, pollID string, req pollRequest, now time.Time) ([]models.CampaignSession, int, string) {
	if len(req.Dates) == 0 {
		return nil, http.StatusBadRequest, "a poll needs at least one date"
	}
	if len(req.Dates) > maxPollDates {
		return nil, http.StatusBadRequest, fmt.Sprintf("a poll can have at most %d dates", maxPollDates)
	}
	timezone := strings.TrimSpace(req.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, http.StatusBadRequest, "invalid timezone"
	}

	sessions := make([]models.CampaignSession, 0, len(req.Dates))
	seen := map[string]bool{}
	for _, d := range req.Dates {
		date, clock := strings.TrimSpace(d.Date), strings.TrimSpace(d.Time)
		day, err := time.Parse("2006-01-02", date)
		if err != nil {
			return nil, http.StatusBadRequest, "invalid date (expected YYYY-MM-DD)"
		}
		if _, err := time.Parse("15:04", clock); err != nil {
			return nil, http.StatusBadRequest, "invalid time (expected HH:MM)"
		}
		if d.Duration < 1 || d.Duration > 1440 {
			return nil, http.StatusBadRequest, "duration must be between 1 and 1440 minutes"
		}
		label := day.Format("Mon, Jan 2")
		if seen[date] {
			return nil, http.StatusBadRequest, label + " is picked twice"
		}
		seen[date] = true
		session := models.CampaignSession{
			ID:         db.GenerateUUID(),
			CampaignID: campaignID,
			Date:       date,
			Time:       clock,
			Duration:   d.Duration,
			Timezone:   timezone,
			Status:     models.SessionSuggested,
			PollID:     pollID,
		}
		if start, ok := sessionStart(session); !ok || !start.After(now) {
			return nil, http.StatusConflict, label + " has already begun"
		}
		sessions = append(sessions, session)
	}
	return sessions, 0, ""
}

// insertPollSessions checks the game's session cap and stores the dates in one transaction.
// It writes the error response itself and reports whether the dates were stored.
func insertPollSessions(w http.ResponseWriter, campaignID string, sessions []models.CampaignSession) bool {
	existing, err := db.GetSessionsByCampaignIDs([]string{campaignID}, "")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sessions")
		return false
	}
	if len(existing)+len(sessions) > maxSessionsPerCampaign {
		writeJSONError(w, http.StatusConflict, "a game can have at most 999 sessions")
		return false
	}
	if err := db.CreatePollSessions(sessions); err != nil {
		var taken *db.DayTakenError
		if errors.As(err, &taken) {
			label := taken.Date
			if day, perr := time.Parse("2006-01-02", taken.Date); perr == nil {
				label = day.Format("Mon, Jan 2")
			}
			writeJSONError(w, http.StatusConflict, label+" already has a session")
			return false
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to create poll")
		return false
	}
	return true
}

func handleCreatePoll(w http.ResponseWriter, r *http.Request, campaign *models.Campaign, userID string) {
	var req pollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	now := time.Now()
	pollID := db.GenerateUUID()
	sessions, code, msg := buildPollSessions(campaign.ID, pollID, req, now)
	if code != 0 {
		writeJSONError(w, code, msg)
		return
	}
	if !insertPollSessions(w, campaign.ID, sessions) {
		return
	}
	notifySessionChange(userID, campaign, upcomingDates(sessions, now)[0], models.NotificationSessionSuggested, "")
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status":   "success",
		"message":  "poll created",
		"poll_id":  pollID,
		"sessions": sessions,
	})
}

func handleAddPollDates(w http.ResponseWriter, r *http.Request, campaign *models.Campaign, userID, pollID string) {
	var req pollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	now := time.Now()
	existing, err := db.GetPollSessions(campaign.ID, pollID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve poll")
		return
	}
	if len(existing) == 0 {
		writeJSONError(w, http.StatusNotFound, "poll not found")
		return
	}
	if len(upcomingDates(existing, now)) == 0 {
		writeJSONError(w, http.StatusConflict, "this poll has ended")
		return
	}
	if strings.TrimSpace(req.Timezone) == "" {
		req.Timezone = existing[0].Timezone
	}
	sessions, code, msg := buildPollSessions(campaign.ID, pollID, req, now)
	if code != 0 {
		writeJSONError(w, code, msg)
		return
	}
	if !insertPollSessions(w, campaign.ID, sessions) {
		return
	}
	notifySessionChange(userID, campaign, upcomingDates(sessions, now)[0], models.NotificationSessionSuggested, "")
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status":   "success",
		"message":  "dates added",
		"poll_id":  pollID,
		"sessions": sessions,
	})
}

func handleWithdrawPoll(w http.ResponseWriter, campaign *models.Campaign, userID, pollID string) {
	existing, err := db.GetPollSessions(campaign.ID, pollID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve poll")
		return
	}
	if len(existing) == 0 {
		writeJSONError(w, http.StatusNotFound, "poll not found")
		return
	}
	upcoming := upcomingDates(existing, time.Now())
	removed, err := db.DeletePoll(campaign.ID, pollID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to withdraw poll")
		return
	}
	if len(upcoming) > 0 {
		notifySessionChange(userID, campaign, upcoming[0], models.NotificationSessionCancelled, string(models.SessionSuggested))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "poll withdrawn",
		"removed": removed,
	})
}

// upcomingDates returns the Suggested dates whose start has not passed, earliest first.
// Manual sessions may have no time, so they are skipped before sessionStart is asked.
func upcomingDates(sessions []models.CampaignSession, now time.Time) []models.CampaignSession {
	starts := map[string]time.Time{}
	out := []models.CampaignSession{}
	for _, s := range sessions {
		if s.Manual || s.Status != models.SessionSuggested {
			continue
		}
		if start, ok := sessionStart(s); ok && start.After(now) {
			starts[s.ID] = start
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return starts[out[a].ID].Before(starts[out[b].ID]) })
	return out
}

// withdrawSuggestedDate deletes one Suggested date with its replies. It is silent unless the
// date was the poll's last upcoming one, which counts as withdrawing the poll.
func withdrawSuggestedDate(actorID string, campaign *models.Campaign, session models.CampaignSession) error {
	now := time.Now()
	wasLast := false
	if session.PollID != "" {
		siblings, err := db.GetPollSessions(campaign.ID, session.PollID)
		if err != nil {
			return err
		}
		upcoming := upcomingDates(siblings, now)
		wasLast = len(upcoming) == 1 && upcoming[0].ID == session.ID
	} else if start, ok := sessionStart(session); ok && start.After(now) {
		wasLast = true
	}
	if err := db.DeleteSession(session.ID); err != nil {
		return err
	}
	if wasLast {
		notifySessionChange(actorID, campaign, session, models.NotificationSessionCancelled, string(models.SessionSuggested))
	}
	return nil
}
