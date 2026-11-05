// Calendar page (/games/{id}/calendar). Relies on campaigns.js (loaded first) for the top-level
// helpers: REPLY_GLYPHS, userAvatarHtml, parseDateLocal, getSessionEndTime, getDurationLabel,
// getDefaultTimezone and getTimezoneOptions. It does not use campaignsPage().

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
        guestLinks: {},
        guestTtlHours: 24,
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
        showCreateSessionModal: false,
        pendingSessionAction: '',
        selectedSession: null,
        sessionFormData: {
            date: '',
            time: '',
            durationHours: 1,
            durationMinutes: 0,
            timezone: ''
        },
        timezoneOptions: getTimezoneOptions(),
        sessionResponseCache: {},
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
                await this.openCalendar(data.first_unanswered_session_date || '');
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
            this.showCreateSessionModal = false;
            this.selectedSession = null;
            this.pendingSessionAction = '';
            this.sessionResponseCache = {};
            this.guestLinks = {};
            this.generateCalendarDays();
            if (this.isCurrentUserDM) {
                this.loadGuestLinks();
            }
            await this.loadSessions();
            if (focus) {
                await this.selectDay(focus);
            }
        },

        handleCreateSessionBackdropClick(event) {
            if (event.target.id === 'createSessionModal') {
                this.closeCreateSessionModal();
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
            return this.currentCalendarDate.toLocaleString('default', { month: 'long', year: 'numeric' });
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
            try {
                const response = await fetch(`/api/campaigns/${this.currentCampaignId}/sessions?month=${year}-${month}`);
                if (!response.ok) {
                    throw new Error('Failed to load sessions');
                }
                const data = await response.json();
                this.sessions = Array.isArray(data) ? data : [];
                this.buildSessionMap();
            } catch (error) {
                this.showMessage('Error loading sessions: ' + error.message, 'error');
                this.sessions = [];
                this.sessionMap = {};
            }
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
            const summary = this.getDaySummary(date);
            if (!this.isHistoricalDay(date)) return summary;
            return `${summary}. ${this.isCurrentUserDM
                ? 'Previously planned session is inactive. Click to suggest a new one.'
                : 'Previously planned session is inactive.'}`;
        },

        getDayAriaLabel(date) {
            const summary = this.getDaySummary(date);
            if (!summary) return '';
            const head = this.formatDayHeading(date);
            if (!this.isHistoricalDay(date)) return `${head}. ${summary}`;
            return `${head}. ${summary}. ${this.isCurrentUserDM
                ? 'Previously planned session is inactive. Select this day to suggest a new session.'
                : 'Previously planned session is inactive.'}`;
        },

        getSessionEndTime(session) {
            return getSessionEndTime(session);
        },

        getSessionTimeRange(session) {
            const startTime = session?.time || '';
            const endTime = this.getSessionEndTime(session);
            return `${startTime}-${endTime}`;
        },

        getSessionCounts(session) {
            const accepted = Number(session?.accepted_count) || 0;
            const tentative = Number(session?.tentative_count) || 0;
            const declined = Number(session?.declined_count) || 0;
            const awaiting = Number(session?.pending_count) || 0;
            return { accepted, tentative, declined, awaiting, total: accepted + tentative + declined + awaiting };
        },

        getTally(session) {
            const c = this.getSessionCounts(session);
            return [
                { kind: 'accepted', n: c.accepted },
                { kind: 'tentative', n: c.tentative },
                { kind: 'declined', n: c.declined },
                { kind: 'awaiting', n: c.awaiting }
            ];
        },

        // A suggestion with over 70% of all players accepted (awaiting players count; exactly 70% does not).
        isStrongCandidate(session) {
            const c = this.getSessionCounts(session);
            return session?.status === 'Suggested' && c.total > 0 && c.accepted * 10 > c.total * 7;
        },

        getCandidateNote(session) {
            const c = this.getSessionCounts(session);
            return `Strong candidate: ${c.accepted} of ${c.total} accepted`;
        },

        getDaySummary(date) {
            const s = this.getPrimarySessionForDay(date);
            if (!s) return '';
            const parts = [`${s.status} ${this.getSessionTimeRange(s)}`];
            if (s.status !== 'Cancelled') {
                const c = this.getSessionCounts(s);
                parts[0] += `: ${c.accepted} accepted, ${c.tentative} tentative, ${c.declined} declined, ${c.awaiting} awaiting`;
            }
            if (this.isStrongCandidate(s)) parts.push('Strong candidate');
            const more = this.getCellOverflow(date);
            if (more > 0) parts.push(`+${more} more session${more === 1 ? '' : 's'}`);
            return parts.join('. ');
        },

        replyGlyph(kind) {
            return REPLY_GLYPHS[kind] || '';
        },

        getRoster(session) {
            return this.getResponseGroups(session).flatMap((g) => g.users.map((user, i) =>
                ({ key: `${g.kind}-${user.id || i}`, user, label: g.label, cls: g.cls, kind: g.kind })));
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
            this.showCreateSessionModal = false;
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

        openCreateSession(date) {
            if (!this.isCurrentUserDM || !date) return;
            this.sessionFormData = {
                date: this.formatDateLocal(date),
                time: '19:00',
                durationHours: 1,
                durationMinutes: 0,
                timezone: getDefaultTimezone()
            };
            this.showCreateSessionModal = true;
        },

        async loadDayResponses() {
            try {
                await Promise.all(this.getSelectedDaySessions().map((session) => this.getSessionResponseDetails(session.id)));
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

        formatDayHeading(date) {
            return date.toLocaleDateString('default', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
        },

        formatAgendaDate(session) {
            return this.parseDateLocal(session.date).toLocaleDateString('default', { weekday: 'short', day: 'numeric', month: 'short' });
        },

        getDurationLabel(session) {
            return getDurationLabel(session);
        },

        getSessionMeta(session) {
            const parts = [this.getDurationLabel(session)];
            if (session?.timezone) parts.push(session.timezone);
            return parts.join(' · ');
        },

        getResponseGroups(session) {
            const details = this.sessionResponseCache[session.id];
            if (!details) return [];
            const byStatus = (participation) => details.all
                .filter((entry) => entry?.participation === participation)
                .map((entry) => entry.user || { display_name: 'Unknown' });
            return [
                { kind: 'accepted', label: 'Accepted', cls: 'participation-accepted', users: byStatus('Accepted') },
                { kind: 'tentative', label: 'Tentative', cls: 'participation-tentative', users: byStatus('Tentative') },
                { kind: 'declined', label: 'Declined', cls: 'participation-declined', users: byStatus('Declined') },
                { kind: 'awaiting', label: 'Awaiting', cls: 'participation-pending', users: details.pending }
            ];
        },

        clearSelectedDay() {
            this.selectedDay = null;
            this.selectedSession = null;
            this.pendingSessionAction = '';
        },

        closeCreateSessionModal() {
            this.showCreateSessionModal = false;
            this.sessionFormData = { date: '', time: '', durationHours: 1, durationMinutes: 0, timezone: '' };
        },

        adjustDurationPart(field, delta) {
            if (field !== 'durationHours' && field !== 'durationMinutes') {
                return;
            }

            const current = Math.floor(Number(this.sessionFormData[field]) || 0);
            let next = current + delta;
            if (field === 'durationHours') {
                next = Math.max(0, next);
            } else {
                next = Math.max(0, Math.min(59, next));
            }
            this.sessionFormData[field] = next;
        },

        async createSession() {
            if (!this.isCurrentUserDM) {
                this.showMessage('Only the game DM can create sessions', 'error');
                return;
            }
            const durationHours = Math.max(0, Math.floor(Number(this.sessionFormData.durationHours) || 0));
            const durationMinutes = Math.max(0, Math.min(59, Math.floor(Number(this.sessionFormData.durationMinutes) || 0)));
            this.sessionFormData.durationHours = durationHours;
            this.sessionFormData.durationMinutes = durationMinutes;
            const totalDuration = (durationHours * 60) + durationMinutes;
            if (!this.sessionFormData.date || !this.sessionFormData.time || totalDuration <= 0) {
                this.showMessage('All session fields are required', 'error');
                return;
            }

            try {
                const payload = {
                    date: this.sessionFormData.date,
                    time: this.sessionFormData.time,
                    duration: totalDuration,
                    timezone: this.sessionFormData.timezone
                };
                const response = await fetch(`/api/campaigns/${this.currentCampaignId}/sessions`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to create session', 'error');
                    return;
                }
                this.showMessage('Session created successfully!', 'success');
                this.closeCreateSessionModal();
                await this.loadSessions();
                await this.loadDayResponses();
            } catch (error) {
                this.showMessage('Error creating session: ' + error.message, 'error');
            }
        },

        async requestSessionStatusChange(status, session) {
            if (!session) return;
            if (status === 'Confirmed' && session.status !== 'Suggested') {
                this.showMessage('Only suggested sessions can be confirmed', 'error');
                return;
            }

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
                this.showMessage(`Session ${status.toLowerCase()} successfully`, 'success');
                this.invalidateSessionCache(session.id);
                await this.loadSessions();
                await this.loadDayResponses();
            } catch (error) {
                this.showMessage('Error updating session: ' + error.message, 'error');
            }
        },

        async submitResponse(participation, session) {
            if (!session) return;
            this.pendingSessionAction = '';
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
                await this.loadSessions();
                await this.loadDayResponses();
                this.showMessage('Response submitted!', 'success');
            } catch (error) {
                this.showMessage('Error submitting response: ' + error.message, 'error');
            }
        },

        getSessionStatusWarning() {
            if (this.pendingSessionAction === 'Confirmed') {
                return 'Are you sure you want to confirm the session?';
            }
            if (this.pendingSessionAction === 'Cancelled') {
                return 'Are you sure you want to cancel the session?';
            }
            return '';
        },

        async loadGuestLinks() {
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.currentCampaignId)}/guest-links`);
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to load guest links', 'error');
                    return;
                }
                this.guestLinks = data.links || {};
            } catch (error) {
                this.showMessage('Error loading guest links: ' + error.message, 'error');
            }
        },

        guestUrl(userId) {
            const link = this.guestLinks[userId];
            return link ? `${window.location.origin}/r/${link.token}` : '';
        },

        get currentCampaignPlayers() {
            return Array.isArray(this.campaign?.player_users) ? this.campaign.player_users : [];
        },

        isGuestLinkExpired(userId) {
            const link = this.guestLinks[userId];
            return !!link && new Date(link.expires_at) <= new Date();
        },

        guestLinkExpiry(userId) {
            const link = this.guestLinks[userId];
            if (!link) return 'no link';
            const when = new Date(link.expires_at);
            const text = when.toLocaleString('default', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
            return when <= new Date() ? `expired ${text}` : `until ${text}`;
        },

        async createGuestLink(userId) {
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.currentCampaignId)}/players/${encodeURIComponent(userId)}/guest-link`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ttl_hours: this.guestTtlHours })
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to create guest link', 'error');
                    return;
                }
                await this.loadGuestLinks();
            } catch (error) {
                this.showMessage('Error creating guest link: ' + error.message, 'error');
            }
        },

        async revokeGuestLink(userId, name) {
            const ok = await this.askConfirm({
                title: 'Revoke guest link',
                message: `Revoke the guest link for ${name}? The current link will stop working.`,
                confirmLabel: 'Revoke'
            });
            if (!ok) {
                return;
            }
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.currentCampaignId)}/players/${encodeURIComponent(userId)}/guest-link`, {
                    method: 'DELETE'
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to revoke guest link', 'error');
                    return;
                }
                await this.loadGuestLinks();
            } catch (error) {
                this.showMessage('Error revoking guest link: ' + error.message, 'error');
            }
        },

        async copyGuestLink(userId) {
            try {
                await navigator.clipboard.writeText(this.guestUrl(userId));
                this.showMessage('Link copied', 'success');
            } catch (error) {
                window.prompt('Copy this link:', this.guestUrl(userId));
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
