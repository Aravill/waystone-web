---
name: banner-design
description: Design rules for the game banners (game cards on the Games page). Use whenever designing, mocking up, iterating on or implementing banners - new variants, silhouettes, ornaments, the type medallion, notice/headpiece row, button rows, alignment - in mock-ups or in campaigns.js / styles.css.
---

# banner-design

1. **Read first:** `BANNER_DESIGN_GUIDE.md` (banner decisions) and `STYLE_GUIDE.md` (tokens, type, general rules). The banner guide wins for anything banner-specific. Don't make the user re-explain anything recorded there.
2. **Hold the fixed anchors** on every variant and change:
   - Type medallion: a circle at `(64, 26)`, r24/r20, with the icon at `(50, 12)`. Same spot and shape on every variant. No type text label.
   - Notice / headpiece row at `y 58–80`: the alert replaces the centred headpiece in that row, never on the top edge, never pushing content down.
   - Subgrid tracks (top 92px · head · people · spacer · buttons · bottom 120px). The separator starts the people track so it always aligns.
   - Room for two button rows (3 × 2).
   - Every silhouette feature stays clear of the headpiece row and the content box.
3. **Build banners to stretch:** one continuous outline path per banner, authored for a 598px card and regenerated for the real height. Points below the join line `y = 470` shift down, side-edge wear stays above it, and hanging pieces follow their anchor point. Never assemble an outline from separate pieces (translucent `--line` doubles up at the joins), and never scale an SVG non-uniformly. Keep the content layer positioned above the absolutely positioned outline.
4. **Motion:** only the cloth moves (sway, flutter with ripple, ornament swing); text and buttons stay still. Respect `prefers-reduced-motion`. Current amounts are in the guide's Motion section.
5. **Style:** lined only (hairlines, `--line` / `--muted` / `--ink`, `--rubric` for needs-action). Asymmetry, tears, cuts, wear and hanging ornaments are welcome at the edges. Check both themes.
6. **Mock-ups:** iterate on `docs/design/mockups/banners.dc.html` (the design canvas linked in the guide) rather than starting over. Show one mock-up and iterate on it.
7. **Record decisions:** whenever the user settles or rejects something about banners, update `BANNER_DESIGN_GUIDE.md` in the same turn without being asked. Put settled rules in the matching section and rejected ideas under "Tried and rejected", with the reason.
