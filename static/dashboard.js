// Home page Alpine component. Relies on campaigns.js (loaded first) for escapeHtml, userAvatarHtml,
// REPLY_GLYPHS, replyWord, formatShortDate, sessionRangeWithZone, timeWithZone, zoneAbbr and getSessionCounts.
window.dashboardPage = function () {
    return {
        displayName: '',
        isAdmin: false,
        loading: true,
        loaded: false,
        loadError: '',
        busy: false,
        agenda: { in_progress: [], awaiting: [], ahead: [], games: [], open_to_join_count: 0 },
        now: Date.now(),
        loadSeq: 0,
        messageVisible: false,
        messageText: '',
        messageType: '',
        _messageTimer: null,

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
            } catch (error) {
                console.error('Failed to fetch user info:', error);
                window.location.href = '/login.html';
                return;
            }
            await this.loadAgenda();
            // A session can start or end while the page is open: refresh every minute while visible,
            // when a notification arrives and when the tab comes back.
            setInterval(() => { if (!document.hidden) this.loadAgenda(true); }, 60000);
            window.addEventListener('notifications:new', () => this.loadAgenda(true));
            document.addEventListener('visibilitychange', () => { if (!document.hidden) this.loadAgenda(true); });
            window.addEventListener('pageshow', (event) => { if (event.persisted) this.loadAgenda(true); });
        },

        // quiet: a background refresh that keeps what is shown if it fails.
        async loadAgenda(quiet = false) {
            const seq = ++this.loadSeq; // an older response must not overwrite a newer one
            if (!quiet) this.loading = true;
            try {
                const response = await fetch('/api/agenda');
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (seq !== this.loadSeq) return;
                if (!response.ok) throw new Error(data.error || 'request failed');
                this.agenda = {
                    in_progress: data.in_progress || [],
                    awaiting: data.awaiting || [],
                    ahead: data.ahead || [],
                    games: data.games || [],
                    open_to_join_count: data.open_to_join_count || 0
                };
                this.now = Date.now();
                this.loaded = true;
                this.loadError = '';
            } catch (error) {
                console.error('Failed to load the agenda:', error);
                if (!quiet) this.loadError = 'Could not load thy agenda: ' + error.message;
            } finally {
                if (seq === this.loadSeq) this.loading = false;
            }
        },

        // Nothing to show but the introduction: no games and nothing pending.
        get isNewcomer() {
            const a = this.agenda;
            return a.games.length === 0 && a.in_progress.length === 0 && a.awaiting.length === 0 && a.ahead.length === 0;
        },

        // Games where the viewer is an accepted player with no character yet (optional, so a nudge only).
        get characterNudges() {
            return this.agenda.games.filter((g) => g.character_nudge);
        },

        // One line: what is on now, what awaits a reply, and the next game.
        get subtitle() {
            const a = this.agenda;
            const parts = [];
            if (a.in_progress.length > 0) {
                const names = a.in_progress.map((s) => s.campaign.title);
                const list = names.length > 1 ? names.slice(0, -1).join(', ') + ' and ' + names[names.length - 1] : names[0];
                parts.push(`${list} ${names.length > 1 ? 'are' : 'is'} in session now.`);
            }
            if (a.awaiting.length > 0) {
                parts.push(a.awaiting.length === 1 ? 'One session awaits thy reply.' : `${a.awaiting.length} sessions await thy reply.`);
            }
            const nudges = this.characterNudges.length;
            if (nudges > 0) {
                parts.push(nudges === 1 ? 'One game awaits thy character.' : `${nudges} games await thy character.`);
            }
            const next = a.ahead[0];
            if (next) {
                parts.push(`Thy next game is ${parseDateLocal(next.date).toLocaleDateString('en-US', { weekday: 'long', month: 'short', day: 'numeric' })}.`);
            }
            if (parts.length === 0) {
                parts.push(this.isNewcomer ? 'Thy road begins here.' : 'Nothing awaits thy reply.');
            }
            return parts.join(' ');
        },

        get zoneNote() {
            const zones = [...new Set(this.agenda.ahead.map((s) => zoneAbbr(s.timezone, s.date)).filter(Boolean))];
            if (zones.length === 1) return `Times are shown in ${zones[0]}.`;
            if (zones.length > 1) return "Times are shown in each game's own zone.";
            return '';
        },

        gameUrl(s) {
            return '/games/' + encodeURIComponent(s.campaign.id);
        },
        shortDate(s) {
            return formatShortDate(s.date);
        },
        timeRange(s) {
            return s.time ? sessionRangeWithZone(s) : '';
        },
        statusClass(status) {
            return campaignStatusClass(status);
        },
        dmName(s) {
            return s.campaign.dm_user?.display_name || 'the DM';
        },
        userAvatarHtml(u, link) {
            return userAvatarHtml(u, link);
        },
        // The red-letter date: month, day number and weekday.
        dayParts(s) {
            const d = parseDateLocal(s.date);
            return {
                month: d.toLocaleDateString('en-US', { month: 'short' }),
                day: d.getDate(),
                weekday: d.toLocaleDateString('en-US', { weekday: 'long' })
            };
        },
        awaitMeta(s) {
            const c = getSessionCounts(s);
            const replied = c.accepted + c.tentative + c.declined;
            return `${this.timeRange(s)}, proposed by ♛ ${this.dmName(s)}. ${replied} of ${c.total} have replied.`;
        },

        // "Began 18:00 CET, ends 22:00 · 1h 20m left"
        nowWhen(s) {
            return `Began ${timeWithZone(s.time, s)}, ends ${s.end_time} · ${this.timeLeft(s)}`;
        },
        timeLeft(s) {
            const minutes = Math.max(0, Math.ceil((new Date(s.end_at).getTime() - this.now) / 60000));
            if (minutes < 1) return 'less than a minute left';
            const h = Math.floor(minutes / 60);
            const m = minutes % 60;
            return (h ? `${h}h ` : '') + (h && !m ? '' : `${m}m `) + 'left';
        },
        acceptedUsers(s) {
            return (s.replies || []).filter((r) => r.participation === 'Accepted').map((r) => r.user);
        },
        mineText(s) {
            return 'Thy reply: ' + (s.current_user_participation ? replyWord(s.current_user_participation) : 'no reply yet');
        },

        // The viewer's own reply on a row, with a glyph so colour never carries it alone.
        mineHtml(s) {
            if (s.role !== 'player') return '';
            const p = s.current_user_participation;
            if (p === 'Accepted') return REPLY_GLYPHS.accepted + ' ' + replyWord(p);
            if (p === 'Tentative') return '? ' + replyWord(p);
            if (p === 'Declined') return REPLY_GLYPHS.declined + ' ' + replyWord(p);
            return s.status === 'Suggested' ? 'Thy reply?' : 'No reply yet';
        },
        mineClass(s) {
            const p = s.current_user_participation;
            return p === 'Accepted' ? 'reply-accepted' : p === 'Tentative' ? 'reply-tentative' : p === 'Declined' ? 'reply-nay' : s.status === 'Suggested' ? 'is-you' : 'reply-none';
        },
        gameMeta(g) {
            const played = parseInt(g.played_sessions_count, 10) || 0;
            const sessions = played > 0 ? `${played} session${played === 1 ? '' : 's'} played` : 'No session yet';
            return g.role === 'dm' ? `♛ You are the DM. ${sessions}.` : `Player, ${sessions.charAt(0).toLowerCase()}${sessions.slice(1)}`;
        },

        async reply(participation, s) {
            if (this.busy) return;
            this.busy = true;
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(s.campaign.id)}/sessions/${encodeURIComponent(s.id)}/responses`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ participation })
                });
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to send thy reply', 'error');
                } else {
                    this.showMessage(`Thy reply for ${s.campaign.title}: ${replyWord(participation)}`, 'success');
                }
                await this.loadAgenda(true);
            } catch (error) {
                this.showMessage('Error sending thy reply: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        },

        async handleLogout() {
            try {
                const response = await fetch('/auth/logout', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' }
                });
                if (response.ok) {
                    window.location.href = '/login.html';
                } else {
                    this.showMessage('Log out failed. Try again.', 'error');
                }
            } catch (error) {
                console.error('Error logging out:', error);
                this.showMessage('Log out failed. Try again.', 'error');
            }
        },

        showMessage(text, type) {
            this.messageText = text;
            this.messageType = type;
            this.messageVisible = true;
            clearTimeout(this._messageTimer);
            this._messageTimer = setTimeout(() => { this.messageVisible = false; }, 5000);
        }
    };
};
