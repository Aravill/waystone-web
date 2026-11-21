package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

const (
	maxTimelineYear     = 1000000000
	maxTimelineMonths   = 100
	maxTimelineMonthLen = 40
	maxTimelineEraLen   = 12
	maxTimelineTitle    = 120
)

// codexTimelineError writes the response for the errors the timeline store returns, then falls
// back to the page errors.
func codexTimelineError(w http.ResponseWriter, err error, what string) {
	switch {
	case errors.Is(err, db.ErrNotTimeline):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "This page is not a timeline.", "code": "not_a_timeline"})
	case errors.Is(err, db.ErrEventNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error(), "code": "event_deleted"})
	case errors.Is(err, db.ErrLinkNotInCodex):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "page_not_in_codex"})
	default:
		codexStoreError(w, err, what)
	}
}

// handleCodexTimelineRoutes serves /api/codices/pages/{id}/timeline/...; false when the path is not one of its routes.
func handleCodexTimelineRoutes(w http.ResponseWriter, r *http.Request, id string, rest []string, method func(...string) bool) bool {
	if rest[0] != "timeline" {
		return false
	}
	switch {
	case len(rest) == 1:
		if method(http.MethodGet) {
			handleGetTimeline(w, r, id)
		}
	case rest[1] == "events" && len(rest) == 2:
		if method(http.MethodPost) {
			handleCreateTimelineEvent(w, r, id)
		}
	case rest[1] == "events" && len(rest) == 3:
		if method(http.MethodPut, http.MethodDelete) {
			if r.Method == http.MethodPut {
				handleUpdateTimelineEvent(w, r, id, rest[2])
			} else {
				handleDeleteTimelineEvent(w, r, id, rest[2])
			}
		}
	case rest[1] == "calendar" && len(rest) == 2:
		if method(http.MethodGet, http.MethodPut) {
			if r.Method == http.MethodGet {
				handleGetTimelineCalendar(w, r, id)
			} else {
				handleSetTimelineCalendar(w, r, id)
			}
		}
	default:
		return false
	}
	return true
}

// codexTimelinePayload is the timeline as the client shows it: {events, calendar}. Each event has
// a linked page (title, folder, excerpt, tags) or null; a link to a page that is gone counts as no link.
func codexTimelinePayload(pageID, codexID string) (map[string]interface{}, error) {
	events, err := db.GetTimelineEvents(pageID)
	if err != nil {
		return nil, err
	}
	cal, err := db.GetTimelineCalendar(pageID)
	if err != nil {
		return nil, err
	}
	linkedAny := false
	for _, e := range events {
		if e.LinkedPageID != "" {
			linkedAny = true
			break
		}
	}
	graph := map[string]models.GraphPage{}
	folderName := map[string]string{}
	if linkedAny {
		pages, _, err := db.GetCodexGraph(codexID)
		if err != nil {
			return nil, err
		}
		for _, p := range pages {
			graph[p.ID] = p
		}
		folders, err := db.GetFolders(codexID)
		if err != nil {
			return nil, err
		}
		for _, f := range folders {
			folderName[f.ID] = f.Name
		}
	}
	out := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		o := map[string]interface{}{"id": e.ID, "title": e.Title, "year": e.Year, "month": e.Month, "day": e.Day, "linked": nil}
		if p, ok := graph[e.LinkedPageID]; ok && e.LinkedPageID != "" {
			tags := p.Tags
			if tags == nil {
				tags = []string{}
			}
			o["title"] = p.Title
			o["linked"] = map[string]interface{}{
				"id": p.ID, "title": p.Title, "folder_id": p.FolderID, "folder": folderName[p.FolderID],
				"excerpt": graphExcerpt(p.Body), "tags": tags,
			}
		}
		out = append(out, o)
	}
	return map[string]interface{}{"events": out, "calendar": cal}, nil
}

