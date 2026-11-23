window.adminSystemsPage = function () {
    return {
        displayName: '',
        isAdmin: false,
        loading: false,
        systems: [],
        newSystemName: '',
        confirmModal: { visible: false, title: '', message: '', onConfirm: null },
        messageVisible: false,
        messageText: '',
        messageType: '',

        async init() {
            try {
                const res = await fetch('/auth/current-user');
                if (!res.ok) { window.location.href = '/login.html'; return; }
                const user = await res.json();
                this.displayName = user.display_name || user.username;
                this.isAdmin = (user.roles || []).includes('admin');
                if (!this.isAdmin) { window.location.href = '/'; return; }
                await this.loadSystems();
            } catch {
                window.location.href = '/login.html';
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
