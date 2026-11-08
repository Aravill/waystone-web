// Campaign types: single source for labels and tooltip/form descriptions.
const CAMPAIGN_TYPES = [
    { value: 'One-shot', label: 'One-shot', description: 'A single, self-contained story told in 1–2 sessions. Low commitment, ideal for trying a new system, a new DM, or just dropping in.' },
    { value: 'Adventure', label: 'Adventure', description: 'A short run of 3+ sessions with one self-contained plot and a clear ending. A few weeks to a few months of commitment.' },
    { value: 'Campaign', label: 'Campaign', description: 'A long, overarching story with no set number of sessions. Characters grow over many months; expect a regular, ongoing commitment.' }
];

// Game-type marks (24px grid, 1.5 stroke, currentColor). See STYLE_GUIDE.md "Campaign type badge".
const TYPE_ICON_PATHS = {
    'One-shot': '<path d="M12 1.5 L13.6 4 V15 H10.4 V4 Z"/><path d="M7 15 H17"/><path d="M12 15 V19.5"/><circle cx="12" cy="21" r="1.4"/>',
    'Adventure': '<path d="M21.5 2.5 L7 17"/><path d="M5 15 L9 19"/><path d="M7 17 L4 20"/><circle cx="3.2" cy="20.8" r="1.1"/><path d="M2.5 2.5 L17 17"/><path d="M15 19 L19 15"/><path d="M17 17 L20 20"/><circle cx="20.8" cy="20.8" r="1.1"/><path class="type-icon__fill" d="M12 6 L15.5 7.5 V11.5 Q15.5 15 12 17 Q8.5 15 8.5 11.5 V7.5 Z"/><path d="M12 8 V15"/>',
    'Campaign': '<circle cx="12" cy="1.8" r="0.9"/><path d="M12 2.7 V4"/><path d="M8 4 H16"/><path d="M9 4 V14.5 L12 12 L15 14.5 V4"/><path d="M12 12 V22.5"/><path d="M2.5 3 V15"/><path d="M0.8 15 H4.2"/><path d="M2.5 15 V18.6"/><circle cx="2.5" cy="19.7" r="1"/><path d="M21.5 3 V15"/><path d="M19.8 15 H23.2"/><path d="M21.5 15 V18.6"/><circle cx="21.5" cy="19.7" r="1"/>'
};

// Headpieces, one per banner variant. A headpiece is end · rule · centre · rule · end, so it
// stretches to the card width without distorting the curls. Ends are drawn for the left
// side (outer edge at x=0) and mirrored for the right.
const HEADPIECES = [
    {   // 1: scrollwork around a diamond
        end: { w: 24, d: '<circle cx="2" cy="9" r="1"/><path d="M24 9 C18 9 14 13 10 10.5 C7.5 8.5 9.5 5 12 7"/>' },
        centre: { w: 84, d: '<path d="M42 3.5 L46.5 9 L42 14.5 L37.5 9 Z"/><circle cx="42" cy="9" r="1"/><path d="M48 9 C53 3 59 3 64 9 C69 15 75 15 80 9 L84 9"/><path d="M36 9 C31 3 25 3 20 9 C15 15 9 15 4 9 L0 9"/>' }
    },
    {   // 2: leaf trefoil
        end: { w: 18, d: '<path d="M18 9 H9"/><path d="M9 9 C7 5.5 3.5 5.5 1 9 C3.5 12.5 7 12.5 9 9 Z"/>' },
        centre: { w: 56, d: '<path d="M28 2 C32 5.5 32 9.5 28 13 C24 9.5 24 5.5 28 2 Z"/><path d="M28 13 C23 15.5 17 14.5 14 10.5 C19 9.5 24 10.5 28 13"/><path d="M28 13 C33 15.5 39 14.5 42 10.5 C37 9.5 32 10.5 28 13"/><path d="M28 13 V16.5"/><path d="M0 9 H12 M44 9 H56"/>' }
    },
    {   // 3: interlaced rings
        end: { w: 16, d: '<path d="M16 9 H4"/><path d="M8 5 L3 9 L8 13"/>' },
        centre: { w: 52, d: '<circle cx="20" cy="9" r="6.5"/><circle cx="32" cy="9" r="6.5"/><path d="M0 9 H11 M41 9 H52"/><circle cx="26" cy="9" r="1"/>' }
    }
];

// Banner geometry: each game gets one variant (stable, from its id) - a banner shape
// (BANNERS below, drawn as SVG and animated by bannerFrame) and a headpiece.
const BANNER_W = 360, BANNER_H = 598, BANNER_JOIN = 470, BANNER_SPLIT_L = 120, BANNER_SPLIT_R = 300;
const BANNER_PAD = 48, BANNER_PAD_B = 160;
const BANNER_BEND = 0.45, BANNER_SWAY_PX = 7;
const BANNER_ARGS = { M: 2, L: 2, H: 1, V: 1, C: 6, S: 4, A: 7, Z: 0 };

// Authored for a 360x598 card. Points above BANNER_JOIN only stretch sideways; points below
// follow the card's height and flutter. Hanging pieces (hang) follow their anchor `at`; swing
// ones also sway on their own. Variant order matches HEADPIECES.
const BANNERS = [
    {   // 1: gonfalon
        period: 3.3, phase: 0,
        outline: 'M0.5 26 H329.5 A30 30 0 0 0 359.5 56 V470 V506 L346 530 C310 514 268 502 230 494 C160 510 82 536 20 560 L0.5 542 L0.5 470 Z',
        inner: 'M38 33 H7 V470 V524 C7 532 13 535 17 530',
        orn: [],
        hang: [{ at: [20, 560], swing: true, html: '<path d="M20 560 V570 M20 570 L24 575 L20 580 L16 575 Z M17.5 580 V591 M20 580 V594 M22.5 580 V591"/>' }]
    },
    {   // 2: torn standard
        period: 3.8, phase: 2.1,
        outline: 'M12 26 H359.5 V470 V480 L340 492 L332 486 L318 502 L306 496 L290 512 L276 507 L262 522 L248 516 L232 532 L218 527 L200 544 L186 539 L170 554 L156 551 L130 566 L0.5 510 L0.5 470 V316 L16 299 L0.5 282 V38 Z',
        inner: 'M302 41 C298 39 300 33 306 33 H353 V470 V474 C353 481 348 483 344 479',
        orn: ['M16 299 L22 297 M2 284 L4 312 L7 289 L10 305.5 L13 296'],
        hang: [
            { at: [318, 502], html: '<path d="M318 502 L316 511"/>' },
            { at: [290, 512], html: '<path d="M290 512 L291 519"/>' },
            { at: [262, 522], html: '<path d="M262 522 L260 531"/>' },
            { at: [200, 544], html: '<path d="M200 544 L199 552"/>' },
            { at: [130, 566], swing: true, html: '<path d="M130 566 V574 M130 581 C125 585 125 591 130 595 C135 591 135 585 130 581 Z"/><circle cx="130" cy="577.5" r="3.5"/>' }
        ]
    },
    {   // 3: dagged, with a dog-ear
        period: 3.0, phase: 4.0, stitch: true,
        outline: 'M0.5 26 H330 L359.5 55.5 V196 L352 201 L356 207 L350 212 L359.5 216 V470 V488 L336 548 L300 500 L246 530 L196 496 L70 572 L0.5 530 V520 L14 514 L0.5 510 V470 Z',
        inner: 'M7 40 V470 V500',
        orn: ['M330 26 V55.5 H359.5', 'M7 33 H38 M90 33 H324'],
        hang: [
            { at: [122, 541], html: '<path d="M96 518 L122 541 L99 523"/>' },
            { at: [246, 530], html: '<path d="M246 530 L245 538 M250 527 L251 534"/>' },
            { at: [70, 572], swing: true, html: '<path d="M70 572 V580 M70 580 L74 585 L70 590 L66 585 Z M67.5 590 V600 M70 590 V603 M72.5 590 V600"/>' },
            { at: [336, 548], swing: true, html: '<path d="M336 548 V554 M331 572 C331 564 333 561 336 561 C339 561 341 564 341 572 Z M329 572 H343"/><circle cx="336" cy="557.5" r="3.5"/><circle cx="336" cy="575" r="1.2"/>' }
        ]
    }
];

