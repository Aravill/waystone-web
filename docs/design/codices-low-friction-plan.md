# Codices: low-friction plan

**Status:** proposed on 2026-11-21. Where this plan and the code disagree, the code and CLAUDE.md win.

- **Goal:** the least possible effort between "I want to note this" and a note in the codex, and wiki pages that keep themselves tidy.
- **Design source:** `docs/design/mockups/codex-low-friction.html` (interactive: N for a note, drag in the tree, `/` and Ctrl+K in the editor).
- **Code references** are from `main` at `4148bde` and will drift.

## User decisions

| Topic | Decision |
|---|---|
| Quick notes | Private: only the author sees a note until they file it. One inbox per user per codex |
| Journal | Journal entries become codex pages. A game with a codex keeps its journal in a folder `{game}: Campaign Journal` in that codex, one page per played session |
| Journal rights | Normal codex rights: only users with Edit on the codex write journal pages; reading needs See |
| Journal without a codex | No journal. The old journal page goes away; a game gets a journal when its DM links a codex, and its existing entries move into it then |
| Top-level pages | Already supported (`pages.folder_id = ''`), listed under the folders. Drag-and-drop adds a "Top level" drop target |
| In scope | Quick notes, drag-and-drop, draft autosave, link from selection, the journal move, live lists, default template per folder, the `/` menu, unlinked mentions |

## Facts from the code that shape the design

- **New pages are created, then edited.** `submitCreatePage` (`static/codex.js:1923`) POSTs and goes to `/edit`, so a new page's draft is a draft of revision 1.
- **The server can start a page with a body or a template.** `handleCreatePage` (`api/codex.go:490`) through `pageStart` (`api/codex_templates.go:250`); `fillTemplate` (`:57`) fills `{{title}}` and `{{date}}` only.
- **The Markdown renderer is small.** `renderMarkdown` (`static/campaigns.js:1250`) has quotes but no code blocks, `---` or tables. `codexMarkdown` (`codex.js:44`) already swaps placeholders in and out around it.
- **Links come only from the stored body.** `ParseWikiLinks` (`db/codex_links.go:20`). Anything rendered on the client never reaches `page_links`.
- **The tree is HTML strings** (`codexFolderHtml` / `codexPageItemHtml`, `codex.js:276-292`, `x-html` at `codex.html:88`) with delegated clicks (`onTreeClick`, `:681`).
- **The move APIs exist:** `PUT /api/codices/pages/{id}/folder` and `PUT /api/codices/folders/{id}` `{parent_id}` (`MoveFolder` checks cycles, depth and names, `db/sqlite_codex_folders.go:202`).
- **Session numbers are positional and never stored** (CLAUDE.md). Journal page titles can't contain them.
- **`codex.html` loads `journal.js` only for its diff helpers** (`diffWords` / `diffHtml`, `codex.html:932`, used in `save()` at `codex.js:2437`).
- **New columns go in by ALTER-if-missing** (`db/sqlite_store.go:467-498`). An index on a new column is created after its ALTER, not in the CREATE block.
- **The seed exercises the journal move.** `seedIfEmpty` writes journal rows, then `seedCodex` calls `SetCampaignCodex` for the Sojourn game (`db/seed_codex.go:49`). E2E runs with `SEED_DEMO_DATA=1`, so the migration runs on every E2E start.

## 1. Quick notes and the Inbox

**What the user sees.** A **Note** button in the Codices top bar (next to search) and the `n` key (ignored while typing, like `/`) open one text box: no title, no folder. Ctrl/Cmd+Enter saves; the toast offers "Open inbox". The nav's "This codex" group gets **Inbox** with a count badge. The inbox lists the viewer's notes, newest first, each with **New page**, **Add to page…**, **Edit** and **Delete** (immediate, with Undo in the toast, which posts the note again). The game page gets the same button and key when the game's codex is editable by the viewer.

**Schema** (CREATE block):
```sql
CREATE TABLE IF NOT EXISTS codex_notes (
    id TEXT PRIMARY KEY,
    codex_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codex_notes_owner ON codex_notes(codex_id, user_id, created_at);
```
- Body 1–5000 characters after trimming. At most 200 notes per user per codex (409 `inbox_full`).
- Cascades by hand: `DeleteCodex` deletes the codex's notes; `AnonymizeUser` deletes the user's (`sqlite_store.go:1124` table loop).

**API** (new `api/codex_notes.go`, dispatched from `HandleCodexRoutes`, `api/codex.go:181`):

