# Discord notifications

**Status:** approved design, 2026-11-22. First Discord iteration: notifications and reminders only.

Waystone posts game announcements into a Discord channel per game and mirrors each user's inbox into their Discord DMs. One bot talks only to Discord's HTTP API: there's no gateway connection, no slash commands and no buttons. The inbox stays the source of truth, and Discord is just another delivery channel.

## Configuration

- **Env (secrets):** `DISCORD_BOT_TOKEN`, `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET` (OAuth linking). `PUBLIC_URL` (e.g. `https://waystone.aravill.org`) builds absolute links in messages and the OAuth redirect `{PUBLIC_URL}/auth/discord/callback`, because the background sender has no request to take a host from. `DISCORD_API_BASE` is optional, defaults to `https://discord.com/api/v10`, and is only set by the E2E fake. All of them are optional and listed in `.env.example` and `docker-compose.yml`.
- **Feature on** means `DISCORD_BOT_TOKEN` is set. When it's off, the Profile section, outbox writes and the game page line are all hidden or skipped. `/auth/current-user` sends `discord_enabled`. `/admin/discord` still shows, explaining which env vars to set.
- **Admin page `/admin/discord`** (admins only) holds everything else:
  - **Server** (guild ID). Switching to another server clears every game's channel first. The page warns "N games will lose their channel" and asks for confirmation. User links are Discord accounts, not tied to a server, so they stay.
  - **Invite link** (shown on Profile).
  - **Channel events:** one list of checkboxes for all games: *Session confirmed*, *Session cancelled*, *New poll posted*, *Session reminder*, *Signups opened*, *Game status changed*. All are on by default.
  - **Games → channel:** every game (Pitch, Ongoing and On Hiatus first, then Finished and Cancelled) with a dropdown of the server's text channels the bot can post in, or *none*, plus a **Send test** button per game.
  - **Outbox:** queued count and the last 20 failures, with error and time.

## Data model

- `campaigns.discord_channel_id` and `campaigns.discord_channel_name` (ALTER-if-missing, default `''`). Only the admin route writes them (`SetCampaignDiscordChannel`), never `SaveCampaign`, the same way `codex_id` works. The game API sends `discord_channel` `{id, name}` (the name is cached when the admin sets the channel) so the game page can show "Posts to #name" read-only.
- `app_settings (key PRIMARY KEY, value)`: `discord_guild_id`, `discord_invite_url`, `discord_channel_events` (JSON list of event keys).
- `discord_links`:

| Column | Meaning |
|--------|---------|
| `user_id` | primary key |
| `discord_user_id` | unique: one Discord account links one Waystone user |
| `discord_username` | shown on Profile, refreshed on each link |
| `dm_channel_id` | cached from `POST /users/@me/channels` |
| `dm_sessions`, `dm_signups`, `dm_characters` | 0/1 toggles, all 1 on link |
| `dm_error`, `dm_error_at` | last refused DM. Cleared by the next successful DM |
| `linked_at` | |

- `discord_outbox`:

| Column | Meaning |
|--------|---------|
| `id` | |
| `kind` | `dm` or `channel` |
| `target` | user ID (`dm`) or channel ID (`channel`) |
| `campaign_id` | `''` for no game. Used to drop rows when a game or its channel goes |
| `payload` | JSON Discord message (content plus one embed) |
| `attempts`, `next_attempt_at`, `last_error` | |
| `status` | `queued`, `failed` |
| `created_at` | |

  Sent rows are deleted, and failed rows are kept for 7 days for the admin page.
- Foreign keys are off. `AnonymizeUser` deletes the user's `discord_links` row and queued DMs. `DeleteCampaign` deletes the game's queued outbox rows.

## Rules

### Linking (Profile)
- The Profile section *Discord* reads "Connect Discord" when not linked, which starts OAuth (scope `identify`). A random `state` is kept in the session for CSRF. The callback stores the link and returns to Profile.
- If the Discord account is already linked to another Waystone user, linking is refused with "This Discord account is linked to another Waystone account." Nothing moves.
- When linked, Profile shows "Connected as @name", **Disconnect**, the three toggles and **Send test DM**.
  - When Profile loads, the server asks Discord whether the user is in the guild (`GET /guilds/{g}/members/{u}`, one call, nothing stored). It shows "in the server", or "join the server to get DMs" with the admin's invite link.
  - With `dm_error` set: "Discord can't reach you: rejoin the server or allow DMs from server members", and the time of the error. DMs keep being tried.
- Disconnect deletes the link row and the user's queued DMs.

