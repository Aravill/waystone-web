# Game journal: implementation plan

**Status:** implemented on `feat/game-journal` on 2026-11-06. Kept as the record of the design decisions.

- **Design source:** `docs/design/mockups/game-journal.html`. Port it, don't redesign it. Its JS holds the behaviour, copy and merge logic.
- **Code references:** line numbers are from `main` at `c68cbd2` and will drift.
- **Uncommitted right now:** this file and the mock-up are untracked in the main checkout. The nav fix is uncommitted in `.claude/worktrees/fix-sub-page-nav` (branch `fix/sub-page-nav`). See "Nav fix" below.

## To resume

1. Answer the one open question below.
2. Follow the `coding` skill from its worktree step: this file is the plan.
3. Don't commit until asked.

## Decisions

| Topic | Decision |
|---|---|
| Who can read | Anyone logged in, including history |
| Who can create, edit and restore | The DM and current players, including players who missed the session. Admins can't, unless they're also members |
| Who can delete | The DM and site admins. Permanent, removes all revisions, and the session stays |
| Former players | Keep their credit, can't edit |
| Which sessions can have an entry | **Only sessions that happened:** Confirmed and in the past, or created by hand. Anything else is refused |
| By-hand session deleted | Its entry and revisions are deleted too, because the session didn't happen. The confirmation warns and names the revision count |
| Revision limit | **Keep the latest 200 per entry and prune the oldest**, so saving never gets stuck |
| Body length | 20,000 characters, same as the game description |
| "What changed" note | Stays, optional |
| Notifications | Out of scope |
| PR | **One PR** for the journal and the nav fix |
| Tests | **E2E tests are in scope** |

## Open question

**An entry already exists and its session is then cancelled. What happens?** Decision 2 stops new entries on such a session but doesn't settle existing ones. The calendar lets a DM cancel a past session, with a generic warning and no undo in the UI, though the API can set it back to Confirmed.

- **Recommended:** keep the entry read-only and out of the played list, with one muted "Entries on cancelled sessions" line of links under Played so the DM can find and delete it. Nothing is lost by a one-click cancel. This line isn't in the mock-up.
- **Alternative:** treat it like decision 3 and delete the entry on cancel. Consistent with "it didn't happen", but a one-click cancel would destroy other people's writing, so the cancel warning would need to say so.

The steps below assume the recommended option.

## Key facts from the code

- **Session numbers are positional.** They're computed per request and never stored (`campaignSessionSummaries`, `api/campaigns.go:37-90`, number is `i + 1` at `:292`). Recording, deleting or re-dating a by-hand session, or cancelling a played one, renumbers later sessions. So entries key on **session id**. The title number is computed at read time and URLs use the id. Never store a number in a revision, a notification or seed data.
- **One id space.** By-hand and calendar sessions are both rows in `campaign_sessions`, split by `manual` (`db/sqlite_store.go:100-114`).
- **Foreign keys are not enforced** (`:33`, `:1645`). Cascades are written by hand.
- **No migration framework.** New tables go in the `CREATE TABLE IF NOT EXISTS` block in `initSchema` (`:55-183`).
- **Writes are serialised.** Each takes `s.mutex.Lock()`, so check-then-insert in one transaction is atomic. The mutex isn't reentrant, so use `tx.*` inside a transaction, never `s.*`. Assumes one app process, as in `docker-compose.yml`.

## Schema

Add after `notifications`:

```sql
CREATE TABLE IF NOT EXISTS journal_entries (
    id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL,
    session_id TEXT NOT NULL UNIQUE,
    author_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES campaign_sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (author_id) REFERENCES users(id)
);
CREATE TABLE IF NOT EXISTS journal_revisions (
    entry_id TEXT NOT NULL,
    number INTEGER NOT NULL,
    author_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    restored_from INTEGER NOT NULL DEFAULT 0,
    merged_with INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    PRIMARY KEY (entry_id, number),
    FOREIGN KEY (entry_id) REFERENCES journal_entries(id) ON DELETE CASCADE,
    FOREIGN KEY (author_id) REFERENCES users(id)
);
-- Credit outlives pruning: one row per person per entry.
CREATE TABLE IF NOT EXISTS journal_contributors (
    entry_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    revisions INTEGER NOT NULL,
    first_revision INTEGER NOT NULL,
    PRIMARY KEY (entry_id, user_id),
    FOREIGN KEY (entry_id) REFERENCES journal_entries(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_journal_entries_campaign_id ON journal_entries(campaign_id);
```

