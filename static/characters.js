// Characters: the list page (/characters), the detail page (/characters/{id}) and the helpers the
// game page's picker shares. Relies on campaigns.js (loaded first) for escapeHtml, renderMarkdown
// and userAvatarHtml.

const CHARACTER_STATUSES = {
    alive: { glyph: '●', label: 'Alive' },
    retired: { glyph: '◐', label: 'Retired' },
    dead: { glyph: '✗', label: 'Dead' }
};

const GAME_STATUS_GLYPHS = { Pitch: '✦', Ongoing: '●', 'On Hiatus': '◐', Finished: '✓', Cancelled: '✗' };

// Status is a label with a glyph, never colour alone.
function characterStatusHtml(status) {
    const s = CHARACTER_STATUSES[status] || CHARACTER_STATUSES.alive;
    return `<span class="char-status char-status--${escapeHtml(status)}"><span aria-hidden="true">${s.glyph}</span> ${s.label}</span>`;
}

// One badge per game: active normal, ended dimmed with a dashed border and the word "ended".
function characterGameBadgesHtml(games) {
    if (!games || games.length === 0) return '<span class="char-no-games">Not in any game yet</span>';
    return games.map((g) => {
        const glyph = GAME_STATUS_GLYPHS[g.campaign_status] || '●';
        const state = g.active ? g.campaign_status : `ended · ${g.campaign_status}`;
        return `<a class="char-game${g.active ? '' : ' is-ended'}" href="/games/${encodeURIComponent(g.campaign_id)}" title="${escapeHtml(state)}">`
            + `<span aria-hidden="true">${glyph}</span> ${escapeHtml(g.title)}`
            + `<span class="char-game-state">${escapeHtml(g.active ? g.campaign_status : 'ended')}</span></a>`;
    }).join('');
}

function characterSystemHtml(system) {
    if (!system) return '<span class="char-system">Unknown system</span>';
    const logo = system.logo_url ? `<img class="char-system-logo" src="${escapeHtml(system.logo_url)}" alt="">` : '';
    return `<span class="char-system">${logo}<span>${escapeHtml(system.name)}</span></span>`;
}

// Shared by both pages: the signed-in user, the toast and logout.
function characterPageBase() {
    return {
        displayName: 'User',
        isAdmin: false,
        currentUserId: '',
        messageTimer: null,
        messageVisible: false,
        messageText: '',
        messageType: '',
        statuses: Object.entries(CHARACTER_STATUSES).map(([value, s]) => ({ value, label: s.label })),
        systems: [],

        async loadUser() {
            try {
                const response = await fetch('/auth/current-user');
                if (!response.ok) {
                    window.location.href = '/login.html';
                    return false;
                }
                const user = await response.json();
                this.displayName = user.display_name || user.username;
                this.isAdmin = (user.roles || []).includes('admin');
                this.currentUserId = user.user_id || user.id || '';
                return true;
            } catch (error) {
                console.error('Auth check failed:', error);
                window.location.href = '/login.html';
                return false;
            }
        },

        async loadSystems() {
            try {
                const response = await fetch('/api/systems');
                if (!response.ok) throw new Error('bad response');
                this.systems = await response.json();
            } catch (error) {
                this.showMessage('Could not load systems', 'error');
            }
        },

        statusHtml: characterStatusHtml,
        gameBadgesHtml: characterGameBadgesHtml,
        systemHtml: characterSystemHtml,
        markdown: renderMarkdown,

        async handleLogout() {
            try {
                const response = await fetch('/auth/logout', { method: 'POST' });
                if (response.ok) window.location.href = '/login.html';
            } catch (error) {
                this.showMessage('Logout failed: ' + error.message, 'error');
            }
        },

        showMessage(text, type) {
            this.messageText = text;
            this.messageType = type;
            this.messageVisible = true;
            clearTimeout(this.messageTimer);
            this.messageTimer = setTimeout(() => { this.messageVisible = false; }, 5000);
        }
    };
}

function emptyCharacterForm(systemId = '') {
    return { name: '', system_id: systemId, status: 'alive', description: '', background: '' };
}

