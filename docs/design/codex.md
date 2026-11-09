# Codex (draft)

**Status:** draft from the design grilling on 2026-11-08 and 2026-11-09. Nothing is
built. The remaining open questions are at the end.

The Codex is a wiki that players mainly maintain. It holds lore about worlds and
characters, and the lore can span several games. Session notes (today's journal)
become part of it. It should feel like Obsidian: Markdown pages, easy `[[links]]`,
backlinks and a graph view.

## Approach

- **Not real Obsidian.** Obsidian is a closed-source desktop app and can't be
  embedded. We build the core in the app instead: Markdown pages, `[[links]]` with
  autocomplete, `#tags`, a backlinks panel and a graph view. The graph library and
  Leaflet are vendored under `static/vendor/`, the way Alpine.js is.
- **Editor: CodeMirror 6 with live preview.** It ships as many small modules, so it is
  bundled **once** into a single vendored file (about 300 KB, loaded only by the
  editor). The bundle script and pinned versions are checked in next to it so it can
  be rebuilt. The app itself still has no build step.
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

**Instead, reuse libraries:** a Markdown parser with wikilink support, CodeMirror 6,
Leaflet, a vendored graph library, and SQLite FTS5. Revisions already exist in
the journal. *Fallback:* if the integrated Codex is dropped, SilverBullet behind
Caddy is the closest match to Obsidian for a standalone lore space.

## Data model

Names are provisional.

**`worlds`**: `id`, `name` (unique, case-insensitive), `description` (Markdown),
`created_by`, `created_at`, `updated_at`.
- `campaigns.world_id` (empty = no world). A game has at most one world, and a world
  can have many games.

**`pages`**: `id`, `title`, `kind` (`lore` | `session` | `character` | `map`), `session_id`
(session pages only, unique), `character_id` (character pages only, unique),
`author_id`, `created_at`, `updated_at`, `deleted_at` (`''` = live; see Deleting).
- The body lives in revisions, reusing the journal's design: saves are refused when
  made on top of an outdated version (409), the newest 200 revisions per page are
  kept, and credit is kept in a contributors table so it survives pruning.

**`page_aliases`**: `page_id`, `alias`. Aliases resolve exactly like titles.

**`page_scopes`**: `page_id`, `world_id` or `campaign_id`. Many-to-many: a page
belongs to zero or more worlds and zero or more games.

**`page_links`**: `from_page_id`, `to_page_id` (empty for a red link), `target_text`.
Rebuilt from the body on every save. It drives backlinks, red links and the graph.

**`page_tags`**: `page_id`, `tag`. Rebuilt from the `#tags` in the body on every
save, like links. Page kinds are added as built-in tags (see Tags).

**`map_images`**: `id`, `page_id`, `file` (name under `UPLOAD_DIR`), `width`, `height`,
`uploaded_by`, `created_at`. The newest row is the current image; older rows are its
history.

**`map_pins`**: `id`, `map_page_id`, `x`, `y` (fractions 0–1 of the image's width and
height), `target_page_id` (empty for a red pin), `target_text`, `created_by`,
`created_at`, `updated_at`.

**`map_pin_events`**: who added, moved or removed which pin, with the old position or
target, so a change can be undone. Lighter than full revisions.

**FTS5 index** over titles, aliases, tags and bodies.

**`campaigns.world_id`**, plus a new notification type for session notes.

Foreign keys are off, as everywhere in this app, so cascades are written by hand.

## Rules

### Worlds
- Any logged-in user can create a world. It's only a container.
- Only a game's DM (or an admin) links the game to a world, from game edit.
- Only admins can delete a world, and not while any game is linked to it. Deleting it
  removes it from every page's scope. Pages left with no world and no game show
  under "Unsorted" in the Codex. (Stated decision; not asked.)
- **Re-linking a game** to another world, or unlinking it: session pages follow
  automatically, because they reach the world through the game. The confirmation also
  lists the other pages scoped to this game and the old world, with checkboxes to move
  them to the new world. Unticked pages keep their scope. Links are never rewritten,
  but they may resolve differently afterwards.

### Pages and scope
- Every page can belong to any number of worlds and games.
- **Session pages** are always scoped to their own game, which they can't leave, and
  reach the game's world through the game. If the game has no world, its session
  pages are still in the Codex, grouped under the game. Games are usually events or
  quests inside a world, so the chain is session → game → world.
- **Character pages** belong to no world by default and can be attached to several
  worlds.
- **Scope UI:** chips under the title ("Barovia · Curse of Strahd"), with a + picker
  (worlds and games, search as you type) and × to remove. A session page's game chip
  is locked. Scope changes are recorded in the page history.
- **New pages inherit scope:** a page created from a red link or red pin starts with
  the source page's scope. A page created from a world's home starts in that world.
  Either can be edited afterwards.

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
- The game page chronicle stays: it lists played sessions with attendees and their
  characters. "Read" or "Write notes" opens the session page in the Codex. Today's
  journal page is replaced by the Codex page view, so there's one editor and one
  history.
- **Notification:** when a session page first gets text, the game's DM, players and
  followers are notified, except the writer. New notification type. No other Codex
  notifications in v1.

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

### Tags
- Tags are written inline in the body, as in Obsidian: `#npc`, and nested
  `#faction/thieves`. They're parsed on save into `page_tags`, so tag changes show in
  the page history for free. The editor autocompletes `#` from tags already in use.
  Tags show as chips, and clicking one lists every page with that tag.
- Every page also carries a built-in tag for its kind (`#lore`, `#session`,
  `#character`, `#map`). It's added automatically and can't be removed.
- Tags are global: `#npc` means the same in every world. Nobody manages them. A tag
  exists while any page uses it, and renaming a tag means editing the pages.
  *Later:* a tag rename tool.

### Renaming
- Renaming a page rewrites every link to it, in one transaction.
- Each rewritten page gets a revision ("Renamed Ireena → Ireena Kolyana") credited to
  the person who renamed it. Session pages they couldn't edit are rewritten anyway, as
  a system edit.

### Deleting
- **Lore and map pages:** anyone logged in can delete them. Deletion is soft: the page
  goes to "Recently deleted" on the Codex page for 30 days, and anyone can restore it.
  Links and pins to it turn red, and come back on restore, because links store their
  target text. After 30 days a daily job (next to the backup job) purges the page with
  its revisions and map images.
- **Session pages:** only the game's DM and admins can delete them, as with the
  journal today, but the deletion is soft too (30 days to restore).
- **Deleting one by-hand session** soft-deletes its page, because the session never
  happened. The confirmation names the page.
- **Deleting a game** (admin) keeps its session pages and turns them into plain lore
  pages: the title is frozen (e.g. "Curse of Strahd — Session 4", worked out at
  deletion time), the game chip is replaced by the game's world (if any),
  `session_id` is cleared, and anyone can edit them. History and credit stay. Lore
  and map pages just lose the game from their scope. The confirmation says "N session
  pages will be kept as lore pages".
- **Deleting a character** (still blocked while it's actively assigned) soft-deletes
  its page like any other page. **Anonymizing the owner** keeps the character and its
  page, with the owner shown as "Deleted user".

### Maps
- A map is a page of kind `map`: an uploaded image on top of an ordinary page. It has
  everything other pages have (title, aliases, body, worlds and games, links,
  backlinks, a graph node, search).
- Shown with Leaflet (vendored), using its plain-image mode (`CRS.Simple`): pan and
  zoom over one image, no tiles.
- **Upload:** PNG, JPEG or WebP, up to 25 MB, stored as files under `UPLOAD_DIR` on
  the data volume (not in SQLite) and served with caching. New env var:
  `UPLOAD_DIR` goes in `.env.example` and `docker-compose.yml`. Backups
  (`db/backup.go`) must copy the folder too.
- **Who:** anyone logged in uploads maps, replaces images and adds, moves or removes
  pins. Each pin change is logged on the map's history and can be undone.
- **Replacing the image:** pins keep their fractional positions, so a higher-resolution
  version of the same map keeps them in place. If the aspect ratio changes, a warning
  ("pins may land in the wrong place") comes before saving. Old images stay in the
  history and can be restored.
- **Pins:** a pin points to an existing page (picked with autocomplete) or to a new
  title, which makes a red pin. Clicking a red pin creates the page, starting with the
  map's worlds and games. Pinning an existing page never changes its scope. One page
  can be pinned on several maps, and several times on one map. Pins count as links:
  they appear in backlinks and as map → page edges in the graph. Deleting the target
  page turns its pin red.
- **Drill-down:** a pin pointing to another map page opens that map (world → region →
  city).
- **Look:** each page kind has its own glyph in the Scriptorium style. The viewer's
  own characters get the rubric-red accent. Labels show on hover or tap. No custom pin
  categories in v1.
- **Side panel:** clicking a pin opens the page read-only in a side panel, beside the
  map. `[[Links]]` in it open in the same panel, and an "Open page" link goes to the
  editor. In reverse, a pinned page lists "On maps: Barovia", and clicking that opens
  the map centred on the pin.

*Later:* tiling very large maps, pin categories, areas (polygons), a marker for the
party's position, DM-only pins (blocked by the "no secrets in v1" decision).

### Graph
- A world's graph tab shows its pages as nodes and links and pins as edges. Worlds and
  games aren't nodes. Nodes grow with their backlink count, use the kind glyph, and the
  viewer's own characters are in rubric red. Clicking a node opens the page in the
  same side panel the maps use.
- **Tag filter:** a checkbox list of the tags used in the world. `#session` is off by
  default. The choice is remembered per browser (`localStorage`). A page shows when
  its kind tag is checked and either it has no other tags or one of them is checked.
  The kind tag works as a gate, so a session page tagged `#ambush` stays hidden while
  `#session` is off. (Stated decision; not asked.)
- No local graph on each page in v1.

## Pages and URLs

- A **Codex** nav item replaces the Home placeholder ("Codex: World database").
- `/codex`: search on top, then worlds, games with no world, characters, "Unsorted"
  pages, and "Recently deleted".
- `/codex/w/<world>`: the world's home, with its description, maps, recent changes,
  all its pages, and the graph tab.
- `/codex/p/<id>/<slug>`: a page. The URL keys on the id, so it survives renames.
- The game page gets a "Codex" link that opens `/codex` filtered to that game.

## Migration

Runs once, on the first start of the new version, in one transaction. The startup
backup runs before it.
- Each journal entry becomes a session page, with all its revisions, contributors and
  credit.
- Each character gets a page. The body is its description followed by its background
  (under `## Background`), and the first revision is credited to the owner. Owner,
  status, system and assignments stay on the character.
- The old journal tables stay, read-only, for one release in case something went
  wrong, and are dropped in the release after.

## v1 scope

Everything above ships in one release:
- Pages, `[[links]]` with autocomplete, red links, backlinks, aliases, `#tags`,
  revisions, soft delete.
- Worlds, scope chips, the Codex pages above.
- Graph view with the tag filter.
- Full-text search (FTS5), filtered by world.
- Maps with pins and the side panel.
- Session notes moved into the Codex, the session-notes notification, and the
  migration.
- **Seed data** (`SEED_DEMO_DATA=1`): a demo world linked to a demo game, about a
  dozen interlinked lore pages with tags, an alias, a red link, character pages, and
  a small map with pins (a demo map image checked in with the seed assets).
- **E2E:** no new Codex tests were asked for, but the existing journal E2E tests have
  to be ported to session pages, because the `playwright` check must pass.

*Later:* exporting a world as an Obsidian vault (a .zip of `.md` files with links
intact). Character sheets. DM-only pages or secret blocks.

## Open questions

1. **Editing in the side panel:** stays read-only for now. Revisit after using it.
2. **Character page edits:** we decided not to notify the owner. Revisit if people
   miss edits to their characters.
3. **Size of v1:** with maps, tags and the migration it's one large release. It could
   still be split into several PRs into a `feat/codex` integration branch.
