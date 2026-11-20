# Codices: restructure plan

**Status:** agreed on 2026-11-20, not started. Replaces the Codex as merged in PR 58 (`8eea696`).

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
| Base template | One fixed starter text offered in the New page dialog: Overview, Description, History, Relations, Notes headings. Per-codex templates can come later |
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

## Graph view

Shown in the mock-up: the side panel's **Local graph**, and **Graph view** in the left nav.

- **Full graph** (`/codices/{id}/graph`, a tab like a page): every page in the codex as a node, every `[[link]]` as a line. Links count in both directions but are drawn without arrows.
- **Nodes:** coloured by top-level folder (Places, Factions, People, top level) in the style guide's inks (verdigris, ochre, rubric, muted). Size grows with the number of links. Pages not written yet are hollow and dashed. The page you came from has a ring.
- **Moving around:** drag to pan, wheel or pinch to zoom, + / − / Fit buttons, and drag a node to move it.
- **Hover** (mouse only) highlights a page and its neighbours and fades the rest. **Click** opens a card with the title, folder, tags, the first lines, link counts and **Open page** (or **Create page** for a missing one).
- **Filters** (folded on phones): find a page by title (Enter centres on it), highlight a tag, show or hide pages not written yet, show or hide pages with no links, always show titles or only on hover.
- **Local graph** in the side panel: the current page and its direct neighbours, with the links between them. Clicking it opens the full graph.
- **Data:** one endpoint, `GET /api/codices/{id}/graph`, returning nodes (id, title, top-level folder, folder path, tags, first line, missing) and links, from `page_links`. It follows the codex's See right.
- **Layout:** the mock-up hand-rolls a small force layout, which is enough for a codex of tens of pages. For the build, vendor `d3-force` through `tools/vendor` the way CodeMirror and Leaflet are, and keep drawing with plain SVG. Node positions aren't saved.

## Maps

Hidden, not deleted. The map UI (`static/codex-map.js`, the § Map section, Add/Replace/Remove map, pins, map history) is taken out of the pages. The map tables, upload files, API and backups stay, so maps can come back later.

## PRs

Six PRs, in this order. Each one leaves the app working.

1. **`feat/codices`**: rename to Codices (text, URLs, redirects, code and schema names), worlds become codices, Unsorted removed, every page needs a codex, maps hidden, seed updated, game label instead of the world page's games list.
2. **`feat/codices access rights`**: the Access rights page, its two tables, enforcement in every Codices route and API, the creator rule, the filtered game picker, `CODEX_ADMIN_ONLY` removed.
3. **`feat/codices layout`**: the D layout from the mock-up: top bar with search, folder tree (a drawer on phones), page tabs, properties, side panel (Outline, Linked from, Wanted pages), one Edit button plus More menu, link previews.
4. **`feat/codices tags and images`**: tags, the base template, image upload, cover, lightbox.
5. **`feat/codices graph`**: the graph endpoint, the full graph view and the side panel's local graph. After tags, so the tag filter works from the start.
6. **`feat/character codex link`**: the character URL field and button.

E2E tests: add them for access rights (a user without See can't reach a codex), for one image upload, and for the graph (it opens and lists the codex's pages). Everything else as the coding skill decides.