function bannerSeed(id) {
    let sum = 0;
    for (const ch of String(id || '')) sum = (sum + ch.charCodeAt(0)) % 9973;
    return sum;
}

function bannerVariant(id) {
    return (bannerSeed(id) % HEADPIECES.length) + 1;
}

function headpieceHtml(variant) {
    const hp = HEADPIECES[variant - 1];
    const svg = (part, cls) => `<svg class="${cls}" viewBox="0 0 ${part.w} 18" width="${part.w}" height="18" fill="none" stroke="currentColor" stroke-width="1" stroke-linecap="round">${part.d}</svg>`;
    return `<div class="card-headpiece" aria-hidden="true">${svg(hp.end, 'hp-end')}<span class="hp-rule"></span>${svg(hp.centre, 'hp-centre')}<span class="hp-rule"></span>${svg(hp.end, 'hp-end hp-end--right')}</div>`;
}

// Rewrite an absolute path through map(x, y); H and V become L so they can bend.
const bannerTokens = new Map();
function mapBannerPath(d, map) {
    let t = bannerTokens.get(d);
    if (!t) { t = d.match(/[A-Za-z]|-?\d*\.?\d+/g); bannerTokens.set(d, t); }
    const f = (v) => Math.round(v * 100) / 100;
    const out = [];
    let i = 0, cx = 0, cy = 0, cmd = 'M';
    while (i < t.length) {
        if (/[A-Za-z]/.test(t[i])) { cmd = t[i]; i++; if (cmd === 'Z') { out.push('Z'); continue; } }
        const n = t.slice(i, i + BANNER_ARGS[cmd]).map(parseFloat);
        i += BANNER_ARGS[cmd];
        let p;
        if (cmd === 'H') { p = map(n[0], cy); cx = n[0]; out.push('L' + f(p[0]) + ' ' + f(p[1])); }
        else if (cmd === 'V') { p = map(cx, n[0]); cy = n[0]; out.push('L' + f(p[0]) + ' ' + f(p[1])); }
        else if (cmd === 'A') { p = map(n[5], n[6]); cx = n[5]; cy = n[6]; out.push('A' + n.slice(0, 5).join(' ') + ' ' + f(p[0]) + ' ' + f(p[1])); }
        else {
            let s = cmd;
            for (let k = 0; k < n.length; k += 2) { p = map(n[k], n[k + 1]); s += (k ? ' ' : '') + f(p[0]) + ' ' + f(p[1]); }
            cx = n[n.length - 2]; cy = n[n.length - 1]; out.push(s);
        }
    }
    return out.join(' ');
}

// dw: extra width (the middle band stretches, the sides keep their shape); dy: extra height;
// below BANNER_JOIN points also skew and ripple.
// sway: the sideways drift (px) at the bottom edge. The cloth bends rather than rotates: the top
// and upper sides stay straight (a slightly tilted hairline shows pixel steps), and the drift
// grows quadratically from BANNER_BEND of the height down, so the bend has no kink.
function bannerMapper(dw, dy, skewDeg, ripple, wave, sway = 0) {
    const k = Math.tan(skewDeg * Math.PI / 180), mid = BANNER_SPLIT_R - BANNER_SPLIT_L;
    const mx = (x) => x <= BANNER_SPLIT_L ? x : x >= BANNER_SPLIT_R ? x + dw : BANNER_SPLIT_L + (x - BANNER_SPLIT_L) * (mid + dw) / mid;
    const h = BANNER_H + dy, start = h * BANNER_BEND;
    const bend = (p) => {
        const t = (p[1] - start) / (h - start);
        if (t > 0) p[0] += sway * t * t;
        return p;
    };
    return (x, y) => {
        if (y < BANNER_JOIN) return bend([mx(x), y]);
        const d = y - BANNER_JOIN;
        return bend([mx(x) + k * d, y + dy + ripple * Math.sin(wave - x * 0.035) * (d / 100)]);
    };
}

// The cloth: one SVG frame (outline, inner line, ornaments, hanging pieces, medallion) that
// bannerFrame reshapes to the card's size, plus the headpiece.
function bannerClothHtml(variant, typeValue, withHeadpiece) {
    const b = BANNERS[variant - 1];
    const still = bannerMapper(0, 0, 0, 0, 0);
    const path = (cls, d) => `<path class="${cls}" data-d="${d}" d="${mapBannerPath(d, still)}"/>`;
    const hangs = b.hang.map((h) =>
        `<g class="banner-orn${h.swing ? ' banner-hang' : ''}" data-at="${h.at[0]} ${h.at[1]}">${h.swing ? `<g class="banner-swing">${h.html}</g>` : h.html}</g>`
    ).join('');
    // The SVG box is padded by BANNER_PAD on all sides (plus BANNER_PAD_B below) so nothing it paints, shadows and
    // swaying tails included, falls outside it and gets clipped.
    return `<div class="banner-cloth" aria-hidden="true"><svg class="banner-frame" width="${BANNER_W + 2 * BANNER_PAD}" height="${BANNER_H + BANNER_PAD + BANNER_PAD_B}">`
        + `<g transform="translate(${BANNER_PAD} ${BANNER_PAD})">`
        + path('banner-outline', b.outline)
        + path(`banner-orn${b.stitch ? ' banner-stitch' : ''}`, b.inner)
        + b.orn.map((d) => path('banner-orn', d)).join('')
        + hangs
        + '<circle class="banner-ring-o" cx="64" cy="26" r="24"/><circle class="banner-ring-i" cx="64" cy="26" r="20"/>'
        + `<svg class="type-icon" x="50" y="12" width="28" height="28" viewBox="0 0 24 24">${TYPE_ICON_PATHS[typeValue] || ''}</svg>`
        + '</g></svg>'
        + (withHeadpiece ? headpieceHtml(variant) : '')
        + '</div>';
}

// Frame loop: reshape every visible banner each frame (read all layout first, then write).
const bannerReduced = window.matchMedia('(prefers-reduced-motion: reduce)');
function bannerFrame(now) {
    const moving = !bannerReduced.matches;
    const t = now / 1000;
    const cards = [];
    for (const c of document.querySelectorAll('.campaign-item')) {
        let s = c._banner;
        if (!s) {
            const cloth = c.querySelector('.banner-cloth');
            if (!cloth) continue;
            s = c._banner = {
                gust: 0, key: '', cloth,
                paths: [...cloth.querySelectorAll('path[data-d]')],
                hangs: [...cloth.querySelectorAll('g[data-at]')].map((g) => ({ g, at: g.dataset.at.split(' ').map(Number) })),
                period: Number(c.dataset.period) || 3.3,
                phase: Number(c.dataset.phase) || 0,
                swayPeriod: Number(c.dataset.swayPeriod) || 14,
                swayPhase: Number(c.dataset.swayPhase) || 0
            };
        }
        const r = c.getBoundingClientRect();
        cards.push({
            s,
            w: s.cloth.offsetWidth, h: s.cloth.offsetHeight,
            visible: r.bottom > -150 && r.top < window.innerHeight + 150,
            hover: c.classList.contains('on-cloth')
        });
    }
    for (const { s, w, h, visible, hover } of cards) {
        if (!w || !visible) continue;
        s.gust += ((moving && hover ? 1 : 0) - s.gust) * 0.06;
        const key = w + 'x' + h;
        if (!moving && s.key === key) continue;
        s.key = moving ? '' : key;
        const wave = 2 * Math.PI * t / s.period + s.phase;
        const map = bannerMapper(w - BANNER_W, h - BANNER_H,
            moving ? 4 * (1 + s.gust) * Math.sin(wave) : 0, moving ? 3 * (1 + 1.3 * s.gust) : 0, wave * 1.7,
            moving ? BANNER_SWAY_PX * Math.sin(2 * Math.PI * t / s.swayPeriod + s.swayPhase) : 0);
        for (const p of s.paths) p.setAttribute('d', mapBannerPath(p.dataset.d, map));
        for (const { g, at } of s.hangs) {
            const p = map(at[0], at[1]);
            g.setAttribute('transform', `translate(${(p[0] - at[0]).toFixed(2)} ${(p[1] - at[1]).toFixed(2)})`);
        }
    }
    requestAnimationFrame(bannerFrame);
}
if (document.getElementById('campaignsList')) requestAnimationFrame(bannerFrame);

