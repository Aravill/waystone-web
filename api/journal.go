package api

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
	"waystone-web/db"
	"waystone-web/models"
)

// The game journal lives in the codex: a session's entry is a page with pages.session_id set (see
// db/sqlite_journal.go). This file keeps the one write route, the number check and the user
// helpers the codex code shares.

// HandleJournal handles POST /api/campaigns/{id}/journal/{sid}: it starts the session's journal
// page (or returns the one it already has).
func HandleJournal(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) != 3 || parts[1] != "journal" || parts[0] == "" || parts[2] == "" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	handleStartJournal(w, r, parts[0], parts[2])
}

// loadJournalSession authenticates the caller and loads the campaign and one of its sessions,
// writing an error response and returning ok=false on failure.
func loadJournalSession(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) (*models.Campaign, *models.CampaignSession, string, bool) {
	_, userID, ok := loadCampaignAndUser(w, r, campaignID)
	if !ok {
		return nil, nil, "", false
	}
	campaign, session, err := validateSessionCampaign(campaignID, sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve session")
		return nil, nil, "", false
	}
	if campaign == nil || session == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return nil, nil, "", false
	}
	return campaign, session, userID, true
}

func journalConflict(w http.ResponseWriter, code, message string) {
	writeJSON(w, http.StatusConflict, map[string]string{"error": message, "code": code})
}

func journalPageResult(w http.ResponseWriter, status int, id, codexID string) {
	writeJSON(w, status, map[string]interface{}{"page": map[string]interface{}{"id": id, "codex_id": codexID}})
}

func handleStartJournal(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) {
	campaign, session, userID, ok := loadJournalSession(w, r, campaignID, sessionID)
	if !ok {
		return
	}
	_, played, err := journalSession(campaign, sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sessions")
		return
	}
	if !played {
		journalConflict(w, "session_not_played", "only a session that has been played can have a journal entry")
		return
	}
	if campaign.CodexID == "" {
		journalConflict(w, "no_codex", "link a codex to this game to keep a journal")
		return
	}
	rt, err := loadCodexRights(userID)
	if err != nil {
		codexServerError(w, "retrieve rights")
		return
	}
	if !rt.canEdit(campaign.CodexID) {
		writeJSONError(w, http.StatusForbidden, "only someone who can edit the game's codex can write its journal")
		return
	}
	if id, err := db.GetJournalPageID(sessionID); err != nil {
		codexServerError(w, "retrieve journal page")
		return
	} else if id != "" {
		pageCodex, err := db.GetPageCodexID(id)
		if err != nil {
			codexServerError(w, "retrieve journal page")
			return
		}
		journalPageResult(w, http.StatusOK, id, pageCodex)
		return
	}

	_, template, err := db.EnsureJournalFolder(campaign.ID, campaign.CodexID, userID)
	if err != nil {
		journalStoreError(w, err)
		return
	}
	lang, ok := codexLanguage(w, campaign.CodexID)
	if !ok {
		return
	}
	date := session.Date
	if t, err := time.Parse("2006-01-02", session.Date); err == nil {
		date = codexDate(t, lang)
	}
	base := db.JournalPageBase(campaign.Title, session.Date)
	body, tags, ok := pageStartOn(w, template, campaign.CodexID, lang, base, "", date)
	if !ok {
		return
	}
	if tags == nil {
		tags = []string{}
	}
	page := models.Page{ID: db.GenerateUUID(), CodexID: campaign.CodexID, AuthorID: userID, SessionID: sessionID}
	id, created, err := db.CreateJournalPage(campaign.ID, page, base, body, models.NewPageExtras{TagNames: tags})
	if err != nil {
		journalStoreError(w, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	journalPageResult(w, status, id, campaign.CodexID)
}

func journalStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrJournalSessionGone):
		writeJSONError(w, http.StatusNotFound, "session not found")
	case errors.Is(err, db.ErrCodexNotFound):
		journalConflict(w, "no_codex", "link a codex to this game to keep a journal")
	default:
		log.Printf("journal: %v", err)
		codexStoreError(w, err, "write the journal page")
	}
}

