// Campaigns Alpine component
window.campaignsPage = function() {
    return {
        displayName: 'User',
        campaigns: [],
        loading: true,
        modalOpen: false,
        messageVisible: false,
        messageText: '',
        messageType: '',
        formData: {
            title: '',
            summary: '',
            description: '',
            desired_player_count: 1
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
                    body: JSON.stringify({
                        title,
                        summary,
                        description,
                        desired_player_count
                    })
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
            const statusTooltips = {
                Pitch: 'Campaign pitch phase - recruiting players',
                Ongoing: 'Campaign is actively running',
                Finished: 'Campaign has concluded',
                'On Hiatus': 'Campaign is temporarily paused',
                Cancelled: 'Campaign has been cancelled'
            };
            const statusTooltip = statusTooltips[status] || status;
            const statusAriaLabel = `Campaign status: ${status}. ${statusTooltip}`;

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
                            <span
                                class="campaign-status-badge"
                                data-tooltip="${escapeHtml(statusTooltip)}"
                                title="${escapeHtml(statusTooltip)}"
                                tabindex="0"
                                aria-label="${escapeHtml(statusAriaLabel)}"
                            >${escapeHtml(status)}</span>
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
                        <button type="button" class="campaign-action-btn">Codex</button>
                        <button type="button" class="campaign-action-btn">Calendar</button>
                        <button type="button" class="campaign-action-btn" ${!signUpsOpen ? 'disabled' : ''}>Sign-up</button>
                    </section>
                </article>
            `;
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
    if (!text) {
        return '';
    }
    const map = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#039;'
    };
    return String(text).replace(/[&<>"']/g, (m) => map[m]);
}
