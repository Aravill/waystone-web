// Dashboard Alpine component
window.dashboardPage = function() {
    return {
        displayName: 'User',
        isAdmin: false,
        
        async init() {
            try {
                const userResponse = await fetch('/auth/current-user');
                if (userResponse.ok) {
                    const user = await userResponse.json();
                    this.displayName = user.display_name || user.name || user.email || 'User';
                    this.isAdmin = (user.roles || []).includes('admin');
                } else {
                    window.location.href = '/login.html';
                }
            } catch (error) {
                console.error('Failed to fetch user info:', error);
                window.location.href = '/login.html';
            }
        },
        
        async handleLogout() {
            try {
                const response = await fetch('/auth/logout', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    }
                });
                if (response.ok) {
                    window.location.href = '/login.html';
                } else {
                    alert('Logout failed');
                }
            } catch (error) {
                console.error('Error logging out:', error);
                alert('Logout failed');
            }
        }
    };
};
