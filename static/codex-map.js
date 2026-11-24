// The Codices map viewer: a Leaflet map (simple CRS, no tiles) showing one image with pins on it.
// Leaflet (static/vendor/leaflet) loads on first use. Positions are fractions (0..1) of the image,
// x from the left and y from the top; lengths are measured in pixels of the image.
(function () {
    const loading = {};
    function loadOnce(src, css) {
        if (!loading[src]) {
            loading[src] = new Promise((resolve, reject) => {
                const el = document.createElement(css ? 'link' : 'script');
                if (css) { el.rel = 'stylesheet'; el.href = src; } else { el.src = src; }
                el.onload = resolve;
                el.onerror = () => {
                    delete loading[src];
                    reject(new Error('Could not load ' + src));
                };
                document.head.appendChild(el);
            });
        }
        return loading[src];
    }
    async function loadLeaflet() {
        if (window.L) return;
        await Promise.all([loadOnce('/static/vendor/leaflet/leaflet.css?v=1', true), loadOnce('/static/vendor/leaflet/leaflet.js?v=1')]);
    }

    const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));
    const PIN_SIZE = 30; // the tile's side in px at full size
    const PIN_TAIL = 0.27; // the tail under it, as a part of the side (codex.css .mp-in)

    const iconBodies = {};
    for (const g of window.CODEX_PIN_ICONS || []) for (const [id, , body] of g.icons) iconBodies[id] = body;

    // An icon as an inline SVG string (ids come from our own list, so no escaping is needed).
    window.codexPinSvg = (id) => `<svg viewBox="0 0 512 512" aria-hidden="true">${iconBodies[id] || iconBodies.place || ''}</svg>`;

    // "12 miles", "0.4 km", "1,250 feet": about three significant digits.
    window.codexDistance = function (n, unit) {
        const digits = n >= 100 ? 0 : n >= 10 ? 1 : 2;
        return Number(n.toFixed(digits)).toLocaleString('en-US') + ' ' + unit;
    };

    // mountCodexMap(el, opts) resolves to the viewer. opts:
    //   image {url, width, height}, pins, scale ({x1,y1,x2,y2,length,unit} or null),
    //   onPin(id), onBackground(), onView() (panned or zoomed), onPlace({x,y}), onMove({x,y}),
    //   onRuler({legs, total}), onScaleLine(points), imageAlt.
    // A pin is {id, x, y, name, icon, color, linked}.
    window.mountCodexMap = async function (el, opts) {
        await loadLeaflet();
        const L = window.L;
        const w = opts.image.width, h = opts.image.height;
        const bounds = L.latLngBounds([[0, 0], [h, w]]);
        const map = L.map(el, {
            crs: L.CRS.Simple, zoomSnap: 0, zoomDelta: 0.5, wheelPxPerZoomLevel: 140, minZoom: -8, maxZoom: 3,
            attributionControl: false, zoomControl: false, maxBoundsViscosity: 0.8
        });
        const overlay = L.imageOverlay(opts.image.url, bounds, { alt: opts.imageAlt || 'Map' }).addTo(map);
        const toLL = (p) => L.latLng(h * (1 - p.y), w * p.x);
        const toFrac = (ll) => ({ x: clamp(ll.lng / w, 0, 1), y: clamp(1 - ll.lat / h, 0, 1) });
        const inside = (ll) => ll.lng >= 0 && ll.lng <= w && ll.lat >= 0 && ll.lat <= h;

        let pins = [], hidden = new Set(), openId = '', mode = '', scale = opts.scale || null, destroyed = false;
        let fitZoom = 0, wholeZoom = null; // the zoom that shows the whole map; the zoom left by the last fit
        const markers = new Map();

        // ---- zoom: pins shrink as the map zooms out ----
        function setLimits() {
            if (!el.clientWidth) return; // not shown yet
            fitZoom = map.getBoundsZoom(bounds);
            map.setMinZoom(fitZoom - 0.5);
            map.setMaxZoom(Math.max(fitZoom + 3, 2));
            map.setMaxBounds(bounds.pad(1)); // room to pan a pin out from under the preview sheet
        }
        // Whole, even sizes keep the tiles sharp.
        function pinSize(zoom = map.getZoom()) {
            return 2 * Math.round(PIN_SIZE * clamp(0.6 + 0.2 * (zoom - fitZoom), 0.5, 1) / 2);
        }
        function applyPinSize(zoom) {
            el.style.setProperty('--mp-t', pinSize(zoom) + 'px');
        }
        function fit(animate) {
            map.fitBounds(bounds, { animate: !!animate });
            wholeZoom = animate ? fitZoom : map.getZoom();
        }

        // ---- pins ----
        function pinIcon(p) {
            const tile = document.createElement('span');
            tile.className = 'mp-pin mp-c-' + p.color + (p.linked ? ' mp-pin--linked' : '');
            tile.innerHTML = `<span class="mp-label"></span><span class="mp-in"><span class="mp-tile">${window.codexPinSvg(p.icon)}</span></span>`;
            tile.querySelector('.mp-label').textContent = p.name;
            return L.divIcon({ className: 'mp-marker', html: tile, iconSize: [0, 0] });
        }
        function addMarker(p) {
            const m = L.marker(toLL(p), { icon: pinIcon(p), keyboard: true, riseOnHover: true, draggable: false, autoPanOnFocus: false });
            m.on('click', () => {
                if (!mode) opts.onPin(p.id);
                else if (mode !== 'move') clickAt(m.getLatLng());
            });
            m.addTo(map);
            const node = m.getElement();
            node.setAttribute('aria-label', p.name + (p.linked ? ', has a page' : ''));
            node.dataset.pin = p.id;
            node.addEventListener('keydown', (e) => {
                if ((e.key !== 'Enter' && e.key !== ' ') || mode === 'move') return; // Enter saves a move (the page's keys)
                e.preventDefault();
                e.stopPropagation();
                if (!mode) opts.onPin(p.id);
                else clickAt(m.getLatLng());
            });
            for (const t of ['mouseenter', 'focus']) node.addEventListener(t, () => showLabel(p.id, true));
            for (const t of ['mouseleave', 'blur']) node.addEventListener(t, () => showLabel(p.id, false));
            markers.set(p.id, m);
        }
        function showLabel(id, on) {
            markers.get(id)?.getElement()?.classList.toggle('is-hot', on);
        }
        function setPins(list) {
            if (moving) endMove(false);
            pins = list;
            markers.forEach((m) => m.remove());
            markers.clear();
            for (const p of list) if (!hidden.has(p.icon) || p.id === openId) addMarker(p);
            markOpen();
            layoutLabels();
        }
        function markOpen() {
            markers.forEach((m, id) => m.getElement()?.classList.toggle('is-open', id === openId));
        }
        // Labels are placed greedily, the open pin first: one that would cover another label or a
        // pin is hidden until the map is zoomed in, or the pin is hovered or focused.
        function layoutLabels() {
            if (destroyed) return;
            const tw = pinSize(), th = tw * (1 + PIN_TAIL);
            const items = [];
            markers.forEach((m, id) => {
                const node = m.getElement();
                const label = node && node.querySelector('.mp-label');
                if (!label) return;
                const pt = map.latLngToContainerPoint(m.getLatLng());
                const lw = label.offsetWidth;
                label.style.transform = 'none'; // a whole-pixel offset, so the text stays sharp
                label.style.marginLeft = -Math.round(lw / 2) + 'px';
                items.push({ id, node, pt, lw, lh: label.offsetHeight, tile: [pt.x - tw / 2, pt.y - th, pt.x + tw / 2, pt.y] });
            });
            items.sort((a, b) => (b.id === openId) - (a.id === openId) || a.pt.y - b.pt.y || a.pt.x - b.pt.x);
            const hit = (a, b) => a[0] < b[2] && b[0] < a[2] && a[1] < b[3] && b[1] < a[3];
            const placed = [];
            for (const it of items) {
                const bottom = it.pt.y - th - 2;
                const box = [it.pt.x - it.lw / 2 - 2, bottom - it.lh, it.pt.x + it.lw / 2 + 2, bottom];
                const clash = placed.some((b) => hit(b, box)) || items.some((o) => o !== it && hit(o.tile, box));
                const show = !clash || it.id === openId;
                it.node.classList.toggle('no-label', !show);
                if (show) placed.push(box);
            }
        }

        // ---- modes: '' (look), 'place' (click a spot), 'move' (one pin), 'ruler', 'scale' ----
        let moving = null; // {id, marker, from, at}
        let points = []; // ruler and scale points, as LatLng
        const drawn = L.layerGroup().addTo(map);

        function setMode(next) {
            if (mode === 'move') endMove(false);
            mode = next || '';
            points = [];
            draw();
            el.classList.toggle('is-aiming', mode === 'place' || mode === 'ruler' || mode === 'scale' || mode === 'move');
            el.dataset.mode = mode;
        }
        function clickAt(ll) {
            if (!inside(ll)) return;
            if (mode === 'place') opts.onPlace(toFrac(ll));
            else if (mode === 'move') moveTo(ll);
            else if (mode === 'ruler') { points.push(ll); draw(); }
            else if (mode === 'scale') {
                if (points.length >= 2) points = [];
                points.push(ll);
                draw();
            }
        }
        function startMove(id) {
            setMode('');
            const m = markers.get(id), p = pins.find((x) => x.id === id);
            if (!m || !p) return;
            mode = 'move';
            el.dataset.mode = mode;
            el.classList.add('is-aiming');
            moving = { id, marker: m, at: { x: p.x, y: p.y }, from: { x: p.x, y: p.y } };
            m.dragging.enable();
            m.on('drag', onDrag);
            m.getElement().classList.add('is-moving');
            m.getElement().focus({ preventScroll: true });
        }
        function onDrag() {
            const f = toFrac(moving.marker.getLatLng());
            moving.at = f;
            moving.marker.setLatLng(toLL(f));
            opts.onMove && opts.onMove(f);
        }
        function moveTo(ll) {
            if (!moving) return;
            moving.at = toFrac(ll);
            moving.marker.setLatLng(toLL(moving.at));
            layoutLabels();
            opts.onMove && opts.onMove(moving.at);
        }
        function nudge(dx, dy) {
            if (!moving) return;
            const pt = map.latLngToContainerPoint(moving.marker.getLatLng());
            const ll = map.containerPointToLatLng([pt.x + dx, pt.y + dy]);
            moveTo(L.latLng(clamp(ll.lat, 0, h), clamp(ll.lng, 0, w)));
            map.panInside(moving.marker.getLatLng(), { padding: [40, 60] });
        }
        // Ends the move; keep leaves the pin where it is (the caller saves it), otherwise it goes back.
        function endMove(keep) {
            if (!moving) return null;
            const { marker, at, from } = moving;
            marker.dragging && marker.dragging.disable();
            marker.off('drag', onDrag);
            marker.getElement()?.classList.remove('is-moving');
            if (!keep) marker.setLatLng(toLL(from));
            moving = null;
            mode = '';
            el.dataset.mode = '';
            el.classList.remove('is-aiming');
            layoutLabels();
            return at;
        }

        // ---- ruler and scale line ----
        const pxDist = (a, b) => Math.hypot(a.lng - b.lng, a.lat - b.lat);
        // Units per image pixel; 0 without a scale.
        function perPx() {
            if (!scale) return 0;
            const d = pxDist(toLL({ x: scale.x1, y: scale.y1 }), toLL({ x: scale.x2, y: scale.y2 }));
            return d > 0 ? scale.length / d : 0;
        }
        function note(ll, text, cls) {
            const span = document.createElement('span');
            span.textContent = text;
            L.marker(ll, { pane: 'tooltipPane', interactive: false, keyboard: false, icon: L.divIcon({ className: 'mp-note ' + (cls || ''), html: span, iconSize: [0, 0] }) }).addTo(drawn);
        }
        function draw() {
            drawn.clearLayers();
            const legs = [];
            if (points.length) {
                if (points.length > 1) {
                    L.polyline(points, { className: 'mp-line mp-line--halo', interactive: false }).addTo(drawn);
                    L.polyline(points, { className: 'mp-line', interactive: false }).addTo(drawn);
                }
                for (const p of points) L.circleMarker(p, { radius: 5, className: 'mp-dot', interactive: false }).addTo(drawn);
                if (mode === 'ruler') {
                    const k = perPx();
                    for (let i = 1; i < points.length; i++) {
                        const d = pxDist(points[i - 1], points[i]) * k;
                        legs.push(d);
                        if (points.length > 2) note(L.latLng((points[i - 1].lat + points[i].lat) / 2, (points[i - 1].lng + points[i].lng) / 2), window.codexDistance(d, scale.unit));
                    }
                    if (legs.length) note(points[points.length - 1], window.codexDistance(legs.reduce((a, b) => a + b, 0), scale.unit), 'mp-note--total');
                }
            }
            if (mode === 'ruler') opts.onRuler && opts.onRuler({ points: points.length, legs, total: legs.reduce((a, b) => a + b, 0) });
            if (mode === 'scale') opts.onScaleLine && opts.onScaleLine(points.map(toFrac));
        }

        // ---- scale bar ----
        const bar = document.createElement('div');
        bar.className = 'mp-scale';
        bar.hidden = true;
        bar.innerHTML = '<span></span><i></i>';
        el.appendChild(bar);
        function drawBar() {
            const k = perPx();
            bar.hidden = !k;
            if (!k) return;
            const unitsPerScreenPx = k / Math.pow(2, map.getZoom());
            const max = unitsPerScreenPx * Math.min(140, el.clientWidth * 0.35);
            const pow = Math.pow(10, Math.floor(Math.log10(max)));
            const n = [5, 2, 1].map((f) => f * pow).find((v) => v <= max) || pow;
            bar.querySelector('span').textContent = window.codexDistance(n, scale.unit);
            bar.querySelector('i').style.width = (n / unitsPerScreenPx).toFixed(1) + 'px';
        }

        // ---- events ----
        map.on('click', (e) => {
            if (mode) clickAt(e.latlng);
            else opts.onBackground && opts.onBackground();
        });
        map.on('zoomanim', (e) => applyPinSize(e.zoom));
        map.on('zoom', () => applyPinSize());
        map.on('move zoom', () => opts.onView && opts.onView());
        map.on('zoomend moveend', () => { layoutLabels(); drawBar(); });
        // A map shown whole stays whole when its box changes size (full screen, page width).
        map.on('resize', () => {
            const whole = map.getZoom() === wholeZoom || Math.abs(map.getZoom() - wholeZoom) < 0.01;
            setLimits();
            if (whole) fit(false);
            applyPinSize();
            layoutLabels();
            drawBar();
        });
        el.addEventListener('keydown', (e) => {
            if (mode === 'move' && e.key.startsWith('Arrow')) {
                const step = e.shiftKey ? 10 : 2;
                nudge({ ArrowLeft: -step, ArrowRight: step }[e.key] || 0, { ArrowUp: -step, ArrowDown: step }[e.key] || 0);
                e.preventDefault();
                e.stopPropagation();
            } else if (e.key === 'Enter' && e.target === el && (mode === 'place' || mode === 'ruler' || mode === 'scale')) {
                e.preventDefault();
                clickAt(map.getCenter());
            } else if (e.key === 'Backspace' && mode === 'ruler') {
                e.preventDefault();
                points.pop();
                draw();
            }
        }, true);
        // Labels are measured, so lay them out again once the fonts are in.
        if (document.fonts && document.fonts.ready) document.fonts.ready.then(layoutLabels);
        const watch = new ResizeObserver(() => map.invalidateSize({ pan: true }));
        watch.observe(el);

        setLimits();
        fit(false);
        applyPinSize();
        setPins(opts.pins || []);
        drawBar();

        return {
            el,
            imageUrl: opts.image.url,
            setPins,
            setHidden(set) { hidden = set; setPins(pins); },
            setOpen(id) {
                const was = pins.find((p) => p.id === openId);
                openId = id || '';
                // A hidden pin shows only while it is open.
                if ((openId && !markers.has(openId)) || (was && was.id !== openId && hidden.has(was.icon))) setPins(pins);
                else { markOpen(); layoutLabels(); }
            },
            setScale(s) { scale = s || null; draw(); drawBar(); },
            setMode,
            mode: () => mode,
            startMove,
            endMove,
            undoPoint() { points.pop(); draw(); },
            clearPoints() { points = []; draw(); },
            // Where to put the preview card (w by h) of a pin: beside it and clear of its label, inside the map.
            cardSpot(id, w, h) {
                const m = markers.get(id);
                if (!m) return null;
                const pt = map.latLngToContainerPoint(m.getLatLng()), size = map.getSize();
                const pin = Math.max(pinSize(), 26) * (1 + PIN_TAIL);
                const top = pt.y - pin;
                const y = Math.max(8, Math.min(size.y - h - 8, top));
                const half = (m.getElement().querySelector('.mp-label')?.offsetWidth || 0) / 2;
                const gap = y < top ? Math.max(28, half + 8) : 28; // moved up beside the label
                let x = pt.x + gap;
                if (x + w > size.x - 8) x = pt.x - gap - w;
                if (x < 8) x = Math.max(8, Math.min(size.x - w - 8, pt.x - w / 2));
                return { x, y };
            },
            // Pans so the pin isn't under a sheet that covers the bottom of the map.
            reveal(id, bottom) {
                const m = markers.get(id);
                if (m) map.panInside(m.getLatLng(), { paddingTopLeft: [60, 70], paddingBottomRight: [60, bottom + 10] });
            },
            focusPin(id) { markers.get(id)?.getElement()?.focus({ preventScroll: true }); },
            flyTo(id) {
                const p = pins.find((x) => x.id === id);
                if (p) map.setView(toLL(p), Math.max(map.getZoom(), Math.min(fitZoom + 1.25, map.getMaxZoom())));
            },
            zoomIn: () => map.zoomIn(),
            zoomOut: () => map.zoomOut(),
            fit: () => fit(true),
            focus: () => el.focus({ preventScroll: true }),
            overlay,
            destroy() { destroyed = true; watch.disconnect(); map.remove(); bar.remove(); }
        };
    };
})();
