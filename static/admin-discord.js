window.adminDiscordPage = function () {
    return {
        displayName: '',
        isAdmin: false,
        loaded: false,
        saving: false,
        data: {
            enabled: false, missing_env: [], guild_id: '', guild_name: '', invite_url: '', events: [],
            event_options: [], channels: [], channels_error: '', games: [], outbox: { queued: 0, failures: [] },
        },
        form: { guild_id: '', invite_url: '', events: [] },
        messageVisible: false,
        messageText: '',
        messageType: '',
        messageTimer: null,
        confirmDialog: { open: false, title: '', message: '', confirmLabel: '', resolve: null },

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
                const res = await fetch('/api/admin/discord');
                if (!res.ok) throw new Error('failed');
                const data = await res.json();
                this.data = data;
                this.form = {
                    guild_id: data.guild_id || '',
                    invite_url: data.invite_url || '',
                    events: [...(data.events || [])],
                };
            } catch {
                this.showMessage('Failed to load Discord settings', 'error');
            } finally {
                this.loaded = true;
            }
        },

        toggleEvent(key, on) {
            const rest = this.form.events.filter(e => e !== key);
            this.form.events = on ? [...rest, key] : rest;
        },

        // The channels the bot can post in, plus the game's current one if it dropped out of the list.
        channelOptions(game) {
            const list = this.data.channels || [];
            if (game.channel_id && !list.some(c => c.id === game.channel_id)) {
                return [...list, { id: game.channel_id, name: game.channel_name || game.channel_id }];
            }
            return list;
        },

        async save() {
            const guildChanged = this.form.guild_id.trim() !== this.data.guild_id;
            const withChannel = this.data.games.filter(g => g.channel_id).length;
            if (guildChanged && this.data.guild_id && withChannel > 0) {
                const ok = await this.askConfirm({
                    title: 'Change the server',
                    message: withChannel + (withChannel === 1 ? ' game' : ' games') + ' will lose their channel.',
                    confirmLabel: 'Change server',
                });
                if (!ok) return;
            }
            this.saving = true;
            try {
                const res = await fetch('/api/admin/discord', {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        guild_id: this.form.guild_id.trim(),
                        invite_url: this.form.invite_url.trim(),
                        events: this.form.events,
                    }),
                });
                const body = await res.json().catch(() => ({}));
                if (!res.ok) throw new Error(body.error || 'Failed to save');
                this.showMessage('Saved', 'success');
                await this.load();
            } catch (e) {
                this.showMessage(e.message || 'Failed to save', 'error');
            } finally {
                this.saving = false;
            }
        },

        async setChannel(game, event) {
            const el = event.target, before = game.channel_id || '';
            try {
                const res = await fetch('/api/admin/discord/games/' + encodeURIComponent(game.id), {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ channel_id: el.value }),
                });
                const body = await res.json().catch(() => ({}));
                if (!res.ok) throw new Error(body.error || 'Failed to save');
                game.channel_id = body.channel_id;
                game.channel_name = body.channel_name;
                this.showMessage('Saved', 'success');
            } catch (e) {
                el.value = before;
                this.showMessage(e.message || 'Failed to save', 'error');
            }
        },

        async testGame(game) {
            try {
                const res = await fetch('/api/admin/discord/games/' + encodeURIComponent(game.id) + '/test', { method: 'POST' });
                const body = await res.json().catch(() => ({}));
                if (!res.ok) throw new Error(body.error || 'Failed to send');
                this.showMessage('Test message sent', 'success');
            } catch (e) {
                this.showMessage(e.message || 'Failed to send', 'error');
            }
        },

        formatTime(s) {
            const t = new Date(s);
            return isNaN(t) ? s : t.toLocaleString();
        },

        askConfirm({ title, message, confirmLabel }) {
            if (this.confirmDialog.resolve) this.confirmDialog.resolve(false);
            return new Promise((resolve) => {
                this.confirmDialog = { open: true, title, message, confirmLabel: confirmLabel || 'Confirm', resolve };
                this.$nextTick(() => this.$refs.confirmCancelBtn?.focus());
            });
        },

        closeConfirm(result) {
            const resolve = this.confirmDialog.resolve;
            this.confirmDialog = { open: false, title: '', message: '', confirmLabel: '', resolve: null };
            if (resolve) resolve(result);
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