| Route | Rights | Notes |
|---|---|---|
| `GET /api/codices/c/{id}/notes` | See | The caller's own notes only |
| `POST /api/codices/c/{id}/notes` `{body}` | Edit | 201 `{note}` |
| `PUT /api/codices/notes/{id}` `{body}` | Owner + Edit | Edit a note |
| `DELETE /api/codices/notes/{id}` | Owner + See | Someone else's note is 404 |
| `POST /api/codices/notes/{id}/append` `{page_id}` | Owner + Edit | Page must be in the note's codex. Appends, saves a revision noted "Added a quick note", deletes the note, in one transaction |
| `POST /api/codices/pages` with `note_id` | Edit | Filing as a new page: the create and the note's deletion in one transaction |

- `handleGetCodex` (`api/codex.go:411`) adds `note_count` for the viewer.
- **Appended text:** a blank line, `_21 Nov 2026 · Roy_` on its own line, then the note.
- **New page from a note:** title defaults to the first line (`[]|` removed, ≤120 characters). The body is the chosen template (filled), a blank line, then the note. The server builds it from `note_id`.
- Listing and deleting need only See, so someone demoted to See can still read and clear their own notes.

**Store:** `GetCodexNotes`, `CountCodexNotes`, `CreateCodexNote` (`ErrInboxFull`), `GetCodexNote`, `UpdateCodexNote`, `DeleteCodexNote` (`ErrNoteNotFound`), `AppendNoteToPage(noteID, userID, pageID, addition, maxRunes, keep)` (does what `SavePageRevision`, `db/sqlite_codex.go:383`, does, in one transaction). Extract `createPageWithTx(tx, p, body, x)` from `CreatePageWith` (`db/sqlite_codex_templates.go:14`) so note filing and journal pages reuse it. `models.NewPageExtras` gets `NoteID`.

**Frontend:** `onGlobalKey` (`codex.js:1022`) opens `dlg.type = 'quickNote'`; the box keeps its own local draft (`codex-quicknote:{codexId}`). Inbox route `/codices/c/{id}/inbox` in `parseRoute` (`:444`), like `graph`. **New page** calls `openCreatePage(firstLine, null, '', { noteId, noteBody })` (`:1568`) and shows the note under the preview. **Add to page…** reuses the timeline page picker (`tlPickResults` / `pickPage`, `:1265` / `:1365`). Game page: `codexRef` (`api/campaigns.go:177`) gets `can_edit`; `game.js` / `game.html` get the small note modal.

**Edge cases:** losing access keeps the notes stored (back when access returns). Appending to a page deleted meanwhile is 404 and keeps the note. Two tabs filing one note: the second gets 404 `note_not_found`.

**E2E:** `n` on a codex page, save, badge shows 1; file it as a new page (title from the first line, note gone); add another to an existing page (new revision with the signed paragraph); a See-only user gets no dialog and 403 on POST.

## 2. Drag-and-drop in the tree

**No backend change:** the two move routes above.

- `codexPageItemHtml(p, currentId, canDrag)` puts `draggable="true" data-drag="page:{id}"` on the `<a>`; `codexFolderHtml` does the same on the folder's `<summary>` with `folder:{id}`. Only with Edit and `matchMedia('(pointer: fine)')`.
- Delegated `dragstart/over/leave/drop/end` on `.codex-tree`, with a custom `application/x-codex-item` type so the image-drop handlers (`codex.js:424-429`) never fire.
- **Targets:** a folder row (dashed rubric box and "← drop here", never colour alone), and a "Top level of {codex}" strip that appears above the tree while dragging. Hovering a closed folder for 700 ms opens it (transient open).
- **Refused on the client** (no drop cursor): a folder into itself or a descendant, deeper than 10 levels (mirror `folderDepth` / `folderHeight`), or onto its current parent.
- **After a drop:** `keepFolderOpen(target)`, reload, toast "Moved X to Y." with **Undo** (moves it back). Errors use the `failDlg` wording (`:1912`), e.g. `folder_name_taken`.
- **Touch:** no long-press drag (clashes with scrolling and the link menu, and HTML5 drag is unreliable on phones). The folder menu's Move and the page's Folder › Change stay the touch path.

**E2E:** drag a page onto a folder (its `folder_id` changes); drag a folder to the top level; a folder dropped into its own child sends no request.

## 3. Draft autosave

