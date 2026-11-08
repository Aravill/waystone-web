# Waystone Web Style Guide

The visual conventions for Waystone Web. Every page and feature follows them. Reference mock-ups live in `docs/design/mockups/`:

| Mock-up | Shows |
|---------|-------|
| `style-sheet.png` | Inks, letters, seals, instruments: the source for this guide |
| `login.png` | Narrow framed form |
| `campaigns-parchment.png` | Header, nav, page title, game card grid (light) |
| `type-icons.png` | The three game-type marks, light and dark |
| `campaigns-candlelight.png` | The same page in the dark theme |
| `calendar.png` | Calendar grid, reply buttons, drawn when the calendar was a modal sheet (its session chips are superseded by `calendar-days.html`) |
| `calendar-days.html` | Calendar redesign: the Ledger day cell, the Roll call panel, the strong candidate frame (variant A and panel 2 are the chosen ones) |
| `game-detail.dc.html` | The game page as a modal sheet: the tale, the chronicle (Ahead cells, Played rows) and the session panel. The sheet and the Ahead cells are superseded by `ux-game-calendar.dc.html`; Played rows and the panel still apply |
| `game-journal.html` | The game journal: the Played rows' entry line, the panel block, the entry page, editor, "Combine your changes" and history (plan: `game-journal-plan.md`) |
| `banners.dc.html` | The banner redesign: three variants, see `BANNER_DESIGN_GUIDE.md` |
| `ux-reading.dc.html` | The reading pass: type scale, IBM Plex Mono, dates, red means you, contrast (board 1, Today against Proposed) |
| `ux-home-games.dc.html` | Home, the Games page sections, the user menu header and the toast (board 2). Its blue `.tag` labels are review notes and never ship |
| `ux-game-calendar.dc.html` | The game page as a normal page, Ahead as ledger rows and the calendar (board 3). Its two-number calendar cells are superseded: the cells keep the four-column tally (see Session replies) |

## Design Philosophy

**"Scriptorium": typewritten text on a manuscript page.**

- A calm parchment page with iron-gall ink, and a single red accent. Red means you: things that need the viewer's action
- Typewriter text (IBM Plex Mono) for everything you read. Old-style display type only for fixed titles, headings and the brand. Dates are never set in the display face: its old-style zero reads like a letter o
- Minimal: square corners, hairline and double rules, no textures, shadows, gradients or glows
- Decoration is typographic and sparse: `§`, `❦`, `✦`, `♛`. Never images of parchment, wax seals or scrolls
- Copy is plain, modern English ("Your reply", "Yes / Maybe / No"). The medieval feel comes from the visuals, never from archaic wording ("thy", "aye", "well met")
- Mobile-first and responsive

---

## Colour

All colours are CSS custom properties on `:root`. Never hard-code a hex value in a component. Use the token.

### Parchment (light, default)

| Token | Name | Hex | Usage |
|-------|------|-----|-------|
| `--bg` | Vellum | `#F2EAD8` | Page background, cards, inputs |
| `--folio` | Folio | `#E6DBC2` | Selected / hover fill, modal backdrop page, raised areas |
| `--ink` | Iron-gall | `#2B241C` | Text, strong rules, primary button fill |
| `--muted` | Faded ink | `#6B5E4B` | Metadata, labels, inactive nav, finished/cancelled |
| `--line` | Hairline | `rgba(43, 36, 28, 0.4)` | Card borders and dividers |
| `--ctl` | Control | `rgba(43, 36, 28, 0.64)` | Control borders: secondary buttons, the Nay button |
| `--rubric` | Rubric | `#8E2A1E` | The accent, reserved for: things needing the viewer's action, text links, the small `§` marks, the Pitch status, destructive actions, errors, the active nav underline |
| `--verdigris` | Verdigris | `#355E52` | Success, Ongoing, accepted, confirmed sessions |
| `--ochre` | Ochre | `#7A5A12` | Tentative, On Hiatus |
| `--shadow` | Shadow | `rgba(43, 36, 28, 0.24)` | Banner drop-shadow colour |

### Candlelight (dark)

The same tokens redefined. Applied with `[data-theme="dark"]` on `<html>`, and by default under `@media (prefers-color-scheme: dark)`. A sun/moon button in the header (`static/theme.js`, loaded non-deferred in `<head>` so it applies before paint) flips the theme and stores the choice in `localStorage` under `waystone-theme`; until the user picks one, the system preference wins.

