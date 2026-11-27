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
                <li>
                    <a class="notif-item" :class="{ 'is-unread': !n.read_at }" :href="n.url" @click="seen(n)" @auxclick="$event.button === 1 && seen(n)">
                        <span class="notif-msg" x-text="message(n)"></span>
                        <span class="notif-time" x-text="timeAgo(n.created_at)"></span>
                    </a>
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

    // A page opened through a notification's link (?notif={id}, from the inbox or a Discord DM) marks it
    // read, however it was opened, and drops the id from the address bar.
    function readFromUrl() {
        const url = new URL(location.href);
        const id = url.searchParams.get('notif');
        if (!id) return null;
        url.searchParams.delete('notif');
        history.replaceState(history.state, '', url.pathname + url.search + url.hash);
        return fetch('/api/notifications/' + encodeURIComponent(id) + '/read', { method: 'POST' })
            .catch((error) => console.error('Failed to mark notification read:', error));
    }
    const reading = readFromUrl();

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

            async init() {
                try {
                    this.unreadOnly = localStorage.getItem('waystone-notif-unread') !== '0';
                } catch (error) { /* storage blocked: keep the default */ }
                if (this._timer) return;
                // The header jukebox piggybacks on the same timer through 'waystone:poll'.
                this._timer = setInterval(() => {
                    if (document.hidden) return;
                    this.refreshCount();
                    window.dispatchEvent(new Event('waystone:poll'));
                }, 60000);
                document.addEventListener('visibilitychange', () => {
                    if (document.hidden) return;
                    this.refreshCount();
                    window.dispatchEvent(new Event('waystone:poll'));
                });
                await reading; // so the first count leaves out the notification that was just opened
                this.refreshCount();
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

            // The page the link opens marks the notification read (see readFromUrl); this only keeps
            // the list and the badge in step when it opens in another tab.
            seen(n) {
                if (n.read_at) return;
                n.read_at = new Date().toISOString();
                this.unread = Math.max(0, this.unread - 1);
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

            // Keep in step with discordText in api/discord_text.go. Links come from the server (notificationTarget).
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
                        if (d.channel === 'voice') return `The Discord voice channel of "${title}" is gone, so its events are off. Pick another in Admin › Discord`;
                        return `The Discord channel of "${title}" is gone. Pick another in Admin › Discord`;
                    default:
                        return `Update for "${title}"`;
                }
            },
        };
    };

    // Jukebox icon: next to the inbox, hidden unless a game of the user's is live or their own pop-up is open.
    const JUKEBOX_TEMPLATE = `
<div class="jukebox" x-data="jukeboxHeader()" x-show="visible" x-cloak @click.outside="open = false" @keydown.escape.window="open = false">
    <button type="button" class="notif-btn" x-ref="btn" @click="toggle()" :aria-expanded="open" aria-label="Jukebox" title="Jukebox">
        <svg viewBox="0 0 24 24" width="1.125rem" height="1.125rem" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="square" aria-hidden="true"><path d="M9 18V5l11-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="17" cy="16" r="3"/></svg><span class="jukebox-dot" x-show="anyLive" x-cloak></span>
    </button>
    <div class="notif-panel jukebox-panel" x-show="open" x-cloak>
        <div class="notif-panel-header">
            <span class="notif-panel-title">Jukebox</span>
        </div>
        <ul class="notif-list">
            <template x-for="g in rows" :key="g.campaign_id">
                <li class="jukebox-row">
                    <div class="jukebox-row-head">
                        <span class="jukebox-name" x-text="g.title"></span>
                        <span class="jukebox-state" :class="'is-' + g.state" x-text="stateLabel(g)"></span>
                    </div>
                    <div class="jukebox-meta" x-show="g.listeners != null" x-text="g.listeners + ' listening'"></div>
                    <div class="jukebox-actions">
                        <button type="button" class="campaign-action-btn" @click="openPopup(g)" x-text="g.role === 'dm' ? (g.live ? 'Broadcasting' : 'Open') : 'Listen'"></button>
                    </div>
                    <div class="jukebox-remote" x-show="isCurrent(g)">
                        <label class="jukebox-vol"><span x-text="g.role === 'dm' ? 'Send volume' : 'Volume'"></span>
                            <input type="range" min="0" max="1" step="0.01" :value="popup ? popup.volume : 0" @input="command('volume', Number($event.target.value))"></label>
                        <div class="jukebox-remote-buttons">
                            <button type="button" class="campaign-action-btn" @click="command('mute', !(popup && popup.muted))" x-text="popup && popup.muted ? 'Unmute' : 'Mute'"></button>
                            <button type="button" class="btn-danger" @click="command('stop')" x-text="g.role === 'dm' ? 'Stop' : 'Close'"></button>
                        </div>
                    </div>
                </li>
            </template>
        </ul>
    </div>
</div>`;

    window.jukeboxHeader = function () {
        return {
            open: false,
            live: [], // from GET /api/jukebox/live
            pops: {}, // pop-up instances heard on the BroadcastChannel
            _bc: null,

            init() {
                if (window.BroadcastChannel) {
                    this._bc = new BroadcastChannel('waystone-jukebox');
                    this._bc.onmessage = (event) => {
                        const m = event.data || {};
                        if (m.type === 'hello' && m.instance) {
                            this.pops = { ...this.pops, [m.instance]: { ...m, seen: Date.now() } };
                        } else if (m.type === 'bye' && m.instance) {
                            const next = { ...this.pops };
                            delete next[m.instance];
                            this.pops = next;
                        }
                    };
                    this.ping();
                }
                window.addEventListener('waystone:poll', () => { this.refresh(); this.ping(); });
                // A pop-up that stopped answering (crashed) must not keep the icon alive.
                setInterval(() => {
                    const fresh = {};
                    for (const [k, v] of Object.entries(this.pops)) if (Date.now() - v.seen < 90000) fresh[k] = v;
                    if (Object.keys(fresh).length !== Object.keys(this.pops).length) this.pops = fresh;
                }, 15000);
                this.refresh();
            },

            ping() {
                if (this._bc) this._bc.postMessage({ type: 'ping' });
            },

            async refresh() {
                try {
                    const response = await fetch('/api/jukebox/live');
                    if (!response.ok) return;
                    const data = await response.json();
                    this.live = data.live || [];
                    window.dispatchEvent(new CustomEvent('jukebox:live', { detail: this.live }));
                } catch (error) {
                    console.error('Failed to refresh the jukebox:', error);
                }
            },

            // The newest pop-up that has been heard from recently.
            get popup() {
                let best = null;
                for (const p of Object.values(this.pops)) {
                    if (Date.now() - p.seen < 90000 && (!best || p.opened_at > best.opened_at)) best = p;
                }
                return best;
            },
            get visible() {
                return this.live.length > 0 || !!this.popup;
            },
            get anyLive() {
                return this.live.some((g) => !g.paused);
            },
            get rows() {
                const rows = this.live.map((g) => ({
                    campaign_id: g.campaign_id, title: g.title, role: g.role, listeners: g.listeners,
                    live: true, state: g.paused ? 'paused' : 'live',
                }));
                const p = this.popup;
                if (p && !rows.some((r) => r.campaign_id === p.campaign_id)) {
                    rows.push({ campaign_id: p.campaign_id, title: p.title, role: p.role, listeners: null, live: false, state: 'off' });
                }
                return rows;
            },
            isCurrent(g) {
                return !!this.popup && this.popup.campaign_id === g.campaign_id;
            },
            stateLabel(g) {
                if (g.role === 'dm' && !g.live) return '○ Not broadcasting';
                return { live: '● Live', paused: '◐ Paused', off: '○ Off air' }[g.state];
            },

            toggle() {
                this.open = !this.open;
                if (!this.open) return;
                this.$root.style.setProperty('--notif-top', this.$refs.btn.getBoundingClientRect().bottom + 8 + 'px');
                this.refresh();
                this.ping();
            },

            command(type, value) {
                const p = this.popup;
                if (this._bc && p) this._bc.postMessage({ type, value, campaign_id: p.campaign_id });
            },

            openPopup(g) {
                const p = this.popup;
                // Opening another game in the same window would end a broadcast that is running there.
                if (p && p.broadcasting && p.campaign_id !== g.campaign_id
                    && !window.confirm('This will stop your broadcast of "' + p.title + '". Continue?')) return;
                window.open('/games/' + encodeURIComponent(g.campaign_id) + '/jukebox', 'waystone-jukebox', 'popup,width=440,height=640');
                this.open = false;
            },
        };
    };

    const header = document.querySelector('.header-right');
    if (header) header.insertAdjacentHTML('afterbegin', JUKEBOX_TEMPLATE + TEMPLATE);
})();