**Local only, no backend change.** A closed tab, crash or expired session are the common losses; server drafts can come later.

- Key `codex-draft:{pageId}:{base}`, value `{body, note, initial, at}` (`initial` is the body at the base, for the conflict diff). Helpers `codexDraftWrite` / `Find` (newest wins) / `Clear` (every base) / `Prune` (older than 30 days, at most 20 kept; storage errors swallowed).
- Written 800 ms after the last change (editor `onChange` at `:2419`, the note field, the fallback textarea). The editor bar shows "Draft kept on this device · 18:04".
- **Opening the editor** (`initDraft`, `:2409`) with a stored draft that differs: a banner "You have unsaved changes from <time>, kept on this device." with **Restore them** and **Discard**.
  - Draft on the current base: restore body and note.
  - Draft on an older base: restore it with its old base and `initial`. The next Save gets 409 `stale_base` and the existing conflict panel (`codex.html:386`) handles it. No new conflict UI.
- **Cleared on** a successful save, `discardMine` (`:2461`) and Cancel. Cancel becomes a button that asks "Discard your changes?" when there are any.
- Covers new pages (revision 1) and, after section 5, journal pages.

**E2E:** type, reload, Restore, text is back; type, another user saves, reload, Restore, Save, the conflict panel appears.

## 4. Link from selected text, and creating the missing pages

**Ctrl/Cmd+K** (and a **Link** button next to "Add image", `codex.html:408`) in `codex-editor.js`: a `Mod-k` keymap entry running `linkSelection(view)`, off when nothing is selected. The selection (one line, ≤120 characters, no `[]|`) is matched by `norm` against titles and aliases:
- same title, same case: `[[Title]]`
- other case or an alias: `[[Title|as selected]]`
- no match: `[[as selected]]` (a page not written yet)

**After saving** (`save()`, `codex.js:2426`), `codexNewMissing(initialBody, savedBody, titleLookup)` finds link targets that don't exist and weren't targets before (deduped, at most 20). If any, a dialog **"Create these N pages?"**: one row each with a checkbox (on), a folder (default: the saved page's folder, or the top level when saving a journal page) and a template (default: that folder's default, section 7, else Blank). **Save and create** POSTs each ticked row to the existing create route in turn and reports any failure on its row (e.g. `name_taken`); **Just save** skips. No batch endpoint.

**E2E:** select text matching an alias, Ctrl+K gives `[[Title|text]]`; save with two new links, tick one, only that page exists, with its template's body.

## 5. The game journal becomes codex pages

### Decisions

- **No new page kind.** Journal pages are ordinary pages (`kind = 'lore'`) with `pages.session_id` set. A copy never takes `session_id`.
- **Title:** `{game}: {YYYY-MM-DD}` (the session's date), set once and then a normal title. Duplicates get " (2)". ISO dates sort in date order in the tree. Re-dating a session doesn't rename its page.
- **In the tree,** a page inside a journal folder drops the `{game}: ` prefix, so the folder reads as a list of dates.
- **The number is shown when read.** The page payload gets `session {campaign_id, campaign_title, session_id, number|null, date, status, played}` from `journalSession` (`api/journal.go:88`). The page shows the eyebrow "Campaign journal · {game}", the title "Session 12", and a strip with the date, who was at the table and a link to the session.
- **The folder is found by id,** `campaigns.journal_folder_id`, created when first needed at the codex's top level as `{game}: Campaign Journal` with the built-in Session template as its default (section 7). It is an ordinary folder: rename, move or delete it freely. If it's gone, the next entry makes a new one.
- **Game rename:** the folder follows only while it still has the generated name. Page titles don't change (that would rewrite links and add revisions across the codex).
- **Changing or clearing the game's codex moves nothing.** `journal_folder_id` is cleared; existing pages stay with their `session_id` and the game page still lists them ("in {codex}" as plain text if the viewer can't see it). New entries go to the new codex. One page per session (unique index).
- **Deleting a by-hand session keeps its page** and clears `session_id`, so it becomes an ordinary page. This changes the old rule: codex pages have links and other authors, and the DM may not have Edit on the codex. The confirm text says "Its journal page stays in the codex as an ordinary page." `DeleteCampaign` does the same.
- **A cancelled session's page** keeps its link and stays editable; the game page lists it under "Journal pages of cancelled sessions".
- **Revisions** follow the page limit (`PageKeepRevisions`, 200). Migrated revisions keep their numbers and `page_contributors` keeps the credit.
- **Template date:** `{{date}}` is the session's date, through a new `fillTemplateOn(body, title, date)`.
- **Entries are started from the game page.** "Write the entry" creates the page and opens its editor.
- **The legacy journal tables stay** until no rows remain (entries of games without a codex wait there). Only `SaveJournalRevision` stays in the Store interface, for the seed. Drop the tables in a later cleanup.