// Clicking the banner cloth opens its game page; the empty corners around the
// silhouette, the banner's own links and buttons and the medallion don't.
// Keyboard users reach the page via the title link.
function bannerOnCloth(card, event) {
    const outline = card.querySelector('.banner-outline');
    const ctm = outline?.getScreenCTM();
    if (!ctm || !outline.isPointInFill) return true;
    const pt = new DOMPoint(event.clientX, event.clientY).matrixTransform(ctm.inverse());
    return outline.isPointInFill(pt);
}
function bannerGameUrl(event) {
    const card = event.target.closest('.campaign-item');
    if (!card || event.target.closest('a, button, input, select, textarea, label, [tabindex]')) return '';
    if (!bannerOnCloth(card, event)) return '';
    if (String(window.getSelection?.() || '')) return ''; // let people select text
    return card.querySelector('.campaign-title-link')?.getAttribute('href') || '';
}
const bannerList = document.getElementById('campaignsList');
// Hover (gust, outline colour, cursor) follows the cloth's silhouette, not the card's
// rectangle: .on-cloth stands in for :hover. Only real mice count (touch has no hover).
// Re-checked on scroll and after a re-render, since the page can move under a still pointer.
// The medallion hotspot counts as cloth for hover.
let hoverCard = null;
let clothPointer = null;
function updateClothHover() {
    if (!bannerList) return;
    const el = clothPointer && document.elementFromPoint(clothPointer.clientX, clothPointer.clientY);
    const card = el && bannerList.contains(el) ? el.closest('.campaign-item') : null;
    if (hoverCard && hoverCard !== card) hoverCard.classList.remove('on-cloth');
    hoverCard = card;
    if (card) card.classList.toggle('on-cloth', !!el.closest('.banner-type') || bannerOnCloth(card, clothPointer));
}
if (bannerList) {
    bannerList.addEventListener('pointermove', (event) => {
        if (event.pointerType !== 'mouse') return;
        clothPointer = { clientX: event.clientX, clientY: event.clientY };
        updateClothHover();
    });
    bannerList.addEventListener('pointerleave', (event) => {
        if (event.pointerType !== 'mouse') return;
        clothPointer = null;
        updateClothHover();
    });
    bannerList.addEventListener('pointerdown', (event) => {
        if (event.pointerType === 'mouse') return;
        clothPointer = null;
        updateClothHover();
        bannerList.querySelectorAll('.on-cloth').forEach((c) => c.classList.remove('on-cloth'));
    });
    window.addEventListener('scroll', () => { if (clothPointer) updateClothHover(); }, { passive: true });
    bannerList.addEventListener('click', (event) => {
        const url = bannerGameUrl(event);
        if (!url) return;
        if (event.ctrlKey || event.metaKey || event.shiftKey) window.open(url, '_blank', 'noopener');
        else window.location.href = url;
    });
    bannerList.addEventListener('auxclick', (event) => {
        const url = event.button === 1 ? bannerGameUrl(event) : '';
        if (url) window.open(url, '_blank', 'noopener');
    });
}

// Banner order: those needing the user's action first, then by status. Array.sort is stable,
// so ties keep the API's order.
const BANNER_STATUS_ORDER = ['Pitch', 'Ongoing', 'On Hiatus', 'Finished', 'Cancelled'];
function bannerStatusRank(status) {
    const i = BANNER_STATUS_ORDER.indexOf(status);
    return i === -1 ? BANNER_STATUS_ORDER.length : i;
}

// Reordering slides banners to their new place (FLIP): note each banner's position before the
// list changes, then animate every moved banner from its old position to the new one.
function bannerPositions() {
    const pos = new Map();
    for (const el of document.querySelectorAll('.campaign-item')) {
        const r = el.getBoundingClientRect();
        pos.set(el.id, [r.left, r.top]);
    }
    return pos;
}

function slideBanners(before) {
    if (bannerReduced.matches) return;
    for (const el of document.querySelectorAll('.campaign-item')) {
        const old = before.get(el.id);
        if (!old) continue;
        const r = el.getBoundingClientRect();
        const dx = old[0] - r.left, dy = old[1] - r.top;
        if (Math.abs(dx) < 1 && Math.abs(dy) < 1) continue;
        el.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: 'none' }],
            { duration: 700, easing: 'cubic-bezier(0.25, 0.8, 0.25, 1)' });
    }
}

// CSS class suffix for a campaign status (status-pitch, status-hiatus, ...).
function campaignStatusClass(status) {
    const statusMap = {
        Pitch: 'pitch',
        Ongoing: 'ongoing',
        Finished: 'finished',
        'On Hiatus': 'hiatus',
        Cancelled: 'cancelled'
    };
    return statusMap[status] || String(status).toLowerCase().replace(/\s+/g, '-');
}

// Small square avatar: uploaded picture, or the default letter. Linked to the profile unless link is false.
function userAvatarHtml(user, link = true) {
    if (!user) return '';
    const name = user.display_name || user.name || 'Unknown';
    const inner = user.avatar_url
        ? `<img class="user-avatar" src="${escapeHtml(user.avatar_url)}" alt="">`
        : `<span class="user-avatar user-avatar--letter" aria-hidden="true">${escapeHtml(user.avatar_letter || '?')}</span>`;
    if (!link || !(user.profile_url || user.id)) return inner;
    const href = user.profile_url || `/profile?user_id=${encodeURIComponent(user.id)}`;
    return `<a href="${escapeHtml(href)}" class="user-avatar-link" title="${escapeHtml(name)}" aria-label="${escapeHtml(name)}">${inner}</a>`;
}

// Reply glyphs for the calendar. The check and cross are inline SVG: the text glyphs fall back to a font where the check reads like a root sign.
const REPLY_GLYPHS = {
    accepted: '<svg class="reply-glyph" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 13l5 6L20 5"/></svg>',
    tentative: '?',
    declined: '<svg class="reply-glyph" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 5l14 14M19 5L5 19"/></svg>',
    awaiting: '…'
};

// Reply words, one vocabulary wherever a person reads a reply state (API values are unchanged).
const REPLY_WORDS = { Accepted: 'Aye', Tentative: 'Perhaps', Declined: 'Nay', '': 'No reply yet' };
function replyWord(participation) {
    return REPLY_WORDS[participation || ''] || REPLY_WORDS[''];
}