### DMs (mirror of the inbox)
- `notify()` stays the only entry point. After writing the inbox rows, it queues one `dm` outbox row per recipient who has a link, isn't blocked, and has the type's group toggled on. The text is rendered in Go (`discordText`, mirroring `notifText` in `static/notifications.js`) at queue time.
- Groups:
  - **Sessions & polls:** `session_suggested`, `session_confirmed`, `session_cancelled`, `session_started`, `session_reminder`, `poll_reminder`, `poll_complete`, `poll_posted`, `poll_skipped`.
  - **Sign-ups & players:** `signup_created`, `signup_accepted`, `signup_declined`, `signup_withdrawn`, `player_kicked`, `player_left`, `campaign_full`, `signups_opened`, `campaign_status`.
  - **Characters:** `character_status`, `character_assigned`, `character_cleared`.
  - `discord_channel_lost` (admins only) is always sent.
- Session times use Discord timestamps `<t:UNIX:F>` (`<t:UNIX:R>` alongside), from `sessionStart`. A session without a parseable start (manual, `time=''`) shows its date as plain text.
- Every message links to the relevant Waystone page (game, calendar month or inbox) under `PUBLIC_URL`. Without `PUBLIC_URL` there are no links.

### Channel posts (per game)
- These are separate from the inbox: one post per event per game, queued at the same call sites, only when the game has a channel and the event is ticked on the admin page. Posts have no mentions.
- **Session confirmed** and **Session cancelled**: only a Confirmed session being cancelled. Removing a poll date or withdrawing a poll doesn't post.
- **New poll posted**: a new poll, by hand or by a repeating schedule, listing every date. Adding dates to an existing poll doesn't post.
- **Session reminder**: see below.
- **Signups opened**, and **Game status changed** (Pitch → Ongoing → Finished and so on).
- Once a game is Finished or Cancelled, only the status change itself posts. After that, nothing else posts.
- Manual sessions never post.

### Session reminder (new type `session_reminder`)
- The minute ticker (`StartSessionStartNotifier`) also sends `session_reminder` once, 24h before a Confirmed, non-manual session of a game that isn't Finished or Cancelled. It goes to the players and followers, but not the DM (like `session_started`), so it reaches their inboxes and DMs, plus one channel post.
- The `notifications` table is the dedupe. The channel post is queued once per session in the same pass that writes the inbox rows. If no one is notified (no followers), a `session_reminder` row with `user_id ''` marks the session as done.
- A session confirmed less than 24h before it starts gets no reminder: the confirmation was the notice. The inbox UI gets a wording for the new type.

### Sending (outbox worker)
- A goroutine started from `main.go` drains `discord_outbox` in `created_at` order. It honours Discord's rate-limit headers (`X-RateLimit-*`, 429 `retry_after`, global limit) and retries 5xx and network errors with backoff for up to 6 hours, then marks the row `failed`. It wakes on new rows and every 30 s.
- A DM refused by Discord (50007, cannot send to this user) isn't retried: it sets `dm_error` on the link and marks the row `failed`. Any later successful DM clears `dm_error`. A 404 on the cached DM channel re-creates it once.
- A channel answering Unknown Channel (10003) or Missing Access (50001) clears `campaigns.discord_channel_id` and drops that game's queued channel rows. It then sends a `discord_channel_lost` notification to every admin ("{game}'s Discord channel is gone", linking to `/admin/discord`). The game's DM isn't told.
- Missing Permissions (50013, e.g. Embed Links removed) marks the post failed but keeps the channel.
- **Send test DM** (Profile) and **Send test** (admin) call Discord directly, not through the outbox, so errors show at once.
- A session counts as confirmed at its `updated_at` (there is no `confirmed_at`), for the "less than 24h" rule.
- If Discord is off (no token), nothing is queued.

## Pages and API

- `GET /auth/discord/start` and `GET /auth/discord/callback` (auth-wrapped).
- `GET /api/profile/discord` returns link, membership, error and invite. `PUT` updates the toggles, `DELETE` disconnects, and `POST .../test` sends a test DM.
- `/admin/discord` (page) uses `GET /api/admin/discord` (settings, games with channels, channel list, outbox summary). `PUT /api/admin/discord` takes the guild, invite and events. `PUT /api/admin/discord/games/{id}` takes `{channel_id}`, and `POST /api/admin/discord/games/{id}/test` posts a test message. The admin nav gets a Discord link.
- The game page shows "Discord: posts to #name" read-only when a channel is set and Discord is on.

## Scope extras

- E2E: a tiny fake Discord server in `e2e/` (pointed to by `DISCORD_API_BASE` and a dummy token). One spec: the admin sets the guild and assigns a channel, the DM confirms a session, and the fake receives the channel post and a linked player's DM. The link goes through the real OAuth start and callback against the fake (no test-only path). Only the Discord-side consent screen is tested by hand.
- No demo seed for Discord (it needs a real bot).
- The production Discord app needs the bot invited to the server with *View Channels*, *Send Messages* and *Embed Links*, and the redirect URI registered.

*Later:* slash commands (`/agenda`, `/link`), buttons to answer polls from Discord, a DM-chosen reminder lead time, role mentions, DM self-service channel picking.
