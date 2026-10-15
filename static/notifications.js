// Header inbox: injects a notifications dropdown into .header-right.
(function () {
    const TEMPLATE = `
<div class="notif" x-data="notificationsInbox()" @click.outside="open = false" @keydown.escape.window="open = false">
    <button type="button" class="notif-btn" x-ref="btn" @click="toggle()" :aria-expanded="open">
        Inbox<span class="notif-badge" x-show="unread > 0" x-cloak x-text="unread > 99 ? '99+' : unread"></span>
    </button>
    <div class="notif-panel" x-show="open" x-cloak>
        <div class="notif-panel-header">
            <span class="notif-panel-title">Notifications</span>
            <button type="button" class="notif-markall" :disabled="unread === 0" @click="markAllRead()">Mark all read</button>
        </div>
        <div class="notif-empty" x-show="!loading && items.length === 0">No notifications</div>
        <ul class="notif-list">
            <template x-for="n in items" :key="n.id">
                <li class="notif-item" :class="{ 'is-unread': !n.read_at }" @click="openItem(n)">
                    <div class="notif-msg" x-text="message(n)"></div>
                    <div class="notif-time" x-text="timeAgo(n.created_at)"></div>
                </li>
            </template>
        </ul>
    </div>
</div>`;

    window.notificationsInbox = function () {
        return {
            open: false,
            items: [],
            unread: 0,
            loading: false,
            _timer: null,
            _counted: false,

            init() {
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
                // Mobile panel is position:fixed; anchor it just below the button.
                this.$root.style.setProperty('--notif-top', this.$refs.btn.getBoundingClientRect().bottom + 8 + 'px');
                this.load();
            },

            async load() {
                this.loading = true;
                try {
                    const response = await fetch('/api/notifications?limit=30');
                    if (!response.ok) return;
                    const data = await response.json();
                    this.items = data.notifications;
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
                if (n.campaign_id) {
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
                    const now = new Date().toISOString();
                    this.items.forEach((n) => { if (!n.read_at) n.read_at = now; });
                    this.unread = 0;
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

            message(n) {
                const d = n.data || {};
                const actor = (n.actor_user && n.actor_user.display_name) || 'Someone';
                const title = n.campaign_title || d.campaign_title || '';
                const when = `${d.date || ''} ${d.time || ''}`.trim() + (d.timezone ? ` (${d.timezone})` : '');
                switch (n.type) {
                    case 'campaign_status':
                        return `"${title}" moved from ${d.from_status} to ${d.to_status}`;
                    case 'signup_accepted':
                        return `Your sign-up for "${title}" was accepted`;
                    case 'signup_declined':
                        return `Your sign-up for "${title}" was declined`;
                    case 'player_kicked':
                        return `You were removed from "${title}"`;
                    case 'session_suggested':
                        return `${actor} suggested a session for "${title}" on ${when}`;
                    case 'session_cancelled':
                        return d.was === 'Suggested'
                            ? `${actor} withdrew the suggested session for "${title}" on ${when}`
                            : `${actor} cancelled the session for "${title}" on ${when}`;
                    case 'signup_created':
                        return `${actor} signed up for "${title}"`;
                    case 'player_left':
                        return `${actor} left "${title}"`;
                    case 'signup_withdrawn':
                        return `${actor} withdrew their sign-up for "${title}"`;
                    case 'campaign_full':
                        return `"${title}" reached ${d.player_count} ${d.player_count === '1' ? 'player' : 'players'} - sign-ups closed automatically`;
                    case 'session_confirmed':
                        return `${actor} confirmed the session for "${title}" on ${when}`;
                    default:
                        return `Update for "${title}"`;
                }
            },
        };
    };

    const header = document.querySelector('.header-right');
    if (header) header.insertAdjacentHTML('afterbegin', TEMPLATE);
})();
