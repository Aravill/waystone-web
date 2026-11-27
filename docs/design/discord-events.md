# Discord events

**Status:** approved design, 2026-11-27. Builds on `docs/design/discord-notifications.md`.

When a session is confirmed, the bot creates a Discord scheduled event in the game's voice channel, with an optional cover image, and the game's "session confirmed" channel post carries the event's link, which Discord shows as an event card with an Interested button. The admin turns this on per game.

## Configuration (admin, `/admin/discord`)

Each game row gets three new controls beside the existing text channel dropdown:

- **Discord events**: on/off, off by default.
- **Voice channel**: a dropdown of the guild's voice channels the bot can see.
- **Event cover**: an image upload (PNG, JPEG or WebP, stored in `UPLOAD_DIR` like map images), with preview and remove. It's optional, and without it Discord shows its default banner.

Rules:
- You can't save events on unless both the **text channel** and the **voice channel** are set (400, and the missing field is highlighted). The cover is optional.
- The per-game toggle doesn't depend on the server-wide "Channel events" checkboxes. If "Session confirmed" posts are switched off server-wide, the event is still created but not announced in the channel.
- Switching to another Discord server clears every game's voice channel and cover and turns its events off, the same way text channels are already cleared. Events already created stay in the old server.
- Changing a game's voice channel or cover only affects events created after the change. Events already scheduled keep their room and image.
- The production bot needs the **Create Events / Manage Events** permission as well as the existing ones.

## Data model

- `campaigns` (ALTER-if-missing): `discord_events` (0/1), `discord_voice_id` and `discord_voice_name` (the name is cached for display), and `discord_event_cover` (the file name in `UPLOAD_DIR`, or `''`). Only the admin route writes these (an extension of `SetCampaignDiscordChannel`, or a sibling), never `SaveCampaign`. They aren't added to `models.Campaign` JSON beyond what the game page needs.
- A Discord event ID is stored per session, e.g. `sessions.discord_event_id`, or a small `discord_events (session_id, guild_id, event_id, status)` table if the outbox needs more. The ID is needed to start, end and delete the event and to link it (`https://discord.com/events/{guild}/{event}`).
- Foreign keys are off, so cascades are by hand (see Lifecycle).

## Rules

### Creating
- A session that becomes **Confirmed** (by confirming a poll date, or by re-confirming a cancelled session whose start is still in the future) creates one event, but only when the game has events on, a text channel and a voice channel, and the game isn't Finished or Cancelled. Manual sessions never get an event.
- **Event:** `entity_type` voice (2), `channel_id` = voice channel, `scheduled_start_time` from `sessionStart`. The name is the game title. The description is the game's summary, trimmed to fit Discord's 1000 characters, followed by the Waystone session link (`notificationTarget` under `PUBLIC_URL`). The image is the cover as a data URI, when one is set. There's no session number, because numbers are positional and only exist for played sessions.
- Discord calls go through the outbox, as always: a new outbox kind (e.g. `event`). While the event is still being retried, the "session confirmed" channel post waits and is queued after the event exists, with the event link added to its text. If creating the event fails for good (retries run out after 6h, or there's a permission error), the post goes out anyway with today's text and no link. The failure shows in the admin page's Outbox failures.

### Starting and ending
- The minute ticker (`StartSessionStartNotifier`, which already sends `session_started`) sets the event to **Active** at the session's start, then to **Completed** at start + duration (start + 4h when the duration is 0).

### Reminder
- The 24h "next session" reminder post adds the event link when the session has one.

### Lifecycle
- **Session cancelled** (cancelling or deleting a Confirmed session): its event is deleted on Discord (or cancelled, if it has already started). The "session cancelled" post works as it does today.
- **Game moved to Finished or Cancelled, or deleted by the admin:** every event of the game that hasn't started yet is deleted. The sessions stay as they are in Waystone, as today.
- **Lost channel:** when Discord answers Unknown Channel (10003) or Missing Access (50001) for the game's text channel or voice channel, the lost channel is cleared, the game's events are switched off, and admins get the existing `discord_channel_lost` notice (its wording says whether the text or the voice channel is gone). The admin turns events back on after fixing it, so there's never a half-configured state.
- Missing Permissions (50013) marks the row failed and keeps the settings, as today.

## Pages and API

- `GET /api/admin/discord` also returns each game's `events`, `voice_channel` `{id, name}` and `event_cover`, plus the list of voice channels (a `PostableChannels`-like call that filters for type 2 and the View/Connect permissions).
- `PUT /api/admin/discord/games/{id}` also takes `events` and `voice_channel_id`, and refuses events on without both channels.
- Cover: `POST` / `DELETE /api/admin/discord/games/{id}/cover` (raw body, the same checks as other image uploads, and Discord's size limit in mind; around 8 MB is safe).
- Discord client: create, modify (status) and delete a guild scheduled event (`/guilds/{g}/scheduled-events[/{id}]`).
- **Game page:** when events are on, the read-only "Discord: posts to #name" line becomes "Discord: posts to #name · events in 🔊Voice". The DM gets no controls.

## Scope extras

- **E2E:** extend the Discord spec and `e2e/fake-discord.js` with the scheduled-event routes. In the test, the admin turns on events for a game (voice channel and cover set), the DM confirms a session, and the fake receives an event with the voice channel and image, and a post containing its link. Then the DM cancels, and the event is deleted. Starting and ending by the ticker is checked by hand.
- No env vars. No demo seed.

*Later:* updating scheduled events when the voice channel or cover changes; external events for games without a voice channel; adding the event cover to the game card.
