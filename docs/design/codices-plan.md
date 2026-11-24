# Codices: restructure plan

**Status:** agreed on 2026-11-20 and built in PRs 60–66 (live as v0.14.0 on 2026-11-21). It replaced the Codex as merged in PR 58 (`8eea696`). Where this plan and the code disagree, the code and CLAUDE.md win.

- **Design source:** layout **D Vault + Waystone** in `docs/design/mockups/codex-wiki.html`. A, B and C stay in the file only for comparison.
- **Code references** are from `main` at `8eea696` and will drift.

## Structure

- **Codices** is the module (was Codex).
- A **codex** (replaces a world) holds folders and pages.
- A **folder** holds folders and pages. Folders nest, as today (up to 10 levels).
- A **page** holds text and images and links to other pages with `[[Title]]`. Every page belongs to a codex.
- There is no Unsorted bucket and no map (see "Maps" below).

## Decisions

| Topic | Decision |
|---|---|
| Name | **Codices** in all user-facing text, the style guide and docs |
| URLs | Pages at `/codices/...`, API at `/api/codices/...`. Old `/codex/...` page URLs redirect permanently (301) to the new ones |
| Code names | `world` becomes `codex` in Go, JS and the schema (`worlds` → `codices`, `world_id` → `codex_id`), renamed in place with `ALTER TABLE` |
| Migration | None. There are no worlds in production. Update the demo seed (`config/codex_seed.go`, `db/seed_codex.go`): its Unsorted page goes into the codex |
| Links | `[[links]]` reach pages in the same codex only. Titles (and aliases) are unique within a codex. A page can still move to another codex; its links then show as missing |
| Page content | Free text (Markdown) plus tags. No page types or structured fields |
| Templates | Built-in templates plus each codex's own, picked in the New page dialog. See "Templates and copying" below |
| Copying pages | **Copy page…** in the More menu, into the same or another codex. See "Templates and copying" below |
| Tags | Belong to a codex. Anyone with Edit on the codex creates, renames and deletes them. A page has any number. Only labels: no colour, no hierarchy. Clicking a tag lists its pages |
| Images | Uploaded into a page (PNG, JPEG, WebP, GIF, up to 25 MB), written as `![[file|caption]]`. One image can be the page's cover. An image belongs to one page and follows that page's rights; its history is kept with the page. Lightbox on click |
| Search, recent changes, wanted pages, backlinks, link previews | Kept, and limited to codices the viewer can see |
| Graph | A full graph view per codex and a local graph in the page's side panel. See "Graph view" below |
| Games | A DM picks at most one codex when creating or editing a game. Several games can use the same codex. The game page shows a label naming it: a link if the viewer can see that codex, plain text otherwise. The codex never lists its games, and the link grants no access |
| Characters | Optional **Codex page** URL field, set by the owning player. When set, the character page shows a button that opens it |
| Character URL rule | Only `https://waystone.aravill.org/codices/...`. Also accept the server's own host so local dev and E2E work. Anything else is refused with a message |

## Access rights

A new admin page, **Access rights**, with matrix tables. More actions will be added later.

| Action | Scope | Meaning |
|---|---|---|
| See Codices | Per user | The module shows on Home and in the nav |
| Edit Codices | Per user | Create, rename and delete codices |
| See this codex | Per user, per codex | Read the codex, its pages and history |
| Edit this codex | Per user, per codex | See it, plus create, edit, move, restore and delete pages, folders, tags and images |

- **Layout:** one table of users × the two module rights, and one grid of users × codices where each cell is **None / See / Edit**.
- **Admins** always have every right and aren't listed.
- **New users** get nothing.
- **Creating a codex** gives its creator Edit on it.
- **Without See Codices** a user sees no codex at all, whatever their per-codex cells say: the pages redirect to Home and the API returns 404, as `codexGate` does today.
- **The game form's codex picker** lists only codices the DM can see.
- **`CODEX_ADMIN_ONLY`** is removed, from the code, `.env.example` and `docker-compose.yml`. The matrix replaces it.

## Templates and copying

The goal is the least possible friction when making a page. Shown in the mock-up: **+ New page**, any missing link, **Templates** in the left nav, and **Copy page… / Save as template…** under More.

