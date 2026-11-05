// Sends an expired session to login, returning to this page afterwards. Not for logout.
window.redirectToLogin = function () {
    const here = location.pathname + location.search + location.hash;
    location.href = here === '/' ? '/login.html' : '/login.html?next=' + encodeURIComponent(here);
};