### Schema
```sql
ALTER TABLE pages ADD COLUMN session_id TEXT NOT NULL DEFAULT '';
ALTER TABLE campaigns ADD COLUMN journal_folder_id TEXT NOT NULL DEFAULT '';
-- after the ALTERs:
CREATE UNIQUE INDEX IF NOT EXISTS idx_pages_session ON pages(session_id) WHERE session_id <> '';
```
`models.Page.SessionID` is read by `GetPage` and written by `createPageNoteTx` (`db/sqlite_codex.go:114`). `journal_folder_id` stays out of `models.Campaign`.

### Migration
`migrateCampaignJournalTx(tx, campaignID)` does nothing without a valid codex. Otherwise, per `journal_entries` row:
1. Ensure the journal folder.
2. Read the session's date and the entry's newest revision.
3. Title `{game}: {date}`, plus ` – {name}` if the newest revision has a name, cleaned like `codexName` and made unique with " (n)".
4. Insert the page (author, `created_at` from the entry, `updated_at` from the newest revision, `session_id`).
5. Each kept `journal_revisions` row becomes a `page_revisions` row **with the same number** (body, `restored_from`, `created_at`, the old note plus "Merged with revision N" when merged).
6. Copy `journal_contributors` into `page_contributors`.
7. `rebuildPageLinks`, then delete the entry's legacy rows. After the campaign, `resolveCodexLinks(tx, codexID)`.

Called from `SetCampaignCodex` (now one transaction: set `codex_id`, clear `journal_folder_id`, migrate) and from a startup `migrateJournalToCodex()` at the end of `initSchema` (one transaction per campaign; idempotent because migrated rows are gone). Entries whose session row is gone are skipped and kept.

**Before deploying:** run the build against a copy of the latest `./backups` snapshot and compare entry and revision counts before and after.

### API
- `api/journal.go` keeps only `POST /api/campaigns/{id}/journal/{sid}`: a played session (`journalSession`), a codex on the game (409 `no_codex`), See Codices and Edit on that codex (403). Returns 201 `{page:{id}}`, or 200 with the existing page. Flow: `db.EnsureJournalFolder`, `pageStart` with the folder's template and the session date, `db.CreateJournalPage` (unique title and `session_id` checked in the transaction).
- Remove the old GET, PUT, DELETE, revisions and restore routes. Keep `journalUsers` / `journalUser` / `journalUserList` (the codex code uses them).
- `handleGetCampaign` (`api/campaigns.go:307-371`): `GetJournalPages(campaignID)` replaces `GetJournalSummaries` (one query: pages by session, newest revision excerpt, revision count, contributors). Each row gets `page_id, title, codex_id, can_open`. Add `journal_pending` (legacy rows, DM and admin only) and `codex.can_edit`.
- `handleUpdateCampaign` (`:688`): on a title change, `db.RenameJournalFolder(campaignID, oldTitle, newTitle)`.
- `DeleteManualSession` (`sqlite_store.go:1369`) and `DeleteCampaign` (`:2004`): `UPDATE pages SET session_id = '' WHERE session_id IN (...)`. `DeleteCodex` clears the games' `journal_folder_id`.
- **Old URLs** `/games/{id}/journal/{sid}[/edit|/history]` (in the `/games/` handler, `api/router.go:104-118`): 303 to the page (`/codices/p/{pageId}/…`) or to the game. 303, not 301: a page can be deleted and written again under a new id.
- **Store:** remove `GetJournalSummaries`, `GetJournalEntry`, `GetJournalRevisions`, `DeleteJournalEntry`; add `GetJournalPages`, `CountLegacyJournal`, `EnsureJournalFolder`, `CreateJournalPage`, `RenameJournalFolder`, `GetJournalPageID`.