**New page dialog.** Title, folder (defaults to the folder you're in) and **Start from**: a grid of templates in two groups, Built-in and This codex, with a live preview of the text the page will start with and the tags it adds. **Create page** opens the editor with that text. It's the same dialog everywhere a page gets made: the nav button, clicking a missing link or a wanted page (title filled in), and **Create page** on a missing page in the graph. Enter in the title field creates the page. The dialog remembers the last template you used.

**Built-in templates** (fixed, in Go, the same in every codex):

| Template | Adds tag | Sections |
|---|---|---|
| Blank page | none | none |
| Session | `session` | Date, who was there, Summary, What happened, People met, Loot and rewards, Loose ends, Next time |
| Character | `npc` | Lives in, belongs to, Appearance, Personality, Background, Relationships, Notes |
| Settlement | `settlement` | Population, ruled by, region, Places of note, People of note, History, Rumours |
| Landmark | `landmark` | Near, Description, History, Dangers, Rumours |
| Event | `event` | When, where, who, What happened, Why it matters, What came of it |
| Faction | `faction` | Led by, based in, Goals, Members, Allies and enemies, Resources |
| Item | `item` | Held by, Appearance, What it does, History, Where it is now |
| Lore | `lore` | The story, What people believe, Sources |

The exact texts are in the mock-up's `TEMPLATES`. Each starts with a one-line prompt in italics and leaves empty `[[ ]]` links where other pages belong.

**Codex templates.** Belong to one codex, like tags. Anyone with **Edit** on the codex can create, edit, copy and delete them (in practice the DM). A template has a name (unique in the codex), a one-line description, tags it adds, and Markdown text. The **Templates** list shows the built-in ones (Use, Copy) and the codex's own (Use, Copy, Edit, Delete, and who made it). Copying a built-in one is how you change it. Changing or deleting a template never changes pages already made from it. No revision history for templates.

**Placeholders:** `{{title}}` becomes the page's title and `{{date}}` the day it's made. Nothing else.

**Save as template…** (More menu, needs Edit): name, description, **Headings only** (the default: the page's `##`/`###` headings, empty) or **All the text**, and whether new pages get its tags. Images aren't kept.

**Copy page…** (More menu): needs See on this codex and Edit on the target. Title (defaults to "… (copy)"), codex (any you can edit), folder, copy tags (on), copy images (on, the files are duplicated). The copy is a new page with its own history, starting at revision 1 with the note "Copied from <title>". Aliases aren't copied (they must be unique). `[[Links]]` stay as written and resolve in the target codex. Copying a template to another codex isn't offered: copy a page made from it and save that as a template.

**Schema:** `codex_templates (id, codex_id, name, description, body, created_by, created_at, updated_at)` and `codex_template_tags (template_id, tag_id)`. Built-ins aren't stored.

## Graph view

Shown in the mock-up: the side panel's **Local graph**, and **Graph view** in the left nav.

- **Full graph** (`/codices/c/{id}/graph`, a tab like a page): every page in the codex as a node, every `[[link]]` as a line. Links count in both directions but are drawn without arrows.
- **Nodes:** coloured by top-level folder (Places, Factions, People, top level) in the style guide's inks (verdigris, ochre, rubric, muted). Size grows with the number of links. Pages not written yet are hollow and dashed. The page you came from has a ring.
- **Moving around:** drag to pan, wheel or pinch to zoom, + / − / Fit buttons, and drag a node to move it.
- **Hover** (mouse only) highlights a page and its neighbours and fades the rest. **Click** opens a card with the title, folder, tags, the first lines, link counts and **Open page** (or **Create page** for a missing one).
- **Filters** (folded on phones): find a page by title (Enter centres on it), highlight a tag, show or hide pages not written yet, show or hide pages with no links, always show titles or only on hover.
- **Local graph** in the side panel: the current page and its direct neighbours, with the links between them. Clicking it opens the full graph.
- **Data:** one endpoint, `GET /api/codices/c/{id}/graph` (and `?around={pageId}` for the local graph), returning nodes (id, title, top-level folder, folder path, tags, first line, missing) and links, from `page_links`. It follows the codex's See right.
- **Layout:** the mock-up hand-rolls a small force layout, which is enough for a codex of tens of pages. For the build, vendor `d3-force` through `tools/vendor` the way CodeMirror and Leaflet are, and keep drawing with plain SVG. Node positions aren't saved.

## Maps

*Back as a page type since 2026-11-24: see `codex-maps.md`. What follows is how they were hidden.*

Hidden, not deleted. The map UI (`static/codex-map.js`, the § Map section, Add/Replace/Remove map, pins, map history) is taken out of the pages. The map tables, upload files, API and backups stay, so maps can come back later.

## PRs

Seven PRs, in this order. Each one leaves the app working.

1. **`feat/codices`**: rename to Codices (text, URLs, redirects, code and schema names), worlds become codices, Unsorted removed, every page needs a codex, maps hidden, seed updated, game label instead of the world page's games list.
2. **`feat/codices access rights`**: the Access rights page, its two tables, enforcement in every Codices route and API, the creator rule, the filtered game picker, `CODEX_ADMIN_ONLY` removed.
3. **`feat/codices layout`**: the D layout from the mock-up: top bar with search, folder tree (a drawer on phones), page tabs, properties, side panel (Outline, Linked from, Wanted pages), one Edit button plus More menu, link previews.
4. **`feat/codices tags and images`**: tags, image upload, cover, lightbox.
5. **`feat/codices templates`**: the New page dialog with built-in and codex templates, the Templates list and editor, Save as template, Copy page. After tags, because templates add tags and copies keep them.
6. **`feat/codices graph`**: the graph endpoint, the full graph view and the side panel's local graph. After tags, so the tag filter works from the start.
7. **`feat/character codex link`**: the character URL field and button.

E2E tests: add them for access rights (a user without See can't reach a codex), for one image upload, for making a page from a template and copying it, and for the graph (it opens and lists the codex's pages). Everything else as the coding skill decides.
