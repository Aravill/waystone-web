// The Codex map viewer: a Leaflet map (simple CRS, no tiles) showing one image with pins on it.
// Leaflet (static/vendor/leaflet) loads on first use. Pin positions are fractions (0..1) of the
// image, x from the left and y from the top.
(function () {
    const loading = {};
    function loadOnce(src, css) {
        if (!loading[src]) {
            loading[src] = new Promise((resolve, reject) => {
                const el = document.createElement(css ? 'link' : 'script');
                if (css) { el.rel = 'stylesheet'; el.href = src; } else { el.src = src; }
                el.onload = resolve;
                el.onerror = () => reject(new Error('Could not load ' + src));
                document.head.appendChild(el);
            });
        }
        return loading[src];
    }

    // mountCodexMap(el, { image, pins, onPinClick(pin), onMapClick(info | null), onPinMove(id, x, y) })
    // resolves to { el, imageUrl, setPins, setOpen, setMovable, focusPin, destroy }.
    // image is { url, width, height }; a pin is { id, x, y, target_text, status, target }.
    window.mountCodexMap = async function (el, { image, pins, onPinClick, onMapClick, onPinMove }) {
        await Promise.all([loadOnce('/static/vendor/leaflet/leaflet.css?v=1', true), loadOnce('/static/vendor/leaflet/leaflet.js?v=1')]);
        const w = image.width, h = image.height;
        const bounds = [[0, 0], [h, w]];
        const map = L.map(el, { crs: L.CRS.Simple, minZoom: -4, maxZoom: 3, zoomSnap: 0.25, attributionControl: false });
        L.imageOverlay(image.url, bounds).addTo(map);
        map.fitBounds(bounds);
        map.setMaxBounds(L.latLngBounds(bounds).pad(0.5));

        const Fit = L.Control.extend({
            onAdd() {
                const box = L.DomUtil.create('div', 'leaflet-bar');
                const a = L.DomUtil.create('a', '', box);
                a.href = '#';
                a.textContent = 'Fit';
                a.title = 'Fit the whole map';
                a.setAttribute('role', 'button');
                L.DomEvent.on(a, 'click', (e) => { L.DomEvent.preventDefault(e); map.fitBounds(bounds); });
                L.DomEvent.disableClickPropagation(box);
                return box;
            }
        });
        new Fit({ position: 'topleft' }).addTo(map);

        const toLatLng = (p) => [h * (1 - p.y), w * p.x];
        const label = (p) => (p.target ? p.target.title : p.target_text);
        let current = [], openId = '', movableId = '';
        const markers = new Map();

        function iconFor(p) {
            const kind = p.target ? p.target.kind : '';
            const cls = ['codex-pin', kind === 'map' ? 'codex-pin--map' : '', p.status !== 'ok' ? 'codex-pin--missing' : '', p.id === openId ? 'codex-pin--open' : ''];
            return L.divIcon({ className: cls.filter(Boolean).join(' '), html: kind === 'map' ? '◈' : '✦', iconSize: [30, 30], iconAnchor: [15, 15] });
        }

        function render(list) {
            current = list;
            markers.forEach((m) => m.remove());
            markers.clear();
            for (const p of list) {
                const m = L.marker(toLatLng(p), { icon: iconFor(p), title: label(p), draggable: p.id === movableId, riseOnHover: true });
                const tip = document.createElement('span');
                tip.textContent = label(p) + (p.status === 'ok' ? '' : ' (no page yet)');
                m.bindTooltip(tip, { direction: 'top', offset: [0, -14] });
                m.on('click', () => onPinClick(p));
                m.on('dragend', () => {
                    const ll = m.getLatLng();
                    onPinMove(p.id, Math.min(1, Math.max(0, ll.lng / w)), Math.min(1, Math.max(0, 1 - ll.lat / h)));
                });
                m.addTo(map);
                markers.set(p.id, m);
            }
        }

        map.on('click', (e) => {
            const x = e.latlng.lng / w, y = 1 - e.latlng.lat / h;
            if (x < 0 || x > 1 || y < 0 || y > 1) { onMapClick(null); return; }
            onMapClick({ x, y, px: e.containerPoint.x, py: e.containerPoint.y });
        });
        map.on('movestart zoomstart', () => onMapClick(null));
        render(pins);

        return {
            el,
            imageUrl: image.url,
            setPins: render,
            setOpen(id) {
                openId = id || '';
                current.forEach((p) => markers.get(p.id)?.setIcon(iconFor(p)));
            },
            // Only this pin can be dragged ('' for none); it keeps its position until dropped.
            setMovable(id) {
                movableId = id || '';
                markers.forEach((m, pid) => (pid === movableId ? m.dragging.enable() : m.dragging.disable()));
            },
            focusPin(id) {
                const m = markers.get(id);
                if (m) map.setView(m.getLatLng(), Math.max(map.getZoom(), 1));
            },
            destroy() { map.remove(); }
        };
    };
})();
