// Game detail page. Relies on campaigns.js (loaded first) for CAMPAIGN_TYPES, TYPE_ICON_PATHS,
// campaignStatusClass, userAvatarHtml, escapeHtml, renderMarkdown and getDefaultTimezone.

// A blank record-session form. attendees holds ticked user ids; former holds ex-players kept on an edited session.
function emptyForm() {
    return { date: '', start: '', end: '', timezone: '', attendees: [], guests: [], guestDraft: '', former: [], error: '' };
}

window.gamePage = function () {
    return {
        displayName: 'User',
        isAdmin: false,
        currentUserId: '',
        gameId: '',
        campaign: null,
        loading: true,
        notFound: false,
        showAll: false,
        taleOpen: false, // phones fold a long tale to three lines
        taleCanFold: false,
        selectedId: null, // a session id from upcoming or played; null shows the company
        panelOpened: false, // the first load picks the opening selection, later loads keep it
        recording: false,
        editingId: '',
        form: emptyForm(),
        busy: false, // an action request is in flight
        loadError: '',
        loadSeq: 0,
        messageTimer: null,
        messageVisible: false,
        messageText: '',
        messageType: '',
        confirmDialog: { open: false, title: '', message: '', confirmLabel: '', resolve: null },

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
            this.gameId = decodeURIComponent(location.pathname.split('/')[2] || '');
            window.addEventListener('resize', () => this.measureTale());
            await this.load();
            if (new URLSearchParams(location.search).get('updated') === '1') {
                this.showMessage('Game updated', 'success');
                history.replaceState(null, '', location.pathname);
            }
            const deleted = new URLSearchParams(location.search).get('entry_deleted');
            if (deleted !== null) {
                this.showMessage(Number(deleted) > 0 ? `Entry for session ${Number(deleted)} deleted` : 'Entry deleted', 'success');
                history.replaceState(null, '', location.pathname);
            }
        },

        async load() {
            const seq = ++this.loadSeq; // an older response must not overwrite a newer one
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.gameId)}`);
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (seq !== this.loadSeq) return;
                if (response.status === 404) {
                    this.notFound = true;
                    this.loadError = '';
                    this.campaign = null;
                    return;
                }
                if (!response.ok) {
                    this.loadError = data.error || 'Failed to load game';
                    return;
                }
                this.campaign = data;
                if (!this.panelOpened) {
                    // Like the calendar: open on the first session awaiting a reply, otherwise on the company.
                    this.panelOpened = true;
                    this.selectedId = this.upcoming.find((s) => this.awaitsMe(s))?.id || null;
                } else if (this.selectedId && !this.selectedUpcoming && !this.selectedPlayed) {
                    this.selectedId = null;
                }
                this.notFound = false;
                this.loadError = '';
                this.$nextTick(() => this.measureTale());
                document.title = 'Waystone - ' + (this.campaign.title || 'Game');
            } catch (error) {
                if (seq === this.loadSeq) this.loadError = 'Could not load the game: ' + error.message;
            } finally {
                if (seq === this.loadSeq) this.loading = false;
            }
        },

        get isDM() {
            return !!this.currentUserId && this.campaign?.dm === this.currentUserId;
        },
        get isPlayer() {
            return (this.campaign?.players || []).includes(this.currentUserId);
        },
        get isMember() {
            return this.isDM || this.isPlayer;
        },
        get statusClass() {
            return campaignStatusClass(this.campaign?.status);
        },
        get typeInfo() {
            return CAMPAIGN_TYPES.find((t) => t.value === this.campaign?.type) || CAMPAIGN_TYPES.find((t) => t.value === 'Campaign');
        },
        get typeIconPaths() {
            return TYPE_ICON_PATHS[this.typeInfo.value] || '';
        },
        get descriptionHtml() {
            return renderMarkdown(this.campaign?.description);
        },
        get upcoming() {
            return this.campaign?.upcoming_sessions || [];
        },
        get played() {
            return this.campaign?.played_sessions || [];
        },
        // Entries written for sessions that were played; the cancelled ones are listed apart.
        get journalCount() {
            return this.played.filter((s) => s.journal).length;
        },
        get cancelledEntries() {
            return this.campaign?.journal_cancelled || [];
        },
        get visibleSessions() {
            return this.showAll ? this.played : this.played.slice(0, 5);
        },
        get selectedUpcoming() {
            return this.recording ? null : this.upcoming.find((s) => s.id === this.selectedId) || null;
        },
        get selectedPlayed() {
            return this.recording ? null : this.played.find((s) => s.id === this.selectedId) || null;
        },
        // x-for over a one-item list gives the panel a named session without repeating the getter.
        get selectedUpcomingList() {
            return this.selectedUpcoming ? [this.selectedUpcoming] : [];
        },
        get selectedPlayedList() {
            return this.selectedPlayed ? [this.selectedPlayed] : [];
        },
        // The session the viewer owes a reply on: the phone layout shows it right under the lede.
        get replySession() {
            return this.upcoming.find((s) => this.awaitsMe(s)) || null;
        },
        // The DM, then each player with how many played sessions they attended.
        get company() {
            const total = this.played.length;
            const attended = (id) => this.played.filter((s) => s.attendees.some((a) => a.id === id)).length;
            const rows = (this.campaign?.dm_user ? [{ user: this.campaign.dm_user, dm: true, detail: 'Dungeon master' }] : []);
            return rows.concat((this.campaign?.player_users || []).map((u) => ({
                user: u, dm: false, detail: total > 0 ? `${attended(u.id)} of ${total}` : ''
            })));
        },
        // Everyone who can be ticked in the record form: the DM, the players, and former players already on the session.
        get formPeople() {
            const list = this.campaign?.dm_user ? [this.campaign.dm_user] : [];
            return list.concat(this.campaign?.player_users || [], this.form.former);
        },
        get formTitle() {
            return this.editingId ? 'Edit a past session' : 'Record a past session';
        },
        get today() {
            const d = new Date();
            return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
        },
        get seatsLabel() {
            const max = parseInt(this.campaign?.desired_player_count, 10) || 0;
            const taken = (this.campaign?.players || []).length;
            return `${taken} / ${max > 0 ? max : '∞'} seats · sign-ups ${this.campaign?.sign_ups_open ? 'open' : 'closed'}`;
        },
        get signupStatus() {
            return this.campaign?.current_user_signup_status || '';
        },
        get unansweredCount() {
            return this.isPlayer ? (parseInt(this.campaign?.unanswered_sessions_count, 10) || 0) : 0;
        },
        get pendingNotice() {
            if (this.unansweredCount > 0) return this.unansweredCount === 1 ? 'A session awaits thy reply' : `${this.unansweredCount} sessions await thy reply`;
            if (this.pendingCount > 0) return `${this.pendingCount} sign-up${this.pendingCount === 1 ? '' : 's'} to review`;
            return '';
        },
        get pendingCount() {
            return this.isDM ? (parseInt(this.campaign?.pending_signups_count, 10) || 0) : 0;
        },

        // Dates are set in the text face: "Fri, Oct 30, 2026", "Fri, Nov 13", "Friday, Nov 20".
        formatDate(dateStr) {
            return parseDateLocal(dateStr).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' });
        },
        formatShort(dateStr) {
            return formatShortDate(dateStr);
        },
        formatHeading(dateStr, withYear = false) {
            return parseDateLocal(dateStr).toLocaleDateString('en-US', { weekday: 'long', month: 'short', day: 'numeric', ...(withYear ? { year: 'numeric' } : {}) });
        },
        // Fold the tale on phones only when it is longer than three lines.
        measureTale() {
            const el = document.getElementById('taleText');
            if (!el || !window.matchMedia('(max-width: 900px)').matches) { this.taleCanFold = false; return; }
            // Measure unfolded, then put the fold back
            const folded = el.classList.contains('is-folded');
            el.classList.remove('is-folded');
            const overflows = el.scrollHeight > parseFloat(getComputedStyle(el).lineHeight) * 3 + 2;
            if (folded) el.classList.add('is-folded');
            this.taleCanFold = overflows;
        },
        // Times use an en dash here; the calendar spec pins the hyphen form there.
        timeRange(s) {
            if (!s.time) return '';
            return s.duration > 0 ? `${s.time}–${getSessionEndTime(s)}` : s.time;
        },
        sessionMeta(s) {
            let meta = s.time ? timeWithZone(this.timeRange(s), s) : 'Time not recorded';
            if (s.time && s.duration > 0) meta += ` · ${getDurationLabel(s)}`;
            return meta;
        },
        upcomingTime(s) {
            return s.time ? timeWithZone(this.timeRange(s), s) : 'Time not recorded';
        },
        playedTime(s) {
            return s.time ? timeWithZone(this.timeRange(s), s) : 'Time not recorded';
        },
        // Ahead rows: "3 aye, 1 no reply" (members only; zero counts are left out).
        aheadCounts(s) {
            const c = getSessionCounts(s);
            return [[c.accepted, 'aye'], [c.tentative, 'perhaps'], [c.declined, 'nay'], [c.awaiting, 'no reply']]
                .filter(([n]) => n > 0).map(([n, word]) => `${n} ${word}`).join(', ');
        },
        // The viewer's own reply on an Ahead row, with a glyph so colour never carries it alone.
        aheadMine(s) {
            if (!this.isPlayer) return '';
            const p = s.current_user_participation;
            if (p === 'Accepted') return REPLY_GLYPHS.accepted + ' ' + replyWord(p);
            if (p === 'Tentative') return '? ' + replyWord(p);
            if (p === 'Declined') return REPLY_GLYPHS.declined + ' ' + replyWord(p);
            return this.awaitsMe(s) ? 'Thy reply?' : 'No reply yet';
        },
        aheadMineClass(s) {
            const p = s.current_user_participation;
            return p === 'Accepted' ? 'reply-accepted' : p === 'Tentative' ? 'reply-tentative' : p === 'Declined' ? 'reply-nay' : this.awaitsMe(s) ? 'is-you' : 'reply-none';
        },
        // Phone reply block: "18:00–22:00 CET, proposed by ♛ X. 1 aye, 3 no reply."
        replyBlockMeta(s) {
            const counts = this.aheadCounts(s);
            return `${this.upcomingTime(s)}, proposed by ♛ ${this.campaign?.dm_user?.display_name || 'the DM'}.${counts ? ' ' + counts + '.' : ''}`;
        },
        // The DM is marked in the name too, not by colour alone.
        attendeeName(u) {
            return (u.display_name || u.name || 'Unknown') + (u.id === this.campaign?.dm ? ' (DM)' : '');
        },
        rosterName(u) {
            return u.display_name || u.name || 'Unknown';
        },
        guestLetter(name) {
            return (name.match(/[\p{L}\p{N}]/u)?.[0] || '?').toUpperCase();
        },
        awaitsMe(s) {
            return this.isPlayer && s.status === 'Suggested' && !s.current_user_participation;
        },
        // Tally, candidate mark and roster are member-only: a visitor's data carries no replies.
        getTally(s) {
            return getTally(s);
        },
        isStrongCandidate(s) {
            return this.isMember && isStrongCandidate(s);
        },
        getCandidateNote(s) {
            return getCandidateNote(s);
        },
        replyGlyph(kind) {
            return REPLY_GLYPHS[kind] || '';
        },
        cellLabel(s) {
            const parts = [`${s.status} session`, this.formatShort(s.date)];
            if (s.time) parts.push(this.upcomingTime(s));
            if (this.isMember) {
                const c = getSessionCounts(s);
                parts.push(`${c.accepted} aye, ${c.tentative} perhaps, ${c.declined} nay, ${c.awaiting} no reply yet`);
            }
            if (this.isStrongCandidate(s)) parts.push('strong candidate');
            if (this.awaitsMe(s)) parts.push('awaiting your reply');
            return parts.join(', ');
        },
        journalUrl(sessionId) {
            return `/games/${encodeURIComponent(this.campaign.id)}/journal/${encodeURIComponent(sessionId)}`;
        },
        cancelledLabel(c) {
            return this.formatDate(c.date) + (c.name ? ': ' + c.name : '');
        },
        entryLabel(s) {
            return `Session ${s.number}, ${this.formatHeading(s.date, true)}, ${s.attendees.length} there`;
        },
        upcomingEyebrow(s) {
            if (this.campaign?.next_session?.id === s.id) return 'The next session';
            return s.status === 'Suggested' ? 'Suggested session' : 'Confirmed session';
        },
        upcomingMeta(s) {
            const duration = s.duration > 0 ? `${getDurationWords(s)}, ` : '';
            return `${duration}proposed by ♛ ${this.campaign?.dm_user?.display_name || 'the DM'}`;
        },
        playedMeta(s) {
            const duration = s.duration > 0 ? `${getDurationLabel(s)} · ` : '';
            const total = s.attendees.length + this.playedAbsent(s).length;
            let meta = `${duration}${s.attendees.length} of ${total} at the table`;
            if (s.manual) meta += ` · added by ♛ ${this.campaign?.dm_user?.display_name || 'the DM'}`;
            return meta;
        },
        // The DM and players who weren't at a played session.
        playedAbsent(s) {
            const there = new Set(s.attendees.filter((a) => a.id).map((a) => a.id));
            const everyone = (this.campaign?.dm_user ? [this.campaign.dm_user] : []).concat(this.campaign?.player_users || []);
            return everyone.filter((u) => !there.has(u.id));
        },
        upcomingRoster(s) {
            const kinds = { Accepted: 'accepted', Tentative: 'tentative', Declined: 'declined' };
            const order = ['accepted', 'tentative', 'declined', 'awaiting'];
            return (s.replies || [])
                .map((r) => {
                    const kind = kinds[r.participation] || 'awaiting';
                    // The awaited reply is the viewer's own: ask for it in rubric; others stay quiet
                    const you = kind === 'awaiting' && r.user.id === this.currentUserId && s.status === 'Suggested';
                    return { key: `${kind}-${r.user.id}`, user: r.user, label: you ? 'Thy reply?' : replyWord(r.participation), cls: `participation-${kind === 'awaiting' ? 'pending' : kind}`, kind, you };
                })
                .sort((a, b) => order.indexOf(a.kind) - order.indexOf(b.kind));
        },
        playedRoster(s) {
            const there = s.attendees.map((u, i) => ({ key: u.id || 'guest-' + i, user: u, label: 'There', cls: 'participation-accepted', kind: 'accepted' }));
            const absent = this.playedAbsent(s).map((u) => ({ key: 'absent-' + u.id, user: u, label: 'Absent', cls: 'participation-declined', kind: 'declined' }));
            return there.concat(absent);
        },

        pick(id) {
            this.selectedId = id;
            this.recording = false;
            this.revealPanel();
        },
        showCompany() {
            this.selectedId = null;
            this.focusPanel('.panel-heading');
        },
        // Move focus into the panel after it changes state because of a button the user pressed.
        focusPanel(selector) {
            this.$nextTick(() => document.querySelector('#gamePanel ' + selector)?.focus());
        },
        // On a stacked layout the panel sits below the list, so bring it into view.
        revealPanel() {
            if (window.matchMedia('(max-width: 900px)').matches) {
                this.$nextTick(() => document.querySelector('.game-panel-states')?.scrollIntoView({ block: 'start' }));
            }
        },

        async reply(participation, s) {
            await this.postAction(`/api/campaigns/${encodeURIComponent(this.campaign.id)}/sessions/${encodeURIComponent(s.id)}/responses`,
                'POST', 'Response submitted', 'Failed to submit response', { participation });
        },
        async setSessionStatus(status, s) {
            if (this.busy) return;
            const when = `${this.formatHeading(s.date)}${s.time ? ', ' + s.time : ''}`;
            const ok = await this.askConfirm(status === 'Confirmed'
                ? { title: 'Confirm session', message: `Confirm the session on ${when}? The players will be notified.`, confirmLabel: 'Confirm' }
                : { title: 'Cancel session', message: `Cancel the session on ${when}?`, confirmLabel: 'Cancel session' });
            if (!ok) return;
            await this.postAction(`/api/campaigns/${encodeURIComponent(this.campaign.id)}/sessions/${encodeURIComponent(s.id)}`,
                'PUT', 'Session updated', 'Failed to update session', { status });
        },

        startRecording() {
            this.editingId = '';
            this.form = emptyForm();
            this.form.attendees = this.formPeopleIds();
            this.form.timezone = getDefaultTimezone();
            this.recording = true;
            this.revealPanel();
            this.focusPanel('#recDate');
        },
        startEdit(s) {
            const campaignIds = new Set((this.campaign.dm_user ? [this.campaign.dm_user] : []).concat(this.campaign.player_users || []).map((p) => p.id));
            this.editingId = s.id;
            this.form = {
                ...emptyForm(),
                date: s.date,
                start: s.time || '',
                end: s.duration > 0 ? getSessionEndTime(s) : '',
                timezone: s.timezone || getDefaultTimezone(),
                attendees: s.attendees.filter((a) => a.id).map((a) => a.id),
                guests: s.attendees.filter((a) => a.guest).map((a) => a.display_name),
                former: s.attendees.filter((a) => a.id && !campaignIds.has(a.id))
            };
            this.recording = true;
            this.revealPanel();
            this.focusPanel('#recDate');
        },
        formPeopleIds() {
            return this.formPeople.map((p) => p.id);
        },
        cancelRecord() {
            this.recording = false;
            this.editingId = '';
            this.form = emptyForm();
            this.focusPanel('.panel-heading');
        },
        addGuest() {
            const name = this.form.guestDraft.trim().replace(/\s+/g, ' ');
            if (!name || this.form.guests.some((g) => g.toLowerCase() === name.toLowerCase())) {
                this.form.guestDraft = '';
                return;
            }
            if (this.form.guests.length >= 20) {
                this.form.error = 'At most 20 guests.';
                return;
            }
            this.form.guestDraft = '';
            this.form.error = '';
            this.form.guests.push(name);
        },
        removeGuest(i) {
            this.form.guests.splice(i, 1);
        },
        async savePlayed() {
            if (this.busy) return;
            const f = this.form;
            f.error = '';
            this.addGuest(); // a name typed but not yet added must not be lost
            if (f.error) return;
            f.error = !f.date ? 'Choose a date.'
                : f.date > this.today ? 'The date must not be in the future.'
                : f.end && !f.start ? 'An end time needs a start time.'
                : f.start && new Date(`${f.date}T${f.start}`) > new Date() ? 'The session must not be in the future.'
                : f.start && f.end && f.end === f.start ? 'The end time must differ from the start time.'
                : f.attendees.length === 0 && f.guests.length === 0 ? 'Choose at least one person who was there.' : '';
            if (f.error) return;
            this.busy = true;
            try {
                const base = `/api/campaigns/${encodeURIComponent(this.campaign.id)}/played-sessions`;
                const response = await fetch(this.editingId ? `${base}/${encodeURIComponent(this.editingId)}` : base, {
                    method: this.editingId ? 'PUT' : 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ date: f.date, start: f.start, end: f.end, timezone: f.timezone, attendee_ids: f.attendees, guests: f.guests })
                });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (!response.ok) {
                    const msg = data.error || 'Failed to save the session';
                    f.error = msg.charAt(0).toUpperCase() + msg.slice(1) + (msg.endsWith('.') ? '' : '.');
                    return;
                }
                this.recording = false;
                this.editingId = '';
                this.form = emptyForm();
                await this.load();
                this.selectedId = data.session.id;
                this.showAll = true;
                this.focusPanel('.panel-heading');
                this.showMessage(data.message ? data.message.charAt(0).toUpperCase() + data.message.slice(1) : 'Session saved', 'success');
            } catch (error) {
                this.showMessage(`Error: ${error.message}`, 'error');
            } finally {
                this.busy = false;
            }
        },
        async deletePlayed(s) {
            if (this.busy) return;
            const n = s.journal?.revisions || 0;
            const entry = s.journal ? ` Its journal entry is deleted too, with ${n} revision${n === 1 ? '' : 's'} of text, including text written by other people.` : '';
            const ok = await this.askConfirm({
                title: 'Delete session',
                message: `Delete the session of ${this.formatDate(s.date)}?${entry} This cannot be undone.`,
                confirmLabel: 'Delete'
            });
            if (!ok) return;
            const done = await this.postAction(`/api/campaigns/${encodeURIComponent(this.campaign.id)}/played-sessions/${encodeURIComponent(s.id)}`,
                'DELETE', 'Session deleted', 'Failed to delete session');
            if (done) this.focusPanel('.panel-heading');
        },
        async postAction(url, method, fallback, errorPrefix, body, successText) {
            if (this.busy) return false;
            this.busy = true;
            try {
                const response = await fetch(url, body
                    ? { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
                    : { method });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || errorPrefix, 'error');
                } else {
                    const msg = successText || data.message || fallback;
                    this.showMessage(msg.charAt(0).toUpperCase() + msg.slice(1), 'success');
                }
                await this.load();
                return response.ok;
            } catch (error) {
                this.showMessage(`Error: ${error.message}`, 'error');
            } finally {
                this.busy = false;
            }
        },

        async signUp() {
            await this.postAction(`/api/campaigns/${encodeURIComponent(this.campaign.id)}/signups`, 'POST', 'Signed up successfully', 'Failed to sign up');
        },

        async toggleFollow() {
            const following = !!this.campaign.is_following;
            const title = this.campaign.title;
            await this.postAction(`/api/campaigns/${encodeURIComponent(this.campaign.id)}/follow`, following ? 'DELETE' : 'PUT', following ? 'Unfollowed' : 'Following', 'Failed to update follow', undefined,
                following ? `No longer following ${title}` : `Now following ${title}`);
        },

        async withdraw() {
            if (this.busy) return;
            const leaving = this.isPlayer;
            const title = this.campaign.title;
            const ok = await this.askConfirm(leaving
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
            await this.postAction(`/api/campaigns/${encodeURIComponent(this.campaign.id)}/signups/${encodeURIComponent(this.currentUserId)}`, 'DELETE', 'Sign-up withdrawn', 'Failed to withdraw');
        },

        async deleteGame() {
            if (this.busy) return;
            const ok = await this.askConfirm({
                title: 'Delete game',
                message: `Delete "${this.campaign.title}"? This also deletes its sessions and sign-ups and cannot be undone.`,
                confirmLabel: 'Delete'
            });
            if (!ok || this.busy) return;
            this.busy = true;
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.campaign.id)}`, { method: 'DELETE' });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to delete game', 'error');
                    return;
                }
                window.location.href = '/campaigns'; // the game page no longer exists
            } catch (error) {
                this.showMessage('Error deleting game: ' + error.message, 'error');
            } finally {
                this.busy = false;
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
