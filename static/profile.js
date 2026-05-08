document.addEventListener("DOMContentLoaded", async () => {
    try {
        const userResponse = await fetch("/auth/current-user");
        if (!userResponse.ok) {
            window.location.href = "/login.html";
            return;
        }

        const user = await userResponse.json();
        const userName = document.getElementById("userName");
        if (userName) {
            userName.textContent = user.display_name || user.name || user.email || "User";
        }
    } catch (error) {
        console.error("Auth check failed:", error);
        window.location.href = "/login.html";
        return;
    }

    const logoutBtn = document.getElementById("logoutBtn");
    if (logoutBtn) {
        logoutBtn.addEventListener("click", logout);
    }

    loadProfile();
});

async function logout() {
    try {
        const response = await fetch("/auth/logout", { method: "POST" });
        if (response.ok) {
            window.location.href = "/login.html";
        }
    } catch (error) {
        showMessage("Logout failed: " + error.message, "error");
    }
}

async function loadProfile() {
    try {
        const params = new URLSearchParams(window.location.search);
        const userID = params.get("user_id");

        let url = "/api/profile";
        if (userID) {
            url += "?user_id=" + encodeURIComponent(userID);
        }

        const response = await fetch(url);
        if (!response.ok) {
            if (response.status === 404) {
                showMessage("User not found.", "error");
            } else {
                throw new Error("Failed to load profile");
            }
            return;
        }

        const profile = await response.json();
        renderProfile(profile);
    } catch (error) {
        showMessage("Error loading profile: " + error.message, "error");
    }
}

function renderProfile(profile) {
    const profileCard = document.getElementById("profileCard");
    const dangerZone = document.getElementById("dangerZone");

    profileCard.innerHTML = "";

    const card = document.createElement("div");
    card.className = "profile-content";

    // Avatar section
    const avatarSection = document.createElement("div");
    avatarSection.className = "profile-avatar-section";

    const avatarContainer = document.createElement("div");
    avatarContainer.className = "profile-avatar";

    if (profile.avatar && profile.avatar.has_picture && profile.avatar.picture) {
        const img = document.createElement("img");
        img.src = profile.avatar.picture;
        img.alt = profile.display_name || "User avatar";
        avatarContainer.appendChild(img);
    } else {
        const initials = document.createElement("div");
        initials.className = "profile-avatar-initials";
        initials.textContent = (profile.avatar && profile.avatar.initials) || "?";
        avatarContainer.appendChild(initials);
    }

    avatarSection.appendChild(avatarContainer);

    // User info section
    const infoSection = document.createElement("div");
    infoSection.className = "profile-info";

    const displayNameEl = document.createElement("h2");
    displayNameEl.className = "profile-display-name";
    displayNameEl.textContent = profile.display_name || "Unknown User";
    infoSection.appendChild(displayNameEl);

    if (profile.user && profile.user.name) {
        const nameRow = document.createElement("p");
        nameRow.className = "profile-meta";
        nameRow.innerHTML = `<strong>Name:</strong> ${escapeHtml(profile.user.name)}`;
        infoSection.appendChild(nameRow);
    }

    if (profile.user && profile.user.nickname) {
        const nicknameRow = document.createElement("p");
        nicknameRow.className = "profile-meta";
        nicknameRow.innerHTML = `<strong>Nickname:</strong> ${escapeHtml(profile.user.nickname)}`;
        infoSection.appendChild(nicknameRow);
    }

    if (profile.user && profile.user.email) {
        const emailRow = document.createElement("p");
        emailRow.className = "profile-meta";
        emailRow.innerHTML = `<strong>Email:</strong> ${escapeHtml(profile.user.email)}`;
        infoSection.appendChild(emailRow);
    }

    avatarSection.appendChild(infoSection);
    card.appendChild(avatarSection);

    profileCard.appendChild(card);

    // Show/hide danger zone based on is_self
    if (profile.is_self) {
        dangerZone.style.display = "block";
        const deleteBtn = document.getElementById("deleteBtn");
        if (deleteBtn) {
            deleteBtn.addEventListener("click", deleteProfile);
        }
    } else {
        dangerZone.style.display = "none";
    }

    // Render campaigns
    renderCampaignsList(
        profile.campaigns.dm || [],
        document.getElementById("dmCampaignsList")
    );
    renderCampaignsList(
        profile.campaigns.playing || [],
        document.getElementById("playingCampaignsList")
    );
}

function renderCampaignsList(campaigns, container) {
    container.innerHTML = "";

    if (!Array.isArray(campaigns) || campaigns.length === 0) {
        const empty = document.createElement("p");
        empty.textContent = "No campaigns.";
        container.appendChild(empty);
        return;
    }

    campaigns.forEach((campaign) => {
        const link = document.createElement("a");
        link.className = "campaign-link-button";
        link.href = `/campaigns#campaign-${campaign.id}`;
        link.textContent = campaign.title || "Untitled";
        container.appendChild(link);
    });
}

async function deleteProfile() {
    const confirmed = confirm(
        "Are you sure? This will permanently delete your account and remove you from all campaigns."
    );

    if (!confirmed) {
        return;
    }

    try {
        const response = await fetch("/api/profile", { method: "DELETE" });

        if (!response.ok) {
            const data = await response.json();
            throw new Error(data.message || "Failed to delete account");
        }

        const data = await response.json();
        showMessage(data.message || "Account deleted successfully", "success");

        // Redirect to login after a delay
        setTimeout(() => {
            window.location.href = "/login.html";
        }, 1500);
    } catch (error) {
        showMessage("Error deleting account: " + error.message, "error");
    }
}

function showMessage(text, type) {
    const messageDiv = document.getElementById("message");
    if (!messageDiv) {
        return;
    }

    messageDiv.className = "message " + type;
    messageDiv.textContent = text;
    messageDiv.style.display = "block";

    setTimeout(() => {
        messageDiv.style.display = "none";
    }, 5000);
}

function escapeHtml(text) {
    if (!text) {
        return "";
    }

    const map = {
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        "\"": "&quot;",
        "'": "&#039;"
    };

    return String(text).replace(/[&<>"']/g, (m) => map[m]);
}
