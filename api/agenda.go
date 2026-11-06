package api

import (
	"net/http"
	"sort"
	"time"
	"waystone-web/db"
	"waystone-web/models"
)

// agendaItem is one session on the Home page: the session with its replies (as in a game's
// upcoming sessions), the game it belongs to and what the viewer is to that game.
type agendaItem struct {
	upcomingSession
	Campaign map[string]interface{} `json:"campaign"`
	Role     string                 `json:"role"`     // dm, player or follower
	StartAt  string                 `json:"start_at"` // RFC3339 instant
	EndAt    string                 `json:"end_at"`
	EndTime  string                 `json:"end_time"` // HH:MM in the session's own timezone
}

// HandleAgenda returns what Home shows: sessions in progress, sessions awaiting the viewer's
// reply, the road ahead, the viewer's games and how many games are open to join (GET /api/agenda).
func HandleAgenda(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	campaigns, err := db.GetAllCampaigns()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve games")
		return
	}
	v, msg := loadCampaignViewer(r, campaigns)
	if msg != "" {
		writeJSONError(w, http.StatusInternalServerError, msg)
		return
	}

	now := time.Now()
	inProgress, awaiting, ahead := []agendaItem{}, []agendaItem{}, []agendaItem{}
	games := []map[string]interface{}{}
	openToJoin := 0

	for _, campaign := range campaigns {
		campaign := campaign
		role := ""
		switch {
		case v.userID != "" && campaign.DM == v.userID:
			role = "dm"
		case containsString(campaign.Players, v.userID):
			role = "player"
		}
		isMember := role != ""
		live := campaign.Status != models.Finished && campaign.Status != models.Cancelled

		if isMember {
			obj := v.enrich(campaign)
			obj["role"] = role
			games = append(games, obj)
		} else if live && (campaign.SignUpsOpen || v.signups[campaign.ID].Status == models.SignupPending) {
			openToJoin++ // the same games the Games page lists under Open to join
		}

		if !live {
			continue
		}
		if !isMember {
			if !v.followed[campaign.ID] {
				continue
			}
			role = "follower"
		}

		sessions, err := upcomingSessions(&campaign, v)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sessions")
			return
		}
		for _, session := range sessions {
			// Suggestions are availability requests: only the players and the DM see them.
			if session.Manual || session.Status == models.SessionCancelled || (!isMember && session.Status != models.SessionConfirmed) {
				continue
			}
			start, ok := sessionStart(session.CampaignSession)
			if !ok {
				continue
			}
			end := start.Add(time.Duration(session.Duration) * time.Minute)
			if !end.After(now) {
				continue
			}
			item := agendaItem{
				upcomingSession: session,
				Campaign:        agendaCampaign(v, campaign),
				Role:            role,
				StartAt:         start.UTC().Format(time.RFC3339),
				EndAt:           end.UTC().Format(time.RFC3339),
				EndTime:         end.In(start.Location()).Format("15:04"),
			}
			switch {
			case session.Status == models.SessionConfirmed && !start.After(now):
				inProgress = append(inProgress, item)
			case session.Status == models.SessionSuggested && role == "player" && session.CurrentUserParticipation == "":
				awaiting = append(awaiting, item)
			case session.Status == models.SessionConfirmed:
				// Other suggestions (already answered, or the viewer's own as DM) need nothing here.
				ahead = append(ahead, item)
			}
		}
	}

	byStart := func(items []agendaItem) {
		sort.SliceStable(items, func(a, b int) bool { return items[a].StartAt < items[b].StartAt })
	}
	byStart(inProgress)
	byStart(awaiting)
	byStart(ahead)
	if len(ahead) > 12 {
		ahead = ahead[:12]
	}
	sort.SliceStable(games, func(a, b int) bool {
		return agendaStatusRank(games[a]["status"]) < agendaStatusRank(games[b]["status"])
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"in_progress":        inProgress,
		"awaiting":           awaiting,
		"ahead":              ahead,
		"games":              games,
		"open_to_join_count": openToJoin,
	})
}

// agendaCampaign is the slice of a game an agenda row needs.
func agendaCampaign(v *campaignViewer, campaign models.Campaign) map[string]interface{} {
	obj := map[string]interface{}{
		"id":     campaign.ID,
		"title":  campaign.Title,
		"status": campaign.Status,
		"type":   campaign.Type,
		"dm":     campaign.DM,
	}
	if dm, ok := v.users[campaign.DM]; ok && dm != nil {
		obj["dm_user"] = userDisplayObject(dm)
	}
	return obj
}

func agendaStatusRank(status interface{}) int {
	switch status {
	case models.Pitch:
		return 0
	case models.Ongoing:
		return 1
	case models.OnHiatus:
		return 2
	case models.Finished:
		return 3
	case models.Cancelled:
		return 4
	}
	return 5
}

func containsString(list []string, s string) bool {
	if s == "" {
		return false
	}
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