// Zone abbreviation for a session's own timezone ("CET"): times are shown as stored, never converted.
// An empty timezone has no label. dateStr (YYYY-MM-DD) picks the right side of a DST change.
function zoneAbbr(timezone, dateStr) {
    if (!timezone) return '';
    try {
        const at = dateStr ? new Date(dateStr + 'T12:00:00Z') : new Date();
        const parts = new Intl.DateTimeFormat('en-GB', { timeZone: timezone, timeZoneName: 'short' }).formatToParts(at);
        return (parts.find((p) => p.type === 'timeZoneName') || {}).value || '';
    } catch (error) {
        return '';
    }
}

// "18:00 CET", or just the time when the session has no zone.
function timeWithZone(time, session) {
    const zone = zoneAbbr(session?.timezone, session?.date);
    return zone ? time + ' ' + zone : time;
}

// "18:00–22:00 CET"
function sessionRangeWithZone(session) {
    return timeWithZone((session?.time || '') + '–' + getSessionEndTime(session), session);
}

// "Fri, Nov 13" from a YYYY-MM-DD date.
function formatShortDate(dateStr) {
    return parseDateLocal(dateStr).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
}

// Campaigns Alpine component
window.campaignsPage = function () {
    return {
        displayName: 'User',
        isAdmin: false,
        currentUserId: '',
        campaigns: [],
        loading: true,
        modalOpen: false,
        messageVisible: false,
        messageText: '',
        messageType: '',
        formData: {
            title: '',
            summary: '',
            description: '',
            type: 'Campaign',
            system: '',
            desired_player_count: ''
        },
        systems: [],
        campaignTypes: CAMPAIGN_TYPES,
        editingCampaignId: '',
        returnTo: '', // set when a dialog was opened by a game page link; closing it goes back there
        signupsModalOpen: false,
        signupsCampaignId: '',
        signupsCampaignTitle: '',
        signups: [],
        signupsAcceptedCount: 0,
        signupsDesiredCount: 0,
        signupsOpen: false,
        showDeclinedSignups: false,
        confirmDialog: { open: false, title: '', message: '', confirmLabel: '', resolve: null },
        messageAction: null,
        _messageTimer: null,
        otherOpen: true,

        // Sections of the Games page: your games, games open to join, everything else.
        get myGames() { return this.campaigns.filter((c) => this.isMember(c)); },
        get openGames() { return this.campaigns.filter((c) => this.isOpenToJoin(c)); },
        get otherGames() { return this.campaigns.filter((c) => !this.isMember(c) && !this.isOpenToJoin(c)); },

        isMember(c) {
            return (!!this.currentUserId && c?.dm === this.currentUserId)
                || (Array.isArray(c?.players) && c.players.includes(this.currentUserId));
        },

        // Not a member, the game is live and sign-ups are open (or your own sign-up is waiting).
        isOpenToJoin(c) {
            if (this.isMember(c) || c?.status === 'Finished' || c?.status === 'Cancelled') return false;
            return c?.sign_ups_open === true || c?.current_user_signup_status === 'Pending';
        },

        get editingCampaign() {
            return this.campaigns.find((c) => c.id === this.editingCampaignId) || null;
        },

        async init() {
            try {
                const userResponse = await fetch('/auth/current-user');
                if (!userResponse.ok) {
                    window.location.href = '/login.html';
                    return;
                }

                const user = await userResponse.json();
                this.displayName = user.display_name || user.username;
                this.isAdmin = (user.roles || []).includes('admin');
                this.currentUserId = user.user_id || user.id || '';

                if (!this._signupButtonHandler) {
                    this._signupButtonHandler = (event) => {
                        const followBtn = event.target.closest('.campaign-follow-btn');
                        if (followBtn) {
                            event.preventDefault();
                            this.toggleFollow(followBtn.dataset.campaignId || '', followBtn.dataset.following === 'true');
                            return;
                        }
                        const withdrawBtn = event.target.closest('.campaign-withdraw-btn');
                        if (withdrawBtn) {
                            event.preventDefault();
                            this.withdrawSignup(withdrawBtn.dataset.campaignId || '', withdrawBtn.dataset.campaignTitle || '', withdrawBtn.dataset.withdrawKind || '');
                            return;
                        }
                        const signupBtn = event.target.closest('.campaign-signup-btn:not([disabled])');
                        if (signupBtn) {
                            event.preventDefault();
                            this.signUp(signupBtn.dataset.campaignId || '');
                            return;
                        }
                        const manageBtn = event.target.closest('.campaign-signups-btn');
                        if (manageBtn) {
                            event.preventDefault();
                            this.openSignupsModal(manageBtn.dataset.campaignId || '', manageBtn.dataset.campaignTitle || '');
                            return;
                        }
                        const editBtn = event.target.closest('.campaign-edit-btn');
                        if (editBtn) {
                            event.preventDefault();
                            this.openEditModal(editBtn.dataset.campaignId || '');
                        }
                    };
                    document.addEventListener('click', this._signupButtonHandler);
                }
                if (!this._notificationsHandler) {
                    // New notifications may change the cards' pending-action highlights.
                    this._notificationsHandler = () => this.loadCampaigns(true);
                    window.addEventListener('notifications:new', this._notificationsHandler);
                }
                if (!this._pageshowHandler) {
                    // Back/forward can restore this page from the bfcache with stale counts (e.g. unanswered sessions).
                    this._pageshowHandler = (event) => { if (event.persisted) this.loadCampaigns(true); };
                    window.addEventListener('pageshow', this._pageshowHandler);
                }
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
                return;
            }

            try { this.otherOpen = localStorage.getItem('waystone-other-games') !== 'closed'; } catch (e) { /* storage unavailable: stay open */ }
            await this.loadCampaigns();
            if (location.hash) {
                // A link to a row in the collapsed section opens it (without remembering that choice).
                const id = decodeURIComponent(location.hash.slice(1)).replace(/^campaign-/, '');
                if (this.otherGames.some((c) => c.id === id)) this.otherOpen = true;
                await this.$nextTick();
                document.getElementById(location.hash.slice(1))?.scrollIntoView({ block: 'start' });
            }
            await this.loadSystems();
            this.openDeepLink();
        },

        // Links from the game page: ?edit=ID or ?signups=ID open that dialog,
        // and closing it returns to the game page.
        openDeepLink() {
            const params = new URLSearchParams(location.search);
            const find = (key) => this.campaigns.find((c) => c.id === params.get(key));
            const edit = find('edit'), signups = find('signups');
            const target = (edit?.can_manage ? edit : null)
                || (signups && signups.dm === this.currentUserId ? signups : null);
            if (!target) {
                if (['edit', 'signups'].some((key) => params.has(key))) {
                    history.replaceState(null, '', '/campaigns');
                    this.showMessage("Game not found or you can't open that dialog", 'error');
                }
                return;
            }
            this.returnTo = '/games/' + encodeURIComponent(target.id);
            if (target === edit) {
                this.openEditModal(edit.id);
            } else {
                this.openSignupsModal(signups.id, signups.title);
            }
            history.replaceState(null, '', '/campaigns#campaign-' + target.id);
            document.getElementById('campaign-' + target.id)?.scrollIntoView({ block: 'start' });
        },

        // Leaves for the page that opened the dialog, if any. replace() keeps the history
        // from growing: the list page with the dialog is not a stop worth going back to.
        leaveToReturnPage() {
            if (!this.returnTo) return;
            const to = this.returnTo;
            this.returnTo = '';
            window.location.replace(to);
        },

        async loadSystems() {
            try {
                const response = await fetch('/api/systems');
                if (!response.ok) {
                    throw new Error('request failed');
                }
                this.systems = await response.json();
            } catch (error) {
                this.systems = [];
                this.showMessage('Failed to load systems', 'error');
            }
        },

        async handleLogout() {
            try {
                const response = await fetch('/auth/logout', { method: 'POST' });
                if (response.ok) {
                    window.location.href = '/login.html';
                }
            } catch (error) {
                this.showMessage('Logout failed: ' + error.message, 'error');
            }
        },

        toggleOther() {
            this.otherOpen = !this.otherOpen;
            try { localStorage.setItem('waystone-other-games', this.otherOpen ? 'open' : 'closed'); } catch (e) { /* not persisted */ }
        },

        openModal() {
            this.editingCampaignId = '';
            this.formData = { title: '', summary: '', description: '', type: 'Campaign', system: '', desired_player_count: '' };
            this.modalOpen = true;
        },

        openEditModal(campaignId) {
            const c = this.campaigns.find((x) => x.id === campaignId);
            if (!c) {
                return;
            }
            this.editingCampaignId = c.id;
            this.formData = {
                title: c.title || '',
                summary: c.summary || '',
                description: c.description || '',
                type: c.type || 'Campaign',
                system: c.system || '',
                desired_player_count: parseInt(c.desired_player_count, 10) || ''
            };
            this.modalOpen = true;
        },

        closeModal() {
            this.modalOpen = false;
            this.leaveToReturnPage();
        },

        handleModalBackdropClick(event) {
            if (event.target.id === 'createCampaignModal') {
                this.closeModal();
            }
        },

        async handleCreateCampaign() {
            const { title, summary, description, type, system, desired_player_count } = this.formData;
            if (!title || !summary || !description) {
                this.showMessage('Title, summary and description are required', 'error');
                return;
            }
            // x-model.number turns a cleared field into null, an untouched one stays ''
            const unlimited = desired_player_count === '' || desired_player_count == null;
            if (!unlimited && desired_player_count < 1) {
                this.showMessage('Desired player count must be at least 1', 'error');
                return;
            }

            const editing = !!this.editingCampaignId;
            try {
                const response = await fetch(editing ? `/api/campaigns/${encodeURIComponent(this.editingCampaignId)}` : '/api/campaigns', {
                    method: editing ? 'PUT' : 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ title, summary, description, type, system: (system || '').trim(), desired_player_count: Number(desired_player_count) || 0 })
                });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || (editing ? 'Failed to update game' : 'Failed to create game'), 'error');
                    return;
                }

                if (editing && this.returnTo) {
                    // the game page shows "Game updated"
                    const to = this.returnTo;
                    this.returnTo = '';
                    window.location.replace(to + '?updated=1');
                    return;
                }
                this.showMessage(editing ? 'Game updated' : 'Game created successfully!', 'success');
                this.closeModal();
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage((editing ? 'Error updating game: ' : 'Error creating game: ') + error.message, 'error');
            }
        },

        async setCampaignStatus(status) {
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.editingCampaignId)}/status`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ status })
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to change status', 'error');
                } else {
                    this.showMessage(data.message, 'success');
                }
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage('Error changing status: ' + error.message, 'error');
            }
        },

        async deleteCampaign() {
            const campaign = this.editingCampaign;
            if (!campaign) {
                return;
            }
            const ok = await this.askConfirm({
                title: 'Delete game',
                message: `Delete "${campaign.title}"? This also deletes its sessions and sign-ups and cannot be undone.`,
                confirmLabel: 'Delete'
            });
            if (!ok) {
                return;
            }
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(campaign.id)}`, { method: 'DELETE' });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to delete game', 'error');
                    return;
                }
                this.returnTo = ''; // the game page is gone; stay on the list
                this.closeModal();
                this.showMessage(data.message || 'Game deleted', 'success');
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage('Error deleting game: ' + error.message, 'error');
            }
        },

        // quiet: background refresh without the loading state; keeps the list on failure.
        async loadCampaigns(quiet = false) {
            if (!quiet) this.loading = true;
            try {
                const response = await fetch('/api/campaigns');
                if (!response.ok) {
                    throw new Error('Failed to load games');
                }
                const data = await response.json();
                const before = bannerPositions();
                this.campaigns = (Array.isArray(data) ? data : []).sort((a, b) =>
                    (this.campaignNeedsAction(b) - this.campaignNeedsAction(a))
                    || (bannerStatusRank(a?.status) - bannerStatusRank(b?.status)));
                await this.$nextTick();
                slideBanners(before);
                updateClothHover();
            } catch (error) {
                if (quiet) {
                    console.error('Failed to refresh campaigns:', error);
                    return;
                }
                this.showMessage('Error loading games: ' + error.message, 'error');
                this.campaigns = [];
            } finally {
                if (!quiet) this.loading = false;
            }
        },

        renderCampaignHtml(campaign) {
            const id = escapeHtml(campaign?.id || '');
            const title = campaign?.title || 'Untitled Game';
            const summary = campaign?.summary || 'No summary available.';
            const status = campaign?.status || 'Unknown';
            const signUpsOpen = campaign?.sign_ups_open === true;
            const next = campaign?.next_session;
            const isDM = !!this.currentUserId && campaign?.dm === this.currentUserId;
            const campaignPlayers = Array.isArray(campaign?.players) ? campaign.players : [];
            const isPlayer = campaignPlayers.includes(this.currentUserId);
            const isMember = isDM || isPlayer;
            const signupStatus = campaign?.current_user_signup_status || '';
            const unansweredCount = isPlayer ? (parseInt(campaign?.unanswered_sessions_count, 10) || 0) : 0;
            const pendingSignupsCount = isDM ? (parseInt(campaign?.pending_signups_count, 10) || 0) : 0;
            const needsAction = unansweredCount > 0 || pendingSignupsCount > 0;
            // Characters are optional: a nudge, never needs-action (the outline stays quiet).
            const characterNudge = campaign?.character_nudge === true;
            const maxPlayers = parseInt(campaign?.desired_player_count, 10) || 0;

            // Facts: seats and sign-ups, then the next session.
            let factsLine;
            if (!isMember && signupStatus === 'Declined') {
                factsLine = 'Sign-up declined';
            } else if (!isMember && signupStatus === 'Kicked') {
                factsLine = 'Removed from this game';
            } else {
                const seats = maxPlayers > 0
                    ? `${campaignPlayers.length} of ${maxPlayers} seats taken.`
                    : `${campaignPlayers.length} player${campaignPlayers.length === 1 ? '' : 's'}, no seat limit.`;
                factsLine = `${seats} Sign-ups ${signUpsOpen ? 'open' : 'closed'}.`;
            }
            const nextSession = next
                ? `Next: ${formatShortDate(next.date)}, ${timeWithZone(next.time, next)}`
                : 'No session scheduled';
            const nextTimezone = next?.timezone || '';

            const isFollowing = !!campaign?.is_following;
            const followBtnHtml = (!isMember && this.currentUserId)
                ? `<button type="button" class="campaign-action-btn campaign-follow-btn" data-campaign-id="${id}" data-following="${isFollowing}" title="${isFollowing ? 'Stop getting notifications for this game' : 'Get notified about status changes, sign-ups and sessions'}">${isFollowing ? 'Unfollow' : 'Follow'}</button>`
                : '';
            const calendarBtnHtml = isMember
                ? `<a class="campaign-action-btn campaign-calendar-btn" href="/games/${encodeURIComponent(campaign?.id ?? '')}/calendar"${unansweredCount > 0 ? ` title="${unansweredCount} suggested session${unansweredCount === 1 ? '' : 's'} awaiting your response" aria-label="Calendar, ${unansweredCount} awaiting your response"` : ''}>Calendar${unansweredCount > 0 ? `<span class="campaign-action-count">${unansweredCount}</span>` : ''}</a>`
                : '';
            const editBtnHtml = campaign?.can_manage
                ? `<button type="button" class="campaign-action-btn campaign-edit-btn" data-campaign-id="${id}">Edit</button>`
                : '';

            // One clear action per banner (see BANNER_DESIGN_GUIDE.md, Buttons).
            let buttonsHtml;
            if (isDM) {
                buttonsHtml = `<button type="button" class="campaign-action-btn campaign-signups-btn${pendingSignupsCount > 0 ? ' campaign-primary-btn' : ''}" data-campaign-id="${id}" data-campaign-title="${escapeHtml(campaign?.title || '')}"${pendingSignupsCount > 0 ? ` title="${pendingSignupsCount} pending sign-up${pendingSignupsCount === 1 ? '' : 's'} awaiting your decision" aria-label="Sign-ups, ${pendingSignupsCount} pending"` : ''}>Sign-ups${pendingSignupsCount > 0 ? `<span class="campaign-action-count">${pendingSignupsCount}</span>` : ''}</button>${calendarBtnHtml}${editBtnHtml}`;
            } else if (isPlayer) {
                buttonsHtml = (unansweredCount > 0
                    ? `<a class="campaign-action-btn campaign-reply-btn span-2" href="/games/${encodeURIComponent(campaign?.id ?? '')}">Reply</a>`
                    : characterNudge
                        ? `<a class="campaign-action-btn campaign-primary-btn campaign-character-btn" href="/games/${encodeURIComponent(campaign?.id ?? '')}?character=1">Choose<br>character</a>`
                        : '') + calendarBtnHtml + editBtnHtml;
            } else if (signupStatus === 'Pending') {
                buttonsHtml = `<button type="button" class="campaign-action-btn campaign-withdraw-btn" data-campaign-id="${id}" data-campaign-title="${escapeHtml(campaign?.title || '')}" data-withdraw-kind="withdraw" title="Withdraw your sign-up">Withdraw</button>${followBtnHtml}${editBtnHtml}`;
            } else if (signUpsOpen && signupStatus !== 'Declined' && signupStatus !== 'Kicked') {
                buttonsHtml = `<button type="button" class="campaign-action-btn campaign-signup-btn span-2" data-campaign-id="${id}">Sign up</button>${followBtnHtml}${editBtnHtml}`;
            } else {
                buttonsHtml = followBtnHtml + editBtnHtml;
            }

            const statusClass = campaignStatusClass(status);

            const typeInfo = CAMPAIGN_TYPES.find((t) => t.value === campaign?.type) || CAMPAIGN_TYPES.find((t) => t.value === 'Campaign');
            const typeTip = escapeHtml(`${typeInfo.label}: ${typeInfo.description}`);
            const typeHotspotHtml = `<span class="banner-type" tabindex="0" role="img" aria-label="${typeTip}" data-tooltip="${typeTip}"></span>`;
            const variant = bannerVariant(campaign?.id);
            const seed = bannerSeed(campaign?.id);
            const banner = BANNERS[variant - 1];

            const systemName = campaign?.system || '';
            const systemHtml = campaign?.system_logo_url
                ? `<img class="campaign-system-logo" src="${escapeHtml(campaign.system_logo_url)}" alt="${escapeHtml(systemName)}" title="${escapeHtml(systemName)}">`
                : (systemName ? `<span class="campaign-system-badge">${escapeHtml(systemName)}</span>` : '');

            const dmUser = campaign?.dm_user;
            const dmHtml = dmUser
                ? `<a href="${escapeHtml(dmUser.profile_url || `/profile?user_id=${encodeURIComponent(dmUser.id)}`)}" class="campaign-dm-link">${userAvatarHtml(dmUser, false)}<span class="user-button user-button--dm">${escapeHtml(dmUser.display_name || dmUser.name || 'Unknown')}</span></a>`
                : `<span class="campaign-nameplate">${escapeHtml(campaign?.dm || 'Unassigned')}</span>`;

            const playerUsers = Array.isArray(campaign?.player_users) ? campaign.player_users : [];
            const players = Array.isArray(campaign?.players) ? campaign.players : [];
            let playersHtml = '';
            if (playerUsers.length === 0 && players.length === 0) {
                playersHtml = '<span class="campaign-meta">No players yet</span>';
            } else if (playerUsers.length > 0) {
                playersHtml = playerUsers.map((player) => userAvatarHtml(player)).join('');
            } else {
                playersHtml = players.map((player) =>
                    `<span class="campaign-nameplate">${escapeHtml(player)}</span>`
                ).join('');
            }

            let pendingNoticeHtml = '';
            if (unansweredCount > 0) {
                pendingNoticeHtml = `<p class="pending-notice"><span aria-hidden="true">✦</span> ${unansweredCount === 1 ? 'A session awaits thy reply' : `${unansweredCount} sessions await thy reply`}</p>`;
            } else if (pendingSignupsCount > 0) {
                pendingNoticeHtml = `<p class="pending-notice"><span aria-hidden="true">✦</span> ${pendingSignupsCount} sign-up${pendingSignupsCount === 1 ? '' : 's'} to review</p>`;
            } else if (characterNudge) {
                pendingNoticeHtml = '<p class="pending-notice"><span aria-hidden="true">✦</span> Choose your character</p>';
            }

            return `
                <article class="campaign-item status-${escapeHtml(statusClass)}${needsAction ? ' needs-action' : ''}" id="campaign-${escapeHtml(campaign?.id ?? 'unknown')}" data-period="${banner.period + (seed % 5) / 10}" data-phase="${banner.phase + seed % 7}" data-sway-period="${13 + (seed % 21) / 10}" data-sway-phase="${(seed % 63) / 10}" style="--swing-dur:${2.1 + (seed % 5) / 10}s;--swing-delay:-${(seed % 23) / 10}s">
                    ${bannerClothHtml(variant, typeInfo.value, !pendingNoticeHtml)}
                    ${typeHotspotHtml}
                    ${pendingNoticeHtml ? `<div class="campaign-notice">${pendingNoticeHtml}</div>` : ''}
                    <section class="campaign-row campaign-row-top">
                        <div class="campaign-badges">
                            <span class="campaign-status-badge">${escapeHtml(status)}</span>
                            ${systemHtml}
                        </div>
                        <div class="campaign-title-container">
                            <h3 class="campaign-title"><a href="/games/${encodeURIComponent(campaign?.id ?? '')}" class="campaign-title-link">${escapeHtml(title)}</a></h3>
                        </div>
                        <p class="campaign-summary-text">${escapeHtml(summary)}</p>
                    </section>

                    <section class="campaign-row campaign-row-middle" aria-label="Game details">
                        <div class="banner-separator" aria-hidden="true"><span></span><svg width="14" height="10" viewBox="0 0 14 10"><path d="M7 1 L13 5 L7 9 L1 5 Z"/></svg><span></span></div>
                        <div class="campaign-dm"><span class="visually-hidden">Dungeon master: </span>${dmHtml}</div>
                        <div class="campaign-players"><span class="visually-hidden">Players: </span>${playersHtml}</div>
                        <p class="campaign-meta">${escapeHtml(factsLine)}</p>
                        <p class="campaign-meta"${nextTimezone ? ` title="${escapeHtml(nextTimezone)}"` : ''}>${escapeHtml(nextSession)}</p>
                    </section>

                    <section class="campaign-row campaign-row-bottom">
                        ${buttonsHtml}
                    </section>
                </article>
            `;
        },

        // A compact ledger row for a game that is neither yours nor open to join.
        renderOtherRowHtml(campaign) {
            const id = escapeHtml(campaign?.id || '');
            const status = campaign?.status || 'Unknown';
            const typeInfo = CAMPAIGN_TYPES.find((t) => t.value === campaign?.type) || CAMPAIGN_TYPES.find((t) => t.value === 'Campaign');
            const dmUser = campaign?.dm_user;
            const dm = dmUser ? (dmUser.display_name || dmUser.name || 'Unknown') : (campaign?.dm || 'Unassigned');
            const isFollowing = !!campaign?.is_following;
            return `
                <div role="listitem" class="ledger-row ledger-row--game status-${escapeHtml(campaignStatusClass(status))}" id="campaign-${id}">
                    <svg class="type-icon ledger-type" viewBox="0 0 24 24" role="img" aria-label="${escapeHtml(typeInfo.label)}"><title>${escapeHtml(typeInfo.label)}</title>${TYPE_ICON_PATHS[typeInfo.value] || ''}</svg>
                    <a class="ledger-title" href="/games/${encodeURIComponent(campaign?.id ?? '')}">${escapeHtml(campaign?.title || 'Untitled Game')}</a>
                    <span class="ledger-meta ledger-system">${escapeHtml(campaign?.system || '')}</span>
                    <span class="ledger-meta ledger-dm"><span class="ledger-crown" aria-hidden="true">♛</span> <span class="visually-hidden">Dungeon master: </span>${escapeHtml(dm)}</span>
                    <span class="ledger-status"><span class="campaign-status-badge">${escapeHtml(status)}</span></span>
                    ${this.currentUserId ? `<button type="button" class="campaign-action-btn campaign-follow-btn ledger-follow" data-campaign-id="${id}" data-following="${isFollowing}">${isFollowing ? 'Unfollow' : 'Follow'}</button>` : ''}
                </div>
            `;
        },

        async signUp(campaignId) {
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(campaignId)}/signups`, { method: 'POST' });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to sign up', 'error');
                    await this.loadCampaigns();
                    return;
                }
                this.showMessage(data.message || 'Signed up successfully', 'success');
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage('Error signing up: ' + error.message, 'error');
            }
        },

        async toggleFollow(campaignId, following) {
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(campaignId)}/follow`, { method: following ? 'DELETE' : 'PUT' });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to update follow', 'error');
                    await this.loadCampaigns();
                    return;
                }
                const title = this.campaigns.find((c) => c.id === campaignId)?.title || 'this game';
                this.showMessage(following ? `No longer following ${title}` : `Now following ${title}`, 'success', {
                    label: 'Undo',
                    run: () => this.toggleFollow(campaignId, !following)
                });
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage('Error updating follow: ' + error.message, 'error');
            }
        },

        async withdrawSignup(campaignId, title, kind) {
            const ok = await this.askConfirm(kind === 'leave'
                ? {
                    title: 'Leave game',
                    message: `Leave "${title}"? You'll stop being a player; you can sign up again if sign-ups are open.`,
                    confirmLabel: 'Leave'
                }
                : {
                    title: 'Withdraw sign-up',
                    message: `Withdraw your sign-up for "${title}"?`,
                    confirmLabel: 'Withdraw'
                });
            if (!ok) return;
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(campaignId)}/signups/${encodeURIComponent(this.currentUserId)}`, { method: 'DELETE' });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to withdraw', 'error');
                    await this.loadCampaigns();
                    return;
                }
                this.showMessage(data.message || 'Sign-up withdrawn', 'success');
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage('Error withdrawing: ' + error.message, 'error');
            }
        },

        // askConfirm opens the confirm modal and resolves to true on confirm, false on cancel.
        askConfirm({ title, message, confirmLabel }) {
            if (this.confirmDialog.resolve) this.confirmDialog.resolve(false);
            return new Promise((resolve) => {
                this.confirmDialog = { open: true, title, message, confirmLabel: confirmLabel || 'Confirm', resolve };
                this.$nextTick(() => this.$refs.confirmCancelBtn?.focus());
            });
        },

        closeConfirm(result) {
            const resolve = this.confirmDialog.resolve;
            this.confirmDialog = { open: false, title: '', message: '', confirmLabel: '', resolve: null };
            if (resolve) resolve(result);
        },

        async openSignupsModal(campaignId, campaignTitle) {
            this.signupsCampaignId = campaignId;
            this.signupsCampaignTitle = campaignTitle;
            this.signups = [];
            this.showDeclinedSignups = false;
            this.signupsModalOpen = true;
            await this.loadSignups();
        },

        closeSignupsModal() {
            this.signupsModalOpen = false;
            this.leaveToReturnPage();
        },

        handleSignupsBackdropClick(event) {
            if (event.target.id === 'signupsModal') {
                this.closeSignupsModal();
            }
        },

        async loadSignups() {
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.signupsCampaignId)}/signups`);
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to load sign-ups', 'error');
                    return;
                }
                this.signups = Array.isArray(data.signups) ? data.signups : [];
                this.signupsAcceptedCount = data.accepted_count || 0;
                this.signupsDesiredCount = parseInt(data.desired_player_count, 10) || 0;
                this.signupsOpen = data.sign_ups_open === true;
            } catch (error) {
                this.showMessage('Error loading sign-ups: ' + error.message, 'error');
            }
        },

        signupsByStatus(status) {
            return this.signups.filter((signup) => signup.status === status);
        },

        isCampaignFull() {
            return this.signupsDesiredCount > 0 && this.signupsAcceptedCount >= this.signupsDesiredCount;
        },

        formatSignupTime(ts) {
            return ts ? new Date(ts).toLocaleString() : '—';
        },

        async setSignupStatus(userId, status) {
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.signupsCampaignId)}/signups/${encodeURIComponent(userId)}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ status })
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to update sign-up', 'error');
                }
                await this.loadSignups();
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage('Error updating sign-up: ' + error.message, 'error');
            }
        },

        async kickPlayer(userId) {
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.signupsCampaignId)}/players/${encodeURIComponent(userId)}`, {
                    method: 'DELETE'
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to remove player', 'error');
                }
                await this.loadSignups();
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage('Error removing player: ' + error.message, 'error');
            }
        },

        async setSignupsOpen(open) {
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.signupsCampaignId)}/signups-open`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ open })
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to update sign-ups', 'error');
                } else {
                    this.showMessage(data.message, 'success');
                }
                await this.loadSignups();
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage('Error updating sign-ups: ' + error.message, 'error');
            }
        },

        // True when the user owes this game a reply: unanswered sessions (player) or pending sign-ups (DM).
        campaignNeedsAction(c) {
            const isDM = !!this.currentUserId && c?.dm === this.currentUserId;
            const isPlayer = Array.isArray(c?.players) && c.players.includes(this.currentUserId);
            return (isPlayer && (parseInt(c?.unanswered_sessions_count, 10) || 0) > 0)
                || (isDM && (parseInt(c?.pending_signups_count, 10) || 0) > 0);
        },

        // "6 games, 1 awaits thy reply": the page subtitle under the h1.
        gamesSubtitle() {
            const total = this.campaigns.length;
            const awaiting = this.campaigns.filter((c) => this.campaignNeedsAction(c)).length;
            const roll = `${total} game${total === 1 ? '' : 's'}`;
            return awaiting > 0 ? `${roll}, ${awaiting} await${awaiting === 1 ? 's' : ''} thy reply` : roll;
        },

        avatarHtml(user, link = true) {
            return userAvatarHtml(user, link);
        },

        parseDateLocal(dateStr) {
            return parseDateLocal(dateStr);
        },

        formatAgendaDate(session) {
            return formatShortDate(session.date);
        },

        // A toast; action is an optional { label, run } text button ("Undo").
        showMessage(text, type, action = null) {
            this.messageText = text;
            this.messageType = type;
            this.messageAction = action;
            this.messageVisible = true;
            clearTimeout(this._messageTimer);
            this._messageTimer = setTimeout(() => {
                this.messageVisible = false;
                this.messageAction = null;
            }, 5000);
        },

        runMessageAction() {
            const action = this.messageAction;
            this.messageVisible = false;
            this.messageAction = null;
            if (action) action.run();
        }
    };
};

