// Game journal page: /games/{id}/journal/{sessionId} (the entry), /edit and /history. Relies on
// campaigns.js (loaded first) for userAvatarHtml, escapeHtml, renderMarkdown, parseDateLocal and the
// journal helpers (journalByline, journalTitleHtml). One component picks the screen from the path.

const esc = escapeHtml;

// Past this many cells the LCS matrix would be too big for the browser, so a large rewrite is
// shown (and merged) as one removal plus one addition.
const MAX_LCS_CELLS = 4e6;

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
        if (g.t === '+') return `<ins>${esc(g.v)}</ins>`;
        if (g.t === '-') return `<del>${esc(g.v)}</del>`;
        const paras = g.v.split(/\n\s*\n/);
        const first = idx === 0, last = idx === segs.length - 1;
        // Keep the paragraph touching each change; collapse the rest.
        const keepHead = first ? 0 : 1, keepTail = last ? 0 : 1;
        const hidden = paras.length - keepHead - keepTail;
        if (hidden < 1) return esc(g.v);
        const label = `<span class="gap">⋯ ${hidden} unchanged paragraph${hidden === 1 ? '' : 's'} ⋯</span>`;
        const head = keepHead ? esc(paras[0]) : '';
        const tail = keepTail ? esc(paras.at(-1)) : '';
        return head + label + tail;
    }).join('');
}

// Three-way merge by paragraph. Paragraphs unchanged in both versions anchor the comparison;
// between anchors, a change on one side is taken and different changes on both sides conflict.
// toParas matches the server's canonical form (api/journal.go), so "no change" is an exact comparison.
const toParas = (t) => String(t).split(/\n[ \t]*\n/).map((p) => p.trim()).filter(Boolean);

function lcsPairs(x, y) {
    const pairs = new Map();
    if (x.length * y.length > MAX_LCS_CELLS) return pairs; // no anchors: the whole text is one conflict
    const L = Array.from({ length: x.length + 1 }, () => new Uint16Array(y.length + 1));
    for (let i = x.length - 1; i >= 0; i--) {
        for (let j = y.length - 1; j >= 0; j--) {
            L[i][j] = x[i] === y[j] ? L[i + 1][j + 1] + 1 : Math.max(L[i + 1][j], L[i][j + 1]);
        }
    }
    let i = 0, j = 0;
    while (i < x.length && j < y.length) {
        if (x[i] === y[j]) { pairs.set(i, j); i++; j++; }
        else if (L[i + 1][j] >= L[i][j + 1]) i++;
        else j++;
    }
    return pairs;
}

function merge3(baseText, mineText, theirsText) {
    const base = toParas(baseText), mine = toParas(mineText), theirs = toParas(theirsText);
    const toMine = lcsPairs(base, mine), toTheirs = lcsPairs(base, theirs);
    const same = (a, b) => a.length === b.length && a.every((p, i) => p === b[i]);
    const parts = [];
    let auto = 0, nextId = 1;
    const addOk = (paras) => {
        if (!paras.length) return;
        const last = parts.at(-1);
        if (last && !last.conflict) last.paras.push(...paras); else parts.push({ paras: [...paras] });
    };
    let b = 0, m = 0, t = 0;
    const anchors = [...toMine.keys()].filter((i) => toTheirs.has(i));
    anchors.push(base.length); // end of text
    for (const anchor of anchors) {
        const bs = base.slice(b, anchor);
        const ms = mine.slice(m, anchor < base.length ? toMine.get(anchor) : mine.length);
        const ts = theirs.slice(t, anchor < base.length ? toTheirs.get(anchor) : theirs.length);
        const mineChanged = !same(bs, ms), theirsChanged = !same(bs, ts);
        if (mineChanged && theirsChanged && !same(ms, ts)) {
            parts.push({ conflict: true, id: 'c' + nextId++, base: bs, mine: ms, theirs: ts, choice: '', text: '' });
        } else {
            if (mineChanged || theirsChanged) auto++;
            addOk(mineChanged ? ms : ts);
        }
        if (anchor < base.length) {
            addOk([base[anchor]]);
            b = anchor + 1; m = toMine.get(anchor) + 1; t = toTheirs.get(anchor) + 1;
        }
    }
    return { parts, auto };
}

function mergeName(base, mine, theirs) {
    if (mine === theirs || theirs === base) return { name: mine };
    if (mine === base) return { name: theirs };
    return { conflict: { id: 'name', isName: true, mine, theirs, choice: '', text: mine } };
}