func handleGetTimeline(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, false) {
		return
	}
	codexID, err := db.GetPageCodexID(id)
	if err != nil {
		codexServerError(w, "retrieve page")
		return
	}
	if kind, err := pageKind(id); err != nil {
		codexServerError(w, "retrieve page")
		return
	} else if kind != models.PageTimeline {
		codexTimelineError(w, db.ErrNotTimeline, "retrieve timeline")
		return
	}
	payload, err := codexTimelinePayload(id, codexID)
	if err != nil {
		codexServerError(w, "retrieve timeline")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// pageKind returns the kind of a page ("" when it is gone).
func pageKind(id string) (string, error) {
	p, err := db.GetPage(id)
	if err != nil || p == nil {
		return "", err
	}
	return p.Kind, nil
}

type timelineEventRequest struct {
	Title        string `json:"title"`
	LinkedPageID string `json:"linked_page_id"`
	Year         *int   `json:"year"`
	Month        int    `json:"month"`
	Day          int    `json:"day"`
	CreatePage   bool   `json:"create_page"`
}

// normalize checks the request and fills e (title, link, date). The title of a linked event is
// replaced by the store. It returns an error message, or "".
func (req *timelineEventRequest) normalize(e *models.TimelineEvent) string {
	if req.Year == nil || *req.Year > maxTimelineYear || *req.Year < -maxTimelineYear {
		return "year is required"
	}
	if req.Month < 0 || req.Month > 100 || req.Day < 0 || req.Day > 100 {
		return "month and day are out of range"
	}
	if req.Month == 0 {
		req.Day = 0
	}
	e.Year, e.Month, e.Day, e.LinkedPageID = *req.Year, req.Month, req.Day, req.LinkedPageID
	if req.LinkedPageID != "" {
		req.CreatePage = false
		return ""
	}
	if req.CreatePage {
		title, ok := codexName(req.Title)
		if !ok {
			return "title must be 1 to 120 characters and can't contain [ ] or |"
		}
		e.Title = title
		return ""
	}
	title := strings.Join(strings.Fields(req.Title), " ")
	if n := utf8.RuneCountInString(title); n < 1 || n > maxTimelineTitle {
		return "title must be 1 to 120 characters"
	}
	e.Title = title
	return ""
}

func handleCreateTimelineEvent(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req timelineEventRequest
	if !codexDecode(w, r, 16<<10, &req) {
		return
	}
	e := models.TimelineEvent{PageID: id, CreatedBy: userID}
	if msg := req.normalize(&e); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	codexID, err := db.GetPageCodexID(id)
	if err != nil {
		codexServerError(w, "retrieve page")
		return
	}
	var page interface{}
	if req.CreatePage {
		b := builtinTemplateByID("builtin:event")
		p := models.Page{ID: db.GenerateUUID(), Kind: models.PageLore, CodexID: codexID, Title: e.Title, AuthorID: userID}
		if p.FolderID, err = eventsFolderID(codexID); err != nil {
			codexServerError(w, "retrieve folders")
			return
		}
		if e, err = db.CreateTimelineEventWithPage(e, p, fillTemplate(b.Body, e.Title), models.NewPageExtras{TagNames: b.Tags}); err != nil {
			codexTimelineError(w, err, "create event")
			return
		}
		page = map[string]interface{}{"id": p.ID, "title": p.Title}
	} else if e, err = db.CreateTimelineEvent(e); err != nil {
		codexTimelineError(w, err, "create event")
		return
	}
	payload, err := codexTimelinePayload(id, codexID)
	if err != nil {
		codexServerError(w, "retrieve timeline")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"event": map[string]interface{}{"id": e.ID}, "page": page, "timeline": payload})
}

// eventsFolderID is the codex's top-level folder named "Events" ("" = top level when there is none).
// A page made for an event never creates the folder.
func eventsFolderID(codexID string) (string, error) {
	folders, err := db.GetFolders(codexID)
	if err != nil {
		return "", err
	}
	for _, f := range folders {
		if f.ParentID == "" && db.NormName(f.Name) == "events" {
			return f.ID, nil
		}
	}
	return "", nil
}

