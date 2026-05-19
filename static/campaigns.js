// Campaigns Alpine component
window.campaignsPage = function () {
    return {
        displayName: 'User',
        currentUserId: '',
        campaigns: [],
        loading: true,
        modalOpen: false,
        calendarModalOpen: false,
        messageVisible: false,
        messageText: '',
        messageType: '',
        formData: {
            title: '',
            summary: '',
            description: '',
            desired_player_count: 1
        },
        currentCalendarDate: new Date(),
        currentCampaignId: null,
        currentCampaignTitle: '',
        isCurrentUserDM: false,
        calendarDays: [],
        sessions: [],
        selectedDay: null,
        showCreateSessionModal: false,
        showSessionStatusModal: false,
        pendingSessionAction: '',
        selectedSession: null,
        sessionFormData: {
            date: '',
            time: '',
            durationHours: 1,
            durationMinutes: 0
        },
        sessionResponses: [],
        pendingResponses: [],
        showResponses: false,
        playerResponse: '',
        sessionResponseCache: {},
        hoverResponsePanel: {
            visible: false,
            sessionId: '',
            type: '',
            users: [],
            loading: false,
            error: ''
        },

        async init() {
            try {
                const userResponse = await fetch('/auth/current-user');
                if (!userResponse.ok) {
                    window.location.href = '/login.html';
                    return;
                }

                const user = await userResponse.json();
                this.displayName = user.display_name || user.name || user.email || 'User';
                this.currentUserId = user.user_id || user.id || '';

                if (!this._calendarButtonHandler) {
                    this._calendarButtonHandler = (event) => {
                        const button = event.target.closest('.campaign-calendar-btn');
                        if (!button) {
                            return;
                        }

                        event.preventDefault();
                        this.openCalendar(
                            button.dataset.campaignId || '',
                            button.dataset.campaignTitle || '',
                            (button.dataset.dmId || '') === this.currentUserId
                        );
                    };
                    document.addEventListener('click', this._calendarButtonHandler);
                }
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
                return;
            }

            await this.loadCampaigns();
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

        openModal() {
            this.formData = { title: '', summary: '', description: '', desired_player_count: 1 };
            this.modalOpen = true;
        },

        closeModal() {
            this.modalOpen = false;
        },

        handleModalBackdropClick(event) {
            if (event.target.id === 'createCampaignModal') {
                this.closeModal();
            }
        },

        async handleCreateCampaign() {
            const { title, summary, description, desired_player_count } = this.formData;
            if (!title || !summary || !description || !desired_player_count || desired_player_count <= 0) {
                this.showMessage('All fields are required', 'error');
                return;
            }

            try {
                const response = await fetch('/api/campaigns', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ title, summary, description, desired_player_count })
                });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to create campaign', 'error');
                    return;
                }

                this.showMessage('Campaign created successfully!', 'success');
                this.closeModal();
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage('Error creating campaign: ' + error.message, 'error');
            }
        },

        async loadCampaigns() {
            this.loading = true;
            try {
                const response = await fetch('/api/campaigns');
                if (!response.ok) {
                    throw new Error('Failed to load campaigns');
                }
                const data = await response.json();
                this.campaigns = Array.isArray(data) ? data : [];
            } catch (error) {
                this.showMessage('Error loading campaigns: ' + error.message, 'error');
                this.campaigns = [];
            } finally {
                this.loading = false;
            }
        },

        renderCampaignHtml(campaign) {
            const title = campaign?.title || 'Untitled Campaign';
            const summary = campaign?.summary || 'No summary available.';
            const description = campaign?.description || '';
            const status = campaign?.status || 'Unknown';
            const signUpsOpen = campaign?.sign_ups_open === true;
            const nextSession = 'N/A';

            const statusMap = {
                Pitch: 'pitch',
                Ongoing: 'ongoing',
                Finished: 'finished',
                'On Hiatus': 'hiatus',
                Cancelled: 'cancelled'
            };
            const statusClass = statusMap[status] || String(status).toLowerCase().replace(/\s+/g, '-');

            const dmUser = campaign?.dm_user;
            const dmHtml = dmUser
                ? `<a href="${escapeHtml(dmUser.profile_url || `/profile?user_id=${encodeURIComponent(dmUser.id)}`)}" class="user-button user-button--dm">${escapeHtml(dmUser.display_name || dmUser.name || dmUser.email || 'Unknown')}</a>`
                : (campaign?.dm ? `<p class="campaign-value campaign-nameplate">${escapeHtml(campaign.dm)}</p>` : '<p class="campaign-value campaign-nameplate">Unassigned</p>');

            const playerUsers = Array.isArray(campaign?.player_users) ? campaign.player_users : [];
            const players = Array.isArray(campaign?.players) ? campaign.players : [];
            let playersHtml = '';
            if (playerUsers.length === 0 && players.length === 0) {
                playersHtml = '<p class="campaign-value">No players yet</p>';
            } else if (playerUsers.length > 0) {
                playersHtml = playerUsers.map((player) =>
                    `<a href="${escapeHtml(player.profile_url || `/profile?user_id=${encodeURIComponent(player.id)}`)}" class="user-button">${escapeHtml(player.display_name || player.name || player.email || 'Unknown')}</a>`
                ).join('');
            } else {
                playersHtml = players.map((player) =>
                    `<span class="campaign-nameplate">${escapeHtml(player)}</span>`
                ).join('');
            }

            return `
                <article class="campaign-item status-${escapeHtml(statusClass)}" id="campaign-${escapeHtml(campaign?.id ?? 'unknown')}">
                    <section class="campaign-row campaign-row-top">
                        <div class="campaign-title-container">
                            <h3 class="campaign-title">${escapeHtml(title)}</h3>
                            <span class="campaign-status-badge">${escapeHtml(status)}</span>
                        </div>
                        <div class="campaign-summary-row">
                            <p class="campaign-summary-text">${escapeHtml(summary)}</p>
                            <button type="button" class="campaign-summary-arrow" aria-expanded="false" aria-label="Expand campaign description" onclick="const expanded = this.getAttribute('aria-expanded') !== 'true'; this.classList.toggle('campaign-summary-arrow-expanded', expanded); this.parentElement.nextElementSibling.classList.toggle('campaign-description-expanded', expanded); this.setAttribute('aria-expanded', expanded ? 'true' : 'false'); this.setAttribute('aria-label', expanded ? 'Collapse campaign description' : 'Expand campaign description');">▼</button>
                        </div>
                        <p class="campaign-description">${escapeHtml(description || 'No additional description.')}</p>
                    </section>

                    <section class="campaign-row campaign-row-middle">
                        <div class="campaign-column campaign-column-left">
                            <div class="campaign-field">
                                <h4 class="campaign-label">Dungeon Master</h4>
                                ${dmHtml}
                            </div>
                            <div class="campaign-field">
                                <h4 class="campaign-label">Sign-ups</h4>
                                <p class="campaign-value">${signUpsOpen ? 'Open' : 'Closed'}</p>
                            </div>
                        </div>
                        <div class="campaign-column campaign-column-right">
                            <div class="campaign-field">
                                <h4 class="campaign-label">Next Session</h4>
                                <p class="campaign-value">${escapeHtml(nextSession)}</p>
                            </div>
                            <div class="campaign-field">
                                <h4 class="campaign-label">Players</h4>
                                <div class="campaign-players">${playersHtml}</div>
                            </div>
                        </div>
                    </section>

                    <section class="campaign-row campaign-row-bottom">
                        <button type="button" class="campaign-action-btn" disabled aria-disabled="true">Codex</button>
                        <button type="button" class="campaign-action-btn campaign-calendar-btn" data-campaign-id="${escapeHtml(campaign?.id || '')}" data-campaign-title="${escapeHtml(campaign?.title || '')}" data-dm-id="${escapeHtml(campaign?.dm || '')}">Calendar</button>
                        <button type="button" class="campaign-action-btn" ${!signUpsOpen ? 'disabled' : ''}>Sign-up</button>
                    </section>
                </article>
            `;
        },

        openCalendar(campaignId, campaignTitle, isDM) {
            this.currentCampaignId = campaignId;
            this.currentCampaignTitle = campaignTitle;
            this.isCurrentUserDM = !!isDM;
            this.currentCalendarDate = new Date();
            this.calendarModalOpen = true;
            this.selectedDay = null;
            this.showCreateSessionModal = false;
            this.showSessionStatusModal = false;
            this.selectedSession = null;
            this.sessionResponses = [];
            this.pendingResponses = [];
            this.showResponses = false;
            this.sessionResponseCache = {};
            this.clearDayHoverPanel();
            this.generateCalendarDays();
            this.loadSessions();
        },

        closeCalendarModal() {
            this.calendarModalOpen = false;
            this.showCreateSessionModal = false;
            this.showSessionStatusModal = false;
            this.clearSelectedSession();
            this.clearSelectedDay();
            this.showResponses = false;
            this.sessionResponseCache = {};
            this.clearDayHoverPanel();
        },

        handleCalendarBackdropClick(event) {
            if (event.target.id === 'calendarModal') {
                this.closeCalendarModal();
            }
        },

        handleCreateSessionBackdropClick(event) {
            if (event.target.id === 'createSessionModal') {
                this.clearSelectedDay();
            }
        },

        handleSessionStatusBackdropClick(event) {
            if (event.target.id === 'sessionStatusModal') {
                this.clearSelectedSession();
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
            this.currentCalendarDate = new Date(this.currentCalendarDate.getFullYear(), this.currentCalendarDate.getMonth() - 1, 1);
            this.generateCalendarDays();
            this.loadSessions();
        },

        nextMonth() {
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
            } catch (error) {
                this.showMessage('Error loading sessions: ' + error.message, 'error');
                this.sessions = [];
            }
        },

        hasSession(date) {
            return this.getSessionsForDay(date).length > 0;
        },

        getSessionsForDay(date) {
            const dateStr = this.formatDateLocal(date);
            return this.sessions.filter((session) => session.date === dateStr);
        },

        getPlannedSessionsForDay(date) {
            return this.getSessionsForDay(date).filter((session) => session.status !== 'Cancelled');
        },

        getPrimarySessionForDay(date) {
            const daySessions = this.getSessionsForDay(date);
            if (daySessions.length === 0) return null;
            return [...daySessions].sort((a, b) => String(a.time || '').localeCompare(String(b.time || '')))[0];
        },

        getPendingResponsesForDay(date) {
            return this.getSessionsForDay(date).reduce((total, session) => total + (Number(session?.pending_count) || 0), 0);
        },

        getDayStatusClass(date) {
            const primarySession = this.getPrimarySessionForDay(date);
            if (!primarySession) return '';
            if (primarySession.status === 'Suggested') return 'day-suggested';
            if (primarySession.status === 'Confirmed') return 'day-confirmed';
            return 'day-cancelled';
        },

        getSessionEndTime(session) {
            const time = session?.time || '';
            const duration = Number(session?.duration) || 0;
            const [hour, minute] = String(time).split(':').map((part) => Number(part));
            if (Number.isNaN(hour) || Number.isNaN(minute)) return time;
            const totalMinutes = (hour * 60) + minute + duration;
            const endHour = Math.floor((totalMinutes % (24 * 60)) / 60);
            const endMinute = totalMinutes % 60;
            return `${String(endHour).padStart(2, '0')}:${String(endMinute).padStart(2, '0')}`;
        },

        getSessionTimeRange(session) {
            const startTime = session?.time || '';
            const endTime = this.getSessionEndTime(session);
            return `${startTime}-${endTime}`;
        },

        getAcceptedReaction(session) {
            return `${session?.accepted_count || 0}✓`;
        },

        getDeclinedReaction(session) {
            return `${session?.declined_count || 0}✗`;
        },

        getTentativeReaction(session) {
            return `${session?.tentative_count || 0}?`;
        },

        getPrimarySessionIdForDay(date) {
            const session = this.getPrimarySessionForDay(date);
            return session?.id || '';
        },

        getHoverPanelTitle(type) {
            if (type === 'accepted') return 'Accepted';
            if (type === 'declined') return 'Declined';
            if (type === 'tentative') return 'Tentative';
            return 'Pending';
        },

        getUserDisplayName(user) {
            return user?.nickname || user?.display_name || user?.name || user?.email || 'Unknown';
        },

        isHoverPanelVisibleForDay(date) {
            return this.hoverResponsePanel.visible && this.hoverResponsePanel.sessionId === this.getPrimarySessionIdForDay(date);
        },

        clearDayHoverPanel() {
            this.hoverResponsePanel = {
                visible: false,
                sessionId: '',
                type: '',
                users: [],
                loading: false,
                error: ''
            };
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

        getUsersForHoverType(details, type) {
            if (type === 'pending') {
                return details.pending;
            }

            const participation = type === 'accepted'
                ? 'Accepted'
                : type === 'declined'
                    ? 'Declined'
                    : 'Tentative';

            return details.all
                .filter((entry) => entry?.participation === participation)
                .map((entry) => entry?.user)
                .filter(Boolean);
        },

        async handleDayReactionHover(day, type) {
            const sessionId = this.getPrimarySessionIdForDay(day);
            if (!sessionId) {
                return;
            }

            this.hoverResponsePanel = {
                visible: true,
                sessionId,
                type,
                users: [],
                loading: true,
                error: ''
            };

            try {
                const details = await this.getSessionResponseDetails(sessionId);
                if (!this.hoverResponsePanel.visible || this.hoverResponsePanel.sessionId !== sessionId || this.hoverResponsePanel.type !== type) {
                    return;
                }
                this.hoverResponsePanel = {
                    ...this.hoverResponsePanel,
                    users: this.getUsersForHoverType(details, type),
                    loading: false
                };
            } catch (error) {
                if (!this.hoverResponsePanel.visible || this.hoverResponsePanel.sessionId !== sessionId || this.hoverResponsePanel.type !== type) {
                    return;
                }
                this.hoverResponsePanel = {
                    ...this.hoverResponsePanel,
                    users: [],
                    loading: false,
                    error: error.message || 'Failed to load responses'
                };
            }
        },

        selectDay(date) {
            if (!this.isCurrentMonth(date)) return;
            const daySessions = this.getPlannedSessionsForDay(date);
            this.selectedDay = date;

            if (daySessions.length > 0) {
                this.selectSession(daySessions[0], date);
                return;
            }

            if (!this.isCurrentUserDM) {
                return;
            }

            this.selectedSession = null;
            this.playerResponse = '';
            this.sessionFormData = {
                date: this.formatDateLocal(date),
                time: '19:00',
                durationHours: 1,
                durationMinutes: 0
            };
            this.showCreateSessionModal = true;
            this.showSessionStatusModal = false;
        },

        selectSession(session, day) {
            this.selectedSession = session;
            this.selectedDay = day || null;
            this.showCreateSessionModal = false;
            this.showSessionStatusModal = this.isCurrentUserDM && session.status !== 'Cancelled';
            this.pendingSessionAction = '';
            this.playerResponse = session.current_user_participation || '';
            this.showResponses = false;
        },

        clearSelectedDay() {
            this.selectedDay = null;
            this.showCreateSessionModal = false;
            this.sessionFormData = { date: '', time: '', durationHours: 1, durationMinutes: 0 };
        },

        clearSelectedSession() {
            this.selectedSession = null;
            this.showSessionStatusModal = false;
            this.pendingSessionAction = '';
            this.playerResponse = '';
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
                this.showMessage('Only the campaign DM can create sessions', 'error');
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
                    duration: totalDuration
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
                this.clearSelectedDay();
                await this.loadSessions();
            } catch (error) {
                this.showMessage('Error creating session: ' + error.message, 'error');
            }
        },

        async confirmSession() {
            await this.requestSessionStatusChange('Confirmed');
        },

        async cancelSession() {
            await this.requestSessionStatusChange('Cancelled');
        },

        async requestSessionStatusChange(status) {
            if (!this.selectedSession) return;
            if (status === 'Confirmed' && this.selectedSession.status !== 'Suggested') {
                this.showMessage('Only suggested sessions can be confirmed', 'error');
                return;
            }

            if (this.pendingSessionAction !== status) {
                this.pendingSessionAction = status;
                return;
            }

            this.pendingSessionAction = '';
            await this.updateSessionStatus(status);
        },

        async updateSessionStatus(status) {
            if (!this.selectedSession) return;
            try {
                const response = await fetch(`/api/campaigns/${this.currentCampaignId}/sessions/${this.selectedSession.id}`, {
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
                this.clearSelectedSession();
                this.showResponses = false;
                await this.loadSessions();
            } catch (error) {
                this.showMessage('Error updating session: ' + error.message, 'error');
            }
        },

        async submitResponse(participation) {
            if (!this.selectedSession) return;
            try {
                const response = await fetch(`/api/campaigns/${this.currentCampaignId}/sessions/${this.selectedSession.id}/responses`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ participation })
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to submit response', 'error');
                    return;
                }
                this.playerResponse = participation;
                await this.loadSessions();
                this.selectedSession = this.sessions.find((session) => session.id === this.selectedSession.id) || this.selectedSession;
                this.showMessage('Response submitted!', 'success');
            } catch (error) {
                this.showMessage('Error submitting response: ' + error.message, 'error');
            }
        },

        async showResponseBreakdown(session) {
            const targetSession = session || this.selectedSession;
            if (!targetSession) return;
            this.selectedSession = targetSession;
            try {
                const details = await this.getSessionResponseDetails(targetSession.id);
                this.sessionResponses = details.all;
                this.pendingResponses = details.pending;
                this.showResponses = true;
            } catch (error) {
                this.showMessage('Error loading responses: ' + error.message, 'error');
            }
        },

        getParticipationClass(participation) {
            if (participation === 'Accepted') return 'participation-accepted';
            if (participation === 'Declined') return 'participation-declined';
            if (participation === 'Tentative') return 'participation-tentative';
            return '';
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

        showMessage(text, type) {
            this.messageText = text;
            this.messageType = type;
            this.messageVisible = true;
            setTimeout(() => {
                this.messageVisible = false;
            }, 5000);
        }
    };
};

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
