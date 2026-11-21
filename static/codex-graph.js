// The Codices graph: the full graph screen and the page's local graph, drawn as plain SVG.
// The layout is d3-force (static/vendor/d3-force.bundle.js, global D3F), loaded on first use.
// Node positions are never saved. Data is the /api/codices/c/{id}/graph payload.
(function () {
    const NS = 'http://www.w3.org/2000/svg';
    const PALETTE = 8; // --g-0 .. --g-7 in codex.css; top-level pages use --g-top
    let d3Promise = null;

    function loadD3() {
        if (window.D3F) return Promise.resolve(window.D3F);
        if (!d3Promise) {
            d3Promise = new Promise((resolve, reject) => {
                const el = document.createElement('script');
                el.src = '/static/vendor/d3-force.bundle.js?v=2';
                el.onload = () => resolve(window.D3F);
                el.onerror = () => {
                    d3Promise = null;
                    reject(new Error('Could not load the graph layout'));
                };
                document.head.appendChild(el);
            });
        }
        return d3Promise;
    }

    function svgEl(tag, cls) {
        const e = document.createElementNS(NS, tag);
        if (cls) e.setAttribute('class', cls);
        return e;
    }

    const radius = (d) => (d.missing ? 5 : 5 + Math.sqrt(d.degree) * 1.8);
    const plural = (n, word) => `${n} ${word}${n === 1 ? '' : 's'}`;

    // Nodes with link counts and colour slots, the neighbours of each, and one pair per linked
    // couple (A to B and B to A are two links in the data but one line).
    function prepare(data) {
        const slot = new Map((data.top_folders || []).map((f, i) => [f.id, i % PALETTE]));
        const byId = new Map();
        const nodes = data.nodes.map((n) => {
            const d = { ...n, inn: 0, out: 0, degree: 0, slot: n.top_folder ? slot.get(n.top_folder.id) : undefined };
            byId.set(d.id, d);
            return d;
        });
        const nb = new Map(nodes.map((d) => [d.id, new Set()]));
        const seen = new Map();
        for (const l of data.links) {
            const a = byId.get(l.source);
            const b = byId.get(l.target);
            if (!a || !b || a === b) continue;
            a.out++;
            b.inn++;
            nb.get(a.id).add(b.id);
            nb.get(b.id).add(a.id);
            const key = a.id < b.id ? a.id + '|' + b.id : b.id + '|' + a.id;
            if (!seen.has(key)) seen.set(key, [a, b]);
        }
        nodes.forEach((d) => (d.degree = d.inn + d.out));
        return { nodes, byId, nb, pairs: [...seen.values()] };
    }

    function colorOf(d) {
        return d.slot === undefined ? 'var(--g-top)' : `var(--g-${d.slot})`;
    }

    // Runs the force layout to rest (a fixed number of ticks, capped by time) and returns the simulation.
    function layout(D3, nodes, pairs, o) {
        const links = pairs.map(([a, b]) => ({ source: a, target: b }));
        const sim = D3.forceSimulation(nodes)
            .force('link', D3.forceLink(links).distance(o.distance).strength(0.4))
            .force('charge', D3.forceManyBody().strength(o.charge).distanceMax(500))
            .force('x', D3.forceX().strength((d) => (d.degree ? o.pull : o.pull * 3) * (o.xScale || 1)))
            .force('y', D3.forceY().strength((d) => (d.degree ? o.pull : o.pull * 3)))
            .force('collide', D3.forceCollide((d) => radius(d) + 8))
            .stop();
        const ticks = Math.ceil(Math.log(sim.alphaMin()) / Math.log(1 - sim.alphaDecay()));
        const t0 = performance.now();
        for (let i = 0; i < ticks && performance.now() - t0 < 1500; i++) sim.tick();
        return sim;
    }

    function bbox(nodes) {
        const b = { x0: Infinity, y0: Infinity, x1: -Infinity, y1: -Infinity };
        nodes.forEach((d) => {
            b.x0 = Math.min(b.x0, d.x);
            b.y0 = Math.min(b.y0, d.y);
            b.x1 = Math.max(b.x1, d.x);
            b.y1 = Math.max(b.y1, d.y);
        });
        return b;
    }

    function truncate(s, n) {
        const c = Array.from(s);
        return c.length > n ? c.slice(0, n - 1).join('') + '…' : s;
    }

    // Draws the lines and nodes into svg. place() moves them to the nodes' current x and y.
    function draw(svg, nodes, pairs, o) {
        svg.textContent = '';
        const vp = svgEl('g');
        const eg = svgEl('g');
        const ng = svgEl('g');
        vp.append(eg, ng);
        svg.append(vp);
        const lines = pairs.map(([a, b]) => {
            const el = svgEl('line', 'edge');
            eg.append(el);
            return { el, a, b };
        });
        const nodeEls = new Map();
        nodes.forEach((d) => {
            const g = svgEl('g', 'node' + (d.missing ? ' missing' : '') + (d.id === o.currentId ? ' current' : ''));
            g.style.setProperty('--c', colorOf(d));
            const r = radius(d) + (d.id === o.currentId ? 2 : 0);
            const title = svgEl('title');
            title.textContent = d.title;
            g.append(title);
            if (d.id === o.currentId) {
                const ring = svgEl('circle', 'ring');
                ring.setAttribute('r', (r + 4).toFixed(1));
                g.append(ring);
            }
            const c = svgEl('circle', 'dot');
            c.setAttribute('r', r.toFixed(1));
            const t = svgEl('text');
            t.setAttribute('y', (r + 13).toFixed(1));
            t.textContent = o.maxLabel ? truncate(d.title, o.maxLabel) : d.title;
            g.append(c, t);
            if (o.focusable) {
                g.setAttribute('tabindex', '-1'); // roving: the mount makes one node tabbable
                g.setAttribute('role', 'button');
                g.setAttribute('aria-label', d.title + (d.missing ? ' (not written yet)' : ''));
            }
            g._d = d;
            ng.append(g);
            nodeEls.set(d.id, g);
        });
        function place() {
            nodes.forEach((d) => nodeEls.get(d.id).setAttribute('transform', `translate(${d.x.toFixed(1)},${d.y.toFixed(1)})`));
            lines.forEach((l) => {
                l.el.setAttribute('x1', l.a.x.toFixed(1));
                l.el.setAttribute('y1', l.a.y.toFixed(1));
                l.el.setAttribute('x2', l.b.x.toFixed(1));
                l.el.setAttribute('y2', l.b.y.toFixed(1));
            });
        }
        place();
        return { vp, nodeEls, lines, place };
    }

    // Local graph: the page and its direct neighbours (both directions) with the links among them.
    // Resolves to false when the page has no neighbours, so the caller can hide the box.
    window.mountCodexLocalGraph = async function (svg, data, currentId) {
        const g = prepare(data);
        const around = g.nb.get(currentId);
        if (!around || around.size === 0) return false;
        const D3 = await loadD3();
        const keep = [...around].map((id) => g.byId.get(id)).sort((a, b) => b.degree - a.degree || (a.title < b.title ? -1 : 1)).slice(0, 30);
        const ids = new Set([currentId, ...keep.map((d) => d.id)]);
        const nodes = [g.byId.get(currentId), ...keep];
        const pairs = g.pairs.filter(([a, b]) => ids.has(a.id) && ids.has(b.id));
        nodes.forEach((d) => {
            d.x = undefined;
            d.y = undefined;
        });
        layout(D3, nodes, pairs, { distance: 85, charge: -240, pull: 0.08 });
        draw(svg, nodes, pairs, { currentId, maxLabel: 16 });
        const b = bbox(nodes);
        svg.setAttribute('viewBox', [b.x0 - 60, b.y0 - 26, b.x1 - b.x0 + 120, b.y1 - b.y0 + 66].map(Math.round).join(' '));
        return true;
    };

    // The full graph. root holds the stage, the filters and the card (see codex.html).
    // opts: { data, currentId, canEdit, pageUrl(node), onCreate(title) }
    window.mountCodexGraph = async function (root, opts) {
        const D3 = await loadD3();
        const g = prepare(opts.data);
        const $ = (sel) => root.querySelector(sel);
        const svg = $('#gvSvg');
        const card = $('#gvCard');
        const find = $('#gvFind');
        const tagSel = $('#gvTag');
        const showMissing = $('#gvMissing');
        const showOrphans = $('#gvOrphans');
        const showLabels = $('#gvLabels');
        const settings = $('#gvSettings');
        const noMatch = $('#gvNoMatch');
        if (window.matchMedia('(max-width: 900px)').matches) settings.open = false;
        showLabels.checked = g.nodes.length <= 60;

        // Legend: one entry per top-level folder that holds a page
        const legend = $('#gvLegend');
        legend.textContent = '';
        const entry = (label, style, cls) => {
            const s = document.createElement('span');
            const i = document.createElement('i');
            if (cls) i.className = cls;
            if (style) i.style.setProperty('--c', style);
            s.append(i, label);
            legend.append(s);
        };
        (opts.data.top_folders || []).forEach((f, i) => {
            if (g.nodes.some((d) => d.top_folder && d.top_folder.id === f.id)) entry(f.name, `var(--g-${i % PALETTE})`);
        });
        if (g.nodes.some((d) => !d.missing && !d.top_folder)) entry('Top level', 'var(--g-top)');
        if (g.nodes.some((d) => d.missing)) entry('Not written yet', '', 'ghost');

        const tags = [...new Set(g.nodes.flatMap((d) => d.tags))].sort((a, b) => a.localeCompare(b));
        tagSel.querySelectorAll('option:not([value=""])').forEach((o) => o.remove());
        tags.forEach((t) => tagSel.append(new Option(t, t)));

        nodesInit(g.nodes);
        // A second mount (the screen reloaded) replaces the first: stop its simulation and listeners.
        if (root._gv) root._gv.abort();
        if (root._gvSim) root._gvSim.stop();
        const ac = new AbortController();
        root._gv = ac;
        const on = (el, type, fn, opt) => el.addEventListener(type, fn, { ...opt, signal: ac.signal });
        const sim = layout(D3, g.nodes, g.pairs, { distance: 110, charge: -450, pull: 0.03, xScale: Math.min(1, Math.max(0.25, svg.clientHeight / (svg.clientWidth || 1))) });
        root._gvSim = sim;
        const dr = draw(svg, g.nodes, g.pairs, { currentId: opts.currentId, focusable: true });
        const view = { k: 1, tx: 0, ty: 0 };
        let selected = null;
        const applyView = () => {
            dr.vp.setAttribute('transform', `translate(${view.tx},${view.ty}) scale(${view.k})`);
            svg.style.setProperty('--k', view.k.toFixed(3)); // labels keep their size on screen
            svg.classList.toggle('zoomed', view.k >= 1.2);
        };
        const hiddenOf = (id) => dr.nodeEls.get(id).classList.contains('is-hidden');
        const visibleNodes = () => g.nodes.filter((d) => !hiddenOf(d.id));
        function fit() {
            const w = svg.clientWidth;
            const h = svg.clientHeight;
            const vis = visibleNodes();
            if (!w || !h || !vis.length) return;
            const b = bbox(vis);
            const pad = w < 600 ? 44 : 80;
            b.y1 += 18; // room for the titles under the lowest nodes
            const bw = Math.max(b.x1 - b.x0, 1);
            const bh = Math.max(b.y1 - b.y0, 1);
            view.k = Math.max(0.2, Math.min((w - pad * 2) / bw, (h - pad * 2) / bh, 1.6));
            view.tx = w / 2 - (b.x0 + bw / 2) * view.k;
            view.ty = h / 2 - (b.y0 + bh / 2) * view.k;
            applyView();
        }
        function zoomAt(f, cx, cy) {
            const k2 = Math.max(0.2, Math.min(4, view.k * f));
            f = k2 / view.k;
            view.tx = cx - (cx - view.tx) * f;
            view.ty = cy - (cy - view.ty) * f;
            view.k = k2;
            applyView();
        }
        function centerOn(d) {
            view.tx = svg.clientWidth / 2 - d.x * view.k;
            view.ty = svg.clientHeight / 2 - d.y * view.k;
            applyView();
        }
        function hover(id) {
            svg.classList.toggle('hovering', !!id);
            const around = id ? g.nb.get(id) : null;
            g.nodes.forEach((d) => dr.nodeEls.get(d.id).classList.toggle('hl', !!id && (d.id === id || around.has(d.id))));
            dr.lines.forEach((l) => l.el.classList.toggle('hl', !!id && (l.a.id === id || l.b.id === id)));
        }
        function select(id) {
            if (selected) dr.nodeEls.get(selected).classList.remove('sel');
            selected = id;
            if (!id) {
                card.hidden = true;
                return;
            }
            dr.nodeEls.get(id).classList.add('sel');
            const d = g.byId.get(id);
            $('#gvCardTitle').textContent = d.title;
            $('#gvCardWhere').textContent = d.missing ? `Not written yet · linked from ${plural(d.inn, 'page')}` : d.folder_path.length ? d.folder_path.map((f) => f.name).join(' › ') : 'Top level';
            const tagBox = $('#gvCardTags');
            tagBox.textContent = '';
            d.tags.forEach((t) => {
                const c = document.createElement('span');
                c.className = 'cx-chip cx-chip--tag';
                c.textContent = t;
                tagBox.append(c, ' ');
            });
            $('#gvCardText').textContent = d.missing ? 'Nobody has written this page yet.' : d.excerpt;
            $('#gvCardCount').textContent = d.missing ? '' : `Links to ${d.out} · linked from ${d.inn}`;
            const open = $('#gvCardOpen');
            const create = $('#gvCardCreate');
            open.hidden = d.missing;
            if (!d.missing) open.href = opts.pageUrl(d);
            create.hidden = !(d.missing && opts.canEdit);
            create.dataset.title = d.title;
            card.hidden = false;
            card.scrollIntoView({ block: 'nearest' });
            setTab(dr.nodeEls.get(id));
        }
        // Roving tabindex: one node is in the tab order; arrow keys move between nodes.
        let tabbed = null;
        function setTab(el) {
            if (tabbed) tabbed.setAttribute('tabindex', '-1');
            tabbed = el;
            if (el) el.setAttribute('tabindex', '0');
        }
        function applyFilters() {
            const q = find.value.trim().toLowerCase();
            const tag = tagSel.value;
            const filtering = !!(q || tag);
            svg.classList.toggle('nolabels', !showLabels.checked);
            g.nodes.forEach((d) => {
                const e = dr.nodeEls.get(d.id);
                const hide = (d.missing && !showMissing.checked) || (!d.missing && d.degree === 0 && !showOrphans.checked);
                e.classList.toggle('is-hidden', hide);
                e.classList.toggle('match', filtering && (!q || d.title.toLowerCase().includes(q)) && (!tag || d.tags.includes(tag)));
            });
            dr.lines.forEach((l) => l.el.classList.toggle('is-hidden', hiddenOf(l.a.id) || hiddenOf(l.b.id)));
            const hits = filtering ? g.nodes.filter((d) => dr.nodeEls.get(d.id).classList.contains('match') && !hiddenOf(d.id)).length : 0;
            // With nothing to match, say so instead of fading every node.
            noMatch.hidden = !filtering || hits > 0;
            svg.classList.toggle('filtering', filtering && hits > 0);
            if (selected && hiddenOf(selected)) select(null);
            if (!tabbed || hiddenOf(tabbed._d.id)) setTab(dr.nodeEls.get((visibleNodes()[0] || g.nodes[0]).id));
        }
        [find, tagSel, showMissing, showOrphans, showLabels].forEach((c) => {
            on(c, 'input', applyFilters);
            on(c, 'change', applyFilters);
        });
        on(find, 'keydown', (e) => {
            if (e.key !== 'Enter') return;
            e.preventDefault();
            const q = find.value.trim().toLowerCase();
            const hits = g.nodes.filter((d) => dr.nodeEls.get(d.id).classList.contains('match') && !hiddenOf(d.id));
            const m = hits.find((d) => d.title.toLowerCase() === q) || hits[0];
            if (m) {
                select(m.id);
                centerOn(m);
            }
        });

        // Pan, pinch-zoom and drag a node (pointer events, so touch works too)
        const pointers = new Map();
        let drag = null;
        const nodeAt = (e) => (e.target.closest ? e.target.closest('.node') : null);
        on(svg, 'pointerdown', (e) => {
            svg.setPointerCapture(e.pointerId);
            pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
            if (pointers.size === 2) {
                const [p, q] = [...pointers.values()];
                drag = { pinch: true, d0: Math.hypot(p.x - q.x, p.y - q.y) || 1, k0: view.k };
                return;
            }
            const n = nodeAt(e);
            drag = { node: n ? n._d : null, lx: e.clientX, ly: e.clientY, moved: 0 };
            if (!n) svg.classList.add('panning');
        });
        on(svg, 'pointermove', (e) => {
            if (!pointers.has(e.pointerId)) return;
            pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
            if (!drag) return;
            if (drag.pinch) {
                if (pointers.size < 2) return;
                const [p, q] = [...pointers.values()];
                const r = svg.getBoundingClientRect();
                const d = Math.hypot(p.x - q.x, p.y - q.y);
                zoomAt((drag.k0 * d) / drag.d0 / view.k, (p.x + q.x) / 2 - r.left, (p.y + q.y) / 2 - r.top);
                return;
            }
            const dx = e.clientX - drag.lx;
            const dy = e.clientY - drag.ly;
            drag.lx = e.clientX;
            drag.ly = e.clientY;
            drag.moved += Math.abs(dx) + Math.abs(dy);
            if (drag.node) {
                if (drag.moved < 5) {
                    drag.lx -= dx; // not a drag yet: keep the distance so the page doesn't lag behind the pointer
                    drag.ly -= dy;
                    return;
                }
                const n = drag.node;
                n.fx = (n.fx === undefined || n.fx === null ? n.x : n.fx) + dx / view.k;
                n.fy = (n.fy === undefined || n.fy === null ? n.y : n.fy) + dy / view.k;
                sim.alphaTarget(0.15).restart(); // gently re-heat so its neighbours follow
            } else {
                view.tx += dx;
                view.ty += dy;
                applyView();
            }
        });
        function up(e) {
            pointers.delete(e.pointerId);
            if (drag && !drag.pinch && drag.moved < 5) select(drag.node ? drag.node.id : null);
            if (drag && drag.node) sim.alphaTarget(0); // the dropped page stays where it was put
            if (!drag || drag.pinch || pointers.size === 0) drag = null;
            svg.classList.remove('panning');
        }
        on(svg, 'pointerup', up);
        on(svg, 'pointercancel', up);
        on(svg, 'wheel', (e) => {
            e.preventDefault();
            const r = svg.getBoundingClientRect();
            zoomAt(Math.exp(-e.deltaY * 0.0015), e.clientX - r.left, e.clientY - r.top);
        }, { passive: false });
        on(svg, 'pointerover', (e) => {
            if (e.pointerType !== 'mouse' || drag) return;
            const n = nodeAt(e);
            if (n) hover(n._d.id);
        });
        on(svg, 'pointerout', (e) => {
            const n = nodeAt(e);
            if (n && !(e.relatedTarget && n.contains(e.relatedTarget))) hover(null);
        });
        on(svg, 'focusin', (e) => {
            const n = nodeAt(e);
            if (n) {
                setTab(n);
                hover(n._d.id);
            }
        });
        on(svg, 'focusout', () => hover(null));
        on(svg, 'keydown', (e) => {
            const n = nodeAt(e);
            if (!n) return;
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                select(n._d.id);
                return;
            }
            const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
            if (!step) return;
            e.preventDefault();
            const vis = visibleNodes();
            const next = vis[(vis.indexOf(n._d) + step + vis.length) % vis.length];
            if (next) dr.nodeEls.get(next.id).focus();
        });
        sim.on('tick', dr.place);
        on($('#gvIn'), 'click', () => zoomAt(1.3, svg.clientWidth / 2, svg.clientHeight / 2));
        on($('#gvOut'), 'click', () => zoomAt(1 / 1.3, svg.clientWidth / 2, svg.clientHeight / 2));
        on($('#gvFit'), 'click', fit);
        on($('#gvCardClose'), 'click', () => select(null));
        on($('#gvCardCreate'), 'click', (e) => {
            const title = e.currentTarget.dataset.title;
            select(null);
            opts.onCreate(title);
        });
        on(document, 'keydown', (e) => {
            if (e.key === 'Escape' && root.isConnected && !card.hidden) select(null);
        });

        applyFilters();
        applyView();
        fit();
        if (!svg.clientWidth) requestAnimationFrame(fit);
        return { fit, select, count: g.nodes.length };
    };

    // forceSimulation seeds x and y itself (a spiral) when they are missing.
    function nodesInit(nodes) {
        nodes.forEach((d) => {
            d.x = undefined;
            d.y = undefined;
            d.fx = null;
            d.fy = null;
        });
    }
})();