func handleUpdateTimelineEvent(w http.ResponseWriter, r *http.Request, id, eventID string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req timelineEventRequest
	if !codexDecode(w, r, 16<<10, &req) {
		return
	}
	e := models.TimelineEvent{ID: eventID, PageID: id, CreatedBy: userID}
	req.CreatePage = false
	if msg := req.normalize(&e); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	if err := db.UpdateTimelineEvent(e); err != nil {
		codexTimelineError(w, err, "update event")
		return
	}
	replyTimeline(w, id)
}

func handleDeleteTimelineEvent(w http.ResponseWriter, r *http.Request, id, eventID string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	if err := db.DeleteTimelineEvent(id, eventID); err != nil {
		codexTimelineError(w, err, "delete event")
		return
	}
	replyTimeline(w, id)
}

// replyTimeline answers 200 {timeline} after a change.
func replyTimeline(w http.ResponseWriter, id string) {
	codexID, err := db.GetPageCodexID(id)
	if err != nil {
		codexServerError(w, "retrieve page")
		return
	}
	payload, err := codexTimelinePayload(id, codexID)
	if err != nil {
		codexServerError(w, "retrieve timeline")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"timeline": payload})
}

func handleGetTimelineCalendar(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, false) {
		return
	}
	cal, err := db.GetTimelineCalendar(id)
	if err != nil {
		codexServerError(w, "retrieve calendar")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"calendar": cal})
}

func handleSetTimelineCalendar(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req models.TimelineCalendar
	if !codexDecode(w, r, 16<<10, &req) {
		return
	}
	cal := models.TimelineCalendar{Months: []string{}, Era: strings.TrimSpace(req.Era), BeforeEra: strings.TrimSpace(req.BeforeEra)}
	for _, m := range req.Months {
		if m = strings.TrimSpace(m); m != "" {
			cal.Months = append(cal.Months, m)
		}
	}
	bad := len(cal.Months) > maxTimelineMonths ||
		utf8.RuneCountInString(cal.Era) > maxTimelineEraLen || utf8.RuneCountInString(cal.BeforeEra) > maxTimelineEraLen ||
		strings.ContainsAny(cal.Era+cal.BeforeEra, "\r\n")
	for _, m := range cal.Months {
		if utf8.RuneCountInString(m) > maxTimelineMonthLen || strings.ContainsAny(m, "\r\n") {
			bad = true
		}
	}
	if bad {
		writeJSONError(w, http.StatusBadRequest, "up to 100 month names of 40 characters each, and eras of up to 12 characters")
		return
	}
	if err := db.SetTimelineCalendar(id, cal, userID); err != nil {
		codexTimelineError(w, err, "save calendar")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"calendar": cal})
}

// copyTimelineInto puts the calendar and events of a timeline into the extras of its copy.
func copyTimelineInto(x *models.NewPageExtras, srcID string, sameCodex bool) error {
	cal, err := db.GetTimelineCalendar(srcID)
	if err != nil {
		return err
	}
	events, err := db.GetTimelineEvents(srcID)
	if err != nil {
		return err
	}
	x.Calendar, x.Events = &cal, events
	if !sameCodex {
		for i := range x.Events {
			x.Events[i].LinkedPageID = ""
		}
	}
	return nil
}

// codexOnTimelines lists the timelines that have an event linking the page, for its side panel.
func codexOnTimelines(pageID string) ([]map[string]interface{}, error) {
	events, err := db.GetEventsLinkingTo(pageID)
	if err != nil {
		return nil, err
	}
	out := []map[string]interface{}{}
	calendars := map[string]models.TimelineCalendar{}
	titles := map[string]string{}
	for _, e := range events {
		cal, seen := calendars[e.PageID]
		if !seen {
			t, err := db.GetPage(e.PageID)
			if err != nil {
				return nil, err
			}
			if t == nil {
				continue
			}
			titles[e.PageID] = t.Title
			if cal, err = db.GetTimelineCalendar(e.PageID); err != nil {
				return nil, err
			}
			calendars[e.PageID] = cal
		}
		out = append(out, map[string]interface{}{
			"timeline": map[string]interface{}{"id": e.PageID, "title": titles[e.PageID]}, "event_id": e.ID,
			"year": e.Year, "month": e.Month, "day": e.Day, "calendar": cal,
		})
	}
	return out, nil
}
