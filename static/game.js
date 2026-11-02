// Game detail page. Relies on campaigns.js (loaded first) for CAMPAIGN_TYPES, TYPE_ICON_PATHS,
// campaignStatusClass, userAvatarHtml, escapeHtml and getDefaultTimezone.

// Basic Markdown, safe by construction: the whole input is HTML-escaped first, then only
// whitelisted tags are produced. Supports paragraphs, **bold**, *italic*/_italic_, [text](url),
// "- " / "1. " lists, "#" headings (rendered as h3/h4) and "> " quotes.
function renderMarkdown(text) {
    const src = escapeHtml(String(text || '').replace(/\0/g, '').replace(/\r\n?/g, '\n'));
    const inline = (line) => {
        const links = [];
        let out = line.replace(/\[([^\]\n]+)\]\(([^)\s]+)\)/g, (m, label, url) => {
            if (!/^(https?:\/\/|mailto:)\S+$/i.test(url)) return m;
            links.push(`<a href="${url}" target="_blank" rel="noopener noreferrer nofollow">${label}</a>`);
            return `\0${links.length - 1}\0`;
        });
        out = out
            .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
            .replace(/(^|[^*\w])\*([^*\s][^*\n]*?)\*(?![*\w])/g, '$1<em>$2</em>')
            .replace(/(^|[^\w])_([^_\s][^_\n]*?)_(?!\w)/g, '$1<em>$2</em>');
        return out.replace(/\0(\d+)\0/g, (m, i) => links[Number(i)]);
    };
    const blocks = src.split(/\n[ \t]*\n/).map((b) => b.trim()).filter(Boolean);
    return blocks.map((block) => {
        const lines = block.split('\n');
        if (lines.every((l) => /^[-*]\s+/.test(l))) {
            return '<ul>' + lines.map((l) => `<li>${inline(l.replace(/^[-*]\s+/, ''))}</li>`).join('') + '</ul>';
        }
        if (lines.every((l) => /^\d+\.\s+/.test(l))) {
            return '<ol>' + lines.map((l) => `<li>${inline(l.replace(/^\d+\.\s+/, ''))}</li>`).join('') + '</ol>';
        }
        if (lines.every((l) => /^&gt;\s?/.test(l))) {
            return '<blockquote>' + lines.map((l) => inline(l.replace(/^&gt;\s?/, ''))).join('<br>') + '</blockquote>';
        }
        const head = lines[0].match(/^(#{1,3})\s+(.*)$/);
        if (head) {
            const tag = head[1].length === 3 ? 'h4' : 'h3';
            const rest = lines.slice(1);
            return `<${tag}>${inline(head[2])}</${tag}>` + (rest.length ? `<p>${rest.map(inline).join('<br>')}</p>` : '');
        }
        return `<p>${lines.map(inline).join('<br>')}</p>`;
    }).join('');
}

function toRoman(n) {
    const map = [[1000, 'M'], [900, 'CM'], [500, 'D'], [400, 'CD'], [100, 'C'], [90, 'XC'], [50, 'L'], [40, 'XL'], [10, 'X'], [9, 'IX'], [5, 'V'], [4, 'IV'], [1, 'I']];
    let out = '';
    for (const [v, s] of map) {
        while (n >= v) { out += s; n -= v; }
    }
    return out;
}

function parseDateLocal(dateStr) {
    const [year, month, day] = String(dateStr).split('-').map(Number);
    return new Date(year, month - 1, day);
}

function getSessionEndTime(session) {
    const time = session?.time || '';
    const duration = Number(session?.duration) || 0;
    const [hour, minute] = String(time).split(':').map((part) => Number(part));
    if (Number.isNaN(hour) || Number.isNaN(minute)) return time;
    const totalMinutes = (hour * 60) + minute + duration;
    const endHour = Math.floor((totalMinutes % (24 * 60)) / 60);
    const endMinute = totalMinutes % 60;
    return `${String(endHour).padStart(2, '0')}:${String(endMinute).padStart(2, '0')}`;
}

function getDurationLabel(session) {
    const total = Number(session?.duration) || 0;
    const hours = Math.floor(total / 60);
    const minutes = total % 60;
    if (hours && minutes) return `${hours}h ${minutes}m`;
    if (hours) return `${hours}h`;
    return `${minutes}m`;
}

window.gamePage = function () {
    return {
        displayName: 'User',
        isAdmin: false,
        currentUserId: '',
        gameId: '',
        campaign: null,
        loading: true,
        notFound: false,
        showAll: false,
        messageVisible: false,
        messageText: '',
        messageType: '',
        confirmDialog: { open: false, title: '', message: '', confirmLabel: '', resolve: null },

        async init() {
            try {
                const userResponse = await fetch('/auth/current-user');
                if (!userResponse.ok) {
                    window.location.href = '/login.html';
                    return;
                }
                const user = await userResponse.json();
                this.displayName = user.display_name || user.username;
                this.isAdmin = (user.roles || []).includes('admin');
                this.currentUserId = user.user_id || user.id || '';
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
                return;
            }
            this.gameId = decodeURIComponent(location.pathname.split('/')[2] || '');
            await this.load();
        },

        async load() {
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.gameId)}`);
                if (response.status === 404) {
                    this.notFound = true;
                    this.campaign = null;
                    return;
                }
                if (!response.ok) {
                    throw new Error('Failed to load game');
                }
                this.campaign = await response.json();
                this.notFound = false;
                document.title = 'Waystone - ' + (this.campaign.title || 'Game');
            } catch (error) {
                this.showMessage('Error loading game: ' + error.message, 'error');
            } finally {
                this.loading = false;
            }
        },

        get isDM() {
            return !!this.currentUserId && this.campaign?.dm === this.currentUserId;
        },
        get isPlayer() {
            return (this.campaign?.players || []).includes(this.currentUserId);
        },
        get isMember() {
            return this.isDM || this.isPlayer;
        },
        get statusClass() {
            return campaignStatusClass(this.campaign?.status);
        },
        get typeInfo() {
            return CAMPAIGN_TYPES.find((t) => t.value === this.campaign?.type) || CAMPAIGN_TYPES.find((t) => t.value === 'Campaign');
        },
        get typeIconPaths() {
            return TYPE_ICON_PATHS[this.typeInfo.value] || '';
        },
        get descriptionHtml() {
            return renderMarkdown(this.campaign?.description);
        },
        get visibleSessions() {
            const all = this.campaign?.played_sessions || [];
            return this.showAll ? all : all.slice(0, 5);
        },
        get seatsLabel() {
            const max = parseInt(this.campaign?.desired_player_count, 10) || 0;
            const taken = (this.campaign?.players || []).length;
            return `${taken} / ${max > 0 ? max : '∞'} seats · sign-ups ${this.campaign?.sign_ups_open ? 'open' : 'closed'}`;
        },
        get signupStatus() {
            return this.campaign?.current_user_signup_status || '';
        },
        get pendingCount() {
            return this.isDM ? (parseInt(this.campaign?.pending_signups_count, 10) || 0) : 0;
        },

        formatDate(dateStr) {
            return parseDateLocal(dateStr).toLocaleDateString('default', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
        },
        sessionMeta(s) {
            const end = getSessionEndTime(s);
            let meta = `${s.time} – ${end} · ${getDurationLabel(s)}`;
            if (s.timezone && s.timezone !== getDefaultTimezone()) meta += ` · ${s.timezone}`;
            return meta;
        },
        attendeeNames(s) {
            return (s.attendees || []).map((u) => u.display_name || u.name || 'Unknown').join(', ');
        },
        attendedByMe(s) {
            return (s.attendees || []).some((u) => u.id === this.currentUserId);
        },
        roman(n) {
            return toRoman(n);
        },

        async postAction(url, method, fallback, errorPrefix) {
            try {
                const response = await fetch(url, { method });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || errorPrefix, 'error');
                } else {
                    this.showMessage(data.message || fallback, 'success');
                }
                await this.load();
            } catch (error) {
                this.showMessage(`Error: ${error.message}`, 'error');
            }
        },

        async signUp() {
            await this.postAction(`/api/campaigns/${encodeURIComponent(this.campaign.id)}/signups`, 'POST', 'Signed up successfully', 'Failed to sign up');
        },

        async toggleFollow() {
            const following = !!this.campaign.is_following;
            await this.postAction(`/api/campaigns/${encodeURIComponent(this.campaign.id)}/follow`, following ? 'DELETE' : 'PUT', following ? 'Unfollowed' : 'Following', 'Failed to update follow');
        },

        async withdraw() {
            const leaving = this.isPlayer;
            const title = this.campaign.title;
            const ok = await this.askConfirm(leaving
                ? {
                    title: 'Leave game',
                    message: `Leave "${title}"? You'll stop being a player; you can sign up again if sign-ups are open.`,
                    confirmLabel: 'Leave'
                }
                : {
                    title: 'Withdraw sign-up',
                    message: `Withdraw your sign-up for "${title}"?`,
                    confirmLabel: 'Withdraw'
                });
            if (!ok) return;
            await this.postAction(`/api/campaigns/${encodeURIComponent(this.campaign.id)}/signups/${encodeURIComponent(this.currentUserId)}`, 'DELETE', 'Sign-up withdrawn', 'Failed to withdraw');
        },

        async deleteGame() {
            const ok = await this.askConfirm({
                title: 'Delete game',
                message: `Delete "${this.campaign.title}"? This also deletes its sessions and sign-ups and cannot be undone.`,
                confirmLabel: 'Delete'
            });
            if (!ok) return;
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.campaign.id)}`, { method: 'DELETE' });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to delete game', 'error');
                    return;
                }
                window.location.href = '/campaigns'; // the game page no longer exists
            } catch (error) {
                this.showMessage('Error deleting game: ' + error.message, 'error');
            }
        },

        // askConfirm opens the confirm modal and resolves to true on confirm, false on cancel.
        askConfirm({ title, message, confirmLabel }) {
            if (this.confirmDialog.resolve) this.confirmDialog.resolve(false);
            return new Promise((resolve) => {
                this.confirmDialog = { open: true, title, message, confirmLabel: confirmLabel || 'Confirm', resolve };
                this.$nextTick(() => this.$refs.confirmCancelBtn?.focus());
            });
        },

        closeConfirm(result) {
            const resolve = this.confirmDialog.resolve;
            this.confirmDialog = { open: false, title: '', message: '', confirmLabel: '', resolve: null };
            if (resolve) resolve(result);
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
            setTimeout(() => {
                this.messageVisible = false;
            }, 5000);
        }
    };
};
