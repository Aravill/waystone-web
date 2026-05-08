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
            userName.textContent = user.name || user.email || "User";
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

    loadCampaigns();
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

async function loadCampaigns() {
    try {
        const response = await fetch("/api/campaigns");
        if (!response.ok) {
            throw new Error("Failed to load campaigns");
        }

        const campaigns = await response.json();
        const campaignsList = document.getElementById("campaignsList");
        if (!campaignsList) {
            return;
        }

        campaignsList.innerHTML = "";
        if (!Array.isArray(campaigns) || campaigns.length === 0) {
            campaignsList.innerHTML = "<p>No campaigns available.</p>";
            return;
        }

        campaigns.forEach((campaign) => {
            const item = document.createElement("div");
            item.className = "campaign-item";

            const players = Array.isArray(campaign.players) && campaign.players.length > 0
                ? campaign.players.map((player) => escapeHtml(player)).join(", ")
                : "No players yet";

            item.innerHTML = `
                <h3>${escapeHtml(campaign.title)}</h3>
                <p class="campaign-status"><strong>Status:</strong> ${escapeHtml(campaign.status)}</p>
                <p><strong>Summary:</strong> ${escapeHtml(campaign.summary)}</p>
                <p>${escapeHtml(campaign.description)}</p>
                <p><strong>DM:</strong> ${escapeHtml(campaign.dm)}</p>
                <p><strong>Players:</strong> ${players}</p>
                <p><strong>Sign-ups Open:</strong> ${campaign.sign_ups_open ? "Yes" : "No"}</p>
            `;

            campaignsList.appendChild(item);
        });
    } catch (error) {
        showMessage("Error loading campaigns: " + error.message, "error");
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
