# Jukebox

**Status:** built 2026-11-27 (`api/jukebox.go`, `api/jukebox_webm.go`, `static/jukebox.*`, the header part in `static/notifications.js`; see CLAUDE.md).

**Decisions made while building:**
- The server re-wraps the stream instead of passing it through: every flush of the DM's chunks becomes one self-contained known-size Cluster with the source Timecode, so a late joiner simply starts at the next one (at most ~250 ms later) and nothing is special-cased. The init segment drops SeekHead, Cues, Tags and Void and always has an unknown-size Segment.
- `ended` has a fourth reason, `deleted`, so a pop-up can say the game is gone. There is no `stopped`: when the DM stops, listeners stay connected and see Off air.
- Following is always explicit (being accepted adds no follow row), so a kicked player who had also followed the game keeps listening as a follower.
- No handler can change a game's DM today, so that recheck has no call site yet.
- The pop-up checks access through `GET /api/campaigns/{id}` before connecting, because a browser can't read the status of a refused WebSocket handshake.
- A game counts as live (header, game page) once the server has the stream's init segment. The header refreshes on the inbox's 60 s timer, so a page already open can lag up to a minute.
- From the header, Listen on another game while broadcasting asks first, since it replaces the DM's pop-up.

A game's DM streams music (usually Spotify) from their browser through Waystone, and the game's players and followers listen in a small pop-up window. This replaces playing music through a Discord screen share, because Discord mutes Spotify while the stream is on. Nothing about it is stored in the database. A stream exists only in server memory while the DM is broadcasting.

## Model

- **One jukebox per game.** Only the game's **DM** can broadcast. Admins can't take over.
- **Listeners** are the game's **players and followers**. Players follow automatically. The DM doesn't listen to their own stream: their pop-up is the broadcaster view.
- The game must be **Ongoing**. Pitch, On Hiatus, Finished and Cancelled games have no jukebox: the button is hidden and the server refuses connections (404, like an unknown game).
- No new tables or columns, no env vars and no notification types.

## Delivery

- The DM's browser captures audio, runs it through Web Audio (a gain node for send volume and mute, an analyser for the level meter) and encodes it with `MediaRecorder` as `audio/webm;codecs=opus` at about 128 kbps, in chunks of about 250 ms.
- Chunks go over a **WebSocket** to the Go server (`github.com/coder/websocket` is already a dependency), and the server fans them out to listeners over their own WebSockets. Delay is about 1 to 3 seconds. That's fine for background music, and nothing new is needed in Caddy, Docker or the ports.
- The server keeps a small EBML reader per stream so a listener can join mid-stream: it stores the **init segment** (EBML header, Segment and Tracks, everything before the first Cluster) and follows block boundaries in both unknown-size clusters (Chrome) and known-size ones (Firefox). A late joiner gets the init segment, then the next re-wrapped Cluster (see the decisions above). Audio blocks are all keyframes, so they start at the live point; the listener seeks to the start of its buffer once.
- Listeners play through Media Source Extensions (`audio/webm; codecs="opus"`). If the buffer falls more than about 3 seconds behind live, they skip ahead, and old buffered data is trimmed.
- **Restarts:** every new broadcast (a reconnect after a dropout, or a takeover from a second window) has a new init segment. The server tells listeners "new stream", and they reset their source buffer.
- The WebSocket handlers must clear the `http.Server` read and write deadlines (30 s and 60 s in `main.go`) before upgrading, through `http.NewResponseController`, or long streams get cut. Ping and pong keep dead peers from lingering.
- Bandwidth is about 128 kbps per listener from the server. No listener cap.

## Audio sources (DM)

- **Share a tab or screen** (Chrome and Edge only): `getDisplayMedia` with audio. A Spotify web-player tab works, or the whole screen with "Share system audio" on Windows. The video track is stopped right away.
- **Audio input** (any browser, Firefox included): `getUserMedia` with a picked input device, with echo cancellation, noise suppression and auto gain turned off. Examples are a virtual cable (VB-Audio Cable) or Windows "Stereo Mix" set to carry Spotify.
- The start screen offers both. It hides or explains "Share a tab or screen" where the browser can't capture audio (Firefox), and says what each option needs.
- Changing the source means Stop, then Start again.
- Broadcasting is desktop only.

## Pages

