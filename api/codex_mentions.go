package api

import (
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

const (
	maxMentionPages    = 50
	maxMentionSnippets = 2
	mentionContext     = 60
)

var (
	snippetLinkRe   = regexp.MustCompile(`\[\[(?:[^\[\]\n|]*\|)?([^\[\]\n]*)\]\]`)
	snippetMarkerRe = regexp.MustCompile(`[#*>~]+|` + "`" + `|\[\[|\]\]|!\[|\]\([^)]*\)?`)
)

// mentionPlain turns a bit of markdown into plain text for a snippet.
func mentionPlain(s string) string {
	s = snippetLinkRe.ReplaceAllString(s, "$1")
	s = snippetMarkerRe.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}

func lastRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[len(r)-n:])
}

func firstRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// snippetBefore is the plain text before a mention, cut at a word start with "…" when shortened.
func snippetBefore(s string) string {
	cut := lastRunes(s, mentionContext)
	if len(cut) < len(s) {
		if i := strings.IndexAny(cut, " \t\n"); i >= 0 {
			cut = cut[i:]
		}
		return "…" + mentionPlain(cut)
	}
	return mentionPlain(cut)
}

// snippetAfter is the plain text after a mention, cut at a word end with "…" when shortened.
func snippetAfter(s string) string {
	cut := firstRunes(s, mentionContext)
	if len(cut) < len(s) {
		if i := strings.LastIndexAny(cut, " \t\n"); i >= 0 {
			cut = cut[:i]
		}
		return mentionPlain(cut) + "…"
	}
	return mentionPlain(cut)
}

// mentionNamesOf is the title and aliases of a page.
func mentionNamesOf(p *models.Page) []string {
	return append([]string{p.Title}, p.Aliases...)
}

// handleGetMentions lists the pages of the codex that name this page in plain text without linking to it.
func handleGetMentions(w http.ResponseWriter, r *http.Request, id string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, false) {
		return
	}
	page, err := db.GetPage(id)
	if err != nil {
		codexServerError(w, "retrieve page")
		return
	}
	if page == nil {
		codexStoreError(w, db.ErrPageNotFound, "")
		return
	}
	backs, err := db.GetBacklinks(id)
	if err != nil {
		codexServerError(w, "retrieve backlinks")
		return
	}
	skip := map[string]bool{id: true}
	for _, b := range backs {
		skip[b.PageID] = true
	}
	bodies, err := db.GetCodexBodies(page.CodexID)
	if err != nil {
		codexServerError(w, "retrieve pages")
		return
	}
	names := mentionNamesOf(page)
	list := []map[string]interface{}{}
	total := 0
	for _, b := range bodies {
		if skip[b.ID] {
			continue
		}
		ms := db.FindMentions(b.Body, names)
		if len(ms) == 0 {
			continue
		}
		total++
		if len(list) >= maxMentionPages {
			continue
		}
		snippets := []map[string]string{}
		for i, m := range ms {
			if i >= maxMentionSnippets {
				break
			}
			snippets = append(snippets, map[string]string{
				"before": snippetBefore(b.Body[:m.Start]),
				"match":  m.Text,
				"after":  snippetAfter(b.Body[m.End:]),
			})
		}
		list = append(list, map[string]interface{}{
			"page":             map[string]string{"id": b.ID, "title": b.Title},
			"current_revision": b.CurrentRevision,
			"count":            len(ms),
			"snippets":         snippets,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"total": total, "mentions": list, "can_edit": rt.canEdit(page.CodexID)})
}

// handleLinkMentions links every plain mention of this page's names in another page of the codex.
func handleLinkMentions(w http.ResponseWriter, r *http.Request, id string) {
	userID, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkPage(w, id, true) {
		return
	}
	var req struct {
		FromPageID   string `json:"from_page_id"`
		BaseRevision int    `json:"base_revision"`
	}
	if !codexDecode(w, r, 4<<10, &req) {
		return
	}
	if req.BaseRevision < 1 || req.FromPageID == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	page, err := db.GetPage(id)
	if err != nil {
		codexServerError(w, "retrieve page")
		return
	}
	from, err := db.GetPage(req.FromPageID)
	if err != nil {
		codexServerError(w, "retrieve page")
		return
	}
	if page == nil {
		codexStoreError(w, db.ErrPageNotFound, "")
		return
	}
	if from == nil || from.CodexID != page.CodexID || from.ID == page.ID {
		writeJSONError(w, http.StatusBadRequest, "the page is not in this codex")
		return
	}
	body, n := db.LinkMentions(from.Body, page.Title, mentionNamesOf(page))
	if n == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"linked": 0})
		return
	}
	saveCodexRevision(w, from.ID, req.BaseRevision, models.PageRevision{AuthorID: userID, Body: body, Note: "Linked mentions of " + page.Title})
}
