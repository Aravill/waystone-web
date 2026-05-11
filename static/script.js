// Signup Page Alpine component
window.signupPage = function() {
    return {
        displayName: 'User',
        messageVisible: false,
        messageText: '',
        messageType: '',
        formData: {
            name: '',
            email: '',
            phone: ''
        },
        
        async init() {
            try {
                const userResponse = await fetch('/auth/current-user');
                if (!userResponse.ok) {
                    if (userResponse.status === 401) {
                        window.location.href = '/login.html';
                        return;
                    }
                    throw new Error('Failed to fetch user');
                }
                const user = await userResponse.json();
                
                this.displayName = user.display_name || user.name || user.email || 'User';
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
                return;
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
        
        async handleSignup() {
            // Form submission stub - empty per original implementation
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
