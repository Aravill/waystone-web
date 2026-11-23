// Books for codices on the Codices home: every codex is a bound book that opens on hover or keyboard focus.
// The look (binding, emblem, dye) comes from the codex id, as a banner's does, and never changes; the text
// block's thickness grows with the page count. Spec: CODEX_BOOK_DESIGN_GUIDE.md. Uses escapeHtml (campaigns.js).
(() => {
    const r1 = (n) => Math.round(n * 10) / 10;
    const DYES = 8; // --bk-dye-1 ... in codex.css

    // FNV-1a: the ids are uuids, and their character sums are too alike to pick from.
    function seed(id) {
        let h = 2166136261;
        for (const ch of String(id || '')) h = Math.imul(h ^ ch.charCodeAt(0), 16777619) >>> 0;
        return h;
    }

    // Thickness of the text block: grows with the page count and stops at 26.
    const depth = (pages) => Math.round(5 + Math.min(21, Math.sqrt(pages) * 1.5));

    // Emblems: 48 x 48, centred on the cover's lower field at (109, 244).
    const EMBLEMS = {
        ship: 'M8 30H40L35 39H13ZM24 30V7M24 9C34 13 36 20 34 26H24M24 12C17 15 14 20 14 26H24M5 44Q11 40 17 44T29 44T41 44',
        star: 'M24 4L28 20L44 24L28 28L24 44L20 28L4 24L20 20ZM24 14V34M14 24H34M12 12L19 19M36 12L29 19M12 36L19 29M36 36L29 29',
        eye: 'M4 24Q24 6 44 24Q24 42 4 24ZM24 16A8 8 0 1 0 24 32A8 8 0 1 0 24 16ZM24 21A3 3 0 1 0 24 27A3 3 0 1 0 24 21ZM24 4V9M11 8L14 12M37 8L34 12M24 39V44M11 40L14 36M37 40L34 36',
        mountain: 'M4 40L18 12L25 26L30 18L44 40ZM14 20L18 23L22 20M27 23L30 25L33 23M10 44H38',
        key: 'M24 4A8 8 0 1 0 24 20A8 8 0 1 0 24 4ZM24 9A3 3 0 1 0 24 15A3 3 0 1 0 24 9ZM24 20V44M24 34H33V40M24 29H30M20 23H28',
        tree: 'M24 44V22M24 34L14 26M24 30L34 22M24 26L18 18M24 5C33 5 38 11 36 17C41 20 40 28 34 29C30 33 27 31 24 32C21 31 18 33 14 29C8 28 7 20 12 17C10 11 15 5 24 5ZM15 44H33',
    };

    // Covers: 218 x 320, hinge at x 0. Anchors shared by all of them: plate 36-182 x 60-176, emblem at (109, 244).
    const PLATE = `<path class="f-plate" d="M36 60H182V176H36Z"/><path class="s-soft" d="M40 64H178V172H40Z"/>`
        + [[36, 60], [182, 60], [36, 176], [182, 176]].map(([x, y]) => `<circle class="f-board" cx="${x}" cy="${y}" r="3"/>`).join('');
    const PLAIN = 'M0 0H214Q218 0 218 4V316Q218 320 214 320H0Z';
    // Mirrors a top-left corner piece into all four corners.
    const corners = (d) => [[1, 1, 0, 0], [-1, 1, 218, 0], [1, -1, 0, 320], [-1, -1, 218, 320]]
        .map(([sx, sy, tx, ty]) => `<g transform="translate(${tx} ${ty}) scale(${sx} ${sy})">${d}</g>`).join('');

    function lattice() {
        // Diagonal fillets inside x 24-194, y 200-296, left open around the emblem.
        let d = '';
        const x0 = 22, x1 = 196, y0 = 196, y1 = 298, step = 34;
        for (let c = x0 - (y1 - y0); c < x1; c += step) {
            // down-right: x = c + (y - y0)
            let ya = y0 + Math.max(0, x0 - c), yb = y0 + Math.min(y1 - y0, x1 - c);
            if (yb > ya) d += `M${c + ya - y0} ${ya}L${c + yb - y0} ${yb}`;
            // down-left: mirrored
            if (yb > ya) d += `M${218 - (c + ya - y0)} ${ya}L${218 - (c + yb - y0)} ${yb}`;
        }
        return `<path class="s-soft" d="${d}"/>`;
    }

    function zigzag() {
        let d = 'M16 16';
        for (let x = 16; x < 202; x += 6) d += `l3 5l3 -5`;
        let e = 'M16 304';
        for (let x = 16; x < 202; x += 6) e += 'l3 -5l3 5';
        return d + e;
    }

    const BINDINGS = [
        // 0 Clasped: metal corner-pieces that stand proud of the boards, a boss ring, two strap clasps.
        {
            outline: PLAIN,
            art: `<path class="s-tool" d="M12 12H206V308H12Z"/><path class="s-soft" d="M16 16H202V304H16Z"/>`
                + `<path class="s-soft" d="M16 190H202M16 194H202"/>`
                + corners(`<path class="f-board" d="M-3 -3H44Q40 10 30 12Q34 24 24 24Q24 34 12 30Q10 40 -3 44Z"/><circle class="s-tool" cx="13" cy="13" r="7"/><circle class="s-soft" cx="13" cy="13" r="3"/><circle class="f-dot" cx="34" cy="3" r="1.3"/><circle class="f-dot" cx="3" cy="34" r="1.3"/>`)
                + `<circle class="s-soft" cx="109" cy="244" r="37"/><circle class="s-tool" cx="109" cy="244" r="34"/>`,
            straps: [88, 218],
        },
        // 1 Diapered: a tooled roll, a lattice in the lower field, leather ties, one ribbon.
        {
            outline: 'M0 0H214Q218 0 218 4V262L215 268L218 272V316Q218 320 214 320H0Z',
            art: `<path class="s-tool" d="M11 11H207V309H11Z"/><path class="s-soft" d="${zigzag()}"/><path class="s-soft" d="M16 27H202M16 293H202"/>`
                + lattice()
                + `<path class="f-board" d="M109 204L149 244L109 284L69 244Z"/><path class="s-soft" d="M109 209L144 244L109 279L74 244Z"/>`
                + `<path class="s-soft" d="M186 40L192 34M190 44L198 36"/><circle class="f-dot" cx="30" cy="188" r="1"/><circle class="f-dot" cx="35" cy="184" r=".8"/>`,
            ties: [110, 210],
            ribbon: [118],
        },
        // 2 Wrapped: limp vellum with an uneven edge, tackets at the spine, a cord wound round it.
        {
            outline: 'M0 1Q40 -2 78 1T150 0T212 2Q219 3 218 10Q215 48 218 86T217 168Q220 214 217 250T218 310Q219 320 211 319Q170 322 128 318T50 320T0 319Z',
            art: `<path class="s-soft" d="M12 16Q90 12 204 16Q201 160 204 304Q110 308 12 304"/>`
                + `<path class="s-tool" d="M8 40V52M14 40V52M8 96V108M14 96V108M8 212V224M14 212V224M8 268V280M14 268V280"/>`
                + `<path class="s-soft" d="M196 22Q188 30 190 44M26 290Q34 284 46 288M198 262Q192 270 196 284"/>`,
            cord: true,
        },
        // 3 Arcaded: a pointed arch with tracery over the plate, a quatrefoil, a chipped corner, two ribbons.
        {
            outline: 'M0 0H214Q218 0 218 4V292L213 297L211 304L203 308L199 314L191 320H0Z',
            art: `<path class="s-tool" d="M12 12H206V286M184 308H12V12"/>`
                + `<path class="s-soft" d="M206 286L203 292L196 298L190 302L184 308"/>`
                + `<path class="s-tool" d="M22 298V66Q26 34 109 20Q192 34 196 66V286"/><path class="s-soft" d="M27 298V68Q32 40 109 27Q186 40 191 68V284"/>`
                + `<path class="s-soft" d="M20 20Q34 20 38 34Q24 38 20 52ZM198 20Q184 20 180 34Q194 38 198 52Z"/>`
                + `<path class="s-soft" d="M27 190H191M27 194H191"/>`
                + `<path class="s-soft" d="M109 196A24 24 0 0 1 133 220A24 24 0 0 1 157 244A24 24 0 0 1 133 268A24 24 0 0 1 109 292A24 24 0 0 1 85 268A24 24 0 0 1 61 244A24 24 0 0 1 85 220A24 24 0 0 1 109 196Z"/>`
                + `<circle class="s-tool" cx="109" cy="244" r="33"/>`,
            ribbon: [92, 108],
        },
    ];

    // Everything under the cover: shadow, back board, text block, spine, the top leaf, ribbons.
    function bodySvg(b, dx) {
        const dy = r1(dx * .55), n = Math.max(2, Math.round(dx / 2.6));
        let leaves = '';
        for (let k = 1; k < n; k++) {
            const ox = r1(dx * k / n), oy = r1(dy * k / n);
            leaves += `M${230 + ox} ${5 + oy}V${315 + oy}H${12 + ox}`;
        }
        const bands = [44, 104, 206, 266];
        const spine = 'M20 0H9Q1 0 0 8' + bands.map((y) => `V${y}C-6 ${y + 1} -6 ${y + 11} 0 ${y + 12}`).join('') + 'V312Q1 320 9 320H20Z';
        const ribs = (b.ribbon || []).map((x, i) => {
            const y = 315 + dy, len = 18 + i * 7;
            return `<g class="bk-hang"><path class="f-board" d="M${x} ${y - 4}Q${x - 2} ${y + len / 2} ${x + 1} ${y + len}L${x + 5} ${y + len - 5}L${x + 9} ${y + len + 1}Q${x + 6} ${y + len / 2} ${x + 8} ${y - 4}"/></g>`;
        }).join('');
        const ties = (b.ties || []).map((y) => `<g class="bk-hang"><path class="s-tool" d="M${236 + dx} ${y + dy}q9 3 7 14q-3 12 3 24M${236 + dx} ${y + dy}q4 6 1 16q-4 10 0 17"/></g>`).join('');
        return `<svg class="bk-body" viewBox="-20 -20 320 420" aria-hidden="true">
            <path class="f-board" filter="url(#book-shadow)" d="M9 0H${232}L${236 + dx} ${dy}V${316 + dy}Q${236 + dx} ${321 + dy} ${231 + dx} ${321 + dy}H${9 + dx}Q${2 + dx / 2} ${321 + dy / 2} 0 312V8Z"/>
            <path class="f-board" d="M${dx + 2} ${dy}H${232 + dx}Q${236 + dx} ${dy} ${236 + dx} ${dy + 4}V${316 + dy}Q${236 + dx} ${320 + dy} ${232 + dx} ${320 + dy}H${9 + dx}Q${1 + dx} ${319 + dy} ${dx} ${312 + dy}Z"/>
            <path class="f-board" d="M0 312Q1 320 9 320H20L${20 + dx} ${320 + dy}H${9 + dx}Q${3 + dx / 2} ${320 + dy / 2} 0 312Z"/>
            ${ribs}
            <path class="f-paper" d="M12 315H230V5L${230 + dx} ${5 + dy}V${315 + dy}H${12 + dx}Z"/>
            <path class="s-edge" d="${leaves}"/>
            <path class="f-paper" d="M16 5H230V315H16Z"/>
            <path class="f-board" d="${spine}"/>
            <path class="s-tool" d="${bands.map((y) => `M-4 ${y + 2}H20M-4 ${y + 10}H20`).join('')}"/>
            <path class="s-soft" d="M15 2V318"/>
            ${ties}
        </svg>`;
    }

    function coverHtml(c, b, emblem) {
        const n = c.name.length, longest = Math.max(...c.name.split(/\s+/).map((w) => w.length));
        // Sized by the whole name and its longest word; a word too long even at 16px breaks.
        const fs = Math.max(16, Math.min(n <= 8 ? 40 : n <= 18 ? 31 : n <= 28 ? 25 : n <= 50 ? 19 : 16, Math.floor(250 / longest)));
        const cord = b.cord ? `<g class="bk-cord"><path class="s-tool" pathLength="1" d="M0 198Q60 194 109 198T217 196"/><path class="s-tool" pathLength="1" d="M0 204Q50 207 109 203T217 204"/></g>
            <g class="bk-hang"><path class="f-board" d="M170 193Q178 190 186 194Q190 201 185 207Q177 211 170 206Q166 199 170 193Z"/><path class="s-tool" d="M170 195Q178 202 186 206M171 206Q177 198 185 195M174 208Q170 224 175 240M182 208Q187 222 183 250"/><path class="f-board" d="M180 250H187L189 266Q183 270 178 266Z"/></g>` : '';
        return `<div class="bk-cover">
            <div class="bk-face bk-face--out">
                <svg viewBox="-10 -10 238 340" aria-hidden="true">
                    <path class="f-board" d="${b.outline}"/>
                    <path class="s-soft" d="M5 0V320"/>
                    ${b.art}${PLATE}
                    <path class="s-tool" transform="translate(85 220)" d="${emblem}"/>
                    ${cord}
                </svg>
                <div class="bk-title" style="--fs:${fs}"><span>${escapeHtml(c.name)}</span></div>
            </div>
            <div class="bk-face bk-face--in">
                <svg viewBox="-10 -10 238 340" aria-hidden="true">
                    <path class="f-board" transform="translate(218 0) scale(-1 1)" d="${b.outline}"/>
                    <path class="f-paper" d="M16 14H214V306H16Z"/>
                    <path class="s-tool" d="M0 0L16 14M0 320L16 306"/>
                    <path class="s-edge" d="M78 112H152V208H78ZM82 116H148V204H82Z"/>
                    <path class="s-edge" transform="translate(91 136)" d="${emblem}"/>
                </svg>
            </div>
        </div>`;
    }

    // Strap clasps hinge on the back board and fold over the cover's edge.
    function strapsHtml(b, dx) {
        const dy = dx * .55;
        return (b.straps || []).map((y) => `<div class="bk-strap" style="left:calc(206 * var(--u));top:calc(${y} * var(--u));width:calc(${32 + dx} * var(--u));height:calc(${22 + dy} * var(--u))">
            <svg viewBox="0 0 ${32 + dx} ${22 + dy}" aria-hidden="true"><path class="f-board" d="M8 1H31L${31 + dx} ${dy + 1}V${dy + 19}L31 19H8L1 10Z"/><path class="s-soft" d="M31 1V19M12 4H27M12 16H27"/><circle class="s-tool" cx="14" cy="10" r="3"/></svg></div>`).join('');
    }

    // The inside of one .codex-book. `opts`: url, pageUrl(page), date(iso), pages (the "12 pages" label).
    window.codexBookHtml = (c, opts) => {
        const h = seed(c.id), names = Object.keys(EMBLEMS);
        const b = BINDINGS[h % BINDINGS.length], emblem = EMBLEMS[names[(h >>> 8) % names.length]], dx = depth(c.page_count);
        const latest = c.latest.map((p) => `<li><a href="${opts.pageUrl(p)}" title="${escapeHtml(p.title)}">${escapeHtml(p.title)}</a><span>${escapeHtml(opts.date(p.updated_at))}</span></li>`).join('');
        return `<article class="book" style="--dye:var(--bk-dye-${(h >>> 16) % DYES + 1})">
            ${bodySvg(b, dx)}
            <a class="bk-hit" href="${opts.url}" aria-label="${escapeHtml(c.name)}, ${opts.pages}"></a>
            <div class="bk-leaf">
                <p class="bk-leaf-title" aria-hidden="true">${escapeHtml(c.name)}</p>
                <p class="bk-leaf-text">${escapeHtml(c.excerpt || 'No description yet.')}</p>
                ${latest ? `<ul class="bk-leaf-latest" aria-label="Changed last">${latest}</ul>` : ''}
                <p class="bk-leaf-foot" aria-hidden="true">Open codex</p>
            </div>
            <div class="bk-turn"><div class="bk-fly bk-fly--2"></div><div class="bk-fly bk-fly--1"></div>${coverHtml(c, b, emblem)}</div>
            ${strapsHtml(b, dx)}
        </article><span class="codex-book-meta" aria-hidden="true">${opts.pages}</span>`;
    };

    // Opens on a mouse over the closed book's own rectangle, after a short wait so a passing pointer doesn't
    // riffle the whole shelf. The swung cover takes no pointer, so it never holds the book open. A touch only
    // lifts the cover while the codex loads.
    let timer = 0;
    const bookAt = (e) => (e.target.closest ? e.target.closest('.book') : null);
    const clear = () => { clearTimeout(timer); document.querySelectorAll('.book.is-open, .book.is-pressed').forEach((b) => b.classList.remove('is-open', 'is-pressed')); };
    document.addEventListener('pointerover', (e) => {
        const b = bookAt(e);
        if (!b || e.pointerType !== 'mouse' || b.contains(e.relatedTarget)) return;
        clearTimeout(timer);
        timer = setTimeout(() => b.classList.add('is-open'), 90);
    });
    document.addEventListener('pointerout', (e) => {
        const b = bookAt(e);
        if (!b || b.contains(e.relatedTarget)) return;
        clearTimeout(timer);
        b.classList.remove('is-open');
    });
    document.addEventListener('pointerdown', (e) => { const b = bookAt(e); if (b && e.pointerType !== 'mouse') b.classList.add('is-pressed'); });
    ['pointerup', 'pointercancel'].forEach((t) => window.addEventListener(t, () => document.querySelectorAll('.book.is-pressed').forEach((b) => b.classList.remove('is-pressed'))));
    window.addEventListener('pageshow', clear); // coming Back to a page kept in memory
})();
