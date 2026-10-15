// First-login profile setup Alpine component
window.welcomePage = function() {
    return {
        nickname: '',
        savedNickname: '',
        avatarUrl: '',
        initials: '',
        busy: false,
        messageVisible: false,
        messageText: '',
        messageType: '',

        async init() {
            try {
                const response = await fetch('/auth/current-user');
                if (!response.ok) {
                    window.location.href = '/login.html';
                    return;
                }
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
                return;
            }
            await this.loadProfile(true);
        },

        async loadProfile(initial) {
            try {
                const response = await fetch('/api/profile');
                if (!response.ok) {
                    throw new Error('Failed to load profile');
                }
                const profile = await response.json();
                this.avatarUrl = (profile.avatar && profile.avatar.url) || '';
                this.initials = (profile.avatar && profile.avatar.initials) || '';
                if (initial) {
                    this.savedNickname = (profile.user && profile.user.nickname) || '';
                    this.nickname = this.savedNickname || (profile.user && profile.user.name) || '';
                }
            } catch (error) {
                this.showMessage('Error loading profile: ' + error.message, 'error');
            }
        },

        async uploadAvatar(event) {
            const input = event.target;
            const file = input.files && input.files[0];
            if (!file) return;
            this.busy = true;
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
                await this.loadProfile(false);
            } catch (error) {
                this.showMessage('Error uploading avatar: ' + error.message, 'error');
            } finally {
                input.value = '';
                this.busy = false;
            }
        },

        async removeAvatar() {
            this.busy = true;
            try {
                const response = await fetch('/api/profile/avatar', { method: 'DELETE' });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.error || 'Failed to remove avatar');
                }
                await this.loadProfile(false);
            } catch (error) {
                this.showMessage('Error removing avatar: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        },

        async complete() {
            const response = await fetch('/api/profile/setup-complete', { method: 'POST' });
            const data = await response.json();
            if (!response.ok) {
                throw new Error(data.error || 'Failed to complete setup');
            }
            window.location.href = '/';
        },

        async submit() {
            this.busy = true;
            try {
                const nickname = this.nickname.trim();
                if (nickname !== this.savedNickname) {
                    const response = await fetch('/api/profile', {
                        method: 'PUT',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ nickname: nickname }),
                    });
                    const data = await response.json();
                    if (!response.ok) {
                        throw new Error(data.error || 'Failed to save nickname');
                    }
                }
                await this.complete();
            } catch (error) {
                this.showMessage(error.message, 'error');
                this.busy = false;
            }
        },

        async skip() {
            this.busy = true;
            try {
                await this.complete();
            } catch (error) {
                this.showMessage(error.message, 'error');
                this.busy = false;
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

        showMessage(text, type) {
            this.messageText = text;
            this.messageType = type;
            this.messageVisible = true;
            setTimeout(() => { this.messageVisible = false; }, 5000);
        }
    };
};
