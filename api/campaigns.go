package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
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
		summary := result[session.CampaignID]
		if session.Manual { // recorded by hand: always played, never next, and may have no time
			summary.playedCount++
			summary.played = append(summary.played, session)
			result[session.CampaignID] = summary
			continue
		}
		start, ok := sessionStart(session)
		if !ok {
			continue
		}
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
	// Number sessions chronologically across timezones: by start instant, then id.
	for id, summary := range result {
		starts := make(map[string]time.Time, len(summary.played))
		for _, p := range summary.played {
			starts[p.ID] = sessionSortInstant(p)
		}
		sort.SliceStable(summary.played, func(a, b int) bool {
			pa, pb := summary.played[a], summary.played[b]
			if !starts[pa.ID].Equal(starts[pb.ID]) {
				return starts[pa.ID].Before(starts[pb.ID])
			}
			return pa.ID < pb.ID
		})
		result[id] = summary
	}
	return result, nil
}

// sessionSortInstant is the key for numbering played sessions: the start instant, or
// noon (in the session's timezone) of the date for by-hand sessions without a time.
func sessionSortInstant(s models.CampaignSession) time.Time {
	if start, ok := sessionStart(s); ok {
		return start
	}
	loc := time.UTC
	if l, err := time.LoadLocation(s.Timezone); err == nil && s.Timezone != "" {
		loc = l
	}
	d, _ := time.ParseInLocation("2006-01-02 15:04", s.Date+" 12:00", loc)
	return d
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
	characters     map[string]models.CharacterAssignment // the viewer's active character per campaign
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
	if v.characters, err = db.GetActiveAssignmentsByUser(v.userID); err != nil {
		return nil, "failed to retrieve characters"
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
		"world_id":                   campaign.WorldID,
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
	systemID := campaignSystemID(v.systems, &campaign)
	mine, hasCharacter := v.characters[campaign.ID]
	campaignObj["character_system_id"] = systemID
	campaignObj["accepts_characters"] = systemID != ""
	campaignObj["current_user_character"] = nil
	if hasCharacter {
		campaignObj["current_user_character"] = map[string]interface{}{"id": mine.CharacterID, "name": mine.CharacterName, "status": mine.CharacterStatus}
	}
	campaignObj["character_nudge"] = isPlayer && campaign.DM != currentUserID && systemID != "" && !hasCharacter &&
		campaign.Status != models.Finished && campaign.Status != models.Cancelled
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
	journals, err := db.GetJournalSummaries(campaign.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve journal entries")
		return
	}

	assignments, err := db.GetCampaignAssignments(campaign.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve characters")
		return
	}
	// Who plays what now, and (for a player without one) the character from their last assignment here.
	playerCharacters := map[string]interface{}{}
	previous := ""
	for _, a := range assignments { // oldest first, so the last ended one wins
		if a.EndedAt == "" {
			playerCharacters[a.UserID] = map[string]interface{}{"id": a.CharacterID, "name": a.CharacterName, "status": a.CharacterStatus}
		} else if a.UserID == v.userID && a.CharacterName != "" {
			previous = a.CharacterID
		}
	}
	if _, has := playerCharacters[v.userID]; has {
		previous = ""
	}
	obj["player_characters"] = playerCharacters
	obj["previous_character_id"] = previous

	list := make([]map[string]interface{}, 0, len(played))
	for i := len(played) - 1; i >= 0; i-- {
		s := played[i]
		row := map[string]interface{}{
			"id":        s.ID,
			"number":    i + 1,
			"date":      s.Date,
			"time":      s.Time,
			"duration":  s.Duration,
			"timezone":  s.Timezone,
			"manual":    s.Manual,
			"attendees": playedAttendees(campaign, v.users, s, responses[s.ID], assignments),
			"journal":   nil,
		}
		if j, ok := journals[s.ID]; ok {
			row["journal"] = journalSummaryObject(v.users, j)
			delete(journals, s.ID)
		}
		list = append(list, row)
	}
	obj["played_sessions"] = list
	// Entries left over belong to sessions that were cancelled after someone wrote them.
	cancelled := []map[string]interface{}{}
	for sessionID, j := range journals {
		session, err := db.GetSessionByID(sessionID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sessions")
			return
		}
		if session == nil {
			continue
		}
		entry := journalSummaryObject(v.users, j)
		entry["session_id"], entry["date"] = sessionID, session.Date
		cancelled = append(cancelled, entry)
	}
	sort.Slice(cancelled, func(a, b int) bool { return cancelled[a]["date"].(string) > cancelled[b]["date"].(string) })
	obj["journal_cancelled"] = cancelled
	upcoming, err := upcomingSessions(campaign, v)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sessions")
		return
	}
	obj["upcoming_sessions"] = upcoming
	writeJSON(w, http.StatusOK, obj)
}