const conflictsOf = (m) => [...(m.nameConflict ? [m.nameConflict] : []), ...m.parts.filter((p) => p.conflict)];
const resolvedParas = (c) =>
    c.choice === 'mine' ? c.mine : c.choice === 'theirs' ? c.theirs :
    c.choice === 'both' ? [...c.mine, ...c.theirs] : toParas(c.text);
const mergedText = (m) => m.parts.flatMap((p) => p.conflict ? resolvedParas(p) : p.paras).join('\n\n');
const mergedName = (m) => {
    const c = m.nameConflict;
    if (!c) return m.name;
    return (c.choice === 'mine' ? c.mine : c.choice === 'theirs' ? c.theirs : c.text).trim();
};
const isResolved = (c) => !!c.choice && (c.choice !== 'edit' || c.isName || toParas(c.text).length > 0);

window.journalPage = function () {
    return {
        displayName: 'User',
        isAdmin: false,
        currentUserId: '',
        gameId: '',
        sessionId: '',
        view: 'entry', // from the path: entry, edit or history
        campaign: null,
        data: null, // GET .../journal/{sid}
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
        // Editor
        draft: null, // { name, body, note, base, entryId, tab, error, locked, startedNew, initial }
        merge: null,
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
            // /games/{id}/journal/{sessionId}[/edit|/history]
            const parts = location.pathname.split('/').filter(Boolean);
            this.gameId = decodeURIComponent(parts[1] || '');
            this.sessionId = decodeURIComponent(parts[3] || '');
            this.view = parts[4] === 'edit' || parts[4] === 'history' ? parts[4] : 'entry';
            window.addEventListener('beforeunload', (event) => {
                if (this.isDirty) {
                    event.preventDefault();
                    event.returnValue = '';
                }
            });
            await this.load();
            this.showArrivalMessage();
        },

        async load() {
            const seq = ++this.loadSeq; // an older response must not overwrite a newer one
            try {
                const base = `/api/campaigns/${encodeURIComponent(this.gameId)}`;
                const [campaignRes, journalRes] = await Promise.all([fetch(base), fetch(`${base}/journal/${encodeURIComponent(this.sessionId)}`)]);
                for (const res of [campaignRes, journalRes]) {
                    if (res.redirected || !res.headers.get('Content-Type')?.includes('application/json')) {
                        window.location.href = '/login.html';
                        return;
                    }
                }
                if (seq !== this.loadSeq) return;
                if (campaignRes.status === 404 || journalRes.status === 404) {
                    this.notFound = true;
                    this.loadError = '';
                    return;
                }
                const campaign = await campaignRes.json();
                const data = await journalRes.json();
                if (!campaignRes.ok || !journalRes.ok) {
                    this.loadError = (!campaignRes.ok ? campaign.error : data.error) || 'Failed to load the journal';
                    return;
                }
                this.campaign = campaign;
                this.data = data;
                this.notFound = false;
                this.loadError = '';
                document.title = 'Waystone - ' + journalTitle(campaign.title, data.session.number, data.entry ? data.entry.name : '');
                if (this.view === 'edit' && data.can_edit && !this.draft) this.initDraft();
                if (this.view === 'history' && data.entry) await this.loadHistory();
            } catch (error) {
                if (seq === this.loadSeq) this.loadError = 'Could not load the journal: ' + error.message;
            } finally {
                if (seq === this.loadSeq) this.loading = false;
            }
        },

        // ---- Derived state ----
        get entry() {
            return this.data?.entry || null;
        },
        get played() {
            return !!this.data?.session.played;
        },
        get attendees() {
            return this.data?.session.attendees || [];
        },
        get scribes() {
            return this.entry?.scribes || [];
        },
        get screen() {
            if (this.view === 'edit') return !this.data.can_edit ? 'denied' : this.merge ? 'merge' : 'edit';
            return this.view === 'history' && this.entry ? 'history' : 'entry';
        },
        get sessionLabel() {
            return this.data?.session.number ? `Session ${this.data.session.number}` : 'Session';
        },
        get eyebrow() {
            const s = this.data.session;
            return `Journal · ${s.played ? 'Session ' + s.number : 'Cancelled session'} · ${this.formatDate(s.date)}`;
        },
        get crumbs() {
            if (!this.campaign) return [];
            const list = [{ label: this.campaign.title, href: `/games/${encodeURIComponent(this.gameId)}` }];
            if (!this.data) return list;
            const isNew = this.view === 'edit' && !this.entry;
            if (this.view === 'entry' || isNew || (this.view === 'history' && !this.entry)) {
                list.push({ label: isNew ? 'Write ' + this.sessionLabel.toLowerCase() : this.sessionLabel, href: '' });
                return list;
            }
            list.push({ label: this.sessionLabel, href: this.url('') });
            list.push({ label: this.screen === 'merge' ? 'Combine changes' : this.view === 'history' ? 'History' : 'Edit', href: '' });
            return list;
        },
        get emptyNote() {
            return this.played ? 'Nobody has written this entry yet.' : 'This session has not been played, so it has no journal entry.';
        },
        get deniedText() {
            return !this.played ? 'This session has not been played, so its entry cannot be changed.' : "Only the game's DM and players can write the journal.";
        },
        get bodyHtml() {
            return renderMarkdown(this.entry?.body);
        },
        // Entries before and after this one in the game, by session number.
        get otherEntries() {
            return (this.campaign?.played_sessions || []).filter((s) => s.journal).sort((a, b) => a.number - b.number);
        },
        get prevEntry() {
            const list = this.otherEntries, i = list.findIndex((s) => s.id === this.sessionId);
            return i > 0 ? list[i - 1] : null;
        },
        get nextEntry() {
            const list = this.otherEntries, i = list.findIndex((s) => s.id === this.sessionId);
            return i >= 0 && i < list.length - 1 ? list[i + 1] : null;
        },
        get editByline() {
            if (this.draft.startedNew) return 'You will be listed as the author. Other players and the DM can amend it afterwards.';
            return `${journalByline(this.entry)} · saving adds revision ${this.draft.base + 1}`;
        },
        get cancelUrl() {
            return this.entry ? this.url('') : `/games/${encodeURIComponent(this.gameId)}`;
        },
        get isDirty() {
            if (this.saved || !this.draft) return false;
            const i = this.draft.initial, d = this.draft;
            return !!this.merge || d.name !== i.name || d.body !== i.body || d.note !== i.note;
        },

        url(tail) {
            return `/games/${encodeURIComponent(this.gameId)}/journal/${encodeURIComponent(this.sessionId)}${tail}`;
        },
        journalLink(s) {
            return `/games/${encodeURIComponent(this.gameId)}/journal/${encodeURIComponent(s.id)}`;
        },
        entryNavLabel(s) {
            return s.number + (s.journal.name ? ': ' + s.journal.name : '');
        },
        // The title as HTML: game, number from the session, optional name.
        titleHtml(dropCap, name = this.entry ? this.entry.name : '') {
            return journalTitleHtml(this.campaign.title, this.data.session.number, name, dropCap);
        },
        renderMarkdown(text) {
            return renderMarkdown(text);
        },
        personName(u) {
            return (u.id === this.campaign.dm ? '♛ ' : '') + u.display_name + (u.id === this.currentUserId ? ' (you)' : '');
        },
        formatDate(dateStr) {
            return parseDateLocal(dateStr).toLocaleDateString('default', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
        },
        shortDate(iso) {
            return new Date(iso).toLocaleDateString('default', { day: 'numeric', month: 'short' });
        },
        formatStamp(iso) {
            return new Date(iso).toLocaleString('default', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
        },
        userName(id) {
            const people = [...this.scribes.map((s) => s.user), this.campaign?.dm_user, ...(this.campaign?.player_users || [])];
            return people.find((u) => u && u.id === id)?.display_name || '';
        },

        // ---- Messages and requests ----
        showArrivalMessage() {
            const q = new URLSearchParams(location.search);
            if (!q.has('saved') && !q.has('unchanged') && !q.has('entry_deleted')) return;
            const who = this.userName(q.get('merged') || '');
            if (q.has('unchanged')) {
                this.showMessage(q.get('merged')
                    ? `Your changes were already in ${who || 'the other'}'s revision, so no revision was saved.`
                    : 'Nothing changed, so no revision was saved.', 'info');
            } else if (q.has('saved')) {
                const n = Number(q.get('saved'));
                this.showMessage(q.get('merged')
                    ? `Merged with ${who ? who + "'s" : 'the other'} changes and saved as revision ${n}`
                    : n === 1 ? 'Entry saved' : `Saved as revision ${n}`, 'success');
            }
            history.replaceState(null, '', location.pathname);
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
        apiUrl(tail = '') {
            return `/api/campaigns/${encodeURIComponent(this.gameId)}/journal/${encodeURIComponent(this.sessionId)}${tail}`;
        },
        sentence(text) {
            const t = String(text || '');
            return t.charAt(0).toUpperCase() + t.slice(1) + (t.endsWith('.') ? '' : '.');
        },

        // ---- Editor ----
        initDraft() {
            const e = this.entry;
            const draft = { name: e ? e.name : '', body: e ? e.body : '', note: '', base: e ? e.current_revision : 0, entryId: e ? e.id : '', tab: 'write', error: '', locked: false, startedNew: !e };
            draft.initial = { name: draft.name, body: draft.body, note: '' };
            this.draft = draft;
        },

        async save() {
            if (this.busy || !this.draft) return;
            const d = this.draft;
            d.error = '';
            if (!d.body.trim()) {
                d.error = 'Write some text before saving the entry.';
                return;
            }
            await this.attempt({ name: d.name.trim(), body: d.body, note: d.note.trim(), base: d.base, entryId: d.entryId, mergedWith: 0, mergedFrom: null });
        },

        // Save p. When someone else saved first, merge their text with ours: no conflicts means
        // another save against the newer revision, conflicts open the "Combine your changes" screen.
        // Whatever fails, the draft stays where it is.
        async attempt(p) {
            const d = this.draft;
            this.busy = true;
            d.error = '';
            try {
                for (let round = 0; round < 5; round++) {
                    let result;
                    try {
                        result = await this.send(this.apiUrl(), 'PUT', {
                            entry_id: p.entryId, base_revision: p.base, name: p.name, body: p.body, note: p.note, merged_with: p.mergedWith
                        });
                    } catch (error) {
                        d.error = `Could not reach the server: ${error.message}. Your draft is still here.`;
                        return;
                    }
                    const { response, data } = result;
                    if (response.ok) {
                        this.finishSave(response.status, data, p);
                        return;
                    }
                    if (response.status === 409 && (data.code === 'stale_base' || data.code === 'entry_replaced')) {
                        const next = await this.mergeAgainst(p, data);
                        if (!next) return;
                        p = next;
                        continue;
                    }
                    this.failSave(response.status, data, p);
                    return;
                }
                d.error = 'The entry keeps changing while you save. Try saving again.';
            } finally {
                this.busy = false;
            }
        },

        finishSave(status, data, p) {
            this.saved = true;
            const merged = p.mergedFrom ? `&merged=${encodeURIComponent(p.mergedFrom.id)}` : '';
            location.href = this.url('') + (status === 200 ? '?unchanged=1' : `?saved=${data.revision.number}`) + merged;
        },

        failSave(status, data, p) {
            const d = this.draft;
            if (status === 401) {
                d.error = 'Your login has expired. Your draft is still here. Log in again in another tab, then save again.';
            } else if (status === 403) {
                d.locked = true;
                d.error = "You can no longer write this journal: only the game's DM and players can. Your draft is still here.";
            } else if (status === 404) {
                d.locked = true;
                d.error = 'This game or session no longer exists. Your draft is still here.';
            } else if (status === 409 && data.code === 'entry_deleted') {
                // Keep the text, and let the next save create a fresh entry with this user as its author.
                d.name = p.name;
                d.body = p.body;
                d.base = 0;
                d.entryId = '';
                this.merge = null;
                d.error = 'The entry was deleted while you were editing. Your draft is still here, and saving creates a new entry with you as the author.';
            } else if (status === 409 && data.code === 'session_not_played') {
                d.locked = true;
                d.error = "This session is no longer marked as played, so the entry can't be saved. Your draft is still here.";
            } else if (status === 400) {
                d.error = this.sentence(data.error || 'The entry could not be saved');
            } else {
                d.error = this.sentence(data.error || 'Failed to save the entry') + ' Your draft is still here.';
            }
        },

        // Merge p with the revision someone else saved. Returns the next request to send, or null when the
        // merge screen is showing, the page is leaving or an error is shown.
        async mergeAgainst(p, conflict) {
            const d = this.draft, latest = conflict.latest;
            let revs;
            try {
                const { response, data } = await this.send(this.apiUrl('/revisions'), 'GET');
                if (!response.ok) throw new Error(data.error || 'status ' + response.status);
                revs = data.revisions;
            } catch (error) {
                d.error = `Could not load the latest version to combine with: ${error.message}. Your draft is still here.`;
                return null;
            }
            // A new draft meets an existing entry, a pruned base or a replaced entry: merge against empty text.
            const baseRev = conflict.code === 'entry_replaced' ? null : revs.find((r) => r.number === p.base);
            const m = merge3(baseRev ? baseRev.body : '', p.body, latest.body);
            const named = mergeName(baseRev ? baseRev.name : '', p.name, latest.name);
            m.name = named.name || '';
            m.nameConflict = named.conflict || null;
            if (conflictsOf(m).length) {
                this.openMerge(m, latest, conflict.entry_id);
                return null;
            }
            const body = mergedText(m);
            if (latest.name === m.name && toParas(latest.body).join('\n\n') === toParas(body).join('\n\n')) {
                this.saved = true;
                location.href = this.url('') + `?unchanged=1&merged=${encodeURIComponent(latest.author_id)}`;
                return null;
            }
            return { ...p, name: m.name, body, base: latest.number, entryId: conflict.entry_id, mergedWith: latest.number, mergedFrom: latest.author_user };
        },

        openMerge(m, latest, entryId) {
            const trim = (t) => t.length > 110 ? t.slice(0, 110).trimEnd() + '…' : t;
            const list = conflictsOf(m);
            for (const c of list) {
                if (c.isName) continue;
                const at = m.parts.indexOf(c);
                const before = at > 0 && !m.parts[at - 1].conflict ? m.parts[at - 1].paras.at(-1) : '';
                const after = at < m.parts.length - 1 && !m.parts[at + 1].conflict ? m.parts[at + 1].paras[0] : '';
                c.before = before ? trim(before) : '';
                c.after = after ? trim(after) : '';
                c.mineRemoved = c.mine.length === 0;
                c.theirsRemoved = c.theirs.length === 0;
                c.mineHtml = c.mineRemoved ? '' : diffHtml(diffWords(c.base.join('\n\n'), c.mine.join('\n\n')).segs);
                c.theirsHtml = c.theirsRemoved ? '' : diffHtml(diffWords(c.base.join('\n\n'), c.theirs.join('\n\n')).segs);
            }
            this.merge = { parts: m.parts, auto: m.auto, name: m.name, nameConflict: m.nameConflict, conflicts: list, theirsN: latest.number, by: latest.author_user, entryId };
            this.draft.error = '';
            this.$nextTick(() => document.querySelector('h1')?.focus());
        },

        get mergeWho() {
            return this.merge?.by?.display_name || 'Someone';
        },
        get openConflicts() {
            return this.merge ? this.merge.conflicts.filter((c) => !isResolved(c)).length : 0;
        },
        get mergeTitleHtml() {
            const nc = this.merge.nameConflict;
            if (nc && !isResolved(nc)) {
                return `${esc(this.campaign.title)} <span class="t-no">${this.data.session.number}</span>: <span class="unresolved">name to be decided</span>`;
            }
            return this.titleHtml(false, mergedName(this.merge));
        },
        get mergeBodyHtml() {
            return this.merge.parts.map((p) => {
                if (!p.conflict) return renderMarkdown(p.paras.join('\n\n'));
                if (isResolved(p)) return renderMarkdown(resolvedParas(p).join('\n\n'));
                return `<p class="unresolved">✦ Conflict ${this.merge.conflicts.findIndex((c) => c.id === p.id) + 1} goes here once you decide</p>`;
            }).join('');
        },
        isResolved(c) {
            return isResolved(c);
        },
        pick(c, choice) {
            c.choice = choice;
            if (choice === 'edit' && !c.isName && !c.text) c.text = [...c.mine, ...c.theirs].join('\n\n');
        },
        // Keeps the draft and its starting revision, so saving merges again.
        backToDraft() {
            this.merge = null;
            this.draft.error = '';
            this.$nextTick(() => document.querySelector('h1')?.focus());
        },
        async saveMerged() {
            if (this.busy || this.openConflicts > 0) return;
            const m = this.merge;
            await this.attempt({
                name: mergedName(m), body: mergedText(m), note: this.draft.note.trim(),
                base: m.theirsN, entryId: m.entryId, mergedWith: m.theirsN, mergedFrom: m.by
            });
        },

        // ---- History ----
        get revisionsNewestFirst() {
            return [...this.revisions].reverse();
        },
        get selectedRevision() {
            return this.revisions.find((r) => r.number === this.selected) || null;
        },
        get previousRevision() {
            return this.revisions.find((r) => r.number === this.selected - 1) || null;
        },

        async loadHistory(select) {
            this.historyLoading = true;
            try {
                const { response, data } = await this.send(this.apiUrl('/revisions'), 'GET');
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
            const list = this.revisionsNewestFirst;
            for (let i = 0; i < list.length; i++) {
                const prev = this.revisions.find((x) => x.number === list[i].number - 1);
                if (prev) {
                    const d = diffWords(prev.body, list[i].body);
                    if (seq !== this.deltaSeq) return;
                    this.deltas = { ...this.deltas, [list[i].number]: { added: d.added, removed: d.removed } };
                }
                if (i % 4 === 3) await new Promise((resolve) => setTimeout(resolve));
            }
        },
        deltaHtml(r) {
            const d = this.deltas[r.number];
            if (r.number === 1) return 'New entry';
            if (!this.revisions.some((x) => x.number === r.number - 1)) return 'Oldest kept';
            return d ? `<span aria-label="${d.added} words added, ${d.removed} removed"><span class="plus">+${d.added}</span> <span class="minus">−${d.removed}</span></span>` : '';
        },
        revNote(r) {
            if (r.restored_from) return `↺ Restored revision ${r.restored_from}`;
            if (r.merged_with) {
                const other = this.revisions.find((x) => x.number === r.merged_with);
                return `Merged with ${other ? other.author_user.display_name + "'s " : ''}revision ${r.merged_with}` + (r.note ? `: ${r.note}` : '');
            }
            return r.note;
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
                message: `The entry goes back to ${r.author_user.display_name}'s version from ${this.shortDate(r.created_at)}. This saves as revision ${this.currentRevision + 1}. The current version stays in the history, so you can undo this.`,
                confirmLabel: 'Restore',
                danger: false
            });
            if (!ok) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(this.apiUrl(`/revisions/${r.number}/restore`), 'POST', { base_revision: this.currentRevision });
                if (response.ok && response.status === 201) {
                    await this.reloadAfterWrite(data.revision.number);
                    this.showMessage(`Restored revision ${r.number} as revision ${data.revision.number}`, 'success');
                } else if (response.ok) {
                    this.showMessage('That version is already the current one, so no revision was saved.', 'info');
                } else if (response.status === 409 && data.code === 'stale_base') {
                    await this.reloadAfterWrite();
                    this.showMessage('The entry changed while you were looking at it. The history is up to date now. Choose the version again.', 'error');
                } else if (response.status === 409 || response.status === 403 || response.status === 404) {
                    this.showMessage(this.sentence(data.error || 'The version could not be restored'), 'error');
                } else if (response.status === 401) {
                    this.showMessage('Your login has expired. Log in again, then restore the version.', 'error');
                } else {
                    this.showMessage(this.sentence(data.error || 'Failed to restore the version'), 'error');
                }
            } catch (error) {
                this.showMessage('Error restoring the version: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        },
        async reloadAfterWrite(select) {
            await this.load();
            await this.loadHistory(select);
        },

        // ---- Delete ----
        async deleteEntry() {
            if (this.busy || !this.entry) return;
            const number = this.data.session.number;
            const e = this.entry;
            const ok = await this.askConfirm({
                eyebrow: 'Journal',
                title: number ? `Delete the entry for session ${number}?` : 'Delete this entry?',
                message: `This deletes "${journalTitle(this.campaign.title, number, e.name)}" and all ${e.revision_count} of its revisions, including text written by other people. It cannot be undone. The session itself stays, and someone can write a new entry for it.`,
                confirmLabel: 'Delete entry',
                danger: true
            });
            if (!ok) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(this.apiUrl(), 'DELETE');
                if (response.ok) {
                    window.location.href = `/games/${encodeURIComponent(this.gameId)}?entry_deleted=${number || 0}`;
                    return;
                }
                if (response.status === 404) {
                    this.showMessage('This entry was already deleted.', 'info');
                    await this.load();
                } else {
                    this.showMessage(response.status === 401 ? 'Your login has expired. Log in again, then delete the entry.' : this.sentence(data.error || 'Failed to delete the entry'), 'error');
                }
            } catch (error) {
                this.showMessage('Error deleting the entry: ' + error.message, 'error');
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
