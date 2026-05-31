window.adminUsersPage = function () {
    return {
        displayName: '',
        isAdmin: false,
        loading: false,
        users: [],
        searchQuery: '',
        newEmail: '',
        confirmModal: { visible: false, title: '', message: '', onConfirm: null },
        messageVisible: false,
        messageText: '',
        messageType: '',

        get filteredUsers() {
            const q = this.searchQuery.toLowerCase();
            if (!q) return this.users;
            return this.users.filter(u =>
                u.email.toLowerCase().includes(q) ||
                (u.name || '').toLowerCase().includes(q) ||
                (u.nickname || '').toLowerCase().includes(q)
            );
        },

        async init() {
            try {
                const res = await fetch('/auth/current-user');
                if (!res.ok) { window.location.href = '/login.html'; return; }
                const user = await res.json();
                this.displayName = user.display_name || user.email;
                this.isAdmin = (user.roles || []).includes('admin');
                if (!this.isAdmin) { window.location.href = '/'; return; }
                await this.loadUsers();
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
            if (!this.newEmail) return;
            this.loading = true;
            try {
                const res = await fetch('/api/admin/users', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ email: this.newEmail }),
                });
                const data = await res.json();
                if (!res.ok) { this.showMessage(data.error || 'failed to add user', 'error'); return; }
                this.newEmail = '';
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
