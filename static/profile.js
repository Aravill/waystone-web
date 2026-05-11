// Profile Alpine component
window.profilePage = function() {
    return {
        displayName: 'User',
        loading: true,
        profile: null,
        dmCampaigns: [],
        playingCampaigns: [],
        isSelf: false,
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
                this.displayName = user.display_name || user.name || user.email || 'User';
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
                return;
            }
            
            await this.loadProfile();
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
            
            let avatarHtml = '';
            if (profile.avatar && profile.avatar.has_picture && profile.avatar.picture) {
                avatarHtml = `<img src="${escapeHtml(profile.avatar.picture)}" alt="${escapeHtml(displayName)}">`;
            } else {
                const initials = (profile.avatar && profile.avatar.initials) || '?';
                avatarHtml = `<div class="profile-avatar-initials">${escapeHtml(initials)}</div>`;
            }
            
            let metaHtml = '';
            if (profile.user && profile.user.name) {
                metaHtml += `<p class="profile-meta"><strong>Name:</strong> ${escapeHtml(profile.user.name)}</p>`;
            }
            if (profile.user && profile.user.nickname) {
                metaHtml += `<p class="profile-meta"><strong>Nickname:</strong> ${escapeHtml(profile.user.nickname)}</p>`;
            }
            if (profile.user && profile.user.email) {
                metaHtml += `<p class="profile-meta"><strong>Email:</strong> ${escapeHtml(profile.user.email)}</p>`;
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
