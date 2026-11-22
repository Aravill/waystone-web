package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"waystone-web/db"
	"waystone-web/models"
)

// pollScheduleMu serialises every read-modify-write of poll schedules (handlers and the ticker),
// so a cycle is handled exactly once.
var pollScheduleMu sync.Mutex

// maxCyclesPerRun bounds how many due cycles one posting check handles.
const maxCyclesPerRun = 60

type pollScheduleRequest struct {
	PollID     string `json:"poll_id"`
	EveryWeeks int    `json:"every_weeks"`
	LeadDays   int    `json:"lead_days"`
	Paused     bool   `json:"paused"`
}

type pollScheduleView struct {
	models.PollSchedule
	NextPostAt string   `json:"next_post_at"`
	NextDates  []string `json:"next_dates"`
}

// shiftDate moves a YYYY-MM-DD date by whole calendar days.
func shiftDate(date string, days int) string {
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return day.AddDate(0, 0, days).Format("2006-01-02")
}

// cycleSlots returns the slots of cycle k as ordinary session dates (no ID, no poll).
func cycleSlots(sc models.PollSchedule, k int) []models.CampaignSession {
	out := make([]models.CampaignSession, 0, len(sc.Slots))
	for _, slot := range sc.Slots {
		out = append(out, models.CampaignSession{
			CampaignID: sc.CampaignID,
			Date:       shiftDate(slot.Date, k*sc.EveryWeeks*7),
			Time:       slot.Time,
			Duration:   slot.Duration,
			Timezone:   sc.Timezone,
		})
	}
	return out
}

// cycleFirstStart is the start of cycle k's earliest slot.
func cycleFirstStart(sc models.PollSchedule, k int) (time.Time, bool) {
	var first time.Time
	found := false
	for _, s := range cycleSlots(sc, k) {
		if start, ok := sessionStart(s); ok && (!found || start.Before(first)) {
			first, found = start, true
		}
	}
	return first, found
}

// cyclePostTime is when cycle k's poll is posted: its first start minus the lead time.
func cyclePostTime(sc models.PollSchedule, k int) (time.Time, bool) {
	first, ok := cycleFirstStart(sc, k)
	if !ok {
		return time.Time{}, false
	}
	return first.AddDate(0, 0, -sc.LeadDays), true
}

// firstCycleAhead is the first cycle (from 1) whose first date has not started yet.
func firstCycleAhead(sc models.PollSchedule, now time.Time) int {
	for k := 1; k < 5000; k++ {
		if first, ok := cycleFirstStart(sc, k); ok && first.After(now) {
			return k
		}
	}
	return 1
}

// postDuePolls handles every schedule's due cycles. It runs from the minute ticker.
func postDuePolls(now time.Time) {
	pollScheduleMu.Lock()
	defer pollScheduleMu.Unlock()

	schedules, err := db.GetPollSchedules()
	if err != nil {
		log.Printf("poll schedules: %v", err)
		return
	}
	for _, sc := range schedules {
		campaign, err := db.GetCampaignByID(sc.CampaignID)
		if err != nil {
			log.Printf("poll schedules: campaign %s: %v", sc.CampaignID, err)
			continue
		}
		if campaign != nil {
			postDueCycles(sc, campaign, now)
		}
	}
}

// postDueCycles handles each cycle whose post time has passed, advancing next_cycle first
// so that a cycle is handled at most once. The caller holds pollScheduleMu.
func postDueCycles(sc models.PollSchedule, campaign *models.Campaign, now time.Time) {
	for i := 0; i < maxCyclesPerRun; i++ {
		post, ok := cyclePostTime(sc, sc.NextCycle)
		if !ok || post.After(now) {
			return
		}
		k := sc.NextCycle
		sc.NextCycle++
		if err := db.SavePollSchedule(sc); err != nil {
			log.Printf("poll schedule %s: %v", sc.CampaignID, err)
			return
		}
		postCycle(sc, campaign, k, now)
	}
}

