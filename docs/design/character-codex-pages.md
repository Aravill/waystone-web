# Character codex pages

**Status:** approved design, 2026-11-27.

A character lives in a game world, and the world lives in codices. A character may now
reference any number of codex pages (its own lore page, family, hometown, rivals...).
The coupling is loose: the character keeps its base fields (name, system, status,
description, background), and the codex knows nothing about characters.

This replaces the single free-text `codex_url` field ("Codex page" button).

## Data model

`character_pages`
- `character_id`, `page_id`, `created_at`. Primary key `(character_id, page_id)`.
- Ordered by `created_at` (order added), oldest first. At most 50 per character.
- No stored title: reads join `pages` (current title) and `codices` (codex name).

Foreign keys are off, so cascades are by hand:
- `DeleteCharacter` deletes its rows.
- `DeletePage` deletes the rows of the page it removes. `DeleteCodex` needs no cascade: it refuses while the codex has pages.
- Reads also join on existing pages, so a dead row never shows.

## Rules

### Which pages
- Any codex page the owner can see, in any codex. Not tied to the character's system
  or its games' codices.
- Kind doesn't matter (lore, timeline, map).

### Who edits
- The owner only, like the other base fields. Active-game DMs still only change status;
  admins don't get link editing either.
- Adding or removing a link saves immediately (no Edit mode).
- Adding needs See on the page's codex (`codexRights.canSee`), checked server-side.
  Linking a page that's already linked is a no-op (200).

### Visibility
- The character payload's `codex_pages` is filtered per viewer with the existing
  `codexRights`: a viewer only gets links to pages in codices they can see. Nothing
  leaks, not even titles.
- A viewer with no Codices access gets an empty list, and the section is hidden.
- Lost access: if the owner loses See on a codex, their links into it stay (others who
  can see it still see them; they reappear if access comes back). The owner can't see
  or remove them meanwhile, and they still count toward the 50.

### Migration
- One-off at startup, idempotent: every non-empty `characters.codex_url` whose path is
  `/codices/p/{pageId}...` and whose page exists becomes a `character_pages` row; then
  `codex_url` is cleared. Anything else (codex home, folder, missing page) is dropped.
- The `codex_url` column stays in the table (SQLite) but is no longer read or written;
  `validCodexURL` / `safeCodexUrl` and the form fields go.

### Names
- Character names stay capped at 64 characters (already enforced).
- The game page's "❦ as Kael" (company panel and roster) is one line with an ellipsis;
  the full name is in a `title` tooltip and the link still opens the character.

## Pages and API

- `/characters/{id}`: a **Codex pages** section below the base fields. Each link shows
  the page title (opens `/codices/p/{id}/{slug}`) and its codex name. The owner (with
  Codices access) gets "+ Link a page", a dialog that searches pages they can see, and
  a × per link. Empty state for the owner: "No codex pages linked yet." Hidden for
  others when empty.
- `/characters` create form and the edit form drop the "Codex page" URL field.
- API (behind `AuthMiddleware`, not `codexGate`, so non-codex users still read characters):
  - `GET /api/characters/{id}` adds `codex_pages: [{page_id, title, kind, codex_id, codex_name}]`, filtered per viewer.
  - `POST /api/characters/{id}/pages` `{page_id}`: owner only; 403 not owner, 404 page
    missing or not visible, 409 at 50 links. 201 (200 if already linked).
  - `DELETE /api/characters/{id}/pages/{pageId}`: owner only; 204 (also when not linked).
  - The picker loads `GET /api/codices/titles` (every page the viewer can see) and `GET /api/codices` (codex names) once and filters client-side.

## Notifications
None.

## Scope extras
- `SEED_DEMO_DATA` links a couple of demo characters to seeded codex pages.
- One Playwright test: link a page to a character, see it on the character page, remove
  it; a viewer without access to that codex doesn't see it.

*Later:* a "Characters" backlink list on the codex page.