function getDefaultTimezone() {
    try {
        return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
    } catch (error) {
        return 'UTC';
    }
}

function parseDateLocal(dateStr) {
    const [year, month, day] = String(dateStr).split('-').map(Number);
    return new Date(year, month - 1, day);
}

// End time "HH:MM" of a session; a malformed start time is returned as is.
function getSessionEndTime(session) {
    const time = session?.time || '';
    const duration = Number(session?.duration) || 0;
    const [hour, minute] = String(time).split(':').map((part) => Number(part));
    if (!Number.isFinite(hour) || !Number.isFinite(minute)) return time;
    const totalMinutes = (hour * 60) + minute + duration;
    const endHour = Math.floor((totalMinutes % (24 * 60)) / 60);
    const endMinute = totalMinutes % 60;
    return `${String(endHour).padStart(2, '0')}:${String(endMinute).padStart(2, '0')}`;
}

function getSessionCounts(session) {
    const accepted = Number(session?.accepted_count) || 0;
    const tentative = Number(session?.tentative_count) || 0;
    const declined = Number(session?.declined_count) || 0;
    const awaiting = Number(session?.pending_count) || 0;
    return { accepted, tentative, declined, awaiting, total: accepted + tentative + declined + awaiting };
}

function getTally(session) {
    const c = getSessionCounts(session);
    return [
        { kind: 'accepted', n: c.accepted },
        { kind: 'tentative', n: c.tentative },
        { kind: 'declined', n: c.declined },
        { kind: 'awaiting', n: c.awaiting }
    ];
}

