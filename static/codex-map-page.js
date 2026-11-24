// Map pages of the Codices: everything codex.js's component does for a page with kind "map". The
// viewer itself is codex-map.js, the icons codex-pins.js. Pins save at once, each change on its own,
// and are logged (History lists them with Undo); they are not part of the page's revisions.
(function () {
    // The viewer stays out of Alpine's reactive state.
    let view = null;
    let mounting = false;

    const iconLabels = Object.fromEntries(window.CODEX_PIN_ICONS.flatMap((g) => g.icons.map(([id, label]) => [id, label])));

    const mapPart = {
        mapMode: '', // what the map waits for: '' | 'place' | 'move' | 'ruler' | 'scale'
        mapOpenId: '', // the pin whose preview is open
        mapCardPos: { x: 8, y: 8 },
        mapFull: false,
        mapHidden: [], // icon ids whose pins are hidden (this viewer only, not kept)
        mapQ: '',
        mapRuler: { points: 0, legs: [], total: 0 },
        mapScalePts: [],
        mapMoveId: '',
        mapUp: { busy: false, pct: 0, over: false },
        mapError: '',
        mapHist: { loading: false, events: [], images: [] },
        CODEX_PIN_ICONS: window.CODEX_PIN_ICONS,
        CODEX_PIN_COLORS: window.CODEX_PIN_COLORS,

        get isMap() {
            return !!(this.page && this.page.kind === 'map');
        },
        get mapData() {
            return (this.pageData && this.pageData.map) || { image: null, pins: [], scale: null };
        },
        get mapPins() {
            return this.mapData.pins;
        },
        get mapOpenPin() {
            return this.mapPins.find((p) => p.id === this.mapOpenId) || null;
        },
        // "13 pins, 5 with a page · scale in miles · image added by Roy Nov 22, 2026"
        get mapSummary() {
            const m = this.mapData;
            if (!m.image) return 'No image yet';
            const linked = m.pins.filter((p) => p.linked).length;
            return [
                m.pins.length ? `${this.countLabel(m.pins.length, 'pin')}, ${linked} with a page` : 'No pins yet',
                m.scale ? `scale in ${m.scale.unit}` : 'no scale',
                `image added by ${m.image.uploaded_by_user.display_name}, ${this.mapDay(m.image.created_at)}`
            ].join(' · ');
        },
        get mapPinList() {
            const q = codexNorm(this.mapQ);
            return this.mapPins.filter((p) => !q || codexNorm(p.name).includes(q)).sort((a, b) => a.name.localeCompare(b.name));
        },
        // The icons in use with their pin counts.
        get mapKey() {
            const counts = new Map();
            for (const p of this.mapPins) counts.set(p.icon, (counts.get(p.icon) || 0) + 1);
            return [...counts].map(([id, count]) => ({ id, count, label: this.pinLabel(id), on: !this.mapHidden.includes(id) })).sort((a, b) => a.label.localeCompare(b.label));
        },
        // The maps that have a pin pointing at this page.
        get onMaps() {
            return ((this.pageData && this.pageData.on_maps) || []).map((o) => ({ key: o.pin_id, title: o.map.title, url: this.pageUrl(o.map) + '?pin=' + encodeURIComponent(o.pin_id) }));
        },
        get mapBanner() {
            if (this.mapMode === 'place') return 'Click where the pin goes.';
            if (this.mapMode === 'move') return `Drag “${(this.mapPins.find((p) => p.id === this.mapMoveId) || {}).name || ''}”, click its new place, or use the arrow keys.`;
            if (this.mapMode === 'ruler') {
                const r = this.mapRuler;
                if (r.points < 2) return 'Click points along the way to measure.';
                return 'Total ' + window.codexDistance(r.total, this.mapData.scale.unit) + (r.legs.length > 1 ? `, ${r.legs.length} legs` : '');
            }
            if (this.mapMode === 'scale') {
                return [
                    "Click two points you know the distance between, like the ends of the map's own scale bar.",
                    'Now click the second point.',
                    'Set the distance, or click again to start over.'
                ][this.mapScalePts.length];
            }
            return '';
        },

        pinLabel(id) {
            return iconLabels[id] || 'Pin';
        },
        pinSvg(id) {
            return window.codexPinSvg(id);
        },
        mapDay(iso) {
            return new Date(iso).toLocaleDateString('default', { day: 'numeric', month: 'short', year: 'numeric' });
        },
        mapUrl(tail) {
            return `/api/codices/pages/${encodeURIComponent(this.page.id)}/${tail}`;
        },
        viewPins() {
            return this.mapPins.map((p) => ({ id: p.id, x: p.x, y: p.y, name: p.name, icon: p.icon, color: p.color, linked: !!p.linked }));
        },

        // Shows the map in its stage, or brings a shown one up to date. Called after every load and change.
        async mountMap() {
            const el = document.getElementById('cxMap');
            const image = this.mapData.image;
            if (!el || !image || mounting) return;
            if (view && (view.el !== el || view.imageUrl !== image.url)) {
                view.destroy();
                view = null;
            }
            if (view) {
                view.setScale(this.mapData.scale);
                view.setPins(this.viewPins());
                return;
            }
            document.getElementById('cxMapStage').style.setProperty('--mp-ar', image.width + ' / ' + image.height);
            mounting = true;
            try {
                view = await window.mountCodexMap(el, {
                    image, imageAlt: this.page.title, pins: this.viewPins(), scale: this.mapData.scale,
                    onPin: (id) => this.openPin(id),
                    onBackground: () => this.closePin(),
                    onView: () => this.placeMapCard(),
                    onPlace: (at) => this.openPinDialog(null, at),
                    onRuler: (r) => { this.mapRuler = r; },
                    onScaleLine: (pts) => { this.mapScalePts = pts; }
                });
                this.mapError = '';
            } catch (error) {
                this.mapError = 'Could not load the map: ' + error.message;
                return;
            } finally {
                mounting = false;
            }
            if (view.imageUrl !== this.mapData.image.url) { // replaced while the viewer was loading
                this.mountMap();
                return;
            }
            // Handlers of Alpine live on elements; the map's own keys are the document's.
            if (!this.mapKeysOn) {
                this.mapKeysOn = true;
                document.addEventListener('keydown', (event) => this.onMapKey(event));
            }
            const q = new URLSearchParams(location.search);
            const pin = q.get('pin');
            if (pin) {
                history.replaceState(null, '', location.pathname);
                if (this.mapPins.some((p) => p.id === pin)) {
                    document.getElementById('cxMapStage').scrollIntoView({ block: 'nearest' });
                    this.goToPin(pin);
                } else {
                    this.showMessage('That pin is no longer on the map.', 'info');
                }
            }
        },
        setMap(map) {
            // A redraw ends a move, and there is nothing to measure with once the scale is gone.
            if (this.mapMode === 'move' || (this.mapMode === 'ruler' && !map.scale)) this.stopMapMode();
            this.pageData.map = map;
            if (this.mapOpenId && !this.mapOpenPin) this.closePin();
            this.mapHidden = this.mapHidden.filter((i) => map.pins.some((p) => p.icon === i));
            if (view) view.setHidden(new Set(this.mapHidden));
            this.$nextTick(() => this.mountMap());
        },
        async refreshMap() {
            try {
                const { response, data } = await this.send(this.mapUrl('map'), 'GET');
                if (response.ok) this.setMap(data.map);
            } catch (error) {
                console.error('Failed to reload the map:', error);
            }
        },
        // The error text of a failed change outside a dialog; a pin that is gone also reloads the map.
        mapFailed(response, data, what) {
            if (data.code === 'pin_deleted') this.refreshMap();
            this.showMessage(response.status === 401 ? 'Your login has expired. Log in again in another tab, then try again.'
                : data.code === 'pin_deleted' ? 'Someone deleted that pin.'
                    : data.code === 'already_undone' ? 'That change was already undone.' : this.sentence(data.error || what), 'error');
        },

        // ---- Preview ----
        openPin(id, focus = true) {
            if (!view) return;
            this.mapOpenId = id;
            view.setOpen(id);
            this.$nextTick(() => {
                const card = document.getElementById('cxMapCard');
                if (!card) return;
                if (window.matchMedia('(max-width: 600px)').matches) view.reveal(id, card.offsetHeight);
                this.placeMapCard();
                if (focus) document.getElementById('cxMapCardTitle')?.focus({ preventScroll: true });
            });
        },
        placeMapCard() {
            const card = document.getElementById('cxMapCard');
            if (!this.mapOpenId || !card || !view) return;
            const at = view.cardSpot(this.mapOpenId, card.offsetWidth, card.offsetHeight);
            if (at) this.mapCardPos = at;
        },
        closePin(refocus = false) {
            if (!this.mapOpenId) return;
            const id = this.mapOpenId;
            this.mapOpenId = '';
            if (view) {
                view.setOpen('');
                if (refocus) view.focusPin(id);
            }
        },
        goToPin(id) {
            this.stopMapMode();
            if (!view) return;
            view.flyTo(id);
            this.openPin(id);
        },

        // ---- Tools ----
        mapZoom(by) {
            if (view) by > 0 ? view.zoomIn() : view.zoomOut();
        },
        mapFit() {
            if (view) view.fit();
        },
        toggleMapFull() {
            this.mapFull = !this.mapFull;
            document.body.classList.toggle('mp-full-on', this.mapFull);
            this.hidePeek();
        },
        startMapMode(mode) {
            if (!view) return;
            const same = this.mapMode === mode;
            this.stopMapMode();
            if (same) return;
            this.closePin();
            this.mapScalePts = [];
            this.mapRuler = { points: 0, legs: [], total: 0 };
            this.mapMode = mode;
            view.setMode(mode);
            view.focus();
        },
        stopMapMode() {
            if (!this.mapMode) return;
            if (view && this.mapMode === 'move') view.endMove(false);
            if (view) view.setMode('');
            this.mapMode = '';
            this.mapMoveId = '';
        },
        undoMapPoint() {
            if (view) view.undoPoint();
        },
        clearMapPoints() {
            if (view) view.clearPoints();
        },
        openScale() {
            if (!this.mapData.image) {
                this.showMessage("Add the map's image first.", 'info');
                return;
            }
            document.getElementById('cxMapStage')?.scrollIntoView({ block: 'nearest' });
            this.startMapMode('scale');
        },
        toggleMapIcon(id) {
            if (this.mapMode === 'move') this.stopMapMode();
            this.mapHidden = this.mapHidden.includes(id) ? this.mapHidden.filter((i) => i !== id) : [...this.mapHidden, id];
            if (this.mapOpenPin && this.mapHidden.includes(this.mapOpenPin.icon)) this.closePin();
            if (view) view.setHidden(new Set(this.mapHidden));
        },
        // Esc leaves, in this order: a mode, the preview, full screen. Enter saves a move.
        onMapKey(event) {
            if (!this.isMap || this.screen !== 'page' || this.dlg.type || this.confirmDialog.open || this.moreOpen || document.querySelector('dialog[open]')) return;
            const inMap = event.target.closest && event.target.closest('#cxMap');
            if (event.key === 'Enter' && this.mapMode === 'move' && inMap) {
                event.preventDefault();
                this.saveMove();
            } else if (event.key === 'Escape') {
                if (this.mapMode) {
                    const id = this.mapMoveId;
                    this.stopMapMode();
                    if (id && view) view.focusPin(id);
                } else if (this.mapOpenId) this.closePin(true);
                else if (this.mapFull) this.toggleMapFull();
            } else if (event.key === 'Tab' && this.mapFull) {
                // Full screen covers the page, so Tab stays inside it.
                const all = [...document.querySelectorAll('#cxMapStage :is(button, a[href], [tabindex="0"])')].filter((n) => n.getClientRects().length && !n.disabled);
                const i = all.indexOf(document.activeElement);
                if (event.shiftKey ? i <= 0 && document.activeElement.id !== 'cxMapCardTitle' : i === all.length - 1 || !event.target.closest('#cxMapStage')) {
                    event.preventDefault();
                    (event.shiftKey ? all[all.length - 1] : all[0])?.focus();
                }
            }
        },

        // ---- Move, delete, undo ----
        startMove(p) {
            this.stopMapMode();
            this.closePin();
            this.mapMoveId = p.id;
            this.mapMode = 'move';
            view.startMove(p.id);
        },
        async saveMove() {
            const id = this.mapMoveId;
            const p = this.mapPins.find((x) => x.id === id);
            if (!view || !p) return;
            const at = view.endMove(true);
            this.mapMode = '';
            this.mapMoveId = '';
            if (!at) return;
            await this.changePin('PUT', p, { x: at.x, y: at.y }, `Moved “${p.name}”.`);
            this.$nextTick(() => view && view.focusPin(id));
        },
        async deletePin(p) {
            await this.changePin('DELETE', p, undefined, `Deleted “${p.name}”.`);
        },
        // Sends a move or delete and offers Undo in the toast; on failure the map shows the saved state again.
        async changePin(method, p, body, done) {
            try {
                const { response, data } = await this.send(this.mapUrl('pins/' + encodeURIComponent(p.id)), method, body);
                if (!response.ok) {
                    this.mapFailed(response, data, 'Could not change the pin');
                    this.mountMap();
                    return;
                }
                this.setMap(data.map);
                if (data.event_id) this.showMessage(done, 'success', { label: 'Undo', run: () => this.undoPin(data.event_id) });
            } catch (error) {
                this.showMessage('Could not change the pin: ' + error.message, 'error');
                this.mountMap();
            }
        },
        async undoPin(eventId) {
            try {
                const { response, data } = await this.send(this.mapUrl(`pin-events/${encodeURIComponent(eventId)}/undo`), 'POST', {});
                if (!response.ok) {
                    this.mapFailed(response, data, 'Could not undo the change');
                    return false;
                }
                this.setMap(data.map);
                this.showMessage('Undone.', 'success');
                return true;
            } catch (error) {
                this.showMessage('Could not undo the change: ' + error.message, 'error');
                return false;
            }
        },

        // ---- Pin dialog ----
        // p: a pin of this map to change, or null to add one at the place `at`.
        async openPinDialog(p, at) {
            if (!this.canEdit || !this.isMap) return;
            if (this.navPromise) await this.navPromise; // the codex's pages, for the page picker
            const l = p && p.linked;
            const last = this.lastPinLook || { icon: 'place', color: 'red' };
            this.dlg = {
                type: 'pin', pinId: p ? p.id : '', at: at || null, title: p ? p.name : '', ownTitle: p && !l ? p.name : '', desc: p ? p.description : '',
                linked: l ? { id: l.id, title: l.title, folder: l.folder_id ? (this.folderById(l.folder_id) || {}).name || '' : 'Top level', tags: [] } : null,
                icon: p ? p.icon : last.icon, color: p ? p.color : last.color,
                pickQ: '', pickOpen: false, pickIdx: 0, createPage: false, error: '', busy: false
            };
            this.$nextTick(() => requestAnimationFrame(() => document.getElementById(l ? 'dPinUnlink' : 'dPinName')?.focus()));
        },
        unlinkPin() {
            const d = this.dlg;
            d.linked = null;
            d.title = d.ownTitle || d.title;
            this.$nextTick(() => document.getElementById('dPinName')?.focus());
        },
        // Arrow keys move through a radio group whose values are in order.
        onPinRadio(event, values, key) {
            const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[event.key];
            if (!step) return;
            event.preventDefault();
            const d = this.dlg;
            d[key] = values[(values.indexOf(d[key]) + step + values.length) % values.length];
            const group = event.currentTarget;
            this.$nextTick(() => group.querySelector('[aria-checked="true"]')?.focus());
        },
        get pinIconIds() {
            return window.CODEX_PIN_ICONS.flatMap((g) => g.icons.map((i) => i[0]));
        },
        get pinPageHint() {
            const d = this.dlg;
            return `(an article in the map's folder, from the ${d.icon === 'settlement' || d.icon === 'city' ? 'Settlement' : 'Landmark'} template)`;
        },
        async submitPin(d) {
            const name = d.title.trim().split(/\s+/).join(' ');
            if (!d.linked && !name) {
                d.error = 'Give the pin a name or link a page.';
                return;
            }
            if (!d.linked && Array.from(d.desc.trim()).length > 1000) {
                d.error = 'The description can be up to 1000 characters.';
                return;
            }
            const create = !d.linked && !d.pinId && d.createPage;
            if (create && /[[\]|]/.test(name)) {
                d.error = "A page title can't contain [ ] or |.";
                return;
            }
            const body = { name, description: d.linked ? '' : d.desc, linked_page_id: d.linked ? d.linked.id : '', icon: d.icon, color: d.color };
            const { response, data } = d.pinId
                ? await this.send(this.mapUrl('pins/' + encodeURIComponent(d.pinId)), 'PUT', body)
                : await this.send(this.mapUrl('pins'), 'POST', { ...body, x: d.at.x, y: d.at.y, create_page: create });
            if (!response.ok) {
                if (data.code === 'pin_deleted') this.refreshMap();
                this.failDlg(d, response, data);
                return;
            }
            this.lastPinLook = { icon: d.icon, color: d.color };
            this.closeDlg();
            this.stopMapMode();
            this.mapHidden = this.mapHidden.filter((i) => i !== d.icon);
            this.setMap(data.map);
            const id = d.pinId || data.pin.id;
            this.$nextTick(() => requestAnimationFrame(() => this.openPin(id)));
            if (data.page) {
                this.showMessage(`Created the page “${data.page.title}”.`, 'success');
                this.loadNav(this.page);
            }
        },

        // ---- Scale ----
        openScaleDialog() {
            const s = this.mapData.scale;
            this.dlg = { type: 'mapScale', pts: [...this.mapScalePts], length: '', unit: s ? s.unit : 'miles', error: '', busy: false };
            this.focusDlg();
        },
        async submitScale(d) {
            const length = Number(String(d.length).trim().replace(',', '.'));
            const unit = d.unit.trim();
            if (!(length > 0) || length > 1e9 || !unit || Array.from(unit).length > 20) {
                d.error = 'Enter a distance above 0 and a unit of up to 20 characters.';
                return;
            }
            const [a, b] = d.pts;
            const { response, data } = await this.send(this.mapUrl('map/scale'), 'PUT', { x1: a.x, y1: a.y, x2: b.x, y2: b.y, length, unit });
            if (!response.ok) {
                this.failDlg(d, response, data);
                return;
            }
            this.closeDlg();
            this.stopMapMode();
            this.setMap(data.map);
            this.showMessage('Scale set.', 'success');
        },
        async clearScale() {
            this.stopMapMode();
            try {
                const { response, data } = await this.send(this.mapUrl('map/scale'), 'DELETE');
                if (!response.ok) {
                    this.mapFailed(response, data, 'Could not clear the scale');
                    return;
                }
                this.setMap(data.map);
                this.showMessage('Scale cleared. The ruler is back once a scale is set.', 'success');
            } catch (error) {
                this.showMessage('Could not clear the scale: ' + error.message, 'error');
            }
        },

        // ---- Image ----
        chooseMapImage() {
            this.$refs.mapInput?.click();
        },
        onMapFile(event) {
            const file = event.target.files && event.target.files[0];
            event.target.value = '';
            if (file) this.uploadMapImage(file);
        },
        onMapDrop(event) {
            this.mapUp.over = false;
            const file = this.canEdit && !this.mapUp.busy && event.dataTransfer && event.dataTransfer.files[0];
            if (file) this.uploadMapImage(file);
        },
        async uploadMapImage(file) {
            if (this.mapUp.busy) return;
            if (file.size > 25 * 1024 * 1024) {
                this.showMessage('The image must be 25 MB or smaller.', 'error');
                return;
            }
            this.stopMapMode();
            this.closePin();
            this.mapUp.busy = true;
            this.mapUp.pct = 0;
            try {
                const { status, data } = await codexUpload('POST', this.mapUrl('images'), file, (p) => { this.mapUp.pct = p; });
                if (status !== 201) {
                    this.showMessage(status === 401 ? 'Your login has expired. Log in again in another tab, then try again.' : this.sentence(data.error || 'Could not save the image'), 'error');
                    return;
                }
                this.setMap(data.map);
                this.showMessage(data.scale_cleared ? 'Image saved. Its shape is different, so the scale was cleared: check the pins and set it again.' : 'Image saved.', data.scale_cleared ? 'info' : 'success');
            } catch (error) {
                this.showMessage('Could not save the image: ' + error.message, 'error');
            } finally {
                this.mapUp.busy = false;
            }
        },

        // ---- History ----
        async loadMapHistory() {
            this.mapHist.loading = !this.mapHist.images.length && !this.mapHist.events.length;
            try {
                const [events, images] = await Promise.all([this.send(this.mapUrl('pin-events'), 'GET'), this.send(this.mapUrl('images'), 'GET')]);
                if (!events.response.ok || !images.response.ok) throw new Error('request failed');
                this.mapHist.events = events.data;
                this.mapHist.images = images.data;
            } catch (error) {
                this.loadError = 'Could not load the history: ' + error.message;
            } finally {
                this.mapHist.loading = false;
            }
        },
        mapEventText(e) {
            return `${{ add: 'added', edit: 'edited', move: 'moved', remove: 'removed' }[e.action] || 'changed'} “${e.name}”`;
        },
        async undoMapEvent(e) {
            if (this.busy) return;
            this.busy = true;
            try {
                await this.undoPin(e.id);
                await this.loadMapHistory();
            } finally {
                this.busy = false;
            }
        },
        async restoreMapImage(img) {
            if (this.busy) return;
            this.busy = true;
            try {
                const { response, data } = await this.send(this.mapUrl(`images/${encodeURIComponent(img.id)}/restore`), 'POST', {});
                if (!response.ok) {
                    this.mapFailed(response, data, 'Could not restore the image');
                    return;
                }
                this.setMap(data.map);
                await this.loadMapHistory();
                this.showMessage(data.scale_cleared ? 'Image restored. Its shape is different, so the scale was cleared.' : 'Image restored.', data.scale_cleared ? 'info' : 'success');
            } catch (error) {
                this.showMessage('Could not restore the image: ' + error.message, 'error');
            } finally {
                this.busy = false;
            }
        }
    };

    // Adds the map part (with its getters) to the page component.
    window.codexWithMap = (component) => Object.defineProperties(component, Object.getOwnPropertyDescriptors(mapPart));
})();
