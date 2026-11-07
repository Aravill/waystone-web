package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

const (
	maxJournalBodyRunes = 20000
	maxJournalNameRunes = 80
	maxJournalNoteRunes = 120
)

// journalParagraphBreak splits paragraphs the same way the page's merge does (toParas in journal.js).
var journalParagraphBreak = regexp.MustCompile(`\n[ \t]*\n`)

// journalSaveRequest is the body for saving an entry. A new entry sends entry_id "" and base_revision 0.
type journalSaveRequest struct {
	EntryID      string `json:"entry_id"`
	BaseRevision int    `json:"base_revision"`
	Name         string `json:"name"`
	Body         string `json:"body"`
	Note         string `json:"note"`
	MergedWith   int    `json:"merged_with"`
}

// HandleJournal handles /api/campaigns/{id}/journal/{sid}[/revisions[/{n}/restore]].
func HandleJournal(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) < 3 || parts[1] != "journal" || parts[0] == "" || parts[2] == "" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	campaignID, sessionID := parts[0], parts[2]
	switch {
	case len(parts) == 3 && r.Method == http.MethodGet:
		handleGetJournal(w, r, campaignID, sessionID)
	case len(parts) == 3 && r.Method == http.MethodPut:
		handleSaveJournal(w, r, campaignID, sessionID)
	case len(parts) == 3 && r.Method == http.MethodDelete:
		handleDeleteJournal(w, r, campaignID, sessionID)
	case len(parts) == 4 && parts[3] == "revisions" && r.Method == http.MethodGet:
		handleGetJournalRevisions(w, r, campaignID, sessionID)
	case len(parts) == 6 && parts[3] == "revisions" && parts[5] == "restore" && r.Method == http.MethodPost:
		n, err := strconv.Atoi(parts[4])
		if err != nil || n < 1 {
			writeJSONError(w, http.StatusNotFound, "revision not found")
			return
		}
		handleRestoreJournal(w, r, campaignID, sessionID, n)
	case len(parts) == 3 || (len(parts) == 4 && parts[3] == "revisions") || (len(parts) == 6 && parts[3] == "revisions" && parts[5] == "restore"):
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
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

func journalRevisionObject(users map[string]*models.User, rev models.JournalRevision) map[string]interface{} {
	return map[string]interface{}{
		"number":        rev.Number,
		"author_id":     rev.AuthorID,
		"author_user":   journalUser(users, rev.AuthorID),
		"name":          rev.Name,
		"body":          rev.Body,
		"note":          rev.Note,
		"restored_from": rev.RestoredFrom,
		"merged_with":   rev.MergedWith,
		"created_at":    rev.CreatedAt,
	}
}

// journalSummaryObject is the digest of an entry shown on the game page.
func journalSummaryObject(users map[string]*models.User, j models.JournalSummary) map[string]interface{} {
	return map[string]interface{}{
		"entry_id":       j.EntryID,
		"name":           j.Name,
		"revisions":      j.Revisions,
		"updated_at":     j.UpdatedAt,
		"excerpt_source": j.ExcerptSource,
		"author_id":      j.AuthorID,
		"author_user":    journalUser(users, j.AuthorID),
		"amender_ids":    j.AmenderIDs,
		"amender_users":  journalUserList(users, j.AmenderIDs),
	}
}

// isCampaignManager reports whether the user is the game's DM or a site admin.
func isCampaignManager(campaign *models.Campaign, userID string) (bool, error) {
	if campaign.DM == userID {
		return true, nil
	}
	user, err := db.GetUserByID(userID)
	if err != nil {
		return false, err
	}
	return user != nil && user.IsAdmin(), nil
}

func handleGetJournal(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) {
	campaign, session, userID, ok := loadJournalSession(w, r, campaignID, sessionID)
	if !ok {
		return
	}
	number, played, err := journalSession(campaign, sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sessions")
		return
	}
	manager, err := isCampaignManager(campaign, userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve user")
		return
	}
	entry, err := db.GetJournalEntry(sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve journal entry")
		return
	}
	users, err := journalUsers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve users")
		return
	}
	responses, err := db.GetSessionResponses(sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sessions")
		return
	}

	assignments, err := db.GetCampaignAssignments(campaign.ID)
	if err != nil {
		log.Printf("error fetching assignments: %v", err)
		assignments = nil
	}

	var numberValue interface{} // null unless the session was played
	if played {
		numberValue = number
	}
	obj := map[string]interface{}{
		"session": map[string]interface{}{
			"id":        session.ID,
			"number":    numberValue,
			"played":    played,
			"status":    session.Status,
			"date":      session.Date,
			"time":      session.Time,
			"duration":  session.Duration,
			"timezone":  session.Timezone,
			"manual":    session.Manual,
			"attendees": playedAttendees(campaign, users, *session, responses, assignments),
		},
		"can_edit":   played && hasCampaignSessionAccess(campaign, userID),
		"can_delete": manager,
		"entry":      nil,
	}
	if entry != nil {
		// The author leads the scribes; the rest keep their first-revision order.
		amenders := []string{}
		scribes := []map[string]interface{}{}
		for pass := 0; pass < 2; pass++ {
			for _, c := range entry.Scribes {
				if (c.UserID == entry.AuthorID) != (pass == 0) {
					continue
				}
				if pass == 1 {
					amenders = append(amenders, c.UserID)
				}
				scribes = append(scribes, map[string]interface{}{"user_id": c.UserID, "user": journalUser(users, c.UserID), "revisions": c.Revisions})
			}
		}
		obj["entry"] = map[string]interface{}{
			"id":               entry.ID,
			"session_id":       entry.SessionID,
			"campaign_id":      entry.CampaignID,
			"current_revision": entry.Latest.Number,
			"revision_count":   entry.RevisionCount,
			"name":             entry.Latest.Name,
			"body":             entry.Latest.Body,
			"updated_at":       entry.Latest.CreatedAt,
			"author_id":        entry.AuthorID,
			"author_user":      journalUser(users, entry.AuthorID),
			"amender_ids":      amenders,
			"amender_users":    journalUserList(users, amenders),
			"updated_by":       entry.Latest.AuthorID,
			"updated_by_user":  journalUser(users, entry.Latest.AuthorID),
			"scribes":          scribes,
		}
	}
	writeJSON(w, http.StatusOK, obj)
}

