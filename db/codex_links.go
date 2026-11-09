package db

import (
	"database/sql"
	"regexp"
	"strings"
)

// wikiLinkRe matches [[Title]] and [[Title|label]]. Mirrored in static/codex.js.
var wikiLinkRe = regexp.MustCompile(`\[\[([^\[\]\n|]{1,200})(?:\|([^\[\]\n]{1,200}))?\]\]`)

// ParsedLink is one [[...]] in a body. Start/End cover the whole match, TextStart/TextEnd just the title part.
type ParsedLink struct {
	Text, Label        string
	Start, End         int
	TextStart, TextEnd int
}

func ParseWikiLinks(body string) []ParsedLink {
	var out []ParsedLink
	for _, m := range wikiLinkRe.FindAllStringSubmatchIndex(body, -1) {
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

// codexIndex maps world -> normalized title or alias -> page id. Names are unique per world.
type codexIndex struct {
	byWorld map[string]map[string]string
}

// worldFilter returns " WHERE col IN (?, ?)" and its args for the given worlds, or "" for all.
func worldFilter(col string, worlds []string) (string, []interface{}) {
	if len(worlds) == 0 {
		return "", nil
	}
	args := make([]interface{}, len(worlds))
	for i, w := range worlds {
		args[i] = w
	}
	return " WHERE " + col + " IN (?" + strings.Repeat(", ?", len(worlds)-1) + ")", args
}

// loadCodexIndex indexes the pages of the given worlds (every world when none is given).
func loadCodexIndex(q querier, worlds ...string) (*codexIndex, error) {
	idx := &codexIndex{byWorld: map[string]map[string]string{}}
	add := func(world, name, id string) {
		m := idx.byWorld[world]
		if m == nil {
			m = map[string]string{}
			idx.byWorld[world] = m
		}
		if _, taken := m[NormName(name)]; !taken {
			m[NormName(name)] = id
		}
	}
	where, args := worldFilter("p.world_id", worlds)
	// Titles first so a title wins over an alias if legacy data ever collides.
	for _, query := range []string{
		`SELECT p.id, p.world_id, p.title FROM pages p` + where + ` ORDER BY p.created_at, p.id`,
		`SELECT p.id, p.world_id, a.alias FROM page_aliases a JOIN pages p ON p.id = a.page_id` + where + ` ORDER BY p.created_at, p.id, a.alias`,
	} {
		rows, err := q.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, world, name string
			if err := rows.Scan(&id, &world, &name); err != nil {
				rows.Close()
				return nil, err
			}
			add(world, name, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return idx, nil
}

// resolve returns the page a [[text]] in the given world points at, or "" when there is none.
func (idx *codexIndex) resolve(world, text string) string {
	return idx.byWorld[world][NormName(text)]
}

// nameConflicts lists pages in the world (other than pageID) whose title or alias equals one of names.
func nameConflicts(q querier, world, pageID string, names []string) ([]NameConflict, error) {
	taken := map[string]NameConflict{}
	for _, query := range []string{
		`SELECT id, title, title FROM pages WHERE world_id = ? AND id <> ?`,
		`SELECT p.id, p.title, a.alias FROM page_aliases a JOIN pages p ON p.id = a.page_id WHERE p.world_id = ? AND p.id <> ?`,
	} {
		rows, err := q.Query(query, world, pageID)
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
func rebuildPageLinks(tx *sql.Tx, idx *codexIndex, pageID, world, body string) error {
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
			pageID, l.Text, idx.resolve(world, l.Text)); err != nil {
			return err
		}
	}
	return nil
}

// resolveWorldLinks re-resolves the stored links and pins of the given worlds (pages and maps
// that live there) against the current titles and aliases. Call it at the end of every
// transaction that changes what a name in a world can point at: CreatePage, RenamePage,
// SetPageAliases, DeletePage (that page's world) and MovePage (the old and the new world).
// Body saves only rebuild that page's rows; world deletes need none because a world with pages
// can't be deleted; game-to-world labels never affect links.
func resolveWorldLinks(tx *sql.Tx, worlds ...string) error {
	idx, err := loadCodexIndex(tx, worlds...)
	if err != nil {
		return err
	}
	return resolveLinksWith(tx, idx, worlds...)
}

func resolveLinksWith(tx *sql.Tx, idx *codexIndex, worlds ...string) error {
	where, args := worldFilter("p.world_id", worlds)
	type row struct{ a, b, to, world string }
	for _, t := range []struct{ sel, upd string }{
		{`SELECT l.from_page_id, l.target_text, l.to_page_id, p.world_id FROM page_links l JOIN pages p ON p.id = l.from_page_id` + where,
			`UPDATE page_links SET to_page_id = ? WHERE from_page_id = ? AND target_text = ?`},
		{`SELECT m.id, m.target_text, m.to_page_id, p.world_id FROM map_pins m JOIN pages p ON p.id = m.map_page_id` + where,
			`UPDATE map_pins SET to_page_id = ? WHERE id = ? AND target_text = ?`},
	} {
		rows, err := tx.Query(t.sel, args...)
		if err != nil {
			return err
		}
		var changes []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.a, &r.b, &r.to, &r.world); err != nil {
				rows.Close()
				return err
			}
			if now := idx.resolve(r.world, r.b); now != r.to {
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
