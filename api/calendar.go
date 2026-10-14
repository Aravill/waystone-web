package api

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

// HandleCalendarToken lets the signed-in user read, create or regenerate
// their private calendar feed token.
func HandleCalendarToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	switch r.Method {
	case http.MethodGet:
		token, err := db.GetCalendarToken(userID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to retrieve calendar token")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"token": token})
	case http.MethodPost:
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}
		token := base64.RawURLEncoding.EncodeToString(buf)
		if err := db.SetCalendarToken(userID, token); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to save calendar token")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"token": token})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleCalendarFeed serves /calendar/{token}.ics. It is intentionally not
// behind the auth middleware: calendar apps can't log in.
func HandleCalendarFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/calendar/")
	if !strings.HasSuffix(name, ".ics") {
		http.NotFound(w, r)
		return
	}
	token := strings.TrimSuffix(name, ".ics")

	userID, err := db.GetUserIDByCalendarToken(token)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if userID == "" {
		http.NotFound(w, r)
		return
	}
	user, err := db.GetUserByID(userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if user == nil || user.Blocked {
		http.NotFound(w, r)
		return
	}

	campaigns, err := db.GetAllCampaigns()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	campaignByID := map[string]*models.Campaign{}
	campaignIDs := []string{}
	for i := range campaigns {
		if hasCampaignSessionAccess(&campaigns[i], userID) {
			campaignByID[campaigns[i].ID] = &campaigns[i]
			campaignIDs = append(campaignIDs, campaigns[i].ID)
		}
	}

	fromDate := time.Now().UTC().AddDate(0, 0, -365).Format("2006-01-02")
	sessions, err := db.GetSessionsByCampaignIDs(campaignIDs, fromDate)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	sessionIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.ID)
	}
	responsesBySession, err := db.GetSessionResponsesBySessionIDs(sessionIDs)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	for i := range sessions {
		applySessionCounts(&sessions[i], campaignByID[sessions[i].CampaignID], responsesBySession[sessions[i].ID], userID)
	}

	body := buildCalendarFeed(sessions, campaignByID, userID)

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="waystone.ics"`)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte(body))
	}
}

func buildCalendarFeed(sessions []models.CampaignSession, campaignByID map[string]*models.Campaign, userID string) string {
	var b strings.Builder
	for _, line := range []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Waystone//Waystone Web//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"X-WR-CALNAME:Waystone sessions",
		"REFRESH-INTERVAL;VALUE=DURATION:PT1H",
		"X-PUBLISHED-TTL:PT1H",
	} {
		b.WriteString(icsLine(line))
	}

	for _, session := range sessions {
		campaign := campaignByID[session.CampaignID]
		if campaign == nil {
			continue
		}
		isDM := campaign.DM == userID
		if session.Status == models.SessionSuggested && !isDM && session.CurrentUserParticipation == models.Declined {
			continue
		}

		start, end, ok := sessionTimes(session)
		if !ok {
			continue
		}

		var status, summary, transp string
		switch session.Status {
		case models.SessionConfirmed:
			status, summary, transp = "CONFIRMED", campaign.Title, "OPAQUE"
		case models.SessionCancelled:
			status, summary, transp = "CANCELLED", "[cancelled] "+campaign.Title, "TRANSPARENT"
		default:
			status, summary, transp = "TENTATIVE", "[suggested] "+campaign.Title, "TRANSPARENT"
		}

		role := "Player"
		yourResponse := "none"
		if isDM {
			role = "DM"
		}
		if session.CurrentUserParticipation != "" {
			yourResponse = string(session.CurrentUserParticipation)
		}
		description := fmt.Sprintf("Role: %s\nStatus: %s\nYour response: %s\nResponses: %d accepted, %d tentative, %d declined, %d pending",
			role, session.Status, yourResponse,
			session.AcceptedCount, session.TentativeCount, session.DeclinedCount, session.PendingCount)

		stamp := time.Now().UTC()
		if t, err := time.Parse(time.RFC3339, session.UpdatedAt); err == nil {
			stamp = t.UTC()
		}
		stampStr := stamp.Format("20060102T150405Z")

		b.WriteString(icsLine("BEGIN:VEVENT"))
		b.WriteString(icsLine("UID:" + session.ID + "@waystone-web"))
		b.WriteString(icsLine("DTSTAMP:" + stampStr))
		b.WriteString(icsLine("LAST-MODIFIED:" + stampStr))
		b.WriteString(icsLine("DTSTART:" + start))
		b.WriteString(icsLine("DTEND:" + end))
		b.WriteString(icsLine("SUMMARY:" + icsEscape(summary)))
		b.WriteString(icsLine("DESCRIPTION:" + icsEscape(description)))
		b.WriteString(icsLine("STATUS:" + status))
		b.WriteString(icsLine("TRANSP:" + transp))
		b.WriteString(icsLine("END:VEVENT"))
	}

	b.WriteString(icsLine("END:VCALENDAR"))
	return b.String()
}

// sessionTimes returns DTSTART/DTEND values: UTC (with Z) when the session has
// a timezone, floating local time for legacy sessions without one.
func sessionTimes(session models.CampaignSession) (string, string, bool) {
	loc := time.UTC
	if session.Timezone != "" {
		l, err := time.LoadLocation(session.Timezone)
		if err != nil {
			return "", "", false
		}
		loc = l
	}
	start, err := time.ParseInLocation("2006-01-02 15:04", session.Date+" "+session.Time, loc)
	if err != nil {
		return "", "", false
	}
	end := start.Add(time.Duration(session.Duration) * time.Minute)
	if session.Timezone == "" {
		return start.Format("20060102T150405"), end.Format("20060102T150405"), true
	}
	return start.UTC().Format("20060102T150405Z"), end.UTC().Format("20060102T150405Z"), true
}

func icsEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ";", `\;`)
	s = strings.ReplaceAll(s, ",", `\,`)
	s = strings.ReplaceAll(s, "\r\n", `\n`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\n`)
	return s
}

// icsLine folds a content line at 75 octets (never inside a UTF-8 sequence)
// and terminates it with CRLF.
func icsLine(line string) string {
	var b strings.Builder
	limit := 75
	for len(line) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		b.WriteString(line[:cut])
		b.WriteString("\r\n ")
		line = line[cut:]
		limit = 74 // continuation lines carry a leading space
	}
	b.WriteString(line)
	b.WriteString("\r\n")
	return b.String()
}