// postCycle posts cycle k as a poll, or tells the DM why it could not. Cycles of a paused
// schedule or a game that is not Ongoing are skipped silently, and so is a cycle whose
// dates have all begun already.
func postCycle(sc models.PollSchedule, campaign *models.Campaign, k int, now time.Time) {
	if sc.Paused || campaign.Status != models.Ongoing {
		return
	}
	existing, err := db.GetSessionsByCampaignIDs([]string{campaign.ID}, "")
	if err != nil {
		log.Printf("poll schedule %s: %v", campaign.ID, err)
		return
	}
	planned := map[string]bool{}
	for _, s := range existing {
		if !s.Manual && s.Status != models.SessionCancelled {
			planned[s.Date] = true
		}
	}
	var ahead, blocked []models.CampaignSession
	var dates []pollDateRequest
	for _, slot := range cycleSlots(sc, k) {
		start, ok := sessionStart(slot)
		if !ok || !start.After(now) {
			continue
		}
		ahead = append(ahead, slot)
		if planned[slot.Date] {
			blocked = append(blocked, slot)
			continue
		}
		dates = append(dates, pollDateRequest{Date: slot.Date, Time: slot.Time, Duration: slot.Duration})
	}
	skip := func(list []models.CampaignSession, reason string) {
		if len(list) > 0 {
			notifyPoll("", []string{campaign.DM}, models.NotificationPollSkipped, campaign, list[0], list, map[string]string{"reason": reason})
		}
	}
	if len(dates) == 0 {
		skip(blocked, "taken")
		return
	}
	pollID := db.GenerateUUID()
	sessions, code, msg := buildPollSessions(campaign.ID, pollID, pollRequest{Timezone: sc.Timezone, Dates: dates}, now)
	if code != 0 {
		log.Printf("poll schedule %s: %s", campaign.ID, msg)
		return
	}
	if len(existing)+len(sessions) > maxSessionsPerCampaign {
		skip(ahead, "cap")
		return
	}
	if err := db.CreatePollSessions(sessions); err != nil {
		var taken *db.DayTakenError
		if errors.As(err, &taken) {
			skip(ahead, "taken")
		} else {
			log.Printf("poll schedule %s: %v", campaign.ID, err)
		}
		return
	}
	fresh := upcomingDates(sessions, now)
	notifyPoll(campaign.DM, campaign.Players, models.NotificationSessionSuggested, campaign, fresh[0], fresh, nil)
	notifyPoll("", []string{campaign.DM}, models.NotificationPollPosted, campaign, fresh[0], fresh, nil)
}

func newPollScheduleView(sc models.PollSchedule) pollScheduleView {
	view := pollScheduleView{PollSchedule: sc, NextDates: []string{}}
	if post, ok := cyclePostTime(sc, sc.NextCycle); ok {
		view.NextPostAt = post.UTC().Format(time.RFC3339)
	}
	for _, s := range cycleSlots(sc, sc.NextCycle) {
		view.NextDates = append(view.NextDates, s.Date)
	}
	return view
}

// HandlePollSchedule serves /api/campaigns/{id}/poll-schedule (GET, PUT, DELETE). DM only.
func HandlePollSchedule(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) != 2 || parts[1] != "poll-schedule" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPut && r.Method != http.MethodDelete {
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
		writeJSONError(w, http.StatusForbidden, "only game DM can change the repeating poll")
		return
	}

	pollScheduleMu.Lock()
	defer pollScheduleMu.Unlock()
	switch r.Method {
	case http.MethodGet:
		sc, err := db.GetPollSchedule(campaign.ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to retrieve schedule")
			return
		}
		writeScheduleResponse(w, sc)
	case http.MethodPut:
		handleSavePollSchedule(w, r, campaign)
	case http.MethodDelete:
		if err := db.DeletePollSchedule(campaign.ID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to stop the repeating poll")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "success", "message": "repeating poll stopped"})
	}
}

func writeScheduleResponse(w http.ResponseWriter, sc *models.PollSchedule) {
	if sc == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"schedule": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"schedule": newPollScheduleView(*sc)})
}

func handleSavePollSchedule(w http.ResponseWriter, r *http.Request, campaign *models.Campaign) {
	var req pollScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.EveryWeeks < 1 || req.EveryWeeks > 4 {
		writeJSONError(w, http.StatusBadRequest, "every_weeks must be between 1 and 4")
		return
	}
	if req.LeadDays < 7 || req.LeadDays > 28 {
		writeJSONError(w, http.StatusBadRequest, "lead_days must be between 7 and 28")
		return
	}
	now := time.Now()
	var sc models.PollSchedule
	if req.PollID != "" {
		sessions, err := db.GetPollSessions(campaign.ID, req.PollID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to retrieve poll")
			return
		}
		if len(sessions) == 0 {
			writeJSONError(w, http.StatusNotFound, "poll not found")
			return
		}
		upcoming := upcomingDates(sessions, now)
		if len(upcoming) == 0 {
			writeJSONError(w, http.StatusConflict, "this poll has no upcoming date")
			return
		}
		sc = models.PollSchedule{
			CampaignID: campaign.ID,
			EveryWeeks: req.EveryWeeks,
			LeadDays:   req.LeadDays,
			Timezone:   upcoming[0].Timezone,
			NextCycle:  1,
		}
		for _, d := range upcoming {
			sc.Slots = append(sc.Slots, models.PollSlot{Date: d.Date, Time: d.Time, Duration: d.Duration})
		}
	} else {
		existing, err := db.GetPollSchedule(campaign.ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to retrieve schedule")
			return
		}
		if existing == nil {
			writeJSONError(w, http.StatusNotFound, "no repeating poll")
			return
		}
		sc = *existing
		changed := sc.EveryWeeks != req.EveryWeeks
		sc.EveryWeeks, sc.LeadDays, sc.Paused = req.EveryWeeks, req.LeadDays, req.Paused
		if changed {
			sc.NextCycle = firstCycleAhead(sc, now)
		}
	}
	if err := db.SavePollSchedule(sc); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save the repeating poll")
		return
	}
	postDueCycles(sc, campaign, now)
	saved, err := db.GetPollSchedule(campaign.ID)
	if err != nil || saved == nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve schedule")
		return
	}
	writeScheduleResponse(w, saved)
}