// A suggestion with over 70% of all players accepted (awaiting players count; exactly 70% does not).
function isStrongCandidate(session) {
    const c = getSessionCounts(session);
    return session?.status === 'Suggested' && c.total > 0 && c.accepted * 10 > c.total * 7;
}

function getCandidateNote(session) {
    const c = getSessionCounts(session);
    return `Strong candidate: ${c.accepted} of ${c.total} accepted`;
}

function getDurationLabel(session) {
    const total = Number(session?.duration) || 0;
    const hours = Math.floor(total / 60);
    const minutes = total % 60;
    if (hours && minutes) return `${hours}h ${minutes}m`;
    if (hours) return `${hours}h`;
    return `${minutes}m`;
}

// "4 hours", "1 hour 30 minutes": the duration in words.
function getDurationWords(session) {
    const total = Number(session?.duration) || 0;
    const hours = Math.floor(total / 60);
    const minutes = total % 60;
    const parts = [];
    if (hours) parts.push(`${hours} hour${hours === 1 ? '' : 's'}`);
    if (minutes || !hours) parts.push(`${minutes} minute${minutes === 1 ? '' : 's'}`);
    return parts.join(' ');
}

function getTimezoneOptions() {
    let zones = [];
    try {
        zones = Intl.supportedValuesOf('timeZone');
    } catch (error) {
        zones = [];
    }
    for (const zone of [getDefaultTimezone(), 'UTC']) {
        if (!zones.includes(zone)) zones.push(zone);
    }
    return zones;
}

