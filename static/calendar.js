// Calendar page (/games/{id}/calendar). Relies on campaigns.js (loaded first) for the top-level
// helpers: REPLY_GLYPHS, userAvatarHtml, parseDateLocal, getSessionEndTime, getSessionCounts, getTally,
// isStrongCandidate, getCandidateNote, getDurationLabel, getDefaultTimezone and getTimezoneOptions, and for the poll
// helpers groupPolls, pollRangeLabel, pollConfirmWarning and sessionRoster. It does not use campaignsPage().

// Choices for a poll date's length, in minutes (a copied length outside the list is added to its row's select).
const LENGTH_OPTIONS = [60, 90, 120, 150, 180, 210, 240, 270, 300, 360, 420, 480];
const MAX_POLL_DATES = 31;

window.calendarPage = function () {
    return {
        displayName: 'User',
        isAdmin: false,
        currentUserId: '',
        campaign: null,
        loading: true,
        notFound: false,
        forbidden: false, // signed in, but neither the DM nor a player of this game
        loadError: '',
        messageTimer: null,
        messageVisible: false,
        messageText: '',
        messageType: '',
        currentCalendarDate: new Date(),
        currentCampaignId: '',
        currentCampaignTitle: '',
        isCurrentUserDM: false,
        calendarDays: [],
        sessions: [],
        sessionMap: {},
        selectedDay: null,
        pendingSessionAction: '',
        selectedSession: null,
        timezoneOptions: getTimezoneOptions(),
        sessionResponseCache: {},
        // Poll picking: the DM clicks days across months, then "Next" opens the dialog with a row per day.
        picking: null, // null, or { mode: 'new' | 'add', pollId, preset: { time, duration, timezone } | null }
        pickSelection: {}, // YYYY-MM-DD -> true
        pickNote: { text: '', type: '' },
        showPollDialog: false,
        pollRows: [], // { date, time, duration }
        pollTimezone: '',
        pollSubmitting: false,
        pollError: '',
        pollInfo: {}, // pollId -> { id, dates (upcoming, by start), multi, letter }
        hoverPollId: '',
        hoverDay: '',
        pendingPollWithdraw: '',
        ...repeatDialogMixin(),
        sessionsMonth: '',
        sessionsLoadSeq: 0,

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
            this.currentCampaignId = decodeURIComponent(location.pathname.split('/')[2] || '');
            await this.load();
        },

        async load() {
            this.loading = true;
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.currentCampaignId)}`);
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (response.status === 404) {
                    this.notFound = true;
                    this.loadError = '';
                    return;
                }
                if (!response.ok) {
                    this.loadError = data.error || 'Failed to load game';
                    return;
                }
                this.campaign = data;
                this.notFound = false;
                this.loadError = '';
                this.currentCampaignTitle = data.title || '';
                this.isCurrentUserDM = !!this.currentUserId && data.dm === this.currentUserId;
                document.title = 'Waystone - ' + (data.title || 'Game') + ' calendar';
                // The sessions API only answers the DM and the players (403 for anyone else).
                this.forbidden = !this.isCurrentUserDM && !(data.players || []).includes(this.currentUserId);
                if (this.forbidden) return;
                const params = new URLSearchParams(location.search);
                await this.openCalendar(params.get('date') || data.first_unanswered_session_date || '');
                // "+ Add a date" on the game page lands here with ?add={pollId}
                const addTo = params.get('add');
                if (addTo && this.isCurrentUserDM && this.pollInfo[addTo]) this.addPollDate(this.pollInfo[addTo]);
            } catch (error) {
                this.loadError = 'Could not load the game: ' + error.message;
            } finally {
                this.loading = false;
            }
        },

        // Sets the calendar up once the game is loaded: opens on the month of the first
        // unanswered session (with that day selected), or on the current month.
        async openCalendar(focusDate = '') {
            const focus = /^\d{4}-\d{2}-\d{2}$/.test(focusDate) ? this.parseDateLocal(focusDate) : null;
            this.currentCalendarDate = focus ? new Date(focus.getFullYear(), focus.getMonth(), 1) : new Date();
            this.selectedDay = null;
            this.selectedSession = null;
            this.pendingSessionAction = '';
            this.pendingPollWithdraw = '';
            this.sessionResponseCache = {};
            this.cancelPicking();
            this.generateCalendarDays();
            await this.loadSessions();
            if (focus) {
                await this.selectDay(focus);
            }
        },

        generateCalendarDays() {
            const year = this.currentCalendarDate.getFullYear();
            const month = this.currentCalendarDate.getMonth();
            const firstDay = new Date(year, month, 1);
            const lastDay = new Date(year, month + 1, 0);

            let startDay = firstDay.getDay() - 1;
            if (startDay < 0) startDay = 6;

            const days = [];
            for (let i = 0; i < startDay; i++) {
                const prevDate = new Date(firstDay);
                prevDate.setDate(prevDate.getDate() - (startDay - i));
                days.push(prevDate);
            }

            for (let d = 1; d <= lastDay.getDate(); d++) {
                days.push(new Date(year, month, d));
            }

            const remaining = 42 - days.length;
            for (let i = 0; i < remaining; i++) {
                const nextDate = new Date(lastDay);
                nextDate.setDate(nextDate.getDate() + i + 1);
                days.push(nextDate);
            }
            this.calendarDays = days;
        },

        formatMonthYear() {
            return this.currentCalendarDate.toLocaleString('en-US', { month: 'long', year: 'numeric' });
        },

        // The month header sets the name in the display face and the year in the text face.
        formatMonthName() {
            return this.currentCalendarDate.toLocaleString('en-US', { month: 'long' });
        },

        formatYear() {
            return String(this.currentCalendarDate.getFullYear());
        },

        previousMonth() {
            this.clearSelectedDay();
            this.currentCalendarDate = new Date(this.currentCalendarDate.getFullYear(), this.currentCalendarDate.getMonth() - 1, 1);
            this.generateCalendarDays();
            this.loadSessions();
        },

        nextMonth() {
            this.clearSelectedDay();
            this.currentCalendarDate = new Date(this.currentCalendarDate.getFullYear(), this.currentCalendarDate.getMonth() + 1, 1);
            this.generateCalendarDays();
            this.loadSessions();
        },

        isCurrentMonth(date) {
            return date.getMonth() === this.currentCalendarDate.getMonth() && date.getFullYear() === this.currentCalendarDate.getFullYear();
        },

        formatDateLocal(date) {
            const year = date.getFullYear();
            const month = String(date.getMonth() + 1).padStart(2, '0');
            const day = String(date.getDate()).padStart(2, '0');
            return `${year}-${month}-${day}`;
        },

        async loadSessions() {
            const year = this.currentCalendarDate.getFullYear();
            const month = String(this.currentCalendarDate.getMonth() + 1).padStart(2, '0');
            const seq = ++this.sessionsLoadSeq;
            try {
                const response = await fetch(`/api/campaigns/${this.currentCampaignId}/sessions?month=${year}-${month}`);
                if (!response.ok) {
                    throw new Error('Failed to load sessions');
                }
                const data = await response.json();
                if (seq !== this.sessionsLoadSeq) return;
                this.sessions = Array.isArray(data) ? data : [];
                this.buildSessionMap();
                this.sessionsMonth = `${year}-${month}`;
                this.buildPollInfo();
            } catch (error) {
                if (seq !== this.sessionsLoadSeq) return;
                this.showMessage('Error loading sessions: ' + error.message, 'error');
                this.sessions = [];
                this.sessionMap = {};
                this.sessionsMonth = '';
            }
        },

        // The game's own data holds every open poll across months, with replies; the month view holds only one month.
        async refreshCampaign() {
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.currentCampaignId)}`);
                if (!response.ok) return;
                const data = await response.json();
                this.campaign = data;
                this.buildPollInfo();
            } catch (error) {
                console.error('Could not refresh the game:', error);
            }
        },

        // After a reply, confirm, withdraw or new poll: both views change.
        async refresh() {
            await Promise.all([this.loadSessions(), this.refreshCampaign()]);
            await this.loadDayResponses();
        },

        // The Repeat dialog saved: a schedule can post a poll at once.
        async repeatSaved() {
            await this.refresh();
        },

        // Open polls from the game's upcoming Suggested dates. Letters (A, B, ...) go to polls of several dates
        // that have a date in the shown month, ordered by that date; they are per month view, never stored.
        buildPollInfo() {
            const ym = `${this.currentCalendarDate.getFullYear()}-${String(this.currentCalendarDate.getMonth() + 1).padStart(2, '0')}`;
            const byStart = (a, b) => `${a.date} ${a.time || ''}`.localeCompare(`${b.date} ${b.time || ''}`);
            const info = {};
            for (const [id, list] of Object.entries(groupPolls(this.campaign?.upcoming_sessions))) {
                const dates = [...list].sort(byStart);
                info[id] = { id, dates, multi: dates.length > 1, letter: '' };
            }
            Object.values(info)
                .filter((p) => p.multi && p.dates.some((d) => d.date.startsWith(ym)))
                .sort((a, b) => byStart(a.dates.find((d) => d.date.startsWith(ym)), b.dates.find((d) => d.date.startsWith(ym))))
                .forEach((p, i) => { p.letter = i < 26 ? String.fromCharCode(65 + i) : String(i + 1); });
            this.pollInfo = info;
        },

        // The poll a session belongs to while it is one of its upcoming dates (not a past suggestion or a mocked one).
        pollOf(session) {
            const info = session?.status === 'Suggested' ? this.pollInfo[session.poll_id] : null;
            return info && info.dates.some((d) => d.id === session.id) ? info : null;
        },

        buildSessionMap() {
            this.sessionMap = {};
            for (const session of this.sessions) {
                const dateStr = session.date;
                if (!this.sessionMap[dateStr]) {
                    this.sessionMap[dateStr] = { all: [], planned: [] };
                }
                this.sessionMap[dateStr].all.push(session);
                if (session.status !== 'Cancelled') {
                    this.sessionMap[dateStr].planned.push(session);
                }
            }
            for (const dateStr in this.sessionMap) {
                const primary = this.getEarliestSession(this.sessionMap[dateStr].planned);
                this.sessionMap[dateStr].primary = primary || this.getEarliestSession(this.sessionMap[dateStr].all);
            }
        },

        hasSession(date) {
            return this.getSessionsForDay(date).length > 0;
        },

        hasPlannedSession(date) {
            return this.getPlannedSessionsForDay(date).length > 0;
        },

        isHistoricalDay(date) {
            return this.hasSession(date) && !this.hasPlannedSession(date);
        },

        getSessionsForDay(date) {
            const dateStr = this.formatDateLocal(date);
            return this.sessionMap[dateStr]?.all || [];
        },

        getPlannedSessionsForDay(date) {
            const dateStr = this.formatDateLocal(date);
            return this.sessionMap[dateStr]?.planned || [];
        },

        getEarliestSession(sessions) {
            if (!Array.isArray(sessions) || sessions.length === 0) return null;
            return [...sessions].sort((a, b) => String(a.time || '').localeCompare(String(b.time || '')))[0];
        },

        getPrimarySessionForDay(date) {
            const dateStr = this.formatDateLocal(date);
            return this.sessionMap[dateStr]?.primary || null;
        },

        getDayStatusClass(date) {
            const primarySession = this.getPrimarySessionForDay(date);
            if (!primarySession) return '';
            if (primarySession.status === 'Suggested') return 'day-suggested';
            if (primarySession.status === 'Confirmed') return 'day-confirmed';
            if (primarySession.status === 'Cancelled') return 'day-cancelled';
            return 'day-historical';
        },

        getDayTitle(date) {
            if (this.picking && this.isCurrentMonth(date)) return this.getPickDayLabel(date);
            const summary = this.getDaySummary(date);
            if (!this.isHistoricalDay(date)) return summary;
            return `${summary}. ${this.isCurrentUserDM
                ? 'Previously planned session is inactive. Click to suggest a new one.'
                : 'Previously planned session is inactive.'}`;
        },

        getDayAriaLabel(date) {
            if (this.picking && this.isCurrentMonth(date)) return this.getPickDayLabel(date);
            const summary = this.getDaySummary(date);
            if (!summary) return '';
            const head = this.formatDayHeading(date);
            if (!this.isHistoricalDay(date)) return `${head}. ${summary}`;
            return `${head}. ${summary}. ${this.isCurrentUserDM
                ? 'Previously planned session is inactive. Select this day to suggest a new session.'
                : 'Previously planned session is inactive.'}`;
        },

        getPickDayLabel(date) {
            const head = this.formatDayHeading(date);
            if (this.pickSelection[this.formatDateLocal(date)]) return `Remove ${head} from the poll`;
            return this.getPickBlockReason(date) || `Add ${head} to the poll`;
        },

        getSessionEndTime(session) {
            return getSessionEndTime(session);
        },

        // The hyphen form is for accessible names (the e2e tests pin it); the panel shows getSessionTimeRangeZone.
        getSessionTimeRange(session) {
            const startTime = session?.time || '';
            const endTime = this.getSessionEndTime(session);
            return `${startTime}-${endTime}`;
        },

        // "18:00–22:00 CET"
        getSessionTimeRangeZone(session) {
            return sessionRangeWithZone(session);
        },

        getSessionCounts(session) {
            return getSessionCounts(session);
        },

        getTally(session) {
            return getTally(session);
        },

        isStrongCandidate(session) {
            return isStrongCandidate(session);
        },

        getCandidateNote(session) {
            return getCandidateNote(session);
        },

        getDaySummary(date) {
            const s = this.getPrimarySessionForDay(date);
            if (!s) return '';
            const parts = [`${s.status} ${this.getSessionTimeRange(s)}`];
            if (s.status !== 'Cancelled') {
                const c = this.getSessionCounts(s);
                parts[0] += `: ${c.accepted} yes, ${c.tentative} maybe, ${c.declined} no, ${c.awaiting} no reply yet`;
            }
            const tag = this.getPollTag(date);
            if (tag) parts.push(`Poll ${tag.letter}, date ${tag.n} of ${tag.of}`);
            if (this.isStrongCandidate(s)) parts.push('Strong candidate');
            const more = this.getCellOverflow(date);
            if (more > 0) parts.push(`+${more} more session${more === 1 ? '' : 's'}`);
            return parts.join('. ');
        },

        replyGlyph(kind) {
            return REPLY_GLYPHS[kind] || '';
        },

        // {letter, n, of} when the day's session is an upcoming date of a poll with several dates.
        getPollTag(date) {
            const s = this.getPrimarySessionForDay(date);
            const info = this.pollOf(s);
            if (!info || !info.multi || !info.letter) return null;
            return { letter: info.letter, n: info.dates.findIndex((d) => d.id === s.id) + 1, of: info.dates.length };
        },

        pollIdOfDay(date) {
            const info = this.pollOf(this.getPrimarySessionForDay(date));
            return info && info.multi ? info.id : '';
        },

        // The other dates of the poll under the pointer or the selected day get an outline.
        isPollSibling(date) {
            if (this.picking) return false;
            const id = this.pollIdOfDay(date);
            if (!id) return false;
            const key = this.formatDateLocal(date);
            if (this.isSelectedDay(date) || key === this.hoverDay) return false;
            return id === this.hoverPollId || (!!this.selectedDay && id === this.pollIdOfDay(this.selectedDay));
        },

        getRoster(session) {
            // Poll dates come with their replies; other sessions load them per day.
            if (session.replies) return sessionRoster(session, this.currentUserId);
            return this.getResponseGroups(session).flatMap((g) => g.users.map((user, i) => {
                // The awaited reply is the viewer's own: ask for it in rubric; others stay quiet
                const you = g.kind === 'awaiting' && !!user.id && user.id === this.currentUserId && session.status === 'Suggested';
                return { key: `${g.kind}-${user.id || i}`, user, label: you ? 'Your reply?' : g.label, cls: g.cls, kind: g.kind, you };
            }));
        },

        avatarHtml(user, link = true) {
            return userAvatarHtml(user, link);
        },

        async getSessionResponseDetails(sessionId) {
            if (!sessionId) {
                return { all: [], pending: [] };
            }
            if (this.sessionResponseCache[sessionId]) {
                return this.sessionResponseCache[sessionId];
            }

            const response = await fetch(`/api/campaigns/${this.currentCampaignId}/sessions/${sessionId}/responses`);
            if (!response.ok) {
                throw new Error('Failed to load responses');
            }
            const payload = await response.json();
            const details = {
                all: Array.isArray(payload.all) ? payload.all : [],
                pending: Array.isArray(payload.pending) ? payload.pending : []
            };
            this.sessionResponseCache = {
                ...this.sessionResponseCache,
                [sessionId]: details
            };
            return details;
        },

        async selectDay(date) {
            if (!this.isCurrentMonth(date)) return;
            this.selectedDay = date;
            this.selectedSession = null;
            this.pendingSessionAction = '';
            this.pendingPollWithdraw = '';
            for (const session of this.getSelectedDaySessions()) {
                this.invalidateSessionCache(session.id);
            }
            if (window.matchMedia('(max-width: 768px)').matches) {
                this.$nextTick(() => {
                    if (this.$refs.dayPanel) {
                        this.$refs.dayPanel.scrollIntoView({ behavior: 'smooth', block: 'start' });
                    }
                });
            }
            await this.loadDayResponses();
        },

        // ---- Poll picking: choose days on the month, then set the times in a dialog ----

        // preset is { time, duration, timezone } from a copied session or a poll's first date; date is picked at once.
        startPicking({ mode = 'new', pollId = '', preset = null, date = null } = {}) {
            if (!this.isCurrentUserDM) return;
            // Read refs first: clearing the selected day removes the button that was clicked, and $refs resolves from it.
            const { pickCancel, calendarMain, dayPanel } = this.$refs;
            this.clearSelectedDay();
            this.picking = { mode, pollId, preset };
            this.pickSelection = {};
            this.pickNote = { text: '', type: '' };
            this.pollRows = [];
            this.pollTimezone = '';
            this.pollError = '';
            if (date) this.togglePickDay(date);
            this.$nextTick(() => {
                // x-show reveals the bar a frame later, and a hidden button can't take focus.
                requestAnimationFrame(() => setTimeout(() => {
                    if (dayPanel) dayPanel.scrollTop = 0;
                    pickCancel?.focus({ preventScroll: true });
                    if (window.matchMedia('(max-width: 768px)').matches) {
                        calendarMain?.scrollIntoView({ behavior: 'smooth', block: 'start' });
                    }
                }));
            });
        },

        cancelPicking() {
            this.picking = null;
            this.pickSelection = {};
            this.pickNote = { text: '', type: '' };
            this.showPollDialog = false;
            this.pollRows = [];
            this.pollTimezone = '';
            this.pollError = '';
        },

        // "+ Suggest session" on the selected day: picking with that day already picked.
        openCreateSession(date) {
            this.startPicking({ mode: 'new', date });
        },

        // Copy: a new poll that starts from this session's time, length and zone.
        copySession(session) {
            if (!session) return;
            this.startPicking({
                mode: 'new',
                preset: { time: session.time, duration: Number(session.duration) || 240, timezone: session.timezone || getDefaultTimezone() }
            });
        },

        addPollDate(info) {
            const first = info?.dates[0];
            if (!first) return;
            this.startPicking({
                mode: 'add',
                pollId: info.id,
                preset: { time: first.time, duration: Number(first.duration) || 240, timezone: first.timezone || getDefaultTimezone() }
            });
        },

        getPickLabel() {
            if (this.picking?.mode !== 'add') return 'Suggest dates';
            const letter = this.pollInfo[this.picking.pollId]?.letter;
            return letter ? `Add dates to poll ${letter}` : 'Add dates to the poll';
        },

        getPickBlockReason(date) {
            const year = this.currentCalendarDate.getFullYear();
            const month = String(this.currentCalendarDate.getMonth() + 1).padStart(2, '0');
            if (this.sessionsMonth !== `${year}-${month}`) return "This month's sessions have not loaded yet";
            const key = this.formatDateLocal(date);
            if (key < this.formatDateLocal(new Date())) return 'That day is in the past';
            if (this.picking?.mode === 'add' && this.pollInfo[this.picking.pollId]?.dates.some((d) => d.date === key)) return 'That day is already in this poll';
            if (this.hasPlannedSession(date)) return 'That day already has a session';
            return '';
        },

        handleDayActivate(date) {
            if (this.picking) {
                this.togglePickDay(date);
            } else {
                this.selectDay(date);
            }
        },

        togglePickDay(date) {
            if (!this.picking || !this.isCurrentMonth(date) || this.pollSubmitting) return;
            const key = this.formatDateLocal(date);
            if (this.pickSelection[key]) {
                const next = { ...this.pickSelection };
                delete next[key];
                this.pickSelection = next;
                this.pickNote = { text: '', type: '' };
                return;
            }
            const reason = this.getPickBlockReason(date) || (this.pickCount >= MAX_POLL_DATES ? `A poll can have up to ${MAX_POLL_DATES} dates` : '');
            if (reason) {
                this.pickNote = { text: reason, type: 'error' };
                return;
            }
            this.pickSelection = { ...this.pickSelection, [key]: true };
            this.pickNote = { text: '', type: '' };
        },

        get pickedDates() {
            return Object.keys(this.pickSelection).sort();
        },

        get pickCount() {
            return this.pickedDates.length;
        },

        getPickNote() {
            return this.pickNote.text || (this.pickCount ? pluralCount(this.pickCount, 'day') + ' picked' : 'Choose the days to offer.');
        },

        // ---- The "Suggest dates" dialog ----

        // Next: one row per picked day. Rows already edited keep their values; a new row takes the first row's time.
        openPollDialog() {
            if (!this.picking || this.pickCount === 0) return;
            const preset = this.picking.preset;
            const first = this.pollRows[0] || (preset ? { time: preset.time, duration: preset.duration } : { time: '19:00', duration: 240 });
            const kept = Object.fromEntries(this.pollRows.map((r) => [r.date, r]));
            this.pollRows = this.pickedDates.map((date) => kept[date] || { date, time: first.time, duration: first.duration });
            this.pollTimezone = this.pollTimezone || preset?.timezone || getDefaultTimezone();
            this.pollError = '';
            this.showPollDialog = true;
            this.$nextTick(() => document.querySelector('#pollDialog .poll-row input[type="time"]')?.focus());
        },

        // Closes the dialog and keeps the picks.
        backToCalendar() {
            if (this.pollSubmitting) return;
            this.showPollDialog = false;
            this.pollError = '';
            this.$nextTick(() => this.$refs.pickNext?.focus({ preventScroll: true }));
        },

        // The dialog's Cancel and x: leave picking altogether.
        closePollDialog() {
            if (this.pollSubmitting) return;
            this.cancelPicking();
        },

        removePollRow(i) {
            const row = this.pollRows[i];
            if (!row) return;
            const next = { ...this.pickSelection };
            delete next[row.date];
            this.pickSelection = next;
            this.pollRows = this.pollRows.filter((_, j) => j !== i);
            if (this.pollRows.length === 0) this.backToCalendar();
        },

        useFirstRowForAll() {
            const first = this.pollRows[0];
            if (!first) return;
            this.pollRows = this.pollRows.map((r) => ({ ...r, time: first.time, duration: first.duration }));
        },

        lengthOptions(row) {
            return LENGTH_OPTIONS.includes(row.duration) ? LENGTH_OPTIONS : [...LENGTH_OPTIONS, row.duration].sort((a, b) => a - b);
        },

        lengthLabel(minutes) {
            return getDurationLabel({ duration: minutes });
        },

        get pollSubmitLabel() {
            const verb = this.picking?.mode === 'add' ? 'Add' : 'Suggest';
            if (this.pollSubmitting) return verb === 'Add' ? 'Adding…' : 'Suggesting…';
            return `${verb} ${pluralCount(this.pollRows.length, 'date')}`;
        },

        async submitPoll() {
            if (this.pollSubmitting || !this.picking || this.pollRows.length === 0) return;
            const missing = this.pollRows.find((r) => !r.time);
            if (missing) {
                this.pollError = `Set a start time for ${formatShortDate(missing.date)}.`;
                return;
            }
            const add = this.picking.mode === 'add';
            const base = `/api/campaigns/${encodeURIComponent(this.currentCampaignId)}/polls`;
            this.pollSubmitting = true;
            this.pollError = '';
            try {
                const response = await fetch(add ? `${base}/${encodeURIComponent(this.picking.pollId)}/dates` : base, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        timezone: this.pollTimezone,
                        dates: this.pollRows.map((r) => ({ date: r.date, time: r.time, duration: Number(r.duration) }))
                    })
                });
                const data = await response.json().catch(() => ({}));
                if (!response.ok) {
                    const msg = data.error || 'Failed to suggest the dates';
                    this.pollError = msg.charAt(0).toUpperCase() + msg.slice(1) + (msg.endsWith('.') ? '' : '.');
                    return;
                }
                const n = Array.isArray(data.sessions) ? data.sessions.length : this.pollRows.length;
                this.cancelPicking();
                await this.refresh();
                this.showMessage(`${add ? 'Added' : 'Suggested'} ${pluralCount(n, 'date')}`, 'success');
            } catch (error) {
                this.pollError = 'Error: ' + error.message;
            } finally {
                this.pollSubmitting = false;
            }
        },

        async loadDayResponses() {
            try {
                // Poll dates carry their replies in the game's data
                await Promise.all(this.getSelectedDaySessions().filter((session) => !this.pollOf(session)).map((session) => this.getSessionResponseDetails(session.id)));
            } catch (error) {
                this.showMessage('Error loading responses: ' + error.message, 'error');
            }
        },

        invalidateSessionCache(sessionId) {
            const next = { ...this.sessionResponseCache };
            delete next[sessionId];
            this.sessionResponseCache = next;
        },

        isSelectedDay(date) {
            return !!this.selectedDay && this.formatDateLocal(date) === this.formatDateLocal(this.selectedDay);
        },

        isToday(date) {
            return this.formatDateLocal(date) === this.formatDateLocal(new Date());
        },

        sortedByTime(sessions) {
            return [...sessions].sort((a, b) => String(a.time || '').localeCompare(String(b.time || '')));
        },

        getCellSessions(date) {
            const s = this.getPrimarySessionForDay(date);
            return s ? [s] : [];
        },

        getCellOverflow(date) {
            return Math.max(0, this.getSessionsForDay(date).length - 1);
        },

        getSelectedDaySessions() {
            return this.selectedDay ? this.sortedByTime(this.getSessionsForDay(this.selectedDay)) : [];
        },

        parseDateLocal(dateStr) {
            return parseDateLocal(dateStr);
        },

        // "Friday, Nov 20": dates are set in the text face, never the display face
        formatDayHeading(date) {
            return date.toLocaleDateString('en-US', { weekday: 'long', month: 'short', day: 'numeric' });
        },

        formatAgendaDate(session) {
            return formatShortDate(session.date);
        },

        getDurationLabel(session) {
            return getDurationLabel(session);
        },

        getSessionMeta(session) {
            const dm = this.campaign?.dm_user?.display_name || 'the DM';
            return `${getDurationWords(session)}, proposed by ♛ ${dm}`;
        },

        getResponseGroups(session) {
            const details = this.sessionResponseCache[session.id];
            if (!details) return [];
            const byStatus = (participation) => details.all
                .filter((entry) => entry?.participation === participation)
                .map((entry) => entry.user || { display_name: 'Unknown' });
            return [
                { kind: 'accepted', label: replyWord('Accepted'), cls: 'participation-accepted', users: byStatus('Accepted') },
                { kind: 'tentative', label: replyWord('Tentative'), cls: 'participation-tentative', users: byStatus('Tentative') },
                { kind: 'declined', label: replyWord('Declined'), cls: 'participation-declined', users: byStatus('Declined') },
                { kind: 'awaiting', label: replyWord(''), cls: 'participation-pending', users: details.pending }
            ];
        },

        clearSelectedDay() {
            this.selectedDay = null;
            this.selectedSession = null;
            this.pendingSessionAction = '';
            this.pendingPollWithdraw = '';
        },

        // The selected day's cards. A date of an open poll opens the whole poll (every upcoming date, from the game's
        // own data, across months); anything else (Confirmed, Cancelled, past suggestions) is a one-session card.
        getPanelGroups() {
            const groups = [];
            const seen = {};
            for (const s of this.getSelectedDaySessions()) {
                const info = this.pollOf(s);
                if (!info) {
                    groups.push({ key: s.id, poll: null, sessions: [s], selectedId: s.id });
                } else if (!seen[info.id]) {
                    seen[info.id] = true;
                    groups.push({ key: 'poll:' + info.id, poll: info, sessions: info.dates, selectedId: s.id });
                }
            }
            return groups;
        },

        // A Suggested date whose start has passed: it dropped out of its poll and takes no replies or confirmation.
        isPastSuggestion(session) {
            return session.status === 'Suggested' && !this.pollOf(session) && session.date <= this.formatDateLocal(new Date());
        },

        // The viewer owes a reply on this date.
        awaitsMe(session) {
            return !this.isCurrentUserDM && session.status === 'Suggested' && !session.current_user_participation && !this.isPastSuggestion(session);
        },

        pollGroupAwaitsMe(group) {
            return !!group.poll?.multi && group.sessions.some((s) => this.awaitsMe(s));
        },

        pollHeading(info) {
            return `Poll ${info.letter}`.trim() + ` · ${pluralCount(info.dates.length, 'date')} · ${pollRangeLabel(info.dates)}`;
        },

        // Month agenda: each poll of several dates is one row, at its first date in the month.
        get agendaRows() {
            const rows = [];
            const seen = {};
            for (const s of this.sessions) {
                const info = this.pollOf(s);
                if (info && info.multi) {
                    if (seen[info.id]) continue;
                    seen[info.id] = true;
                    rows.push({ key: 'poll:' + info.id, date: s.date, text: this.pollHeading(info), status: 'Suggested', cls: 'chip-Suggested' });
                } else {
                    rows.push({ key: s.id, date: s.date, text: this.getSessionTimeRangeZone(s), status: s.status, cls: `chip-${s.status}` });
                }
            }
            return rows;
        },

        async requestSessionStatusChange(status, session) {
            if (!session) return;
            if (status === 'Confirmed' && session.status !== 'Suggested') {
                this.showMessage('Only suggested sessions can be confirmed', 'error');
                return;
            }
            this.pendingPollWithdraw = '';

            if (this.selectedSession?.id !== session.id || this.pendingSessionAction !== status) {
                this.selectedSession = session;
                this.pendingSessionAction = status;
                return;
            }

            this.pendingSessionAction = '';
            this.selectedSession = null;
            await this.updateSessionStatus(status, session);
        },

        async updateSessionStatus(status, session) {
            if (!session) return;
            const withdrawing = status === 'Cancelled' && session.status === 'Suggested';
            try {
                const response = await fetch(`/api/campaigns/${this.currentCampaignId}/sessions/${session.id}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ status })
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || `Failed to update session to ${status}`, 'error');
                    return;
                }
                this.showMessage(withdrawing ? 'Date withdrawn' : `Session ${status.toLowerCase()} successfully`, 'success');
                this.invalidateSessionCache(session.id);
                await this.refresh();
            } catch (error) {
                this.showMessage('Error updating session: ' + error.message, 'error');
            }
        },

        // Withdraw the poll: two steps, like Confirm.
        async requestPollWithdraw(info) {
            if (!info) return;
            if (this.pendingPollWithdraw !== info.id) {
                this.pendingPollWithdraw = info.id;
                this.pendingSessionAction = '';
                this.selectedSession = null;
                return;
            }
            this.pendingPollWithdraw = '';
            try {
                const response = await fetch(`/api/campaigns/${this.currentCampaignId}/polls/${encodeURIComponent(info.id)}`, { method: 'DELETE' });
                const data = await response.json().catch(() => ({}));
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to withdraw the poll', 'error');
                    return;
                }
                this.showMessage('Poll withdrawn', 'success');
                await this.refresh();
            } catch (error) {
                this.showMessage('Error withdrawing the poll: ' + error.message, 'error');
            }
        },

        async submitResponse(participation, session) {
            if (!session) return;
            this.pendingSessionAction = '';
            this.pendingPollWithdraw = '';
            this.selectedSession = null;
            try {
                const response = await fetch(`/api/campaigns/${this.currentCampaignId}/sessions/${session.id}/responses`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ participation })
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to submit response', 'error');
                    return;
                }
                this.invalidateSessionCache(session.id);
                await this.refresh();
                this.showMessage('Response submitted!', 'success');
            } catch (error) {
                this.showMessage('Error submitting response: ' + error.message, 'error');
            }
        },

        getSessionStatusWarning() {
            const s = this.selectedSession;
            const info = this.pollOf(s);
            if (this.pendingSessionAction === 'Confirmed') {
                return (info && pollConfirmWarning(s, info.dates)) || 'Are you sure you want to confirm the session?';
            }
            if (this.pendingSessionAction === 'Cancelled') {
                if (s?.status === 'Suggested') {
                    // The last upcoming date counts as withdrawing the poll, so the players are told.
                    return `Withdraw ${formatShortDate(s.date)}?${info && info.dates.length === 1 ? ' Players will be told.' : ''}`;
                }
                return 'Are you sure you want to cancel the session?';
            }
            return '';
        },

        getPollWithdrawWarning(info) {
            return `Withdraw the poll? All ${pluralCount(info.dates.length, 'date')} and their replies will be removed. Players will be told.`;
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
