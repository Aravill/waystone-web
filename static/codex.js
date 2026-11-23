// Codices page: /codices (home), /codices/c/{id} (codex), /codices/c/{id}/graph (graph), /codices/c/{id}/inbox (inbox),
// /codices/p/{id}/{slug}[/edit|/history] (page).
// Relies on campaigns.js (escapeHtml, renderMarkdown, journalByline, userAvatarHtml) and
// codex-editor.js (createCodexEditor). One component picks the screen
// from the path. Top-level names here must not clash with those scripts' (const clashes throw).

// Diff helpers for the conflict panel and History (moved here from the old journal page).
// Past this many cells the LCS matrix would be too big for the browser, so a large rewrite is
// shown (and merged) as one removal plus one addition.
const MAX_LCS_CELLS = 4e6;
const CODEX_LIST_PAGE = 5; // rows per page in the short lists (Recent changes, Wanted pages)

// Word-level LCS diff. Long unchanged stretches collapse to a marker.
function diffWords(a, b) {
    const A = a.match(/\s+|\S+/g) || [], B = b.match(/\s+|\S+/g) || [];
    let s = 0;
    while (s < A.length && s < B.length && A[s] === B[s]) s++;
    let e = 0;
    while (e < A.length - s && e < B.length - s && A[A.length - 1 - e] === B[B.length - 1 - e]) e++;
    const X = A.slice(s, A.length - e), Y = B.slice(s, B.length - e);
    const segs = [];
    const push = (t, v) => {
        const last = segs.at(-1);
        if (last && last.t === t) last.v += v; else segs.push({ t, v });
    };
    if (s) push('=', A.slice(0, s).join(''));
    if (X.length * Y.length > MAX_LCS_CELLS) {
        if (X.length) push('-', X.join(''));
        if (Y.length) push('+', Y.join(''));
    } else {
        const L = Array.from({ length: X.length + 1 }, () => new Uint16Array(Y.length + 1));
        for (let i = X.length - 1; i >= 0; i--) {
            for (let j = Y.length - 1; j >= 0; j--) {
                L[i][j] = X[i] === Y[j] ? L[i + 1][j + 1] + 1 : Math.max(L[i + 1][j], L[i][j + 1]);
            }
        }
        let i = 0, j = 0;
        while (i < X.length || j < Y.length) {
            if (i < X.length && j < Y.length && X[i] === Y[j]) { push('=', X[i]); i++; j++; }
            else if (i < X.length && (j === Y.length || L[i + 1][j] >= L[i][j + 1])) { push('-', X[i]); i++; }
            else { push('+', Y[j]); j++; }
        }
    }
    if (e) push('=', A.slice(A.length - e).join(''));
    const words = (t) => segs.filter((g) => g.t === t).reduce((n, g) => n + (g.v.match(/\S+/g) || []).length, 0);
    return { segs: tidySegs(segs), added: words('+'), removed: words('-') };
}

// Absorb one- or two-word matches between changes, so a rewritten passage reads as
// one removal followed by one addition: removed text first, then added.
function tidySegs(segs) {
    const out = [];
    let del = '', ins = '';
    const flush = () => {
        if (del) out.push({ t: '-', v: del });
        if (ins) out.push({ t: '+', v: ins });
        del = ''; ins = '';
    };
    segs.forEach((g, idx) => {
        if (g.t === '-') del += g.v;
        else if (g.t === '+') ins += g.v;
        else if ((del || ins) && idx < segs.length - 1 && !g.v.includes('\n') && (g.v.match(/\S+/g) || []).length <= 2) {
            del += g.v; ins += g.v;
        } else {
            flush();
            out.push(g);
        }
    });
    flush();
    return out;
}

function diffHtml(segs) {
    return segs.map((g, idx) => {
        if (g.t === '+') return `<ins>${escapeHtml(g.v)}</ins>`;
        if (g.t === '-') return `<del>${escapeHtml(g.v)}</del>`;
        const paras = g.v.split(/\n\s*\n/);
        const first = idx === 0, last = idx === segs.length - 1;
        // Keep the paragraph touching each change; collapse the rest.
        const keepHead = first ? 0 : 1, keepTail = last ? 0 : 1;
        const hidden = paras.length - keepHead - keepTail;
        if (hidden < 1) return escapeHtml(g.v);
        const label = `<span class="gap">⋯ ${hidden} unchanged paragraph${hidden === 1 ? '' : 's'} ⋯</span>`;
        const head = keepHead ? escapeHtml(paras[0]) : '';
        const tail = keepTail ? escapeHtml(paras.at(-1)) : '';
        return head + label + tail;
    }).join('');
}

// Mirrors wikiLinkRe in db/codex_links.go.
const CODEX_LINK_RE = /\[\[([^\[\]\n|]{1,200})(?:\|([^\[\]\n]{1,200}))?\]\]/g;

// @[Name](userId): a mention of a user. The id is the reference; the name is only what it was when written.
const CODEX_USER_RE = /@\[([^\[\]\n]{1,100})\]\(([\w-]{1,64})\)/g;

// ![[name]] and ![[name|caption]] embed a picture uploaded to the page. Same shape as the link
// regex; the leading "!" makes it a picture, so pictures are swapped out before links.
const CODEX_IMAGE_RE = /!\[\[([^\[\]\n|]{1,200})(?:\|([^\[\]\n]{0,200}))?\]\]/g;

// Files that can be attached to a page (the server's allow-list), and the same plus the picture types.
// An embed ![[name.pdf]] of one of these that matches no attachment reads "Attachment removed".
const CODEX_FILE_EXT_RE = /\.(pdf|docx?|xlsx?|pptx?|od[tsp]|txt|md|csv|zip|mp3)$/i;
const CODEX_ATTACH_RE = /\.(pdf|docx?|xlsx?|pptx?|od[tsp]|txt|md|csv|zip|mp3|png|jpe?g|webp|gif)$/i;
const CODEX_ATTACH_MAX = 100 << 20;

// 512 B, 14 KB, 3.2 MB
function codexFileSize(n) {
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return Math.round(n / 1024) + ' KB';
    return (n / (1024 * 1024)).toFixed(1) + ' MB';
}

// Sends a file as the raw request body with progress (the one place that is not fetch: fetch cannot
// report upload progress). Resolves {status, data} (status 401 when the login has expired, as send()
// reports it); rejects on a network error or when it takes longer than 16 minutes.
function codexUpload(method, url, file, onProgress) {
    return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.open(method, url);
        xhr.timeout = 16 * 60 * 1000;
        xhr.setRequestHeader('Content-Type', file.type || 'application/octet-stream');
        xhr.setRequestHeader('X-File-Name', encodeURIComponent(file.name || 'file'));
        xhr.upload.onprogress = (e) => {
            if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100));
        };
        xhr.onload = () => {
            let data = {};
            try { data = JSON.parse(xhr.responseText); } catch (e) { /* not JSON */ }
            const login = xhr.responseURL && new URL(xhr.responseURL).pathname.includes('login');
            resolve({ status: login ? 401 : xhr.status, data });
        };
        xhr.onerror = () => reject(new Error('the connection failed'));
        xhr.ontimeout = () => reject(new Error('it took too long'));
        xhr.send(file);
    });
}

// ![[name|OPTIONS|caption]]: OPTIONS is a first segment made only of the keywords below (any case),
// otherwise everything after the name is the caption. The last keyword of each kind wins; defaults
// are medium and right. Options matter only for a picture alone in its paragraph.
const CODEX_PIC_SIZES = ['small', 'medium', 'large', 'full'];
const CODEX_PIC_ALIGNS = ['left', 'center', 'right'];
function codexPicOptions(raw) {
    const parts = String(raw || '').split('|');
    const words = parts[0].trim().toLowerCase().split(/\s+/).filter(Boolean);
    const isOpts = words.length > 0 && words.every((w) => CODEX_PIC_SIZES.includes(w) || CODEX_PIC_ALIGNS.includes(w));
    let size = 'medium';
    let align = 'right';
    if (isOpts) {
        for (const w of words) {
            if (CODEX_PIC_SIZES.includes(w)) size = w;
            else align = w;
        }
    }
    const rest = (isOpts ? parts.slice(1) : parts).join('|');
    return { size, align, caption: rest.trim(), captionRaw: rest };
}

// Rewrites the index-th picture embed (counted like codexMarkdown: {{list ...}} lines are skipped)
// with a new size and spot; null when nothing changes or the index is out of range. Defaults and
// full's spot are left out; the caption is kept as written.
function codexSetPicLayout(body, index, size, align) {
    if (!CODEX_PIC_SIZES.includes(size) || !CODEX_PIC_ALIGNS.includes(align)) return null;
    const re = new RegExp(CODEX_LIST_RE.source + '|' + CODEX_IMAGE_RE.source, 'gm');
    let i = 0;
    const out = String(body || '').replace(re, (m, n, raw) => {
        if (n === undefined || i++ !== index) return m;
        const cap = codexPicOptions(raw).captionRaw;
        let opts = size === 'full' ? 'full' : [size === 'medium' ? '' : size, align === 'right' ? '' : align].filter(Boolean).join(' ');
        // A caption that reads as keywords would be taken for options, so say "medium" first.
        if (!opts && cap && codexPicOptions(cap).caption === '') opts = 'medium';
        return `![[${n}${opts ? '|' + opts : ''}${cap ? '|' + cap : ''}]]`;
    });
    return out !== body && i > index ? out : null;
}

const codexNorm = (s) => String(s || '').trim().split(/\s+/).join(' ').toLowerCase();
// Link targets in savedBody that name no page (lookup(title) is falsy) and were no targets in initialBody;
// ![[picture]] embeds are not links. Deduped by name, at most 20.
function codexNewMissing(initialBody, savedBody, lookup) {
    const targets = (body) => [...String(body).matchAll(CODEX_LINK_RE)].filter((m) => body[m.index - 1] !== '!').map((m) => m[1].trim());
    const before = new Set(targets(initialBody).map(codexNorm));
    const seen = new Set();
    const out = [];
    for (const t of targets(savedBody)) {
        const k = codexNorm(t);
        if (!k || seen.has(k) || before.has(k) || lookup(t)) continue;
        seen.add(k);
        out.push(t.split(/\s+/).join(' '));
    }
    return out.slice(0, 20);
}

function codexSlug(title) {
    const s = Array.from(String(title || '').toLowerCase().replace(/[^\p{L}\p{N}]+/gu, '-').replace(/^-+|-+$/g, '')).slice(0, 60).join('').replace(/-+$/, '');
    return s || 'page';
}

function codexPageUrl(p, tail = '') {
    return `/codices/p/${encodeURIComponent(p.id)}/${encodeURIComponent(codexSlug(p.title))}${tail}`;
}

// A picture of the page as the Markdown renderer's output: a button (opens the lightbox) around
// the image, and a caption under it when the text has one. pic is null when the name isn't one of
// the page's pictures.
// opts ({size, align, n, editable}) is given for a lone figure: size/spot classes, data-pic (the
// embed's index) and, with editable, the layout bar.
function codexPicHtml(pic, name, caption, kind, opts) {
    if (!pic) return `<span class="cx-img-missing">Missing image: ${escapeHtml(name)}</span>`;
    const shown = escapeHtml(caption || pic.name);
    const meta = escapeHtml(`${pic.name} · ${pic.width}×${pic.height}`);
    const btn = `<button type="button" class="cx-img-btn${kind === 'inline' ? ' cx-img-btn--inline' : ''}" data-url="${escapeHtml(pic.url)}" data-caption="${shown}" data-meta="${meta}" aria-label="View larger: ${shown}">` +
        `<img src="${escapeHtml(pic.url)}" alt="${shown}" width="${Number(pic.width) || ''}" height="${Number(pic.height) || ''}" loading="lazy"></button>`;
    if (kind === 'inline') return btn;
    const figcaption = caption ? `<figcaption>${escapeHtml(caption)}</figcaption>` : '';
    if (!opts) return `<figure class="cx-figure">${btn}${figcaption}</figure>`;
    const full = opts.size === 'full';
    const align = full ? 'center' : opts.align;
    const cap = (v) => v[0].toUpperCase() + v.slice(1);
    const group = (kind, values, current, off) => `<span class="cx-layout-group" role="group" aria-label="Picture ${kind}">` + values.map((v) =>
        `<button type="button" data-pic-${kind}="${v}" aria-pressed="${v === current}" aria-label="Picture ${kind}: ${cap(v)}"${off ? ' disabled' : ''}>${cap(v)}</button>`).join('') + '</span>';
    const bar = opts.editable ? '<div class="cx-layout"><button type="button" class="cx-layout-toggle" data-pic-toggle aria-label="Picture layout">Layout</button>' +
        `<div class="cx-layout-bar">${group('size', CODEX_PIC_SIZES, opts.size, false)}${group('align', CODEX_PIC_ALIGNS, align, full)}</div></div>` : '';
    return `<figure class="cx-figure cx-figure--${opts.size} cx-figure--${align}" data-pic="${opts.n}" data-size="${opts.size}" data-align="${opts.align}">${btn}${bar}${figcaption}</figure>`;
}

// ---- Live lists: a paragraph of its own, {{list tag:npc folder:Places/Inns links:here kind:lore sort:updated limit:20}} ----
// Rendered in the browser from the codex payload, never stored: so a list adds no [[links]], no
// backlinks and nothing to the graph. Values are bare or "quoted".
const CODEX_LIST_RE = /^[ \t]*\{\{list(?:[ \t][^\n{}]*)?\}\}[ \t]*$/gm;
const CODEX_LIST_MAX = 200;

