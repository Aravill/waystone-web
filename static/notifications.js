// Header inbox: injects a notifications dropdown into .header-right.
(function () {
    const TEMPLATE = `
<div class="notif" x-data="notificationsInbox()" @click.outside="open = false" @keydown.escape.window="open = false">
    <button type="button" class="notif-btn" :class="{ 'has-unread': unread > 0 }" x-ref="btn" @click="toggle()" :aria-expanded="open" aria-label="Inbox" title="Inbox">
        <svg viewBox="0 0 24 24" width="1.125rem" height="1.125rem" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="square" aria-hidden="true"><rect x="3" y="5" width="18" height="14"/><path d="M3 6l9 7 9-7"/></svg><span class="notif-badge" x-show="unread > 0" x-cloak x-text="unread > 99 ? '99+' : unread"></span>
    </button>
    <div class="notif-panel" x-show="open" x-cloak>
        <div class="notif-panel-header">
            <span class="notif-panel-title">Notifications</span>
            <label class="notif-filter"><input type="checkbox" :checked="unreadOnly" @change="setUnreadOnly($event.target.checked)"> Only unread</label>
            <button type="button" class="notif-markall" :disabled="unread === 0" @click="markAllRead()">Mark all read</button>
        </div>
        <div class="notif-empty" x-show="!loading && items.length === 0">
            <span x-text="unreadOnly ? 'No unread notifications' : 'No notifications'"></span>
            <button type="button" class="notif-markall" x-show="unreadOnly" @click="setUnreadOnly(false)">Show all</button>
        </div>
        <ul class="notif-list">
            <template x-for="n in items" :key="n.id">
                <li class="notif-item" :class="{ 'is-unread': !n.read_at }" @click="openItem(n)">
                    <div class="notif-msg" x-text="message(n)"></div>
                    <div class="notif-time" x-text="timeAgo(n.created_at)"></div>
                </li>
            </template>
        </ul>
        <div class="notif-pager" x-show="page > 0 || hasMore">
            <button type="button" class="notif-markall" :disabled="page === 0" @click="turn(-1)" aria-label="Newer notifications">‹ Newer</button>
            <span class="notif-range" x-text="(page * PAGE + 1) + '\u2013' + (page * PAGE + items.length)"></span>
            <button type="button" class="notif-markall" :disabled="!hasMore" @click="turn(1)" aria-label="Older notifications">Older ›</button>
        </div>
    </div>
</div>`;

    window.notificationsInbox = function () {
        return {
            open: false,
            items: [],
            unread: 0,
            unreadOnly: true, // "Only unread" is on until the user turns it off (kept in localStorage)
            page: 0,
            hasMore: false,
            PAGE: 10,
            loading: false,
            _timer: null,
            _counted: false,

            init() {
                try {
                    this.unreadOnly = localStorage.getItem('waystone-notif-unread') !== '0';
                } catch (error) { /* storage blocked: keep the default */ }
                if (this._timer) return;
                this.refreshCount();
                this._timer = setInterval(() => {
                    if (!document.hidden) this.refreshCount();
                }, 60000);
                document.addEventListener('visibilitychange', () => {
                    if (!document.hidden) this.refreshCount();
                });
            },

            async refreshCount() {
                try {
                    const response = await fetch('/api/notifications/unread-count');
                    if (response.status === 401) {
                        // Session revoked (e.g. password changed elsewhere).
                        window.location.href = '/login.html';
                        return;
                    }
                    if (!response.ok) return;
                    const data = await response.json();
                    // Let the page refresh its own data when new notifications arrive.
                    if (this._counted && data.unread_count > this.unread) {
                        window.dispatchEvent(new CustomEvent('notifications:new'));
                    }
                    this._counted = true;
                    this.unread = data.unread_count;
                } catch (error) {
                    // Polling must stay quiet (e.g. expired session redirects to HTML).
                    console.error('Failed to refresh notifications:', error);
                }
            },

            toggle() {
                this.open = !this.open;
                if (!this.open) return;
                this.page = 0;
                // Mobile panel is position:fixed; anchor it just below the button.
                this.$root.style.setProperty('--notif-top', this.$refs.btn.getBoundingClientRect().bottom + 8 + 'px');
                this.load();
            },

            setUnreadOnly(v) {
                this.unreadOnly = v;
                try { localStorage.setItem('waystone-notif-unread', v ? '1' : '0'); } catch (error) { /* not saved */ }
                this.page = 0;
                this.load();
            },

            turn(d) {
                this.page = Math.max(0, this.page + d);
                this.load();
            },

            async load() {
                this.loading = true;
                try {
                    const response = await fetch(`/api/notifications?limit=${this.PAGE}&offset=${this.page * this.PAGE}` + (this.unreadOnly ? '&unread=1' : ''));
                    if (!response.ok) return;
                    const data = await response.json();
                    if (this.page > 0 && data.notifications.length === 0) {
                        // The page emptied (items were read): step back
                        this.page -= 1;
                        this.loading = false;
                        return this.load();
                    }
                    this.items = data.notifications;
                    this.hasMore = !!data.has_more;
                    this.unread = data.unread_count;
                } catch (error) {
                    console.error('Failed to load notifications:', error);
                } finally {
                    this.loading = false;
                }
            },

            async openItem(n) {
                if (!n.read_at) {
                    try {
                        const response = await fetch('/api/notifications/' + encodeURIComponent(n.id) + '/read', { method: 'POST' });
                        if (response.ok) {
                            n.read_at = new Date().toISOString();
                            this.unread = Math.max(0, this.unread - 1);
                        }
                    } catch (error) {
                        console.error('Failed to mark notification read:', error);
                    }
                }
                if (n.type === 'character_status' && (n.data || {}).character_id) {
                    location.href = '/characters/' + encodeURIComponent(n.data.character_id);
                } else if (n.type === 'discord_channel_lost') {
                    location.href = '/admin/discord';
                } else if (n.campaign_id && (n.type === 'session_started' || n.type === 'session_reminder' || n.type === 'character_assigned' || n.type === 'character_cleared')) {
                    location.href = '/games/' + encodeURIComponent(n.campaign_id);
                } else if (n.campaign_id && n.type === 'signup_accepted' && (n.data || {}).characters) {
                    location.href = '/games/' + encodeURIComponent(n.campaign_id) + '?character=1';
                } else if (n.campaign_id && (n.data || {}).date && ['session_suggested', 'poll_reminder', 'poll_complete', 'poll_posted', 'poll_skipped'].includes(n.type)) {
                    // A poll or suggestion: the calendar, on the month of its first date
                    location.href = '/games/' + encodeURIComponent(n.campaign_id) + '/calendar?date=' + encodeURIComponent(n.data.date);
                } else if (n.campaign_id) {
                    const anchor = 'campaign-' + n.campaign_id;
                    const target = location.pathname === '/campaigns' ? document.getElementById(anchor) : null;
                    if (target) {
                        target.scrollIntoView({ block: 'start' });
                        this.open = false;
                    } else {
                        location.href = '/campaigns#' + encodeURIComponent(anchor);
                    }
                }
            },

            async markAllRead() {
                try {
                    const response = await fetch('/api/notifications/read-all', { method: 'POST' });
                    if (!response.ok) return;
                    this.unread = 0;
                    if (this.unreadOnly) {
                        this.page = 0;
                        await this.load();
                        return;
                    }
                    const now = new Date().toISOString();
                    this.items.forEach((n) => { if (!n.read_at) n.read_at = now; });
                } catch (error) {
                    console.error('Failed to mark notifications read:', error);
                }
            },

            timeAgo(ts) {
                const seconds = Math.floor((Date.now() - new Date(ts).getTime()) / 1000);
                if (seconds < 60) return 'just now';
                if (seconds < 3600) return Math.floor(seconds / 60) + 'm ago';
                if (seconds < 86400) return Math.floor(seconds / 3600) + 'h ago';
                if (seconds < 7 * 86400) return Math.floor(seconds / 86400) + 'd ago';
                return new Date(ts).toLocaleDateString();
            },

            // "Fri, Dec 11" from YYYY-MM-DD (campaigns.js is not loaded on every page)
            fmtDay(ymd) {
                const [y, m, day] = String(ymd).split('-').map(Number);
                return new Date(y, m - 1, day).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
            },

            // Keep in step with discordText in api/discord_text.go (and openItem with discordLink).
            message(n) {
                const d = n.data || {};
                const count = parseInt(d.count, 10) || 1;
                const dayList = d.dates ? d.dates.split(',').map((x) => this.fmtDay(x)).join(' · ') : '';
                const actor = (n.actor_user && n.actor_user.display_name) || 'Someone';
                const title = n.campaign_title || d.campaign_title || '';
                const when = `${d.date || ''} ${d.time || ''}`.trim() + (d.timezone ? ` (${d.timezone})` : '');
                switch (n.type) {
                    case 'campaign_status':
                        return `"${title}" moved from ${d.from_status} to ${d.to_status}`;
                    case 'signup_accepted':
                        return `Your sign-up for "${title}" was accepted` + (d.characters ? '. Choose your character' : '');
                    case 'signup_declined':
                        return `Your sign-up for "${title}" was declined`;
                    case 'player_kicked':
                        return `You were removed from "${title}"`;
                    case 'session_suggested':
                        if (d.added === '1') return `${actor} added ${count === 1 ? 'a date' : count + ' dates'} to the poll for "${title}": ${dayList}`;
                        if (count > 1) return `${actor} suggested ${count} dates for "${title}": ${dayList}`;
                        return `${actor} suggested a session for "${title}" on ${when}`;
                    case 'session_cancelled':
                        if (d.was === 'Suggested' && count > 1) return `${actor} withdrew the poll for "${title}"`;
                        return d.was === 'Suggested'
                            ? `${actor} withdrew the suggested session for "${title}" on ${when}`
                            : `${actor} cancelled the session for "${title}" on ${when}`;
                    case 'poll_reminder':
                        return count > 1
                            ? `The poll for "${title}" still needs your reply. First date: ${this.fmtDay(d.date)}`
                            : `The session suggested for "${title}" on ${when} still needs your reply`;
                    case 'poll_complete':
                        return count > 1
                            ? `Everyone has replied to the poll for "${title}"`
                            : `Everyone has replied to the session suggested for "${title}" on ${when}`;
                    case 'poll_posted':
                        return `Your repeating poll for "${title}" was posted: ${dayList}`;
                    case 'poll_skipped':
                        return d.reason === 'cap'
                            ? `No poll was posted for "${title}": the game has reached its limit of 999 sessions`
                            : `No poll was posted for "${title}": ${dayList} ${count === 1 ? 'already has a session' : 'already have sessions'}`;
                    case 'signup_created':
                        return `${actor} signed up for "${title}"`;
                    case 'player_left':
                        return `${actor} left "${title}"`;
                    case 'signup_withdrawn':
                        return `${actor} withdrew their sign-up for "${title}"`;
                    case 'campaign_full':
                        return `"${title}" reached ${d.player_count} ${d.player_count === '1' ? 'player' : 'players'} - sign-ups closed automatically`;
                    case 'signups_opened':
                        return `Sign-ups opened for "${title}"`;
                    case 'session_started':
                        return `A session of "${title}" has begun`;
                    case 'character_assigned':
                        return `${actor} now plays ${d.character_name} in "${title}"`;
                    case 'character_cleared':
                        return `${actor} removed their character ${d.character_name} from "${title}"`;
                    case 'character_status':
                        return `${actor} marked ${d.character_name} as ${d.to_status}`;
                    case 'session_confirmed':
                        return `${actor} confirmed the session for "${title}" on ${when}`;
                    case 'session_reminder':
                        return `Reminder: the session of "${title}" is on ${when}`;
                    case 'discord_channel_lost':
                        return `The Discord channel of "${title}" is gone. Pick another in Admin › Discord`;
                    default:
                        return `Update for "${title}"`;
                }
            },
        };
    };

    const header = document.querySelector('.header-right');
    if (header) header.insertAdjacentHTML('afterbegin', TEMPLATE);
})();
