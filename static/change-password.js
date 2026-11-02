// Change password Alpine component
window.changePasswordPage = function() {
    return {
        mustChange: false,
        username: '',
        needsProfileSetup: false,
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
                this.needsProfileSetup = !!user.needs_profile_setup;
                this.username = user.username || '';
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

        // Mirrors auth.PasswordPolicyViolations; the server stays the source of truth.
        rules() {
            const pw = this.newPassword;
            const rules = [
                { text: 'at least 10 characters', ok: [...pw].length >= 10 },
                { text: 'at most 72 bytes (accented/non-Latin characters count as 2-4)', ok: new TextEncoder().encode(pw).length <= 72 },
                { text: 'a lowercase letter', ok: /\p{Ll}/u.test(pw) },
                { text: 'an uppercase letter', ok: /\p{Lu}/u.test(pw) },
                { text: 'a digit', ok: /\p{Nd}/u.test(pw) },
            ];
            if (this.username) {
                rules.push({ text: 'does not contain your username', ok: !pw.toLowerCase().includes(this.username.toLowerCase()) });
            }
            return rules;
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
                window.location.href = this.needsProfileSetup ? '/profile' : '/';
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
