window.adminAccessPage = function () {
    return {
        displayName: '',
        isAdmin: false,
        loaded: false,
        users: [],
        codices: [],
        access: {}, // "codexId|userId" -> "see" | "edit"
        messageVisible: false,
        messageText: '',
        messageType: '',
        messageTimer: null,

        async init() {
            try {
                const res = await fetch('/auth/current-user');
                if (!res.ok) { window.location.href = '/login.html'; return; }
                const user = await res.json();
                this.displayName = user.display_name || user.username;
                this.isAdmin = (user.roles || []).includes('admin');
                if (!this.isAdmin) { window.location.href = '/'; return; }
            } catch {
                window.location.href = '/login.html';
                return;
            }
            await this.load();
        },

        async load() {
            try {
                const res = await fetch('/api/admin/access');
                if (!res.ok) throw new Error('failed');
                const data = await res.json();
                this.users = data.users || [];
                this.codices = data.codices || [];
                this.access = {};
                (data.access || []).forEach(a => { this.access[a.codex_id + '|' + a.user_id] = a.level; });
            } catch {
                this.showMessage('Failed to load access rights', 'error');
            } finally {
                this.loaded = true;
            }
        },

        hasRight(user, action) {
            return (user.rights || []).includes(action);
        },

        level(codexId, userId) {
            return this.access[codexId + '|' + userId] || 'none';
        },

        async setRight(user, action, event) {
            const el = event.target, on = el.checked;
            try {
                const res = await fetch('/api/admin/access/users/' + encodeURIComponent(user.id), {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ action, on }),
                });
                if (!res.ok) {
                    const data = await res.json().catch(() => ({}));
                    throw new Error(data.error || 'Failed to save');
                }
                const rights = (user.rights || []).filter(a => a !== action);
                if (on) rights.push(action);
                user.rights = rights;
                this.showMessage('Saved', 'success');
            } catch (e) {
                el.checked = !on;
                this.showMessage(e.message || 'Failed to save', 'error');
            }
        },

        async setLevel(codex, user, event) {
            const el = event.target, key = codex.id + '|' + user.id;
            const before = this.level(codex.id, user.id), next = el.value;
            try {
                const res = await fetch('/api/admin/access/codices/' + encodeURIComponent(codex.id) + '/users/' + encodeURIComponent(user.id), {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ level: next }),
                });
                if (!res.ok) {
                    const data = await res.json().catch(() => ({}));
                    throw new Error(data.error || 'Failed to save');
                }
                if (next === 'none') delete this.access[key]; else this.access[key] = next;
                this.showMessage('Saved', 'success');
            } catch (e) {
                el.value = before;
                this.showMessage(e.message || 'Failed to save', 'error');
            }
        },

        showMessage(text, type) {
            this.messageText = text;
            this.messageType = type;
            this.messageVisible = true;
            clearTimeout(this.messageTimer);
            this.messageTimer = setTimeout(() => { this.messageVisible = false; }, type === 'error' ? 5000 : 1500);
        },

        async logout() {
            await fetch('/auth/logout', { method: 'POST' });
            window.location.href = '/login.html';
        },
    };
};