| Token | Hex |
|-------|-----|
| `--bg` | `#16120E` |
| `--folio` | `#2A231B` |
| `--ink` | `#E8DCC4` |
| `--muted` | `#A8997F` |
| `--line` | `rgba(232, 220, 196, 0.3)` |
| `--ctl` | `rgba(232, 220, 196, 0.5)` |
| `--rubric` | `#D9735C` |
| `--verdigris` | `#7FB09E` |
| `--ochre` | `#D4A94A` |
| `--shadow` | `rgba(0, 0, 0, 0.7)` |

```css
:root {
    --bg: #F2EAD8; --folio: #E6DBC2; --ink: #2B241C; --muted: #6B5E4B;
    --line: rgba(43, 36, 28, 0.4); --ctl: rgba(43, 36, 28, 0.64);
    --rubric: #8E2A1E; --verdigris: #355E52; --ochre: #7A5A12;
    --shadow: rgba(43, 36, 28, 0.24);
}
@media (prefers-color-scheme: dark) {
    :root:not([data-theme="light"]) {
        --bg: #16120E; --folio: #2A231B; --ink: #E8DCC4; --muted: #A8997F;
        --line: rgba(232, 220, 196, 0.3); --ctl: rgba(232, 220, 196, 0.5);
        --rubric: #D9735C; --verdigris: #7FB09E; --ochre: #D4A94A;
        --shadow: rgba(0, 0, 0, 0.7);
    }
}
:root[data-theme="dark"] { /* same values as the media query block */ }
```

### Rules

- One accent. `--rubric` is the only "look here" colour, and red means you: h2 words are ink with only the `§` rubric, avatar letters are ink, DM names are ink with a rubric crown (`♛`), and other players' "no reply yet" is muted. Rubric is the viewer's own awaited reply ("Your reply?"), a text link, the Pitch status, a destructive action, an error or the active nav underline. Verdigris and ochre are only for state (success / tentative), never decoration.
- The disabled look is a dashed outline with muted text, never a fade (`opacity` drops text below 4.5:1).
- Every token pair used for text meets 4.5:1 contrast on `--bg` in both themes. Check any new pairing.
- Inverse elements (primary buttons) use `background: var(--ink); color: var(--bg)`, which flips correctly in dark mode.
- State is never shown by colour alone: pair it with a glyph (`✓ ? ✗ ✦ ● ◐`), a border style (solid / dashed / dotted) or strikethrough.

---

## Typography

Three families, vendored as woff2 under `static/vendor/fonts/` (SIL Open Font License, latin and latin-ext subsets) and declared with `@font-face` at the top of `styles.css`. Pages need no font link, and nothing loads from a third party at runtime.

| Role | Family | Use for |
|------|--------|---------|
| Text | `'IBM Plex Mono', 'Courier New', monospace` (400, 400 italic, 500, 600) | Everything by default: body, labels, buttons, badges, inputs, dates |
| Display | `'IM Fell English SC', serif` | Page titles (h1), card titles, modal titles, section headings, the month name, navigation |
| Blackletter | `'UnifrakturMaguntia', serif` | The "Waystone" wordmark and the drop cap on fixed page titles. Never for a whole word other than the brand, never below 32px |

The drop cap goes only on fixed page titles (the Home greeting, Games, Profile, User Management, Change Password and similar). Never on anything that shows user content or a date: game and journal titles, the calendar's title, selected-day and session panel headings, "The company".

### Scale

Nothing is set below 13px, except the calendar cell's own tally and time (see Session replies). No uppercase and no letter-spacing anywhere (the `❦ ❦ ❦` ornament keeps its spacing): the small caps in titles and navigation come from the display face. Weights are 400, 500 and 600 (no 700).

| Element | Family | Size | Weight | Notes |
|---------|--------|------|--------|-------|
| Wordmark (header) | Blackletter | 34px (28px on phones) | 400 | |
| Page title (h1) | Display | 44px (drop cap 56px in `--rubric`) | 400 | |
| Section heading (h2) | Display | 25px, `--ink`, prefixed with a rubric `§` | 400 | |
| Card / modal title | Display | 27px / 32px | 400 | |
| Navigation | Display | 21px (19px on phones) | 400 | |
| Body | Text | 16px, line-height 1.55 | 400 | |
| Date heading (panels) | Text | 22px | 600 | "Friday, Nov 20" |
| Month header | Display + Text | name 30px; year 18px, `--muted` | 400; 500 | "November" 2026 |
| Meta / small | Text | 14–14.5px, `--muted` | 400 | |
| Game system | Text | 14.5px italic, `--muted` | 400 | |
| Eyebrow / kicker | Text | 14px, `--muted` | 400 | sentence case |
| Badge / status tag | Text | 13.5px | 400 | |
| Form label | Text | 14.5px, `--ink` | 500 | |
| Form hint | Text | 14px, `--muted` | 400 | |
| Button | Text | 14.5px (reply buttons 15px) | 500 | |
| Weekday header | Text | 14px, `--muted` | 400 | |
| Pending notice | Text | 14.5px, `--rubric` | 600 | must still fit the banner's notice row |

