// The Codex page editor: CodeMirror 6 with Markdown highlighting (headings bigger, bold and italic
// styled in place, the markup stays visible), [[wikilinks]] marked, and title completion after "[[".
// The library (static/vendor/codemirror.bundle.js, global CM) loads on first use.
(function () {
    const loading = {};
    function loadScriptOnce(src) {
        if (!loading[src]) {
            loading[src] = new Promise((resolve, reject) => {
                const s = document.createElement('script');
                s.src = src;
                s.onload = resolve;
                s.onerror = () => reject(new Error('Could not load ' + src));
                document.head.appendChild(s);
            });
        }
        return loading[src];
    }

    // [[Title]] and [[Title|label]] (mirrors wikiLinkRe in db/codex_links.go) and ![[picture]] or
    // ![[picture|caption]], where the caption may be empty.
    const LINK_RE = /!?\[\[([^\[\]\n|]{1,200})(?:\|([^\[\]\n]{0,200}))?\]\]/g;
    const norm = (s) => String(s || '').trim().split(/\s+/).join(' ').toLowerCase();

    // createCodexEditor(el, { doc, titles, onChange }) mounts the editor in el and resolves to
    // { view, linkSelection }. doc is the text, or a function returning it (read after the library has loaded, so
    // text typed meanwhile in the fallback box is kept). titles() returns [{ title, aliases }] for this codex; onChange(text) gets every edit;
    // onNotice(text) hears why Ctrl+K did nothing.
    window.createCodexEditor = async function (el, { doc, titles, onChange, onNotice }) {
        await loadScriptOnce('/static/vendor/codemirror.bundle.js?v=1');
        const { EditorView, EditorState, keymap, Decoration, ViewPlugin, MatchDecorator, placeholder, drawSelection,
            history, defaultKeymap, historyKeymap, markdown, autocompletion, completionKeymap,
            HighlightStyle, syntaxHighlighting, tags: t } = window.CM;

        const highlight = HighlightStyle.define([
            { tag: t.heading1, fontFamily: 'var(--font-display)', fontSize: '1.7em', lineHeight: '1.3' },
            { tag: t.heading2, fontFamily: 'var(--font-display)', fontSize: '1.45em', lineHeight: '1.3' },
            { tag: [t.heading3, t.heading4, t.heading5, t.heading6], fontFamily: 'var(--font-display)', fontSize: '1.2em' },
            { tag: t.strong, fontWeight: '600' },
            { tag: t.emphasis, fontStyle: 'italic' },
            { tag: t.link, color: 'var(--rubric)' },
            { tag: t.url, color: 'var(--muted)' },
            { tag: t.quote, color: 'var(--muted)', fontStyle: 'italic' },
            { tag: t.monospace, background: 'var(--folio)' },
            { tag: t.processingInstruction, color: 'var(--muted)' }
        ]);

        const known = () => {
            const names = new Set();
            for (const p of titles()) for (const n of [p.title, ...(p.aliases || [])]) names.add(norm(n));
            return names;
        };
        const decorator = new MatchDecorator({
            regexp: LINK_RE,
            decoration: (m) => {
                if (m[0][0] === '!') return Decoration.mark({ class: 'cm-imgref' });
                if (m[2] === '') return null; // [[Title|]] has no label, so it is no link
                return Decoration.mark({ class: 'cm-wikilink' + (known().has(norm(m[1])) ? '' : ' cm-wikilink--missing') });
            }
        });
        const wikilinks = ViewPlugin.fromClass(class {
            constructor(view) { this.decorations = decorator.createDeco(view); }
            update(u) { this.decorations = decorator.updateDeco(u, this.decorations); }
        }, { decorations: (v) => v.decorations });

        // After "[[": titles insert "Title]]", aliases insert "Title|alias]]".
        const complete = (ctx) => {
            const m = ctx.matchBefore(/\[\[[^\[\]\n|]*/);
            if (!m || (m.from > 0 && ctx.state.doc.sliceString(m.from - 1, m.from) === '!')) return null;
            const options = [];
            for (const p of titles()) {
                options.push({ label: p.title, detail: p.kind === 'map' ? 'map' : '', apply: p.title + ']]' });
                for (const a of p.aliases || []) options.push({ label: a, detail: '→ ' + p.title, apply: p.title + '|' + a + ']]' });
            }
            return { from: m.from + 2, options, validFor: /^[^\[\]\n|]*$/ };
        };

        // Ctrl/Cmd+K and the Link button: the selected words become [[Title]], [[Title|as selected]]
        // (another case, or an alias) or [[as selected]] (a page not written yet). Always true, so the
        // browser's own Ctrl+K stays out of it.
        const linkSelection = (v) => {
            const sel = v.state.selection.main;
            const raw = v.state.doc.sliceString(sel.from, sel.to);
            const text = raw.trim();
            if (!text) {
                if (onNotice) onNotice('Select the words to link first.');
                return true;
            }
            if (/[\n\[\]|]/.test(text) || text.length > 120) {
                if (onNotice) onNotice('Select up to 120 characters on one line, without [ ] or |.');
                return true;
            }
            const from = sel.from + raw.indexOf(text);
            const key = norm(text);
            const all = titles();
            const hit = all.find((p) => norm(p.title) === key) || all.find((p) => (p.aliases || []).some((a) => norm(a) === key));
            const link = !hit ? `[[${text}]]` : hit.title === text ? `[[${hit.title}]]` : `[[${hit.title}|${text}]]`;
            v.dispatch({ changes: { from, to: from + text.length, insert: link }, selection: { anchor: from + link.length }, scrollIntoView: true });
            v.focus();
            return true;
        };

        const view = new EditorView({
            parent: el,
            state: EditorState.create({
                doc: typeof doc === 'function' ? doc() : doc,
                extensions: [
                    history(),
                    drawSelection(),
                    markdown(),
                    syntaxHighlighting(highlight),
                    wikilinks,
                    autocompletion({ override: [complete], icons: false }),
                    placeholder('Write about it. Link other pages with [[Title]].'),
                    EditorView.lineWrapping,
                    EditorView.contentAttributes.of({ 'aria-label': 'The page', 'aria-multiline': 'true' }),
                    keymap.of([{ key: 'Mod-k', run: (v) => linkSelection(v) }, ...completionKeymap, ...historyKeymap, ...defaultKeymap]),
                    EditorView.updateListener.of((u) => { if (u.docChanged) onChange(u.state.doc.toString()); })
                ]
            })
        });
        return { view, linkSelection: () => linkSelection(view) };
    };
})();
