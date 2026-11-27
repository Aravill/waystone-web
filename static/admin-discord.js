window.adminDiscordPage = function () {
    return {
        displayName: '',
        isAdmin: false,
        loaded: false,
        saving: false,
        data: {
            enabled: false, missing_env: [], guild_id: '', guild_name: '', invite_url: '', events: [],
            event_options: [], channels: [], voice_channels: [], channels_error: '', games: [], outbox: { queued: 0, failures: [] },
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
            // The "channel is gone" notification's link
            const gameId = new URLSearchParams(location.search).get('game');
            const row = this.data.games.findIndex((g) => g.id === gameId);
            if (row >= 0) {
                this.$nextTick(() => {
                    const select = document.querySelectorAll('.dc-game tr[data-game] select')[row];
                    select?.scrollIntoView({ block: 'center' });
                    select?.focus({ preventScroll: true });
                });
            }
        },

        async load() {
            try {
                const res = await fetch('/api/admin/discord');
                if (!res.ok) throw new Error('failed');
                const data = await res.json();
                data.games = (data.games || []).map((g) => ({ ...g, missing: '', uploading: false, coverVersion: 0 }));
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

        // The voice channels the bot can join, plus the game's current one if it dropped out of the list.
        voiceOptions(game) {
            const list = this.data.voice_channels || [];
            if (game.voice_id && !list.some(c => c.id === game.voice_id)) {
                return [...list, { id: game.voice_id, name: game.voice_name || game.voice_id }];
            }
            return list;
        },

        coverUrl(game) {
            return '/api/admin/discord/games/' + encodeURIComponent(game.id) + '/cover?v=' + game.coverVersion;
        },

        // PUT one or more of channel_id, events and voice_channel_id; reverts the control on failure.
        async putGame(game, body) {
            const res = await fetch('/api/admin/discord/games/' + encodeURIComponent(game.id), {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            });
            const out = await res.json().catch(() => ({}));
            if (!res.ok) {
                if (out.field) {
                    game.missing = out.field;
                    setTimeout(() => { game.missing = ''; }, 3000);
                }
                throw new Error(out.error || 'Failed to save');
            }
            Object.assign(game, {
                channel_id: out.channel_id, channel_name: out.channel_name,
                events: out.events, voice_id: out.voice_id, voice_name: out.voice_name,
            });
            this.showMessage('Saved', 'success');
        },

        async setEvents(game, event) {
            const el = event.target;
            try {
                await this.putGame(game, { events: el.checked });
            } catch (e) {
                el.checked = game.events;
                this.showMessage(e.message, 'error');
            }
        },

        async setVoice(game, event) {
            const el = event.target, before = game.voice_id || '';
            try {
                await this.putGame(game, { voice_channel_id: el.value });
            } catch (e) {
                el.value = before;
                this.showMessage(e.message, 'error');
            }
        },

        async uploadCover(game, event) {
            const file = event.target.files[0];
            event.target.value = '';
            if (!file) return;
            if (file.size > 8 * 1024 * 1024) {
                this.showMessage('The cover must be 8 MB or smaller', 'error');
                return;
            }
            game.uploading = true;
            try {
                const res = await fetch('/api/admin/discord/games/' + encodeURIComponent(game.id) + '/cover', {
                    method: 'POST',
                    headers: { 'Content-Type': file.type || 'application/octet-stream' },
                    body: file,
                });
                const out = await res.json().catch(() => ({}));
                if (!res.ok) throw new Error(out.error || 'Failed to upload the cover');
                game.cover = true;
                game.coverVersion++;
                this.showMessage('Cover saved', 'success');
            } catch (e) {
                this.showMessage(e.message || 'Failed to upload the cover', 'error');
            } finally {
                game.uploading = false;
            }
        },

        async removeCover(game) {
            try {
                const res = await fetch('/api/admin/discord/games/' + encodeURIComponent(game.id) + '/cover', { method: 'DELETE' });
                const out = await res.json().catch(() => ({}));
                if (!res.ok) throw new Error(out.error || 'Failed to remove the cover');
                game.cover = false;
                this.showMessage('Cover removed', 'success');
            } catch (e) {
                this.showMessage(e.message || 'Failed to remove the cover', 'error');
            }
        },

        async save() {
            const guildChanged = this.form.guild_id.trim() !== this.data.guild_id;
            const withChannel = this.data.games.filter(g => g.channel_id).length;
            if (guildChanged && this.data.guild_id && withChannel > 0) {
                const ok = await this.askConfirm({
                    title: 'Change the server',
                    message: withChannel + (withChannel === 1 ? ' game' : ' games') + ' will lose their channel, and every game loses its voice channel, event cover and Discord events.',
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
                await this.putGame(game, { channel_id: el.value });
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
