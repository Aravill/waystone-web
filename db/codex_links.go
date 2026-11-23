package db

import (
	"database/sql"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// wikiLinkRe matches [[Title]] and [[Title|label]]. Mirrored in static/codex.js.
var wikiLinkRe = regexp.MustCompile(`\[\[([^\[\]\n|]{1,200})(?:\|([^\[\]\n]{1,200}))?\]\]`)

// ParsedLink is one [[...]] in a body. Start/End cover the whole match, TextStart/TextEnd just the title part.
type ParsedLink struct {
	Text, Label        string
	Start, End         int
	TextStart, TextEnd int
}

// ParseWikiLinks lists the [[links]] in a body; ![[...]] image embeds are not links.
func ParseWikiLinks(body string) []ParsedLink {
	var out []ParsedLink
	for _, m := range wikiLinkRe.FindAllStringSubmatchIndex(body, -1) {
		if m[0] > 0 && body[m[0]-1] == '!' {
			continue // ![[picture|caption]] is an image, not a link
		}
		l := ParsedLink{Start: m[0], End: m[1], TextStart: m[2], TextEnd: m[3], Text: strings.TrimSpace(body[m[2]:m[3]])}
		if m[4] >= 0 {
			l.Label = body[m[4]:m[5]]
		}
		if l.Text != "" {
			out = append(out, l)
		}
	}
	return out
}

// NormName is how titles and aliases are compared: lower-case, whitespace collapsed, trimmed.
// SQLite's NOCASE is ASCII-only, so names are never compared in SQL.
func NormName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// querier is satisfied by *sql.DB and *sql.Tx.
type querier interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// codexIndex maps codex -> normalized title or alias -> page id. Names are unique per codex.
type codexIndex struct {
	byCodex map[string]map[string]string
}

// codexFilter returns " WHERE col IN (?, ?)" and its args for the given codices, or "" for all.
func codexFilter(col string, codices []string) (string, []interface{}) {
	if len(codices) == 0 {
		return "", nil
	}
	args := make([]interface{}, len(codices))
	for i, w := range codices {
		args[i] = w
	}
	return " WHERE " + col + " IN (?" + strings.Repeat(", ?", len(codices)-1) + ")", args
}

// loadCodexIndex indexes the pages of the given codices (every codex when none is given).
func loadCodexIndex(q querier, codices ...string) (*codexIndex, error) {
	idx := &codexIndex{byCodex: map[string]map[string]string{}}
	add := func(codex, name, id string) {
		m := idx.byCodex[codex]
		if m == nil {
			m = map[string]string{}
			idx.byCodex[codex] = m
		}
		if _, taken := m[NormName(name)]; !taken {
			m[NormName(name)] = id
		}
	}
	where, args := codexFilter("p.codex_id", codices)
	// Titles first so a title wins over an alias if legacy data ever collides.
	for _, query := range []string{
		`SELECT p.id, p.codex_id, p.title FROM pages p` + where + ` ORDER BY p.created_at, p.id`,
		`SELECT p.id, p.codex_id, a.alias FROM page_aliases a JOIN pages p ON p.id = a.page_id` + where + ` ORDER BY p.created_at, p.id, a.alias`,
	} {
		rows, err := q.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, codex, name string
			if err := rows.Scan(&id, &codex, &name); err != nil {
				rows.Close()
				return nil, err
			}
			add(codex, name, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return idx, nil
}

// resolve returns the page a [[text]] in the given codex points at, or "" when there is none.
func (idx *codexIndex) resolve(codex, text string) string {
	return idx.byCodex[codex][NormName(text)]
}

// nameConflicts lists pages in the codex (other than pageID) whose title or alias equals one of names.
func nameConflicts(q querier, codex, pageID string, names []string) ([]NameConflict, error) {
	taken := map[string]NameConflict{}
	for _, query := range []string{
		`SELECT id, title, title FROM pages WHERE codex_id = ? AND id <> ?`,
		`SELECT p.id, p.title, a.alias FROM page_aliases a JOIN pages p ON p.id = a.page_id WHERE p.codex_id = ? AND p.id <> ?`,
	} {
		rows, err := q.Query(query, codex, pageID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, title, name string
			if err := rows.Scan(&id, &title, &name); err != nil {
				rows.Close()
				return nil, err
			}
			taken[NormName(name)] = NameConflict{PageID: id, Title: title}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	var out []NameConflict
	seen := map[string]bool{}
	for _, n := range names {
		k := NormName(n)
		if c, ok := taken[k]; ok && !seen[k] {
			seen[k] = true
			c.Name = n
			out = append(out, c)
		}
	}
	return out, nil
}

// rebuildPageLinks replaces a page's link rows with the distinct [[targets]] in body.
func rebuildPageLinks(tx *sql.Tx, idx *codexIndex, pageID, codex, body string) error {
	if _, err := tx.Exec(`DELETE FROM page_links WHERE from_page_id = ?`, pageID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, l := range ParseWikiLinks(body) {
		k := NormName(l.Text)
		if seen[k] {
			continue
		}
		seen[k] = true
		if _, err := tx.Exec(`INSERT INTO page_links (from_page_id, target_text, to_page_id) VALUES (?, ?, ?)`,
			pageID, l.Text, idx.resolve(codex, l.Text)); err != nil {
			return err
		}
	}
	return nil
}

// resolveCodexLinks re-resolves the stored links and pins of the given codices (pages and maps
// that live there) against the current titles and aliases. Call it at the end of every
// transaction that changes what a name in a codex can point at: CreatePage, RenamePage,
// SetPageAliases, DeletePage (that page's codex) and MovePage (the old and the new codex).
// Body saves only rebuild that page's rows; codex deletes need none because a codex with pages
// can't be deleted; game-to-codex labels never affect links.
func resolveCodexLinks(tx *sql.Tx, codices ...string) error {
	idx, err := loadCodexIndex(tx, codices...)
	if err != nil {
		return err
	}
	return resolveLinksWith(tx, idx, codices...)
}

func resolveLinksWith(tx *sql.Tx, idx *codexIndex, codices ...string) error {
	where, args := codexFilter("p.codex_id", codices)
	type row struct{ a, b, to, codex string }
	for _, t := range []struct{ sel, upd string }{
		{`SELECT l.from_page_id, l.target_text, l.to_page_id, p.codex_id FROM page_links l JOIN pages p ON p.id = l.from_page_id` + where,
			`UPDATE page_links SET to_page_id = ? WHERE from_page_id = ? AND target_text = ?`},
		{`SELECT m.id, m.target_text, m.to_page_id, p.codex_id FROM map_pins m JOIN pages p ON p.id = m.map_page_id` + where,
			`UPDATE map_pins SET to_page_id = ? WHERE id = ? AND target_text = ?`},
	} {
		rows, err := tx.Query(t.sel, args...)
		if err != nil {
			return err
		}
		var changes []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.a, &r.b, &r.to, &r.codex); err != nil {
				rows.Close()
				return err
			}
			if now := idx.resolve(r.codex, r.b); now != r.to {
				r.to = now
				changes = append(changes, r)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, r := range changes {
			if _, err := tx.Exec(t.upd, r.to, r.a, r.b); err != nil {
				return err
			}
		}
	}
	return nil
}

// rewriteLinksTo replaces the title part of every [[old...]] in body with newTitle, keeping any |label.
func rewriteLinksTo(body, oldTitle, newTitle string) (string, int) {
	want := NormName(oldTitle)
	var b strings.Builder
	last, n := 0, 0
	for _, l := range ParseWikiLinks(body) {
		if NormName(l.Text) != want {
			continue
		}
		b.WriteString(body[last:l.TextStart])
		b.WriteString(newTitle)
		last = l.TextEnd
		n++
	}
	b.WriteString(body[last:])
	return b.String(), n
}

// Mention is one plain-text occurrence of a page name in a body (byte offsets, End exclusive).
type Mention struct {
	Start, End int
	Text       string
}

var (
	mentionCodeSpanRe = regexp.MustCompile("`[^`\n]+`")
	mentionMdLinkRe   = regexp.MustCompile(`\[[^\]\n]*\]\([^)\n]*\)`)
	mentionURLRe      = regexp.MustCompile(`https?://[^\s<>)\]]+`)
	mentionListRe     = regexp.MustCompile(`(?m)^[ \t]*\{\{list\b[^\n]*`)
)

// mentionMask marks the bytes of body that are never searched: fenced and inline code, [[links]],
// ![[embeds]], [text](url), bare URLs and {{list ...}} lines.
func mentionMask(body string) []bool {
	mask := make([]bool, len(body))
	set := func(a, b int) {
		for i := a; i < b; i++ {
			mask[i] = true
		}
	}
	pos, fenced := 0, false
	for pos < len(body) {
		end := strings.IndexByte(body[pos:], '\n')
		if end < 0 {
			end = len(body)
		} else {
			end += pos + 1
		}
		fence := strings.HasPrefix(strings.TrimLeft(body[pos:end], " \t"), "```")
		if fence || fenced {
			set(pos, end)
		}
		if fence {
			fenced = !fenced
		}
		pos = end
	}
	for _, re := range []*regexp.Regexp{wikiLinkRe, mentionCodeSpanRe, mentionMdLinkRe, mentionURLRe, mentionListRe} {
		for _, m := range re.FindAllStringIndex(body, -1) {
			set(m[0], m[1])
		}
	}
	return mask
}

func isMentionWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// mentionNames dedupes names by NormName, drops those under 3 runes (or that can't sit in a
// [[link]]) and returns them longest first.
func mentionNames(names []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range names {
		k := NormName(n)
		if seen[k] || utf8.RuneCountInString(k) < 3 || strings.ContainsAny(k, "[]|") {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// FindMentions lists the plain-text, case-insensitive whole-word occurrences of any of names in
// body, in order, longest name first and without overlaps. Any run of spaces or tabs inside a name
// matches any run of whitespace except newlines. Masked spans (see mentionMask) are skipped.
func FindMentions(body string, names []string) []Mention {
	names = mentionNames(names)
	if len(names) == 0 {
		return nil
	}
	pats := make([]string, len(names))
	anchored := make([]*regexp.Regexp, len(names))
	for i, n := range names {
		parts := strings.Fields(n)
		for j := range parts {
			parts[j] = regexp.QuoteMeta(parts[j])
		}
		pats[i] = strings.Join(parts, `[ \t]+`)
		anchored[i] = regexp.MustCompile(`(?i)^(?:` + pats[i] + `)`)
	}
	any := regexp.MustCompile(`(?i)(?:` + strings.Join(pats, "|") + `)`)
	mask := mentionMask(body)
	var out []Mention
	for pos := 0; pos < len(body); {
		loc := any.FindStringIndex(body[pos:])
		if loc == nil {
			break
		}
		start := pos + loc[0]
		found := false
		if !mask[start] {
			if before, _ := utf8.DecodeLastRuneInString(body[:start]); start == 0 || !isMentionWordRune(before) {
				for _, re := range anchored {
					l := re.FindStringIndex(body[start:])
					if l == nil {
						continue
					}
					end := start + l[1]
					after, _ := utf8.DecodeRuneInString(body[end:])
					masked := false
					for i := start; i < end; i++ {
						if mask[i] {
							masked = true
							break
						}
					}
					if masked || (end < len(body) && isMentionWordRune(after)) {
						continue
					}
					out = append(out, Mention{Start: start, End: end, Text: body[start:end]})
					pos, found = end, true
					break
				}
			}
		}
		if !found {
			_, size := utf8.DecodeRuneInString(body[start:])
			pos = start + size
		}
	}
	return out
}

// LinkMentions turns every mention of the names into a [[link]] to title: [[Title]] when written
// exactly as the title, otherwise [[Title|as written]]. It returns the new body and the count.
func LinkMentions(body, title string, names []string) (string, int) {
	ms := FindMentions(body, names)
	for i := len(ms) - 1; i >= 0; i-- {
		m := ms[i]
		link := "[[" + title + "]]"
		if m.Text != title {
			link = "[[" + title + "|" + m.Text + "]]"
		}
		body = body[:m.Start] + link + body[m.End:]
	}
	return body, len(ms)
}

var usernameRe = regexp.MustCompile(`@[A-Za-z0-9_-]{2,}`)

// LinkUsernames replaces each @username in body with links[username in lower case]; unknown names
// stay. The @ must follow a line start, whitespace or opening punctuation (so no emails), the name
// must end there, and masked spans (see mentionMask) are skipped.
func LinkUsernames(body string, links map[string]string) string {
	mask := mentionMask(body)
	var out strings.Builder
	last := 0
	for _, m := range usernameRe.FindAllStringIndex(body, -1) {
		start, end := m[0], m[1]
		if before, _ := utf8.DecodeLastRuneInString(body[:start]); start > 0 && !unicode.IsSpace(before) && !strings.ContainsRune("(\"'*_“‘—–>,;", before) {
			continue
		}
		// A trailing _ or - may belong to the text around the name (_@roy_), so it is tried without too.
		link := links[strings.ToLower(body[start+1:end])]
		for link == "" && (body[end-1] == '_' || body[end-1] == '-') {
			end--
			link = links[strings.ToLower(body[start+1:end])]
		}
		if link == "" || mask[start] || strings.HasPrefix(body[end:], "@") {
			continue
		}
		if after, _ := utf8.DecodeRuneInString(body[end:]); end < len(body) && after != '_' && isMentionWordRune(after) {
			continue
		}
		out.WriteString(body[last:start])
		out.WriteString(link)
		last = end
	}
	out.WriteString(body[last:])
	return out.String()
}