- `journal_entries.id` is a fresh UUID per entry. Numbers restart after delete and re-create, so a number alone can't identify a base revision.
- `author_id` and `journal_contributors` exist because of pruning. Without them, pruning revision 1 would erase the author and anyone whose only edits were old.

### Pruning

- In the save transaction, after inserting, delete revisions where `number <= latest - 200`.
- Numbers keep counting (revision 201, 202...) and are never reused.
- "Revisions" counts in the scribes panel come from `journal_contributors`, so they include pruned ones. The history button shows how many are kept.
- The history page says "Only the latest 200 revisions are kept" once pruning has happened.
- The oldest kept revision has no previous one to compare, so it shows the first-version state with different wording.
- `restored_from` and `merged_with` can name a pruned number. They're display text only.
- A stale client whose base was pruned merges against an empty base, so everything conflicts and nothing is lost.
- Pruning isn't shown in the mock-up.

## Pages and API

**Pages:** `/games/{id}/journal/{sessionId}`, plus `/edit` and `/history`. Extend the closure at `api/router.go:99-106` to serve `journal.html`. Load the script as `/static/journal.js?v=1`, so no new handler is needed.

**API:** in `HandleCampaignRoutes` (`router.go:130`) dispatch `parts[1] == "journal"` to `HandleJournal` in a new `api/journal.go`.

| Under `/api/campaigns/{id}/journal/{sid}` | Permission | Statuses |
|---|---|---|
| `GET` root | Any logged-in user | 200, 401, 404 |
| `PUT` root (save) | `hasCampaignSessionAccess` (`api/sessions.go:589`) | 201, 200 unchanged, 400, 401, 403, 404, 409 |
| `DELETE` root | `loadManagedCampaign` (`api/campaigns.go:512`, DM or admin) | 200, 403, 404 |
| `GET /revisions` | Any logged-in user | 200, 404 if no entry |
| `POST /revisions/{n}/restore` | `hasCampaignSessionAccess` | 201, 200 unchanged, 401, 403, 404, 409 |

Reuse `loadCampaignAndUser` (`api/signups.go:148`), `validateSessionCampaign` (`api/sessions.go:570`), `writeJSON`/`writeJSONError` (`:620-629`) and `userDisplayObject` (`api/signups.go:167`) over one `GetAllUsers`, which includes deleted users.

**`GET` root:**

```json
{"session":{"id":"","number":31,"played":true,"status":"Confirmed","date":"","time":"","duration":180,"timezone":"","manual":false},
 "can_edit":true,"can_delete":false,"entry":null}
```

`number` is `null` when the session isn't played. When an entry exists, `entry` is:

```json
{"id":"","session_id":"","campaign_id":"","current_revision":5,"revision_count":5,
 "name":"","body":"","updated_at":"",
 "author_id":"","author_user":{},"amender_ids":[],"amender_users":[],
 "updated_by":"","updated_by_user":{},
 "scribes":[{"user_id":"","user":{},"revisions":2}]}
```

**`GET /revisions`:** all kept bodies, ascending.

```json
{"entry_id":"","current_revision":5,"pruned":false,"revisions":[
  {"number":1,"author_id":"","author_user":{},"name":"","body":"","note":"","restored_from":0,"merged_with":0,"created_at":""}]}
```

**`PUT` request:** a new entry sends `entry_id:""` and `base_revision:0`. `merged_with` must be 0 or equal `base_revision`, otherwise 400.

```json
{"entry_id":"","base_revision":4,"name":"","body":"","note":"","merged_with":4}
```

**`PUT` responses:**