function escapeHtml(text) {
    if (!text) return '';
    const map = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#039;'
    };
    return String(text).replace(/[&<>"']/g, (m) => map[m]);
}

// Basic Markdown, safe by construction: the whole input is HTML-escaped first, then only
// whitelisted tags are produced. Supports paragraphs, **bold** (may contain *italic*),
// *italic*/_italic_, [text](url), "- " / "1. " lists (also right after a text line), "#" headings
// (rendered as h3/h4) and "> " quotes. Every pattern is bounded, so rendering stays linear.
function renderMarkdown(text) {
    const src = escapeHtml(String(text || '').replace(/\0/g, '').replace(/\r\n?/g, '\n'));
    const inline = (line) => {
        const links = [];
        let out = line.replace(/\[([^\[\]\n]{1,300})\]\(((?:[^()\s]|\([^()\s]*\)){1,2000})\)/g, (m, label, url) => {
            if (!/^(https?:\/\/|mailto:)\S+$/i.test(url)) return m;
            links.push(`<a href="${url}" target="_blank" rel="noopener noreferrer nofollow">${label}</a>`);
            return `\0${links.length - 1}\0`;
        });
        out = out
            .replace(/\*\*(?=[^\s*])([^\n]{1,1000}?)\*\*/g, '<strong>$1</strong>')
            .replace(/(^|[^*\w])\*([^*\s][^*\n]*?)\*(?![*\w])/g, '$1<em>$2</em>')
            .replace(/(^|[^\w])_([^_\s][^_\n]*?)_(?!\w)/g, '$1<em>$2</em>');
        return out.replace(/\0(\d+)\0/g, (m, i) => links[Number(i)]);
    };
    const kindOf = (l) => /^[-*]\s+/.test(l) ? 'ul' : /^\d+\.\s+/.test(l) ? 'ol' : /^&gt;\s?/.test(l) ? 'quote' : 'text';
    const strip = { ul: /^[-*]\s+/, ol: /^\d+\.\s+/, quote: /^&gt;\s?/ };
    // A run of lines of one kind becomes one element, so a list may follow a text line directly.
    const renderRun = (kind, lines) => {
        if (kind === 'text') return `<p>${lines.map(inline).join('<br>')}</p>`;
        const items = lines.map((l) => inline(l.replace(strip[kind], '')));
        if (kind === 'quote') return '<blockquote>' + items.join('<br>') + '</blockquote>';
        return `<${kind}>` + items.map((i) => `<li>${i}</li>`).join('') + `</${kind}>`;
    };
    const blocks = src.split(/\n[ \t]*\n/).map((b) => b.trim()).filter(Boolean);
    return blocks.map((block) => {
        let lines = block.split('\n');
        let out = '';
        const head = lines[0].match(/^(#{1,3})\s+(.*)$/);
        if (head) {
            const tag = head[1].length === 3 ? 'h4' : 'h3';
            out += `<${tag}>${inline(head[2])}</${tag}>`;
            lines = lines.slice(1);
        }
        let run = [], kind = '';
        for (const line of lines) {
            const k = kindOf(line);
            if (run.length && k !== kind) {
                out += renderRun(kind, run);
                run = [];
            }
            kind = k;
            run.push(line);
        }
        return out + (run.length ? renderRun(kind, run) : '');
    }).join('');
}

// Journal: "Penned by A · amended by B, C" from an entry or summary (author_user, amender_users).
// Repeated names collapse, so several anonymised amenders read as one "Deleted user".
function journalByline(j) {
    const name = (u) => (u && (u.display_name || u.name)) || 'Unknown';
    const others = [...new Set((j.amender_users || []).map(name))];
    return 'Penned by ' + name(j.author_user) + (others.length ? ' · amended by ' + others.join(', ') : '');
}

// First plain-text paragraph of an entry body, cut at 130 characters.
function journalExcerpt(body) {
    const para = String(body || '').split(/\n[ \t]*\n/).map((p) => p.trim()).filter(Boolean).find((p) => !/^#/.test(p)) || '';
    const plain = para.replace(/[*_>#]/g, '').replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').trim();
    return plain.length > 130 ? plain.slice(0, 130).trimEnd() + '…' : plain;
}

// "Age of Sojourn 31: The Summit". The number comes from the session and is never stored with an entry.
function journalTitle(game, number, name) {
    return game + (number ? ' ' + number : '') + (name ? ': ' + name : '');
}

// The same title as escaped HTML. The number is set in the text face: the display face's old-style numerals read badly.
function journalTitleHtml(game, number, name, dropCap = false) {
    const head = dropCap && game ? `<span class="drop-cap">${escapeHtml(game.charAt(0))}</span>${escapeHtml(game.slice(1))}` : escapeHtml(game);
    return head + (number ? ` <span class="t-no">${number}</span>` : '') + (name ? ': ' + escapeHtml(name) : '');
}
