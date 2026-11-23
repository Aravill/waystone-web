# Codex user mentions

**Status:** approved design, 2026-11-23.

An editor can mention a user on a codex page. Clicking the mention opens that user's profile.

## Text
- Writers choose by username: typing `@` in the page editor lists users, and picking one inserts `@[Roy](userId)`. The id is the reference, so nothing that happens to a name can break or redirect a mention.
- `@username` typed by hand becomes `@[Roy](userId)` when the text reaches a page: saving, creating (templates included) and filing a quick note. Only a whole username of an existing account counts, after a line start, a space or an opening quote, bracket or emphasis mark. Nothing changes inside `[[links]]`, pictures, code, web links, URLs and `{{list ...}}` lines. Restoring an old revision leaves its text as it was.
- Readers see `@` and the user's current display name. The name in the text is only the name at the time of writing.

## Rules
- Everyone with an account can be mentioned (blocked users too), whatever their codex rights; any logged-in user can already open any profile.
- A deleted account's mention is plain text with the stored name (`@Roy`), no link. Pages and their history are not rewritten.
- Mentions are not `[[links]]`: no backlinks, no graph edges, no missing pages.
- No notifications.

## API
- `GET /api/codices/users` (See Codices): `[{id, username, display_name, label}]`, deleted users left out. `label` is the display name made safe for the text.

*Later:* notifying the mentioned user.
