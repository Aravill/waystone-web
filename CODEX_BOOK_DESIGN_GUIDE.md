# Codex Book Design Guide

Decisions for the codex books (the Codices home, `/codices`). It extends `STYLE_GUIDE.md` and is the sibling of `BANNER_DESIGN_GUIDE.md`. When a new book decision is made, record it here.

- **Mock-up:** `docs/design/mockups/codex-books.html`.
- **Implemented in:** `static/codex-books.js` (geometry, `codexBookHtml()`, hover), the "shelf" block of `static/codex.css`, the Home screen in `codex.html` (`bookHtml()` in `codex.js`; the hidden SVG there holds `#book-shadow`).

All coordinates are on a 236 × 320 book. CSS sizes are in `--u` (1px on desktop, 0.62px at ≤ 600px), so the art scales uniformly; strokes use `vector-effect: non-scaling-stroke` and stay 1px.

## Look

- **Lined, on one flat fill per book**, with the banners' soft shadow. No textures or gradients.
- **Dyed leather.** The fill is one of eight muted dyes (`--bk-dye-1` … `--bk-dye-8`), full strength on parchment, 58% over the ground on candlelight. None of them is rubric, verdigris or ochre: a dye says nothing.
- **Automatic.** Binding, emblem and dye come from an FNV-1a hash of the codex id. They never change and aren't stored, and nobody picks them. Two codices can look alike.
- **Thickness is data:** the text block's depth is `5 + 1.5·√pages`, at most 26.
- **Line colours are opaque** (`color-mix`), because translucent lines stack where pieces meet.

## Fixed anchors (every binding)

- **Title plate** 146 × 116 at cover `(36, 60)`, filled `--bg`. The title is in the title face (IM Fell, `--font-title`), sized by the length of the name and its longest word, 16–40px (13px or more on phones), at most five lines.
- **Emblem** 48 × 48 centred on cover `(109, 244)`.
- **Spine** 18px wide with four raised bands; the hinge is at its right edge.

## Bindings

| | 0 · Clasped | 1 · Diapered | 2 · Wrapped | 3 · Arcaded |
|---|---|---|---|---|
| Frame | Double fillet, boss ring | Zigzag roll, lattice, lozenge | Uneven vellum edge, spine tackets | Pointed arch, quatrefoil |
| Edge | Metal corner-pieces standing proud | Nick in the fore-edge | Wavy outline, creases | Chipped bottom corner |
| Fastening / hanging | Two strap clasps | Leather ties, one ribbon | Cord with a knot and a tag | Two ribbons |

## Opening

- A **mouse over the closed book's rectangle** for 90ms, or keyboard focus (`:has(:focus-visible)`), opens it: clasps fold back or the cord unwinds, then the cover swings to −111° with two loose leaves behind it (0.75s). It closes in 0.5s. The swung cover and leaves take no pointer, so they never hold the book open.
- The cover and loose leaves share one `preserve-3d` box (`.bk-turn`), so the browser sorts them by depth. The first leaf is flat HTML under them: **its text never moves** and stays crisp.
- The leaf shows the name, the excerpt (five lines), the two pages changed last as links (`latest` of `GET /api/codices`) and "Open codex". The whole book is one link (`.bk-hit`), the page links sit above it.
- The focus ring is drawn on `.book::after`, above the swung cover.
- **≤ 600px:** two small books a row, no opening. One tap opens the codex; a touch lifts the cover to −38° while it loads. The excerpt isn't shown there.
- **Reduced motion:** the cover gives way to the leaf at once.
- **Empty shelf:** a dashed blank book, "Make the first codex", for someone who may create one; otherwise "No codices to show yet."

## The title face's extra letters

IM Fell English SC has basic Latin only. `tools/fell/build.mjs` (no dependencies) composes 127 Latin Extended letters (č ě ř ů ł ą ő ș ğ …) from the vendored file's own letters and accents into `static/vendor/fonts/ws-fell-400-latin-ext.woff2`, family "Waystone Fell", which follows IM Fell in `--font-title`. The name differs because "IM FELL English SC" is a Reserved Font Name under the OFL. The file can't be called `waystone-*`: `.gitignore` drops that.

## Tried and rejected

- **One plain fill for every book** (as `--cloth` is for banners): the books were hard to tell apart.
- **Barlow on the covers:** too modern on the leather.
- **Barlow, or a fallback serif, only for names with letters IM Fell lacks:** titles have to match across all codices, so the letters were added instead.
- **Letting people pick the dye, emblem or binding:** automatic, as with banners.
- **A cover turned all the way over:** it needed the book to shift or cover its neighbour.
- **Loose leaves stacked by `z-index`:** they showed as stray corners behind the swung cover.