- 201: `{"status":"success","message":"","created":true,"entry_id":"","revision":{...}}`
- 200 (unchanged): same shape with `created:false`
- 409: `{"error":"","code":"","entry_id":"","latest":{...}|null}`, where `code` is `stale_base`, `entry_replaced`, `entry_deleted` (`latest:null`) or `session_not_played`

**Restore** takes `{"base_revision":5}`. If stale it returns 409 `stale_base`, and the UI reloads history with a message. It writes `restored_from=n` with an empty note through the same store method, so restoring content identical to the latest creates nothing.

**Played check:** `journalSession(campaign, sessionID)` calls `campaignSessionSummaries([]models.Campaign{*campaign})` and finds the index in `played`, giving number and played flag. Don't duplicate the sort. This one check enforces decision 2 on save and restore.

## Store

Interface in `db/store.go`, implementation in `db/sqlite_store.go`, wrappers in new `db/journal.go`, types in new `models/journal.go`:

```go
GetJournalSummaries(campaignID string) (map[string]models.JournalSummary, error)
GetJournalRevisions(sessionID string) (entryID string, revs []models.JournalRevision, err error)
SaveJournalRevision(campaignID, sessionID, entryID string, baseRevision int, rev models.JournalRevision, keepRevisions int) (models.JournalSaveResult, error)
DeleteJournalEntry(sessionID string) error
```

`SaveJournalRevision`, under one lock and transaction:

1. Session still exists in the campaign, else `ErrJournalSessionGone`.
2. Read the entry and latest revision.
3. No entry and `base != 0`: outcome `EntryDeleted`. No entry and `base == 0`: create the entry with `author_id`, number 1.
4. `entryID` or `base` mismatch: outcome `Stale`, returning the latest.
5. Same name and body as latest: outcome `Unchanged`.
6. Insert the revision, upsert `journal_contributors`, prune, commit.

**Hand cascades:** delete contributors, revisions, then entries, in `DeleteCampaign` (`:1657`, before `campaign_sessions`) and `DeleteManualSession` (`:1070`, using the existing `manual = 1` guard). `DeleteSession` needs none, because it only deletes Suggested sessions.

**Summaries without N+1:** one extra call in `handleGetCampaign` after `:265`, then `"journal": null | {...}` on each played row (`:290-299`). Not in `campaignSessionSummaries`, or the Games list pays for it.

```json
{"entry_id":"","name":"","revisions":5,"updated_at":"","excerpt_source":"",
 "author_id":"","author_user":{},"amender_ids":[],"amender_users":[]}
```

The latest name, time and first 600 characters come from the newest revision. Author from the entry. Amenders from `journal_contributors` ordered by `first_revision`, excluding the author. The chronicle count is `played.filter(s => s.journal).length`. Summaries with no matching played row are the cancelled-session entries (`obj["journal_cancelled"]`).

## Validation

- **Name:** trim and collapse whitespace, optional, max 80 runes.
- **Body:** strip `\0`, CRLF to LF, split on `\n[ \t]*\n`, trim paragraphs, drop empties, rejoin with `\n\n`. Required, max 20,000 runes (`api/campaigns.go:495`). Storing this canonical form makes "no change" an exact comparison.
- **Note:** single line, max 120 runes.
- **Request size:** `http.MaxBytesReader(w, r.Body, 128<<10)` as in `api/played_sessions.go:84`.

## Frontend

**One page:** `static/journal.html` and `static/journal.js`, with one Alpine `journalPage()` picking `entry`, `edit` (merge screen as state inside it) or `history` from `location.pathname`. Real links, no client router.

**Data:** fetch `/api/campaigns/{id}` (title, DM, attendees, prev/next) and `journal/{sid}`. Fetch `/revisions` on the history view and on a 409.

**Shared helpers:** move `renderMarkdown` (`static/game.js:8-54`) unchanged to `static/campaigns.js`, next to `escapeHtml`. Add `journalByline`, `journalExcerpt` and `journalTitle` there. Port `diffWords`, `tidySegs`, `diffHtml`, `toParas`, `lcsPairs`, `merge3`, `mergeName`, `conflictsOf`, `resolvedParas`, `mergedText` and `mergedName` from the mock-up to `journal.js`.