### The pop-up: `/games/{id}/jukebox`
- `jukebox.html`, served from the existing `/games/` route (auth-wrapped). It's opened with `window.open(url, 'waystone-jukebox', 'popup,width=…,height=…')`, so there's one jukebox window per browser, and listening to another game replaces it.
- Someone who isn't the DM, a player or a follower, or a game that isn't Ongoing, gets the usual not-found handling.
- **DM view:** source choice and Start. While live, it shows the level meter, a **send volume** slider (so the music sits under voice chat for everyone), **Mute** (sends silence, listeners stay connected and see "Paused") and **Stop** (ends the broadcast). Closing the window also stops it. Below that is the **list of who's listening** (avatar and display name, live).
- **Listener view:** the game title, the state (Live, Paused, or Off air with "waiting for the DM"), a volume slider and a mute button, and the same who's-listening list with the DM shown as broadcaster. Volume and mute are remembered **per device** in localStorage (`waystone-jukebox-volume`, one value for every game).
- Browsers block autoplay, but opening the pop-up is a click, so listening starts right away. If a browser still blocks it, the window shows a Play button.
- Phones: Chrome and Firefox on Android work. iPhone Safari gets "Listening isn't supported in this browser" rather than something half-working.

### Header icon (every page with the user menu)
- A jukebox icon next to the inbox. It's **hidden unless** one of the user's games (as DM, player or follower) is live, or their own pop-up is open.
- It's fed by `GET /api/jukebox/live`, polled on page load and then on the inbox's timer (only while the tab is visible). It returns the live games the user can broadcast to or listen to: `campaign_id`, `title`, `role` (`dm` or `listener`), `paused` and the listener count.
- Click opens a small menu listing the live games with **Listen** (opens or switches the pop-up), and, for the game the pop-up is playing, volume, mute and Stop. The DM's own live game shows "Broadcasting" and opens their pop-up.
- The header talks to the pop-up through a `BroadcastChannel` (`waystone-jukebox`). The pop-up announces its game, role, state and volume, and the header sends volume, mute and stop commands. If no pop-up answers, the header shows only the live list.

### Game page
- On an Ongoing game, the DM sees **Start jukebox** ("Jukebox: live" while broadcasting), and players and followers see **Listen**, with a Live dot when on air or greyed "Off air" otherwise. Both open the pop-up. The game API adds a `jukebox` object (`live`, `paused`) for the page. No live marks on game cards or Home.

## Rules by flow

- **Going live** notifies nobody: no inbox notification, no Discord DM, no channel post. The header icon and the game page show it.
- **Dropouts** (pop-up crashed, laptop asleep, network blip, server restart): listeners' pop-ups stay open on "Off air: waiting for the DM" and resume by themselves when the stream comes back. The DM's pop-up reconnects with backoff and restarts its recorder on its own. Closing the DM's pop-up or pressing Stop is the real stop.
- **Second broadcaster window** (another tab or computer, same DM): the newest broadcast takes over. The old pop-up shows "Broadcasting from another window" and stops capturing, and listeners hear a short gap.
- **Losing access, right away:** the server closes a listener's stream at once, and their pop-up says "You no longer have access to this jukebox" with no auto-resume, when they:
  - are kicked or removed by the admin, or leave the game;
  - unfollow while not a player;
  - are blocked or anonymized.

  The game leaving Ongoing (status change) or being deleted ends the **DM's broadcast too**, and every pop-up says why. Each of those handlers calls one hook (e.g. `jukeboxRecheck(campaignID)`, or a user-wide one for block and anonymize) that re-checks everyone connected to that game. The DM changing (the admin edits the game) ends the old DM's broadcast through the same recheck.
- **Joining** is at the live point. There's no history and no replay.

## API

- `GET /api/jukebox/live`: the header's list (see above).
- `GET /api/campaigns/{id}/jukebox/ws?role=dm|listen`: the WebSocket. The DM sends binary chunks plus small JSON text messages (`{"muted":bool}`). The server sends listeners binary media plus JSON text messages (`stream` (new init follows), `state` (live, paused, off air), `listeners` (the list), `ended` with a reason: `stopped`, `taken_over`, `no_access`, `not_ongoing`). Access is checked on connect, and again through the recheck hook.
- The game payload gets `jukebox {live, paused}`.

## Scope extras

- **E2E (Playwright, Chromium):** launched with `--use-fake-device-for-media-stream --use-fake-ui-for-media-stream`, so the DM's "audio input" is a generated tone. The DM starts the jukebox, and a player's pop-up shows Live and the DM sees them in the listener list. The player's header icon appears with the game listed, the DM's Mute shows "Paused" to the player, and kicking the player shows them "no longer have access". It doesn't check the audio itself.
- No seed data (live state isn't stored), no `.env.example` changes.
- Check the listener and the input-device broadcaster in Firefox by hand (Michal's browser).

*Later:*
- A "now playing" line, if wanted (decided against for now: Waystone can't read Spotify).
- iPhone Safari listening.
- Spotify's terms don't allow rebroadcasting. This is the same grey zone as the Discord screen share it replaces, so it's for the table's private use only.
