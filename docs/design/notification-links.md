# Notification links

**Status:** approved design, 2026-11-23.

Every notification, in the inbox and in Discord, links to the place where the event happened, and that page opens focused on it.

## One place for the link

- `notificationTarget(n)` in `api/notification_links.go` gives a notification's path and the label of its Discord button. The inbox payload (`url`), the Discord DMs and the channel posts all use it, so they can't drift. Nothing is stored, so old notifications get the new links too, as far as their `data` allows.
- `notificationURL(n)` is the path with `notif={id}` added. It is what the inbox and the DMs use. Channel posts belong to nobody, so they use the plain path.
- A link never depends on the viewer. The target page decides at click time what that person can see.

| Notification | Opens | Discord button |
|---|---|---|
| `session_suggested`, `poll_reminder`, `poll_complete`, `poll_posted` | calendar, the poll's day selected | Open the poll |
| `session_cancelled` of a suggestion or poll | calendar, on its first date, with a notice | Open the calendar |
| `poll_skipped` | calendar, on the first date it skipped | Open the calendar |
| `session_confirmed`, `session_started`, `session_reminder` | calendar, the session's day selected | Open the session |
| `session_cancelled` of a confirmed session | calendar, that day, where it shows as cancelled | Open the session |
| `signup_created`, `signup_withdrawn`, `player_left`, `campaign_full` | Sign-ups dialog of the game (`/campaigns?signups={id}`) | Open sign-ups |
| `signup_accepted`, game takes characters | game page with the character chooser (`?character=1`) | Choose your character |
| `signup_accepted` otherwise, `signup_declined`, `player_kicked`, `campaign_status`, `signups_opened` | game page | Open the game |
| `character_assigned`, `character_status` | the character | Open the character |
| `character_cleared` | game page, company list (`?company=1`) | Open the game |
| `discord_channel_lost` | `/admin/discord?game={id}`, that game's channel select focused | Pick a channel |

There are no follow, account or codex notifications today. An unknown type opens its game, or Home without one.

Channel posts use the same function: session confirmed, cancelled and reminder as their notifications, a new poll as `session_suggested`, sign-ups opened and status changed as theirs.

## Target pages

- **Calendar** (`/games/{id}/calendar`) takes `poll`, `session` and `date`. A poll opens on its first upcoming date (the panel shows the whole poll). A settled poll opens on the session it became. `date` gives the month and is the fallback.
  - A poll that is gone: the calendar on `date` with "This poll is no longer open."
  - A session that is gone: "This session is no longer on the calendar."
  - A cancelled session is still on the calendar, marked cancelled.
  - Someone who is neither the DM nor a player (a follower, a removed player, somebody in the Discord channel) is sent on to the game page with the same `poll` or `session`.
- **Game page** takes `poll`, `session` (from the calendar), `company` and `character`, and drops them from the address bar once handled. A session that the viewer can see is selected in the panel. Otherwise: "Only the game's DM and players can see its polls." or "That session isn't shown here. It may have been cancelled, or only the game's DM and players can see it."
- **Deleted game or character:** the page already says it may have been deleted. The sign-ups dialog of a game that is gone or no longer yours shows the existing "Game not found or you can't open that dialog".
- Everything is in the query string, never after `#`, so a logged-out click survives the login (`?next=`).

## Read state

- Inbox rows are real links, so Ctrl and middle click open a new tab, and they work from the keyboard.
- Opening the link marks the notification read, however it was opened (click, new tab, Discord DM): `static/notifications.js` sees `notif={id}` in the address, posts the read and removes it from the address bar. Someone else's id is a quiet 404.

## Discord

- A message is its text and one link button (component type 2, style 5) named in the table. It replaces the "Waystone" embed. Link buttons need no gateway or interactions endpoint. Without `PUBLIC_URL` there is no button.
- Each DM has its own link because of `notif`. Messages queued before the change are sent as they are.

## Scope extras

- E2E: one spec for the in-app links and one check of the Discord buttons against the fake.
- No config, schema or seed change.