**Conventions:** `fetch` only, try/catch, `showMessage`. `x-text` for user content. `x-html` only for `renderMarkdown`, `diffHtml` and `userAvatarHtml`. Confirm pop-ups copy the modal at `game.html:352-366` and `askConfirm` at `game.js:562`, with the mock-up's copy; don't port `<dialog>`. Breadcrumb is `.game-crumb` as in `calendar.html:32-38`.

**CSS:** a new "Journal" section in `styles.css`. Don't port the mock-up's global `input`, `textarea`, `ins`, `del`, `dialog` or `h1` rules. Scope `ins`, `del` and generic names (`.tab`, `.notice`, `.field`, `.label`, `.version`, `.diff`, `.gap`, `.same`, `.tag`, `.rev`, `.split`, `.detail`) under `.journal-page`. Keep the app's 900px and 560px breakpoints, not the mock-up's 1024px.

| Mock-up | App |
|---|---|
| `.md` | `.game-text` |
| `.crumb` | `.game-crumb` |
| `.eyebrow` | `.modal-eyebrow` |
| `.avatar` | `.user-avatar` via `userAvatarHtml` |
| `.btn--primary` | `.submit-btn` |
| `.btn` | `.campaign-action-btn` |
| `.btn--danger` | `.btn-danger` |

**Played rows:** the `<li>` becomes `.game-row`. Move the bottom border, hover and `.is-selected` from `.game-entry` (`styles.css:2844-2890`) to it, and add `.entry-line` under the button. `styles.css:2815` already reserves this room. The journal block is the first card in `selectedPlayedList` (`game.html:193`).

**Cache-busting:** bump `styles.css?v` once across all 11 pages, plus `campaigns.js?v` on `calendar.html`, `campaigns.html` and `game.html`, `game.js?v`, `change-password.js?v`, and `journal.js?v=1`.

## Departures from the mock-up

- **Large rewrites:** the LCS matrix is X×Y. If `X.length * Y.length > 4e6`, `diffWords` emits one removal plus one addition, and `lcsPairs` returns no pairs, making the whole text one conflict.
- **New draft meets an existing entry:** the mock-up's `saveEntry` crashes when `base` is undefined. Treat a missing base as empty text.
- **`beforeunload` guard** on a dirty draft, since real links are full page loads.
- **Restore carries `base_revision`.**
- **Oversized merges:** a merged body can exceed 20,000 characters. The 400 must keep the merge screen open.

## Edge cases

| Case | Behaviour |
|---|---|
| Entry deleted while editing | 409 `entry_deleted`. Keep the draft, show a notice, reset base and `entry_id`, so saving creates a new entry with this user as author |
| Entry deleted and re-created while editing | 409 `entry_replaced`. Merge with an empty base, so everything conflicts and "Keep both" is offered |
| Two people write the first draft | `base_revision:0` against an existing entry returns `stale_base` |
| Player removed while editing | 403 on save. Draft stays, Save disabled, plain notice |
| Session deleted, game deleted | 404. Draft stays |
| Session cancelled while editing | 409 `session_not_played`. Draft stays |
| Login expired while editing | JSON 401 (`middleware/auth.go:197`). Don't redirect from the editor. Show a notice and keep the draft |
| User anonymised | `GetAllUsers` still returns them, `DisplayName()` gives "Deleted user" (`models/user.go:26-29`). Collapse repeats in the byline only |
| DM anonymised | Players can still edit, an admin can delete |
| Guests | Shown under "At the table" (`guestDisplayObject`). They can't author |
| Admin who isn't a member | Reads like any visitor, plus Delete. No Write, Edit or Restore |
| Future or Suggested session | 409 `session_not_played`. The UI offers no Write link |
| Finished or cancelled game | **Assumption:** still editable, since session code doesn't gate on game status |

## Nav fix

The uncommitted changes in `.claude/worktrees/fix-sub-page-nav` go into this PR. Verified there: 31 E2E tests passing and the pages checked in the running app. Bring them across as the first code commit (for example, `git diff` in that worktree and apply to the new one), then remove the old worktree and branch. If they're gone, these are the changes:

- **`static/game.html`:** replace `<a class="game-crumb" ... x-show="!campaign">← All games</a>` with a `<nav class="game-crumb" aria-label="Breadcrumb">` like `calendar.html`: a "Games" link (to `/campaigns#campaign-{id}`, or `/campaigns` with no campaign), plus ` › ` and the title with `aria-current="page"` once loaded. Delete the `modal-close game-close` × link. Pop-ups keep their ×.
- **`static/styles.css`:** remove `.game-close`.
- **`static/change-password.html`:** add the main nav with Profile active, and a "Profile › Change password" breadcrumb, both `x-show="userLoaded && !mustChange"`.
- **`static/change-password.js`:** add `userLoaded: false` and `isAdmin: false`. After `mustChange` is set in `init()`, set `isAdmin` from `user.roles` and `userLoaded = true`.

## Commits

Branch `feat/game-journal`, worktree `.claude/worktrees/feat-game-journal`, branched from up-to-date `main`. PR title `feat/game journal`. The mock-up and this plan are untracked, so copy them into the worktree.

1. `add journal mock-up`: the mock-up, this plan, a row in the `STYLE_GUIDE.md` table
2. `use breadcrumbs on sub-pages`: the nav fix
3. `add journal store`: models, tables, four methods, errors, pruning, both cascades
4. `add journal api`: `api/journal.go`, router dispatch. Extract `playedAttendees` from `api/campaigns.go:274-289`
5. `add journal to game api`: `journal` on each played row
6. `share markdown renderer`: move `renderMarkdown`, add helpers
7. `show journal on game page`: rows, panel block, count, delete-session warning at `game.js:470`, cancelled entries line
8. `add journal entry page`: `journal.html`, `journal.js`, route, title, byline, scribes, at the table, prev/next, Delete
9. `add journal editor`: name, body, preview, tabs, note, `PUT`, `beforeunload`. `?saved=N&merged=1` or `?unchanged=1` handled like `game.js:103`
10. `add journal merge`: on 409, fetch `/revisions`, find the base, run `merge3` and `mergeName`. No conflicts: if the result equals latest, show "already in", otherwise re-`PUT` with `base_revision` and `merged_with` set to latest, looping on another 409. Conflicts: "Combine your changes" screen
11. `add journal history`: revision list, word diff, full text, restore with confirm, pruned notice
12. `seed demo journal entries`: optional `Journal []SeedRevision` on `SeedSession` (`config/config.go:174`), saved in `seedIfEmpty` (`db/store.go:192-211`). Two Age of Sojourn entries, one with several authors
13. `add journal e2e tests`: see below
14. `document game journal`: `CLAUDE.md` gotcha (entries key on session id, numbers are positional, cascades by hand), `STYLE_GUIDE.md` journal section and breadcrumb

**Verify:** `go build ./... && go vet ./...` each step, `npm run test:e2e` at the end, then both themes and phone width in the app. The PR can't merge without the `playwright` check.

## E2E tests

Using `apiAs` and `loginAs` from `e2e/helpers.js`, in a new `e2e/journal.spec.js`:

- **Merge:** non-overlapping saves auto-merge and record `merged_with`. The same paragraph opens "Combine your changes" and the result saves. A name conflict shows up. A stale base returns 409 with `latest`.
- **Permissions:** a visitor reads but gets 403 on write. A player who missed the session can edit. A removed player gets 403 and keeps credit. The DM and an admin can delete, a player can't. An admin who isn't a member can't edit or restore.
- **Sessions that happened:** saving on a future Confirmed session or a Suggested one returns 409 `session_not_played`, and no Write link appears. A by-hand session can have an entry.
- **Restore and no-op:** restore adds a revision and keeps the others. A no-change save creates none.
- **Pruning:** past 200 saves, only the latest 200 remain, numbering continues, saving still works, and the author and an editor whose only revisions were pruned stay credited.
- **Delete:** deleting an entry leaves the session. Deleting a by-hand session removes its entry.
- **Renumbering:** recording an older session changes the title number while the URL still opens the same entry.
- **Nav:** the game page has the breadcrumb and no × in its header. Change password shows nav normally and hides it on a forced change.