// journalSession reports whether a session has been played and, if so, its number. Numbers are
// positional (see campaignSessionSummaries), so they are computed here and never stored.
func journalSession(campaign *models.Campaign, sessionID string) (number int, played bool, err error) {
	summaries, err := campaignSessionSummaries([]models.Campaign{*campaign})
	if err != nil {
		return 0, false, err
	}
	for i, s := range summaries[campaign.ID].played {
		if s.ID == sessionID {
			return i + 1, true, nil
		}
	}
	return 0, false, nil
}

// journalSessionPayload is the `session` object of a journal page's payload (nil for any other
// page): the game and session it belongs to, the session's number (null unless it was played, and
// computed now, never stored) and who was at the table.
func journalSessionPayload(page *models.Page, users map[string]*models.User) (interface{}, error) {
	if page.SessionID == "" {
		return nil, nil
	}
	session, err := db.GetSessionByID(page.SessionID)
	if err != nil || session == nil {
		return nil, err
	}
	campaign, err := db.GetCampaignByID(session.CampaignID)
	if err != nil || campaign == nil {
		return nil, err
	}
	number, played, err := journalSession(campaign, session.ID)
	if err != nil {
		return nil, err
	}
	var numberValue interface{} // null unless the session was played
	attendees := []map[string]interface{}{}
	if played {
		numberValue = number
		responses, err := db.GetSessionResponses(session.ID)
		if err != nil {
			return nil, err
		}
		assignments, err := db.GetCampaignAssignments(campaign.ID)
		if err != nil {
			assignments = nil
		}
		attendees = playedAttendees(campaign, users, *session, responses, assignments)
	}
	return map[string]interface{}{
		"campaign_id": campaign.ID, "campaign_title": campaign.Title, "session_id": session.ID,
		"number": numberValue, "date": session.Date, "status": session.Status, "played": played,
		"attendees": attendees,
	}, nil
}

// journalUsers maps every user id (anonymised users included) to its display object.
func journalUsers() (map[string]*models.User, error) {
	all, err := db.GetAllUsers()
	if err != nil {
		return nil, err
	}
	users := make(map[string]*models.User, len(all))
	for i := range all {
		users[all[i].ID] = &all[i]
	}
	return users, nil
}

func journalUser(users map[string]*models.User, id string) map[string]interface{} {
	if u, ok := users[id]; ok && u != nil {
		return userDisplayObject(u)
	}
	return map[string]interface{}{"id": id, "display_name": "Deleted user", "avatar_url": "", "avatar_letter": "?"}
}

func journalUserList(users map[string]*models.User, ids []string) []map[string]interface{} {
	list := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		list = append(list, journalUser(users, id))
	}
	return list
}

// journalPageObject is the digest of a session's journal page shown on the game page.
func (v *campaignViewer) journalPageObject(j models.JournalPage) map[string]interface{} {
	return map[string]interface{}{
		"page_id":        j.PageID,
		"title":          j.Title,
		"codex_id":       j.CodexID,
		"codex_name":     v.codices[j.CodexID],
		"can_open":       v.codexRights.canSee(j.CodexID),
		"revisions":      j.Revisions,
		"updated_at":     j.UpdatedAt,
		"excerpt_source": j.ExcerptSource,
		"author_id":      j.AuthorID,
		"author_user":    journalUser(v.users, j.AuthorID),
		"amender_ids":    j.AmenderIDs,
		"amender_users":  journalUserList(v.users, j.AmenderIDs),
	}
}

// redirectOldJournalURL sends /games/{id}/journal/{sid}[/edit|/history] to the session's codex
// page (or to the game when it has none). 303, not 301: a page can be deleted and written again
// under a new id.
func redirectOldJournalURL(w http.ResponseWriter, r *http.Request, parts []string) {
	target := "/games/" + parts[1]
	id, err := db.GetJournalPageID(parts[3])
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve journal page")
		return
	}
	if id != "" {
		target = "/codices/p/" + id + "/journal"
		if len(parts) == 5 {
			target += "/" + parts[4]
		}
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