window.charactersPage = function () {
    return {
        ...characterPageBase(),
        loading: true,
        characters: [],
        creating: false,
        busy: false,
        form: emptyCharacterForm(),
        formError: '',
        deleteDialog: { open: false, character: null, error: '' },

        async init() {
            if (!(await this.loadUser())) return;
            await Promise.all([this.load(), this.loadSystems()]);
        },

        async load() {
            try {
                const response = await fetch('/api/characters');
                const data = await response.json();
                if (!response.ok) throw new Error(data.error || 'Failed to load characters');
                this.characters = data.characters;
            } catch (error) {
                this.showMessage(error.message, 'error');
            } finally {
                this.loading = false;
            }
        },

        startCreate() {
            this.form = emptyCharacterForm('');
            this.formError = '';
            this.creating = true;
            this.$nextTick(() => document.getElementById('newName')?.focus());
        },

        async create() {
            this.formError = '';
            if (!this.form.name.trim()) {
                this.formError = 'Give your character a name.';
                return;
            }
            if (!this.form.system_id) {
                this.formError = 'Choose a system.';
                return;
            }
            this.busy = true;
            try {
                const response = await fetch('/api/characters', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(this.form)
                });
                const data = await response.json();
                if (!response.ok) {
                    this.formError = data.error || 'Failed to create character';
                    return;
                }
                this.creating = false;
                this.showMessage('Character created', 'success');
                await this.load();
            } catch (error) {
                this.formError = error.message;
            } finally {
                this.busy = false;
            }
        },

        askDelete(c) {
            this.deleteDialog = { open: true, character: c, error: '' };
        },

        async confirmDelete() {
            const c = this.deleteDialog.character;
            this.busy = true;
            try {
                const response = await fetch(`/api/characters/${encodeURIComponent(c.id)}`, { method: 'DELETE' });
                const data = await response.json();
                if (!response.ok) {
                    this.deleteDialog.error = data.error || 'Failed to delete character';
                    return;
                }
                this.deleteDialog = { open: false, character: null, error: '' };
                this.showMessage('Character deleted', 'success');
                await this.load();
            } catch (error) {
                this.deleteDialog.error = error.message;
            } finally {
                this.busy = false;
            }
        }
    };
};

window.characterPage = function () {
    return {
        ...characterPageBase(),
        characterId: '',
        character: null,
        loading: true,
        notFound: false,
        editing: false,
        busy: false,
        form: emptyCharacterForm(),
        formError: '',
        statusChoice: '',

        async init() {
            if (!(await this.loadUser())) return;
            this.characterId = decodeURIComponent(location.pathname.split('/')[2] || '');
            if (!this.characterId) {
                location.replace('/characters');
                return;
            }
            await Promise.all([this.load(), this.loadSystems()]);
            if (this.character?.can_edit && new URLSearchParams(location.search).get('edit') === '1') this.startEdit();
        },

        async load() {
            try {
                const response = await fetch(`/api/characters/${encodeURIComponent(this.characterId)}`);
                if (response.status === 404) {
                    this.notFound = true;
                    return;
                }
                const data = await response.json();
                if (!response.ok) throw new Error(data.error || 'Failed to load character');
                this.character = data;
                this.statusChoice = data.status;
                document.title = 'Waystone - ' + data.name;
            } catch (error) {
                this.showMessage(error.message, 'error');
            } finally {
                this.loading = false;
            }
        },

        get ownerName() {
            return this.character?.owner_user?.display_name || 'Unknown';
        },

        startEdit() {
            const c = this.character;
            this.form = { name: c.name, system_id: c.system_id, status: c.status, description: c.description, background: c.background };
            this.formError = '';
            this.editing = true;
        },

        cancelEdit() {
            this.editing = false;
            history.replaceState(null, '', location.pathname);
        },

        async save() {
            this.formError = '';
            if (!this.form.name.trim()) {
                this.formError = 'Give your character a name.';
                return;
            }
            this.busy = true;
            try {
                const response = await fetch(`/api/characters/${encodeURIComponent(this.characterId)}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(this.form)
                });
                const data = await response.json();
                if (!response.ok) {
                    this.formError = data.error || 'Failed to save character';
                    return;
                }
                // Status goes through its own endpoint so an edit can't overwrite a concurrent change.
                if (this.form.status !== this.character.status) {
                    const sr = await fetch(`/api/characters/${encodeURIComponent(this.characterId)}/status`, {
                        method: 'PUT',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ status: this.form.status })
                    });
                    if (!sr.ok) {
                        this.showMessage((await sr.json()).error || 'Failed to change status', 'error');
                    }
                }
                this.editing = false;
                this.showMessage('Character saved', 'success');
                history.replaceState(null, '', location.pathname);
                await this.load();
            } catch (error) {
                this.formError = error.message;
            } finally {
                this.busy = false;
            }
        },

        async saveStatus() {
            if (this.statusChoice === this.character.status) return;
            this.busy = true;
            try {
                const response = await fetch(`/api/characters/${encodeURIComponent(this.characterId)}/status`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ status: this.statusChoice })
                });
                const data = await response.json();
                if (!response.ok) {
                    this.showMessage(data.error || 'Failed to change status', 'error');
                    this.statusChoice = this.character.status;
                    return;
                }
                this.showMessage('Status changed', 'success');
                await this.load();
            } catch (error) {
                this.showMessage(error.message, 'error');
            } finally {
                this.busy = false;
            }
        }
    };
};
