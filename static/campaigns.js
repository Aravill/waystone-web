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
            campaignsList.appendChild(renderCampaign(campaign));
        });
    } catch (error) {
        showMessage("Error loading campaigns: " + error.message, "error");
    }
}

function renderCampaign(campaign) {
    const title = campaign?.title || "Untitled Campaign";
    const summary = campaign?.summary || "No summary available.";
    const description = campaign?.description || "";
    const status = campaign?.status || "Unknown";
    const dm = campaign?.dm || "Unknown";
    const signUpsOpen = campaign?.sign_ups_open === true;
    const nextSession = "N/A";
    const players = Array.isArray(campaign?.players) ? campaign.players : [];

    const card = document.createElement("article");
    card.className = "campaign-item";

    // Row 1: title + summary toggle + optional long description
    const topRow = document.createElement("section");
    topRow.className = "campaign-row campaign-row-top";

    const titleEl = document.createElement("h3");
    titleEl.className = "campaign-title";
    titleEl.textContent = title;

    const summaryRow = document.createElement("div");
    summaryRow.className = "campaign-summary-row";

    const summaryText = document.createElement("p");
    summaryText.className = "campaign-summary-text";
    summaryText.textContent = summary;

    const summaryArrowBtn = document.createElement("button");
    summaryArrowBtn.type = "button";
    summaryArrowBtn.className = "campaign-summary-arrow";
    summaryArrowBtn.setAttribute("aria-expanded", "false");
    summaryArrowBtn.setAttribute("aria-label", "Expand campaign description");
    summaryArrowBtn.textContent = "▼";

    const descriptionEl = document.createElement("p");
    descriptionEl.className = "campaign-description";
    descriptionEl.textContent = description || "No additional description.";

    summaryArrowBtn.addEventListener("click", () => {
        const expanded = summaryArrowBtn.getAttribute("aria-expanded") === "true";
        summaryArrowBtn.setAttribute("aria-expanded", String(!expanded));
        summaryArrowBtn.setAttribute("aria-label", expanded ? "Expand campaign description" : "Collapse campaign description");
        summaryArrowBtn.classList.toggle("campaign-summary-arrow-expanded", !expanded);
        descriptionEl.classList.toggle("campaign-description-expanded", !expanded);
    });

    summaryRow.appendChild(summaryText);
    summaryRow.appendChild(summaryArrowBtn);

    topRow.appendChild(titleEl);
    topRow.appendChild(summaryRow);
    topRow.appendChild(descriptionEl);

    // Row 2: two columns
    const middleRow = document.createElement("section");
    middleRow.className = "campaign-row campaign-row-middle";

    const leftCol = document.createElement("div");
    leftCol.className = "campaign-column campaign-column-left";
    leftCol.appendChild(createField("Status", status));
    leftCol.appendChild(createField("Dungeon Master", dm, true));
    leftCol.appendChild(createField("Sign-ups", signUpsOpen ? "Open" : "Closed"));

    const rightCol = document.createElement("div");
    rightCol.className = "campaign-column campaign-column-right";
    rightCol.appendChild(createField("Next Session", nextSession));
    rightCol.appendChild(createPlayersField(players));

    middleRow.appendChild(leftCol);
    middleRow.appendChild(rightCol);

    // Row 3: action buttons
    const bottomRow = document.createElement("section");
    bottomRow.className = "campaign-row campaign-row-bottom";
    bottomRow.appendChild(createActionButton("Codex", true));
    bottomRow.appendChild(createActionButton("Calendar", true));
    bottomRow.appendChild(createActionButton("Sign-up", !signUpsOpen));

    card.appendChild(topRow);
    card.appendChild(middleRow);
    card.appendChild(bottomRow);

    return card;
}

function createField(label, value, plate = false) {
    const field = document.createElement("div");
    field.className = "campaign-field";

    const labelEl = document.createElement("h4");
    labelEl.className = "campaign-label";
    labelEl.textContent = label;

    const valueEl = document.createElement("p");
    valueEl.className = plate ? "campaign-value campaign-nameplate" : "campaign-value";
    valueEl.textContent = value;

    field.appendChild(labelEl);
    field.appendChild(valueEl);
    return field;
}

function createPlayersField(players) {
    const field = document.createElement("div");
    field.className = "campaign-field";

    const labelEl = document.createElement("h4");
    labelEl.className = "campaign-label";
    labelEl.textContent = "Players";

    const list = document.createElement("div");
    list.className = "campaign-players";

    if (players.length === 0) {
        const empty = document.createElement("p");
        empty.className = "campaign-value";
        empty.textContent = "No players yet";
        list.appendChild(empty);
    } else {
        players.forEach((player) => {
            const plate = document.createElement("span");
            plate.className = "campaign-nameplate";
            plate.textContent = player;
            list.appendChild(plate);
        });
    }

    field.appendChild(labelEl);
    field.appendChild(list);
    return field;
}

function createActionButton(label, disabled) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "campaign-action-btn";
    button.textContent = label;
    button.disabled = disabled;
    return button;
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
