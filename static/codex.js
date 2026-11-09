// Codex page: /codex (home), /codex/w/{id} (world), /codex/p/{id}/{slug}[/edit|/history] (page).
// Relies on campaigns.js (escapeHtml, renderMarkdown, journalByline, userAvatarHtml), journal.js
// (diffWords, diffHtml) and codex-editor.js (createCodexEditor). One component picks the screen
// from the path. Top-level names here must not clash with those scripts' (const clashes throw).

// Mirrors wikiLinkRe in db/codex_links.go.
const CODEX_LINK_RE = /\[\[([^\[\]\n|]{1,200})(?:\|([^\[\]\n]{1,200}))?\]\]/g;

const codexNorm = (s) => String(s || '').trim().split(/\s+/).join(' ').toLowerCase();

function codexSlug(title) {
    const s = String(title || '').toLowerCase().replace(/[^\p{L}\p{N}]+/gu, '-').replace(/^-+|-+$/g, '').slice(0, 60).replace(/-+$/, '');
    return s || 'page';
}

function codexPageUrl(p, tail = '') {
    return `/codex/p/${encodeURIComponent(p.id)}/${encodeURIComponent(codexSlug(p.title))}${tail}`;
}

// Markdown with [[wikilinks]]. lookup(text) returns the target page ({id, title}) or null for a
// missing page; with no lookup (old revisions) links are shown plain. The links are swapped for
// placeholders before rendering so the Markdown renderer never sees them.
function codexMarkdown(body, lookup) {
    const found = [];
    const text = String(body || '').replace(CODEX_LINK_RE, (m, t, label) => {
        found.push({ text: t.trim(), label });
        return `\u0001${found.length - 1}\u0001`;
    });
    return renderMarkdown(text).replace(/\u0001(\d+)\u0001/g, (m, i) => {
        const l = found[Number(i)];
        const shown = escapeHtml(l.label || l.text);
        if (!lookup) return `<span class="wikilink wikilink--plain">${shown}</span>`;
        const target = lookup(l.text);
        if (target) return `<a class="wikilink" href="${codexPageUrl(target)}">${shown}</a>`;
        return `<a class="wikilink wikilink--missing" href="#" data-text="${escapeHtml(l.text)}" title="No page yet. Click to create.">${shown}</a>`;
    });
}