// playedAttendees lists who was at a played session. The DM runs every played session unless
// a by-hand entry says they missed it.
// Players carry the character that was active on the session's date (assignments may be nil).
func playedAttendees(campaign *models.Campaign, users map[string]*models.User, s models.CampaignSession, responses []models.SessionResponse, assignments []models.CharacterAssignment) []map[string]interface{} {
	attendees := []map[string]interface{}{}
	if dm, ok := users[campaign.DM]; ok && dm != nil && !s.DMAbsent {
		attendees = append(attendees, userDisplayObject(dm))
	}
	for _, resp := range responses {
		if resp.UserID == campaign.DM || resp.Participation != models.Accepted {
			continue
		}
		if u, ok := users[resp.UserID]; ok && u != nil {
			obj := userDisplayObject(u)
			if a := characterOnDate(assignments, resp.UserID, s.Date, s.Timezone); a != nil {
				obj["character"] = map[string]interface{}{"id": a.CharacterID, "name": a.CharacterName}
			}
			attendees = append(attendees, obj)
		}
	}
	for _, name := range s.Guests {
		attendees = append(attendees, guestDisplayObject(name))
	}
	return attendees
}

// characterOnDate is the user's assignment covering date d (YYYY-MM-DD) in the session's timezone
// (UTC when unset or unknown): started on or before it and not ended before it. The latest start
// wins (a same-day swap); nil when none covers it.
func characterOnDate(assignments []models.CharacterAssignment, userID, d, timezone string) *models.CharacterAssignment {
	loc := time.UTC
	if timezone != "" {
		if l, err := time.LoadLocation(timezone); err == nil {
			loc = l
		}
	}
	day := func(ts string) (string, bool) {
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return "", false
		}
		return t.In(loc).Format("2006-01-02"), true
	}
	var best *models.CharacterAssignment
	for i := range assignments {
		a := &assignments[i]
		if a.UserID != userID || a.CharacterName == "" {
			continue
		}
		start, ok := day(a.StartedAt)
		if !ok || start > d {
			continue
		}
		if a.EndedAt != "" {
			if end, ok := day(a.EndedAt); !ok || end < d {
				continue
			}
		}
		if best == nil || a.StartedAt >= best.StartedAt {
			best = a
		}
	}
	return best
}

// guestDisplayObject is the attendee object for a named non-member: no id, no profile.
func guestDisplayObject(name string) map[string]interface{} {
	return map[string]interface{}{"guest": true, "display_name": name, "avatar_letter": avatarLetter(name)}
}

type sessionReply struct {
	User          map[string]interface{}     `json:"user"`
	Participation models.ParticipationStatus `json:"participation"` // empty: awaiting
}

type upcomingSession struct {
	models.CampaignSession
	Replies []sessionReply `json:"replies"`
}

