// Profile Alpine component
window.profilePage = function() {
    return {
        displayName: 'User',
        isAdmin: false,
        loading: true,
        profile: null,
        dmCampaigns: [],
        playingCampaigns: [],
        isSelf: false,
        isEnvAdmin: false,
        calendarToken: '',
        messageVisible: false,
        messageText: '',
        messageType: '',
        
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
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
                return;
            }
            
            await this.loadProfile();
            if (this.isSelf) {
                await this.loadCalendarToken();
            }
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
            if (this.calendarToken && !confirm('Regenerate the link? The old link will stop working and you will need to re-subscribe.')) {
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
            } catch (error) {
                this.showMessage('Error loading profile: ' + error.message, 'error');
            } finally {
                this.loading = false;
            }
        },
        
        renderProfileHtml() {
            if (!this.profile) return '';
            
            const profile = this.profile;
            const displayName = profile.display_name || 'Unknown User';
            
            const initials = (profile.avatar && profile.avatar.initials) || '?';
            const avatarHtml = `<div class="profile-avatar-initials">${escapeHtml(initials)}</div>`;
            
            let metaHtml = '';
            if (profile.user && profile.user.name) {
                metaHtml += `<p class="profile-meta"><strong>Name:</strong> ${escapeHtml(profile.user.name)}</p>`;
            }
            if (profile.user && profile.user.nickname) {
                metaHtml += `<p class="profile-meta"><strong>Nickname:</strong> ${escapeHtml(profile.user.nickname)}</p>`;
            }
            if (profile.user && profile.user.username) {
                metaHtml += `<p class="profile-meta"><strong>Username:</strong> ${escapeHtml(profile.user.username)}</p>`;
            }
            
            return `
                <div class="profile-content">
                    <div class="profile-avatar-section">
                        <div class="profile-avatar">${avatarHtml}</div>
                        <div class="profile-info">
                            <h2 class="profile-display-name">${escapeHtml(displayName)}</h2>
                            ${metaHtml}
                        </div>
                    </div>
                </div>
            `;
        },
        
        async deleteProfile() {
            const confirmed = confirm(
                'Are you sure? This will permanently delete your account and remove you from all campaigns.'
            );
            
            if (!confirmed) {
                return;
            }
            
            try {
                const response = await fetch('/api/profile', { method: 'DELETE' });
                
                if (!response.ok) {
                    const data = await response.json();
                    throw new Error(data.message || 'Failed to delete account');
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

function escapeHtml(text) {
    if (!text) return '';
    const map = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#039;'
    };
    return String(text).replace(/[&<>"']/g, m => map[m]);
}
