package api

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
	"waystone-web/db"
)

const graphExcerptChars = 200

var (
	graphImageRe = regexp.MustCompile(`!\[\[[^\[\]\n]*\]\]`)
	graphLinkRe  = regexp.MustCompile(`\[\[([^\[\]\n|]{1,200})(?:\|([^\[\]\n]{1,200}))?\]\]`)
	graphMdLink  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	graphListRe  = regexp.MustCompile(`^([-*+]|\d+\.|>)\s+`)
	graphMarkup  = strings.NewReplacer("*", "", "_", "", "`", "")
)

// graphExcerpt is the first lines of a page as plain text (mirrors codexPlainExcerpt in codex.js):
// headings dropped, links shown as their label, images and Markdown marks removed.
func graphExcerpt(body string) string {
	var lines []string
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		lines = append(lines, graphListRe.ReplaceAllString(l, ""))
	}
	text := strings.Join(lines, " ")
	text = graphImageRe.ReplaceAllString(text, "")
	text = graphLinkRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := graphLinkRe.FindStringSubmatch(m)
		if sub[2] != "" {
			return sub[2]
		}
		return sub[1]
	})
	text = graphMdLink.ReplaceAllString(text, "$1")
	text = strings.Join(strings.Fields(graphMarkup.Replace(text)), " ")
	if r := []rune(text); len(r) > graphExcerptChars {
		return strings.TrimSpace(string(r[:graphExcerptChars])) + "…"
	}
	return text
}

// handleCodexGraph serves GET /api/codices/c/{id}/graph: every page of the codex as a node and
// every stored [[link]] as a link, plus a node for each title that is linked but has no page
// (missing: true, id "missing:<normalized text>"). A↔B stay two links; the client draws one line.
// Map pins are not included.
func handleCodexGraph(w http.ResponseWriter, r *http.Request, codexID string) {
	_, rt, ok := codexRightsFor(w, r)
	if !ok || !rt.checkCodex(w, codexID, false) {
		return
	}
	codex, err := db.GetCodex(codexID)
	if err != nil {
		codexServerError(w, "retrieve codex")
		return
	}
	if codex == nil {
		writeJSONError(w, http.StatusNotFound, db.ErrCodexNotFound.Error())
		return
	}
	pages, links, err := db.GetCodexGraph(codexID)
	if err != nil {
		codexServerError(w, "retrieve graph")
		return
	}
	folders, err := db.GetFolders(codexID)
	if err != nil {
		codexServerError(w, "retrieve folders")
		return
	}
	type folderRef struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	byID := map[string]int{}
	for i, f := range folders {
		byID[f.ID] = i
	}
	// path returns the folders from the top level down to the one holding a page.
	path := func(folderID string) []folderRef {
		out := []folderRef{}
		for i, ok := byID[folderID]; ok && len(out) <= len(folders); i, ok = byID[folders[i].ParentID] {
			out = append([]folderRef{{folders[i].ID, folders[i].Name}}, out...)
		}
		return out
	}
	// Top-level folders in name order, so the client can give each a stable colour.
	roots := []folderRef{}
	for _, f := range folders {
		if f.ParentID == "" {
			roots = append(roots, folderRef{f.ID, f.Name})
		}
	}
	sort.Slice(roots, func(i, j int) bool { return db.NormName(roots[i].Name) < db.NormName(roots[j].Name) })

	known := map[string]bool{}
	nodes := make([]map[string]interface{}, 0, len(pages))
	for _, p := range pages {
		known[p.ID] = true
		fp := path(p.FolderID)
		var top interface{}
		if len(fp) > 0 {
			top = fp[0]
		}
		nodes = append(nodes, map[string]interface{}{
			"id": p.ID, "title": p.Title, "folder_id": p.FolderID, "top_folder": top, "folder_path": fp,
			"tags": p.Tags, "excerpt": graphExcerpt(p.Body), "missing": false,
		})
	}
	out := make([]map[string]string, 0, len(links))
	missing := map[string]bool{}
	for _, l := range links {
		if !known[l.FromPageID] {
			continue
		}
		target := l.ToPageID
		if target == "" || !known[target] {
			target = "missing:" + db.NormName(l.TargetText)
			if !missing[target] {
				missing[target] = true
				nodes = append(nodes, map[string]interface{}{
					"id": target, "title": l.TargetText, "folder_id": "", "top_folder": nil, "folder_path": []folderRef{},
					"tags": []string{}, "excerpt": "", "missing": true,
				})
			}
		} else if target == l.FromPageID {
			continue
		}
		out = append(out, map[string]string{"source": l.FromPageID, "target": target})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"nodes": nodes, "links": out, "top_folders": roots})
}