### Frontend
- Delete `static/journal.html` and `static/journal.js`. Move `diffWords`, `tidySegs`, `diffHtml` and `MAX_LCS_CELLS` into `codex.js` (with `escapeHtml`) and drop the script tag at `codex.html:932`.
- **Game page** (`game.html:106-166`, `224-245`; `game.js` `journalUrl` `:322`, `deletePlayed` `:502`): rows link to the codex page; "✎ Write the entry" is a button (POST, then the editor), shown with `campaign.codex.can_edit`. Without a codex the DM sees "Link a codex in Edit game to keep a journal", plus "{n} older journal entries are waiting" when `journal_pending > 0`. A page the viewer can't open shows "Written · in {codex}".
- **Codex page:** the journal eyebrow, title and strip from `pageData.session`.
- **Docs:** mark `game-journal-plan.md` superseded and rewrite the journal gotcha in CLAUDE.md.

### E2E
Replace `e2e/journal.spec.js`: the seeded Sojourn entries are pages in the demo codex's journal folder with their revision counts and scribes; a player with Edit writes an entry from the game page and lands in the editor with the Session template and the session's date; a player without Edit sees no button and gets 403; a Suggested session gets 409; a game without a codex shows the DM hint; an old journal URL redirects; deleting a by-hand session keeps the page without `session`; changing the game's codex leaves the old page linked.

## 6. Live lists

**Syntax:** a paragraph of its own, `{{list key:value ...}}`. Values are bare or `"quoted"`.