func handleGetJournalRevisions(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) {
	if _, _, _, ok := loadJournalSession(w, r, campaignID, sessionID); !ok {
		return
	}
	entryID, revs, err := db.GetJournalRevisions(sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve revisions")
		return
	}
	if entryID == "" || len(revs) == 0 {
		writeJSONError(w, http.StatusNotFound, "journal entry not found")
		return
	}
	users, err := journalUsers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve users")
		return
	}
	list := make([]map[string]interface{}, 0, len(revs))
	for _, rev := range revs {
		list = append(list, journalRevisionObject(users, rev))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"entry_id":         entryID,
		"current_revision": revs[len(revs)-1].Number,
		"pruned":           revs[0].Number > 1,
		"revisions":        list,
	})
}

func handleDeleteJournal(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) {
	campaign, _, userID, ok := loadJournalSession(w, r, campaignID, sessionID)
	if !ok {
		return
	}
	manager, err := isCampaignManager(campaign, userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve user")
		return
	}
	if !manager {
		writeJSONError(w, http.StatusForbidden, "only the game DM or an admin can delete a journal entry")
		return
	}
	if err := db.DeleteJournalEntry(sessionID); err != nil {
		if errors.Is(err, db.ErrJournalNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to delete journal entry")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "journal entry deleted"})
}

func handleSaveJournal(w http.ResponseWriter, r *http.Request, campaignID, sessionID string) {
	campaign, _, userID, ok := loadJournalSession(w, r, campaignID, sessionID)
	if !ok {
		return
	}
	if !hasCampaignSessionAccess(campaign, userID) {
		writeJSONError(w, http.StatusForbidden, "only the game DM and players can write the journal")
		return
	}
	var req journalSaveRequest
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rev, msg := req.normalize()
	if msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	rev.AuthorID = userID
	saveJournalRevision(w, campaign, sessionID, req.EntryID, req.BaseRevision, rev)
}

func handleRestoreJournal(w http.ResponseWriter, r *http.Request, campaignID, sessionID string, number int) {
	campaign, _, userID, ok := loadJournalSession(w, r, campaignID, sessionID)
	if !ok {
		return
	}
	if !hasCampaignSessionAccess(campaign, userID) {
		writeJSONError(w, http.StatusForbidden, "only the game DM and players can restore a revision")
		return
	}
	var req struct {
		BaseRevision int `json:"base_revision"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BaseRevision < 1 {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	entryID, revs, err := db.GetJournalRevisions(sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve revisions")
		return
	}
	var source *models.JournalRevision
	for i := range revs {
		if revs[i].Number == number {
			source = &revs[i]
		}
	}
	if entryID == "" || source == nil {
		writeJSONError(w, http.StatusNotFound, "revision not found")
		return
	}
	saveJournalRevision(w, campaign, sessionID, entryID, req.BaseRevision, models.JournalRevision{
		AuthorID: userID, Name: source.Name, Body: source.Body, RestoredFrom: number,
	})
}

// saveJournalRevision checks that the session has been played, saves, and writes the response.
func saveJournalRevision(w http.ResponseWriter, campaign *models.Campaign, sessionID, entryID string, base int, rev models.JournalRevision) {
	_, played, err := journalSession(campaign, sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve sessions")
		return
	}
	conflict := func(code, message, entry string, latest map[string]interface{}) {
		body := map[string]interface{}{"error": message, "code": code, "entry_id": entry, "latest": nil}
		if latest != nil {
			body["latest"] = latest
		}
		writeJSON(w, http.StatusConflict, body)
	}
	if !played {
		conflict("session_not_played", "only a session that has been played can have a journal entry", entryID, nil)
		return
	}
	result, err := db.SaveJournalRevision(campaign.ID, sessionID, entryID, base, rev)
	if err != nil {
		if errors.Is(err, db.ErrJournalSessionGone) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to save journal entry")
		return
	}
	users, err := journalUsers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve users")
		return
	}
	var latest map[string]interface{}
	if result.Latest != nil {
		latest = journalRevisionObject(users, *result.Latest)
	}
	switch result.Outcome {
	case models.JournalStale:
		conflict("stale_base", "the entry changed while you were editing", result.EntryID, latest)
	case models.JournalReplaced:
		conflict("entry_replaced", "the entry was deleted and written again while you were editing", result.EntryID, latest)
	case models.JournalDeleted:
		conflict("entry_deleted", "the entry was deleted while you were editing", "", nil)
	default:
		status, message, created := http.StatusOK, "no changes to save", false
		switch result.Outcome {
		case models.JournalCreated:
			status, message, created = http.StatusCreated, "entry saved", true
		case models.JournalSaved:
			status, message = http.StatusCreated, "revision saved"
		}
		writeJSON(w, status, map[string]interface{}{
			"status":   "success",
			"message":  message,
			"created":  created,
			"entry_id": result.EntryID,
			"revision": journalRevisionObject(users, result.Revision),
		})
	}
}

// normalize validates the request and returns the revision to save (without author) and a
// validation message, or "". The body is stored in canonical form: LF line breaks and
// paragraphs separated by exactly one blank line, so "no change" is an exact comparison.
func (req *journalSaveRequest) normalize() (models.JournalRevision, string) {
	var rev models.JournalRevision
	clean := func(s string) string { return strings.ReplaceAll(s, "\x00", "") }
	rev.Name = strings.Join(strings.Fields(clean(req.Name)), " ")
	rev.Note = strings.Join(strings.Fields(clean(req.Note)), " ")
	body := strings.ReplaceAll(strings.ReplaceAll(clean(req.Body), "\r\n", "\n"), "\r", "\n")
	var paras []string
	for _, p := range journalParagraphBreak.Split(body, -1) {
		if p = strings.TrimSpace(p); p != "" {
			paras = append(paras, p)
		}
	}
	rev.Body = strings.Join(paras, "\n\n")
	rev.MergedWith = req.MergedWith

	switch {
	case rev.Body == "":
		return rev, "entry text is required"
	case utf8.RuneCountInString(rev.Body) > maxJournalBodyRunes:
		return rev, "entry text must be at most 20000 characters"
	case utf8.RuneCountInString(rev.Name) > maxJournalNameRunes:
		return rev, "session name must be at most 80 characters"
	case utf8.RuneCountInString(rev.Note) > maxJournalNoteRunes:
		return rev, "note must be at most 120 characters"
	case req.BaseRevision < 0:
		return rev, "invalid base revision"
	case req.MergedWith != 0 && req.MergedWith != req.BaseRevision:
		return rev, "merged_with must be 0 or the base revision"
	}
	return rev, ""
}