let codexMapCtl = null; // the mounted map viewer; kept out of Alpine's reactive state

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
        world: null,
        pageData: null,
        pageFilter: '',
        titles: [], // pages of the current page's world, for previews and the editor
        worldChoices: [],
        dlg: { type: '' },
        // Editor
        draft: null,
        conflict: null,
        editor: null,
        editorReady: false,
        // Map
        pinMenu: null, // { x, y, px, py } where an empty spot was clicked
        panel: null, // { pin, loading, data } for the pin shown in the side panel
        movingPin: '',
        pinDlg: { open: false, x: 0, y: 0, query: '', busy: false, error: '' },
        images: [],
        pinEvents: [],
        // History
        revisions: [],
        pruned: false,
        currentRevision: 0,
        historyLoading: false,
        historyMode: 'changes',
        selected: 0,
        selectedDiff: null,
        deltas: {},
        deltaSeq: 0,

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
            window.addEventListener('beforeunload', (event) => {
                if (this.isDirty) {
                    event.preventDefault();
                    event.returnValue = '';
                }
            });
            await this.load();
            this.showArrivalMessage();
        },

        // /codex, /codex/w/{id}[/slug], /codex/p/{id}/{slug}[/edit|/history]
        parseRoute() {
            const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
            if (parts[1] === 'w' && parts[2]) return { screen: 'world', id: parts[2] };
            if (parts[1] === 'p' && parts[2]) {
                const tail = parts[4] || (parts.length === 4 ? parts[3] : '');
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
            if (s === 'world' && this.world) return [{ label: this.world.world.name, href: '' }];
            if (!this.page) return [];
            const list = [{ label: this.page.world.name, href: '/codex/w/' + encodeURIComponent(this.page.world.id) }];
            if (s === 'page') return [...list, { label: this.page.title, href: '' }];
            return [...list, { label: this.page.title, href: this.pageUrl(this.page) }, { label: s === 'edit' ? 'Edit' : 'History', href: '' }];
        },

        async load() {
            const seq = ++this.loadSeq; // an older response must not overwrite a newer one
            try {
                const r = this.route;
                const url = r.screen === 'home' ? '/api/codex'
                    : r.screen === 'world' ? `/api/codex/worlds/${encodeURIComponent(r.id)}`
                    : `/api/codex/pages/${encodeURIComponent(r.id)}`;
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
                    this.loadError = data.error || 'Failed to load the Codex';
                    return;
                }
                this.notFound = false;
                this.loadError = '';
                if (r.screen === 'home') {
                    this.home = data;
                    document.title = 'Waystone - Codex';
                } else if (r.screen === 'world') {
                    this.world = data;
                    document.title = 'Waystone - ' + data.world.name;
                } else {
                    this.pageData = data;
                    document.title = 'Waystone - ' + data.page.title;
                    if (location.pathname !== this.pageUrl(data.page) + (r.screen === 'page' ? '' : '/' + r.screen)) {
                        history.replaceState(null, '', this.pageUrl(data.page) + (r.screen === 'page' ? '' : '/' + r.screen) + location.search);
                    }
                    if (r.screen === 'edit' && !this.draft) await this.initDraft();
                    if (r.screen === 'history') {
                        await this.loadHistory(Number(new URLSearchParams(location.search).get('r')) || 0);
                        if (data.page.kind === 'map') await this.loadMapHistory();
                    }
                    if (r.screen === 'page') await this.loadTitles();
                    if (r.screen === 'page' && data.page.kind === 'map') this.$nextTick(() => this.mountMap());
                }
            } catch (error) {
                if (seq === this.loadSeq) this.loadError = 'Could not load the Codex: ' + error.message;
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
        recentHtml(r, withWorld) {
            const note = r.note ? ` · ${escapeHtml(r.note)}` : '';
            const where = withWorld ? ` · ${escapeHtml(r.world.name)}` : '';
            return `<a href="${codexPageUrl(r)}">${r.kind === 'map' ? '◈ ' : ''}${escapeHtml(r.title)}</a><span class="muted"> · revision ${r.number} by ${escapeHtml(r.author_user.display_name)}${where}${note} · ${escapeHtml(this.shortDate(r.created_at))}</span>`;
        },
        get worldMaps() {
            return this.world ? this.world.pages.filter((p) => p.kind === 'map') : [];
        },
        get filteredPages() {
            const q = codexNorm(this.pageFilter);
            return this.world.pages.filter((p) => !q || codexNorm(p.title).includes(q) || p.aliases.some((a) => codexNorm(a).includes(q)));
        },

        // fetch with JSON in and out. An expired login comes back as a redirect to the login page,
        // which is reported as status 401 so the editor can keep the draft instead of leaving.
        async send(url, method, body) {
            const response = await fetch(url, body === undefined
                ? { method }
                : { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
            if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                return { response: { ok: false, status: 401 }, data: {} };
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
        // Lookup by title or alias among the pages of this world.
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
        get previewHtml() {
            return this.draft ? codexMarkdown(this.draft.body, this.titleLookup) : '';
        },
        async loadTitles() {
            if (!this.page) return;
            try {
                const { response, data } = await this.send(`/api/codex/titles?world=${encodeURIComponent(this.page.world.id)}`, 'GET');
                if (response.ok) this.titles = data;
            } catch (error) {
                console.error('Failed to load titles:', error);
            }
        },
        onBodyClick(event) {
            const a = event.target.closest('a.wikilink--missing');
            if (!a) return;
            event.preventDefault();
            this.openCreatePage('lore', a.dataset.text);
        },

        // ---- Dialogs ----
        get dlgEyebrow() {
            return { createPage: 'Codex', world: 'Codex', rename: 'Page', aliases: 'Page', move: 'Page' }[this.dlg.type] || '';
        },
        get dlgTitle() {
            const d = this.dlg;
            if (d.type === 'createPage') return d.kind === 'map' ? 'New map' : 'New page';
            return { world: d.world ? 'Edit world' : 'New world', rename: 'Rename page', aliases: 'Aliases', move: 'Move to another world' }[d.type] || '';
        },
        get dlgSubmit() {
            const d = this.dlg;
            if (d.type === 'createPage') return d.busy ? 'Creating...' : 'Create';
            return d.busy ? 'Saving...' : d.type === 'move' ? 'Move page' : d.type === 'world' && !d.world ? 'Create world' : 'Save';
        },
        get moveWarning() {
            const n = this.pageData?.incoming_count || 0;
            const here = this.page?.world.name;
            return (n > 0
                ? `${this.countLabel(n, 'link')} from ${here} will turn into missing links, because links only reach pages in their own world. `
                : '') + "This page's own links are checked again in the new world.";
        },
        async ensureWorlds() {
            if (this.worldChoices.length) return;
            try {
                const { response, data } = await this.send('/api/codex', 'GET');
                if (response.ok) this.worldChoices = [{ id: 'unsorted', name: 'Unsorted' }, ...data.worlds.map((w) => ({ id: w.id, name: w.name }))];
            } catch (error) {
                this.showMessage('Could not load the worlds: ' + error.message, 'error');
            }
        },
        focusDlg() {
            this.$nextTick(() => this.$refs.dlgFocus?.focus());
        },
        closeDlg() {
            this.dlg = { type: '' };
        },
        async openCreatePage(kind, title = '') {
            await this.ensureWorlds();
            const worldId = this.screen === 'world' ? this.world.world.id : this.page ? this.page.world.id : 'unsorted';
            this.dlg = { type: 'createPage', kind, title, worldId, error: '', busy: false };
            this.focusDlg();
        },
        openCreateWorld() {
            this.dlg = { type: 'world', world: null, name: '', description: '', error: '', busy: false };
            this.focusDlg();
        },
        openEditWorld() {
            const w = this.world.world;
            this.dlg = { type: 'world', world: w, name: w.name, description: w.description, error: '', busy: false };
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
            await this.ensureWorlds();
            this.dlg = { type: 'move', worldId: this.worldChoices.find((w) => w.id !== this.page.world.id)?.id || '', error: '', busy: false };
            this.focusDlg();
        },
        conflictText(data) {
            const c = (data.conflicts || [])[0];
            return c ? `“${c.name}” is already used in this world${c.title && c.title !== c.name ? ' (by ' + c.title + ')' : ''}. Titles and aliases are unique within a world.` : this.sentence(data.error);
        },
        async submitDlg() {
            const d = this.dlg;
            if (d.busy) return;
            d.error = '';
            d.busy = true;
            try {
                if (d.type === 'createPage') await this.submitCreatePage(d);
                else if (d.type === 'world') await this.submitWorld(d);
                else if (d.type === 'rename') await this.submitRename(d);
                else if (d.type === 'aliases') await this.submitAliases(d);
                else if (d.type === 'move') await this.submitMove(d);
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
            else d.error = this.sentence(data.error || 'Request failed');
        },
        async submitCreatePage(d) {
            const title = d.title.trim();
            if (!title) {
                d.error = 'Give the page a title.';
                return;
            }
            const { response, data } = await this.send('/api/codex/pages', 'POST', { kind: d.kind, title, world_id: d.worldId });
            if (response.status === 201) {
                this.saved = true;
                window.location.href = codexPageUrl({ id: data.page.id, title }, '/edit');
                return;
            }
            this.failDlg(d, response, data);
        },
        async submitWorld(d) {
            const name = d.name.trim();
            if (!name) {
                d.error = 'Give the world a name.';
                return;
            }
            const body = { name, description: d.description };
            const { response, data } = d.world
                ? await this.send(`/api/codex/worlds/${encodeURIComponent(d.world.id)}`, 'PUT', body)
                : await this.send('/api/codex/worlds', 'POST', body);
            if (response.ok && !d.world) {
                window.location.href = '/codex/w/' + encodeURIComponent(data.world.id);
            } else if (response.ok) {
                this.closeDlg();
                this.worldChoices = [];
                await this.load();
                this.showMessage('World saved', 'success');
            } else if (data.code === 'world_name_taken') {
                d.error = 'A world with that name already exists.';
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
            const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.page.id)}/title`, 'PUT', { title });
            if (response.ok) {
                window.location.href = codexPageUrl({ id: this.page.id, title: data.title }) + `?renamed=${data.rewritten}`;
                return;
            }
            this.failDlg(d, response, data);
        },
        async submitAliases(d) {
            const aliases = d.text.split('\n').map((a) => a.trim()).filter(Boolean);
            const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.page.id)}/aliases`, 'PUT', { aliases });
            if (response.ok) {
                this.closeDlg();
                await this.load();
                this.showMessage('Aliases saved', 'success');
                return;
            }
            this.failDlg(d, response, data);
        },
        async submitMove(d) {
            const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.page.id)}/world`, 'PUT', { world_id: d.worldId });
            if (response.ok) {
                window.location.href = codexPageUrl(this.page) + '?moved=1';
                return;
            }
            this.failDlg(d, response, data);
        },

        // ---- Map ----
        get panelHtml() {
            const d = this.panel && this.panel.data;
            if (!d) return '';
            const map = new Map(d.links.map((l) => [codexNorm(l.text), l.page]));
            return codexMarkdown(d.page.body, (text) => map.get(codexNorm(text)) || null);
        },
        get pinOptions() {
            const q = codexNorm(this.pinDlg.query);
            const options = [];
            for (const t of this.titles) {
                if (!this.page || t.id === this.page.id) continue;
                const alias = t.aliases.find((a) => codexNorm(a).includes(q));
                if (q && !codexNorm(t.title).includes(q) && !alias) continue;
                options.push({ key: t.id, kind: 'page', title: t.title, label: (t.kind === 'map' ? '◈ ' : '✦ ') + t.title + (alias && !codexNorm(t.title).includes(q) ? ` (also known as ${alias})` : '') });
            }
            const typed = this.pinDlg.query.trim().split(/\s+/).join(' ');
            const exists = this.titles.some((t) => [t.title, ...t.aliases].some((n) => codexNorm(n) === q));
            const list = options.slice(0, 30);
            if (typed && !exists && !/[\[\]|]/.test(typed)) {
                list.push({ key: 'new', kind: 'new', title: typed, label: `New page “${typed}” in ${this.page ? this.page.world.name : ''}` });
            }
            return list;
        },
        async mountMap() {
            const m = this.pageData && this.pageData.map;
            const el = this.$refs.mapEl;
            if (!m || !m.image || !el) return;
            try {
                if (codexMapCtl && (codexMapCtl.el !== el || codexMapCtl.imageUrl !== m.image.url)) {
                    codexMapCtl.destroy();
                    codexMapCtl = null;
                }
                if (codexMapCtl) {
                    codexMapCtl.setPins(m.pins);
                    return;
                }
                codexMapCtl = await window.mountCodexMap(el, {
                    image: m.image,
                    pins: m.pins,
                    onPinClick: (pin) => this.onPinClick(pin),
                    onMapClick: (info) => { this.pinMenu = info; },
                    onPinMove: (id, x, y) => this.onPinMove(id, x, y)
                });
                const pinId = new URLSearchParams(location.search).get('pin');
                const pin = pinId && m.pins.find((p) => p.id === pinId);
                if (pin) {
                    codexMapCtl.focusPin(pin.id);
                    if (!pin.target || pin.target.kind !== 'map') this.openPanel(pin);
                }
            } catch (error) {
                this.showMessage('Could not show the map: ' + error.message, 'error');
            }
        },
        onPinClick(pin) {
            this.pinMenu = null;
            if (this.movingPin) return;
            if (pin.target && pin.target.kind === 'map') {
                window.location.href = codexPageUrl(pin.target);
                return;
            }
            this.openPanel(pin);
        },
        async openPanel(pin) {
            this.panel = { pin, loading: !!pin.target, data: null };
            if (codexMapCtl) codexMapCtl.setOpen(pin.id);
            if (pin.target) await this.loadPanelPage(pin.target.id);
        },
        async loadPanelPage(id) {
            const panel = this.panel;
            if (panel) panel.loading = true;
            try {
                const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(id)}`, 'GET');
                if (!this.panel || this.panel !== panel) return;
                if (response.ok) panel.data = data;
                else this.showMessage(this.sentence(data.error || 'Failed to load the page'), 'error');
            } catch (error) {
                this.showMessage('Could not load the page: ' + error.message, 'error');
            } finally {
                if (panel) panel.loading = false;
            }
        },
        // Links inside the panel stay in the panel; a link to another map opens that map.
        onPanelClick(event) {
            const a = event.target.closest('a.wikilink');
            if (!a) return;
            if (a.classList.contains('wikilink--missing')) {
                event.preventDefault();
                this.openCreatePage('lore', a.dataset.text);
                return;
            }
            const id = (a.getAttribute('href').split('/')[3] || '');
            const target = this.panel.data.links.map((l) => l.page).find((p) => p && p.id === id);
            if (!id || (target && target.kind === 'map')) return;
            event.preventDefault();
            this.loadPanelPage(decodeURIComponent(id));
        },
        startAddPin() {
            const at = this.pinMenu;
            this.pinMenu = null;
            if (!at) return;
            this.pinDlg = { open: true, x: at.x, y: at.y, query: '', busy: false, error: '' };
            this.$nextTick(() => this.$refs.pinFocus?.focus());
        },
        closePinDlg() {
            this.pinDlg.open = false;
        },
        async choosePin(option) {
            const d = this.pinDlg;
            if (!option || d.busy) return;
            d.busy = true;
            d.error = '';
            try {
                const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.page.id)}/pins`, 'POST',
                    { x: d.x, y: d.y, target_text: option.title, create_page: option.kind === 'new' });
                if (response.status === 201) {
                    d.open = false;
                    await this.reloadMap();
                    await this.loadTitles();
                    this.showMessage(option.kind === 'new' ? `Pinned the new page “${option.title}”` : 'Pin added', 'success');
                    this.openPanel(this.pageData.map.pins.find((p) => p.id === data.pin.id) || data.pin);
                } else if (data.code === 'name_taken') {
                    await this.loadTitles();
                    d.error = `A page named “${option.title}” already exists in this world. Pick it from the list.`;
                } else {
                    d.error = response.status === 401 ? 'Your login has expired. Log in again, then try once more.' : this.sentence(data.error || 'Failed to add the pin');
                }
            } catch (error) {
                d.error = 'Error adding the pin: ' + error.message;
            } finally {
                d.busy = false;
            }
        },
        // Reload the map page (pins, links) and redraw the pins without losing the zoom.
        async reloadMap() {
            const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.page.id)}`, 'GET');
            if (!response.ok) return;
            this.pageData = data;
            if (codexMapCtl) codexMapCtl.setPins(data.map.pins);
            if (this.panel) {
                const pin = data.map.pins.find((p) => p.id === this.panel.pin.id);
                if (pin) {
                    this.panel.pin = pin;
                    if (codexMapCtl) codexMapCtl.setOpen(pin.id);
                } else {
                    this.panel = null;
                }
            }
        },
        startMove() {
            if (!this.panel || !codexMapCtl) return;
            this.movingPin = this.panel.pin.id;
            codexMapCtl.setMovable(this.movingPin);
        },
        cancelMove() {
            this.movingPin = '';
            if (codexMapCtl) {
                codexMapCtl.setMovable('');
                codexMapCtl.setPins(this.pageData.map.pins);
                if (this.panel) codexMapCtl.setOpen(this.panel.pin.id);
            }
        },
        async onPinMove(id, x, y) {
            if (this.movingPin !== id) return;
            this.movingPin = '';
            if (codexMapCtl) codexMapCtl.setMovable('');
            try {
                const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.page.id)}/pins/${encodeURIComponent(id)}`, 'PUT', { x, y });
                if (!response.ok) this.showMessage(this.sentence(data.error || 'Failed to move the pin'), 'error');
                else this.showMessage('Pin moved', 'success');
            } catch (error) {
                this.showMessage('Error moving the pin: ' + error.message, 'error');
            }
            await this.reloadMap();
        },
        async removePin() {
            if (!this.panel || this.busy) return;
            const pin = this.panel.pin;
            const ok = await this.askConfirm({
                eyebrow: this.page.title,
                title: 'Remove this pin?',
                message: `The pin for “${pin.target ? pin.target.title : pin.target_text}” is removed from the map. The page itself stays. You can undo this from the page history.`,
                confirmLabel: 'Remove pin',
                danger: true
            });
            if (!ok) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.page.id)}/pins/${encodeURIComponent(pin.id)}`, 'DELETE');
                if (response.ok || response.status === 404) {
                    this.panel = null;
                    await this.reloadMap();
                    this.showMessage('Pin removed', 'success');
                } else {
                    this.showMessage(this.sentence(data.error || 'Failed to remove the pin'), 'error');
                }
            } catch (error) {
                this.showMessage('Error removing the pin: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        },
        // The picture's width over its height, read in the browser before uploading.
        imageRatio(file) {
            return new Promise((resolve) => {
                const url = URL.createObjectURL(file);
                const img = new Image();
                img.onload = () => { URL.revokeObjectURL(url); resolve(img.naturalWidth / img.naturalHeight); };
                img.onerror = () => { URL.revokeObjectURL(url); resolve(0); };
                img.src = url;
            });
        },
        async onImageChosen(event) {
            const input = event.target;
            const file = input.files && input.files[0];
            input.value = '';
            if (!file || this.busy) return;
            const current = this.pageData.map.image;
            if (current && this.pageData.map.pins.length) {
                const ratio = await this.imageRatio(file);
                const was = current.width / current.height;
                if (ratio && Math.abs(ratio - was) / was > 0.02) {
                    const ok = await this.askConfirm({
                        eyebrow: this.page.title,
                        title: 'Replace with a different shape?',
                        message: 'The new image has a different width-to-height ratio than the old one. Pins are placed by position, so they may land in the wrong place. You can restore the old image from the page history.',
                        confirmLabel: 'Replace image',
                        danger: true
                    });
                    if (!ok) return;
                }
            }
            this.busy = true;
            try {
                const response = await fetch(`/api/codex/pages/${encodeURIComponent(this.page.id)}/images`, { method: 'POST', headers: { 'Content-Type': file.type || 'application/octet-stream' }, body: file });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    this.showMessage('Your login has expired. Log in again, then upload the image.', 'error');
                    return;
                }
                const data = await response.json().catch(() => ({}));
                if (response.status === 201) {
                    await this.reloadMap();
                    this.$nextTick(() => this.mountMap());
                    this.showMessage('Image uploaded', 'success');
                } else {
                    this.showMessage(this.sentence(data.error || 'Failed to upload the image'), 'error');
                }
            } catch (error) {
                this.showMessage('Error uploading the image: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        },
        async loadMapHistory() {
            try {
                const id = encodeURIComponent(this.route.id);
                const [imgs, events] = await Promise.all([this.send(`/api/codex/pages/${id}/images`, 'GET'), this.send(`/api/codex/pages/${id}/pin-events`, 'GET')]);
                if (imgs.response.ok) this.images = imgs.data;
                if (events.response.ok) this.pinEvents = events.data;
            } catch (error) {
                this.showMessage('Could not load the map history: ' + error.message, 'error');
            }
        },
        async restoreImage(img) {
            if (this.busy) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.page.id)}/images/${encodeURIComponent(img.id)}/restore`, 'POST');
                if (response.ok) {
                    await this.loadMapHistory();
                    this.showMessage('Image restored', 'success');
                } else {
                    this.showMessage(this.sentence(data.error || 'Failed to restore the image'), 'error');
                }
            } catch (error) {
                this.showMessage('Error restoring the image: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        },
        async undoPinEvent(e) {
            if (this.busy) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.page.id)}/pin-events/${encodeURIComponent(e.id)}/undo`, 'POST');
                if (response.ok) this.showMessage('Pin change undone', 'success');
                else if (data.code === 'already_undone') this.showMessage('That change was already undone.', 'info');
                else this.showMessage(this.sentence(data.error || 'Failed to undo the change'), 'error');
                await this.loadMapHistory();
            } catch (error) {
                this.showMessage('Error undoing the change: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        },

        // ---- Delete ----
        async deletePage() {
            if (this.busy) return;
            const p = this.page;
            const ok = await this.askConfirm({
                eyebrow: 'Codex',
                title: `Delete “${p.title}”?`,
                message: `This deletes the page and all ${p.revision_count} of its revisions${p.kind === 'map' ? ', its map images and pins' : ''}. It cannot be undone. Links to it become missing links.`,
                confirmLabel: 'Delete page',
                danger: true
            });
            if (!ok) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(p.id)}`, 'DELETE');
                if (response.ok || response.status === 404) {
                    window.location.href = '/codex/w/' + encodeURIComponent(p.world.id) + '?page_deleted=1';
                    return;
                }
                this.showMessage(response.status === 401 ? 'Your login has expired. Log in again, then delete the page.' : this.sentence(data.error || 'Failed to delete the page'), 'error');
            } catch (error) {
                this.showMessage('Error deleting the page: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        },
        async deleteWorld() {
            if (this.busy) return;
            const w = this.world.world;
            const ok = await this.askConfirm({
                eyebrow: 'Codex',
                title: `Delete the world “${w.name}”?`,
                message: 'A world can only be deleted when it has no pages and no game is set in it. This cannot be undone.',
                confirmLabel: 'Delete world',
                danger: true
            });
            if (!ok) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(`/api/codex/worlds/${encodeURIComponent(w.id)}`, 'DELETE');
                if (response.ok) {
                    window.location.href = '/codex';
                } else if (data.code === 'world_in_use') {
                    const parts = [];
                    if (data.page_count) parts.push(`it still has ${this.countLabel(data.page_count, 'page')}. Move or delete them first`);
                    if (data.games && data.games.length) parts.push(`these games are set in it: ${data.games.join(', ')}. Their DMs can pick another world`);
                    this.showMessage(this.sentence("Can't delete " + w.name + ': ' + parts.join('; ')), 'error');
                } else {
                    this.showMessage(this.sentence(data.error || 'Failed to delete the world'), 'error');
                }
            } catch (error) {
                this.showMessage('Error deleting the world: ' + error.message, 'error');
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
                    doc: this.draft.body,
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
                const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.page.id)}`, 'PUT', { base_revision: d.base, body: d.body, note: d.note.trim() });
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
                const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.route.id)}/revisions`, 'GET');
                if (!response.ok) {
                    this.loadError = data.error || 'Failed to load the history';
                    return;
                }
                this.revisions = data.revisions;
                this.pruned = data.pruned;
                this.currentRevision = data.current_revision;
                this.selected = select && this.revisions.some((r) => r.number === select) ? select : this.currentRevision;
                this.updateDiff();
                this.computeDeltas();
            } catch (error) {
                this.loadError = 'Could not load the history: ' + error.message;
            } finally {
                this.historyLoading = false;
            }
        },
        updateDiff() {
            const r = this.selectedRevision, prev = this.previousRevision;
            if (!r || !prev) {
                this.selectedDiff = null;
                return;
            }
            const d = diffWords(prev.body, r.body);
            this.selectedDiff = { added: d.added, removed: d.removed, html: diffHtml(d.segs) };
        },
        // The +/- counts for the list, a few at a time so a long history doesn't freeze the page.
        async computeDeltas() {
            const seq = ++this.deltaSeq;
            this.deltas = {};
            for (let i = 0; i < this.revisions.length; i++) {
                const r = this.revisions[i];
                const prev = this.revisions.find((x) => x.number === r.number - 1);
                if (prev) {
                    const d = diffWords(prev.body, r.body);
                    if (seq !== this.deltaSeq) return;
                    this.deltas = { ...this.deltas, [r.number]: { added: d.added, removed: d.removed } };
                }
                if (i % 4 === 3) await new Promise((resolve) => setTimeout(resolve));
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
                const { response, data } = await this.send(`/api/codex/pages/${encodeURIComponent(this.page.id)}/revisions/${r.number}/restore`, 'POST', { base_revision: this.currentRevision });
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
