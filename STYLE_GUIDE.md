# Waystone Web Style Guide

The visual conventions for Waystone Web. Every page and feature follows them. Reference mock-ups live in `docs/design/mockups/`:

| Mock-up | Shows |
|---------|-------|
| `style-sheet.png` | Inks, letters, seals, instruments: the source for this guide |
| `login.png` | Narrow framed form |
| `campaigns-parchment.png` | Header, nav, page title, game card grid (light) |
| `type-icons.png` | The three game-type marks, light and dark |
| `campaigns-candlelight.png` | The same page in the dark theme |
| `calendar.png` | Modal sheet, calendar grid, session chips, reply buttons |
| `banners.dc.html` | The banner redesign: three variants, see `BANNER_DESIGN_GUIDE.md` |

## Design Philosophy

**"Scriptorium": typewritten text on a manuscript page.**

- A calm parchment page with iron-gall ink, and a single red accent used the way scribes rubricated headings
- Typewriter text for everything you read. Old-style display type only for titles and the brand
- Minimal: square corners, hairline and double rules, no textures, shadows, gradients or glows
- Decoration is typographic and sparse: `§`, `❦`, `✦`, `♛`. Never images of parchment, wax seals or scrolls
- Light flavour copy ("Thy reply", "Aye / Perhaps / Nay") is welcome in headings and buttons, never in error messages or anything that has to be read precisely
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
| `--line` | Hairline | `rgba(43, 36, 28, 0.4)` | Card borders, dividers, secondary button borders |
| `--rubric` | Rubric | `#8E2A1E` | The accent: section marks, DMs, pending actions, errors, destructive actions, active nav underline, links |
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
| `--rubric` | `#D9735C` |
| `--verdigris` | `#7FB09E` |
| `--ochre` | `#D4A94A` |
| `--shadow` | `rgba(0, 0, 0, 0.7)` |

```css
:root {
    --bg: #F2EAD8; --folio: #E6DBC2; --ink: #2B241C; --muted: #6B5E4B;
    --line: rgba(43, 36, 28, 0.4);
    --rubric: #8E2A1E; --verdigris: #355E52; --ochre: #7A5A12;
    --shadow: rgba(43, 36, 28, 0.24);
}
@media (prefers-color-scheme: dark) {
    :root:not([data-theme="light"]) {
        --bg: #16120E; --folio: #2A231B; --ink: #E8DCC4; --muted: #A8997F;
        --line: rgba(232, 220, 196, 0.3);
        --rubric: #D9735C; --verdigris: #7FB09E; --ochre: #D4A94A;
        --shadow: rgba(0, 0, 0, 0.7);
    }
}
:root[data-theme="dark"] { /* same values as the media query block */ }
```

### Rules

- One accent. `--rubric` is the only "look here" colour. Verdigris and ochre are only for state (success / tentative), never decoration.
- Every token pair used for text meets 4.5:1 contrast on `--bg` in both themes. Check any new pairing.
- Inverse elements (primary buttons) use `background: var(--ink); color: var(--bg)`, which flips correctly in dark mode.
- State is never shown by colour alone: pair it with a glyph (`✓ ? ✗ ✦ ● ◐`), a border style (solid / dashed / dotted) or strikethrough.

---

## Typography

Three families, vendored as woff2 under `static/vendor/fonts/` (SIL Open Font License, latin and latin-ext subsets) and declared with `@font-face` at the top of `styles.css`. Pages need no font link, and nothing loads from a third party at runtime.

| Role | Family | Use for |
|------|--------|---------|
| Text | `'Courier Prime', 'Courier New', monospace` | Everything by default: body, labels, buttons, badges, inputs, nav |
| Display | `'IM Fell English SC', serif` | Page titles (h1), card titles, modal titles, section headings, month names |
| Blackletter | `'UnifrakturMaguntia', serif` | The "Waystone" wordmark and an optional drop cap on h1 only. Never for a whole word other than the brand, never below 32px |

