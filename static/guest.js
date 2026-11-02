window.guestPage = () => ({
    state: 'loading', // loading | ready | invalid | expired
    token: '',
    campaignTitle: '',
    displayName: '',
    expiresAt: '',
    sessions: [],
    messageVisible: false,
    messageText: '',
    messageType: '',

    async init() {
        this.token = decodeURIComponent(location.pathname.split('/r/')[1] || '').split('/')[0];
        await this.load();
    },

    async load() {
        try {
            const res = await fetch(`/api/guest/${encodeURIComponent(this.token)}`);
            if (res.status === 404) { this.state = 'invalid'; return; }
            if (res.status === 410) { this.state = 'expired'; return; }
            if (!res.ok) throw new Error('failed');
            const data = await res.json();
            this.campaignTitle = data.campaign.title;
            this.displayName = data.user.display_name;
            this.expiresAt = data.expires_at;
            this.sessions = data.sessions || [];
            this.state = 'ready';
        } catch {
            this.showMessage('failed to load sessions', 'error');
        }
    },

    async respond(participation, session) {
        try {
            const res = await fetch(`/api/guest/${encodeURIComponent(this.token)}/sessions/${encodeURIComponent(session.id)}/responses`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ participation }),
            });
            if (res.status === 404 || res.status === 410) { await this.load(); return; }
            const data = await res.json();
            if (!res.ok) { this.showMessage(data.error || 'failed to send reply', 'error'); return; }
            this.showMessage('reply sent', 'success');
            await this.load();
        } catch {
            this.showMessage('failed to send reply', 'error');
        }
    },

    formatExpiry() {
        return new Date(this.expiresAt).toLocaleString('default', { dateStyle: 'medium', timeStyle: 'short' });
    },

    parseDateLocal(dateStr) {
        const [year, month, day] = String(dateStr).split('-').map(Number);
        return new Date(year, month - 1, day);
    },

    formatAgendaDate(session) {
        return this.parseDateLocal(session.date).toLocaleDateString('default', { weekday: 'short', day: 'numeric', month: 'short' });
    },

    getSessionTimeRange(session) {
        const time = session?.time || '';
        const duration = Number(session?.duration) || 0;
        const [hour, minute] = time.split(':').map(Number);
        if (Number.isNaN(hour) || Number.isNaN(minute)) return time;
        const total = (hour * 60) + minute + duration;
        const end = `${String(Math.floor((total % 1440) / 60)).padStart(2, '0')}:${String(total % 60).padStart(2, '0')}`;
        return `${time}-${end}`;
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

    getCounts(s) {
        return `✓${s.accepted_count || 0} ?${s.tentative_count || 0} ✗${s.declined_count || 0} · ${s.pending_count || 0} pending`;
    },

    showMessage(text, type) {
        this.messageText = text;
        this.messageType = type;
        this.messageVisible = true;
        setTimeout(() => { this.messageVisible = false; }, 5000);
    },
});
