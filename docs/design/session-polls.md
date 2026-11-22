# Session polls

**Status:** approved design, 2026-11-22. Mock-up: `mockups/session-polls.html` (variant A was chosen).

Today every suggested session is its own event with its own "suggested" notification. In practice the DM and players are answering one question: *when is the next session?* A **poll** ties several suggested dates together. The DM confirms one date, the others disappear, and players get one notification when the poll is posted and one when it is confirmed.

## Data model

- `campaign_sessions` gets `poll_id TEXT NOT NULL DEFAULT ''`. Every date of a poll is a normal Suggested session row with the same `poll_id`. There is no polls table.
- A single suggestion is a poll of one date (`poll_id` = a new UUID, never empty for new suggestions).
- **Migration:** every existing Suggested session with an empty `poll_id` gets `poll_id = id`, so each becomes its own one-date poll. Confirmed, Cancelled and manual sessions are left alone.
- The poll's posting time is the earliest `created_at` among its dates (used by the reminder).
- A poll's **upcoming dates** are its Suggested dates whose start hasn't passed. Only these count anywhere below.
- A game can have several open polls at once (weekly and bi-weekly games will).

## Rules

### Posting a poll (DM only)
- On the calendar page, **Suggest dates** in the side panel enters picking mode (the existing paste-mode look: a rubric-bordered bar at the top of the panel, `+` on free days, `✓` on picked days, refusal reasons in the bar's note). The bar lists the picked days. The DM can move between months while picking. Esc or Cancel leaves the mode.
- **+ Suggest session** on a selected day enters the same mode with that day already picked.
- **Copy** on a session enters the same mode with the copied time, length and timezone pre-filled. Pasting always makes a **new** poll. There is no separate paste flow any more.
- **Next** opens the **Suggest dates** dialog: one row per picked day (date, Start, Length, remove ×), a shared Timezone, a "Use the first row's time for all" link and "Back to the calendar". A new row takes the first row's time and length. Length is one select (1h, 1h 30m … 8h) in place of the hour and minute spinners. The primary button reads "Suggest N dates".
- One date per day: a day that already holds a planned (Suggested or Confirmed) session can't be picked, nor can past days. A poll needs at least one date.
- The server creates all dates in one transaction, and refuses the whole poll if any date is invalid (past, day taken, duplicate) or the game would pass 999 sessions.

### Changing a poll (DM only)
- **Add a date:** from the poll card, "+ Add a date" enters picking mode with the poll's existing dates shown as taken. New dates join the same poll.
- **Withdraw one date:** deletes that date and its replies. Silent, unless it was the poll's last upcoming date, in which case it counts as withdrawing the poll.
- **Withdraw the poll:** deletes every date of the poll and their replies.
- Dates are not edited in place. To change a time, withdraw the date and add it again.

### Confirming
- The DM confirms one upcoming date. In one transaction that date becomes Confirmed and every other date of the same poll is deleted with its replies, without any notification. Other polls in the game are untouched.
- The two-step confirm warning names what goes: "Confirm Sat, Dec 12? Fri, Dec 11 and Wed, Dec 16 will be removed."
- A date whose start has passed can't be confirmed (409).
- Cancelling a confirmed session later does not bring the removed dates back.

### Replies
- Players reply Yes / Maybe / No **per date**, exactly as today (one response per `(session_id, user_id)`).
- A date whose start has passed takes no new replies (409).

### Past dates
- When a poll date's start passes before anything is confirmed, it drops out quietly. It stays in the month view as a dashed past suggestion, but leaves the poll card, Home and Ahead, and can't be replied to or confirmed. When every date has passed the poll simply ends. Nothing is deleted and no background job is needed for this.

## Notifications

| Event | Who | Type | Text |
|-------|-----|------|------|
| Poll posted | players | `session_suggested` (one, carries all dates) | "Mira suggested 3 dates for "X": Fri, Dec 11 · Sat, Dec 12 · Wed, Dec 16" (one date: today's wording) |
| Date(s) added | players | `session_suggested` with `added=1` | "Mira added a date to the poll for "X": Thu, Dec 17" |
| Date withdrawn | nobody | | unless it was the last upcoming one: then as "Poll withdrawn" |
| Poll withdrawn | players | `session_cancelled`, `was=Suggested` (one) | "Mira withdrew the poll for "X"" (one date: today's wording) |
| Date confirmed | today's recipients (players and followers) | `session_confirmed` (one) | unchanged; the removed dates send nothing |
| Reminder | players who haven't answered every upcoming date | new `poll_reminder` | "The poll for "X" still needs your reply. First date: Fri, Dec 11" |
| Everyone replied | the DM | new `poll_complete` | "Everyone has replied to the poll for "X"" |

- Notifications keep `session_id` (the poll's earliest upcoming date, so the link lands on the calendar at that month) and add `poll_id`, `dates` and `count` to `data`.
- **Reminder:** sent once per player per poll, when the poll's earliest upcoming date is 5 days or less away, and only if the poll was posted more than 5 days before that date (a late poll gets no reminder: its "suggested" notification is enough). The minute ticker of the session-start notifier runs the check, and the notifications table is the dedupe, as with `session_started`.
- **Everyone replied:** whenever a poll goes from incomplete to complete (every current player has answered every upcoming date), the DM is told. Checked after a reply, a date added or withdrawn, and a player joining, leaving or being kicked. It can fire again after a date is added or a player joins and the poll is completed once more. A date that drops out by passing does not trigger the check.

## Pages

### Calendar page (`/games/{id}/calendar`)
- **Month cells:** poll dates keep the dashed suggestion frame and tally. Each open poll with dates in the shown month gets a letter (A, B, … ordered by its first date in that month, assigned per month view, not stored), shown as a small ink tag in its cells, plus "1/3" when the cell is wide enough (narrow cells show the letter only). Hovering or selecting a poll day outlines its sibling days. The cell's `aria-label` and `title` say "Poll A, date 1 of 3". The legend adds the poll tag.
- **Day panel:** selecting a poll day shows the **poll card**: every upcoming date of the poll, the selected one highlighted, each with its time, tally, roll call and the player's Yes / Maybe / No. The DM gets Confirm and Withdraw per date, plus "+ Add a date" and "Withdraw poll". The rubric frame (needs your reply) is on the card while the viewer owes a reply on any date. Strong candidate stays per date.
- The month agenda lists each poll as one row ("Poll A · 3 dates · Dec 11–16").

### Game page
- **Ahead** has one row per open poll ("Poll · 3 dates · Dec 11–16" and the viewer's state, "Your reply?" in rubric while they owe one). Selecting it shows the same poll card in the session panel. Confirmed sessions keep their own rows.
- The compact reply block (≤ 900px) shows the poll the viewer owes a reply on, a row per date.

### Home
- **Awaits your reply:** one card per poll, with the game, "proposed by ♛ X" and a row per upcoming date (date, time, Yes / Maybe / No). It leaves the list once every date is answered. `/api/agenda` returns suggestions grouped by poll.

### Counts
- Unanswered counts are per **poll**, not per date: "✦ A poll awaits your reply", the banner count, the Games page subtitle and button tabs. `GetUnansweredSessions` counts distinct polls with an unanswered upcoming date.

### Unchanged
- Followers never see suggestions. Manual sessions are untouched. The ICS feed keeps listing each suggested date the viewer hasn't declined.

## API

- `POST /api/campaigns/{id}/polls`: body `{timezone, dates: [{date, time, duration}]}` → 201 with the poll (`poll_id` and its sessions). DM only.
- `POST /api/campaigns/{id}/polls/{pollId}/dates`: same `dates` body, adds to an open poll. DM only.
- `DELETE /api/campaigns/{id}/polls/{pollId}`: withdraws the poll. DM only.
- `PUT /api/campaigns/{id}/sessions/{sid}` `{status: "Confirmed"}`: confirms and removes the siblings. `{status: "Cancelled"}` / `DELETE` on a Suggested date withdraws that date (as above).
- `POST /api/campaigns/{id}/sessions` stays working (a one-date poll), but the page no longer uses it.
- Session JSON gains `poll_id`. Routes go in `api/router.go`.

## Scope extras

- **E2E (Playwright):** the DM posts a 3-date poll through picking mode, a player answers, the DM confirms one date, the other two are gone, and the player has exactly one "suggested" and one "confirmed" notification for it. Update any existing tests that use Copy / paste or "+ Suggest session".
- No seed demo data and no new env vars.
