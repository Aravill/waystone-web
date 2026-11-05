window.guestPage = () => ({
    state: 'loading', // loading | ready | invalid | expired | error
    token: '',
    campaignTitle: '',
    displayName: '',
    username: '',
    newPassword: '',
    confirmPassword: '',
    saving: false,
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
            this.username = data.user.username;
            this.state = 'ready';
        } catch {
            this.state = 'error';
        }
    },

    rules() {
        return window.passwordRules(this.newPassword, this.username);
    },

    async submit() {
        if (this.newPassword !== this.confirmPassword) {
            this.showMessage('Passwords do not match', 'error');
            return;
        }
        this.saving = true;
        try {
            const res = await fetch(`/api/guest/${encodeURIComponent(this.token)}/password`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ new_password: this.newPassword }),
            });
            if (res.status === 404 || res.status === 410) { await this.load(); return; }
            const data = await res.json();
            if (!res.ok) { this.showMessage(data.error || 'failed to set password', 'error'); return; }
            window.location.href = '/campaigns?calendar=' + encodeURIComponent(data.campaign_id);
        } catch {
            this.showMessage('failed to set password', 'error');
        } finally {
            this.saving = false;
        }
    },

    showMessage(text, type) {
        this.messageText = text;
        this.messageType = type;
        this.messageVisible = true;
        setTimeout(() => { this.messageVisible = false; }, 5000);
    },
});
