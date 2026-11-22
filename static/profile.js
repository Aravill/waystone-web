// Profile Alpine component
window.profilePage = function() {
    return {
        displayName: 'User',
        isAdmin: false,
        loading: true,
        profile: null,
        dmCampaigns: [],
        playingCampaigns: [],
        characters: [],
        isSelf: false,
        isEnvAdmin: false,
        needsProfileSetup: false,
        calendarToken: '',
        discordEnabled: false,
        discord: null,
        discordOffer: false,
        discordImport: { name: true, avatar: true },
        discordImporting: false,
        editing: null,
        editValue: '',
        saving: false,
        avatarMenuOpen: false,
        accountOpen: false,
        gamesOpen: true,
        charactersOpen: true,
        integrationsOpen: false,
        messageVisible: false,
        messageText: '',
        messageType: '',
        confirmDialog: { open: false, title: '', message: '', confirmLabel: '', resolve: null },
        
        async init() {
            try {
                const userResponse = await fetch('/auth/current-user');
                if (!userResponse.ok) {
                    window.location.href = '/login.html';
                    return;
                }
                
                const user = await userResponse.json();
                this.displayName = user.display_name || user.username;
                this.isAdmin = (user.roles || []).includes('admin');
                this.isEnvAdmin = !!user.is_env_admin;
                this.needsProfileSetup = !!user.needs_profile_setup;
                this.discordEnabled = !!user.discord_enabled;
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
                return;
            }
            
            await this.loadProfile();
            if (this.isSelf) {
                await this.loadCalendarToken();
                if (this.discordEnabled) {
                    await this.loadDiscord();
                }
                this.showDiscordResult();
            }
        },

        // ?discord=... is where the OAuth callback sends the user back to.
        showDiscordResult() {
            const params = new URLSearchParams(window.location.search);
            const result = params.get('discord');
            if (!result) return;
            const texts = {
                linked: ['Discord connected.', 'success'],
                taken: ['This Discord account is linked to another Waystone account.', 'error'],
                failed: ['Could not connect Discord. Try again.', 'error'],
                off: ["Discord isn't set up.", 'error']
            };
            if (texts[result]) this.showMessage(texts[result][0], texts[result][1]);
            this.discordOffer = result === 'linked';
            this.integrationsOpen = true;
            params.delete('discord');
            const qs = params.toString();
            history.replaceState(null, '', window.location.pathname + (qs ? '?' + qs : '') + window.location.hash);
        },

        get discordErrorTime() {
            const t = this.discord && this.discord.dm_error_at ? new Date(this.discord.dm_error_at) : null;
            return t && !isNaN(t) ? t.toLocaleString() : '';
        },

        async loadDiscord() {
            try {
                const response = await fetch('/api/profile/discord');
                if (!response.ok) {
                    throw new Error('Failed to load Discord settings');
                }
                this.discord = await response.json();
            } catch (error) {
                this.showMessage('Error loading Discord: ' + error.message, 'error');
            }
        },

        async saveDiscordToggles() {
            try {
                const t = this.discord.toggles;
                const response = await fetch('/api/profile/discord', {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ sessions: !!t.sessions, signups: !!t.signups, characters: !!t.characters })
                });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.error || 'Failed to save');
                }
                this.showMessage('Saved.', 'success');
            } catch (error) {
                this.showMessage('Error saving Discord settings: ' + error.message, 'error');
                await this.loadDiscord();
            }
        },

        // Right after linking: copy the Discord display name and/or picture into the profile.
        async importDiscord() {
            if (!this.discordImport.name && !this.discordImport.avatar) { this.discordOffer = false; return; }
            this.discordImporting = true;
            try {
                const res = await fetch('/api/profile/discord/import', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(this.discordImport)
                });
                const data = await res.json().catch(() => ({}));
                if (!res.ok) throw new Error(data.error || 'Could not copy from Discord.');
                location.reload();
            } catch (e) {
                this.showMessage(e.message, 'error');
            } finally {
                this.discordImporting = false;
            }
        },

        async testDiscord() {
            try {
                const response = await fetch('/api/profile/discord/test', { method: 'POST' });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.error || 'Failed to send');
                }
                this.showMessage('Test DM sent.', 'success');
            } catch (error) {
                this.showMessage(error.message, 'error');
            }
            await this.loadDiscord();
        },

        async disconnectDiscord() {
            if (!(await this.askConfirm({
                title: 'Disconnect Discord',
                message: 'Stop sending Waystone notifications to your Discord DMs?',
                confirmLabel: 'Disconnect'
            }))) {
                return;
            }
            try {
                const response = await fetch('/api/profile/discord', { method: 'DELETE' });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.error || 'Failed to disconnect');
                }
                this.showMessage('Discord disconnected.', 'success');
            } catch (error) {
                this.showMessage('Error disconnecting Discord: ' + error.message, 'error');
            }
            await this.loadDiscord();
        },

        get calendarUrl() {
            return this.calendarToken ? `${window.location.origin}/calendar/${this.calendarToken}.ics` : '';
        },

        get googleCalendarUrl() {
            if (!this.calendarToken) return '';
            return 'https://calendar.google.com/calendar/r?cid=' + encodeURIComponent(this.calendarUrl.replace(/^https?:/, 'webcal:'));
        },

        async loadCalendarToken() {
            try {
                const response = await fetch('/api/profile/calendar');
                if (!response.ok) {
                    throw new Error('Failed to load calendar link');
                }
                const data = await response.json();
                this.calendarToken = data.token || '';
            } catch (error) {
                this.showMessage('Error loading calendar link: ' + error.message, 'error');
            }
        },

        async enableOrRegenerateCalendar() {
            if (this.calendarToken && !(await this.askConfirm({
                title: 'Regenerate link',
                message: 'Regenerate the link? The old link will stop working and you will need to re-subscribe.',
                confirmLabel: 'Regenerate'
            }))) {
                return;
            }
            try {
                const response = await fetch('/api/profile/calendar', { method: 'POST' });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.error || 'Failed to create calendar link');
                }
                this.calendarToken = data.token || '';
                this.showMessage('Calendar link ready.', 'success');
            } catch (error) {
                this.showMessage('Error creating calendar link: ' + error.message, 'error');
            }
        },

        async copyCalendarUrl() {
            try {
                if (navigator.clipboard && window.isSecureContext) {
                    await navigator.clipboard.writeText(this.calendarUrl);
                } else {
                    const input = document.getElementById('calendarUrl');
                    input.select();
                    if (!document.execCommand('copy')) {
                        throw new Error('copy not supported');
                    }
                }
                this.showMessage('Link copied.', 'success');
            } catch (error) {
                this.showMessage('Could not copy, select the link manually.', 'error');
            }
        },
        
        async handleLogout() {
            try {
                const response = await fetch('/auth/logout', { method: 'POST' });
                if (response.ok) {
                    window.location.href = '/login.html';
                }
            } catch (error) {
                this.showMessage('Logout failed: ' + error.message, 'error');
            }
        },
        
        async loadProfile() {
            this.loading = true;
            try {
                const params = new URLSearchParams(window.location.search);
                const userID = params.get('user_id');
                
                let url = '/api/profile';
                if (userID) {
                    url += '?user_id=' + encodeURIComponent(userID);
                }
                
                const response = await fetch(url);
                if (!response.ok) {
                    if (response.status === 404) {
                        this.showMessage('User not found.', 'error');
                    } else {
                        throw new Error('Failed to load profile');
                    }
                    return;
                }
                
                const profile = await response.json();
                this.profile = profile;
                this.isSelf = profile.is_self || false;
                this.dmCampaigns = Array.isArray(profile.campaigns?.dm) ? profile.campaigns.dm : [];
                this.playingCampaigns = Array.isArray(profile.campaigns?.playing) ? profile.campaigns.playing : [];
                this.characters = Array.isArray(profile.characters) ? profile.characters : [];
            } catch (error) {
                this.showMessage('Error loading profile: ' + error.message, 'error');
            } finally {
                this.loading = false;
            }
        },
        
        statusHtml: characterStatusHtml,

        get memberSince() {
            const ms = this.profile && this.profile.stats && this.profile.stats.member_since;
            if (!ms) return '\u2014';
            return new Date(ms).toLocaleDateString('default', { month: 'short', year: 'numeric' });
        },

        statusClass(status) {
            return String(status || '').toLowerCase().replace(/[^a-z]/g, '');
        },

        avatarClick() {
            if (this.profile && this.profile.avatar && this.profile.avatar.url) {
                this.avatarMenuOpen = !this.avatarMenuOpen;
            } else {
                this.$refs.avatarInput.click();
            }
        },

        startEdit(field) {
            this.editing = field;
            this.editValue = (this.profile.user && this.profile.user[field]) || '';
        },

        cancelEdit() {
            this.editing = null;
        },

        async saveEdit() {
            const field = this.editing;
            if (!field || this.saving) return;
            const value = this.editValue.trim();
            if (value === ((this.profile.user && this.profile.user[field]) || '')) {
                this.editing = null;
                return;
            }
            this.saving = true;
            try {
                const response = await fetch('/api/profile', {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ [field]: value })
                });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.error || 'Failed to update profile');
                }
                this.profile.user.name = data.name;
                this.profile.user.nickname = data.nickname;
                this.profile.display_name = data.display_name;
                this.displayName = data.display_name || this.displayName;
                this.editing = null;
                this.needsProfileSetup = false;
                this.showMessage('Profile updated', 'success');
            } catch (error) {
                this.showMessage(error.message, 'error');
            } finally {
                this.saving = false;
            }
        },

        async finishSetup() {
            try {
                const response = await fetch('/api/profile/setup-complete', { method: 'POST' });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.error || 'Failed to finish profile setup');
                }
                this.needsProfileSetup = false;
            } catch (error) {
                this.showMessage(error.message, 'error');
            }
        },

        async uploadAvatar(event) {
            const input = event.target;
            const file = input.files && input.files[0];
            if (!file) return;
            try {
                if (file.size > 1024 * 1024) {
                    throw new Error('avatar must be 1 MB or smaller');
                }
                const response = await fetch('/api/profile/avatar', {
                    method: 'PUT',
                    headers: { 'Content-Type': file.type },
                    body: file
                });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.error || 'Failed to upload avatar');
                }
                this.needsProfileSetup = false;
                this.showMessage(data.message || 'Avatar updated', 'success');
                window.refreshHeaderAvatar?.();
                await this.loadProfile();
            } catch (error) {
                this.showMessage('Error uploading avatar: ' + error.message, 'error');
            } finally {
                input.value = '';
            }
        },

        async removeAvatar() {
            if (!(await this.askConfirm({
                title: 'Remove avatar',
                message: 'Remove your avatar? The first letter of your nickname will be shown instead.',
                confirmLabel: 'Remove'
            }))) {
                return;
            }
            try {
                const response = await fetch('/api/profile/avatar', { method: 'DELETE' });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.error || 'Failed to remove avatar');
                }
                this.showMessage(data.message || 'Avatar removed', 'success');
                window.refreshHeaderAvatar?.();
                await this.loadProfile();
            } catch (error) {
                this.showMessage('Error removing avatar: ' + error.message, 'error');
            }
        },

        // askConfirm opens the confirm modal and resolves to true on confirm, false on cancel.
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

        async deleteProfile() {
            const confirmed = await this.askConfirm({
                title: 'Delete account',
                message: "Are you sure? Your account will be anonymised: you'll be removed from all games, your name and avatar will be erased, and you won't be able to log in again. Past session history is kept as 'Deleted user'.",
                confirmLabel: 'Delete'
            });
            
            if (!confirmed) {
                return;
            }
            
            try {
                const response = await fetch('/api/profile', { method: 'DELETE' });
                
                if (!response.ok) {
                    const data = await response.json();
                    throw new Error(data.error || data.message || 'Failed to delete account');
                }
                
                const data = await response.json();
                this.showMessage(data.message || 'Account deleted successfully', 'success');
                
                // Redirect to login after a delay
                setTimeout(() => {
                    window.location.href = '/login.html';
                }, 1500);
            } catch (error) {
                this.showMessage('Error deleting account: ' + error.message, 'error');
            }
        },
        
        showMessage(text, type) {
            this.messageText = text;
            this.messageType = type;
            this.messageVisible = true;
            
            setTimeout(() => {
                this.messageVisible = false;
            }, 5000);
        }
    };
};