| Key | Meaning |
|---|---|
| `tag:` | Can repeat; every tag must match |
| `folder:` | A name or a path (`Places/Inns`), including sub-folders |
| `links:here` | Only pages that link to this page (from the page's "Linked from" list, already in the page payload) |
| `kind:` | `lore` or `timeline` |
| `sort:` | `title` (default), `updated` (newest first), `created` |
| `limit:` | 1–200, default 50, then "and N more" |

- `{{list tag:npc links:here}}` on a settlement page lists every NPC page that says "Lives in [[that settlement]]", with no one maintaining the list. The `/list` snippet inserts it.
- An unknown key shows "Unknown list option: x". No clash with templates: `fillTemplate` replaces only the exact `{{title}}` and `{{date}}`.
- **Rendered on the client in `codexMarkdown`:** each list line is swapped for a placeholder before rendering, then a `<p>` holding only the placeholder becomes a ruled box (verdigris left edge, "Live list · tag npc · linking here" and a count in its header) of `wikilink` anchors with `data-pid`, so link previews work. Built from `this.codex.pages` / `tags` / `folders` (the current page left out). History revisions and template previews show a plain "Live list: tag:npc".
- **Links in a list don't count:** they're never in the stored body, so never in `page_links`, backlinks or the graph.
- **Server:** `GetCodexPages` adds `created_at` / `updated_at` to `PageListItem`. Strip list lines from `codexPlainExcerpt` (`:159`) and `graphExcerpt`.
- **Not in v1:** filtering by "Key: value" lines (the codices plan keeps pages free text).

**E2E:** `{{list tag:npc sort:title}}` lists the tagged pages in order; tagging another page adds it after a reload; the list adds nothing to the target's "Linked from".

## 7. Default template per folder

**Schema:** `ALTER TABLE codex_folders ADD COLUMN default_template TEXT NOT NULL DEFAULT ''`: `''` none, `builtin:<id>`, or a template id of the same codex. `models.Folder.DefaultTemplate`; add it to `folderColumns`, `scanFolder` and `CreateFolder` (`db/sqlite_codex_folders.go:34`, `:38`, `:170`).

**API:** `PUT /api/codices/folders/{id}` also takes `{default_template}` (Edit; the value must be empty, a built-in, or a template of this codex, else 400). Folders in the codex payload carry it. `DeleteCodexTemplate` (`db/sqlite_codex_templates.go:234`) clears it in the same transaction.

**Frontend:** the folder menu (`codexFolderMenuHtml`, `codex.js:295`) gets **Folder settings…** with "New pages here start from" (and the name). In New page (`openCreatePage`, `:1588-1595`) the folder's default wins over the remembered last template, marked "Folder default" on its card; changing the folder switches to that folder's default unless the user picked a card. A sub-folder without a default uses its parent's. Missing-link pages, note filing, "Create these pages?" and journal pages all use it. Timeline "Create a page for it" keeps `builtin:event`.

**E2E:** set a folder's default to Character; "+ Page" there and a missing link from a page there both preselect Character; deleting a codex template used as a default clears it.

## 8. The `/` menu

Typing `/` at the start of a line or after a space opens a filterable list under the cursor; ↑↓ and Enter insert, Esc closes. A second completion source next to `complete` (`codex-editor.js:66`): `autocompletion({ override: [complete, slash] })`, matching `/[\w-]{0,20}` after a line start or whitespace (URLs and "and/or" never trigger it). A **Insert /** button on the editor bar types the `/` for mouse users.

| Item | Inserts |
|---|---|
| Today's date | The date |
| Person | `**Name:**` / `**Role:**` / `**Wants:**` / `**Lives in:** [[ ]]` |
| Callout | `> **Note:** ` |
| Loot list | `## Loot and rewards` and three bullets |
| Live list | `{{list tag: links:here}}` |
| Divider | `---` (`codexMarkdown` renders a block that is exactly `---` as `<hr>`) |
| Each template | Its `##` headings (`codexHeadingsOnly`, `:76`); templates without headings are left out |

No dice and no tables (the renderer has neither). Templates load once per codex in `initDraft`.

**E2E:** `/npc` + Enter inserts the block; `/Session` inserts that template's headings.

## 9. Unlinked mentions

**What the user sees.** A side-panel box **Mentioned, not linked** under "Linked from": each page that names this page's title or an alias in plain text, with a snippet (the mention underlined) and **Link** (or "Link 3" for several). **Link all** when more than one page. Linking makes a new revision in that page and moves it to "Linked from". Viewers with See get the list without buttons.

**Search on the server:**
- `GetCodexBodies(codexID)`: one query, pages joined to their newest revision.
- Pure functions in `db/codex_links.go`, next to `rewriteLinksTo`: `FindMentions(body, names)` and `LinkMentions(body, title, names)`.
  - Names: the title and aliases, deduped by `NormName`, at least 3 characters; any run of spaces inside a name matches.
  - Case-insensitive whole words (no letter, digit or `_` on either side, checked by hand since Go regexp has no lookbehind); the longest match wins.
  - Masked: `[[…]]`, `![[…]]`, code spans, `[text](url)`, bare URLs, `{{list …}}` lines.
  - A mention spelled exactly as the title becomes `[[Title]]`, anything else `[[Title|as written]]`.
- No SQL prefilter: SQLite's case-insensitive matching is ASCII-only. Lower-casing a few hundred bodies in Go is milliseconds.
- Pages that already link here are left out (they're in "Linked from"). At most 50 pages returned, `total` counted.

**API** (new `api/codex_mentions.go`, from `handleCodexPageRoutes`, `api/codex.go:565`):
- `GET /api/codices/pages/{id}/mentions` (See): `{total, mentions:[{page:{id,title}, current_revision, count, snippets:[{before, match, after}]}], can_edit}`, two snippets at most, ±60 characters.
- `POST /api/codices/pages/{id}/mentions/link` `{from_page_id, base_revision}` (Edit, same codex): `LinkMentions`, then `saveCodexRevision` (`:800`) with the note "Linked mentions of {title}". Nothing found is 200 `{linked: 0}`; a stale base is 409 and the client reloads the list.

**Frontend:** loaded after the page, like `loadLocalGraph` (`codex.js:945`), on the page screen only. **Link all** runs the Links in turn with progress.

**E2E:** B says "the Old Mill"; A ("Old Mill") lists B; Link turns B's text into `[[Old Mill|the Old Mill]]` and B moves to "Linked from"; a See-only user sees no button and gets 403.

## PRs

In this order; each leaves the app working. Every PR updates CLAUDE.md, bumps `?v=` on changed CSS/JS, and adds a link to this doc in `codices-plan.md`.

1. **`feat/codices drafts`** (3). Frontend only.
2. **`feat/codices drag and drop`** (2). Frontend only.
3. **`feat/codices folder templates`** (7). Used by 4, 7 and 9.
4. **`feat/codices editor links`** (4). Ctrl+K and "Create these pages?".
5. **`feat/codices live lists`** (6).
6. **`feat/codices slash menu`** (8). After 4 (same editor file) and 5 (the list snippet).
7. **`feat/codices quick notes`** (1). Includes the game-page button.
8. **`feat/codices unlinked mentions`** (9). Independent.
9. **`feat/codices game journal`** (5). Last and largest: schema, migration, journal API, game page, redirects, removing the old journal, E2E rewrite. Not split: half-moved journal data would break the old UI.

## Later

- Moving a game's journal pages when its codex changes ("Move journal pages to {codex}"), if DMs ask.
- A quick-note button on every page of the app, not only Codices and the game page.
- Live-list filters on "Key: value" lines.
- Server-side drafts.
