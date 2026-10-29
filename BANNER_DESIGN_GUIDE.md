# Banner Design Guide

Decisions for the game banners (game cards on the Games page). It extends `STYLE_GUIDE.md`; everything there (tokens, type, hairlines, no fills or glows) still applies. When a new banner decision is made, record it here.

- **Mock-up:** `docs/design/mockups/banners.dc.html` (source of the design canvas at https://claude.ai/artifact/4oz5scXW83XZTtL3ziQrJw). It is the reference for every coordinate below.
- **Status:** implemented. In `campaigns.js`: the `BANNERS` table (geometry per variant), `bannerClothHtml()`, `mapBannerPath()`, `bannerMapper()`, and a global `requestAnimationFrame` loop `bannerFrame()`. In `styles.css`: `.banner-cloth`, `.banner-outline`, `.banner-orn`, `.banner-hang`, `.banner-swing`, `.banner-ring-o` / `-i`, `.banner-type`, `.banner-separator`, `.campaign-notice`, `.card-headpiece`. One hidden defs SVG in `campaigns.html` holds `#banner-flash` (gradient), `#banner-shadow` and `#banner-shadow-sm`. The previous iteration (`.banner-N` clip-path polygons, corner ornaments) is removed.

All coordinates are px on a 360px-wide card, origin at the card's top-left.

---

## Look

- **Lined, but liberal.** Hairline strokes only, no fills, textures or shading. Silhouettes are free and asymmetric: curved cuts, off-centre notches and points, uneven tails and tongues.
- **Decoration breaks the silhouette.** Tassels, charms and bells hang below; tears, cuts, chips, nicks, dog-ears, frayed edges and stitched repairs eat into it. Wear is drawn as lines (stitches, cracks, loose threads), never as texture.
- **The content area stays calm.** All of that lives at the edges and in the top and bottom zones; text never sits on an ornament.
- **Colours:** outline `--line`, ornaments and inner lines `--muted`, medallion ring `--ink`. Tokens only, so both themes work unchanged.
- **Drop shadow:** the one exception to "no shading". Each banner casts a soft shadow down and to the right, so it reads as cloth hanging in front of the page. The outline path is filled with `--bg` (same as the page, so the banner still looks lined) and carries an SVG `feDropShadow` (dx 4, dy 8, blur 6). Hanging ornaments get a tighter one (dx 2, dy 4, blur 1.5). The colour is a `--shadow` token: `rgba(43,36,28,0.24)` on parchment, `rgba(0,0,0,0.7)` on candlelight. The shadow moves with the cloth; text casts none. Use an SVG filter, not CSS `drop-shadow()` on the SVG element, which would shadow every line inside it.

---

## Fixed anchors (identical on every variant)

### Type medallion

- A **circle**, always in the **same spot**: centre `(64, 26)`, outer ring r24 (`--bg` fill, `--ink` stroke), inner ring r20 (`--line`). It straddles the top edge.
- Holds the game-type icon (`TYPE_ICON_PATHS`) at 28px, top-left at `(50, 12)`.
- Never moved, mirrored or reshaped per variant. A variant can't put the medallion on the right, and the diamond medallion was dropped.
- The medallion is the **only** type indicator. There is no "Campaign / Adventure / One-shot" text label. It must carry the type name and description for accessibility: a focusable hotspot (`.banner-type`) over the medallion has the `aria-label` and the tooltip. There is no info dot.

### Top edge

- The top edge runs at `y = 26`. The strip between the top edge and the headpiece row holds nothing but the medallion and corner features.

### Notice / headpiece row

- One fixed row below the medallion: `y 58–80`, centre line `y 69`.
- **No alert:** the variant's existing headpiece (from `HEADPIECES` in `campaigns.js`), centred, 90% of the card width (`x 18–342`), 18px tall, built as end · rule · centre · rule · end.
- **Alert:** the pending notice replaces the headpiece in the same row, left-aligned at `x = 28`. The full copy fits ("✦ A session awaits thy reply", "✦ 3 sign-ups to review"); no shortened wording is needed.
- The notice never sits on the top edge (it touched the border), and it never pushes content down.
- Silhouette features must clear this row. A corner cut has to end above `y 58`, or stay at least ~17px from the headpiece ends. The top-right concave cut is r30 for this reason; r46 hit the headpiece.

---

## Content and alignment

Cards in a row share row tracks (CSS subgrid), so the same parts line up across the row no matter what the content is:

| Track | Size | Holds |
|-------|------|-------|
| top | `92px` | Medallion, top edge, notice / headpiece row |
| head | auto | Status badge, title, game system, summary |
| people | auto | **Separator**, DM, player avatars, seats · next session |
| spacer | `auto` | Takes the slack when the row is taller than the content |
| buttons | auto | Action buttons |
| bottom | `120px` | Tails, points, tongues, hanging ornaments |
| gap | `var(--banner-gap)` | Space between rows of cards (7th track) |

- **Tracks:** the list is `grid-auto-rows: 92px auto auto auto auto 120px var(--banner-gap)` with `row-gap: 0`. Each card is `grid-row: span 7; grid-template-rows: subgrid`. The gap is a track because a `row-gap` would also be added inside the subgrid and push the 92px track down.
- **The spacer is `auto`, not `1fr`.** In an implicit grid with no definite height, `fr` takes the largest slack across all rows of cards, so every row would get the tallest row's slack.
- **The Alpine `x-for` wrapper div is `display: contents`**, so cards are direct grid items.
- **The separator is always aligned.** It is the first thing in the people track, so a long title or summary on one card never pushes it lower than on its neighbours.
- **Status badge** is the only item in the head row's first line, left-aligned where the type label used to be.
- **Buttons:** a 3-column grid with **up to two rows** (6 buttons). The track takes the height of the card with the most buttons; buttons align to the track's top, so a single row lines up with the first row of a double. Every banner must leave room for two rows.
- **Button hover:** the standard style-guide button states. A secondary button fills with `--ink` and its text turns `--bg`. The primary button drops to `opacity: 0.85`. Transitions are 0.15s on colour, background and border. On `:focus-visible` they get the rubric outline. Buttons don't move with the cloth, and hover doesn't animate the banner.
- **Content padding:** `28px` left, `30px` right. The content box never overlaps an inner line (`x 7` or `x 353`), a tear or a chip. Side tears go at most ~22px deep.
- **Height:** cards in a row are equally tall (the tallest content wins); the minimum is 598px. It is carried by `.banner-cloth` (`grid-row: 1 / 7`), an in-flow grid item, not by a min-height on the card.

---

## Construction: banners that stretch

Cards grow with their content and must never distort, so the outline isn't a fixed drawing. Each banner is authored once, for a 598px-tall card, and **regenerated in JS for the real height**:

- **The outline is one continuous path** (likewise each inner line). Points above the join line `y = 470` stay where they are. Points at or below it move down by `cardHeight − 598`, so the straight side edges simply get longer. Side-edge features (tears, chips) sit above the join line, and the bottom silhouette sits below it.
- Paths are written with absolute commands. Every side edge gets an explicit vertex at `y = 470` (e.g. `V470 V506`), so the edge stays straight down to the join and only bends below it.
- **Hanging pieces** (tassels, charms, bells, loose threads, the slit) each have an anchor point. They are translated with that point, never skewed, so they keep hanging straight down.
- Notches and tongue gaps stay inside the 120px bottom zone, below the buttons.
- Implementation: `mapBannerPath()` / `bannerMapper()` in `campaigns.js` (mock-up: `mapPath()` / `mapper()`). `bannerFrame()` is one global `requestAnimationFrame` loop. It re-queries `.campaign-item` every frame, so it survives Alpine re-renders, reads sizes from `offsetWidth` / `offsetHeight`, skips off-screen cards and eases the hover gust.

- Never scale a banner SVG non-uniformly (`preserveAspectRatio="none"`).
- **Keep everything inside the SVG box.** The live SVG is padded 48px on each side and 160px below (`BANNER_PAD` / `BANNER_PAD_B`), its content shifted back by a `translate`, so shadows and swaying tails fit inside it.
- **Never build an outline from separate pieces** (top SVG + border edges + bottom SVG). `--line` is translucent, so wherever pieces meet or overlap the alpha stacks into darker, doubled-looking lines. Moving the pieces separately makes it worse. This was the first approach and it showed visible artefacts on the sides.
- **Layering:** the cloth (`.banner-cloth`) is an in-flow grid item. The content rows are `position: relative` and come after it in the DOM, so they paint above it. Without that the outline paints over the content, as happened in the mock-up and hid all the text.
- **Width:** fixed 360px columns: `grid-template-columns: repeat(auto-fill, min(100%, 360px)); justify-content: center; column-gap: 1.75rem`. `.container--wide { max-width: 74rem }` fits three per row.
- **Below 360px (phones)** the mapper also remaps x. `x ≤ 120` stays put (medallion, left features), `x ≥ 300` shifts by `width − 360` (right cut / dog-ear, right chip, bell and right tail move rigidly), and the part between stretches linearly. Nothing folds while the width is above 180. The headpiece is HTML at `left` / `right` 5%, so it stretches.

---

## Motion

Banners sway in the breeze, clearly visible but not distracting. This is a banner-only exception to the style guide's "no transforms, 0.15s colour transitions only" rule.

- **Only the cloth moves; text and buttons stay still.** The banner SVG (outline, inner lines, headpiece, medallion, ornaments) sits in one `.cloth` layer. The content layer and the notice don't move.
- **Sway (JS, a bend, not a rotation):** the top edge and upper sides never tilt. From 45% of the height down (`BANNER_BEND`), the cloth drifts sideways, growing quadratically to ±7px at the bottom (`BANNER_SWAY_PX`, about what the old ±0.7° rotation gave). Period 13–15s per card. It's applied in `bannerMapper()`, so hanging pieces ride along.
- **Flutter (JS, per frame):** everything below the join line skews by ±4° (sine, period 3–3.8s per card). On top of that, a ripple of up to 3px runs along the bottom edge (`sin(wave − x·0.035)`, growing with depth below the join), so the tails ripple instead of tilting rigidly.
- **Swing (CSS):** hanging ornaments rotate ±12° about their attachment point (`transform-box: fill-box; transform-origin: 50% 0`), 2.1–2.5s. Each ornament also rides along with its anchor point as the bottom flutters.
- **Out of step:** each card has its own phase, period and CSS delays, seeded from the game id (`data-period` / `data-phase`, inline `--sway-dur` / `--sway-delay` / `--swing-dur` / `--swing-delay`), not from `nth-of-type`. A row never moves in unison.
- **Reduced motion:** everything stops under `prefers-reduced-motion: reduce`, and the outline is drawn still.
- **Tuning history:** ±0.35° / 1.4° / 6° felt too timid; +15% (±0.4° / 1.6° / 6.9°) was still too gentle. The values above are the current pass.

---

## Order

- **Needs action first**, then by status: Pitch · Ongoing · On Hiatus · Finished · Cancelled. Ties keep the API's order. Unknown statuses go last. Sorted in `loadCampaigns()` (`campaignNeedsAction()`, `bannerStatusRank()`).
- **Reordering slides, it never jumps.** When a refresh changes the order (a banner stops needing action, a status changes), every moved banner glides from its old place to its new one: FLIP via `bannerPositions()` / `slideBanners()`, 0.7s ease-out, translation only (no rotation, so the hairlines stay crisp). Skipped under reduced motion.

---

## States

- **Needs action:** the outline (1.5px), side edges, inner lines, ornaments and medallion rings turn `--rubric`, and the notice takes the headpiece row. This replaces the old rubric double frame.
  - **A flash rolls over the outline:** a light band sweeps diagonally (45°, top-left → bottom-right) across the rubric border, then pauses (4.5s cycle: ~2.5s sweep, ~2s rest). It's the outline's own stroke, a `userSpaceOnUse` linear gradient (`--rubric` · `--rubric` 25% mixed with white · `--rubric`) moved by `gradientTransform`, so there's no overlay path. The gradient is one shared definition (`#banner-flash`), so all needs-action cards flash in sync. Only the outline flashes; the ornaments, medallion and notice stay solid. The stroke falls back to solid `--rubric` under reduced motion or with the motion tweak off.
- **Finished:** the whole card at `opacity: 0.72`, rising to `0.92` on hover.
- **Hover:** every banner reacts, needs-action included.
  - **Banners that need no action:** the banner catches a gust, so it feels like something you can pick up. Over 0.3s, the outline, inner lines, ornaments and inner medallion ring darken from `--line` / `--muted` to `--ink`. The flutter eases up to double (±8°) and the ripple to 2.3× (~7px), then settles back when the pointer leaves (eased per frame, factor 0.06, so it never jumps). Text and buttons stay still. With motion off, only the colour changes.
  - **Needs-action banners** get the same gust and keep their rubric flash. The outline thickens from 1.5 to 2px and the inner medallion ring goes to full rubric (opacity 0.55 → 1). There's no colour change, so it still reads as "action needed".

---

## Variants

A game gets one variant, picked from its id (`bannerVariant()`), so it never changes. All three share the anchors and tracks above.

| | 1 · Gonfalon | 2 · Torn standard | 3 · Dagged |
|---|---|---|---|
| Headpiece | Scrollwork around a diamond | Leaf trefoil | Interlaced rings |
| Top corners | Concave r30 cut, top-right | Diagonal nick, top-left | Folded-down dog-ear, top-right |
| Inner line | Left edge and top, into the medallion; ends in a small curl at the bottom | Right edge, starting with a curl near the top-right corner | Top rule broken around the medallion; dashed stitched seam down the left |
| Side wear | None | V-tear in the left edge (~y 282–316) with a crack and a zigzag lacing across it (the earlier three vertical stitches ran parallel to the edge and read as doubled lines) | Chip bitten out of the right edge (~y 196–216); small nick near the bottom-left |
| Bottom | Swallowtail, notch off-centre (~64%), long left tail, short upturned right tail | Off-centre point (~36%); the edge from it up to the right is frayed sawtooth with loose threads | Three uneven tongues; a slit cut up from the left tongue's edge |
| Hanging | Tassel from the left tail | Ring and teardrop charm from the point | Tassel on the left tongue, bell on the right, threads on the middle |

New variants follow the same recipe: a distinct silhouette, one or two wear marks, one or two hanging ornaments, the shared medallion and row, and every feature clear of the content box.

---

## Tried and rejected

- **A vine tendril** along the top in place of the headpiece: dropped in favour of the existing centred headpiece.
- **The notice on the top edge**, between the medallion and the corner: no breathing room, it touched the border.
- **The notice as an extra content line:** it pushed the content down and broke the alignment between cards.
- **The medallion moving per variant** (right side, centre) or **changing shape** (diamond): it must stay in one place.
- **A big top-right cut (r46):** it ran into the headpiece.
- **A pulsing action outline** (stroke opacity fading in and out): wanted a directional flash rolling across the border instead.
- **Swaying the whole card:** the text tilted with it. Only the cloth layers move.
- **Rotating the cloth layer for the sway** (CSS `rotate`, ±0.7°): a hairline tilted by a fraction of a degree steps one pixel sideways every ~150–200px. At each step it's split across two pixel columns at half brightness, which reads as short doubled lines, and the steps crawl up and down as it swings. Promoting the layer (`will-change: transform`) made it blurrier. Replaced by the bend, which keeps the top and upper sides perfectly vertical.
- **Scaling x uniformly on narrow cards:** it pushed variant 3's top rule into the medallion and skewed the dog-ear. The x-split remap (left fixed, right shifted, middle stretched) replaced it.
- **Fluid card width** (`minmax(20rem, 1fr)`): the outline would have needed split top and bottom pieces. Fixed 360px columns instead.