On screens ≤ 768px drop h1 to 34px and card titles to 23px.

---

## Layout & Spacing

- **Page width**: content in a centred column, `max-width: 74rem` (1136px of content), `padding: 0 24px` (16px on phones); the header matches it so nothing shifts between pages.
- **Narrow pages** (login, change password, forms): `max-width: 420px`.
- **Spacing scale** (multiples of 4px): 4, 8, 12, 16, 24, 32, 48, 64. Card padding 22–24px, grid gap 28px, page sections 32–40px apart.
- **Card grid**: `grid-template-columns: repeat(auto-fill, min(100%, 360px)); gap: 28px;` (1.75rem), centred. The Games page uses `.container--wide` (`max-width: 74rem`), which fits three per row. It goes to one column on its own on phones.
- **Units**: sizes in this guide are given in px for readability. The CSS writes them in rem (px / 16), with hairline border and outline widths kept in px.
- **Breakpoint**: `@media (max-width: 768px)` for header stacking and type scale-down.
- **Rules**: a `3px double var(--ink)` rule closes the header and modal headers. Hairlines (`1px solid var(--line)`) divide content within cards. A plain `1px solid var(--ink)` separates major sections.
- **Corners**: always square. `border-radius` only on the round unread-count dot.

---

## Components

Keep the existing class names (`.header`, `.nav-item`, `.message`, `.campaign-status-badge`, `.status-*`, `.module-card`, `.danger-zone`, …) and restyle them. Markup changes only where the design needs new structure: the shared header, page titles, and the banner game cards (cloth, headpiece and the notice slot are built in `campaigns.js`).

### Header & navigation

Wordmark left, nav centre, then the theme toggle, the inbox and the user menu, all on one row closed by a double rule. Flex-wrap so it stacks on phones. The page title moves out of the header into the page body as an h1. The header is the same on every page that has a nav, and its width never changes (the game page does not widen it).

```css
.header {
    display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 16px;
    max-width: 71rem; margin: 0 auto; padding: 28px 0 18px;
    border-bottom: 3px double var(--ink);
}
.logo { font-family: 'UnifrakturMaguntia', serif; font-size: 34px; color: var(--ink); text-decoration: none; }
.nav-item {
    padding: 6px 0; color: var(--muted); text-decoration: none;
    font-family: 'IM Fell English SC', serif; font-size: 21px;
    border-bottom: 2px solid transparent;
}
.nav-item:hover { color: var(--ink); }
.nav-item.active { color: var(--ink); border-bottom-color: var(--rubric); }
```

**User menu.** There is no Profile nav item and no Logout button. `.header-right` holds the theme toggle, the inbox and a native `<details class="user-menu">`: its `<summary>` is a 32px avatar square (the uploaded avatar, set by `refreshHeaderAvatar()` in `theme.js`, else the first letter of the display name in the display face, in `--ink`), the name and a small caret. The panel holds a Profile link and a "Log out" button that calls the page's own logout handler. `theme.js` closes it on a click outside and on Escape.

**Phones.** Row 1 is the logo (28px), the icons and the avatar with the name hidden; row 2 is the nav (19px). That is about 90px in all, instead of the old 190px. The theme toggle stays visible.

The logo mark is the standing-stone from the mock-ups: a tapered stone with a downward rune, on a ground line. It is the `--stone-mark` mask in `styles.css`, coloured by `currentColor`, next to the wordmark (and above it on the login page). Unread inbox count: a 19px `--rubric` circle with 13px `--bg` text.

### Page title

```html
<h1 class="page-title"><span class="drop-cap">G</span>ames</h1>
<p class="page-subtitle">6 games, 1 awaits your reply</p>
```

The drop cap goes on fixed page titles only (see Typography). Titles that show a game's name, the calendar's title and every panel heading have none. The subtitle summarises the page in one line, and counts what awaits the viewer when that's above zero.

