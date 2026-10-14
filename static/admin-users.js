window.adminUsersPage = function () {
    return {
        displayName: '',
        isAdmin: false,
        loading: false,
        users: [],
        systems: [],
        newSystemName: '',
        searchQuery: '',
        newUsername: '',
        newName: '',
        tempPassword: null,
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
                this.showMessage('failed to load users', 'error');
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
                if (!res.ok) { this.showMessage(data.error || 'failed to add user', 'error'); return; }
                this.newUsername = '';
                this.newName = '';
                this.tempPassword = { username: data.user.username, password: data.temp_password };
                this.showMessage('user added', 'success');
                await this.loadUsers();
            } catch {
                this.showMessage('failed to add user', 'error');
            } finally {
                this.loading = false;
            }
        },

        async blockUser(id) {
            await this._postAction(id, 'block', 'user blocked', 'failed to block user');
        },

        async unblockUser(id) {
            await this._postAction(id, 'unblock', 'user unblocked', 'failed to unblock user');
        },

        async makeAdmin(id) {
            await this._postAction(id, 'make-admin', 'user promoted to admin', 'failed to promote user');
        },

        async removeAdmin(id) {
            await this._postAction(id, 'remove-admin', 'admin role removed', 'failed to remove admin role');
        },

        async resetPassword(user) {
            this.loading = true;
            try {
                const res = await fetch(`/api/admin/users/${user.id}/reset-password`, { method: 'POST' });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'failed to reset password', 'error'); return; }
                this.tempPassword = { username: user.username, password: data.temp_password };
                this.showMessage('password reset', 'success');
                await this.loadUsers();
            } catch {
                this.showMessage('failed to reset password', 'error');
            } finally {
                this.loading = false;
            }
        },

        async copyTempPassword() {
            try {
                await navigator.clipboard.writeText(this.tempPassword.password);
                this.showMessage('password copied', 'success');
            } catch {
                this.showMessage('copy failed - select and copy it manually', 'error');
            }
        },

        async removeUser(id) {
            this.loading = true;
            try {
                const res = await fetch(`/api/admin/users/${id}`, { method: 'DELETE' });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'failed to remove user', 'error'); return; }
                this.showMessage('user removed', 'success');
                await this.loadUsers();
            } catch {
                this.showMessage('failed to remove user', 'error');
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
                this.showMessage('failed to load systems', 'error');
            }
        },

        async _sendLogo(id, file) {
            if (file.size > 256 * 1024) { this.showMessage('logo must be 256 KB or smaller', 'error'); return false; }
            const res = await fetch(`/api/admin/systems/${id}/logo`, { method: 'PUT', body: file });
            const data = await res.json();
            if (!res.ok) { this.showMessage(data.error || 'failed to upload logo', 'error'); return false; }
            return true;
        },

        async addSystem() {
            const name = this.newSystemName.trim();
            if (!name) return;
            const input = this.$refs.newSystemLogo;
            const file = input.files[0];
            if (file && file.size > 256 * 1024) { this.showMessage('logo must be 256 KB or smaller', 'error'); return; }
            this.loading = true;
            try {
                const res = await fetch('/api/admin/systems', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name }),
                });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'failed to add system', 'error'); return; }
                this.newSystemName = '';
                input.value = '';
                if (file && !(await this._sendLogo(data.id, file))) {
                    await this.loadSystems();
                    return;
                }
                this.showMessage('system added', 'success');
                await this.loadSystems();
            } catch {
                this.showMessage('failed to add system', 'error');
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
                if (!res.ok) { this.showMessage(data.error || 'failed to rename system', 'error'); return; }
                this.showMessage('system renamed', 'success');
                await this.loadSystems();
            } catch {
                this.showMessage('failed to rename system', 'error');
            } finally {
                this.loading = false;
            }
        },

        async uploadLogo(id, file) {
            if (!file) return;
            this.loading = true;
            try {
                if (await this._sendLogo(id, file)) this.showMessage('logo updated', 'success');
                await this.loadSystems();
            } catch {
                this.showMessage('failed to upload logo', 'error');
            } finally {
                this.loading = false;
            }
        },

        async removeLogo(id) {
            this.loading = true;
            try {
                const res = await fetch(`/api/admin/systems/${id}/logo`, { method: 'DELETE' });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'failed to remove logo', 'error'); return; }
                this.showMessage('logo removed', 'success');
                await this.loadSystems();
            } catch {
                this.showMessage('failed to remove logo', 'error');
            } finally {
                this.loading = false;
            }
        },

        async deleteSystem(id) {
            this.loading = true;
            try {
                const res = await fetch(`/api/admin/systems/${id}`, { method: 'DELETE' });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'failed to delete system', 'error'); return; }
                this.showMessage('system deleted', 'success');
                await this.loadSystems();
            } catch {
                this.showMessage('failed to delete system', 'error');
            } finally {
                this.loading = false;
            }
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
