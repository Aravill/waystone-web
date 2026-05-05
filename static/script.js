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

    loadEvents();
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

async function loadEvents() {
    try {
        const response = await fetch('/api/events');
        if (!response.ok) throw new Error('Failed to load events');
        
        const events = await response.json();
        const eventsList = document.getElementById('eventsList');
        const eventSelect = document.getElementById('eventSelect');
        
        eventsList.innerHTML = '';
        eventSelect.innerHTML = '<option value="">-- Select an event --</option>';
        
        events.forEach(event => {
            const item = document.createElement('div');
            item.className = 'event-item';
            item.innerHTML = `
                <h3>${escapeHtml(event.name)}</h3>
                <p>${escapeHtml(event.description)}</p>
                <p><span class="event-date">${new Date(event.date).toLocaleDateString()}</span></p>
            `;
            eventsList.appendChild(item);
            
            const option = document.createElement('option');
            option.value = event.id;
            option.textContent = event.name;
            eventSelect.appendChild(option);
        });
    } catch (error) {
        showMessage('Error loading events: ' + error.message, 'error');
    }
}

async function handleSignup(e) {
    e.preventDefault();
    
    const formData = {
        event_id: document.getElementById('eventSelect').value,
        name: document.getElementById('name').value,
        email: document.getElementById('email').value,
        phone: document.getElementById('phone').value
    };
    
    if (!formData.event_id) {
        showMessage('Please select an event', 'error');
        return;
    }
    
    try {
        const response = await fetch('/api/signup', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(formData)
        });
        
        if (!response.ok) throw new Error('Signup failed');
        
        const result = await response.json();
        showMessage(result.message || 'Successfully signed up!', 'success');
        document.getElementById('signupForm').reset();
    } catch (error) {
        showMessage('Error: ' + error.message, 'error');
    }
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