### Home

Greeting h1 "Well met, {name}" (blackletter W), a one-line subtitle that sums up what is on ("Age of Sojourn is in session now. One session awaits thy reply. Thy next game is Friday, Nov 13."), then two columns (one on phones). Fed by `GET /api/agenda` (`api/agenda.go`), which Home refreshes every 60 seconds while the tab is visible and on the `notifications:new` event.

- **At the table now** (above everything, directly under the greeting): one card per Confirmed, non-manual session between its start and end in a game the viewer plays, runs or follows. It is the only filled block on the page: `--verdigris` with `--bg` text (5.9:1 in parchment, 7.7:1 in candlelight). It carries a "● In session" marker (so the state never relies on colour), the game title as a link, "Began 18:00 CET, ends 22:00 · 1h 20m left", the DM, the accepted players as avatars and the viewer's reply.
- **Left column:** `§ Awaits thy reply`, red-letter date cards (rubric month, big day number and weekday; game title; time range with zone; "proposed by ♛ X"; "N of M have replied"; Aye / Perhaps / Nay posting to the session's `/responses` endpoint), then `§ The road ahead`, ledger rows of Confirmed sessions only: date, time, game and the viewer's reply. A suggestion appears on Home only as an `Awaits thy reply` card; once answered, or if the viewer is its DM, it stays on the game page and calendar.
- **Right column:** `§ Thy games` rows (role and status, "All games", "N open to join"), `§ Roads yet to open` (small dashed Chat and Codex stones, not clickable) and a line pointing to Discord.
- A user with no games and nothing pending sees the introduction text and a "See the games" link instead.
- Suggested sessions are availability requests: only a game's players and DM see them. Followers see Confirmed sessions only. Manual, Cancelled, Finished and Cancelled-game sessions are left out.

### Games page

Three sections, each with a count and hidden when empty: `§ Thy games` (the viewer is the DM or a player), `§ Open to join` (not a member; sign-ups open, or the viewer's own sign-up is waiting) and `§ Other games`. The first two show banners (each section is its own banner grid, see `BANNER_DESIGN_GUIDE.md`). Other games are compact ledger rows (`.ledger-row--game`): type icon, title as a link, system, ♛ DM, status badge and Follow / Unfollow. That section is collapsible, open by default, and the choice is remembered in `localStorage` (`waystone-other-games`); a link to a row inside it (`#campaign-{id}`) opens it. Within a section, banners keep the needs-action-first, then status order. "+ Register game" is a secondary button. With no games at all the page invites the viewer to register one.

### Game page

A normal page, not a sheet: a breadcrumb, then the head, then the two panes. The head has the kicker (type icon, "Campaign, D&D 5e", status badge), the h1 (no drop cap), the lede and, for members, one "Calendar" button at the top right; one `1px` ink rule closes it. The tale and the chronicle sit on the left, the session panel on the right, split by an ink rule.

- **Ahead** is a ledger of buttons, one per upcoming session: date, time, status tag, "3 aye, 1 no reply" and the viewer's reply ("✓ Aye", "? Perhaps", "✗ Nay", or "Thy reply?" in rubric). The selected row uses the folio fill and a `2px` rubric inset outline. Visitors see no counts and no reply. There is no cell grid, legend or reply key on this page.
- **Played** is one `<li>` per session holding a full-width row button (number, date, time, attendees), with the journal's entry line under it so real links never sit inside a button. Both select into the panel, which reuses the calendar panel's card, roster and reply buttons. Player avatars in a button are never links.
- **Leave this game** is a text link under the panel, for players, and uses the confirm dialog. "Show the company" is a text link too. Follow, Sign up, Sign-ups, Edit and Delete game stay in the panel's foot.
- **At ≤ 900px** the layout stacks. A compact reply block sits directly under the lede for the session the viewer owes a reply on (date, status, "18:00–22:00 CET, proposed by ♛ X. 1 aye, 3 no reply.", a "Who?" link to the roster and Aye / Perhaps / Nay). It is hidden on wider screens. The tale folds to three lines with a "Read the whole tale" link when it is longer. The panel's session details and company sit below the chronicle. On phones Ahead drops the time and status columns.

### Breadcrumb

Every page below the Games page, and the change-password page, opens with `<nav class="game-crumb" aria-label="Breadcrumb">`: 14.5px links separated by ` › `, and the current page last as plain text with `aria-current="page"`. It replaces the × close link, which the game page no longer has. Pop-ups keep their ×. The change-password page also shows the main nav (no item is active, since Profile lives in the user menu), except on a forced change, where neither appears.

### Game journal

Each played session can have one entry, written in the game description's Markdown (`renderMarkdown` in `campaigns.js`, `.game-text`). The mock-up is `docs/design/mockups/game-journal.html`.

- **Titles:** "Game 31: Name", set as the game title (no drop cap), the number in the text face (`.t-no`, because the display face's old-style numerals make "31" read as "3I"), then the optional name. The number comes from the session and is never stored.
- **Pages:** `/games/{id}/journal/{sessionId}` (entry), `/edit` and `/history`, one page with a title block under the breadcrumb and, as on the calendar page, two panes split by an ink rule. The pane rules are scoped under `.journal-page` (`ins` and `del` are the diff colours: verdigris underlined, rubric struck through).
- **Played rows:** the `<li class="game-row">` holds the session button and, under it, `.entry-line`: the entry title as a link plus a "Penned by … · amended by …" byline, or "No entry yet" with a "✎ Write the entry →" link for members. Hover and selection sit on the row. The same block is the first card of the session panel (`.panel-journal`, ink border).
- **Entries on cancelled sessions** are one muted line under Played (`.entry-cancelled`). Their page is read-only.
- **Editor:** name, text and preview side by side (Write/Preview tabs at ≤ 768px), an optional note, one `.notice` for errors. A failed save never discards the draft. Leaving a dirty draft asks first (`beforeunload`).
- **Combine your changes:** one card per conflict, each with Yours and the other scribe's version as word diffs and a row of picks (Keep yours, Keep theirs, Keep both when both added, Combine by hand). A rubric border means undecided, a hairline means decided. The merged entry shows below, and Save stays disabled until every conflict is decided.
- **History:** revisions newest first with a +/- word count, and the selected one as a diff against the one before, or its full text. Restoring asks first and saves a new revision. The newest 200 revisions are kept, and the page says so once older ones are gone.
- **Buttons:** primary is `.submit-btn` (set to `width: auto` outside a form column), secondary `.campaign-action-btn`, Delete `.btn-danger`. Confirmations reuse the game page's modal. Errors are plain and precise.

### Buttons

Square, sentence case, 14.5px at weight 500 (reply buttons 15px), min-height 42px (46px for reply buttons, 44px for primary actions on touch). No transitions longer than 0.15s, no lift, no glow.

| Variant | Style | Use |
|---------|-------|-----|
| Primary | `background: var(--ink); color: var(--bg); border: 1px solid var(--ink)` | One main action per view: Sign up, Enter, Save |
| Secondary | `background: none; color: var(--ink); border: 1px solid var(--ctl)` | Follow, Calendar, Edit, Cancel, + Register game |
| Needs action | `background: var(--rubric); color: var(--bg)` | A banner's "Reply" only |
| Danger | `background: none; color: var(--rubric); border: 1px solid var(--rubric)` | Delete game, Delete account, Remove player |
| Text link | `color: var(--rubric); text-decoration: underline; text-underline-offset: 4px` | "Add to my calendar app", "Leave this game", "Show the company" |

Hover: primary goes to `opacity: 0.85`, secondary and danger fill with their border colour and switch text to `--bg`. Disabled: `color: var(--muted); border: 1px dashed var(--line)` and no fill, never `opacity`.

### Form inputs

A ledger line, not a box:

```css
label {
    font-size: 14.5px; font-weight: 500; color: var(--ink);
}
input, select, textarea {
    font-family: inherit; font-size: 16px; color: var(--ink); background: transparent;
    border: none; border-bottom: 1px solid var(--ink); padding: 10px 2px;
}
input:focus, select:focus, textarea:focus {
    outline: none; border-bottom: 2px solid var(--rubric); padding-bottom: 9px;
}
input::placeholder { color: var(--muted); }
.form-hint { font-size: 14px; color: var(--muted); }
```

Textareas and multi-line fields may use a full `1px solid var(--line)` box.

### Cards

**Game cards are banners.** Everything banner-specific (silhouettes, medallion, headpieces, aligned tracks, button rows, stretching construction, motion, states) lives in [`BANNER_DESIGN_GUIDE.md`](BANNER_DESIGN_GUIDE.md) and takes precedence over this section.

Other cards (`.card`, profile, admin) stay plain rectangles:

```css
.card, .campaign-item {
    background: var(--bg); border: 1px solid var(--line);
    padding: 22px 24px; display: flex; flex-direction: column; gap: 14px;
}
```

Order inside a game card: notice slot (the pending notice, or the variant's headpiece) → status badge + game system → title → summary → hairline → DM, player avatars, seats · sign-ups, next session → actions in a 3-column grid. Each part is one subgrid row (see `BANNER_DESIGN_GUIDE.md`).

**Needs your action** (unanswered session for a player, pending sign-ups for the DM): the card gets `.needs-action`, which turns the banner's outline rubric (see `BANNER_DESIGN_GUIDE.md`), plus a notice line:

```css
.pending-notice {
    font-size: 14.5px; font-weight: 600; color: var(--rubric);
}
/* content: "✦ A session awaits thy reply" / "✦ 3 sign-ups to review" */
```

Finished games are not faded: the title is `--muted` and the rest stays readable.

### Campaign status badge

Outlined, sentence case, a glyph before the label. Not clickable. Keep the existing tooltip (`data-tooltip`, `title`, `tabindex="0"`, `aria-label`) but drop the glow. Focus outline is `2px solid var(--rubric)`.

| Status | Glyph | Colour | Border |
|--------|-------|--------|--------|
| Pitch | `✦` | `--rubric` | solid |
| Ongoing | `●` | `--verdigris` | solid |
| On Hiatus | `◐` | `--ochre` | dashed |
| Finished | `✓` | `--muted` | solid |
| Cancelled | `✗` | `--muted` | solid, label struck through |

```css
.campaign-status-badge {
    font-size: 13.5px;
    padding: 1px 8px; border: 1px solid currentColor; background: none;
}
.status-pitch     .campaign-status-badge { color: var(--rubric); }
.status-ongoing   .campaign-status-badge { color: var(--verdigris); }
.status-hiatus    .campaign-status-badge { color: var(--ochre); border-style: dashed; }
.status-finished  .campaign-status-badge { color: var(--muted); }
.status-cancelled .campaign-status-badge { color: var(--muted); text-decoration: line-through; }
```

### Campaign type badge

On game cards the type is shown only by the banner medallion: no text label and no info dot. The medallion has a focusable hotspot with the `aria-label` and tooltip. The mark is an inline SVG, 28px (24px viewBox, `fill: none`, `stroke: currentColor`, `stroke-width: 1.5`, `aria-hidden="true"`, class `type-icon`). See `type-icons.png`.

| Type | Mark |
|------|------|
| One-shot | A single sword |
| Adventure | Crossed swords behind a shield |
| Campaign | A banner flanked by swords |

The paths live in `TYPE_ICON_PATHS` in `campaigns.js`. The Adventure shield path carries `.type-icon__fill { fill: var(--bg) }` so it hides the swords behind it.

### People

- **DM**: the name in `--ink`, weight 600, prefixed with a `--rubric` crown `♛`. The crown, not the colour of the name, marks the DM.
- **Players**: plain `--ink`, separated by ` · `.
- **User links** (`.user-button`): plain text in the role's colour with an underline on hover. No pill or box.
- Display name precedence is unchanged: Nickname → Name → Username.

### Session replies & chips

One vocabulary wherever a person reads a reply state (the API values are unchanged):

| Reply | Glyph | Colour | Word |
|-------|-------|--------|------|
| Accepted | `✓` | `--verdigris` | Aye |
| Tentative | `?` | `--ochre` | Perhaps |
| Declined | `✗` | `--ink` | Nay |
| No reply | `…` | `--muted` | No reply yet |

When a Suggested session awaits the viewer's own reply, the word is "Thy reply?" in `--rubric`, weight 600. On a Confirmed session it stays a muted "No reply yet", because nothing is being asked of them. Other players' silence is muted. In the calendar `✓` and `✗` are inline SVG (`REPLY_GLYPHS` in `campaigns.js`), because the text glyph falls back to a font where the check reads like a root sign. The words come from `replyWord()` in `campaigns.js`.

Reply buttons sit in a 3-column grid, min-height 46px. The user's current reply is filled with its colour (`color: var(--bg)`), the others are outlined. Nay is outlined in `--ctl` with ink text and, when chosen, fills with `--ink`.

Times carry the session's own zone abbreviation ("18:00–22:00 CET", from `Intl.DateTimeFormat('en-GB', { timeZone, timeZoneName: 'short' })`, shown as stored and never converted; no label when the session has no zone), via `zoneAbbr()` in `campaigns.js`.

Calendar day frames (the session fills the day; sizes scale with the cell in `cqw`):

| Session | Style |
|---------|-------|
| Confirmed | Solid `--verdigris` frame, 10% tint mixed with `--bg` |
| Suggested | Dashed `--ink` frame |
| Strong candidate | Verdigris marching dashes plus a `✦` in `--rubric` |
| Cancelled | `--muted` frame, struck time, the word "cancelled", no counts |

`.chip-*` remains for the status tags in the day panel and the agenda rows.

Calendar grid: hairline cells, out-of-month days at 35% ink, selected day `background: var(--folio)` with a `2px solid var(--rubric)` inset outline. Days are square, and the month's width follows the viewport's height (the calendar is its own page, `/games/{id}/calendar`, which scrolls when the month is taller). A day with a session holds a frame inset from the hairlines: the day number, the **start time only** and a four-column tally (accepted, tentative, declined, awaiting, zeros included) with a hairline above it. The tally keeps its colours and its `cqw` sizes: this is an explicit decision, and the two-number cells in `ux-game-calendar.dc.html` are superseded. These cell sizes are the one place text may go below 13px. Narrow cells (≤ 85px) show only the start time, accepted and awaiting. Today's date number is a filled `--ink` square with `--bg` text, also inside a session frame. The first planned session fills the cell and the rest show as "+N more". Zero counts stay at full `--muted` at weight 400, never reduced opacity (that drops the contrast below 4.5:1). The month header is the name in the display face and the year in the text face ("November" 2026). The legend (■ confirmed · ▢ suggested · ~~cancelled~~) sits right of the month name, and a reply key (✓ said aye · ? perhaps · ✗ nay · … no reply yet · ✦ strong candidate) sits under the grid. "Add to my calendar app" is a text link beside the title.

Paste mode: the DM's Copy button on a session puts the calendar into paste mode, with a `.calendar-paste-bar` (rubric hairline border) stuck to the top of the side panel, so it stays in view while the page scrolls. The reply key hides while it's up. Free days in the shown month get a `+` in `--rubric` and a copy cursor, blocked days (past, or already holding a planned session) get a not-allowed cursor, and chosen days are filled `--folio` with the selection outline and a `✓`. The bar's note explains why a day was refused. Never use dashed outlines for this: dashed means Suggested. Esc or Cancel leaves the mode, and nothing is created until the DM confirms.

Strong candidate: a Suggested session whose accepted players are over 70% of all players. Awaiting players count in the total, and exactly 70% does not qualify. The panel says it in a line ("Strong candidate: 5 of 6 accepted"), and so do the cell's `aria-label` and `title`.

Roll call (day panel): one row per player, ordered Aye, Perhaps, Nay, No reply yet. Each row has the avatar with its reply-coded border, the name, which wraps and never truncates, and a fixed-width reply column.

The day panel heading has an eyebrow ("Selected day" / "Month agenda") over a text-face heading (date headings are weight 600, 22px, with no drop cap). A suggestion the viewer hasn't answered gets a `--rubric` frame, like a card that needs action.

Counts on buttons (pending sign-ups, unanswered sessions) are a small rubric tab on the button's top-right corner, so the label keeps its single line.

### Modals

The page dims to `--folio` (or `rgba(0,0,0,0.6)` in dark). The sheet is `--bg` with a double frame: `border: 1px solid var(--ink); outline: 1px solid var(--ink); outline-offset: 5px`. The header has an eyebrow + display title on the left, a text link + square `×` close button (40×40, `aria-label="Close"`) on the right, and a double rule underneath.

### Messages

`.message` is a toast fixed to the bottom centre of the screen, above modals (`z-index: 2000`): `--ink` fill, `--bg` text, 15px, `role="status"`. It works the same on every page, including `.calendar-message` and `.journal-message`.

```css
.message { position: fixed; left: 50%; bottom: 24px; transform: translateX(-50%); z-index: 2000; background: var(--ink); color: var(--bg); font-size: 15px; padding: 10px 16px; }
.message.success::before { content: '✓'; }
.message.error { border-left: 8px solid var(--rubric); }  /* "✗ …" */
```

Auto-hide after 5 seconds. Error text is plain and precise, no flavour copy. Messages are plain words, not raw API text: following shows "Now following {title}" and "No longer following {title}". On the Games page the follow toast has an "Undo" text button (`.message-undo`) that toggles back.

### Danger zone

`border: 1px solid var(--rubric)`, no fill, `§`-headed h2 in `--rubric`, danger buttons inside.

### Avatars

Square (no radius), `1px solid var(--line)` frame, `object-fit: cover`. Everyone has one: without an upload the avatar is the first letter of the display name (nickname → name → username), in the display face in `--ink` on `--folio`. The API sends it as `avatar_letter` next to `avatar_url` on every user object.

- **On game cards:** the DM's row shows their 30px avatar beside the crown and name, one link to the profile.
- **In lists** (game-card players): 30px avatars in place of names, linked to the profile, with the name as `title` and `aria-label`. Built by `userAvatarHtml()` in `campaigns.js`.
- **Next to a name** (sign-ups dialog): same 30px avatar, not linked.
- **Calendar replies** (the Roll call, beside the name): a 2px border shows the reply. Accepted is solid `--verdigris`, Tentative dashed `--ochre`, Declined solid `--rubric` at 60% opacity, No reply dotted `--muted`.
- **Profile**: the large avatar keeps its own size.

### Ornaments

- `§` prefixes section headings (h2).
- `❦ ❦ ❦` (muted, 12px letter-spacing) may close a long page or list. At most one per page.
- `— ❦ —` in `--rubric` may separate the brand block from a form (login).
- Ornament glyphs are `aria-hidden="true"` when purely decorative.

---

## Interactions

- Transitions: `0.15s` on colour, background and border only. No transforms, no shadows, no glow (banners excepted, see `BANNER_DESIGN_GUIDE.md`). The one continuous animation outside banners is the strong candidate's marching dashes (1.6s, stopped under `prefers-reduced-motion`).
- Hover changes fill or colour (see Buttons). Links underline.
- Focus: every focusable element shows `outline: 2px solid var(--rubric); outline-offset: 2px` on `:focus-visible`. Inputs use the rubric underline instead.
- Touch targets ≥ 40px, ≥ 44px for primary and reply buttons.

---

## Do's and Don'ts

✅ Use tokens, never raw hex
✅ IBM Plex Mono for all text, Fell for titles, blackletter only for the brand and fixed page titles
✅ One accent (rubric), and red means you. State colours only for state
✅ Pair every state colour with a glyph, border style or strikethrough
✅ Square corners, hairlines, double rules
✅ Check both themes and phone width
✅ Stop any continuous animation under `prefers-reduced-motion` and keep the state readable without it

❌ No border-radius (except the unread dot), shadows, glows, gradients or textures (banners excepted, see `BANNER_DESIGN_GUIDE.md`)
❌ No parchment/scroll images, wax seals or fake aging
❌ No blackletter for body text, labels or anything that must be read quickly, and no display face for dates
❌ No new colours outside the token set
❌ No flavour copy in errors, validation or confirmations of destructive actions
❌ No more than one ornament line per page
❌ No uppercase, no letter-spacing and nothing below 13px (the calendar cell is the one exception)
❌ No faded text or controls (`opacity`)

---

## Page template

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Waystone - Page</title>
    <script src="/static/theme.js?v=N"></script> <!-- not deferred: applies the saved theme before paint -->
    <link rel="stylesheet" href="/static/styles.css?v=N">
    <script defer src="/static/vendor/alpinejs.min.js"></script>
</head>
<body x-data="page()">
    <div class="header">
        <a href="/" class="logo">Waystone</a>
        <nav class="nav-bar" aria-label="Main">
            <a href="/" class="nav-item">Home</a>
            <a href="/campaigns" class="nav-item active" aria-current="page">Games</a>
            <a href="/admin/users" class="nav-item" x-show="isAdmin">Admin</a>
        </nav>
        <div class="header-right">
            <!-- theme toggle and inbox (injected), then the user menu: <details class="user-menu"> with Profile and Log out -->
        </div>
    </div>

    <main class="container">
        <h1 class="page-title"><span class="drop-cap">P</span>age</h1>
        <!-- content -->
        <div class="message" :class="messageType" x-show="messageVisible" x-text="messageText"></div>
    </main>
</body>
</html>
```

## Cache-busting

Reference CSS with a version query (`styles.css?v=N`) and bump `N` whenever the CSS changes. Static files are baked into the Docker image, so rebuild with `--no-cache`.

---

**Version**: 2.1 (Scriptorium, reading pass), replacing 1.0 (terminal/cyberpunk)
**Last updated**: 2026-11-06
