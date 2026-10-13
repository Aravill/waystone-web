// Change password Alpine component
window.changePasswordPage = function() {
    return {
        mustChange: false,
        currentPassword: '',
        newPassword: '',
        confirmPassword: '',
        saving: false,
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
                const user = await response.json();
                if (user.is_env_admin) {
                    window.location.href = '/';
                    return;
                }
                this.mustChange = !!user.must_change_password;
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
            }
        },

        async submit() {
            if (this.newPassword !== this.confirmPassword) {
                this.showMessage('New passwords do not match', 'error');
                return;
            }
            this.saving = true;
            try {
                const response = await fetch('/auth/change-password', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        current_password: this.currentPassword,
                        new_password: this.newPassword,
                    }),
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to change password', 'error');
                    return;
                }
                window.location.href = '/';
            } catch (error) {
                this.showMessage('Failed to change password: ' + error.message, 'error');
            } finally {
                this.saving = false;
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