function codexParseList(spec) {
    const q = { tags: [], folder: '', links: false, kind: '', sort: 'title', limit: 50, unknown: [] };
    for (const m of spec.matchAll(/(?:(\w+):)?("[^"]*"|\S+)/g)) {
        const key = (m[1] || '').toLowerCase();
        const val = m[2].replace(/^"|"$/g, '').trim();
        if (key === 'tag' && val) q.tags.push(val);
        else if (key === 'folder' && val) q.folder = val;
        else if (key === 'links' && val.toLowerCase() === 'here') q.links = true;
        else if (key === 'kind' && ['lore', 'timeline'].includes(val.toLowerCase())) q.kind = val.toLowerCase();
        else if (key === 'sort' && ['title', 'updated', 'created'].includes(val.toLowerCase())) q.sort = val.toLowerCase();
        else if (key === 'limit' && /^\d+$/.test(val)) q.limit = Math.min(CODEX_LIST_MAX, Math.max(1, Number(val)));
        else q.unknown.push(key ? `${key}:${val}` : val);
    }
    return q;
}

// ctx: { pages, tags, folders, cur (current page id), back (Set of page ids linking to it) }, or
// null while the codex payload is still loading. Returns the list box.
function codexLiveList(spec, ctx) {
    if (!ctx) return '<div class="cx-live"><p class="cx-live-note">Loading…</p></div>';
    const q = codexParseList(spec);
    const folderById = new Map(ctx.folders.map((f) => [f.id, f]));
    const pathOf = (id) => {
        const names = [];
        for (let f = folderById.get(id), n = 0; f && n < 12; f = folderById.get(f.parent_id), n++) names.unshift(f.name);
        return names;
    };
    let rows = ctx.pages.filter((p) => p.id !== ctx.cur);
    const label = [];
    for (const name of q.tags) {
        const ids = ctx.tags.filter((t) => codexNorm(t.name) === codexNorm(name)).map((t) => t.id);
        rows = rows.filter((p) => (p.tag_ids || []).some((id) => ids.includes(id)));
        label.push('tag ' + name);
    }
    if (q.folder) {
        const want = q.folder.split('/').map((s) => codexNorm(s)).filter(Boolean);
        // A bare name matches every folder with that name, a path matches from the top; sub-folders count too.
        const hit = (id) => {
            const path = pathOf(id).map((s) => codexNorm(s));
            return want.length === 1 ? path.includes(want[0]) : path.length >= want.length && want.every((w, i) => path[i] === w);
        };
        rows = rows.filter((p) => p.folder_id && hit(p.folder_id));
        label.push('in ' + q.folder);
    }
    if (q.links) {
        rows = rows.filter((p) => ctx.back.has(p.id));
        label.push('linking here');
    }
    if (q.kind) {
        rows = rows.filter((p) => p.kind === q.kind);
        label.push('kind ' + q.kind);
    }
    if (q.sort === 'title') rows.sort((a, b) => a.title.localeCompare(b.title, undefined, { sensitivity: 'base' }));
    else rows.sort((a, b) => (b[q.sort + '_at'] || '').localeCompare(a[q.sort + '_at'] || ''));
    const shown = rows.slice(0, q.limit);
    const more = rows.length - shown.length;
    const items = shown.map((p) => {
        const path = pathOf(p.folder_id).join(' › ');
        return `<li><a class="wikilink" href="${codexPageUrl(p)}" data-pid="${escapeHtml(p.id)}">${escapeHtml(p.title)}</a> <small>${escapeHtml(path || 'Top level')}</small></li>`;
    }).join('') + (more > 0 ? `<li class="cx-live-more">and ${more} more</li>` : '');
    const notes = q.unknown.map((u) => `<p class="cx-live-note">Unknown list option: ${escapeHtml(u)}</p>`).join('');
    return `<div class="cx-live"><div class="cx-live-h"><span><b>Live list</b> · ${escapeHtml(label.join(' · ') || 'all pages')}</span><span>${rows.length} ${rows.length === 1 ? 'page' : 'pages'}</span></div>${notes}` +
        (shown.length ? `<ul>${items}</ul>` : '<p class="cx-live-note">No pages match yet.</p>') + '</div>';
}

// Markdown with [[wikilinks]] and ![[pictures]]. lookup(text) returns the target page ({id, title})
// or null for a missing page; with no lookup (old revisions) links are shown plain. images(name)
// returns a picture of the page ({name, url, width, height}) or null. Links and pictures are swapped
// for placeholders before rendering so the Markdown renderer never sees them. A placeholder holds
// spaces, which a Markdown link URL can't, so one inside "(...)" is never swallowed into an href.
// Lines made only of pictures are their own paragraph (text right above or below doesn't join them):
// one picture becomes a figure, two or more in a row a gallery.
// A line that is only --- becomes a rule (<hr>), here and not in the shared renderer.
// A {{list ...}} line is swapped the same way for a live list: lists(spec) returns its HTML, and with
// no lists callback (old revisions, template previews) it shows as a plain "Live list: ..." line.
// A user mention links to the profile under the user's current name; users is the list from
// /api/codices/users (null: not loaded, so the written name), and a deleted user is plain text.
function codexMarkdown(body, lookup, canCreate = true, images = null, lists = null, editable = false, users = null) {
    const found = [];
    const pics = [];
    const specs = [];
    const text = String(body || '').replace(/[\u0001\u0002\u0003\u0004]/g, '').replace(/^[ \t]*---[ \t]*$/gm, '\n\u0004\n').replace(CODEX_LIST_RE, (m) => {
        specs.push(m.trim().slice(6, -2).trim());
        return `\u0003 ${specs.length - 1} \u0003`;
    }).replace(CODEX_IMAGE_RE, (m, n, raw) => {
        const o = codexPicOptions(raw);
        pics.push({ name: n.trim(), caption: o.caption, size: o.size, align: o.align });
        return `\u0002 ${pics.length - 1} \u0002`;
    }).replace(CODEX_LINK_RE, (m, t, label) => {
        found.push({ text: t.trim(), label: label && label.replace(/\u0002 \d+ \u0002/g, '') });
        return `\u0001 ${found.length - 1} \u0001`;
    }).replace(CODEX_USER_RE, (m, name, id) => {
        found.push({ user: id, text: name });
        return `\u0001 ${found.length - 1} \u0001`;
    }).replace(/^(?:[ \t]*(?:\u0002 \d+ \u0002[ \t]*)+(?:\n|$))+/gm, (run) => `\n${run}\n`); // picture-only lines stand as their own paragraph
    const pic = (i, kind, lone) => {
        const p = pics[Number(i)];
        return p ? codexPicHtml(images ? images(p.name) : null, p.name, p.caption, kind, lone ? { size: p.size, align: p.align, n: Number(i), editable } : null) : '';
    };
    const list = (i) => {
        const spec = specs[Number(i)];
        if (spec === undefined) return '';
        return lists ? lists(spec) : `<p class="cx-live-plain">Live list: ${escapeHtml(spec)}</p>`;
    };
    return renderMarkdown(text).replace(/<p>\u0004<\/p>/g, '<hr>').replace(/<p>\u0003 (\d+) \u0003<\/p>/g, (m, i) => list(i)).replace(/\u0003 (\d+) \u0003/g, (m, i) => escapeHtml(`{{list ${specs[Number(i)] || ''}}}`)).replace(/(?:<p>(?:\u0002 \d+ \u0002(?:<br>| )*)+<\/p>)+/g, (run) => {
        const ids = Array.from(run.matchAll(/\u0002 (\d+) \u0002/g), (m) => m[1]);
        if (ids.length === 1) return pic(ids[0], 'figure', true);
        return `<div class="cx-gallery">${ids.map((i) => pic(i, 'figure')).join('')}</div>`;
    }).replace(/\u0002 (\d+) \u0002/g, (m, i) => pic(i, 'inline')).replace(/\u0001 (\d+) \u0001/g, (m, i) => {
        const l = found[Number(i)];
        if (!l) return '';
        if (l.user) {
            const u = users ? users.find((x) => x.id === l.user) : { display_name: l.text };
            if (!u) return escapeHtml('@' + l.text);
            return `<a class="cx-user" href="/profile?user_id=${encodeURIComponent(l.user)}">@${escapeHtml(u.display_name)}</a>`;
        }
        const shown = escapeHtml(l.label || l.text);
        if (!lookup) return `<span class="wikilink wikilink--plain">${shown}</span>`;
        const target = lookup(l.text);
        if (target) return `<a class="wikilink" href="${codexPageUrl(target)}" data-pid="${escapeHtml(target.id)}">${shown}</a>`;
        return `<a class="wikilink wikilink--missing" href="#" data-text="${escapeHtml(l.text)}" title="No page yet.${canCreate ? ' Click to create.' : ''}">${shown}</a>`;
    });
}

// The writing guide's examples: codexMarkdown with a few made-up pages, a stand-in picture and a
// live list over made-up pages, so nothing here depends on the codex being viewed.
const CODEX_HELP_PAGES = ['The Old Mill', 'The Gilded Stag', 'Brindle the Innkeeper', 'Mara Voss'];
const CODEX_HELP_ALIASES = { 'the mill': 'The Old Mill', 'old mill': 'The Old Mill', inn: 'The Gilded Stag' };
function codexHelpHtml(src) {
    const lookup = (text) => {
        const t = codexNorm(text);
        const title = CODEX_HELP_PAGES.find((p) => codexNorm(p) === t) || CODEX_HELP_ALIASES[t];
        return title ? { id: 'example', title } : null;
    };
    const images = (name) => ({ name, url: '/static/img/favicon.svg', width: 64, height: 64 });
    const ctx = {
        pages: [
            { id: 'h1', title: 'Brindle the Innkeeper', folder_id: 'fp', tag_ids: ['tn'], kind: 'lore', created_at: '2026-01-02T10:00:00Z', updated_at: '2026-03-01T10:00:00Z' },
            { id: 'h2', title: 'Mara Voss', folder_id: 'fp', tag_ids: ['tn'], kind: 'lore', created_at: '2026-01-05T10:00:00Z', updated_at: '2026-02-01T10:00:00Z' },
            { id: 'h3', title: 'The Gilded Stag', folder_id: 'fi', tag_ids: [], kind: 'lore', created_at: '2026-01-09T10:00:00Z', updated_at: '2026-04-01T10:00:00Z' },
            { id: 'h4', title: 'The Old Mill', folder_id: 'fl', tag_ids: [], kind: 'lore', created_at: '2026-01-11T10:00:00Z', updated_at: '2026-02-20T10:00:00Z' },
            { id: 'h5', title: 'Timeline of the War', folder_id: '', tag_ids: [], kind: 'timeline', created_at: '2026-01-20T10:00:00Z', updated_at: '2026-01-25T10:00:00Z' }
        ],
        tags: [{ id: 'tn', name: 'npc' }],
        folders: [{ id: 'fp', name: 'People', parent_id: '' }, { id: 'fl', name: 'Places', parent_id: '' }, { id: 'fi', name: 'Inns', parent_id: 'fl' }],
        cur: '',
        back: new Set()
    };
    return codexMarkdown(src, lookup, true, images, (spec) => codexLiveList(spec, ctx), false, [{ id: 'roy', display_name: 'Roy' }]);
}

// Save as template, "Headings only": the page's ## and ### lines (as the renderer reads them: up
// to 3 leading spaces, a space or tab after the hashes, some text), one blank line apart. "" when
// there are none.
function codexHeadingsOnly(body) {
    const out = String(body || '').split('\n').filter((l) => /^[ \t]{0,3}#{2,3}[ \t]+\S/.test(l)).map((l) => l.trim());
    return out.length ? out.join('\n\n') + '\n\n' : '';
}

// The / menu in the page editor: snippets, then each template's headings (templates with none are left out).
const CODEX_SNIPPETS = [
    { label: 'Person', detail: 'Name, role, wants, lives in', keys: 'npc character', text: '**Name:** \n**Role:** \n**Wants:** \n**Lives in:** [[ ]]', cursor: 10, sample: '**Name:** Brindle\n**Role:** Innkeeper\n**Wants:** A quiet winter\n**Lives in:** [[The Gilded Stag]]' },
    { label: 'Callout', detail: 'A quoted note that stands out', keys: 'quote note', text: '> **Note:** ' },
    { label: 'Loot list', detail: 'A heading and three bullets', keys: 'rewards', text: '## Loot and rewards\n\n- \n- \n- ' },
    { label: 'Live list', detail: 'Pages by tag, kept up to date', keys: 'tag', text: '{{list tag: links:here}}', cursor: 11, sample: '{{list tag:npc}}' },
    { label: 'Divider', detail: 'A rule across the page', keys: 'rule line hr', text: '---' }
];
function codexSlashItems(templates, today) {
    const snippets = [...(today ? [{ label: "Today's date", detail: today, keys: 'date', text: today }] : []), ...CODEX_SNIPPETS]
        .map((s) => ({ ...s, section: 'Snippets' }));
    const heads = [];
    for (const t of templates) {
        const text = codexHeadingsOnly(t.body);
        if (text) heads.push({ label: t.name + ' headings', detail: t.by ? 'this codex' : '', section: 'Template headings', text });
    }
    return [...snippets, ...heads];
}

// Save as template, "All the text": pictures belong to their page, so lines that are only
// ![[...]] embeds go, and so do embeds inside a line.
function codexWithoutPictures(body) {
    return String(body || '').split('\n').filter((l) => !/^\s*(!\[\[[^\[\]\n]*\]\]\s*)+$/.test(l)).join('\n')
        .replace(CODEX_IMAGE_RE, '').replace(/\n{3,}/g, '\n\n');
}

// Folder tree. Folders only sort pages. Open/closed state is kept per codex in localStorage and
// is deliberately not reactive, so toggling a folder never re-renders the tree.
let codexOpenState = {};
// Folders opened only to show the current page or a deep link: never saved, and a user toggle drops them.
const codexTransientOpen = new Set();
const codexOpenKey = (codexId) => 'codex-folders-open:' + codexId;

function codexLoadOpenState(codexId) {
    try {
        codexOpenState = JSON.parse(localStorage.getItem(codexOpenKey(codexId))) || {};
    } catch (error) {
        codexOpenState = {};
    }
}

function codexSaveOpenState(codexId) {
    try {
        localStorage.setItem(codexOpenKey(codexId), JSON.stringify(codexOpenState));
    } catch (error) {
        // storage may be blocked; the tree still works
    }
}

// Local drafts of an open editor: `codex-draft:{pageId}:{base}` -> {body, note, initial, at}, where base is
// the revision the edit started from and initial the body at that base (for the conflict diff).
// Storage may be blocked or full; every access swallows its errors and the editor works without drafts.
const CODEX_DRAFT_PREFIX = 'codex-draft:';

function codexDraftKeys(pageId) {
    const keys = [];
    const prefix = CODEX_DRAFT_PREFIX + pageId + ':';
    try {
        for (let i = 0; i < localStorage.length; i++) {
            const k = localStorage.key(i);
            if (k && k.startsWith(prefix)) keys.push(k);
        }
    } catch (error) {
        // no storage
    }
    return keys;
}

function codexDraftWrite(pageId, base, data) {
    try {
        localStorage.setItem(CODEX_DRAFT_PREFIX + pageId + ':' + base, JSON.stringify(data));
        return true;
    } catch (error) {
        return false;
    }
}

// The newest draft of the page as { base, body, note, initial, at }, or null.
function codexDraftFind(pageId) {
    let best = null;
    for (const k of codexDraftKeys(pageId)) {
        try {
            const d = JSON.parse(localStorage.getItem(k));
            if (!d || typeof d.body !== 'string' || typeof d.at !== 'string') continue;
            if (!best || d.at > best.at) {
                best = { ...d, note: d.note || '', initial: typeof d.initial === 'string' ? d.initial : '', base: Number(k.slice(k.lastIndexOf(':') + 1)) };
            }
        } catch (error) {
            // a broken entry is ignored
        }
    }
    return best;
}

function codexDraftClear(pageId) {
    for (const k of codexDraftKeys(pageId)) {
        try {
            localStorage.removeItem(k);
        } catch (error) {
            // no storage
        }
    }
}

// Drops drafts older than 30 days and keeps the 20 newest.
function codexDraftPrune() {
    try {
        const items = [];
        for (let i = 0; i < localStorage.length; i++) {
            const k = localStorage.key(i);
            if (!k || !k.startsWith(CODEX_DRAFT_PREFIX)) continue;
            let at = 0;
            try {
                at = Date.parse(JSON.parse(localStorage.getItem(k)).at) || 0;
            } catch (error) {
                // unreadable: counts as oldest
            }
            items.push({ k, at });
        }
        items.sort((a, b) => b.at - a.at);
        const cutoff = Date.now() - 30 * 24 * 3600 * 1000;
        items.forEach((it, i) => {
            if (i >= 20 || it.at < cutoff) localStorage.removeItem(it.k);
        });
    } catch (error) {
        // no storage
    }
}

// Builds { roots, pages } from the flat folder and page lists. A folder whose parent is missing
// is shown at the top, and a cycle can't loop: only reachable folders are walked.
function codexFolderTree(folders, pages) {
    const nodes = new Map(folders.map((f) => [f.id, { id: f.id, name: f.name, parent: f.parent_id, children: [], pages: [], count: 0 }]));
    const roots = [];
    for (const n of nodes.values()) {
        const parent = nodes.get(n.parent);
        (parent && parent !== n ? parent.children : roots).push(n);
    }
    const rootPages = [];
    for (const p of pages) {
        const n = nodes.get(p.folder_id);
        (n ? n.pages : rootPages).push(p);
    }
    const byName = (a, b) => (codexNorm(a.name) < codexNorm(b.name) ? -1 : codexNorm(a.name) > codexNorm(b.name) ? 1 : 0);
    const seen = new Set();
    const finish = (n) => {
        seen.add(n.id);
        n.children.sort(byName);
        n.count = n.pages.length;
        for (const c of n.children) {
            if (!seen.has(c.id)) n.count += finish(c);
        }
        return n.count;
    };
    roots.sort(byName);
    roots.forEach(finish);
    return { roots, pages: rootPages };
}

// Folders in tree order with a path label ("Places › Cities"); excludeId's subtree is disabled.
function codexFolderOptions(folders, excludeId = '') {
    const { roots } = codexFolderTree(folders || [], []);
    const out = []; // "Top level" is a static <option value=""> in the selects
    const walk = (n, trail, blocked) => {
        const path = [...trail, n.name];
        const off = blocked || n.id === excludeId;
        out.push({ id: n.id, label: path.join(' › '), disabled: off });
        n.children.forEach((c) => walk(c, path, off));
    };
    roots.forEach((r) => walk(r, [], false));
    return out;
}

// Link preview cards by page id (promises, so kept out of Alpine's reactive state).
const codexPeekCache = {};

// A few plain lines from the start of a page, for the link preview card.
function codexPlainExcerpt(body, max = 220) {
    const text = String(body || '').split('\n').map((l) => l.trim()).filter((l) => l && l !== '---' && !/^#{1,6}\s/.test(l) && !/^\{\{list(?:[ \t][^{}]*)?\}\}$/.test(l)).map((l) => l.replace(/^([-*+]|\d+\.|>)\s+/, '')).join(' ')
        .replace(CODEX_IMAGE_RE, '')
        .replace(CODEX_LINK_RE, (m, t, label) => label || t)
        .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
        .replace(/[*_`]/g, '')
        .replace(/\s+/g, ' ').trim();
    const chars = Array.from(text);
    return chars.length > max ? chars.slice(0, max).join('').trim() + '…' : text;
}

// ---- Timeline pages: a page with kind "timeline" lists dated events under its intro, newest first. ----
// Years with events more than this many years apart get a dashed spine and a "N years earlier" note.
const CODEX_TL_GAP = 30;
const CODEX_TL_ICON = '<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M7 3v18"/><rect x="5" y="5" width="4" height="4" fill="currentColor"/><rect x="5" y="15" width="4" height="4"/><path d="M12 7h8M12 17h6"/></svg>';
const CODEX_TL_PEN = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 20h4L19 9l-4-4L4 16z"/></svg>';

// cal is {months, era, before_era}: month names (none = plain numbers) and the era suffixes.
function codexTlYear(y, cal) {
    const c = cal || {};
    if (y < 0) return c.before_era ? `${-y} ${c.before_era}` : `−${-y}`;
    return c.era ? `${y} ${c.era}` : String(y);
}

// The year for a year box: the number, then its era in small type.
function codexTlYearHtml(y, cal) {
    const c = cal || {};
    if (y < 0) return c.before_era ? `${-y}<small>${escapeHtml(c.before_era)}</small>` : `−${-y}`;
    return c.era ? `${y}<small>${escapeHtml(c.era)}</small>` : String(y);
}

// "2 Thaw" / "Thaw"; without a name for the month (no calendar, or beyond its names) "Month 3, day 14" / "Month 3".
function codexTlDayMonth(ev, cal) {
    if (!ev.month) return '';
    const name = ((cal || {}).months || [])[ev.month - 1];
    if (name) return ev.day ? `${ev.day} ${name}` : name;
    return ev.day ? `Month ${ev.month}, day ${ev.day}` : `Month ${ev.month}`;
}

// "2 Thaw 1203 AR", "Month 3, day 14, 1203", or just the year when there is no month.
function codexTlDate(ev, cal) {
    const dm = codexTlDayMonth(ev, cal);
    const year = codexTlYear(ev.year, cal);
    if (!dm) return year;
    return ((cal || {}).months || [])[ev.month - 1] ? `${dm} ${year}` : `${dm}, ${year}`;
}

// The events (newest first) as [{year, events}], one group per year.
function codexTlGroups(events) {
    const groups = [];
    for (const e of events) {
        let g = groups[groups.length - 1];
        if (!g || g.year !== e.year) groups.push((g = { year: e.year, events: [] }));
        g.events.push(e);
    }
    return groups;
}

// The Jump to list: a century heading, then its years with their event counts. Negative years
// are grouped by their distance from 0, with the before-era suffix.
function codexTlJump(groups, cal) {
    const c = cal || {};
    const out = [];
    let last = null;
    for (const g of groups) {
        const century = Math.floor(Math.abs(g.year) / 100) * 100;
        const key = (g.year < 0 ? '-' : '') + century;
        if (key !== last) {
            last = key;
            const label = g.year < 0 ? (c.before_era ? `${century}s ${c.before_era}` : `−${century}s`) : `${century}s` + (c.era ? ` ${c.era}` : '');
            out.push({ key: 'c' + key, cent: true, label });
        }
        out.push({ key: 'y' + g.year, cent: false, year: g.year, label: codexTlYear(g.year, c), count: g.events.length });
    }
    return out;
}

// The event rows (layout B: cards alternate left and right of the spine) as escaped HTML for an <ol>.
// opts: canEdit (Edit buttons), justAdded (event id to flash), tagUrl (name -> href).
function codexTlHtml(events, cal, opts) {
    const o = opts || {};
    const groups = codexTlGroups(events);
    let side = 0;
    let html = '';
    groups.forEach((g, i) => {
        const yearLabel = escapeHtml(codexTlYear(g.year, cal));
        html += `<li><div class="tl-row tl-year" id="y${g.year}"><span class="tl-yr">${codexTlYearHtml(g.year, cal)}</span>` +
            '<span class="tl-seg"><span class="tl-mk tl-mk--year" aria-hidden="true"></span></span></div>' +
            `<ol aria-label="${yearLabel}">`;
        for (const e of g.events) {
            const l = e.linked;
            const title = l
                ? `<a class="tl-title wikilink" href="${escapeHtml(codexPageUrl(l))}" data-pid="${escapeHtml(l.id)}">${escapeHtml(e.title)}</a>`
                : `<span class="tl-title">${escapeHtml(e.title)}</span>`;
            const extra = l
                ? (l.excerpt ? `<p class="tl-ex">${escapeHtml(l.excerpt)}</p>` : '') +
                    (l.tags.length ? `<div class="tl-tags">${l.tags.map((t) => `<a class="cx-chip cx-chip--tag" href="${escapeHtml(o.tagUrl ? o.tagUrl(t) : '#')}">${escapeHtml(t)}</a>`).join('')}</div>` : '')
                : '';
            const edit = o.canEdit
                ? `<div><button type="button" class="tl-edit" data-edit="${escapeHtml(e.id)}">${CODEX_TL_PEN}Edit<span class="visually-hidden"> ${escapeHtml(e.title)}</span></button></div>`
                : '';
            const lr = side++ % 2 ? 'r' : 'l';
            html += `<li class="tl-row tl-ev ${lr}" id="ev-${escapeHtml(e.id)}">` +
                `<span class="tl-seg"><span class="tl-mk${l ? ' tl-mk--linked' : ''}" aria-hidden="true"></span></span>` +
                `<div class="tl-card${o.justAdded === e.id ? ' just-added' : ''}"><div class="tl-date">${escapeHtml(codexTlDate(e, cal))}</div>${title}${extra}${edit}</div></li>`;
        }
        html += '</ol>';
        const next = groups[i + 1];
        if (next && g.year - next.year > CODEX_TL_GAP) {
            html += `<div class="tl-row tl-gap"><span class="tl-seg"></span><span class="tl-gap-note">${g.year - next.year} years earlier</span></div>`;
        }
        html += '</li>';
    });
    if (groups.length) html += '<li class="tl-row tl-end"><span class="tl-seg"></span><span class="tl-end-note">The earliest event</span></li>';
    return html;
}

// A page in a journal folder ("{game}: Campaign Journal") drops its "{game}: " prefix, so the folder reads as a list of dates.
function codexJournalPrefix(folderName) {
    const tail = 'Campaign Journal';
    return folderName.endsWith(': ' + tail) ? folderName.slice(0, folderName.length - tail.length) : '';
}

function codexPageItemHtml(p, currentId, canDrag = false, prefix = '') {
    const shown = prefix && p.title.startsWith(prefix) && p.title.length > prefix.length ? p.title.slice(prefix.length) : p.title;
    const also = p.aliases.length ? `<span class="muted"> · also ${escapeHtml(p.aliases.join(', '))}</span>` : '';
    const kind = p.kind === 'timeline' ? `<span class="cx-ty" aria-hidden="true">${CODEX_TL_ICON}</span>` : '';
    const kindText = p.kind === 'timeline' ? '<span class="visually-hidden"> (timeline)</span>' : '';
    const drag = canDrag ? ` draggable="true" data-drag="page:${escapeHtml(p.id)}"` : '';
    return `<li><a href="${codexPageUrl(p)}"${p.id === currentId ? ' aria-current="page"' : ''}${drag}>${kind}${escapeHtml(shown)}${kindText}</a>${also}</li>`;
}

function codexFolderHtml(n, depth, canEdit, currentId, canDrag = false) {
    const open = codexTransientOpen.has(n.id) || (codexOpenState[n.id] === undefined ? depth === 0 : codexOpenState[n.id]);
    const id = escapeHtml(n.id);
    const prefix = codexJournalPrefix(n.name);
    const pages = n.pages.length ? `<ul class="page-list">${n.pages.map((p) => codexPageItemHtml(p, currentId, canDrag, prefix)).join('')}</ul>` : '';
    return `<details class="codex-folder${depth >= 4 ? ' codex-folder--deep' : ''}" id="folder-${id}" data-id="${id}"${open ? ' open' : ''}>` +
        `<summary${canDrag ? ` draggable="true" data-drag="folder:${id}"` : ''}><span class="codex-folder-name">${escapeHtml(n.name)}</span><span class="codex-folder-count muted" title="${n.count} ${n.count === 1 ? 'page' : 'pages'}"><span aria-hidden="true">${n.count}</span><span class="visually-hidden"> ${n.count} ${n.count === 1 ? 'page' : 'pages'}</span></span>` +
        (canEdit ? `<button type="button" class="codex-menu-btn" data-menu="${id}" aria-label="Actions for folder ${escapeHtml(n.name)}" aria-haspopup="menu" aria-expanded="false">⋯</button>` : '') + '</summary>' +
        `<div class="codex-folder-body">` +
        n.children.map((c) => codexFolderHtml(c, depth + 1, canEdit, currentId, canDrag)).join('') + pages + '</div></details>';
}

// The ⋯ menu's items; every label names the folder for screen readers.
function codexFolderMenuHtml(f) {
    const id = escapeHtml(f.id);
    const name = escapeHtml(f.name);
    const item = (act, label, aria, cls = '') => `<button type="button" role="menuitem" class="codex-menu-item${cls}" data-act="${act}" data-id="${id}" aria-label="${aria} ${name}">${label}</button>`;
    return item('newPage', '+ Page', '+ Page in') + item('newFolder', '+ Folder', '+ Folder in') + item('rename', 'Rename', 'Rename') +
        item('folderTpl', 'New pages start from…', 'New pages start from, for') + item('move', 'Move', 'Move') + item('delete', 'Delete', 'Delete', ' codex-menu-item--danger');
}

// The template new pages in a folder start from: its own default, else the nearest ancestor's ('' = none).
function codexFolderDefault(folders, folderId) {
    const byId = new Map((folders || []).map((f) => [f.id, f]));
    for (let cur = byId.get(folderId), hops = 0; cur && hops <= byId.size; hops++, cur = byId.get(cur.parent_id)) {
        if (cur.default_template) return cur.default_template;
    }
    return '';
}

window.codexPage = function () {
    return {
        displayName: 'User',
        isAdmin: false,
        currentUserId: '',
        route: { screen: 'home', id: '' },
        loading: true,
        notFound: false,
        loadError: '',
        loadSeq: 0,
        busy: false,
        saved: false, // a save went through and the page is about to leave, so the unload guard stands down
        messageTimer: null,
        messageVisible: false,
        messageText: '',
        messageType: '',
        messageAction: null,
        dragOn: false, // a tree drag is in progress: shows the Top level strip
        treeDrag: null, // { kind: 'page' | 'folder', id } being dragged
        springId: '',
        springTimer: null,
        notes: [], // the inbox: the viewer's own quick notes of this codex, newest first
        notesLoading: false,
        notesError: '',
        confirmDialog: { open: false, eyebrow: '', title: '', message: '', confirmLabel: '', danger: true, resolve: null },
        pageWidth: 'thin', // page screen reading width: thin, medium or wide (kept in localStorage)
        home: null,
        pagers: { homeRecent: 0, recent: 0, wanted: 0 }, // current page of each short list
        pagerId: '',
        codex: null,
        pageData: null,
        toc: [],
        tocActive: '',
        pageFilter: '',
        folderTarget: '',
        folderRevealed: false, // the #folder-… deep link is applied on the first load only
        toggledBy: null, // the folder a user just clicked, so only user toggles are saved
        folderMenu: { id: '', top: 0, right: 0, html: '' },
        tocHashDone: false,
        navPromise: null,
        codexChoices: [],
        codicesLoaded: false, // codexChoices was read from the server (it may be legitimately empty)
        dlg: { type: '' },
        tplBuiltin: null, // the built-in templates and the day {{date}} stands for, from the server (once)
        // Tags and pictures
        tagFilter: '', // ?tag=name on the codex screen: list only the pages with that tag
        lightbox: { url: '', caption: '', meta: '' },
        uploading: '', // status text while a batch uploads
        uploadJobs: 0,
        attach: { busy: false, text: '', pct: 0, replaceId: '' }, // the page's Attachments box
        dragOver: false,
        // Editor
        draft: null,
        draftStart: '', // the body as the page loaded, to tell whether a draft is worth keeping
        draftTimer: null,
        draftOffer: null, // a stored draft waiting for Restore or Discard
        draftStatus: '',
        conflict: null,
        editor: null,
        editorReady: false,
        // History
        revisions: [],
        pruned: false,
        currentRevision: 0,
        historyLoading: false,
        historyMode: 'changes',
        selected: 0,
        selectedDiff: null,
        selectedBody: '',
        bodyLoading: false,
        bodies: {}, // revision number -> text, fetched on demand
        diffSeq: 0,
        deltas: {},
        // Codices shell: nav drawer, codex picker, search, page tabs, More menu, link preview
        navSeq: 0,
        // Graph screen and the page's local graph (the drawing lives in codex-graph.js)
        graphLoading: false,
        graphError: '',
        graphReady: false,
        graphFrom: null, // the page the graph was opened from (?from=), if it is in this codex
        graphSummary: '',
        localGraph: false,
        users: null, // who can be @mentioned, see loadUsers

        // Mentioned, not linked: plain mentions of this page elsewhere in the codex
        mentions: { loaded: false, total: 0, list: [], can_edit: false, busy: false, progress: '' },
        mentionSeq: 0,
        picBusy: false,
        localSeq: 0,
        navOpen: false,
        navAnim: false, // the drawer only animates after the first open, so it never slides shut on load
        hashScrolled: false,
        allCodices: null,
        pickOpen: false,
        searchQ: '',
        searchOpen: false,
        searchIdx: -1,
        searchKey: '',
        searchLoading: false,
        searchTitles: [],
        tabs: [],
        moreOpen: false,
        // Timeline pages
        tlActiveYear: null, // the year in view, for Jump to
        tlJustAdded: '', // the event id whose row flashes after an add, edit or deep link
        tlJustTimer: null,
        peek: { open: false, x: 0, y: 0, title: '', where: '', text: '' },
        peekTimer: null,
        peekFor: null,

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
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
                return;
            }
            try {
                const w = localStorage.getItem('codex-page-width');
                if (w === 'medium' || w === 'wide') this.pageWidth = w;
            } catch (e) { /* storage blocked: stay thin */ }
            this.route = this.parseRoute();
            if (!this.route) {
                this.route = { screen: 'home', id: '' };
                this.notFound = true;
                this.loading = false;
                return;
            }
            this.loadUsers();
            this.tagFilter = (new URLSearchParams(location.search).get('tag') || '').trim();
            document.addEventListener('keydown', (event) => this.onGlobalKey(event));
            document.addEventListener('click', (event) => this.onImageClick(event));
            window.matchMedia('(min-width: 901px)').addEventListener('change', (e) => e.matches && this.closeNav(false));
            window.addEventListener('scroll', () => {
                if (this.peek.open) this.hidePeek();
                if (this.searchOpen) this.placeResults();
            }, { passive: true });
            // A file dropped outside the drop targets of the editor would make the browser open it and
            // leave the draft: take it as an upload instead.
            window.addEventListener('dragover', (event) => {
                if (this.screen === 'edit' && this.hasFiles(event)) event.preventDefault();
            });
            window.addEventListener('drop', (event) => {
                if (this.screen === 'edit' && this.hasFiles(event)) this.onDrop(event);
            });
            window.addEventListener('beforeunload', (event) => {
                if (this.isDirty || this.uploading || this.attach.busy) {
                    event.preventDefault();
                    event.returnValue = '';
                }
            });
            // A tab closed within the 800 ms wait still keeps the last keystrokes.
            window.addEventListener('pagehide', () => {
                if (this.draftTimer) this.writeDraft();
            });
            codexDraftPrune();
            await this.load();
            this.showArrivalMessage();
        },

        // /codices, /codices/help, /codices/c/{id}[/slug], /codices/c/{id}/graph, /codices/c/{id}/inbox, /codices/p/{id}/{slug}[/edit|/history]
        // A last segment "graph" or "inbox" straight after the codex id is always that screen, so
        // the codex's own links never carry a slug.
        // Returns null for a malformed URL.
        parseRoute() {
            let parts;
            try {
                parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
            } catch (error) {
                return null;
            }
            if (parts[1] === 'help' && parts.length === 2) return { screen: 'help', id: '' };
            if (parts[1] === 'c' && parts[2]) {
                const tail = parts.length === 4 ? parts[3] : '';
                return { screen: tail === 'graph' || tail === 'inbox' ? tail : 'codex', id: parts[2] };
            }
            if (parts[1] === 'p' && parts[2]) {
                const tail = parts[4] || '';
                return { screen: tail === 'edit' || tail === 'history' ? tail : 'page', id: parts[2] };
            }
            return { screen: 'home', id: '' };
        },

        get screen() {
            return this.route.screen;
        },
        // The screens that look across all codices (search and the picker cover every codex).
        get acrossCodices() {
            return this.screen === 'home' || this.screen === 'help';
        },
        // The writing guide: the sections with each example rendered through the real page renderer.
        get helpSections() {
            return CODEX_HELP.map((sec) => {
                const examples = sec.snippets
                    ? CODEX_SNIPPETS.map((s) => ({ src: s.text, note: s.label + ': ' + s.detail, sample: s.sample }))
                    : sec.examples;
                return { ...sec, examples: examples.map((ex) => ({ ...ex, html: codexHelpHtml(ex.sample || ex.src) })) };
            });
        },
        // The screens that show the codex itself rather than a page.
        get onCodexScreen() {
            return this.screen === 'codex' || this.screen === 'graph' || this.screen === 'inbox';
        },
        inboxUrl() {
            return '/codices/c/' + encodeURIComponent(this.currentCodexId) + '/inbox';
        },
        graphUrl(fromId = '') {
            const id = this.currentCodexId || (this.page && this.page.codex.id) || '';
            return '/codices/c/' + encodeURIComponent(id) + '/graph' + (fromId ? '?from=' + encodeURIComponent(fromId) : '');
        },
        get page() {
            return this.pageData ? this.pageData.page : null;
        },
        // The journal side of a session's page (null for any other page): the game, the session and who was there.
        get journal() {
            return (this.pageData && this.pageData.session) || null;
        },
        // A journal page reads "Session 12" (the number is computed when read, never stored in the title).
        get pageHeading() {
            return this.journal && this.journal.number ? 'Session ' + this.journal.number : this.page ? this.page.title : '';
        },
        get journalWho() {
            const list = this.journal ? this.journal.attendees : [];
            return list.length ? list.map((u) => u.display_name || u.name || 'Guest').join(', ') : 'Nobody recorded';
        },
        // Whether the viewer may change what is on screen: the codex's pages and folders, or this page.
        get canEdit() {
            if (this.onCodexScreen) return !!(this.codex && this.codex.can_edit);
            return !!(this.pageData && this.pageData.can_edit);
        },
        // Copy page needs See on the page (it is on screen) and Edit on some codex to copy into.
        get canCopy() {
            return this.codexChoices.length > 0;
        },
        get crumbs() {
            const s = this.screen;
            if (s === 'help') return [{ label: 'Writing guide', href: '' }];
            if (s === 'codex' && this.codex) return [{ label: this.codex.codex.name, href: '' }];
            if (s === 'graph' && this.codex) return [{ label: this.codex.codex.name, href: '/codices/c/' + encodeURIComponent(this.codex.codex.id) }, { label: 'Graph', href: '' }];
            if (s === 'inbox' && this.codex) return [{ label: this.codex.codex.name, href: '/codices/c/' + encodeURIComponent(this.codex.codex.id) }, { label: 'Inbox', href: '' }];
            if (!this.page) return [];
            const codexHref = '/codices/c/' + encodeURIComponent(this.page.codex.id);
            const list = [{ label: this.page.codex.name, href: '/codices/c/' + encodeURIComponent(this.page.codex.id) }];
            // Folder crumbs link to the codex page with that folder opened. With more than two
            // folders the middle ones collapse to "…" on narrow screens (CSS).
            const path = this.page.folder_path || [];
            path.forEach((f, i) => {
                if (path.length > 2 && i === 1) list.push({ label: '…', href: '', ellipsis: true });
                list.push({ label: f.name, href: `${codexHref}#folder-${encodeURIComponent(f.id)}`, mid: path.length > 2 && i > 0 && i < path.length - 1 });
            });
            if (s === 'page') return [...list, { label: this.page.title, href: '' }];
            return [...list, { label: this.page.title, href: this.pageUrl(this.page) }, { label: s === 'edit' ? 'Edit' : 'History', href: '' }];
        },

        setPageWidth(w) {
            this.pageWidth = w;
            try { localStorage.setItem('codex-page-width', w); } catch (e) { /* not saved */ }
        },

        // Short lists on the homes and in the side panel show CODEX_LIST_PAGE rows at a time.
        pageLast(list) {
            return Math.max(0, Math.ceil(list.length / CODEX_LIST_PAGE) - 1);
        },
        pageCur(list, key) {
            return Math.min(this.pagers[key], this.pageLast(list)); // a shrunk list never shows an empty page
        },
        pagedSlice(list, key) {
            const from = this.pageCur(list, key) * CODEX_LIST_PAGE;
            return list.slice(from, from + CODEX_LIST_PAGE);
        },
        pageRange(list, key) {
            const from = this.pageCur(list, key) * CODEX_LIST_PAGE;
            return (from + 1) + '–' + Math.min(from + CODEX_LIST_PAGE, list.length) + ' of ' + list.length;
        },
        pagerAtEnd(list, key) {
            return this.pageCur(list, key) >= this.pageLast(list);
        },
        turnPage(list, key, d) {
            this.pagers[key] = Math.max(0, Math.min(this.pageLast(list), this.pageCur(list, key) + d));
        },

        async load() {
            const seq = ++this.loadSeq; // an older response must not overwrite a newer one
            try {
                const r = this.route;
                if (r.screen === 'help') {
                    document.title = 'Waystone - Writing guide';
                    this.notFound = false;
                    this.loadError = '';
                    this.loadCodexList().catch(() => {}); // the picker and search cover every codex
                    return;
                }
                if (r.id !== this.pagerId) {
                    this.pagerId = r.id;
                    this.pagers = { homeRecent: 0, recent: 0, wanted: 0 };
                }
                const url = r.screen === 'home' ? '/api/codices'
                    : r.screen === 'codex' || r.screen === 'graph' || r.screen === 'inbox' ? `/api/codices/c/${encodeURIComponent(r.id)}`
                    : `/api/codices/pages/${encodeURIComponent(r.id)}`;
                const { response, data } = await this.send(url, 'GET');
                if (response.status === 401) {
                    window.location.href = '/login.html';
                    return;
                }
                if (seq !== this.loadSeq) return;
                if (response.status === 404) {
                    this.notFound = true;
                    this.loadError = '';
                    return;
                }
                if (!response.ok) {
                    this.loadError = data.error || 'Failed to load the codices';
                    return;
                }
                this.notFound = false;
                this.loadError = '';
                if (r.screen === 'home') {
                    this.home = data;
                    this.allCodices = data.codices;
                    document.title = 'Waystone - Codices';
                } else if (r.screen === 'codex' || r.screen === 'graph' || r.screen === 'inbox') {
                    codexLoadOpenState(data.codex.id);
                    if (!this.folderRevealed) {
                        this.folderRevealed = true;
                        this.revealFolderFromHash(data.folders);
                    }
                    this.codex = data;
                    this.syncTabs(data);
                    document.title = 'Waystone - ' + data.codex.name + (r.screen === 'graph' ? ' graph' : r.screen === 'inbox' ? ' inbox' : '');
                    if (r.screen === 'graph') await this.loadGraph();
                    else if (r.screen === 'inbox') await this.loadInbox();
                    else this.$nextTick(() => this.scrollToFolder());
                } else {
                    if (r.screen === 'edit' && !data.can_edit) {
                        window.location.replace(this.pageUrl(data.page));
                        return;
                    }
                    this.pageData = data;
                    this.ensureCodices(true); // the codices Copy page can go to
                    this.trackTab(data.page);
                    this.navPromise = this.loadNav(data.page);
                    if (r.screen === 'page') {
                        this.loadLocalGraph(data);
                        this.loadMentions(data.page.id);
                    }
                    if (r.screen === 'page' && data.page.kind === 'timeline') this.$nextTick(() => this.scrollToEventHash());
                    document.title = 'Waystone - ' + data.page.title;
                    if (location.pathname !== this.pageUrl(data.page) + (r.screen === 'page' ? '' : '/' + r.screen)) {
                        history.replaceState(null, '', this.pageUrl(data.page) + (r.screen === 'page' ? '' : '/' + r.screen) + location.search);
                    }
                    if (r.screen === 'edit' && !this.draft) await this.initDraft();
                    if (r.screen === 'history') {
                        await this.loadHistory(Number(new URLSearchParams(location.search).get('r')) || 0);
                    }
                }
            } catch (error) {
                if (seq === this.loadSeq) this.loadError = 'Could not load the codices: ' + error.message;
            } finally {
                if (seq === this.loadSeq) this.loading = false;
            }
        },

        // ---- Helpers ----
        pageUrl(p, tail = '') {
            return codexPageUrl(p, tail);
        },
        renderMarkdown(text) {
            return renderMarkdown(text);
        },
        codexMarkdown(body, lookup) {
            return codexMarkdown(body, lookup, true, this.imageLookup, null, false, this.users);
        },
        countLabel(n, word) {
            return `${n} ${word}${n === 1 ? '' : 's'}`;
        },
        shortDate(iso) {
            return new Date(iso).toLocaleDateString('default', { day: 'numeric', month: 'short' });
        },
        formatStamp(iso) {
            return new Date(iso).toLocaleString('default', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
        },
        sentence(text) {
            const t = String(text || '');
            return t.charAt(0).toUpperCase() + t.slice(1) + (t.endsWith('.') ? '' : '.');
        },
        recentHtml(r, withCodex) {
            const note = r.note ? ` · ${escapeHtml(r.note)}` : '';
            const where = withCodex ? ` · ${escapeHtml(r.codex.name)}` : '';
            return `<a href="${codexPageUrl({ id: r.page_id, title: r.title })}">${escapeHtml(r.title)}</a><span class="muted"> · revision ${r.number} by ${escapeHtml(r.author_user.display_name)}${where}${note} · ${escapeHtml(this.shortDate(r.created_at))}</span>`;
        },
        get treeHtml() {
            if (!this.codex) return '';
            const tree = codexFolderTree(this.codex.folders, this.codex.pages);
            const cur = !this.onCodexScreen && this.page ? this.page.id : '';
            // Mouse drag and drop only: on touch the folder menu's Move and the page's Folder > Change stay the way.
            const canDrag = !!this.codex.can_edit && window.matchMedia('(pointer: fine)').matches;
            const pages = tree.pages.length ? `<ul class="page-list">${tree.pages.map((p) => codexPageItemHtml(p, cur, canDrag)).join('')}</ul>` : '';
            return tree.roots.map((r) => codexFolderHtml(r, 0, this.codex.can_edit, cur, canDrag)).join('') + pages;
        },
        // Marks a folder and the ones above it open.
        openFolderChain(id, folders, transient = false) {
            const byID = new Map(folders.map((f) => [f.id, f]));
            for (let cur = id, hops = 0; byID.has(cur) && hops <= folders.length; hops++) {
                if (transient) codexTransientOpen.add(cur);
                else codexOpenState[cur] = true;
                cur = byID.get(cur).parent_id;
            }
        },
        // Keeps a folder (and its ancestors) open across the reload that follows a change.
        keepFolderOpen(id) {
            if (!id) return;
            this.openFolderChain(id, this.codex.folders);
            codexSaveOpenState(this.codex.codex.id);
        },
        // #folder-{id} on the codex page opens that folder and the ones above it.
        revealFolderFromHash(folders) {
            let hash;
            try {
                hash = decodeURIComponent(location.hash || '');
            } catch (error) {
                return;
            }
            const m = /^#folder-(.+)$/.exec(hash);
            if (!m) return;
            this.openFolderChain(m[1], folders, true);
            this.folderTarget = m[1];
        },
        // A folder with only sub-folders has no group of its own: its first descendant group stands in.
        firstGroupUnder(id) {
            const tree = codexFolderTree(this.codex.folders, this.codex.pages);
            const find = (nodes) => {
                for (const n of nodes) {
                    if (n.id === id) return n;
                    const f = find(n.children);
                    if (f) return f;
                }
                return null;
            };
            const first = (n) => {
                for (const c of n.children) {
                    if (c.pages.length) return c;
                    const f = first(c);
                    if (f) return f;
                }
                return null;
            };
            const start = find(tree.roots);
            const g = start && first(start);
            return g ? document.getElementById('group-' + g.id) : null;
        },
        scrollToFolder() {
            if (!this.hashScrolled && ['#wanted', '#recent', '#tags'].includes(location.hash)) {
                this.hashScrolled = true;
                document.getElementById(location.hash.slice(1))?.scrollIntoView({ block: 'start' });
                return;
            }
            if (!this.folderTarget) return;
            const el = document.getElementById('group-' + this.folderTarget) || this.firstGroupUnder(this.folderTarget) || document.getElementById('folder-' + this.folderTarget);
            this.folderTarget = '';
            if (!el) return;
            el.classList.add('codex-folder--target');
            el.scrollIntoView({ block: 'start' });
        },
        // After a reload, put keyboard focus back on a folder's row (or the New folder button).
        focusFolder(id) {
            this.$nextTick(() => {
                const el = id && document.getElementById('folder-' + id);
                (el ? el.querySelector(':scope > summary') : document.getElementById('new-folder-btn'))?.focus();
            });
        },
        // Only toggles the user caused are saved; the ones fired while the tree renders are not.
        onFolderToggle(event) {
            const d = event.target;
            if (!this.codex || !d.matches || !d.matches('details.codex-folder') || d !== this.toggledBy) return;
            this.toggledBy = null;
            codexTransientOpen.delete(d.dataset.id);
            codexOpenState[d.dataset.id] = d.open;
            codexSaveOpenState(this.codex.codex.id);
        },
        onTreeClick(event) {
            const sum = event.target.closest('summary');
            const m = event.target.closest('button[data-menu]');
            this.toggledBy = sum && !m ? sum.parentElement : null;
            if (m) {
                event.preventDefault();
                if (this.folderMenu.id === m.dataset.menu) this.closeFolderMenu();
                else this.openFolderMenu(m);
            }
        },
        // Tree drag and drop (mouse, Edit only). The custom type keeps the image file-drop handlers out of it.
        // A drop reuses the two move routes; the checks below mirror the server, so a refused drop has no drop cursor.
        treeDragTarget(event) {
            const strip = event.target.closest?.('.codex-root-drop');
            if (strip) return { el: strip, id: '' };
            const sum = event.target.closest?.('.codex-tree summary');
            return sum ? { el: sum, id: sum.parentElement.dataset.id } : null;
        },
        // Mirrors db.folderDepth: a top-level folder is level 1 (db.MaxFolderDepth is 10).
        folderLevels(id) {
            let n = 0;
            for (let cur = id; cur && n <= this.codex.folders.length; n++) cur = (this.folderById(cur) || {}).parent_id;
            return n;
        },
        // Mirrors db.folderHeight: the levels in the subtree under id, itself included.
        folderHeight(id, hops = 0) {
            let h = 0;
            if (hops <= this.codex.folders.length) {
                for (const f of this.codex.folders) {
                    if (f.parent_id === id) h = Math.max(h, this.folderHeight(f.id, hops + 1));
                }
            }
            return h + 1;
        },
        treeDropOk(targetId) {
            const it = this.treeDrag;
            if (!it || !this.codex) return false;
            if (it.kind === 'page') {
                const p = this.codex.pages.find((x) => x.id === it.id);
                return !!p && (p.folder_id || '') !== targetId;
            }
            const f = this.folderById(it.id);
            if (!f || (f.parent_id || '') === targetId) return false;
            for (let cur = targetId, n = 0; cur && n <= this.codex.folders.length; n++) {
                if (cur === f.id) return false; // itself or a descendant
                cur = (this.folderById(cur) || {}).parent_id;
            }
            return this.folderLevels(targetId) + this.folderHeight(f.id) <= 10;
        },
        onTreeDragStart(event) {
            const el = event.target.closest?.('[data-drag]');
            if (!el) return;
            const [kind, id] = el.dataset.drag.split(':');
            this.treeDrag = { kind, id };
            event.dataTransfer.effectAllowed = 'move';
            event.dataTransfer.setData('application/x-codex-item', el.dataset.drag);
            setTimeout(() => { // not inside the handler: changing the layout there can cancel the drag
                if (!this.treeDrag) return;
                el.classList.add('dragging');
                this.dragOn = true;
            });
        },
        clearTreeDrag() {
            this.treeDrag = null;
            this.dragOn = false;
            clearTimeout(this.springTimer);
            this.springId = '';
            document.querySelectorAll('.codex-tree-wrap .dragging, .codex-tree-wrap .over').forEach((el) => el.classList.remove('dragging', 'over'));
        },
        onTreeDragOver(event) {
            if (!this.treeDrag) return;
            const t = this.treeDragTarget(event);
            document.querySelectorAll('.codex-tree-wrap .over').forEach((el) => el !== (t && t.el) && el.classList.remove('over'));
            if (t && this.treeDropOk(t.id)) {
                event.preventDefault();
                event.dataTransfer.dropEffect = 'move';
                t.el.classList.add('over');
            }
            // Hovering a closed folder for a moment opens it, so the drop can go deeper.
            const d = t && t.id ? t.el.parentElement : null;
            if (!d || d.open) {
                clearTimeout(this.springTimer);
                this.springId = '';
                return;
            }
            if (this.springId === t.id) return;
            clearTimeout(this.springTimer);
            this.springId = t.id;
            this.springTimer = setTimeout(() => {
                codexTransientOpen.add(t.id);
                d.open = true; // onFolderToggle ignores it: the user did not toggle it
            }, 700);
        },
        onTreeDragLeave(event) {
            if (event.currentTarget.contains(event.relatedTarget)) return;
            document.querySelectorAll('.codex-tree-wrap .over').forEach((el) => el.classList.remove('over'));
            clearTimeout(this.springTimer);
            this.springId = '';
        },
        onTreeDrop(event) {
            const t = this.treeDragTarget(event);
            const it = this.treeDrag;
            if (!it || !t) return;
            event.preventDefault();
            const ok = this.treeDropOk(t.id);
            this.clearTreeDrag();
            if (ok) this.moveTreeItem(it.kind, it.id, t.id);
        },
        async moveTreeItem(kind, id, targetId, back = false) {
            const rec = kind === 'page' ? this.codex.pages.find((x) => x.id === id) : this.folderById(id);
            if (!rec) return;
            const name = kind === 'page' ? rec.title : rec.name;
            const from = (kind === 'page' ? rec.folder_id : rec.parent_id) || '';
            try {
                const { response, data } = kind === 'page'
                    ? await this.send(`/api/codices/pages/${encodeURIComponent(id)}/folder`, 'PUT', { folder_id: targetId })
                    : await this.send(`/api/codices/folders/${encodeURIComponent(id)}`, 'PUT', { parent_id: targetId });
                if (!response.ok) {
                    const d = {};
                    this.failDlg(d, response, data);
                    this.showMessage(d.error, 'error');
                    if (response.status === 404) await this.load();
                    return;
                }
                this.keepFolderOpen(targetId);
                const where = targetId ? (this.folderById(targetId) || {}).name : '';
                await this.load();
                const action = back ? null : { label: 'Undo', run: () => this.moveTreeItem(kind, id, from, true) };
                this.showMessage(back ? `Moved ${name} back.` : `Moved ${name} to ${where || 'the top level'}.`, 'success', action);
            } catch (error) {
                this.showMessage('Could not move it: ' + error.message, 'error');
            }
        },
        openFolderMenu(btn) {
            const f = this.folderById(btn.dataset.menu);
            if (!f) return;
            this.closeFolderMenu();
            const wrap = btn.closest('.codex-tree-wrap').getBoundingClientRect();
            const b = btn.getBoundingClientRect();
            btn.setAttribute('aria-expanded', 'true');
            this.folderMenu = { id: f.id, top: b.bottom - wrap.top, right: wrap.right - b.right, html: codexFolderMenuHtml(f) };
            this.$nextTick(() => this.$refs.folderMenu?.querySelector('button')?.focus());
        },
        closeFolderMenu(refocus = false) {
            if (!this.folderMenu.id) return;
            const btn = document.querySelector(`.codex-menu-btn[aria-expanded="true"]`);
            if (btn) btn.setAttribute('aria-expanded', 'false');
            this.folderMenu = { id: '', top: 0, right: 0, html: '' };
            if (refocus && btn) btn.focus();
        },
        onMenuKey(event) {
            if (event.key === 'Escape') {
                event.stopPropagation();
                this.closeFolderMenu(true);
                return;
            }
            const items = Array.from(this.$refs.folderMenu.querySelectorAll('button'));
            const i = items.indexOf(document.activeElement);
            if (event.key === 'ArrowDown') items[(i + 1) % items.length].focus();
            else if (event.key === 'ArrowUp') items[(i - 1 + items.length) % items.length].focus();
            else if (event.key === 'Home') items[0].focus();
            else if (event.key === 'End') items[items.length - 1].focus();
            else return;
            event.preventDefault();
        },
        onMenuClick(event) {
            const b = event.target.closest('button[data-act]');
            if (!b) return;
            const id = b.dataset.id;
            const act = b.dataset.act;
            this.closeFolderMenu();
            this.closeNav();
            if (act === 'newPage') this.openCreatePage('', id);
            else if (act === 'newFolder') this.openNewFolder(id);
            else if (act === 'rename') this.openRenameFolder(id);
            else if (act === 'folderTpl') this.openFolderTemplate(id);
            else if (act === 'move') this.openMoveFolder(id);
            else if (act === 'delete') this.deleteFolder(id);
        },
        folderOptions(folders, excludeId) {
            return codexFolderOptions(folders, excludeId);
        },
        folderById(id) {
            return this.codex.folders.find((f) => f.id === id);
        },
        // The codex screen's page list: one group per folder that holds pages, in tree order.
        get pageGroups() {
            if (!this.codex) return [];
            const tree = codexFolderTree(this.codex.folders, this.codex.pages);
            const byTitle = (a, b) => (codexNorm(a.title) < codexNorm(b.title) ? -1 : codexNorm(a.title) > codexNorm(b.title) ? 1 : 0);
            const out = [];
            const walk = (n, trail) => {
                const path = [...trail, n.name];
                if (n.pages.length) out.push({ id: n.id, label: path.join(' › '), pages: [...n.pages].sort(byTitle) });
                n.children.forEach((c) => walk(c, path));
            };
            tree.roots.forEach((r) => walk(r, []));
            if (tree.pages.length) out.push({ id: '_top', label: 'Not in a folder', pages: [...tree.pages].sort(byTitle) });
            return out;
        },
        get filteredPages() {
            const q = codexNorm(this.pageFilter);
            return this.codex.pages.filter((p) => !q || codexNorm(p.title).includes(q) || p.aliases.some((a) => codexNorm(a).includes(q)));
        },

        // fetch with JSON in and out. An expired login comes back as a redirect to the login page,
        // which is reported as status 401 so the editor can keep the draft instead of leaving.
        // Any other reply that isn't JSON is reported as a generic failure.
        async send(url, method, body) {
            const response = await fetch(url, body === undefined
                ? { method }
                : { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
            if (response.status === 401 || (response.redirected && new URL(response.url).pathname.includes('login'))) {
                return { response: { ok: false, status: 401 }, data: {} };
            }
            if (!response.headers.get('Content-Type')?.includes('application/json')) {
                return { response: { ok: false, status: response.ok ? 500 : response.status }, data: { error: 'unexpected reply from the server' } };
            }
            return { response, data: await response.json().catch(() => ({})) };
        },

        showArrivalMessage() {
            const q = new URLSearchParams(location.search);
            const texts = {
                saved: () => (Number(q.get('saved')) === 1 ? 'Page saved' : `Saved as revision ${q.get('saved')}`) + (Number(q.get('created')) > 0 ? `, and created ${this.countLabel(Number(q.get('created')), 'page')}` : ''),
                unchanged: () => 'Nothing changed, so no revision was saved.',
                renamed: () => `Renamed. Links were updated in ${this.countLabel(Number(q.get('renamed')), 'page')}.`,
                moved: () => 'Page moved.',
                copied: () => 'Copied.',
                page_deleted: () => 'Page deleted.'
            };
            const key = Object.keys(texts).find((k) => q.has(k));
            if (!key) return;
            this.showMessage(texts[key](), key === 'unchanged' ? 'info' : 'success');
            history.replaceState(null, '', location.pathname);
        },

        // ---- Links in the page body ----
        // Lookup for the stored link statuses of the page being viewed.
        get linkLookup() {
            const map = new Map((this.pageData?.links || []).map((l) => [codexNorm(l.text), l.page]));
            return (text) => map.get(codexNorm(text)) || null;
        },
        // Lookup by title or alias among the pages of this codex.
        get titleLookup() {
            const map = new Map();
            for (const t of this.titles) {
                for (const name of [t.title, ...t.aliases]) if (!map.has(codexNorm(name))) map.set(codexNorm(name), t);
            }
            return (text) => map.get(codexNorm(text)) || null;
        },
        // Lookup by name among every picture ever uploaded to the page (removed ones too, so old
        // revisions still show theirs).
        get imageLookup() {
            const map = new Map(Object.values(this.pageData?.image_index || {}).map((p) => [codexNorm(p.name), p]));
            return (name) => map.get(codexNorm(name)) || null;
        },
        // Renders {{list ...}} from the codex payload (none yet: "Loading…"; reading this.codex makes
        // bodyHtml render again when it arrives). Links here are the page's "Linked from" list.
        get listRenderer() {
            const c = this.codex;
            const cur = this.page ? this.page.id : '';
            const ctx = c && this.page && c.codex.id === this.page.codex.id ? { pages: c.pages, tags: c.tags, folders: c.folders, cur, back: new Set(this.linkedFrom.map((b) => b.id)) } : null;
            return (spec) => codexLiveList(spec, ctx);
        },
        get bodyHtml() {
            return this.page ? codexMarkdown(this.page.body, this.linkLookup, this.canEdit, this.imageLookup, this.listRenderer, this.canEdit, this.users) : '';
        },
        // Pages that link here by [[link]]; map pins are not shown.
        get linkedFrom() {
            return this.pageData ? this.pageData.backlinks.filter((b) => b.via.some((v) => v !== 'pin')) : [];
        },
        get scribes() {
            const p = this.page;
            return [...new Set([p.author_user, ...(p.amender_users || [])].map((u) => (u && u.display_name) || 'Unknown'))].join(', ');
        },
        // Contents rail: ids go on the rendered ## / ### headings (h3 / h4) from their text, so no
        // user text is ever injected as HTML.
        buildToc() {
            const body = this.$refs.body;
            if (!body || this.screen !== 'page') {
                this.toc = [];
                return;
            }
            const used = new Set();
            this.toc = Array.from(body.querySelectorAll('h3, h4')).filter((h) => h.textContent.trim() && h.getClientRects().length > 0).map((h) => {
                const base = 'sec-' + codexSlug(h.textContent);
                let id = base;
                for (let n = 2; used.has(id); n++) id = `${base}-${n}`;
                used.add(id);
                h.id = id;
                return { id, text: h.textContent.trim(), level: h.tagName === 'H3' ? 2 : 3 };
            });
            this.updateToc();
            if (!this.tocHashDone && this.toc.length) {
                this.tocHashDone = true;
                let hash = location.hash.slice(1);
                try {
                    hash = decodeURIComponent(hash);
                } catch (error) {
                    // keep the raw hash
                }
                const t = this.toc.find((x) => x.id === hash);
                if (t) document.getElementById(t.id)?.scrollIntoView({ block: 'start' });
            }
        },
        updateToc() {
            let active = this.toc.length ? this.toc[0].id : '';
            for (const t of this.toc) {
                const el = document.getElementById(t.id);
                if (el && el.getBoundingClientRect().top <= 140) active = t.id;
            }
            // At the very bottom the last heading may never reach the top: use the last one in view.
            if (this.toc.length && window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 2) {
                for (let i = this.toc.length - 1; i >= 0; i--) {
                    const el = document.getElementById(this.toc[i].id);
                    if (el && el.getBoundingClientRect().top < window.innerHeight) {
                        active = this.toc[i].id;
                        break;
                    }
                }
            }
            this.tocActive = active;
        },
        get previewHtml() {
            return this.draft ? codexMarkdown(this.draft.body, this.titleLookup, true, this.imageLookup, this.listRenderer, false, this.users) : '';
        },
        // Pages of the current codex (from the codex payload loadNav fetched), for previews and the editor.
        get titles() {
            return this.codex ? this.codex.pages : [];
        },
        onBodyClick(event) {
            const a = event.target.closest('a.wikilink--missing');
            if (!a) return;
            event.preventDefault();
            this.openCreatePage(a.dataset.text);
        },

        // ---- Codices shell ----
        get currentCodexId() {
            if (this.onCodexScreen) return this.codex ? this.codex.codex.id : this.route.id;
            return this.page ? this.page.codex.id : '';
        },
        get currentCodexName() {
            if (this.onCodexScreen) return this.codex ? this.codex.codex.name : '';
            return this.page ? this.page.codex.name : '';
        },
        get currentPageId() {
            return this.screen !== 'home' && !this.onCodexScreen && this.page ? this.page.id : '';
        },
        get pickLabel() {
            return this.acrossCodices ? 'All codices' : this.currentCodexName || 'Switch codex';
        },
        get searchPlaceholder() {
            return this.acrossCodices ? 'Search all codices' : this.currentCodexName ? 'Search ' + this.currentCodexName : 'Search this codex';
        },
        // The graph screen: fetches the codex's graph and hands it to codex-graph.js to draw.
        async loadGraph() {
            const id = this.route.id;
            this.graphLoading = true;
            this.graphError = '';
            this.graphReady = false;
            try {
                const { response, data } = await this.send(`/api/codices/c/${encodeURIComponent(id)}/graph`, 'GET');
                if (response.status === 401) {
                    window.location.href = '/login.html';
                    return;
                }
                if (!response.ok) {
                    this.graphError = data.error || 'Could not load the graph';
                    return;
                }
                const from = (new URLSearchParams(location.search).get('from') || '').trim();
                const fromNode = data.nodes.find((n) => n.id === from && !n.missing);
                this.graphFrom = fromNode || null;
                const pages = data.nodes.filter((n) => !n.missing).length;
                const wanted = data.nodes.length - pages;
                const pairs = new Set(data.links.map((l) => (l.source < l.target ? l.source + '|' + l.target : l.target + '|' + l.source))).size;
                this.graphSummary = `${this.countLabel(pages, 'page')} · ${this.countLabel(pairs, 'link')}` + (wanted ? ` · ${wanted} not written yet` : '');
                this.graphReady = true;
                await this.$nextTick();
                await window.mountCodexGraph(document.getElementById('gvRoot'), {
                    data,
                    currentId: fromNode ? fromNode.id : '',
                    canEdit: this.canEdit,
                    pageUrl: (n) => this.pageUrl(n),
                    onCreate: (title) => this.openCreatePage(title),
                });
            } catch (error) {
                this.graphError = 'Could not draw the graph: ' + error.message;
                this.graphReady = false;
            } finally {
                this.graphLoading = false;
            }
        },
        // The page screen's Local graph box: only fetched when the page has links either way.
        async loadLocalGraph(pageData) {
            const seq = ++this.localSeq;
            this.localGraph = false;
            if (!(pageData.links || []).length && !pageData.backlinks.some((b) => b.via.some((v) => v !== 'pin'))) return;
            try {
                const { response, data } = await this.send(`/api/codices/c/${encodeURIComponent(pageData.page.codex.id)}/graph?around=${encodeURIComponent(pageData.page.id)}`, 'GET');
                if (seq !== this.localSeq || !response.ok) return;
                this.localGraph = true;
                await this.$nextTick();
                const svg = document.getElementById('localSvg');
                if (!svg || !(await window.mountCodexLocalGraph(svg, data, pageData.page.id)) || seq !== this.localSeq) this.localGraph = false;
            } catch (error) {
                this.localGraph = false;
                console.error('Failed to draw the local graph:', error);
            }
        },
        // The Mentioned, not linked box: fetched after the page, quietly (it's only a hint).
        async loadMentions(id) {
            const seq = ++this.mentionSeq;
            this.mentions.loaded = false;
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(id)}/mentions`, 'GET');
                if (seq !== this.mentionSeq || !response.ok) return;
                this.mentions = { total: data.total, list: data.mentions, can_edit: data.can_edit, loaded: true, busy: false, progress: '' };
            } catch (error) {
                console.error('Failed to load the mentions:', error);
            }
        },
        // Links the mentions in one page; returns 'linked', 'stale' or 'failed' (failures are reported here).
        async linkMentionsIn(m, quiet) {
            const id = this.page.id;
            const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(id)}/mentions/link`, 'POST', { from_page_id: m.page.id, base_revision: m.current_revision });
            if (response.status === 409 && data.code === 'stale_base') return 'stale';
            if (!response.ok) {
                this.showMessage(this.sentence(data.error || 'Could not link the mentions'), 'error');
                return 'failed';
            }
            if (!quiet) this.showMessage(`Linked ${this.countLabel(m.count, 'mention')} in ${m.page.title}.`, 'success');
            return 'linked';
        },
        // After linking: the list and the Linked from list are reloaded.
        async refreshMentions() {
            const id = this.page.id;
            await this.loadMentions(id);
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(id)}`, 'GET');
                if (response.ok && this.pageData && this.pageData.page.id === id) this.pageData.backlinks = data.backlinks;
            } catch (error) {
                console.error('Failed to reload the links:', error);
            }
        },
        async linkMentions(m) {
            if (this.mentions.busy) return;
            this.mentions.busy = true;
            try {
                const res = await this.linkMentionsIn(m, false);
                if (res === 'stale') this.showMessage(`${m.page.title} changed in the meantime. The list is reloaded.`, 'info');
                await this.refreshMentions();
            } catch (error) {
                this.showMessage('Could not link the mentions: ' + error.message, 'error');
                this.mentions.busy = false;
            }
        },
        async linkAllMentions() {
            if (this.mentions.busy) return;
            const todo = [...this.mentions.list];
            this.mentions.busy = true;
            let pages = 0, mentions = 0, stale = 0;
            try {
                for (const m of todo) {
                    this.mentions.progress = `Linking ${pages + stale + 1} of ${todo.length}…`;
                    const res = await this.linkMentionsIn(m, true);
                    if (res === 'failed') break;
                    if (res === 'stale') stale++;
                    else {
                        pages++;
                        mentions += m.count;
                    }
                }
                if (pages) this.showMessage(`Linked ${this.countLabel(mentions, 'mention')} in ${this.countLabel(pages, 'page')}.` + (stale ? ` ${this.countLabel(stale, 'page')} changed in the meantime.` : ''), 'success');
                else if (stale) this.showMessage('The pages changed in the meantime. The list is reloaded.', 'info');
                await this.refreshMentions();
            } catch (error) {
                this.showMessage('Could not link the mentions: ' + error.message, 'error');
                this.mentions.busy = false;
                this.mentions.progress = '';
            }
        },
        // The tree and the side panel need the codex's own payload on the page screens too.
        // Everyone who can be mentioned, for the editor's @ list and the names shown; without it
        // mentions still link, under the name they were written with.
        async loadUsers() {
            try {
                const { response, data } = await this.send('/api/codices/users', 'GET');
                if (response.ok) this.users = data;
            } catch (error) {
                console.error('Failed to load the users:', error);
            }
        },
        async loadNav(page) {
            const seq = ++this.navSeq;
            try {
                const { response, data } = await this.send(`/api/codices/c/${encodeURIComponent(page.codex.id)}`, 'GET');
                if (seq !== this.navSeq || !response.ok) return;
                codexLoadOpenState(data.codex.id);
                this.openFolderChain(page.folder_id, data.folders, true); // the current page's folders show open, unsaved
                this.codex = data;
                this.syncTabs(data);
                this.$nextTick(() => this.revealCurrentPage());
            } catch (error) {
                console.error('Failed to load the page list:', error);
            }
        },
        // Scrolls the nav (not the window) so the current page is in view.
        revealCurrentPage() {
            const el = document.querySelector('.cx-nav [aria-current="page"]');
            const box = el && el.closest('.cx-nav-in');
            if (!box) return;
            const e = el.getBoundingClientRect();
            const b = box.getBoundingClientRect();
            if (e.top < b.top || e.bottom > b.bottom) box.scrollTop += e.top - b.top - box.clientHeight / 3;
        },
        toggleNav() {
            if (this.navOpen) {
                this.closeNav();
                return;
            }
            this.navAnim = true;
            this.navOpen = true;
            document.body.style.overflow = 'hidden'; // no scrolling the page behind the drawer
            this.$nextTick(() => this.$refs.navClose?.focus());
        },
        closeNav(refocus = true) {
            if (!this.navOpen) return;
            this.navOpen = false;
            document.body.style.overflow = '';
            if (refocus) this.$nextTick(() => this.$refs.menuBtn?.focus()); // once the page is no longer inert
        },
        onNavClick(event) {
            if (event.target.closest('a')) this.closeNav();
        },
        async loadCodexList() {
            if (this.allCodices) return this.allCodices;
            const { response, data } = await this.send('/api/codices', 'GET');
            if (!response.ok) throw new Error(data.error || 'request failed');
            this.allCodices = data.codices;
            return this.allCodices;
        },
        async togglePick() {
            this.pickOpen = !this.pickOpen;
            if (!this.pickOpen || this.allCodices) return;
            try {
                await this.loadCodexList();
            } catch (error) {
                this.pickOpen = false;
                this.showMessage('Could not load the codices: ' + error.message, 'error');
            }
        },
        // "/" jumps to the search box and "n" opens a quick note, unless the user is typing somewhere.
        onGlobalKey(event) {
            const note = event.key === 'n' || event.key === 'N';
            if ((event.key !== '/' && !note) || event.ctrlKey || event.metaKey || event.altKey) return;
            const t = event.target;
            if (t && (t.isContentEditable || (t.closest && t.closest('input, textarea, select, [contenteditable]')))) return;
            if (this.dlg.type || this.confirmDialog.open || document.querySelector('dialog[open]')) return;
            if (note) {
                if (!this.canNote) return;
                event.preventDefault();
                this.openQuickNote();
                return;
            }
            event.preventDefault();
            this.$refs.search?.focus();
        },

        // ---- Quick notes and the Inbox: private to their author, one inbox per codex ----
        // The Note button and the n key: anywhere in a codex the viewer can edit.
        get canNote() {
            return this.screen !== 'home' && this.canEdit && !!this.currentCodexId;
        },
        get noteCount() {
            return (this.codex && this.codex.codex.id === this.currentCodexId && this.codex.note_count) || 0;
        },
        setNoteCount(n) {
            if (this.codex && this.codex.codex.id === this.currentCodexId) this.codex.note_count = Math.max(0, n);
        },
        // The note to edit when given (from the inbox); otherwise a new one that keeps its own local draft.
        openQuickNote(note = null) {
            const codexId = this.currentCodexId;
            if (!codexId || !this.canEdit) return;
            let text = note ? note.body : '';
            if (!note) {
                try {
                    text = localStorage.getItem('codex-quicknote:' + codexId) || '';
                } catch (error) {
                    text = '';
                }
            }
            this.dlg = { type: 'quickNote', codexId, codexName: this.currentCodexName, noteId: note ? note.id : '', text, error: '', busy: false };
            this.focusDlg();
        },
        saveNoteDraft() {
            const d = this.dlg;
            if (d.type !== 'quickNote' || d.noteId) return;
            try {
                if (d.text) localStorage.setItem('codex-quicknote:' + d.codexId, d.text);
                else localStorage.removeItem('codex-quicknote:' + d.codexId);
            } catch (error) {
                // storage may be blocked; the draft just isn't kept
            }
        },
        async submitQuickNote(d) {
            const body = d.text.trim();
            if (!body) {
                d.error = 'Write something first.';
                return;
            }
            if (body.length > 5000) {
                d.error = 'A note can be up to 5000 characters.';
                return;
            }
            const base = `/api/codices/c/${encodeURIComponent(d.codexId)}/notes`;
            const { response, data } = d.noteId
                ? await this.send(`/api/codices/notes/${encodeURIComponent(d.noteId)}`, 'PUT', { body })
                : await this.send(base, 'POST', { body });
            if (!response.ok) {
                this.failDlg(d, response, data);
                return;
            }
            this.closeDlg();
            if (d.noteId) {
                const n = this.notes.find((x) => x.id === d.noteId);
                if (n) n.body = body;
                this.showMessage('Note saved.', 'success');
                return;
            }
            try {
                localStorage.removeItem('codex-quicknote:' + d.codexId);
            } catch (error) {
                // nothing to clear
            }
            this.setNoteCount(this.noteCount + 1);
            if (this.screen === 'inbox') {
                this.notes.unshift(data.note);
                this.showMessage('In your inbox.', 'success');
            } else {
                this.showMessage('In your inbox.', 'success', { label: 'Open inbox', run: () => { window.location.href = this.inboxUrl(); } });
            }
        },
        async loadInbox() {
            this.notesLoading = true;
            this.notesError = '';
            try {
                const { response, data } = await this.send(`/api/codices/c/${encodeURIComponent(this.route.id)}/notes`, 'GET');
                if (!response.ok) {
                    this.notesError = response.status === 401 ? 'Your login has expired. Log in again, then reload.' : this.sentence(data.error || 'Could not load your inbox');
                    return;
                }
                this.notes = data.notes;
                this.setNoteCount(this.canEdit ? data.notes.length : 0);
            } catch (error) {
                this.notesError = 'Could not load your inbox: ' + error.message;
            } finally {
                this.notesLoading = false;
            }
        },
        noteWhen(n) {
            return this.formatStamp(n.created_at);
        },
        // The first line of a note as a page title: no [ ] or |, up to 120 characters.
        noteTitle(body) {
            const line = (body.split('\n').find((l) => l.trim()) || '').replace(/[\[\]|]/g, '');
            return line.split(/\s+/).filter(Boolean).join(' ').slice(0, 120);
        },
        fileNoteAsPage(n) {
            this.openCreatePage(this.noteTitle(n.body), null, '', n);
        },
        async deleteNote(n) {
            try {
                const { response, data } = await this.send(`/api/codices/notes/${encodeURIComponent(n.id)}`, 'DELETE');
                if (!response.ok && response.status !== 404) {
                    this.showMessage(response.status === 401 ? 'Your login has expired. Log in again, then try once more.' : this.sentence(data.error || 'Could not delete the note'), 'error');
                    return;
                }
                this.notes = this.notes.filter((x) => x.id !== n.id);
                this.setNoteCount(this.noteCount - 1);
                this.showMessage('Note deleted.', 'success', { label: 'Undo', run: () => this.undoDeleteNote(n) });
            } catch (error) {
                this.showMessage('Could not delete the note: ' + error.message, 'error');
            }
        },
        // Undo posts the same text again (a new note, at the top).
        async undoDeleteNote(n) {
            try {
                const { response, data } = await this.send(`/api/codices/c/${encodeURIComponent(n.codex_id)}/notes`, 'POST', { body: n.body });
                if (!response.ok) {
                    this.showMessage(data.code === 'inbox_full' ? 'Your inbox is full, so the note could not come back.' : this.sentence(data.error || 'Could not bring the note back'), 'error');
                    return;
                }
                this.notes.unshift(data.note);
                this.setNoteCount(this.noteCount + 1);
                this.showMessage('Note back in your inbox.', 'success');
            } catch (error) {
                this.showMessage('Could not bring the note back: ' + error.message, 'error');
            }
        },
        openAddToPage(n) {
            this.dlg = { type: 'addToPage', note: n, pickQ: '', pickOpen: false, pickIdx: 0, linked: null, error: '', busy: false };
            this.$nextTick(() => requestAnimationFrame(() => document.getElementById('dApPick')?.focus()));
        },
        // What Add to page appends: a signed, dated line, then the note (the server writes the same).
        get addNotePreview() {
            const d = this.dlg;
            if (d.type !== 'addToPage') return '';
            const day = new Date().toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
            return `_${day} · ${this.displayName}_\n${d.note.body}`;
        },
        async submitAddToPage(d) {
            if (!d.linked) {
                d.error = 'Choose a page.';
                return;
            }
            const { response, data } = await this.send(`/api/codices/notes/${encodeURIComponent(d.note.id)}/append`, 'POST', { page_id: d.linked.id });
            if (!response.ok) {
                this.failDlg(d, response, data);
                return;
            }
            const target = d.linked;
            this.closeDlg();
            this.notes = this.notes.filter((x) => x.id !== d.note.id);
            this.setNoteCount(this.noteCount - 1);
            this.showMessage(`Added to ${target.title}.`, 'success', { label: 'Open page', run: () => { window.location.href = this.pageUrl({ id: target.id, title: target.title }); } });
        },
        // On phones the results are full width right under the top bar (CSS reads this).
        placeResults() {
            const bar = document.querySelector('.cx-top');
            if (bar) document.documentElement.style.setProperty('--cx-results-top', bar.getBoundingClientRect().bottom + 'px');
        },
        onSearchFocus() {
            this.placeResults();
            this.searchOpen = !!this.searchQ.trim();
            this.ensureSearchTitles();
        },
        onSearchInput() {
            this.placeResults();
            this.searchOpen = true;
            this.searchIdx = -1;
            this.ensureSearchTitles();
        },
        // Titles and aliases of all visible codices, fetched once on Home; elsewhere the codex payload is searched.
        async ensureSearchTitles() {
            if (!this.acrossCodices || this.searchKey) return; // other screens search the codex payload
            this.searchKey = '*';
            this.searchLoading = true;
            try {
                const { response, data } = await this.send('/api/codices/titles', 'GET');
                if (response.ok) this.searchTitles = data;
                else this.searchKey = '';
            } catch (error) {
                this.searchKey = '';
                console.error('Failed to load the titles for search:', error);
            } finally {
                this.searchLoading = false;
            }
        },
        get searchResults() {
            const q = codexNorm(this.searchQ);
            if (!q) return [];
            const names = new Map((this.allCodices || []).map((c) => [c.id, c.name]));
            const out = [];
            for (const t of this.acrossCodices ? this.searchTitles : this.titles) {
                const title = codexNorm(t.title);
                let score = title.startsWith(q) ? 0 : title.includes(q) ? 1 : -1;
                let via = '';
                if (score < 0) {
                    for (const a of t.aliases || []) {
                        const n = codexNorm(a);
                        if (n.includes(q)) {
                            score = n.startsWith(q) ? 2 : 3;
                            via = a;
                            break;
                        }
                    }
                }
                if (score < 0) continue;
                const meta = [via ? 'also ' + via : '', this.acrossCodices ? names.get(t.codex_id) || '' : ''].filter(Boolean).join(' · ');
                out.push({ id: t.id, title: t.title, score, meta });
            }
            out.sort((a, b) => a.score - b.score || (codexNorm(a.title) < codexNorm(b.title) ? -1 : 1));
            return out.slice(0, 8);
        },
        onSearchKey(event) {
            const list = this.searchResults;
            if (event.key === 'Escape') {
                if (this.searchOpen) this.searchOpen = false;
                else event.target.blur();
            } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
                if (!list.length) return;
                this.searchOpen = true;
                const n = this.searchIdx + (event.key === 'ArrowDown' ? 1 : -1);
                this.searchIdx = (n + list.length) % list.length;
            } else if (event.key === 'Enter') {
                const r = list[this.searchIdx >= 0 ? this.searchIdx : 0];
                if (r) window.location.href = this.pageUrl(r);
            } else return;
            event.preventDefault();
        },
        // Page tabs: the last few pages opened in a codex, kept per codex in localStorage.
        readTabs(codexId) {
            try {
                const v = JSON.parse(localStorage.getItem('codex-tabs:' + codexId));
                return Array.isArray(v) ? v.filter((t) => t && typeof t.id === 'string' && typeof t.title === 'string').slice(-6) : [];
            } catch (error) {
                return [];
            }
        },
        writeTabs(codexId, list) {
            try {
                localStorage.setItem('codex-tabs:' + codexId, JSON.stringify(list));
            } catch (error) {
                // storage may be blocked; the tabs just don't persist
            }
        },
        // Drops tabs for pages that are gone or moved away, and refreshes titles from the codex payload.
        syncTabs(data) {
            const byId = new Map(data.pages.map((p) => [p.id, p]));
            const list = this.readTabs(data.codex.id).filter((t) => byId.has(t.id)).map((t) => ({ id: t.id, title: byId.get(t.id).title }));
            this.writeTabs(data.codex.id, list);
            this.tabs = list;
        },
        trackTab(page) {
            const list = this.readTabs(page.codex.id);
            const i = list.findIndex((t) => t.id === page.id);
            if (i >= 0) list[i].title = page.title;
            else list.push({ id: page.id, title: page.title });
            while (list.length > 6) list.splice(Math.max(0, list.findIndex((t) => t.id !== page.id)), 1);
            this.writeTabs(page.codex.id, list);
            this.tabs = list;
        },
        closeTab(t) {
            const i = this.tabs.findIndex((x) => x.id === t.id);
            const left = this.tabs.filter((x) => x.id !== t.id);
            this.tabs = left;
            this.writeTabs(this.currentCodexId, left);
            if (t.id !== this.currentPageId) return;
            const next = left[Math.min(i, left.length - 1)];
            window.location.href = next ? this.pageUrl(next) : '/codices/c/' + encodeURIComponent(this.currentCodexId);
        },
        // More menu on the page screens
        moreItems() {
            return Array.from(this.$refs.moreMenu.querySelectorAll('button')).filter((b) => b.offsetParent !== null);
        },
        toggleMore() {
            this.moreOpen = !this.moreOpen;
            if (this.moreOpen) this.$nextTick(() => this.moreItems()[0]?.focus());
        },
        moreDo(action) {
            this.moreOpen = false;
            this[action]();
        },
        onMoreKey(event) {
            if (!this.moreOpen) return;
            if (event.key === 'Escape') {
                event.stopPropagation();
                this.moreOpen = false;
                document.getElementById('cxMoreBtn')?.focus();
                return;
            }
            const items = this.moreItems();
            const i = items.indexOf(document.activeElement);
            if (event.key === 'ArrowDown') items[(i + 1) % items.length].focus();
            else if (event.key === 'ArrowUp') items[(i - 1 + items.length) % items.length].focus();
            else return;
            event.preventDefault();
        },
        // Link previews: mouse only, after a short hover; the card's text is fetched once per page.
        onBodyOver(event) {
            if (event.pointerType !== 'mouse') return;
            const a = event.target.closest && event.target.closest('a.wikilink');
            if (!a || a === this.peekFor) return;
            this.hidePeek();
            this.peekFor = a;
            this.peekTimer = setTimeout(() => this.showPeek(a), 300);
        },
        onBodyOut(event) {
            const a = event.target.closest && event.target.closest('a.wikilink');
            if (!a || (event.relatedTarget && a.contains(event.relatedTarget))) return;
            this.hidePeek();
        },
        hidePeek() {
            clearTimeout(this.peekTimer);
            this.peekFor = null;
            this.peek.open = false;
        },
        async showPeek(a) {
            const info = a.classList.contains('wikilink--missing')
                ? { title: a.dataset.text, where: '', text: 'No page yet.' + (this.canEdit ? ' Click to create.' : '') }
                : await this.peekInfo(a.dataset.pid);
            if (!info || this.peekFor !== a) return;
            const r = a.getBoundingClientRect();
            const x = Math.max(8, Math.min(r.left, window.innerWidth - Math.min(300, window.innerWidth - 16) - 8));
            this.peek = { open: true, x, y: r.bottom + 6, ...info };
            this.$nextTick(() => {
                const h = document.querySelector('.cx-peek')?.offsetHeight || 0;
                if (r.bottom + 6 + h > window.innerHeight - 8 && r.top - h - 6 > 8) this.peek.y = r.top - h - 6;
            });
        },
        peekInfo(id) {
            if (!id) return Promise.resolve(null);
            if (!codexPeekCache[id]) {
                codexPeekCache[id] = this.send(`/api/codices/pages/${encodeURIComponent(id)}`, 'GET').then(({ response, data }) => {
                    if (response.status === 404) return null; // gone: cached, so it isn't asked again
                    if (!response.ok || !data.page) throw new Error('request failed');
                    const p = data.page;
                    return { title: p.title, where: [p.codex.name, ...(p.folder_path || []).map((f) => f.name)].join(' › '), text: codexPlainExcerpt(p.body) || 'This page is empty.' };
                }).catch((error) => {
                    console.error('Failed to load the preview:', error);
                    delete codexPeekCache[id]; // a passing failure: try again on the next hover
                    return null;
                });
            }
            return codexPeekCache[id];
        },

        // ---- Timeline pages ----
        // Events save at once, each change on its own; they are not part of the page's revisions.
        get isTimeline() {
            return !!(this.page && this.page.kind === 'timeline');
        },
        get tlData() {
            return (this.pageData && this.pageData.timeline) || { events: [], calendar: { months: [], era: '', before_era: '' } };
        },
        get tlEvents() {
            return this.tlData.events;
        },
        get tlCal() {
            return this.tlData.calendar;
        },
        get tlHtml() {
            return this.isTimeline ? codexTlHtml(this.tlEvents, this.tlCal, { canEdit: this.canEdit, justAdded: this.tlJustAdded, tagUrl: (n) => this.tagUrl(n) }) : '';
        },
        get tlJump() {
            return codexTlJump(codexTlGroups(this.tlEvents), this.tlCal);
        },
        // "10 events, 5 with a page · 790–1204 AR" and the usual revision line.
        get tlSummary() {
            const p = this.page;
            const rev = 'Revision ' + p.current_revision + ' · last edited by ' + p.updated_by_user.display_name + ' ' + this.formatStamp(p.updated_at);
            const ev = this.tlEvents;
            if (!ev.length) return 'No events yet · ' + rev;
            const c = this.tlCal;
            const years = ev.map((e) => e.year);
            const lo = Math.min(...years);
            const hi = Math.max(...years);
            let range = codexTlYear(lo, c);
            if (lo !== hi) range = lo >= 0 ? `${lo}–${hi}` + (c.era ? ' ' + c.era : '') : `${codexTlYear(lo, c)} – ${codexTlYear(hi, c)}`;
            return `${this.countLabel(ev.length, 'event')}, ${ev.filter((e) => e.linked).length} with a page · ${range} · ${rev}`;
        },
        // The timelines that have an event pointing at this page.
        get onTimelines() {
            return ((this.pageData && this.pageData.on_timelines) || []).map((o) => ({
                key: o.event_id,
                timeline: o.timeline,
                date: codexTlDate(o, o.calendar),
                url: this.pageUrl(o.timeline) + '#ev-' + encodeURIComponent(o.event_id)
            }));
        },
        // Pages of this codex that match the picker's text (title or alias), at most 8.
        get tlPickResults() {
            const d = this.dlg;
            if ((d.type !== 'event' && d.type !== 'addToPage') || !this.codex) return [];
            const q = codexNorm(d.pickQ);
            return this.codex.pages
                .filter((p) => p.id !== (d.type === 'event' ? this.page.id : '') && (!q || codexNorm(p.title).includes(q) || p.aliases.some((a) => codexNorm(a).includes(q))))
                .slice(0, 8)
                .map((p) => this.tlPickItem(p));
        },
        tlPickItem(p) {
            const folder = p.folder_id ? (this.folderById(p.folder_id) || {}).name || '' : 'Top level';
            const tags = (p.tag_ids || []).map((id) => (this.codex.tags.find((t) => t.id === id) || {}).name).filter(Boolean);
            return { id: p.id, title: p.title, folder, tags };
        },
        get monthOptions() {
            const d = this.dlg;
            const names = this.tlCal.months;
            const count = Math.max(names.length || 12, Number(d.month) || 0);
            return Array.from({ length: count }, (_, i) => ({ n: i + 1, label: names[i] ? `${i + 1}. ${names[i]}` : String(i + 1) }));
        },
        get calExample() {
            const d = this.dlg;
            if (d.type !== 'calendar') return '';
            const cal = { months: d.months.split('\n').map((s) => s.trim()).filter(Boolean), era: d.era.trim(), before_era: d.beforeEra.trim() };
            return codexTlDate({ year: 1203, month: 2, day: 14 }, cal) + ' · ' + codexTlDate({ year: -40, month: 0, day: 0 }, cal);
        },
        // The year row under the sticky bar is the current one in Jump to.
        updateJump() {
            if (!this.isTimeline) return;
            const rows = document.querySelectorAll('.tl-year');
            if (!rows.length) return;
            let cur = rows[0];
            for (const r of rows) {
                if (r.parentElement.getBoundingClientRect().top <= 140) cur = r;
            }
            const y = Number(cur.id.slice(1));
            if (y !== this.tlActiveYear) this.tlActiveYear = y;
        },
        // Flashes an event row and scrolls it into view (after the row exists).
        tlFlash(id) {
            clearTimeout(this.tlJustTimer);
            this.tlJustAdded = id;
            this.tlJustTimer = setTimeout(() => (this.tlJustAdded = ''), 2500);
            this.$nextTick(() => document.getElementById('ev-' + id)?.scrollIntoView({ block: 'center', behavior: 'smooth' }));
        },
        scrollToEventHash() {
            const m = /^#ev-(.+)$/.exec(location.hash);
            if (!m) return;
            let id = m[1];
            try {
                id = decodeURIComponent(id);
            } catch (error) {
                // keep the raw hash
            }
            if (this.tlEvents.some((e) => e.id === id)) this.tlFlash(id);
        },
        // Edit buttons and "Add the first event" inside the timeline.
        onTimelineClick(event) {
            const edit = event.target.closest('[data-edit]');
            if (edit) this.openEvent(this.tlEvents.find((e) => e.id === edit.dataset.edit) || null);
            else if (event.target.closest('[data-add]')) this.openEvent(null);
        },
        async refreshTimeline() {
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/timeline`, 'GET');
                if (response.ok) this.pageData.timeline = data;
            } catch (error) {
                console.error('Failed to reload the timeline:', error);
            }
        },
        // ev: an event of this timeline to edit, or null to add one.
        async openEvent(ev) {
            if (!this.canEdit || !this.isTimeline) return;
            if (this.navPromise) await this.navPromise; // the codex's pages, for the page picker
            const l = ev && ev.linked;
            this.dlg = {
                type: 'event', eventId: ev ? ev.id : '', title: ev ? ev.title : '', ownTitle: ev && !l ? ev.title : '',
                linked: l ? { id: l.id, title: l.title, folder: l.folder || 'Top level', tags: l.tags } : null,
                pickQ: '', pickOpen: false, pickIdx: 0,
                year: ev ? String(ev.year) : '', month: ev && ev.month ? String(ev.month) : '', day: ev && ev.day ? String(ev.day) : '',
                createPage: false, error: '', busy: false
            };
            this.$nextTick(() => requestAnimationFrame(() => document.getElementById(ev ? 'dEvYear' : 'dEvPick')?.focus()));
        },
        // Arrow keys and Enter in the page picker; Esc closes the list only, not the dialog.
        onPickKey(event) {
            const d = this.dlg;
            const list = this.tlPickResults;
            if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
                event.preventDefault();
                d.pickOpen = true;
                if (list.length) d.pickIdx = (d.pickIdx + (event.key === 'ArrowDown' ? 1 : -1) + list.length) % list.length;
            } else if (event.key === 'Enter') {
                event.preventDefault();
                if (d.pickOpen && list[d.pickIdx]) this.pickPage(list[d.pickIdx]);
            } else if (event.key === 'Escape' && d.pickOpen) {
                event.stopPropagation();
                d.pickOpen = false;
            }
        },
        pickPage(p) {
            const d = this.dlg;
            d.linked = { id: p.id, title: p.title, folder: p.folder, tags: p.tags };
            if (d.type === 'addToPage') {
                d.pickQ = '';
                d.pickOpen = false;
                return;
            }
            d.title = p.title;
            d.pickQ = '';
            d.pickOpen = false;
            d.createPage = false;
            this.$nextTick(() => document.getElementById('dEvYear')?.focus());
        },
        unlinkEvent() {
            const d = this.dlg;
            d.linked = null;
            d.title = d.ownTitle;
            this.$nextTick(() => document.getElementById('dEvPick')?.focus());
        },
        async submitEvent(d) {
            const title = d.title.trim().split(/\s+/).join(' ');
            if (!d.linked && !title) {
                d.error = 'Give the event a title or link a page.';
                return;
            }
            if (!d.linked && title.length > 120) {
                d.error = 'The title can be up to 120 characters.';
                return;
            }
            if (!d.linked && d.createPage && /[\[\]|]/.test(title)) {
                d.error = "A page title can't contain [ ] or |.";
                return;
            }
            const year = String(d.year).trim();
            if (!/^-?\d+$/.test(year) || Math.abs(Number(year)) > 1e9) {
                d.error = 'Enter the year as a whole number.';
                return;
            }
            const month = Number(d.month) || 0;
            const day = month ? Number(d.day) || 0 : 0;
            if (month < 0 || month > 100 || day < 0 || day > 100) {
                d.error = 'The month and day must be between 1 and 100.';
                return;
            }
            const url = `/api/codices/pages/${encodeURIComponent(this.page.id)}/timeline/events` + (d.eventId ? '/' + encodeURIComponent(d.eventId) : '');
            const { response, data } = await this.send(url, d.eventId ? 'PUT' : 'POST', {
                title: d.linked ? d.linked.title : title, linked_page_id: d.linked ? d.linked.id : '',
                year: Number(year), month, day, create_page: !d.linked && !d.eventId && d.createPage
            });
            if (response.ok) {
                this.pageData.timeline = data.timeline;
                const id = d.eventId || data.event.id;
                this.closeDlg();
                this.tlFlash(id);
                if (data.page) {
                    this.showMessage(`Created the page “${data.page.title}”.`, 'success');
                    this.loadNav(this.page);
                }
                return;
            }
            if (data.code === 'event_deleted') this.refreshTimeline();
            this.failDlg(d, response, data);
        },
        async deleteEvent() {
            const d = this.dlg;
            if (d.busy || !d.eventId) return;
            const ev = this.tlEvents.find((e) => e.id === d.eventId);
            const ok = await this.askConfirm({
                eyebrow: this.page.title,
                title: `Delete “${ev ? ev.title : d.title}” from the timeline?`,
                message: 'Its page, if it has one, stays.',
                confirmLabel: 'Delete event',
                danger: true
            });
            if (!ok || this.dlg !== d) return;
            d.busy = true;
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/timeline/events/${encodeURIComponent(d.eventId)}`, 'DELETE');
                if (response.ok) {
                    this.pageData.timeline = data.timeline;
                    this.closeDlg();
                    return;
                }
                if (data.code === 'event_deleted') {
                    this.refreshTimeline();
                    this.closeDlg();
                    this.showMessage('That event was already deleted.', 'info');
                    return;
                }
                this.failDlg(d, response, data);
            } catch (error) {
                d.error = 'Something went wrong: ' + error.message;
            } finally {
                d.busy = false;
            }
        },
        openCalendar() {
            const c = this.tlCal;
            this.dlg = { type: 'calendar', months: c.months.join('\n'), era: c.era, beforeEra: c.before_era, error: '', busy: false };
            this.focusDlg();
        },
        async submitCalendar(d) {
            const months = d.months.split('\n').map((s) => s.trim()).filter(Boolean);
            const era = d.era.trim();
            const beforeEra = d.beforeEra.trim();
            if (months.length > 100 || months.some((m) => Array.from(m).length > 40)) {
                d.error = 'Use up to 100 month names of up to 40 characters each.';
                return;
            }
            if (Array.from(era).length > 12 || Array.from(beforeEra).length > 12) {
                d.error = 'An era can be up to 12 characters.';
                return;
            }
            const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/timeline/calendar`, 'PUT', { months, era, before_era: beforeEra });
            if (response.ok) {
                this.pageData.timeline = { ...this.tlData, calendar: data.calendar };
                this.closeDlg();
                this.showMessage('Calendar saved.', 'success');
                return;
            }
            this.failDlg(d, response, data);
        },
        // Arrow keys move through the page types like a radio group; Map (disabled) is skipped.
        onKindKey(event) {
            const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[event.key];
            if (!step) return;
            event.preventDefault();
            const kinds = ['lore', 'timeline'];
            const d = this.dlg;
            d.kind = kinds[(kinds.indexOf(d.kind) + step + kinds.length) % kinds.length];
            this.$nextTick(() => document.querySelector('.cx-type[aria-checked="true"]')?.focus());
        },

        // ---- Dialogs ----
        get dlgEyebrow() {
            if (this.dlg.type === 'event' || this.dlg.type === 'calendar') return this.page ? this.page.title : 'Timeline';
            return { quickNote: 'Only you see it until you file it', addToPage: 'Filing a note', createPage: this.dlg.note ? 'Filing a note' : 'Codex', codex: 'Codex', templates: 'Codex', template: 'Template', rename: 'Page', aliases: 'Page', move: 'Page', copyPage: 'Page', saveTemplate: 'Page', folder: 'Folder', folderTpl: 'Folder', folderMove: 'Folder', pageFolder: 'Page', tags: 'Page', tag: 'Tag', createMissing: 'Page saved' }[this.dlg.type] || '';
        },
        get dlgTitle() {
            const d = this.dlg;
            if (d.type === 'createPage') return 'New page';
            if (d.type === 'quickNote') return d.noteId ? 'Edit note' : 'Quick note';
            if (d.type === 'addToPage') return 'Add to page';
            if (d.type === 'createMissing') return d.rows.length === 1 ? 'Create this page?' : `Create these ${d.rows.length} pages?`;
            if (d.type === 'event') return d.eventId ? 'Edit event' : 'Add event';
            if (d.type === 'calendar') return 'Calendar';
            return { codex: d.codex ? 'Edit codex' : 'New codex', templates: 'Templates', template: d.template ? 'Edit template' : 'New template', rename: 'Rename page', aliases: 'Aliases', move: 'Move to another codex', copyPage: 'Copy page', saveTemplate: 'Save as template', folder: d.folder ? 'Rename folder' : 'New folder', folderMove: 'Move folder', folderTpl: 'New pages start from', pageFolder: 'Change folder', tags: 'Tags', tag: d.tag ? 'Rename tag' : 'New tag' }[d.type] || '';
        },
        get dlgWide() {
            return ['createPage', 'createMissing', 'templates', 'template'].includes(this.dlg.type);
        },
        get dlgSubmit() {
            const d = this.dlg;
            if (d.type === 'createPage') return d.busy ? 'Creating...' : 'Create page';
            if (d.type === 'quickNote') return d.busy ? 'Saving...' : d.noteId ? 'Save' : 'Save to inbox';
            if (d.type === 'addToPage') return d.busy ? 'Adding...' : 'Add to page';
            if (d.type === 'createMissing') return d.busy ? 'Creating...' : 'Save and create';
            if (d.type === 'copyPage') return d.busy ? 'Copying...' : 'Copy page';
            if (d.type === 'event') return d.busy ? 'Saving...' : d.eventId ? 'Save event' : 'Add event';
            if (d.type === 'calendar') return d.busy ? 'Saving...' : 'Save calendar';
            if (d.type === 'template' || d.type === 'saveTemplate') return d.busy ? 'Saving...' : 'Save template';
            return d.busy ? 'Saving...' : d.type === 'tag' && !d.tag ? 'Create tag' : d.type === 'move' ? 'Move page' : d.type === 'folderMove' ? 'Move folder' : (d.type === 'codex' && !d.codex) || (d.type === 'folder' && !d.folder) ? 'Create' + (d.type === 'codex' ? ' codex' : ' folder') : 'Save';
        },
        get moveWarning() {
            const n = this.pageData?.incoming_count || 0;
            const here = this.page?.codex.name;
            const onTl = new Set((this.pageData?.on_timelines || []).map((o) => o.timeline.id)).size;
            return (this.isTimeline ? 'Its events keep their titles but lose their page links. ' : '') +
                (onTl > 0 ? `It leaves ${this.countLabel(onTl, 'timeline')}: those events stay as plain events. ` : '') + (n > 0
                ? `${this.countLabel(n, 'link')} from ${here} will turn into missing links, because links only reach pages in their own codex. `
                : '') + "This page's own links are checked again in the new codex, and it goes to the top level of its folders." +
                (this.pageData?.tags?.length ? ' Its tags stay behind, because tags belong to a codex.' : '');
        },
        // quiet: a background read for the menu, which says nothing if it fails (opening Copy page or Move asks again).
        async ensureCodices(quiet = false) {
            if (this.codicesLoaded && (quiet || this.codexChoices.length)) return;
            try {
                const list = await this.loadCodexList();
                this.codexChoices = list.filter((w) => w.can_edit).map((w) => ({ id: w.id, name: w.name }));
                this.codicesLoaded = true;
            } catch (error) {
                if (!quiet) this.showMessage('Could not load the codices: ' + error.message, 'error');
            }
        },
        focusDlg() {
            this.$nextTick(() => requestAnimationFrame(() => this.$refs.dlgFocus?.focus()));
        },
        closeDlg() {
            this.dlg = { type: '' };
        },
        // Cancel, Esc, the backdrop and the cross: the template editor asks before dropping unsaved
        // changes and goes back to its list; everything else closes.
        async cancelDlg() {
            const d = this.dlg;
            if (d.busy) return;
            if (d.type === 'createMissing') {
                this.finishSave(d);
                return;
            }
            if (d.type === 'template') {
                if (JSON.stringify([d.name, d.description, d.tags, d.body]) !== d.snap) {
                    // After the key event has reached every handler, or the confirm's own Esc handler closes it at once.
                    await new Promise((resolve) => setTimeout(resolve));
                    const ok = await this.askConfirm({
                        eyebrow: 'Template', title: 'Discard your changes?', message: 'The template has changes that are not saved.', confirmLabel: 'Discard changes'
                    });
                    if (!ok || this.dlg !== d) return;
                }
                if (d.back) {
                    this.openTemplates();
                    return;
                }
            }
            this.closeDlg();
        },
        // folderId: the folder to prefill (a page's missing links start in its own folder); tplId: the
        // template to start from (the last one used in this codex when empty).
        // note: an inbox note to file as this page (its text goes under the template's).
        async openCreatePage(title = '', folderId = null, tplId = '', note = null) {
            if (!this.canEdit) {
                this.showMessage("You can't add pages to this codex.", 'error');
                return;
            }
            const codexId = this.currentCodexId;
            if (!codexId) {
                this.showMessage('Make a codex first.', 'error');
                return;
            }
            let templates;
            try {
                templates = await this.loadTemplates(codexId);
            } catch (error) {
                this.showMessage('Could not load the templates: ' + error.message, 'error');
                return;
            }
            const folders = this.codex && this.codex.codex.id === codexId ? this.codex.folders || [] : (await this.fetchFolders(codexId)) || [];
            if (folderId === null) folderId = this.page ? this.page.folder_id || '' : '';
            if (!folders.some((f) => f.id === folderId)) folderId = '';
            const tplTouched = !!tplId;
            if (!tplId) tplId = this.startTemplate(templates, folders, folderId, codexId);
            if (!templates.some((t) => t.id === tplId)) tplId = 'builtin:blank';
            this.dlg = { type: 'createPage', title, codexId, folders, folderId, templates, tplId, tplTouched, kind: 'lore', note, error: '', busy: false };
            this.focusDlg();
        },
        // The folder's default template (own or inherited) when it still exists, else the last one used in this codex.
        startTemplate(templates, folders, folderId, codexId) {
            const def = codexFolderDefault(folders, folderId);
            if (def && templates.some((t) => t.id === def)) return def;
            try {
                return localStorage.getItem('codex-last-template:' + codexId) || '';
            } catch (error) {
                return '';
            }
        },
        // New page: another folder brings its default along, unless a card was already picked.
        onNpFolder() {
            const d = this.dlg;
            if (d.tplTouched) return;
            const id = this.startTemplate(d.templates, d.folders, d.folderId, d.codexId);
            d.tplId = d.templates.some((t) => t.id === id) ? id : 'builtin:blank';
        },
        pickTpl(id) {
            this.dlg.tplId = id;
            this.dlg.tplTouched = true;
        },
        // The default of the New page dialog's folder, as a template of the list ({} when none).
        get npDefault() {
            const d = this.dlg;
            if (!d.folderId) return {};
            const id = codexFolderDefault(d.folders, d.folderId);
            return (d.templates || []).find((t) => t.id === id) || {};
        },
        get npDefaultHint() {
            const d = this.dlg;
            const f = (d.folders || []).find((x) => x.id === d.folderId);
            if (!f || !this.npDefault.id) return '';
            return `New pages in ${f.name} start from ${this.npDefault.name}. Change it in the folder's menu.`;
        },
        async openFolderTemplate(id) {
            const f = this.folderById(id);
            if (!f) return;
            try {
                const templates = await this.loadTemplates(this.codex.codex.id);
                const tpl = templates.some((t) => t.id === f.default_template) ? f.default_template : '';
                this.dlg = { type: 'folderTpl', folder: f, templates, tpl, error: '', busy: false };
                this.focusDlg();
            } catch (error) {
                this.showMessage('Could not load the templates: ' + error.message, 'error');
            }
        },

        // ---- Templates ----
        // The built-in templates (kept after the first load) then the codex's own, as one list.
        async loadTemplates(codexId) {
            if (!this.tplBuiltin) {
                const { response, data } = await this.send('/api/codices/templates/builtin', 'GET');
                if (!response.ok) throw new Error(data.error || 'request failed');
                this.tplBuiltin = data;
            }
            const { response, data } = await this.send(`/api/codices/c/${encodeURIComponent(codexId)}/templates`, 'GET');
            if (!response.ok) throw new Error(data.error || 'request failed');
            return [
                ...this.tplBuiltin.templates.map((t) => ({ id: t.id, name: t.name, description: t.description, tags: t.tags, body: t.body, by: '' })),
                ...data.map((t) => ({ id: t.id, name: t.name, description: t.description, tags: t.tags, body: t.body, by: t.created_by_user.display_name }))
            ];
        },
        tplGroups(list) {
            return [
                { label: 'Built-in', items: list.filter((t) => !t.by) },
                { label: 'This codex', items: list.filter((t) => t.by) }
            ].filter((g) => g.items.length);
        },
        // {{title}} and {{date}}, the same two the server fills in.
        fillTpl(body, title) {
            const today = this.tplBuiltin ? this.tplBuiltin.today : '';
            return body.replace(/\{\{title\}\}|\{\{date\}\}/g, (m) => (m === '{{title}}' ? title : today));
        },
        get tplSelected() {
            const d = this.dlg;
            return (d.templates || []).find((t) => t.id === d.tplId) || (d.templates || [])[0] || { name: '', tags: [], body: '' };
        },
        get tplPreviewHtml() {
            const t = this.tplSelected;
            if (!t.body) return '<p class="cx-tpl-empty">An empty page.</p>';
            // An empty [[ ]] shows as a dotted "…" so it reads as a blank to fill in.
            const text = this.fillTpl(t.body, this.dlg.title.trim() || 'Untitled').replace(/\[\[\s*\]\]/g, '[[…]]');
            return codexMarkdown(text, null, false, null, null, false, this.users);
        },
        // Arrow keys move through the template cards like a radio group.
        onTplKey(event) {
            const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[event.key];
            const list = this.dlg.templates;
            if (!step || !list.length) return;
            event.preventDefault();
            const i = list.findIndex((t) => t.id === this.dlg.tplId);
            this.pickTpl(list[(i + step + list.length) % list.length].id);
            this.$nextTick(() => document.querySelector('.cx-tpl-card[aria-checked="true"]')?.focus());
        },
        async openTemplates() {
            const codexId = this.currentCodexId;
            if (!codexId) return;
            try {
                const templates = await this.loadTemplates(codexId);
                this.dlg = { type: 'templates', codexId, templates, canEdit: this.canEdit, error: '', busy: false };
                this.focusTemplatesIntro();
            } catch (error) {
                this.showMessage('Could not load the templates: ' + error.message, 'error');
            }
        },
        // t: the template to edit, or null for a new one; from: a template whose fields start a new one.
        openTemplateEditor(t, from = null) {
            const s = t || from;
            const f = { name: s ? s.name : '', description: s ? s.description : '', tags: s ? s.tags.join(', ') : '', body: s ? s.body : '## \n\n' };
            this.dlg = {
                type: 'template', template: t, codexId: this.dlg.codexId, back: true, ...f,
                snap: JSON.stringify([f.name, f.description, f.tags, f.body]), error: '', busy: false
            };
        },
        // The Templates list keeps focus inside the dialog: on its intro line (tabindex -1).
        focusTemplatesIntro() {
            this.$nextTick(() => requestAnimationFrame(() => document.getElementById('dTplsIntro')?.focus()));
        },
        copyTemplate(t) {
            const name = Array.from(t.name + ' (copy)').length > 80 ? Array.from(t.name).slice(0, 72).join('') + ' (copy)' : t.name + ' (copy)';
            this.openTemplateEditor(null, { ...t, name });
        },
        async deleteTemplate(t) {
            const d = this.dlg;
            const ok = await this.askConfirm({
                eyebrow: 'Template',
                title: `Delete the “${t.name}” template?`,
                message: 'Pages already made from it stay as they are.',
                confirmLabel: 'Delete template'
            });
            if (!ok) return;
            try {
                const { response, data } = await this.send(`/api/codices/templates/${encodeURIComponent(t.id)}`, 'DELETE');
                if (!response.ok && response.status !== 404) {
                    this.showMessage(this.sentence(data.error || 'Could not delete the template'), 'error');
                    return;
                }
                if (this.dlg === d) {
                    d.templates = await this.loadTemplates(d.codexId);
                    this.focusTemplatesIntro();
                }
                this.showMessage('Template deleted', 'success');
            } catch (error) {
                this.showMessage('Could not delete the template: ' + error.message, 'error');
            }
        },
        async submitTemplate(d) {
            const name = d.name.trim();
            if (!name) {
                d.error = 'Give the template a name.';
                return;
            }
            const tags = d.tags.split(',').map((s) => s.trim().split(/\s+/).join(' ')).filter(Boolean);
            const body = { name, description: d.description, body: d.body, tags };
            const { response, data } = d.template
                ? await this.send(`/api/codices/templates/${encodeURIComponent(d.template.id)}`, 'PUT', body)
                : await this.send(`/api/codices/c/${encodeURIComponent(d.codexId)}/templates`, 'POST', body);
            if (!response.ok) {
                this.failDlg(d, response, data);
                return;
            }
            await this.openTemplates();
            this.showMessage('Template saved', 'success');
        },
        // More > Save as template: the page's headings (or all its text, without pictures) become a template of this codex.
        openSaveTemplate() {
            this.dlg = { type: 'saveTemplate', name: Array.from(this.page.title).slice(0, 80).join(''), description: '', keep: 'headings', tags: true, error: '', busy: false };
            this.focusDlg();
        },
        async submitSaveTemplate(d) {
            const name = d.name.trim();
            if (!name) {
                d.error = 'Give the template a name.';
                return;
            }
            const body = d.keep === 'all' ? codexWithoutPictures(this.page.body) : codexHeadingsOnly(this.page.body);
            if (d.keep === 'headings' && !body) {
                d.error = 'This page has no ## or ### headings to keep. Choose “All the text” instead.';
                return;
            }
            const tags = d.tags ? this.pageData.tags.map((t) => t.name) : [];
            const { response, data } = await this.send(`/api/codices/c/${encodeURIComponent(this.page.codex.id)}/templates`, 'POST', { name, description: d.description, body, tags });
            if (!response.ok) {
                this.failDlg(d, response, data);
                return;
            }
            this.closeDlg();
            this.showMessage(`Saved “${name}” as a template. It is in the New page list now.`, 'success');
        },
        // More > Copy page: into any codex the viewer can edit (the codex list is already loaded for the menu).
        async openCopyPage() {
            await this.ensureCodices();
            if (!this.codexChoices.length) {
                this.showMessage("There's no codex you can copy it to.", 'error');
                return;
            }
            const codexId = this.codexChoices.some((c) => c.id === this.page.codex.id) ? this.page.codex.id : this.codexChoices[0].id;
            const folders = this.codex && this.codex.codex.id === codexId ? this.codex.folders || [] : (await this.fetchFolders(codexId)) || [];
            const chars = Array.from(this.page.title);
            const title = chars.slice(0, 120 - ' (copy)'.length).join('') + ' (copy)';
            const folderId = codexId === this.page.codex.id && folders.some((f) => f.id === this.page.folder_id) ? this.page.folder_id : '';
            this.dlg = { type: 'copyPage', title: chars.length + ' (copy)'.length <= 120 ? this.page.title + ' (copy)' : title, codexId, folders, folderId, copyTags: true, copyImages: true, error: '', busy: false };
            this.focusDlg();
        },
        async submitCopyPage(d) {
            const title = d.title.trim().split(/\s+/).join(' ');
            if (!title) {
                d.error = 'Give the copy a title.';
                return;
            }
            const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/copy`, 'POST',
                { title, codex_id: d.codexId, folder_id: d.folderId || '', copy_tags: d.copyTags, copy_images: d.copyImages });
            if (response.status === 201) {
                this.saved = true;
                window.location.href = codexPageUrl({ id: data.page.id, title }) + '?copied=1';
                return;
            }
            this.failDlg(d, response, data);
        },
        // The codex's folders, or null (after an error message) when the load failed.
        async fetchFolders(codexId) {
            if (!codexId) return [];
            try {
                const { response, data } = await this.send(`/api/codices/c/${encodeURIComponent(codexId)}/folders`, 'GET');
                if (response.ok) return data.folders || [];
            } catch (error) {
                console.error('Failed to load folders:', error);
            }
            this.showMessage('Could not load the folders', 'error');
            return null;
        },
        async onDlgCodexChange() {
            const d = this.dlg;
            const codexId = d.codexId;
            d.folderId = '';
            d.folders = [];
            const folders = await this.fetchFolders(codexId);
            if (this.dlg === d && d.codexId === codexId) {
                d.folders = folders || [];
                if (!d.folders.some((f) => f.id === d.folderId)) d.folderId = '';
            }
        },
        openNewFolder(parentId) {
            this.dlg = { type: 'folder', folder: null, parentId, name: '', error: '', busy: false };
            this.focusDlg();
        },
        openRenameFolder(id) {
            const f = this.folderById(id);
            if (!f) return;
            this.dlg = { type: 'folder', folder: f, name: f.name, error: '', busy: false };
            this.focusDlg();
        },
        openMoveFolder(id) {
            const f = this.folderById(id);
            if (!f) return;
            this.dlg = { type: 'folderMove', folder: f, folders: this.codex.folders, excludeId: id, folderId: f.parent_id || '', error: '', busy: false };
            this.focusDlg();
        },
        async openPageFolder() {
            const folders = this.codex && this.codex.codex.id === this.page.codex.id ? this.codex.folders || [] : await this.fetchFolders(this.page.codex.id);
            if (!folders) return;
            this.dlg = { type: 'pageFolder', folders, excludeId: '', folderId: this.page.folder_id || '', error: '', busy: false };
            this.focusDlg();
        },
        async deleteFolder(id) {
            const f = this.folderById(id);
            if (!f) return;
            const pages = this.codex.pages.filter((p) => p.folder_id === id).length;
            const subs = this.codex.folders.filter((x) => x.parent_id === id).length;
            const parent = this.folderById(f.parent_id);
            const moves = pages + subs
                ? `Its ${this.countLabel(pages, 'page')} and ${this.countLabel(subs, 'folder')} will move up into ${parent ? '“' + parent.name + '”' : 'the top level'}.`
                : 'It is empty.';
            const ok = await this.askConfirm({
                eyebrow: 'Folder',
                title: `Delete the folder “${f.name}”?`,
                message: `${moves} Nothing else is deleted.`,
                confirmLabel: 'Delete folder'
            });
            if (!ok) return;
            try {
                const { response, data } = await this.send(`/api/codices/folders/${encodeURIComponent(id)}`, 'DELETE');
                if (response.status === 401) {
                    this.showMessage('Your login has expired. Log in again, then try once more.', 'error');
                    return;
                }
                if (data.code === 'folder_name_taken') {
                    this.showMessage('A folder inside it has the same name as one next to it. Rename one of them first.', 'error');
                    return;
                }
                if (!response.ok && response.status !== 404) {
                    this.showMessage(this.sentence(data.error || 'Could not delete the folder'), 'error');
                    return;
                }
                await this.load();
                this.focusFolder(f.parent_id);
                this.showMessage(response.ok ? 'Folder deleted' : 'That folder was already gone', response.ok ? 'success' : 'info');
            } catch (error) {
                this.showMessage('Could not delete the folder: ' + error.message, 'error');
            }
        },
        openCreateCodex() {
            this.dlg = { type: 'codex', codex: null, name: '', description: '', error: '', busy: false };
            this.focusDlg();
        },
        openEditCodex() {
            const w = this.codex.codex;
            this.dlg = { type: 'codex', codex: w, name: w.name, description: w.description, error: '', busy: false };
            this.focusDlg();
        },
        openRename() {
            this.dlg = { type: 'rename', title: this.page.title, error: '', busy: false };
            this.focusDlg();
        },
        openAliases() {
            this.dlg = { type: 'aliases', text: this.page.aliases.join('\n'), error: '', busy: false };
            this.focusDlg();
        },
        async openMove() {
            await this.ensureCodices();
            if (!this.codexChoices.some((w) => w.id !== this.page.codex.id)) {
                this.showMessage("There's no other codex you can move it to.", 'error');
                return;
            }
            this.dlg = { type: 'move', codexId: this.codexChoices.find((w) => w.id !== this.page.codex.id)?.id || '', error: '', busy: false };
            this.focusDlg();
        },
        conflictText(data) {
            const c = (data.conflicts || [])[0];
            return c ? `“${c.name}” is already used in this codex${c.title && c.title !== c.name ? ' (by ' + c.title + ')' : ''}. Titles and aliases are unique within a codex.` : this.sentence(data.error);
        },
        async submitDlg() {
            const d = this.dlg;
            if (d.busy) return;
            d.error = '';
            d.busy = true;
            try {
                if (d.type === 'createPage') await this.submitCreatePage(d);
                else if (d.type === 'quickNote') await this.submitQuickNote(d);
                else if (d.type === 'addToPage') await this.submitAddToPage(d);
                else if (d.type === 'createMissing') await this.submitCreateMissing(d);
                else if (d.type === 'event') await this.submitEvent(d);
                else if (d.type === 'calendar') await this.submitCalendar(d);
                else if (d.type === 'template') await this.submitTemplate(d);
                else if (d.type === 'saveTemplate') await this.submitSaveTemplate(d);
                else if (d.type === 'copyPage') await this.submitCopyPage(d);
                else if (d.type === 'codex') await this.submitCodex(d);
                else if (d.type === 'rename') await this.submitRename(d);
                else if (d.type === 'aliases') await this.submitAliases(d);
                else if (d.type === 'move') await this.submitMove(d);
                else if (d.type === 'folder') await this.submitFolder(d);
                else if (d.type === 'folderMove') await this.submitFolderMove(d);
                else if (d.type === 'folderTpl') await this.submitFolderTemplate(d);
                else if (d.type === 'pageFolder') await this.submitPageFolder(d);
                else if (d.type === 'tags') await this.submitTags(d);
                else if (d.type === 'tag') await this.submitTag(d);
            } catch (error) {
                d.error = 'Something went wrong: ' + error.message;
            } finally {
                d.busy = false;
            }
        },
        // Shared error text for a failed dialog request; true when handled.
        failDlg(d, response, data) {
            if (response.status === 401) d.error = 'Your login has expired. Log in again, then try once more.';
            else if (data.code === 'inbox_full') d.error = 'Your inbox is full (200 notes). File or delete some first.';
            else if (data.code === 'note_not_found') d.error = 'That note is gone: it was filed or deleted somewhere else.';
            else if (data.code === 'page_deleted') d.error = 'That page was deleted. Choose another.';
            else if (data.code === 'event_deleted') d.error = 'Someone deleted that event while you were editing it.';
            else if (data.code === 'not_a_timeline') d.error = 'This page is not a timeline.';
            else if (data.code === 'page_not_in_codex') d.error = 'That page is no longer in this codex. Pick another, or leave the event unlinked.';
            else if (response.status === 404) d.error = this.sentence(data.error || 'That no longer exists');
            else if (data.code === 'name_taken') d.error = this.conflictText(data);
            else if (data.code === 'folder_name_taken') d.error = 'A folder with that name already exists there.';
            else if (data.code === 'template_name_taken') d.error = 'A template with that name already exists in this codex.';
            else d.error = this.sentence(data.error || 'Request failed');
        },
        async submitCreatePage(d) {
            const title = d.title.trim();
            if (!title) {
                d.error = 'Give the page a title.';
                return;
            }
            if (this.isDirty) {
                d.error = 'Save or cancel your changes first.';
                return;
            }
            const timeline = d.kind === 'timeline';
            const { response, data } = await this.send('/api/codices/pages', 'POST', timeline
                ? { title, codex_id: d.codexId, folder_id: d.folderId || '', kind: 'timeline' }
                : { title, codex_id: d.codexId, folder_id: d.folderId || '', kind: 'lore', template_id: d.tplId, note_id: d.note ? d.note.id : '' });
            if (response.status === 201) {
                if (!timeline) {
                    try {
                        localStorage.setItem('codex-last-template:' + d.codexId, d.tplId);
                    } catch (error) {
                        // storage may be blocked; the template just isn't remembered
                    }
                }
                this.saved = true;
                window.location.href = codexPageUrl({ id: data.page.id, title }, timeline ? '' : '/edit');
                return;
            }
            this.failDlg(d, response, data);
        },
        // After a save that added links to missing pages: one row each (folder and template default to
        // the saved page's folder and its default). The page is already saved, so Just save only leaves.
        async openCreateMissing(titles, after) {
            const codexId = this.page.codex.id;
            try {
                const templates = await this.loadTemplates(codexId);
                const folders = this.codex && this.codex.codex.id === codexId ? this.codex.folders || [] : await this.fetchFolders(codexId);
                if (!folders) return false;
                const folderId = folders.some((f) => f.id === this.page.folder_id) ? this.page.folder_id : '';
                const tplId = this.cmTemplate(templates, folders, folderId);
                this.dlg = {
                    type: 'createMissing', codexId, folders, templates, after, created: 0, error: '', busy: false,
                    rows: titles.map((title) => ({ title, on: true, folderId, tplId, tplTouched: false, done: false, error: '' }))
                };
                return true;
            } catch (error) {
                return false;
            }
        },
        cmTemplate(templates, folders, folderId) {
            const def = codexFolderDefault(folders, folderId);
            return def && templates.some((t) => t.id === def) ? def : 'builtin:blank';
        },
        onCmFolder(r) {
            if (!r.tplTouched) r.tplId = this.cmTemplate(this.dlg.templates, this.dlg.folders, r.folderId);
        },
        finishSave(d) {
            window.location.href = d.after + (d.created ? (d.after.includes('?') ? '&' : '?') + 'created=' + d.created : '');
        },
        async submitCreateMissing(d) {
            let failed = 0;
            for (const r of d.rows) {
                if (!r.on || r.done) continue;
                r.error = '';
                try {
                    const { response, data } = await this.send('/api/codices/pages', 'POST', { title: r.title, codex_id: d.codexId, folder_id: r.folderId || '', kind: 'lore', template_id: r.tplId });
                    if (response.status === 201) {
                        r.done = true;
                        d.created++;
                    } else {
                        const tmp = {};
                        this.failDlg(tmp, response, data);
                        r.error = tmp.error;
                        failed++;
                    }
                } catch (error) {
                    r.error = 'Something went wrong: ' + error.message;
                    failed++;
                }
            }
            if (!failed) this.finishSave(d);
        },
        async submitFolder(d) {
            const name = d.name.trim();
            if (!name) {
                d.error = 'Give the folder a name.';
                return;
            }
            if (d.folder && name === d.folder.name) {
                this.closeDlg();
                return;
            }
            const { response, data } = d.folder
                ? await this.send(`/api/codices/folders/${encodeURIComponent(d.folder.id)}`, 'PUT', { name })
                : await this.send(`/api/codices/c/${encodeURIComponent(this.codex.codex.id)}/folders`, 'POST', { name, parent_id: d.parentId });
            if (!response.ok) {
                this.failDlg(d, response, data);
                return;
            }
            this.keepFolderOpen(d.parentId);
            this.closeDlg();
            await this.load();
            this.focusFolder(d.folder ? d.folder.id : data.folder.id);
            this.showMessage(d.folder ? 'Folder renamed' : 'Folder created', 'success');
        },
        async submitFolderTemplate(d) {
            const { response, data } = await this.send(`/api/codices/folders/${encodeURIComponent(d.folder.id)}`, 'PUT', { default_template: d.tpl });
            if (!response.ok) {
                this.failDlg(d, response, data);
                return;
            }
            this.closeDlg();
            await this.load();
            this.focusFolder(d.folder.id);
            this.showMessage('Folder saved', 'success');
        },
        async submitFolderMove(d) {
            if ((d.folderId || '') === (d.folder.parent_id || '')) {
                this.closeDlg();
                return;
            }
            const { response, data } = await this.send(`/api/codices/folders/${encodeURIComponent(d.folder.id)}`, 'PUT', { parent_id: d.folderId || '' });
            if (!response.ok) {
                this.failDlg(d, response, data);
                return;
            }
            this.keepFolderOpen(d.folderId);
            this.closeDlg();
            await this.load();
            this.focusFolder(d.folder.id);
            this.showMessage('Folder moved', 'success');
        },
        async submitPageFolder(d) {
            const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/folder`, 'PUT', { folder_id: d.folderId || '' });
            if (!response.ok) {
                this.failDlg(d, response, data);
                return;
            }
            this.closeDlg();
            await this.load();
            this.showMessage('Folder changed', 'success');
        },
        async submitCodex(d) {
            const name = d.name.trim();
            if (!name) {
                d.error = 'Give the codex a name.';
                return;
            }
            const body = { name, description: d.description };
            const { response, data } = d.codex
                ? await this.send(`/api/codices/c/${encodeURIComponent(d.codex.id)}`, 'PUT', body)
                : await this.send('/api/codices', 'POST', body);
            if (response.ok && !d.codex) {
                window.location.href = '/codices/c/' + encodeURIComponent(data.codex.id);
            } else if (response.ok) {
                this.closeDlg();
                this.codexChoices = [];
                this.codicesLoaded = false;
                this.allCodices = null;
                await this.load();
                this.showMessage('Codex saved', 'success');
            } else if (data.code === 'codex_name_taken') {
                d.error = 'A codex with that name already exists.';
            } else {
                this.failDlg(d, response, data);
            }
        },
        async submitRename(d) {
            const title = d.title.trim();
            if (!title) {
                d.error = 'Give the page a title.';
                return;
            }
            if (title === this.page.title) {
                this.closeDlg();
                return;
            }
            const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/title`, 'PUT', { title });
            if (response.ok) {
                window.location.href = codexPageUrl({ id: this.page.id, title: data.title }) + `?renamed=${data.rewritten}`;
                return;
            }
            this.failDlg(d, response, data);
        },
        async submitAliases(d) {
            const aliases = d.text.split('\n').map((a) => a.trim()).filter(Boolean);
            const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/aliases`, 'PUT', { aliases });
            if (response.ok) {
                this.closeDlg();
                await this.load();
                this.showMessage('Aliases saved', 'success');
                return;
            }
            this.failDlg(d, response, data);
        },
        async submitMove(d) {
            const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/codex`, 'PUT', { codex_id: d.codexId });
            if (response.ok) {
                this.writeTabs(this.page.codex.id, this.tabs.filter((t) => t.id !== this.page.id));
                window.location.href = codexPageUrl(this.page) + '?moved=1';
                return;
            }
            this.failDlg(d, response, data);
        },

        // ---- Tags ----
        tagUrl(name) {
            return `/codices/c/${encodeURIComponent(this.currentCodexId)}?tag=${encodeURIComponent(name)}`;
        },
        // The tag named by ?tag= on the codex screen, if this codex has it.
        get filterTag() {
            const q = codexNorm(this.tagFilter);
            return q && this.codex ? this.codex.tags.find((t) => codexNorm(t.name) === q) || null : null;
        },
        get taggedPages() {
            const t = this.filterTag;
            if (!t) return [];
            return this.codex.pages.filter((p) => p.tag_ids.includes(t.id)).sort((a, b) => (codexNorm(a.title) < codexNorm(b.title) ? -1 : 1));
        },
        // Tags of the codex not yet picked in the page's Tags dialog (offered as suggestions).
        get tagSuggestions() {
            const d = this.dlg;
            if (d.type !== 'tags' || !this.codex) return [];
            const have = new Set(d.names.map(codexNorm));
            return this.codex.tags.filter((t) => !have.has(codexNorm(t.name)));
        },
        async openTags() {
            await this.navPromise; // the codex's tags come with the codex payload
            if (!this.codex) {
                this.showMessage('Could not load the tags of this codex. Try again.', 'error');
                return;
            }
            this.dlg = { type: 'tags', names: this.pageData.tags.map((t) => t.name), input: '', error: '', busy: false };
            this.focusDlg();
        },
        // Adds what is typed in the Tags dialog (commas separate several); false when it was refused.
        addTagInput(d) {
            for (const raw of d.input.split(',')) {
                const name = raw.trim().split(/\s+/).join(' ');
                if (!name) continue;
                if (Array.from(name).length > 40) {
                    d.error = 'A tag name can be at most 40 characters.';
                    return false;
                }
                if (!d.names.some((n) => codexNorm(n) === codexNorm(name))) d.names.push(name);
            }
            d.input = '';
            d.error = '';
            return true;
        },
        removeTagName(d, name) {
            d.names = d.names.filter((n) => n !== name);
            this.$refs.dlgFocus?.focus();
        },
        async submitTags(d) {
            if (!this.addTagInput(d)) return;
            const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/tags`, 'PUT', { names: d.names });
            if (!response.ok) {
                this.failDlg(d, response, data);
                return;
            }
            this.closeDlg();
            await this.load();
            this.showMessage('Tags saved', 'success');
        },
        openNewTag() {
            this.dlg = { type: 'tag', tag: null, name: '', error: '', busy: false };
            this.focusDlg();
        },
        openRenameTag(t) {
            this.dlg = { type: 'tag', tag: t, name: t.name, error: '', busy: false };
            this.focusDlg();
        },
        async submitTag(d) {
            const name = d.name.trim().split(/\s+/).join(' ');
            if (!name || Array.from(name).length > 40) {
                d.error = 'Give the tag a name of 1 to 40 characters.';
                return;
            }
            if (d.tag && name === d.tag.name) {
                this.closeDlg();
                return;
            }
            const { response, data } = d.tag
                ? await this.send(`/api/codices/tags/${encodeURIComponent(d.tag.id)}`, 'PUT', { name })
                : await this.send(`/api/codices/c/${encodeURIComponent(this.codex.codex.id)}/tags`, 'POST', { name });
            if (!response.ok) {
                if (data.code === 'tag_name_taken') d.error = 'A tag with that name already exists in this codex.';
                else this.failDlg(d, response, data);
                return;
            }
            if (d.tag && codexNorm(this.tagFilter) === codexNorm(d.tag.name)) {
                this.tagFilter = name;
                history.replaceState(null, '', location.pathname + '?tag=' + encodeURIComponent(name) + location.hash);
            }
            this.closeDlg();
            await this.load();
            this.showMessage(d.tag ? 'Tag renamed' : 'Tag created', 'success');
        },
        async deleteTag(t) {
            const ok = await this.askConfirm({
                eyebrow: 'Tag',
                title: `Delete the tag “${t.name}”?`,
                message: `${t.page_count ? `It will be removed from ${this.countLabel(t.page_count, 'page')}.` : 'No page has it.'} The pages themselves are not changed.`,
                confirmLabel: 'Delete tag'
            });
            if (!ok) return;
            try {
                const { response, data } = await this.send(`/api/codices/tags/${encodeURIComponent(t.id)}`, 'DELETE');
                if (response.status === 401) {
                    this.showMessage('Your login has expired. Log in again, then try once more.', 'error');
                    return;
                }
                if (!response.ok && response.status !== 404) {
                    this.showMessage(this.sentence(data.error || 'Could not delete the tag'), 'error');
                    return;
                }
                if (codexNorm(this.tagFilter) === codexNorm(t.name)) {
                    this.tagFilter = '';
                    history.replaceState(null, '', location.pathname + '#tags');
                }
                await this.load();
                this.showMessage(response.ok ? 'Tag deleted' : 'That tag was already gone', response.ok ? 'success' : 'info');
            } catch (error) {
                this.showMessage('Could not delete the tag: ' + error.message, 'error');
            }
        },

        // ---- Pictures ----
        // Any button that carries a picture (data-url) opens the lightbox: article figures, the
        // editor preview, history, the cover.
        onImageClick(event) {
            if (event.target.closest && event.target.closest('.cx-layout')) return this.onLayoutClick(event);
            const b = event.target.closest && event.target.closest('button.cx-img-btn[data-url]');
            if (!b) return;
            this.openLightbox({ url: b.dataset.url, caption: b.dataset.caption, meta: b.dataset.meta });
        },
        // The layout bar of a lone figure on the page view: Layout toggles it on touch, a size or spot
        // button rewrites that embed and saves the page as a new revision.
        async onLayoutClick(event) {
            const btn = event.target.closest('button');
            if (!btn || btn.disabled) return;
            const box = btn.closest('.cx-layout');
            if (btn.hasAttribute('data-pic-toggle')) {
                box.classList.toggle('is-open');
                return;
            }
            const fig = btn.closest('figure[data-pic]');
            if (!fig || !this.page || this.picBusy) return;
            const size = btn.dataset.picSize || fig.dataset.size;
            const align = btn.dataset.picAlign || fig.dataset.align;
            const body = codexSetPicLayout(this.page.body, Number(fig.dataset.pic), size, align);
            if (body === null) return;
            this.picBusy = true;
            box.querySelectorAll('button').forEach((b) => { b.disabled = true; });
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}`, 'PUT', { base_revision: this.page.current_revision, body, note: 'Picture layout' });
                if (response.status === 409 && data.code === 'stale_base') this.showMessage('The page changed meanwhile. Reloaded it; try again.', 'error');
                else if (!response.ok) this.showMessage(this.sentence(data.error || 'Could not change the picture layout'), 'error');
                await this.load();
            } catch (error) {
                this.showMessage('Could not change the picture layout: ' + error.message, 'error');
                box.querySelectorAll('button').forEach((b) => { b.disabled = false; });
            } finally {
                this.picBusy = false;
            }
        },
        openLightbox(pic) {
            this.lightbox = pic;
            this.$refs.lightbox.showModal();
        },
        onLightboxClick(event) {
            if (event.target === this.$refs.lightbox) this.$refs.lightbox.close();
        },
        coverPic() {
            const c = this.pageData.cover;
            return { url: c.url, caption: c.name, meta: `${c.name} · ${c.width}×${c.height}` };
        },
        thumbPic(img) {
            return { url: img.url, caption: img.name, meta: `${img.name} · ${img.width}×${img.height}` };
        },
        // Uploads, one after another, and puts each into the text where the cursor is.
        async uploadPictures(files) {
            this.uploadJobs++;
            try {
                await this.uploadBatch(files);
            } finally {
                if (--this.uploadJobs === 0) this.uploading = '';
            }
        },
        async uploadBatch(files) {
            const list = Array.from(files).filter((f) => f.type.startsWith('image/') || /\.(png|jpe?g|webp|gif)$/i.test(f.name));
            if (!list.length) {
                this.showMessage('Choose PNG, JPEG, WebP or GIF images.', 'error');
                return;
            }
            for (const [i, file] of list.entries()) {
                if (file.size > 25 << 20) {
                    this.showMessage(`${file.name} is larger than 25 MB.`, 'error');
                    continue;
                }
                this.uploading = `Uploading ${file.name}${list.length > 1 ? ` (${i + 1} of ${list.length})` : ''}...`;
                try {
                    const response = await fetch(`/api/codices/pages/${encodeURIComponent(this.page.id)}/pictures`, {
                        method: 'POST',
                        signal: AbortSignal.timeout(180000), // a hung upload mustn't keep Save disabled
                        headers: { 'Content-Type': file.type || 'application/octet-stream', 'X-File-Name': encodeURIComponent(file.name || 'image') },
                        body: file
                    });
                    if (response.status === 401 || (response.redirected && new URL(response.url).pathname.includes('login'))) {
                        this.showMessage('Your login has expired. Log in again in another tab, then add the image.', 'error');
                        break;
                    }
                    const data = await response.json().catch(() => ({}));
                    if (!response.ok) {
                        this.showMessage(`${file.name}: ${this.sentence(data.error || 'upload failed')}`, 'error');
                        continue;
                    }
                    const img = data.image;
                    this.pageData.images = [...this.pageData.images, img];
                    this.pageData.image_index = { ...this.pageData.image_index, [img.name]: { name: img.name, url: img.url, width: img.width, height: img.height } };
                    this.insertPicture(img);
                } catch (error) {
                    this.showMessage(`Could not upload ${file.name}: ${error.message}`, 'error');
                }
            }
        },
        // ![[name|]] on a paragraph of its own (the renderer makes a figure of a picture-only paragraph),
        // with the cursor after it.
        insertPicture(img) {
            const text = `![[${img.name}|]]`;
            const v = this.editorReady ? this.editor.view : null;
            const ta = this.$refs.bodyArea;
            const doc = v ? v.state.doc.toString() : this.draft.body;
            const from = v ? v.state.selection.main.from : ta.selectionStart || 0;
            const to = v ? v.state.selection.main.to : ta.selectionEnd || 0;
            const before = doc.slice(0, from), after = doc.slice(to);
            const pre = !before ? '' : before.endsWith('\n\n') ? '' : before.endsWith('\n') ? '\n' : '\n\n';
            const post = !after ? '\n' : after.startsWith('\n\n') ? '' : after.startsWith('\n') ? '\n' : '\n\n';
            const insert = pre + text + post;
            if (v) {
                v.dispatch({ changes: { from, to, insert }, selection: { anchor: from + insert.length }, scrollIntoView: true });
                v.focus();
            } else {
                this.draft.body = before + insert + after;
                this.$nextTick(() => {
                    ta.focus();
                    ta.setSelectionRange(from + insert.length, from + insert.length);
                });
            }
        },
        onPickFiles(event) {
            this.uploadPictures(event.target.files);
            event.target.value = '';
        },
        hasFiles(event) {
            return Array.from(event.dataTransfer?.types || []).includes('Files');
        },
        onDragOver(event) {
            if (!this.hasFiles(event)) return;
            event.preventDefault();
            this.dragOver = true;
        },
        // Capture phase: CodeMirror would otherwise read a dropped file as text.
        onDrop(event) {
            this.dragOver = false;
            if (!this.hasFiles(event)) return;
            event.preventDefault();
            event.stopPropagation();
            this.uploadPictures(event.dataTransfer.files);
        },
        onPaste(event) {
            // Text copied from a spreadsheet or document comes with a picture of it: keep the text.
            if (event.clipboardData?.getData('text/plain')) return;
            const files = Array.from(event.clipboardData?.files || []).filter((f) => f.type.startsWith('image/'));
            if (!files.length) return;
            event.preventDefault();
            event.stopPropagation();
            this.uploadPictures(files);
        },
        async setCover(img) {
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/cover`, 'PUT', { image_id: img ? img.id : '' });
                if (!response.ok) {
                    this.showMessage(response.status === 401 ? 'Your login has expired. Log in again in another tab, then try again.' : this.sentence(data.error || 'Could not set the cover'), 'error');
                    return;
                }
                this.pageData.cover = img ? { id: img.id, name: img.name, url: img.url, width: img.width, height: img.height } : null;
                this.showMessage(img ? 'Cover set' : 'Cover removed', 'success');
            } catch (error) {
                this.showMessage('Could not set the cover: ' + error.message, 'error');
            }
        },
        async removePicture(img) {
            const ok = await this.askConfirm({
                eyebrow: 'Image',
                title: `Remove “${img.name}”?`,
                message: 'It leaves this list and can no longer be the cover. Older revisions that show it keep showing it, and so does any ![[…]] still in the text.',
                confirmLabel: 'Remove image'
            });
            if (!ok) return;
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/pictures/${encodeURIComponent(img.id)}`, 'DELETE');
                if (!response.ok && response.status !== 404) {
                    this.showMessage(response.status === 401 ? 'Your login has expired. Log in again in another tab, then try again.' : this.sentence(data.error || 'Could not remove the image'), 'error');
                    return;
                }
                this.pageData.images = this.pageData.images.filter((x) => x.id !== img.id);
                if (this.pageData.cover && this.pageData.cover.id === img.id) this.pageData.cover = null;
                this.showMessage('Image removed', 'success');
            } catch (error) {
                this.showMessage('Could not remove the image: ' + error.message, 'error');
            }
        },

        // ---- Attachments (the box on the page) ----
        // Sends one file (POST to add, PUT to replace) and returns the attachment, or null after
        // reporting why not. stop.now is set when the rest of a batch is pointless.
        async attachSend(method, url, file, stop) {
            this.attach.pct = 0;
            try {
                const { status, data } = await codexUpload(method, url, file, (p) => { this.attach.pct = p; });
                if (status === 401) {
                    this.showMessage('Your login has expired. Log in again in another tab, then try again.', 'error');
                    stop.now = true;
                    return null;
                }
                if (status >= 200 && status < 300 && data.attachment) return data.attachment;
                if (data.code === 'page_deleted') {
                    this.showMessage('This page was deleted.', 'error');
                    stop.now = true;
                    await this.load();
                } else if (data.code === 'attachment_not_found') {
                    this.pageData.attachments = this.pageData.attachments.filter((x) => x.id !== this.attach.replaceId);
                    this.showMessage('That file was already removed.', 'info');
                } else {
                    this.showMessage(`${file.name}: ${this.sentence(data.error || 'upload failed')}`, 'error');
                }
            } catch (error) {
                this.showMessage(`Could not upload ${file.name}: ${error.message}.`, 'error');
            }
            return null;
        },
        async onAttachFiles(event) {
            const files = Array.from(event.target.files);
            event.target.value = '';
            if (!files.length || this.attach.busy) return;
            this.attach.busy = true;
            const pageId = this.page.id;
            const stop = { now: false };
            try {
                for (const [i, file] of files.entries()) {
                    if (stop.now) break;
                    if (!CODEX_ATTACH_RE.test(file.name)) {
                        this.showMessage(`${file.name}: this type of file can't be attached.`, 'error');
                        continue;
                    }
                    if (file.size > CODEX_ATTACH_MAX) {
                        this.showMessage(`${file.name} is larger than 100 MB.`, 'error');
                        continue;
                    }
                    this.attach.text = `Uploading ${file.name}${files.length > 1 ? ` (${i + 1} of ${files.length})` : ''}`;
                    const a = await this.attachSend('POST', `/api/codices/pages/${encodeURIComponent(pageId)}/attachments`, file, stop);
                    if (a && this.page && this.page.id === pageId) this.pageData.attachments = [...this.pageData.attachments, a];
                }
            } finally {
                this.attach.busy = false;
                this.attach.text = '';
            }
        },
        startReplace(a) {
            this.attach.replaceId = a.id;
            this.$refs.attachReplaceInput.click();
        },
        async onReplaceFile(event) {
            const file = event.target.files[0];
            event.target.value = '';
            const cur = this.pageData.attachments.find((x) => x.id === this.attach.replaceId);
            if (!file || !cur || this.attach.busy) return;
            const ext = (n) => (String(n).match(/\.[^.]*$/) || [''])[0].toLowerCase();
            if (ext(file.name) !== ext(cur.name)) {
                this.showMessage(`The new file must be a ${ext(cur.name)} file.`, 'error');
                return;
            }
            if (file.size > CODEX_ATTACH_MAX) {
                this.showMessage(`${file.name} is larger than 100 MB.`, 'error');
                return;
            }
            this.attach.busy = true;
            this.attach.text = `Replacing ${cur.name}`;
            const pageId = this.page.id;
            try {
                const a = await this.attachSend('PUT', `/api/codices/pages/${encodeURIComponent(pageId)}/attachments/${encodeURIComponent(cur.id)}`, file, { now: false });
                if (a && this.page && this.page.id === pageId) {
                    this.pageData.attachments = this.pageData.attachments.map((x) => (x.id === a.id ? a : x));
                    this.showMessage(`Replaced ${a.name}`, 'success');
                }
            } finally {
                this.attach.busy = false;
                this.attach.text = '';
            }
        },
        async removeAttachment(a) {
            if (this.attach.busy) return;
            const ok = await this.askConfirm({
                eyebrow: 'Attachment',
                title: `Remove “${a.name}”?`,
                message: 'This deletes the file for good; it is not in the backups. Links to it will read “attachment removed”.',
                confirmLabel: 'Remove file',
                danger: true
            });
            if (!ok) return;
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/attachments/${encodeURIComponent(a.id)}`, 'DELETE');
                if (!response.ok && response.status !== 404) {
                    this.showMessage(response.status === 401 ? 'Your login has expired. Log in again in another tab, then try again.' : this.sentence(data.error || 'Could not remove the file'), 'error');
                    return;
                }
                this.pageData.attachments = this.pageData.attachments.filter((x) => x.id !== a.id);
                this.showMessage('File removed', 'success');
            } catch (error) {
                this.showMessage('Could not remove the file: ' + error.message, 'error');
            }
        },

        // ---- Delete ----
        async deletePage() {
            if (this.busy) return;
            const p = this.page;
            const ok = await this.askConfirm({
                eyebrow: 'Codex',
                title: `Delete “${p.title}”?`,
                message: `This deletes the page and all ${p.revision_count} of its revisions. It cannot be undone. Links to it become missing links.` +
                    (this.isTimeline ? ` Its ${this.countLabel(this.tlEvents.length, 'event')} go with it.` : '') +
                    ((this.pageData.on_timelines || []).length ? ' Events that point at it stay on their timelines as plain events.' : '') +
                    (this.pageData.attachments.length ? ` Its ${this.countLabel(this.pageData.attachments.length, 'attachment')} ${this.pageData.attachments.length === 1 ? 'is' : 'are'} deleted too.` : ''),
                confirmLabel: 'Delete page',
                danger: true
            });
            if (!ok) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(p.id)}`, 'DELETE');
                if (response.ok || response.status === 404) {
                    this.writeTabs(p.codex.id, this.tabs.filter((t) => t.id !== p.id));
                    window.location.href = '/codices/c/' + encodeURIComponent(p.codex.id) + '?page_deleted=1';
                    return;
                }
                this.showMessage(response.status === 401 ? 'Your login has expired. Log in again, then delete the page.' : this.sentence(data.error || 'Failed to delete the page'), 'error');
            } catch (error) {
                this.showMessage('Error deleting the page: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        },
        async deleteCodex() {
            if (this.busy) return;
            const w = this.codex.codex;
            const ok = await this.askConfirm({
                eyebrow: 'Codex',
                title: `Delete the codex “${w.name}”?`,
                message: 'A codex can only be deleted when it has no pages. Games set in it lose their codex label. This cannot be undone.',
                confirmLabel: 'Delete codex',
                danger: true
            });
            if (!ok) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(`/api/codices/c/${encodeURIComponent(w.id)}`, 'DELETE');
                if (response.ok) {
                    window.location.href = '/codices';
                } else if (data.code === 'codex_in_use') {
                    const parts = [];
                    if (data.page_count) parts.push(`it still has ${this.countLabel(data.page_count, 'page')}. Move or delete them first`);
                    this.showMessage(this.sentence("Can't delete " + w.name + ': ' + parts.join('; ')), 'error');
                } else {
                    this.showMessage(this.sentence(data.error || 'Failed to delete the codex'), 'error');
                }
            } catch (error) {
                this.showMessage('Error deleting the codex: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        },

        // ---- Editor ----
        get isDirty() {
            if (this.saved || !this.draft) return false;
            return this.draft.body !== this.draft.initial.body || this.draft.note !== '';
        },
        get conflictWho() {
            return this.conflict ? this.conflict.latest.author_user.display_name : '';
        },
        async initDraft() {
            const p = this.page;
            this.draft = { body: p.body, note: '', base: p.current_revision, tab: 'write', error: '', locked: false, initial: { body: p.body } };
            this.draftStart = p.body;
            this.draftStatus = '';
            const stored = codexDraftFind(p.id);
            this.draftOffer = stored && (stored.body !== p.body || stored.note !== '')
                ? { ...stored, time: new Date(stored.at).toLocaleString([], { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }) }
                : null;
            await this.navPromise;
            if (!window.createCodexEditor) return;
            await this.$nextTick();
            // The / menu's template headings; if they can't load, the snippets still work.
            let templates = [];
            try { templates = await this.loadTemplates(this.currentCodexId); } catch (error) { console.error('Templates for the / menu:', error); }
            const today = this.tplBuiltin ? this.tplBuiltin.today : '';
            try {
                this.editor = await window.createCodexEditor(this.$refs.editorHost, {
                    doc: () => this.draft.body,
                    titles: () => this.titles,
                    users: () => this.users || [],
                    onChange: (value) => { this.draft.body = value; this.queueDraft(); },
                    onNotice: (text) => this.showMessage(text, 'info'),
                    slash: () => codexSlashItems(templates, today)
                });
                this.editorReady = !!this.editor;
            } catch (error) {
                console.error('Editor failed to load, using the plain text box:', error);
            }
        },
        // Keeps the text on this device 800 ms after the last change (not while a stored draft awaits an answer).
        queueDraft() {
            clearTimeout(this.draftTimer);
            this.draftTimer = null;
            if (this.saved || !this.draft || this.draftOffer) return;
            this.draftTimer = setTimeout(() => this.writeDraft(), 800);
        },
        writeDraft() {
            clearTimeout(this.draftTimer);
            this.draftTimer = null;
            const d = this.draft;
            if (this.saved || !d || this.draftOffer) return;
            if (d.body === this.draftStart && d.note === '') {
                codexDraftClear(this.page.id);
                this.draftStatus = '';
                return;
            }
            const at = new Date();
            if (codexDraftWrite(this.page.id, d.base, { body: d.body, note: d.note, initial: d.initial.body, at: at.toISOString() })) {
                this.draftStatus = 'Draft kept on this device · ' + at.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
            }
        },
        clearDraft() {
            clearTimeout(this.draftTimer);
            this.draftTimer = null;
            codexDraftClear(this.page.id);
            this.draftStatus = '';
        },
        // A draft on an older base is restored onto that base, so Save gets 409 and the conflict panel shows the diff.
        restoreDraft() {
            const o = this.draftOffer;
            if (!o) return;
            const d = this.draft;
            d.body = o.body;
            d.note = o.note;
            if (o.base !== d.base) {
                d.base = o.base;
                d.initial.body = o.initial;
            }
            const v = this.editorReady ? this.editor.view : null;
            if (v) v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: o.body } });
            this.draftOffer = null;
            this.queueDraft();
        },
        discardDraftOffer() {
            this.draftOffer = null;
            this.clearDraft();
        },
        async cancelEdit() {
            if (this.isDirty) {
                const ok = await this.askConfirm({
                    eyebrow: 'Page', title: 'Discard your changes?', message: 'The page has changes that are not saved.', confirmLabel: 'Discard changes'
                });
                if (!ok) return;
            }
            this.clearDraft();
            this.saved = true;
            window.location.href = this.pageUrl(this.page);
        },
        async save() {
            if (this.busy || this.uploading || !this.draft || this.draft.locked) return;
            const d = this.draft;
            d.error = '';
            this.busy = true;
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}`, 'PUT', { base_revision: d.base, body: d.body, note: d.note.trim() });
                if (response.ok) {
                    this.saved = true;
                    this.clearDraft();
                    const after = this.pageUrl(this.page) + (response.status === 201 ? `?saved=${data.revision.number}` : '?unchanged=1');
                    const missing = response.status === 201 ? codexNewMissing(d.initial.body, d.body, this.titleLookup) : [];
                    if (missing.length && this.canEdit && await this.openCreateMissing(missing, after)) return;
                    window.location.href = after;
                } else if (response.status === 409 && data.code === 'stale_base') {
                    const diff = diffWords(d.initial.body, data.latest.body);
                    this.conflict = { latest: data.latest, html: diff.added || diff.removed ? diffHtml(diff.segs) : '' };
                    this.$nextTick(() => document.querySelector('.conflict')?.scrollIntoView({ block: 'start' }));
                } else if (response.status === 404) {
                    d.locked = true;
                    d.error = 'This page was deleted while you were editing. Your text is still here, so copy it somewhere safe.';
                } else if (response.status === 401) {
                    d.error = 'Your login has expired. Log in again in another tab, then save.';
                } else {
                    d.error = this.sentence(data.error || 'Failed to save the page');
                }
            } catch (error) {
                d.error = 'Error saving the page: ' + error.message;
            } finally {
                this.busy = false;
            }
        },
        async saveOnTop() {
            if (!this.conflict) return;
            this.draft.base = this.conflict.latest.number;
            this.draft.initial.body = this.conflict.latest.body;
            this.conflict = null;
            await this.save();
        },
        discardMine() {
            this.saved = true;
            this.clearDraft();
            window.location.href = this.pageUrl(this.page);
        },

        // ---- History ----
        get selectedRevision() {
            return this.revisions.find((r) => r.number === this.selected) || null;
        },
        get previousRevision() {
            return this.revisions.find((r) => r.number === this.selected - 1) || null;
        },
        async loadHistory(select) {
            this.historyLoading = true;
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.route.id)}/revisions`, 'GET');
                if (!response.ok) {
                    this.loadError = data.error || 'Failed to load the history';
                    return;
                }
                this.revisions = data.revisions;
                this.pruned = data.pruned;
                this.currentRevision = data.current_revision;
                this.selected = select && this.revisions.some((r) => r.number === select) ? select : this.currentRevision;
                this.updateDiff();
            } catch (error) {
                this.loadError = 'Could not load the history: ' + error.message;
            } finally {
                this.historyLoading = false;
            }
        },
        // The list has no bodies; a revision's text (kept revisions never change) is fetched when needed.
        async revisionBody(n) {
            if (this.bodies[n] === undefined) {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.route.id)}/revisions/${n}`, 'GET');
                if (!response.ok) throw new Error(data.error || 'request failed');
                this.bodies[n] = data.revision.body;
            }
            return this.bodies[n];
        },
        // Loads the selected revision and the one before it, then diffs them. The +/- count of a
        // revision shows in the list once it has been opened.
        async updateDiff() {
            const seq = ++this.diffSeq;
            const r = this.selectedRevision, prev = this.previousRevision;
            this.selectedBody = '';
            this.selectedDiff = null;
            if (!r) return;
            this.bodyLoading = true;
            try {
                const [body, before] = await Promise.all([this.revisionBody(r.number), prev ? this.revisionBody(prev.number) : null]);
                if (seq !== this.diffSeq) return;
                this.selectedBody = body;
                if (before !== null) {
                    const d = diffWords(before, body);
                    this.selectedDiff = { added: d.added, removed: d.removed, html: diffHtml(d.segs) };
                    this.deltas = { ...this.deltas, [r.number]: { added: d.added, removed: d.removed } };
                }
            } catch (error) {
                if (seq === this.diffSeq) this.showMessage('Could not load the revision: ' + error.message, 'error');
            } finally {
                if (seq === this.diffSeq) this.bodyLoading = false;
            }
        },
        deltaHtml(r) {
            const d = this.deltas[r.number];
            if (r.number === 1) return 'New page';
            if (!this.revisions.some((x) => x.number === r.number - 1)) return 'Oldest kept';
            return d ? `<span aria-label="${d.added} words added, ${d.removed} removed"><span class="plus">+${d.added}</span> <span class="minus">−${d.removed}</span></span>` : '';
        },
        revNote(r) {
            return r.restored_from ? `↺ Restored revision ${r.restored_from}` : r.note;
        },
        selectRevision(n) {
            this.selected = n;
            this.updateDiff();
            if (window.matchMedia('(max-width: 768px)').matches) {
                this.$nextTick(() => document.querySelector('.detail')?.scrollIntoView({ block: 'start' }));
            }
        },
        async restore() {
            const r = this.selectedRevision;
            if (this.busy || !r) return;
            const ok = await this.askConfirm({
                eyebrow: 'History',
                title: `Restore revision ${r.number}?`,
                message: `The text goes back to ${r.author_user.display_name}'s version from ${this.shortDate(r.created_at)}. This saves as revision ${this.currentRevision + 1}, and the title stays as it is now. The current version stays in the history, so you can undo this.`,
                confirmLabel: 'Restore',
                danger: false
            });
            if (!ok) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}/revisions/${r.number}/restore`, 'POST', { base_revision: this.currentRevision });
                if (response.ok && response.status === 201) {
                    await this.loadHistory(data.revision.number);
                    this.showMessage(`Restored revision ${r.number} as revision ${data.revision.number}`, 'success');
                } else if (response.ok) {
                    this.showMessage('That version is already the current one, so no revision was saved.', 'info');
                } else if (response.status === 409 && data.code === 'stale_base') {
                    await this.loadHistory();
                    this.showMessage('The page changed while you were looking at it. The history is up to date now. Choose the version again.', 'error');
                } else {
                    this.showMessage(response.status === 401 ? 'Your login has expired. Log in again, then restore the version.' : this.sentence(data.error || 'Failed to restore the version'), 'error');
                }
            } catch (error) {
                this.showMessage('Error restoring the version: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        },

        // askConfirm opens the confirm modal and resolves to true on confirm, false on cancel.
        askConfirm({ eyebrow, title, message, confirmLabel, danger }) {
            if (this.confirmDialog.resolve) this.confirmDialog.resolve(false);
            return new Promise((resolve) => {
                this.confirmDialog = { open: true, eyebrow, title, message, confirmLabel: confirmLabel || 'Confirm', danger: danger !== false, resolve };
                this.$nextTick(() => this.$refs.confirmCancelBtn?.focus());
            });
        },
        closeConfirm(result) {
            const resolve = this.confirmDialog.resolve;
            this.confirmDialog = { open: false, eyebrow: '', title: '', message: '', confirmLabel: '', danger: true, resolve: null };
            if (resolve) resolve(result);
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
        // action: an optional {label, run} button in the toast (Undo, Open inbox, ...).
        showMessage(text, type, action = null) {
            this.messageText = text;
            this.messageType = type;
            this.messageAction = action;
            this.messageVisible = true;
            clearTimeout(this.messageTimer);
            this.messageTimer = setTimeout(() => {
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