// upcomingSessions lists a campaign's sessions that haven't ended. Members get every
// live suggestion and confirmation with replies; visitors only the next confirmed
// session, with no replies, since the session endpoints are member-only.
func upcomingSessions(campaign *models.Campaign, v *campaignViewer) ([]upcomingSession, error) {
	result := []upcomingSession{}
	if !hasCampaignSessionAccess(campaign, v.userID) {
		if next := v.sessions[campaign.ID].next; next != nil {
			result = append(result, upcomingSession{CampaignSession: *next, Replies: []sessionReply{}})
		}
		return result, nil
	}

	now := time.Now()
	sessions, err := db.GetSessionsByCampaignIDs([]string{campaign.ID}, "")
	if err != nil {
		return nil, err
	}
	ids := []string{}
	live := []models.CampaignSession{}
	starts := map[string]time.Time{}
	for _, session := range sessions {
		if session.Manual || session.Status == models.SessionCancelled {
			continue
		}
		start, ok := sessionStart(session)
		if !ok || !start.Add(time.Duration(session.Duration)*time.Minute).After(now) {
			continue
		}
		live = append(live, session)
		starts[session.ID] = start
		ids = append(ids, session.ID)
	}
	sort.SliceStable(live, func(a, b int) bool { return starts[live[a].ID].Before(starts[live[b].ID]) })
	responses, err := db.GetSessionResponsesBySessionIDs(ids)
	if err != nil {
		return nil, err
	}
	for _, session := range live {
		// Current players only; the counts come from the same replies so tally and roster agree.
		byUser := map[string]models.SessionResponse{}
		for _, resp := range responses[session.ID] {
			byUser[resp.UserID] = resp
		}
		replies := []sessionReply{}
		session.AcceptedCount, session.TentativeCount, session.DeclinedCount, session.PendingCount = 0, 0, 0, 0
		for _, playerID := range campaign.Players {
			u, ok := v.users[playerID]
			if !ok || u == nil {
				continue
			}
			resp, answered := byUser[playerID]
			replies = append(replies, sessionReply{User: userDisplayObject(u), Participation: resp.Participation})
			switch {
			case !answered:
				session.PendingCount++
			case resp.Participation == models.Accepted:
				session.AcceptedCount++
			case resp.Participation == models.Tentative:
				session.TentativeCount++
			case resp.Participation == models.Declined:
				session.DeclinedCount++
			}
		}
		session.CurrentUserParticipation = byUser[v.userID].Participation
		result = append(result, upcomingSession{CampaignSession: session, Replies: replies})
	}
	return result, nil
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
	WorldID            *string             `json:"world_id"` // update only: the Codex world the game is set in; "" clears it
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
	case utf8.RuneCountInString(req.Summary) > 1000:
		return "summary must be at most 1000 characters"
	case utf8.RuneCountInString(req.Description) > 20000:
		return "description must be at most 20000 characters"
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
	// Case-only changes resolve to the same id and pass.
	byName, err := systemsByName()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve systems")
		return
	}
	// Validate the world before anything is saved ("unsorted" and "" both mean no world).
	newWorld := campaign.WorldID
	if userID, _ := r.Context().Value("user_id").(string); req.WorldID != nil && codexAllowedFor(userID) {
		newWorld = codexWorldParam(*req.WorldID)
		if newWorld != "" {
			world, err := db.GetWorld(newWorld)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to retrieve world")
				return
			}
			if world == nil {
				writeJSONError(w, http.StatusBadRequest, db.ErrWorldNotFound.Error())
				return
			}
		}
	}
	probe := *campaign
	probe.System = system
	idChanges := campaignSystemID(byName, campaign) != campaignSystemID(byName, &probe)

	if err := db.UpdateCampaignDetails(campaignID, req.Title, req.Summary, req.Description, req.Type, system, playerCountString(req.DesiredPlayerCount), idChanges); err != nil {
		if errors.Is(err, db.ErrCampaignNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, db.ErrCharacterAssigned) {
			writeJSONError(w, http.StatusConflict, fmt.Sprintf("Players have characters in %s here. They must clear them before the system can change.", campaign.System))
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to update game")
		return
	}
	if newWorld != campaign.WorldID {
		if err := db.SetCampaignWorld(campaignID, newWorld); err != nil {
			if errors.Is(err, db.ErrWorldNotFound) {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "failed to set the game's world")
			return
		}
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
