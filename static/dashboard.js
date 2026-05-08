// Dashboard JavaScript

document.addEventListener('DOMContentLoaded', async function() {
    // Fetch and display user name
    try {
        const userResponse = await fetch('/auth/current-user');
        if (userResponse.ok) {
            const user = await userResponse.json();
            const userNameEl = document.getElementById('userName');
            if (userNameEl) {
                userNameEl.textContent = user.display_name || user.name || user.email || 'User';
            }
        }
    } catch (error) {
        console.error('Failed to fetch user info:', error);
    }

    const logoutBtn = document.getElementById('logoutBtn');
    
    if (logoutBtn) {
        logoutBtn.addEventListener('click', function() {
            handleLogout();
        });
    }
});

function handleLogout() {
    fetch('/auth/logout', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        }
    })
    .then(response => {
        if (response.ok) {
            window.location.href = '/login.html';
        } else {
            alert('Logout failed');
        }
    })
    .catch(error => {
        console.error('Error logging out:', error);
        alert('Logout failed');
    });
}
