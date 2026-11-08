# Codex (draft)

**Status:** draft from the first design grilling on 2026-11-08. Nothing is built.
The open questions at the end come next, before any code.

The Codex is a wiki that players mainly maintain. It holds lore about worlds and
characters, and the lore can span several games. Session notes (today's journal)
become part of it. It should feel like Obsidian: Markdown pages, easy `[[links]]`,
backlinks and a graph view.

## Approach

- **Not real Obsidian.** Obsidian is a closed-source desktop app and can't be
  embedded. We build the core in the app instead: Markdown pages, `[[links]]` with
  autocomplete, a backlinks panel and a graph view. The graph library is vendored
  under `static/vendor/` (no build step), the way Alpine.js is.
- **Database: stay on SQLite.** A table of page-to-page links gives backlinks and the
  graph, and FTS5 (supported by `modernc.org/sqlite`) gives full-text search. With one
  app process and a small group there's no case for Postgres or a graph database.

### Alternatives considered: hosting ready-made wiki software

Checked on 2026-11-08. The candidates were SilverBullet, LeafWiki, Otter Wiki,
Wiki.js 3, Outline, BookStack, Kanka and MediaWiki. **Rejected**, because the Codex
decisions depend on the app's own data, and an external wiki can't see it:
- Session pages follow the journal rules: played sessions only, editable by the game's
  DM and current players, frozen when the session is cancelled.
- Character pages carry metadata only the owner controls, and their game assignments.
- Link resolution depends on games and worlds.
- Login: the app has its own local accounts. Outline needs an external OIDC login
  provider. The others keep their own user lists or only support simple single-space
  logins, so the app would have to become an OIDC provider or hand logins through
  Caddy (`forward_auth`).
- Weight: Wiki.js 3 needs PostgreSQL 17, and Outline needs Postgres, Redis and S3
  storage.

**Instead, reuse libraries:** a Markdown parser with wikilink support, CodeMirror 6
(if chosen), a vendored graph library, and SQLite FTS5. Revisions already exist in
the journal. *Fallback:* if the integrated Codex is dropped, SilverBullet behind
Caddy is the closest match to Obsidian for a standalone lore space.

## Data model

Names are provisional.

**`worlds`**: `id`, `name` (unique, case-insensitive), `description` (Markdown),
`created_by`, `created_at`, `updated_at`.
- `campaigns.world_id` (empty = no world). A game has at most one world, and a world
  can have many games.

**`pages`**: `id`, `title`, `kind` (`lore` | `session` | `character`), `session_id`
(session pages only, unique), `character_id` (character pages only, unique),
`author_id`, `created_at`, `updated_at`.
- The body lives in revisions, reusing the journal's design: saves are refused when
  made on top of an outdated version (409), the newest 200 revisions per page are
  kept, and credit is kept in a contributors table so it survives pruning.

**`page_aliases`**: `page_id`, `alias`. Aliases resolve exactly like titles.

**`page_scopes`**: `page_id`, `world_id` or `campaign_id`. Many-to-many: a page
belongs to zero or more worlds and zero or more games.

**`page_links`**: `from_page_id`, `to_page_id` (empty for a red link), `target_text`.
Rebuilt from the body on every save. It drives backlinks, red links and the graph.

**FTS5 index** over titles, aliases and bodies.

Foreign keys are off, as everywhere in this app, so cascades are written by hand.

## Rules

### Worlds
- Any logged-in user can create a world. It's only a container.
- Only a game's DM (or an admin) links the game to a world, from game edit.
- Only admins can delete a world, and not while any game is linked to it.

### Pages and scope
- Every page can belong to any number of worlds and games.
- **Session pages** are always scoped to their own game, which they can't leave, and
  reach the game's world through the game. If the game has no world, its session
  pages are still in the Codex, grouped under the game. Games are usually events or
  quests inside a world, so the chain is session → game → world.
- **Character pages** belong to no world by default and can be attached to several
  worlds.

### Characters
- A character *is* a Codex page. Today's description and background become the page
  body, which anyone can edit like any other page.
- The owner, status and game assignments stay as metadata only the owner controls,
  with the current rules in `docs/design/characters.md`.
- `system_id` stays for now, because picking a character for a game still matches on
  system. *Later:* character sheets carry the system, and a character can have sheets
  for several systems.

### Session notes
- Today's journal entries become session pages, still keyed on the session id, never
  on a session number.
- The journal rules carry over: only played sessions get a page, and a cancelled
  session's page stays readable and deletable but not editable.

### Editing
- Anyone logged in reads everything and edits any lore or character page.
- Session pages keep the journal rule: only the game's DM and current players edit
  them, including players who missed the session.
- No DM secrets in v1: no hidden pages or blocks. Secrets would mean filtering search,
  graph, backlinks and autocomplete per viewer.

### Links
- Obsidian syntax: `[[Title]]` and `[[Title|label]]`. Aliases resolve like titles.
- A link to a missing page shows in red, and clicking it creates the page.
- Resolution goes from the nearest scope outward: same game, then same world, then
  pages attached to that world (characters included), then pages in no world.
- A title or alias must be unique within each scope. When two pages still tie for a
  link, it shows as ambiguous and opens a picker. Choosing one stores a stable form
  (e.g. `[[Kael#<id>]]`).

### Renaming
- Renaming a page rewrites every link to it, in one transaction.
- Each rewritten page gets a revision ("Renamed Ireena → Ireena Kolyana") credited to
  the person who renamed it. Session pages they couldn't edit are rewritten anyway, as
  a system edit.

## v1 scope

- Pages, `[[links]]` with autocomplete, red links, backlinks, aliases, revisions.
- Graph view, filtered by world or game. The viewer's own characters are highlighted
  in rubric red, the accent that means "you".
- Full-text search (FTS5), filtered by world.

*Later:* exporting a world as an Obsidian vault (a .zip of `.md` files with links
intact). Character sheets. DM-only pages or secret blocks.

## Open questions

1. **Placement and URLs:** a Codex nav page (the Home page already has a "Codex:
   World database" placeholder), world pages, a Codex tab on the game page, and how
   the game page chronicle relates to session pages.
2. **Editor:** a plain textarea with preview (like today's journal), or CodeMirror 6
   with live preview, which feels like Obsidian but is a bigger vendored library.
3. **Lifecycle:** who can delete a page, and what happens to the links pointing at it.
   What happens to pages when their game or world is deleted, or when a game is
   re-linked to another world. What happens to a character page when the character is
   deleted, or when its owner's account is anonymized.
4. **Scoping UI:** how a page gets attached to worlds and games, and whether a page
   created from a red link takes the scope of the page it was clicked on.
5. **Graph:** what the nodes are (pages only, or worlds and games too), and whether
   session pages are hidden by default so they don't swamp the graph.
6. **Notifications:** e.g. "someone edited your character's page", or a new session
   page in a game you play in or follow.
7. **Migration:** turning journal entries and characters into pages while keeping
   revisions, credit and character assignments.
