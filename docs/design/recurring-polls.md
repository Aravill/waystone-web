# Recurring polls

**Status:** approved design, 2026-11-22. Builds on session polls (`session-polls.md`, live in v0.27.0).

A DM who runs a recurring game sets a poll to **repeat**. The app then posts a new poll with the same days and times every N weeks, a fixed number of days ahead, without the DM doing anything. Confirming stays with the DM.

## Data model

New table `poll_schedules`, at most one row per game:

| Column | Meaning |
|--------|---------|
| `campaign_id` | primary key |
| `every_weeks` | 1–4 |
| `lead_days` | how many days before a cycle's first date its poll is posted, 7–28 (default 14) |
| `timezone` | the source poll's timezone |
| `slots` | JSON list of `{date, time, duration}`: the source poll's dates at the moment Repeat was turned on (the **anchor** cycle) |
| `next_cycle` | the number of the next cycle to handle (cycle *k* = every slot date + *k* × `every_weeks` × 7 days); starts at 1 |
| `paused` | 0/1 |
| `created_at`, `updated_at` | |

- Dates are shifted as calendar dates (`YYYY-MM-DD` + days) in the slot's own timezone, so a 19:00 session stays 19:00 across DST changes.
- A cycle's **first date** is its earliest slot date and time. Its **post time** is that start minus `lead_days`.
- Automatic polls are ordinary polls (`poll_id`, Suggested sessions). Nothing marks them as automatic.
- Foreign keys are off: deleting a game deletes its schedule by hand.

## Rules

### Turning Repeat on (DM only, open polls only)
- An open poll's card (calendar and game page) has **Repeat…** for the DM. It opens a small dialog: "Every [1–4] weeks", "Post [7–28] days ahead" (default 14), and a hint that the pattern is the poll's current dates. Confirming a poll removes its other dates, so the card says to set Repeat up before confirming. A confirmed session has no Repeat.
- The pattern is a **snapshot**: the poll's upcoming dates at that moment. Later edits to that poll (adding or withdrawing dates, confirming one) don't change it.
- Turning Repeat on from another poll **replaces** the game's schedule, after a confirm ("This replaces the current schedule: every week, Fri 19:00 · Sat 14:00").
- Turning it on runs the posting check at once: every cycle already due is posted immediately (with a long lead time several can go out at once), and later cycles as they come due.

### Posting (the minute ticker)
- The existing session-start ticker also runs `postDuePolls(now)`. For each schedule, while the next cycle's post time has passed:
  - If the game isn't **Ongoing**, or the schedule is **paused**, the cycle is skipped silently.
  - A slot whose date already has a planned session (Suggested or Confirmed), or whose start has passed, is left out. The rest are posted as one poll, with the game's current DM as the proposer.
  - If no slot is left, nothing is posted and the DM gets "cycle skipped".
  - If the game would pass 999 sessions, nothing is posted and the DM gets "cycle skipped".
  - Either way `next_cycle` moves on, so a cycle is handled exactly once. Restarts don't repost.
- A game coming back to Ongoing (or a schedule resumed) carries on with the next cycle as it comes due. Missed weeks are not caught up.

### Changing and stopping
- **Withdrawing** an automatic poll skips that cycle. It is never posted again.
- **Pause / Resume** stops and restarts posting. Cycles that come due while paused are skipped.
- **Edit** (from the game panel line): every N weeks and days ahead can be changed; the pattern can't (turn Repeat on from another poll for that). Changing them keeps `next_cycle`; with a new `every_weeks` the next cycle is the first one whose first date is still ahead.
- **Stop** deletes the schedule. Polls already posted stay.
- The schedule belongs to the game, not to a person: if the game's DM changes, polls are posted under the new DM.

## Notifications

| Event | Who | Type | Text |
|-------|-----|------|------|
| Automatic poll posted | players | `session_suggested` (as today, actor = the DM) | "Aravill suggested 3 dates for "X": …" |
| Automatic poll posted | the DM | new `poll_posted` (no actor) | "Your repeating poll for "X" was posted: Fri, Dec 11 · Sat, Dec 12" |
| Cycle skipped (no free date, or the session cap) | the DM | new `poll_skipped` (no actor) | "No poll was posted for "X": Fri, Dec 11 and Sat, Dec 12 already have sessions" |

- Skips caused by pausing, a non-Ongoing game or a withdrawn poll send nothing.
- The reminder and "everyone replied" work on automatic polls as on any poll.
- Players never see that a poll was automatic.

## Pages

- **Poll card** (calendar day panel and game page panel), DM only: a "Repeat…" button beside "+ Add a date". The dialog is an S-width form modal: Every (select: every week, every 2 weeks, every 3 weeks, every 4 weeks), Post (select: 7, 10, 14, 21, 28 days ahead), the pattern listed ("Fri 19:00–23:00 · Sat 14:00–19:00, CET"), Cancel and "Repeat". With a schedule already set, the dialog says it replaces it.
- **Game page session panel**, DM only, when a schedule exists: one line under the panel, "New poll every week: Fri 19:00 · Sat 14:00. Next posted Mon, Dec 1." With text links Edit, Pause (or Resume) and Stop (Stop asks first). Paused reads "Repeating poll paused." The line is hidden from players and followers.

## API

- `GET /api/campaigns/{id}/poll-schedule` → 200 `{schedule}` or `{schedule: null}`. DM only (403 otherwise). The schedule JSON includes `next_post_at` (RFC3339) and `next_dates` (the next cycle's slot dates) for the panel line.
- `PUT /api/campaigns/{id}/poll-schedule` body `{poll_id, every_weeks, lead_days}` → creates or replaces it from that poll's upcoming dates (409 if the poll has no upcoming date), then runs the posting check for this game. Body `{every_weeks, lead_days, paused}` without `poll_id` edits the existing one (404 if none).
- `DELETE /api/campaigns/{id}/poll-schedule` → stops it.
- Game deletion deletes its schedule. Routes go in `api/router.go`.

## Scope extras

- **E2E (Playwright):** the DM turns on Repeat (every week, 21 days ahead) on a poll whose dates are about 10 days out. The next cycle is already due, so it is posted at once: the test checks a new poll exists with every date shifted by 7 days, the DM has one `poll_posted` notification, and Stop removes the schedule. No test-only hook is needed.
- No seed data and no new env vars.
