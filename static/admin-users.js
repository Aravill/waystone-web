window.adminUsersPage = function () {
    return {
        displayName: '',
        isAdmin: false,
        loading: false,
        users: [],
        systems: [],
        newSystemName: '',
        searchQuery: '',
        campaigns: [],
        selectedCampaignId: '',
        newPlayerId: '',
        newUsername: '',
        newName: '',
        invite: null,
        confirmModal: { visible: false, title: '', message: '', onConfirm: null },
        messageVisible: false,
        messageText: '',
        messageType: '',

        get filteredUsers() {
            const q = this.searchQuery.toLowerCase();
            if (!q) return this.users;
            return this.users.filter(u =>
                (u.username || '').toLowerCase().includes(q) ||
                (u.name || '').toLowerCase().includes(q) ||
                (u.nickname || '').toLowerCase().includes(q)
            );
        },

        get selectedCampaign() {
            return this.campaigns.find(c => c.id === this.selectedCampaignId) || null;
        },

        get addablePlayers() {
            const c = this.selectedCampaign;
            if (!c) return [];
            return this.users.filter(u => !u.blocked && u.id !== c.dm && !(c.players || []).includes(u.id));
        },

        async init() {
            try {
                const res = await fetch('/auth/current-user');
                if (!res.ok) { window.location.href = '/login.html'; return; }
                const user = await res.json();
                this.displayName = user.display_name || user.username;
                this.isAdmin = (user.roles || []).includes('admin');
                if (!this.isAdmin) { window.location.href = '/'; return; }
                await this.loadUsers();
                await this.loadSystems();
                await this.loadCampaigns();
            } catch {
                window.location.href = '/login.html';
            }
        },

        async loadUsers() {
            this.loading = true;
            try {
                const res = await fetch('/api/admin/users');
                if (!res.ok) throw new Error('failed');
                this.users = await res.json();
            } catch {
                this.showMessage('Failed to load users', 'error');
            } finally {
                this.loading = false;
            }
        },

        async addUser() {
            if (!this.newUsername) return;
            this.loading = true;
            try {
                const res = await fetch('/api/admin/users', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username: this.newUsername, name: this.newName }),
                });
                const data = await res.json();
                if (!res.ok) {
                    this.showMessage(data.error || 'Failed to add user', 'error');
                    await this.loadUsers(); // the user may exist even though the invite failed
                    return;
                }
                this.newUsername = '';
                this.newName = '';
                this.invite = { username: data.user.username, url: `${window.location.origin}/invite/${data.invite_token}` };
                this.showMessage('User added', 'success');
                await this.loadUsers();
            } catch {
                this.showMessage('Failed to add user', 'error');
            } finally {
                this.loading = false;
            }
        },

        async blockUser(id) {
            await this._postAction(id, 'block', 'User blocked', 'Failed to block user');
        },

        async unblockUser(id) {
            await this._postAction(id, 'unblock', 'User unblocked', 'Failed to unblock user');
        },

        async makeAdmin(id) {
            await this._postAction(id, 'make-admin', 'User promoted to admin', 'Failed to promote user');
        },

        async removeAdmin(id) {
            await this._postAction(id, 'remove-admin', 'Admin role removed', 'Failed to remove admin role');
        },

        async resetPassword(user) {
            this.loading = true;
            try {
                const res = await fetch(`/api/admin/users/${user.id}/reset-password`, { method: 'POST' });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'Failed to reset password', 'error'); return; }
                this.invite = { username: user.username, url: `${window.location.origin}/invite/${data.invite_token}` };
                this.showMessage('Invite created', 'success');
                await this.loadUsers();
            } catch {
                this.showMessage('Failed to reset password', 'error');
            } finally {
                this.loading = false;
            }
        },

        async copyInvite() {
            try {
                await navigator.clipboard.writeText(this.invite.url);
                this.showMessage('Invite link copied', 'success');
            } catch {
                this.showMessage('Copy failed - select and copy it manually', 'error');
            }
        },

        async removeUser(id) {
            this.loading = true;
            try {
                const res = await fetch(`/api/admin/users/${id}`, { method: 'DELETE' });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'Failed to remove user', 'error'); return; }
                this.showMessage('User removed', 'success');
                await this.loadUsers();
            } catch {
                this.showMessage('Failed to remove user', 'error');
            } finally {
                this.loading = false;
            }
        },

        async loadSystems() {
            try {
                const res = await fetch('/api/admin/systems');
                if (!res.ok) throw new Error('failed');
                this.systems = await res.json();
            } catch {
                this.showMessage('Failed to load systems', 'error');
            }
        },

        async _sendLogo(id, file) {
            if (file.size > 256 * 1024) { this.showMessage('Logo must be 256 KB or smaller', 'error'); return false; }
            const res = await fetch(`/api/admin/systems/${id}/logo`, { method: 'PUT', body: file });
            const data = await res.json();
            if (!res.ok) { this.showMessage(data.error || 'Failed to upload logo', 'error'); return false; }
            return true;
        },

        async addSystem() {
            const name = this.newSystemName.trim();
            if (!name) return;
            const input = this.$refs.newSystemLogo;
            const file = input.files[0];
            if (file && file.size > 256 * 1024) { this.showMessage('Logo must be 256 KB or smaller', 'error'); return; }
            this.loading = true;
            try {
                const res = await fetch('/api/admin/systems', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name }),
                });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'Failed to add system', 'error'); return; }
                this.newSystemName = '';
                input.value = '';
                if (file && !(await this._sendLogo(data.id, file))) {
                    await this.loadSystems();
                    return;
                }
                this.showMessage('System added', 'success');
                await this.loadSystems();
            } catch {
                this.showMessage('Failed to add system', 'error');
            } finally {
                this.loading = false;
            }
        },

        async renameSystem(system) {
            const name = prompt('New name', system.name);
            if (name === null || name.trim() === '' || name.trim() === system.name) return;
            this.loading = true;
            try {
                const res = await fetch(`/api/admin/systems/${system.id}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name }),
                });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'Failed to rename system', 'error'); return; }
                this.showMessage('System renamed', 'success');
                await this.loadSystems();
            } catch {
                this.showMessage('Failed to rename system', 'error');
            } finally {
                this.loading = false;
            }
        },

        async uploadLogo(id, file) {
            if (!file) return;
            this.loading = true;
            try {
                if (await this._sendLogo(id, file)) this.showMessage('Logo updated', 'success');
                await this.loadSystems();
            } catch {
                this.showMessage('Failed to upload logo', 'error');
            } finally {
                this.loading = false;
            }
        },

        async removeLogo(id) {
            this.loading = true;
            try {
                const res = await fetch(`/api/admin/systems/${id}/logo`, { method: 'DELETE' });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'Failed to remove logo', 'error'); return; }
                this.showMessage('Logo removed', 'success');
                await this.loadSystems();
            } catch {
                this.showMessage('Failed to remove logo', 'error');
            } finally {
                this.loading = false;
            }
        },

        async deleteSystem(id) {
            this.loading = true;
            try {
                const res = await fetch(`/api/admin/systems/${id}`, { method: 'DELETE' });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'Failed to delete system', 'error'); return; }
                this.showMessage('System deleted', 'success');
                await this.loadSystems();
            } catch {
                this.showMessage('Failed to delete system', 'error');
            } finally {
                this.loading = false;
            }
        },

        async loadCampaigns() {
            try {
                const res = await fetch('/api/campaigns');
                if (!res.ok) throw new Error('failed');
                const data = await res.json();
                this.campaigns = (Array.isArray(data) ? data : data.campaigns || []).map(c => ({ ...c, player_users: c.player_users || [], players: c.players || [] }));
            } catch {
                this.showMessage('Failed to load games', 'error');
            }
        },

        async _campaignRequest(path, method, body, successMsg, errorMsg, admin = false) {
            this.loading = true;
            try {
                const res = await fetch(`/api/${admin ? 'admin/' : ''}campaigns/${this.selectedCampaignId}/${path}`, {
                    method,
                    headers: { 'Content-Type': 'application/json' },
                    body: body ? JSON.stringify(body) : undefined,
                });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || errorMsg, 'error'); return; }
                this.showMessage(successMsg, 'success');
                await this.loadCampaigns();
                this.newPlayerId = '';
            } catch {
                this.showMessage(errorMsg, 'error');
            } finally {
                this.loading = false;
            }
        },

        async addPlayer() {
            if (!this.newPlayerId) return;
            await this._campaignRequest('players', 'POST', { user_id: this.newPlayerId }, 'player added', 'failed to add player', true);
        },

        async removePlayer(userId) {
            await this._campaignRequest(`players/${userId}`, 'DELETE', null, 'player removed', 'failed to remove player', true);
        },

        async _postAction(id, action, successMsg, errorMsg) {
            this.loading = true;
            try {
                const res = await fetch(`/api/admin/users/${id}/${action}`, { method: 'POST' });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || errorMsg, 'error'); return; }
                this.showMessage(successMsg, 'success');
                await this.loadUsers();
            } catch {
                this.showMessage(errorMsg, 'error');
            } finally {
                this.loading = false;
            }
        },

        confirmAction(title, message, action) {
            this.confirmModal = { visible: true, title, message, onConfirm: action };
        },

        showMessage(text, type) {
            this.messageText = text;
            this.messageType = type;
            this.messageVisible = true;
            setTimeout(() => { this.messageVisible = false; }, 5000);
        },

        async logout() {
            await fetch('/auth/logout', { method: 'POST' });
            window.location.href = '/login.html';
        },
    };
};
