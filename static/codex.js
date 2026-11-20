// Codices page: /codices (home), /codices/c/{id} (codex), /codices/p/{id}/{slug}[/edit|/history] (page).
// Relies on campaigns.js (escapeHtml, renderMarkdown, journalByline, userAvatarHtml), journal.js
// (diffWords, diffHtml) and codex-editor.js (createCodexEditor). One component picks the screen
// from the path. Top-level names here must not clash with those scripts' (const clashes throw).

// Mirrors wikiLinkRe in db/codex_links.go.
const CODEX_LINK_RE = /\[\[([^\[\]\n|]{1,200})(?:\|([^\[\]\n]{1,200}))?\]\]/g;

const codexNorm = (s) => String(s || '').trim().split(/\s+/).join(' ').toLowerCase();

function codexSlug(title) {
    const s = Array.from(String(title || '').toLowerCase().replace(/[^\p{L}\p{N}]+/gu, '-').replace(/^-+|-+$/g, '')).slice(0, 60).join('').replace(/-+$/, '');
    return s || 'page';
}

function codexPageUrl(p, tail = '') {
    return `/codices/p/${encodeURIComponent(p.id)}/${encodeURIComponent(codexSlug(p.title))}${tail}`;
}

// Markdown with [[wikilinks]]. lookup(text) returns the target page ({id, title}) or null for a
// missing page; with no lookup (old revisions) links are shown plain. The links are swapped for
// placeholders before rendering so the Markdown renderer never sees them. A placeholder holds
// spaces, which a Markdown link URL can't, so one inside "(...)" is never swallowed into an href.
function codexMarkdown(body, lookup) {
    const found = [];
    const text = String(body || '').replace(/\u0001/g, '').replace(CODEX_LINK_RE, (m, t, label) => {
        found.push({ text: t.trim(), label });
        return `\u0001 ${found.length - 1} \u0001`;
    });
    return renderMarkdown(text).replace(/\u0001 (\d+) \u0001/g, (m, i) => {
        const l = found[Number(i)];
        if (!l) return '';
        const shown = escapeHtml(l.label || l.text);
        if (!lookup) return `<span class="wikilink wikilink--plain">${shown}</span>`;
        const target = lookup(l.text);
        if (target) return `<a class="wikilink" href="${codexPageUrl(target)}">${shown}</a>`;
        return `<a class="wikilink wikilink--missing" href="#" data-text="${escapeHtml(l.text)}" title="No page yet. Click to create.">${shown}</a>`;
    });
}

// Folder tree. Folders only sort pages. Open/closed state is kept per codex in localStorage and
// is deliberately not reactive, so toggling a folder never re-renders the tree.
let codexOpenState = {};
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
    const out = [{ id: '', label: 'Top level', disabled: false }];
    const walk = (n, trail, blocked) => {
        const path = [...trail, n.name];
        const off = blocked || n.id === excludeId;
        out.push({ id: n.id, label: path.join(' › '), disabled: off });
        n.children.forEach((c) => walk(c, path, off));
    };
    roots.forEach((r) => walk(r, [], false));
    return out;
}

function codexPageItemHtml(p) {
    const also = p.aliases.length ? `<span class="muted"> · also ${escapeHtml(p.aliases.join(', '))}</span>` : '';
    return `<li><a href="${codexPageUrl(p)}">${escapeHtml(p.title)}</a>${also}</li>`;
}

function codexFolderHtml(n, depth) {
    const open = codexOpenState[n.id] === undefined ? depth === 0 : codexOpenState[n.id];
    const id = escapeHtml(n.id);
    const pages = n.pages.length ? `<ul class="page-list">${n.pages.map(codexPageItemHtml).join('')}</ul>` : '';
    return `<details class="codex-folder${depth >= 4 ? ' codex-folder--deep' : ''}" id="folder-${id}" data-id="${id}"${open ? ' open' : ''}>` +
        `<summary><span class="codex-folder-name">${escapeHtml(n.name)}</span><span class="muted"> · ${n.count} ${n.count === 1 ? 'page' : 'pages'}</span>` +
        `<button type="button" class="codex-menu-btn" data-menu="${id}" aria-label="Actions for folder ${escapeHtml(n.name)}" aria-haspopup="menu" aria-expanded="false">⋯</button></summary>` +
        `<div class="codex-folder-body">` +
        n.children.map((c) => codexFolderHtml(c, depth + 1)).join('') + pages + '</div></details>';
}

