// Campaign types: single source for labels and tooltip/form descriptions.
const CAMPAIGN_TYPES = [
    { value: 'One-shot', label: 'One-shot', description: 'A single, self-contained story told in 1–2 sessions. Low commitment, ideal for trying a new system, a new DM, or just dropping in.' },
    { value: 'Adventure', label: 'Adventure', description: 'A short campaign of 3+ sessions with one self-contained plot and a clear ending. A few weeks to a few months of commitment.' },
    { value: 'Campaign', label: 'Campaign', description: 'A long, overarching story with no set number of sessions. Characters grow over many months; expect a regular, ongoing commitment.' }
];

// Campaigns Alpine component
window.campaignsPage = function () {
    return {
        displayName: 'User',
        isAdmin: false,
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
            type: 'Campaign',
            system: '',
            desired_player_count: ''
        },
        systems: [],
        campaignTypes: CAMPAIGN_TYPES,
        editingCampaignId: '',
        currentCalendarDate: new Date(),
        currentCampaignId: null,
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
        signupsModalOpen: false,
        signupsCampaignId: '',
        signupsCampaignTitle: '',
        signups: [],
        signupsAcceptedCount: 0,
        signupsDesiredCount: 0,
        signupsOpen: false,
        showDeclinedSignups: false,

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
                            (button.dataset.dmId || '') === this.currentUserId,
                            button.dataset.focusDate || ''
                        );
                    };
                    document.addEventListener('click', this._calendarButtonHandler);
                }

                if (!this._signupButtonHandler) {
                    this._signupButtonHandler = (event) => {
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
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
                return;
            }

            await this.loadCampaigns();
            if (location.hash) {
                await this.$nextTick();
                document.getElementById(location.hash.slice(1))?.scrollIntoView({ block: 'start' });
            }
            await this.loadSystems();
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
            if (desired_player_count !== '' && desired_player_count < 1) {
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
                    this.showMessage(data.error || (editing ? 'Failed to update campaign' : 'Failed to create campaign'), 'error');
                    return;
                }

                this.showMessage(editing ? 'Campaign updated' : 'Campaign created successfully!', 'success');
                this.closeModal();
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage((editing ? 'Error updating campaign: ' : 'Error creating campaign: ') + error.message, 'error');
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
            if (!confirm(`Delete campaign "${campaign.title}"? This also deletes its sessions and sign-ups and cannot be undone.`)) {
                return;
            }
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(campaign.id)}`, { method: 'DELETE' });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to delete campaign', 'error');
                    return;
                }
                this.closeModal();
                this.showMessage(data.message || 'Campaign deleted', 'success');
                await this.loadCampaigns();
            } catch (error) {
                this.showMessage('Error deleting campaign: ' + error.message, 'error');
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
            const isDM = !!this.currentUserId && campaign?.dm === this.currentUserId;
            const campaignPlayers = Array.isArray(campaign?.players) ? campaign.players : [];
            const isPlayer = campaignPlayers.includes(this.currentUserId);
            const signupStatus = campaign?.current_user_signup_status || '';
            const unansweredCount = isPlayer ? (parseInt(campaign?.unanswered_sessions_count, 10) || 0) : 0;
            const pendingSignupsCount = isDM ? (parseInt(campaign?.pending_signups_count, 10) || 0) : 0;
            const needsAction = unansweredCount > 0 || pendingSignupsCount > 0;
            const focusDate = campaign?.first_unanswered_session_date || '';
            const maxPlayers = parseInt(campaign?.desired_player_count, 10) || 0;
            const signUpsLabel = (signUpsOpen ? 'Open' : 'Closed') + (maxPlayers > 0 ? ` (${campaignPlayers.length}/${maxPlayers})` : '');

            let signupBtnHtml;
            if (isDM) {
                signupBtnHtml = `<button type="button" class="campaign-action-btn campaign-signups-btn" data-campaign-id="${escapeHtml(campaign?.id || '')}" data-campaign-title="${escapeHtml(campaign?.title || '')}"${pendingSignupsCount > 0 ? ` title="${pendingSignupsCount} pending sign-up${pendingSignupsCount === 1 ? '' : 's'} awaiting your decision" aria-label="Sign-ups, ${pendingSignupsCount} pending"` : ''}>Sign-ups${pendingSignupsCount > 0 ? `<span class="campaign-action-count">${pendingSignupsCount}</span>` : ''}</button>`;
            } else {
                let label = 'Sign-up';
                let disabled = !signUpsOpen;
                if (isPlayer) {
                    label = 'Joined'; disabled = true;
                } else if (signupStatus === 'Pending') {
                    label = 'Signed up'; disabled = true;
                } else if (signupStatus === 'Declined') {
                    label = 'Declined'; disabled = true;
                } else if (signupStatus === 'Kicked') {
                    label = 'Removed'; disabled = true;
                }
                signupBtnHtml = `<button type="button" class="campaign-action-btn campaign-signup-btn" data-campaign-id="${escapeHtml(campaign?.id || '')}" ${disabled ? 'disabled aria-disabled="true"' : ''}>${label}</button>`;
            }

            const statusMap = {
                Pitch: 'pitch',
                Ongoing: 'ongoing',
                Finished: 'finished',
                'On Hiatus': 'hiatus',
                Cancelled: 'cancelled'
            };
            const statusClass = statusMap[status] || String(status).toLowerCase().replace(/\s+/g, '-');

            const typeInfo = CAMPAIGN_TYPES.find((t) => t.value === campaign?.type) || CAMPAIGN_TYPES.find((t) => t.value === 'Campaign');
            const typeBadgeHtml = `<span class="campaign-type-badge type-${escapeHtml(typeInfo.value.toLowerCase())}">${escapeHtml(typeInfo.label)} <span class="info-icon" tabindex="0" role="img" aria-label="${escapeHtml(typeInfo.description)}" data-tooltip="${escapeHtml(typeInfo.description)}">i</span></span>`;

            const systemName = campaign?.system || '';
            const systemHtml = campaign?.system_logo_url
                ? `<img class="campaign-system-logo" src="${escapeHtml(campaign.system_logo_url)}" alt="${escapeHtml(systemName)}" title="${escapeHtml(systemName)}">`
                : (systemName ? `<span class="campaign-system-badge">${escapeHtml(systemName)}</span>` : '');

            const dmUser = campaign?.dm_user;
            const dmHtml = dmUser
                ? `<a href="${escapeHtml(dmUser.profile_url || `/profile?user_id=${encodeURIComponent(dmUser.id)}`)}" class="user-button user-button--dm">${escapeHtml(dmUser.display_name || dmUser.name || 'Unknown')}</a>`
                : (campaign?.dm ? `<p class="campaign-value campaign-nameplate">${escapeHtml(campaign.dm)}</p>` : '<p class="campaign-value campaign-nameplate">Unassigned</p>');

            const playerUsers = Array.isArray(campaign?.player_users) ? campaign.player_users : [];
            const players = Array.isArray(campaign?.players) ? campaign.players : [];
            let playersHtml = '';
            if (playerUsers.length === 0 && players.length === 0) {
                playersHtml = '<p class="campaign-value">No players yet</p>';
            } else if (playerUsers.length > 0) {
                playersHtml = playerUsers.map((player) =>
                    `<a href="${escapeHtml(player.profile_url || `/profile?user_id=${encodeURIComponent(player.id)}`)}" class="user-button">${escapeHtml(player.display_name || player.name || 'Unknown')}</a>`
                ).join('');
            } else {
                playersHtml = players.map((player) =>
                    `<span class="campaign-nameplate">${escapeHtml(player)}</span>`
                ).join('');
            }

            return `
                <article class="campaign-item status-${escapeHtml(statusClass)}${needsAction ? ' needs-action' : ''}" id="campaign-${escapeHtml(campaign?.id ?? 'unknown')}">
                    <section class="campaign-row campaign-row-top">
                        <div class="campaign-title-container">
                            <h3 class="campaign-title">${escapeHtml(title)}</h3>
                            <div class="campaign-badges">
                                <span class="campaign-status-badge">${escapeHtml(status)}</span>
                                ${typeBadgeHtml}
                                ${systemHtml}
                            </div>
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
                                <p class="campaign-value">${escapeHtml(signUpsLabel)}</p>
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
                        <button type="button" class="campaign-action-btn campaign-calendar-btn" data-campaign-id="${escapeHtml(campaign?.id || '')}" data-campaign-title="${escapeHtml(campaign?.title || '')}" data-dm-id="${escapeHtml(campaign?.dm || '')}"${unansweredCount > 0 ? ` title="${unansweredCount} suggested session${unansweredCount === 1 ? '' : 's'} awaiting your response" aria-label="Calendar, ${unansweredCount} awaiting your response"` : ''}${unansweredCount > 0 && focusDate ? ` data-focus-date="${escapeHtml(focusDate)}"` : ''}>Calendar${unansweredCount > 0 ? `<span class="campaign-action-count">${unansweredCount}</span>` : ''}</button>
                        ${signupBtnHtml}
                        ${campaign?.can_manage ? `<button type="button" class="campaign-action-btn campaign-edit-btn" data-campaign-id="${escapeHtml(campaign?.id || '')}">Edit</button>` : ''}
                    </section>
                </article>
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

        async openCalendar(campaignId, campaignTitle, isDM, focusDate = '') {
            this.currentCampaignId = campaignId;
            this.currentCampaignTitle = campaignTitle;
            this.isCurrentUserDM = !!isDM;
            const focus = /^\d{4}-\d{2}-\d{2}$/.test(focusDate) ? this.parseDateLocal(focusDate) : null;
            this.currentCalendarDate = focus ? new Date(focus.getFullYear(), focus.getMonth(), 1) : new Date();
            this.calendarModalOpen = true;
            this.selectedDay = null;
            this.showCreateSessionModal = false;
            this.selectedSession = null;
            this.pendingSessionAction = '';
            this.sessionResponseCache = {};
            this.generateCalendarDays();
            await this.loadSessions();
            if (focus) {
                await this.selectDay(focus);
            }
        },

        closeCalendarModal() {
            this.calendarModalOpen = false;
            this.showCreateSessionModal = false;
            this.clearSelectedDay();
            this.sessionResponseCache = {};
        },

        handleCalendarBackdropClick(event) {
            if (event.target.id === 'calendarModal') {
                this.closeCalendarModal();
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

        getPendingResponsesForDay(date) {
            return this.getPlannedSessionsForDay(date).reduce((total, session) => total + (Number(session?.pending_count) || 0), 0);
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
            if (!this.isHistoricalDay(date)) return '';
            return this.isCurrentUserDM
                ? 'Previously planned session is inactive. Click to suggest a new one.'
                : 'Previously planned session is inactive.';
        },

        getDayAriaLabel(date) {
            if (!this.isHistoricalDay(date)) return '';
            return this.isCurrentUserDM
                ? 'Previously planned session is inactive. Select this day to suggest a new session.'
                : 'Previously planned session is inactive.';
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

        getUserDisplayName(user) {
            return user?.nickname || user?.display_name || user?.name || user?.username || 'Unknown';
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
            return this.sortedByTime(this.getSessionsForDay(date)).slice(0, 2);
        },

        getCellOverflow(date) {
            return Math.max(0, this.getSessionsForDay(date).length - 2);
        },

        getSelectedDaySessions() {
            return this.selectedDay ? this.sortedByTime(this.getSessionsForDay(this.selectedDay)) : [];
        },

        parseDateLocal(dateStr) {
            const [year, month, day] = String(dateStr).split('-').map(Number);
            return new Date(year, month - 1, day);
        },

        formatDayHeading(date) {
            return date.toLocaleDateString('default', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
        },

        formatAgendaDate(session) {
            return this.parseDateLocal(session.date).toLocaleDateString('default', { weekday: 'short', day: 'numeric', month: 'short' });
        },

        getDurationLabel(session) {
            const total = Number(session?.duration) || 0;
            const hours = Math.floor(total / 60);
            const minutes = total % 60;
            if (hours && minutes) return `${hours}h ${minutes}m`;
            if (hours) return `${hours}h`;
            return `${minutes}m`;
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
                { label: 'Accepted', cls: 'participation-accepted', users: byStatus('Accepted') },
                { label: 'Tentative', cls: 'participation-tentative', users: byStatus('Tentative') },
                { label: 'Declined', cls: 'participation-declined', users: byStatus('Declined') },
                { label: 'Pending', cls: 'participation-pending', users: details.pending }
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
                await this.loadCampaigns();
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

function getDefaultTimezone() {
    try {
        return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
    } catch (error) {
        return 'UTC';
    }
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
