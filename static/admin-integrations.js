// A calendar app that hasn't read the link for this long no longer counts as using it.
const CALENDAR_IDLE_MS = 7 * 24 * 60 * 60 * 1000;

window.adminIntegrationsPage = function () {
    return {
        displayName: '',
        isAdmin: false,
        loaded: false,
        users: [],
        discordEnabled: false,
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
            } catch {
                window.location.href = '/login.html';
                return;
            }
            await this.load();
        },

        async load() {
            try {
                const res = await fetch('/api/admin/integrations');
                if (!res.ok) throw new Error('failed');
                const data = await res.json();
                this.users = data.users || [];
                this.discordEnabled = !!data.discord_enabled;
                this.loaded = true;
            } catch {
                this.showMessage('Failed to load integrations', 'error');
            }
        },

        get calendarCount() {
            return this.users.filter(u => this.calendarState(u).tone === 'ok').length;
        },

        get discordCount() {
            return this.users.filter(u => u.discord).length;
        },

        calendarState(u) {
            if (!u.calendar) return { label: '—', tone: 'muted', note: '' };
            const read = u.calendar.last_read_at;
            if (!read) return { label: '○ Not read yet', tone: 'muted', note: 'Link made ' + this.formatDate(u.calendar.created_at) };
            const note = 'Last read ' + this.formatTime(read);
            if (Date.now() - new Date(read) > CALENDAR_IDLE_MS) return { label: '○ Idle', tone: 'muted', note };
            return { label: '● In use', tone: 'ok', note };
        },

        discordState(u) {
            const d = u.discord;
            if (!d) return { label: '—', tone: 'muted', note: '' };
            const name = '@' + d.username;
            if (d.dm_error) return { label: '✗ ' + name, tone: 'danger', note: 'DMs don\'t arrive since ' + this.formatTime(d.dm_error_at) };
            return {
                label: '● ' + name,
                tone: 'ok',
                note: d.topics.length ? 'DMs: ' + d.topics.join(', ') : 'All DMs turned off',
            };
        },

        formatTime(s) {
            const t = new Date(s);
            return isNaN(t) ? s : t.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' });
        },

        formatDate(s) {
            const t = new Date(s);
            return isNaN(t) ? s : t.toLocaleDateString([], { dateStyle: 'medium' });
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