// The ⋯ menu's items; every label names the folder for screen readers.
function codexFolderMenuHtml(f) {
    const id = escapeHtml(f.id);
    const name = escapeHtml(f.name);
    const item = (act, label, aria, cls = '') => `<button type="button" role="menuitem" class="codex-menu-item${cls}" data-act="${act}" data-id="${id}" aria-label="${aria} ${name}">${label}</button>`;
    return item('newPage', '+ Page', '+ Page in') + item('newFolder', '+ Folder', '+ Folder in') + item('rename', 'Rename', 'Rename') +
        item('move', 'Move', 'Move') + item('delete', 'Delete', 'Delete', ' codex-menu-item--danger');
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
        confirmDialog: { open: false, eyebrow: '', title: '', message: '', confirmLabel: '', danger: true, resolve: null },
        home: null,
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
        titles: [], // pages of the current page's codex, for previews and the editor
        codexChoices: [],
        dlg: { type: '' },
        // Editor
        draft: null,
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
            this.route = this.parseRoute();
            if (!this.route) {
                this.route = { screen: 'home', id: '' };
                this.notFound = true;
                this.loading = false;
                return;
            }
            window.addEventListener('beforeunload', (event) => {
                if (this.isDirty) {
                    event.preventDefault();
                    event.returnValue = '';
                }
            });
            await this.load();
            this.showArrivalMessage();
        },

        // /codices, /codices/c/{id}[/slug], /codices/p/{id}/{slug}[/edit|/history]
        // Returns null for a malformed URL.
        parseRoute() {
            let parts;
            try {
                parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
            } catch (error) {
                return null;
            }
            if (parts[1] === 'c' && parts[2]) return { screen: 'codex', id: parts[2] };
            if (parts[1] === 'p' && parts[2]) {
                const tail = parts[4] || '';
                return { screen: tail === 'edit' || tail === 'history' ? tail : 'page', id: parts[2] };
            }
            return { screen: 'home', id: '' };
        },

        get screen() {
            return this.route.screen;
        },
        get page() {
            return this.pageData ? this.pageData.page : null;
        },
        get crumbs() {
            const s = this.screen;
            if (s === 'codex' && this.codex) return [{ label: this.codex.codex.name, href: '' }];
            if (!this.page) return [];
            const codexHref = '/codices/c/' + encodeURIComponent(this.page.codex.id) + '/' + encodeURIComponent(codexSlug(this.page.codex.name));
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

        async load() {
            const seq = ++this.loadSeq; // an older response must not overwrite a newer one
            try {
                const r = this.route;
                const url = r.screen === 'home' ? '/api/codices'
                    : r.screen === 'codex' ? `/api/codices/c/${encodeURIComponent(r.id)}`
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
                    document.title = 'Waystone - Codices';
                } else if (r.screen === 'codex') {
                    codexLoadOpenState(data.codex.id);
                    if (!this.folderRevealed) {
                        this.folderRevealed = true;
                        this.revealFolderFromHash(data.folders);
                    }
                    this.codex = data;
                    document.title = 'Waystone - ' + data.codex.name;
                    this.$nextTick(() => this.scrollToFolder());
                } else {
                    this.pageData = data;
                    document.title = 'Waystone - ' + data.page.title;
                    if (location.pathname !== this.pageUrl(data.page) + (r.screen === 'page' ? '' : '/' + r.screen)) {
                        history.replaceState(null, '', this.pageUrl(data.page) + (r.screen === 'page' ? '' : '/' + r.screen) + location.search);
                    }
                    if (r.screen === 'edit' && !this.draft) await this.initDraft();
                    if (r.screen === 'history') {
                        await this.loadHistory(Number(new URLSearchParams(location.search).get('r')) || 0);
                    }
                    if (r.screen === 'page') await this.loadTitles();
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
            return codexMarkdown(body, lookup);
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
            const pages = tree.pages.length
                ? `<p class="codex-tree-label muted">Not in a folder</p><ul class="page-list">${tree.pages.map(codexPageItemHtml).join('')}</ul>`
                : '';
            return tree.roots.map((r) => codexFolderHtml(r, 0)).join('') + pages;
        },
        // Marks a folder and the ones above it open.
        openFolderChain(id, folders) {
            const byID = new Map(folders.map((f) => [f.id, f]));
            for (let cur = id, hops = 0; byID.has(cur) && hops <= folders.length; hops++) {
                codexOpenState[cur] = true;
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
            this.openFolderChain(m[1], folders);
            this.folderTarget = m[1];
        },
        scrollToFolder() {
            if (!this.folderTarget) return;
            const el = document.getElementById('folder-' + this.folderTarget);
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
            if (act === 'newPage') this.openCreatePage('', id);
            else if (act === 'newFolder') this.openNewFolder(id);
            else if (act === 'rename') this.openRenameFolder(id);
            else if (act === 'move') this.openMoveFolder(id);
            else if (act === 'delete') this.deleteFolder(id);
        },
        folderOptions(folders, excludeId) {
            return codexFolderOptions(folders, excludeId);
        },
        folderById(id) {
            return this.codex.folders.find((f) => f.id === id);
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
                saved: () => (Number(q.get('saved')) === 1 ? 'Page saved' : `Saved as revision ${q.get('saved')}`),
                unchanged: () => 'Nothing changed, so no revision was saved.',
                renamed: () => `Renamed. Links were updated in ${this.countLabel(Number(q.get('renamed')), 'page')}.`,
                moved: () => 'Page moved.',
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
        get bodyHtml() {
            return this.page ? codexMarkdown(this.page.body, this.linkLookup) : '';
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
            this.toc = Array.from(body.querySelectorAll('h3, h4')).filter((h) => h.textContent.trim()).map((h) => {
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
            this.tocActive = active;
        },
        get previewHtml() {
            return this.draft ? codexMarkdown(this.draft.body, this.titleLookup) : '';
        },
        async loadTitles() {
            if (!this.page) return;
            try {
                const { response, data } = await this.send(`/api/codices/titles?codex=${encodeURIComponent(this.page.codex.id)}`, 'GET');
                if (response.ok) this.titles = data;
            } catch (error) {
                console.error('Failed to load titles:', error);
            }
        },
        onBodyClick(event) {
            const a = event.target.closest('a.wikilink--missing');
            if (!a) return;
            event.preventDefault();
            this.openCreatePage(a.dataset.text);
        },

        // ---- Dialogs ----
        get dlgEyebrow() {
            return { createPage: 'Codex', codex: 'Codex', rename: 'Page', aliases: 'Page', move: 'Page', folder: 'Folder', folderMove: 'Folder', pageFolder: 'Page' }[this.dlg.type] || '';
        },
        get dlgTitle() {
            const d = this.dlg;
            if (d.type === 'createPage') return 'New page';
            return { codex: d.codex ? 'Edit codex' : 'New codex', rename: 'Rename page', aliases: 'Aliases', move: 'Move to another codex', folder: d.folder ? 'Rename folder' : 'New folder', folderMove: 'Move folder', pageFolder: 'Change folder' }[d.type] || '';
        },
        get dlgSubmit() {
            const d = this.dlg;
            if (d.type === 'createPage') return d.busy ? 'Creating...' : 'Create';
            return d.busy ? 'Saving...' : d.type === 'move' ? 'Move page' : d.type === 'folderMove' ? 'Move folder' : (d.type === 'codex' && !d.codex) || (d.type === 'folder' && !d.folder) ? 'Create' + (d.type === 'codex' ? ' codex' : ' folder') : 'Save';
        },
        get moveWarning() {
            const n = this.pageData?.incoming_count || 0;
            const here = this.page?.codex.name;
            return (n > 0
                ? `${this.countLabel(n, 'link')} from ${here} will turn into missing links, because links only reach pages in their own codex. `
                : '') + "This page's own links are checked again in the new codex, and it goes to the top level of its folders.";
        },
        async ensureCodices() {
            if (this.codexChoices.length) return;
            try {
                const { response, data } = await this.send('/api/codices', 'GET');
                if (response.ok) this.codexChoices = data.codices.map((w) => ({ id: w.id, name: w.name }));
            } catch (error) {
                this.showMessage('Could not load the codices: ' + error.message, 'error');
            }
        },
        focusDlg() {
            this.$nextTick(() => this.$refs.dlgFocus?.focus());
        },
        closeDlg() {
            this.dlg = { type: '' };
        },
        // folderId: the folder to prefill; a page's missing links start in its own folder.
        async openCreatePage(title = '', folderId = null) {
            await this.ensureCodices();
            if (!this.codexChoices.length) {
                this.showMessage('Make a codex first.', 'error');
                return;
            }
            const codexId = this.screen === 'codex' ? this.codex.codex.id : this.page ? this.page.codex.id : this.codexChoices[0].id;
            const folders = this.screen === 'codex' ? this.codex.folders || [] : (await this.fetchFolders(codexId)) || [];
            if (folderId === null) folderId = this.page ? this.page.folder_id || '' : '';
            if (!folders.some((f) => f.id === folderId)) folderId = '';
            this.dlg = { type: 'createPage', title, codexId, folders, folderId, error: '', busy: false };
            this.focusDlg();
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
            const folders = await this.fetchFolders(codexId);
            if (this.dlg === d && d.codexId === codexId) d.folders = folders || [];
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
            const folders = await this.fetchFolders(this.page.codex.id);
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
                else if (d.type === 'codex') await this.submitCodex(d);
                else if (d.type === 'rename') await this.submitRename(d);
                else if (d.type === 'aliases') await this.submitAliases(d);
                else if (d.type === 'move') await this.submitMove(d);
                else if (d.type === 'folder') await this.submitFolder(d);
                else if (d.type === 'folderMove') await this.submitFolderMove(d);
                else if (d.type === 'pageFolder') await this.submitPageFolder(d);
            } catch (error) {
                d.error = 'Something went wrong: ' + error.message;
            } finally {
                d.busy = false;
            }
        },
        // Shared error text for a failed dialog request; true when handled.
        failDlg(d, response, data) {
            if (response.status === 401) d.error = 'Your login has expired. Log in again, then try once more.';
            else if (response.status === 404) d.error = this.sentence(data.error || 'That no longer exists');
            else if (data.code === 'name_taken') d.error = this.conflictText(data);
            else if (data.code === 'folder_name_taken') d.error = 'A folder with that name already exists there.';
            else d.error = this.sentence(data.error || 'Request failed');
        },
        async submitCreatePage(d) {
            const title = d.title.trim();
            if (!title) {
                d.error = 'Give the page a title.';
                return;
            }
            const { response, data } = await this.send('/api/codices/pages', 'POST', { title, codex_id: d.codexId, folder_id: d.folderId || '' });
            if (response.status === 201) {
                this.saved = true;
                window.location.href = codexPageUrl({ id: data.page.id, title }, '/edit');
                return;
            }
            this.failDlg(d, response, data);
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
                window.location.href = codexPageUrl(this.page) + '?moved=1';
                return;
            }
            this.failDlg(d, response, data);
        },

        // ---- Delete ----
        async deletePage() {
            if (this.busy) return;
            const p = this.page;
            const ok = await this.askConfirm({
                eyebrow: 'Codex',
                title: `Delete “${p.title}”?`,
                message: `This deletes the page and all ${p.revision_count} of its revisions. It cannot be undone. Links to it become missing links.`,
                confirmLabel: 'Delete page',
                danger: true
            });
            if (!ok) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(p.id)}`, 'DELETE');
                if (response.ok || response.status === 404) {
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
            await this.loadTitles();
            if (!window.createCodexEditor) return;
            await this.$nextTick();
            try {
                this.editor = await window.createCodexEditor(this.$refs.editorHost, {
                    doc: () => this.draft.body,
                    titles: () => this.titles,
                    onChange: (value) => { this.draft.body = value; }
                });
                this.editorReady = !!this.editor;
            } catch (error) {
                console.error('Editor failed to load, using the plain text box:', error);
            }
        },
        async save() {
            if (this.busy || !this.draft || this.draft.locked) return;
            const d = this.draft;
            d.error = '';
            this.busy = true;
            try {
                const { response, data } = await this.send(`/api/codices/pages/${encodeURIComponent(this.page.id)}`, 'PUT', { base_revision: d.base, body: d.body, note: d.note.trim() });
                if (response.ok) {
                    this.saved = true;
                    window.location.href = this.pageUrl(this.page) + (response.status === 201 ? `?saved=${data.revision.number}` : '?unchanged=1');
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
        showMessage(text, type) {
            this.messageText = text;
            this.messageType = type;
            this.messageVisible = true;
            clearTimeout(this.messageTimer);
            this.messageTimer = setTimeout(() => {
                this.messageVisible = false;
            }, 5000);
        }
    };
};