### Scale

| Element | Family | Size | Weight | Tracking | Case |
|---------|--------|------|--------|----------|------|
| Wordmark (header) | Blackletter | 34px | 400 | normal | as-is |
| Page title (h1) | Display | 44px (drop cap 56px in `--rubric`) | 400 | normal | as-is (small caps come from the font) |
| Section heading (h2) | Display | 24px, `--rubric`, prefixed `§` | 400 | normal | as-is |
| Card / modal title | Display | 27px / 32px | 400 | normal | as-is |
| Body | Text | 16px, line-height 1.6 | 400 | normal | sentence |
| Small / meta | Text | 14px, `--muted` | 400 | normal | sentence |
| Game system | Text | 14px italic, `--muted` | 400 | normal | as-is |
| Label, nav, button | Text | 12–14px | 400 | 2px | UPPERCASE |
| Eyebrow / kicker | Text | 12px, `--muted` | 400 | 3px | UPPERCASE |
| Badge | Text | 11–12px | 400 | 2px | UPPERCASE |

On screens ≤ 768px drop h1 to 34px and card titles to 23px. Keep tracking and case.

---

## Layout & Spacing

- **Page width**: content in a centred column, `max-width: 1160px`, `padding: 0 24px` (16px on phones).
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

Wordmark left, nav centre, theme toggle + inbox + user + logout right, all on one row closed by a double rule. Flex-wrap so it stacks on phones. The page title moves out of the header into the page body as an h1 (see `campaigns-parchment.png`).

```css
.header {
    display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 16px;
    max-width: 1160px; margin: 0 auto; padding: 28px 24px 18px;
    border-bottom: 3px double var(--ink);
}
.logo { font-family: 'UnifrakturMaguntia', serif; font-size: 34px; color: var(--ink); text-decoration: none; }
.nav-item {
    padding: 8px 0; color: var(--muted); text-decoration: none;
    font-size: 14px; letter-spacing: 2px; text-transform: uppercase;
    border-bottom: 2px solid transparent;
}
.nav-item:hover { color: var(--ink); }
.nav-item.active { color: var(--ink); border-bottom-color: var(--rubric); }
```

The logo mark is the standing-stone from the mock-ups: a tapered stone with a downward rune, on a ground line. It is the `--stone-mark` mask in `styles.css`, coloured by `currentColor`, next to the wordmark (and above it on the login page). Unread inbox count: a 16px `--rubric` circle with `--bg` text.

### Page title

```html
<h1 class="page-title"><span class="drop-cap">G</span>ames</h1>
<p class="page-subtitle">6 games on the roll · 2 await thee</p>
```

The drop cap is optional and goes on the page h1. The one other place for it is a heading inside a modal that names a single thing, like the calendar's selected day (34px there). The subtitle summarises the list, and counts what awaits the viewer when that's above zero.

### Buttons

Square, uppercase, 2px tracking, min-height 40px (44px for primary actions on touch). No transitions longer than 0.15s, no lift, no glow.

| Variant | Style | Use |
|---------|-------|-----|
| Primary | `background: var(--ink); color: var(--bg); border: 1px solid var(--ink)` | One main action per view: Sign up, Register game, Enter, Save |
| Secondary | `background: none; color: var(--ink); border: 1px solid var(--line)` | Follow, Calendar, Edit, Leave, Cancel |
| Danger | `background: none; color: var(--rubric); border: 1px solid var(--rubric)` | Delete game, Delete account, Remove player |
| Text link | `color: var(--rubric); text-decoration: underline; text-underline-offset: 4px` | "Sync to calendar →" |

Hover: primary goes to `opacity: 0.85`, secondary and danger fill with their border colour and switch text to `--bg`. Disabled: `opacity: 0.5; cursor: not-allowed`.

### Form inputs

A ledger line, not a box:

```css
label {
    font-size: 12px; letter-spacing: 2px; text-transform: uppercase; color: var(--muted);
}
input, select, textarea {
    font-family: inherit; font-size: 16px; color: var(--ink); background: transparent;
    border: none; border-bottom: 1px solid var(--ink); padding: 10px 2px;
}
input:focus, select:focus, textarea:focus {
    outline: none; border-bottom: 2px solid var(--rubric); padding-bottom: 9px;
}
input::placeholder { color: var(--muted); }
.form-hint { font-size: 13px; color: var(--muted); }
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

Order inside a game card: notice slot (the pending notice, or the variant's headpiece) → status badge → title + game system → summary → hairline → DM, player avatars, seats · sign-ups, next session → actions in a 3-column grid. Each part is one subgrid row (see `BANNER_DESIGN_GUIDE.md`).

**Needs your action** (unanswered session for a player, pending sign-ups for the DM): the card gets `.needs-action`, which turns the banner's outline rubric (see `BANNER_DESIGN_GUIDE.md`), plus a notice line:

```css
.pending-notice {
    font-size: 12px; font-weight: 700; letter-spacing: 2px; text-transform: uppercase; color: var(--rubric);
}
/* content: "✦ A session awaits thy reply" / "✦ 3 sign-ups to review" */
```

Finished games render at `opacity: 0.72`.

### Campaign status badge

Outlined, uppercase, a glyph before the label. Not clickable. Keep the existing tooltip (`data-tooltip`, `title`, `tabindex="0"`, `aria-label`) but drop the glow. Focus outline is `2px solid var(--rubric)`.

| Status | Glyph | Colour | Border |
|--------|-------|--------|--------|
| Pitch | `✦` | `--rubric` | solid |
| Ongoing | `●` | `--verdigris` | solid |
| On Hiatus | `◐` | `--ochre` | dashed |
| Finished | `✓` | `--muted` | solid |
| Cancelled | `✗` | `--muted` | solid, label struck through |

```css
.campaign-status-badge {
    font-size: 11px; letter-spacing: 2px; text-transform: uppercase;
    padding: 3px 8px; border: 1px solid currentColor; background: none;
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

- **DM**: `--rubric`, bold, prefixed `♛`. This is how DMs are "colourfully distinct" from players.
- **Players**: plain `--ink`, separated by ` · `.
- **User links** (`.user-button`): plain text in the role's colour with an underline on hover. No pill or box.
- Display name precedence is unchanged: Nickname → Name → Username.

### Session replies & chips

| Reply | Glyph | Colour | Button label |
|-------|-------|--------|--------------|
| Accepted | `✓` | `--verdigris` | Aye |
| Tentative | `?` | `--ochre` | Perhaps |
| Declined | `✗` | `--rubric` | Nay |
| Awaiting | `…` | `--rubric` | (none) |

Reply buttons sit in a 3-column grid, min-height 44px. The user's current reply is filled with its colour (`color: var(--bg)`), the others are outlined.

Calendar session chips (11px text, `padding: 3px 5px`):

| Session | Style |
|---------|-------|
| Confirmed | `1px solid var(--verdigris)`, verdigris text, faint verdigris fill |
| Suggested | `1px dashed var(--ink)` |
| Cancelled | `1px solid var(--muted)`, muted text, struck through, no counts |

Calendar grid: hairline cells, out-of-month days at 35% ink, selected day `background: var(--folio)` with a `2px solid var(--rubric)` inset outline. Days are square, and the month is sized to fit the modal's height. A legend (■ confirmed · ▢ suggested · ~~cancelled~~) sits right of the month name. A day's "awaiting a reply" count shows only when it is above zero, as a small rubric box in the corner.

The day panel heading has an eyebrow ("Selected day" / "Month agenda") over a display-face heading. A suggestion the viewer hasn't answered gets a `--rubric` frame, like a card that needs action.

Counts on buttons (pending sign-ups, unanswered sessions) are a small rubric tab on the button's top-right corner, so the label keeps its single line.

### Modals

The page dims to `--folio` (or `rgba(0,0,0,0.6)` in dark). The sheet is `--bg` with a double frame: `border: 1px solid var(--ink); outline: 1px solid var(--ink); outline-offset: 5px`. The header has an eyebrow + display title on the left, a text link + square `×` close button (40×40, `aria-label="Close"`) on the right, and a double rule underneath.

### Messages

```css
.message { padding: 10px 16px; font-size: 14px; border: 1px solid; background: none; }
.message.success { color: var(--verdigris); border-color: var(--verdigris); }  /* "✓ …" */
.message.error   { color: var(--rubric);    border-color: var(--rubric); }     /* "✗ …" */
```

Auto-hide after 5 seconds (unchanged). Error text is plain and precise, no flavour copy.

### Dashboard module cards

Standard cards with a display-face title, a muted description and a status badge. Active module: `border-color: var(--ink)`, hover fills `--folio`. Disabled ("Coming soon"): `opacity: 0.5`, dashed `--line` border, `cursor: not-allowed`.

### Danger zone

`border: 1px solid var(--rubric)`, no fill, `§`-headed h2 in `--rubric`, danger buttons inside.

### Avatars

Square (no radius), `1px solid var(--line)` frame, `object-fit: cover`. Everyone has one: without an upload the avatar is the first letter of the display name (nickname → name → username), in the display face in `--rubric` on `--folio`. The API sends it as `avatar_letter` next to `avatar_url` on every user object.

- **In lists** (game-card players, calendar replies): 30px avatars in place of names, linked to the profile, with the name as `title` and `aria-label`. Built by `userAvatarHtml()` in `campaigns.js`.
- **Next to a name** (sign-ups dialog): same 30px avatar, not linked.
- **Calendar replies**: a 2px border shows the reply. Accepted is solid `--verdigris`, Tentative dashed `--ochre`, Declined solid `--rubric` at 60% opacity, Awaiting dotted `--muted`.
- **Profile**: the large avatar keeps its own size.

### Ornaments

- `§` prefixes section headings (h2).
- `❦ ❦ ❦` (muted, 12px letter-spacing) may close a long page or list. At most one per page.
- `— ❦ —` in `--rubric` may separate the brand block from a form (login).
- Ornament glyphs are `aria-hidden="true"` when purely decorative.

---

## Interactions

- Transitions: `0.15s` on colour, background and border only. No transforms, no shadows, no glow (banners excepted, see `BANNER_DESIGN_GUIDE.md`).
- Hover changes fill or colour (see Buttons). Links underline.
- Focus: every focusable element shows `outline: 2px solid var(--rubric); outline-offset: 2px` on `:focus-visible`. Inputs use the rubric underline instead.
- Touch targets ≥ 40px, ≥ 44px for primary and reply buttons.

---

## Do's and Don'ts

✅ Use tokens, never raw hex
✅ Courier Prime for all text, Fell for titles, blackletter only for the brand
✅ One accent (rubric). State colours only for state
✅ Pair every state colour with a glyph, border style or strikethrough
✅ Square corners, hairlines, double rules
✅ Check both themes and phone width

❌ No border-radius (except the unread dot), shadows, glows, gradients or textures (banners excepted, see `BANNER_DESIGN_GUIDE.md`)
❌ No parchment/scroll images, wax seals or fake aging
❌ No blackletter for body text, labels or anything that must be read quickly
❌ No new colours outside the token set
❌ No flavour copy in errors, validation or confirmations of destructive actions
❌ No more than one ornament line per page

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
            <a href="/" class="nav-item">Dashboard</a>
            <a href="/campaigns" class="nav-item active" aria-current="page">Games</a>
            <a href="/profile" class="nav-item">Profile</a>
            <a href="/admin/users" class="nav-item" x-show="isAdmin">Admin</a>
        </nav>
        <div class="header-right">
            <!-- inbox, user name, logout -->
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

**Version**: 2.0 (Scriptorium), replacing 1.0 (terminal/cyberpunk)
**Last updated**: 2026-10-29
