document.addEventListener('DOMContentLoaded', async () => {
    // Check authentication status and display user name
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
        
        // Display user name in header
        const userName = document.getElementById('userName');
        if (userName) {
            userName.textContent = user.name || user.email || 'User';
        }
        
        // Set up logout button
        const logoutBtn = document.getElementById('logoutBtn');
        if (logoutBtn) {
            logoutBtn.addEventListener('click', logout);
        }
    } catch (error) {
        console.error('Auth check failed:', error);
        window.location.href = '/login.html';
        return;
    }

    document.getElementById('signupForm').addEventListener('submit', handleSignup);
});

async function logout() {
    try {
        const response = await fetch('/auth/logout', { method: 'POST' });
        if (response.ok) {
            window.location.href = '/login.html';
        }
    } catch (error) {
        showMessage('Logout failed: ' + error.message, 'error');
    }
}

async function handleSignup(e) {
    e.preventDefault();
}

function showMessage(text, type) {
    const messageDiv = document.getElementById('message');
    messageDiv.className = 'message ' + type;
    messageDiv.textContent = text;
    messageDiv.style.display = 'block';
    
    setTimeout(() => {
        messageDiv.style.display = 'none';
    }, 5000);
}

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
