# Characters (baseline)

Players keep a roster of characters and assign one to each game they play in.
No character sheets yet: a character is a name, a description and a background.
Decisions below came out of the design grilling on 2026-11-07.

## Data model

`characters`
- `id`, `owner_id` (user), `system_id` (a row in `systems`; known systems only),
  `name` (required, ≤64), `description` (optional, ≤10000, basic Markdown),
  `background` (optional, backstory, ≤10000, basic Markdown),
  `status` (`alive` | `retired` | `dead`, default `alive`), `created_at`, `updated_at`.

`character_assignments` (history is never dropped by leaving or swapping)
- `id`, `character_id`, `campaign_id`, `user_id` (the owner at assignment time),
  `started_at`, `ended_at` (`''` = active).
- At most one active assignment per `(campaign_id, user_id)`: partial unique index
  `WHERE ended_at = ''`.

Foreign keys are off, so cascades are by hand (see Lifecycle).

## Rules

### Which games accept characters
- A game accepts characters only when its `system` text matches a known system
  (case-insensitive name, as `canonicalSystemName` does). Blank or custom-text games
  show "Characters need a known system. Ask an admin to add it." and no nudge.
  When an admin later adds that system, the game starts accepting characters.
- A character fits a game when `character.system_id` = the game's resolved system id.
- Only accepted players get characters. The DM never does (no DMPCs).

### Assigning (game page)
- Assignment happens after acceptance (not at sign-up); admin-added and existing
  players do the same.
- Characters are optional, with a nudge: an accepted player in a game that accepts
  characters, with status not Finished/Cancelled and no active assignment, sees
  "Choose your character" on the game card, the game page and the Home agenda.
- The picker is a dialog on the game page (nudge / "Change character" button). It lists
  the player's characters in the game's system (alive first; any status can be
  picked), shows each one's status and other games, and offers "+ New character",
  which creates one inline with the system locked to the game's.
- One active character per player per game. Swapping is free: the old assignment
  ends (kept as history), the new one starts. The DM is notified.
- A player may clear their character (back to TBD). The DM is notified.
- Rejoining after leaving: the picker pre-selects the character from that game's most
  recent ended assignment. Nothing is restored automatically.

### History and badges
- Leaving, being kicked, swapping or clearing ends the assignment (`ended_at`); nothing
  is deleted. Every assignment shows as a game badge on the character: active ones
  normal, ended ones dimmed. A finished or cancelled game keeps its active assignment,
  and the badge reflects the game's status.
- Chronicle: played-session attendees show the character active on that session's date.
  An assignment covers date `d` when `date(started_at) <= d` and (`ended_at = ''` or
  `date(ended_at) >= d`). If several cover it (a same-day swap), the latest
  `started_at` wins. Sessions before any assignment show no character. Derived per
  request; nothing stored per session.

### Status
- Status is only a label: it never ends assignments and never limits picking.
- The owner edits everything. The DM of a game the character is **actively**
  assigned to may change **status only**. Admins may change status on any character.
  Whenever someone other than the owner changes status, the owner is notified.

### System changes
- Character system: the owner may change it unless the character has an active
  assignment (409: "Kael is playing in Curse of Strahd (D&D 5e). Remove it there
  first."). History-only characters may change, so old badges can name games in
  another system. *Later:* once characters hold system-specific data (sheets),
  changing the system must warn that the data will be deleted.
- Game system: the DM's edit is blocked (409) if it would change the resolved system
  while the game has any active assignment. Case-only changes still resolve to the
  same id and are fine.
- Admin deleting a system: blocked if any campaign uses its name (case-insensitive)
  or any character uses its id: "This system is in use by N games and M characters.
  Migrating systems will come later." Renames are free (characters key on the id).

### Lifecycle
- Deleting a character: blocked while it has an active assignment ("Unassign it
  from Curse of Strahd first."). Otherwise the character and all its (history)
  assignments are deleted.
- Deleting a campaign (admin): its assignments are deleted by hand alongside the rest.
- Deleting an account (anonymize): characters and assignments stay, and the owner
  shows as "Deleted user".

### Visibility
- Every logged-in user can read every character, like profiles and game pages.

## Notifications (inbox)
- `character_assigned` → DM: "Roy now plays Kael in Curse of Strahd" (also on swap).
- `character_cleared` → DM: the player removed their character.
- `character_status` → owner, when a DM or admin changed the status.
- The existing `signup_accepted` text adds "Choose your character" (only when the game
  accepts characters). No new type.

## Pages and API
- Nav item **Characters** on every page with a nav.
- `/characters`: your own list, with create/edit/delete and status.
- `/characters/{id}`: one character for everyone: name, status, system (with logo),
  owner, Markdown description and background, game badges (linking to `/games/{id}`).
  Editable in place by the owner; status control for an active-game DM or admin.
- Profile: a read-only "Characters" section linking to `/characters/{id}`.
- Game page company panel: each player row shows their current character (linking to it),
  or "character TBD" when there's none.
- API (all behind `AuthMiddleware`):
  - `GET /api/characters?user=ID` (default: you), `POST /api/characters`
  - `GET|PUT|DELETE /api/characters/{id}`, `PUT /api/characters/{id}/status`
  - `PUT /api/campaigns/{id}/character` `{character_id}` (assign/swap),
    `DELETE /api/campaigns/{id}/character` (clear)
  - The campaign and agenda payloads add the current user's assignment, a
    `character_nudge` flag and per-player characters for the company panel.

## Scope extras
- `SEED_DEMO_DATA` seeds a few characters in different statuses, some assigned.
- One Playwright test: create a character, assign it on the game page, then see the
  badge and the company panel entry.
