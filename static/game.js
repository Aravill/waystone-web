// Game detail page. Relies on campaigns.js (loaded first) for CAMPAIGN_TYPES, TYPE_ICON_PATHS,
// campaignStatusClass, userAvatarHtml, escapeHtml and getDefaultTimezone.

// Basic Markdown, safe by construction: the whole input is HTML-escaped first, then only
// whitelisted tags are produced. Supports paragraphs, **bold** (may contain *italic*),
// *italic*/_italic_, [text](url), "- " / "1. " lists (also right after a text line), "#" headings
// (rendered as h3/h4) and "> " quotes. Every pattern is bounded, so rendering stays linear.
function renderMarkdown(text) {
    const src = escapeHtml(String(text || '').replace(/\0/g, '').replace(/\r\n?/g, '\n'));
    const inline = (line) => {
        const links = [];
        let out = line.replace(/\[([^\[\]\n]{1,300})\]\(((?:[^()\s]|\([^()\s]*\)){1,2000})\)/g, (m, label, url) => {
            if (!/^(https?:\/\/|mailto:)\S+$/i.test(url)) return m;
            links.push(`<a href="${url}" target="_blank" rel="noopener noreferrer nofollow">${label}</a>`);
            return `\0${links.length - 1}\0`;
        });
        out = out
            .replace(/\*\*(?=[^\s*])([^\n]{1,1000}?)\*\*/g, '<strong>$1</strong>')
            .replace(/(^|[^*\w])\*([^*\s][^*\n]*?)\*(?![*\w])/g, '$1<em>$2</em>')
            .replace(/(^|[^\w])_([^_\s][^_\n]*?)_(?!\w)/g, '$1<em>$2</em>');
        return out.replace(/\0(\d+)\0/g, (m, i) => links[Number(i)]);
    };
    const kindOf = (l) => /^[-*]\s+/.test(l) ? 'ul' : /^\d+\.\s+/.test(l) ? 'ol' : /^&gt;\s?/.test(l) ? 'quote' : 'text';
    const strip = { ul: /^[-*]\s+/, ol: /^\d+\.\s+/, quote: /^&gt;\s?/ };
    // A run of lines of one kind becomes one element, so a list may follow a text line directly.
    const renderRun = (kind, lines) => {
        if (kind === 'text') return `<p>${lines.map(inline).join('<br>')}</p>`;
        const items = lines.map((l) => inline(l.replace(strip[kind], '')));
        if (kind === 'quote') return '<blockquote>' + items.join('<br>') + '</blockquote>';
        return `<${kind}>` + items.map((i) => `<li>${i}</li>`).join('') + `</${kind}>`;
    };
    const blocks = src.split(/\n[ \t]*\n/).map((b) => b.trim()).filter(Boolean);
    return blocks.map((block) => {
        let lines = block.split('\n');
        let out = '';
        const head = lines[0].match(/^(#{1,3})\s+(.*)$/);
        if (head) {
            const tag = head[1].length === 3 ? 'h4' : 'h3';
            out += `<${tag}>${inline(head[2])}</${tag}>`;
            lines = lines.slice(1);
        }
        let run = [], kind = '';
        for (const line of lines) {
            const k = kindOf(line);
            if (run.length && k !== kind) {
                out += renderRun(kind, run);
                run = [];
            }
            kind = k;
            run.push(line);
        }
        return out + (run.length ? renderRun(kind, run) : '');
    }).join('');
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
        busy: false, // an action request is in flight
        loadError: '',
        loadSeq: 0,
        messageTimer: null,
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
            if (new URLSearchParams(location.search).get('updated') === '1') {
                this.showMessage('Game updated', 'success');
                history.replaceState(null, '', location.pathname);
            }
        },

        async load() {
            const seq = ++this.loadSeq; // an older response must not overwrite a newer one
            try {
                const response = await fetch(`/api/campaigns/${encodeURIComponent(this.gameId)}`);
                if (response.redirected || !response.headers.get('Content-Type')?.includes('application/json')) {
                    window.location.href = '/login.html';
                    return;
                }
                const data = await response.json();
                if (seq !== this.loadSeq) return;
                if (response.status === 404) {
                    this.notFound = true;
                    this.loadError = '';
                    this.campaign = null;
                    return;
                }
                if (!response.ok) {
                    this.loadError = data.error || 'Failed to load game';
                    return;
                }
                this.campaign = data;
                this.notFound = false;
                this.loadError = '';
                document.title = 'Waystone - ' + (this.campaign.title || 'Game');
            } catch (error) {
                if (seq === this.loadSeq) this.loadError = 'Could not load the game: ' + error.message;
            } finally {
                if (seq === this.loadSeq) this.loading = false;
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
        get unansweredCount() {
            return this.isPlayer ? (parseInt(this.campaign?.unanswered_sessions_count, 10) || 0) : 0;
        },
        get pendingNotice() {
            if (this.unansweredCount > 0) return this.unansweredCount === 1 ? 'A session awaits thy reply' : `${this.unansweredCount} sessions await thy reply`;
            if (this.pendingCount > 0) return `${this.pendingCount} sign-up${this.pendingCount === 1 ? '' : 's'} to review`;
            return '';
        },
        get needsAction() {
            return this.unansweredCount > 0 || this.pendingCount > 0;
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
        // The DM is marked in the name too, not by colour alone.
        attendeeName(u) {
            return (u.display_name || u.name || 'Unknown') + (u.id === this.campaign?.dm ? ' (DM)' : '');
        },
        attendeeNames(s) {
            return (s.attendees || []).map((u) => this.attendeeName(u)).join(', ');
        },
        attendeeAvatar(u) {
            return userAvatarHtml({ ...u, display_name: this.attendeeName(u) });
        },
        attendedByMe(s) {
            return (s.attendees || []).some((u) => u.id === this.currentUserId);
        },
        async postAction(url, method, fallback, errorPrefix) {
            if (this.busy) return;
            this.busy = true;
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
            } finally {
                this.busy = false;
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
            if (this.busy) return;
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
            if (this.busy) return;
            const ok = await this.askConfirm({
                title: 'Delete game',
                message: `Delete "${this.campaign.title}"? This also deletes its sessions and sign-ups and cannot be undone.`,
                confirmLabel: 'Delete'
            });
            if (!ok || this.busy) return;
            this.busy = true;
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
            } finally {
                this.busy = false;
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
            clearTimeout(this.messageTimer);
            this.messageTimer = setTimeout(() => {
                this.messageVisible = false;
            }, 5000);
        }
    };
};
