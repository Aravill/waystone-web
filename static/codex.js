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
            this.initMap();
        },

        initMap() {},

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
                    if (r.screen === 'history') await this.loadHistory(Number(new URLSearchParams(location.search).get('r')) || 0);
                    if (r.screen === 'page') await this.loadTitles();
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
