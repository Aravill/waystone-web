// Theme toggle (see STYLE_GUIDE.md, Colour). Loaded non-deferred in <head> so the saved theme
// applies before first paint; the header button is added once the DOM is ready.
(function () {
    const KEY = 'waystone-theme';
    const root = document.documentElement;

    try {
        const saved = localStorage.getItem(KEY);
        if (saved === 'light' || saved === 'dark') root.dataset.theme = saved;
    } catch (e) { /* storage unavailable: follow the system theme */ }

    const isDark = () => root.dataset.theme
        ? root.dataset.theme === 'dark'
        : window.matchMedia('(prefers-color-scheme: dark)').matches;

    const SUN = '<circle cx="12" cy="12" r="4"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M4.9 19.1L7 17M17 7l2.1-2.1"/>';
    const MOON = '<path d="M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5z"/>';

    document.addEventListener('DOMContentLoaded', () => {
        const header = document.querySelector('.header-right');
        if (!header) return;

        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'theme-toggle';

        const render = () => {
            const dark = isDark();
            // Show the theme you would switch to: sun while dark, moon while light.
            btn.innerHTML = '<svg viewBox="0 0 24 24" width="1.25rem" height="1.25rem" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">' + (dark ? SUN : MOON) + '</svg>';
            btn.setAttribute('aria-label', dark ? 'Switch to light theme' : 'Switch to dark theme');
        };

        btn.addEventListener('click', () => {
            const next = isDark() ? 'light' : 'dark';
            root.dataset.theme = next;
            try { localStorage.setItem(KEY, next); } catch (e) { /* not persisted */ }
            render();
        });

        render();
        header.prepend(btn);
    });
})();
