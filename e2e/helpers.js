// @ts-check
const base = require('@playwright/test');
const { expect, request } = base;
const env = require('./env');

const BASE_URL = `http://localhost:${env.PORT}`;
const GAME = 'Age of Sojourn';

function passwordFor(user) {
  return user === env.ADMIN_USERNAME ? env.ADMIN_PASSWORD : env.DEMO_PASSWORD;
}

/** Log the browser context in without the UI (the cookie lands in the context). */
async function loginAs(page, user) {
  const res = await page.request.post('/auth/login', {
    data: { username: user, password: passwordFor(user) },
  });
  expect(res.ok(), `login as ${user}`).toBeTruthy();
}

/** An API client logged in as `user`. Dispose it when done. */
async function apiAs(user) {
  const api = await request.newContext({ baseURL: BASE_URL });
  const res = await api.post('/auth/login', {
    data: { username: user, password: passwordFor(user) },
  });
  expect(res.ok(), `API login as ${user}`).toBeTruthy();
  return api;
}

async function sojournId(api) {
  const res = await api.get('/api/campaigns');
  expect(res.ok()).toBeTruthy();
  const game = (await res.json()).find((c) => c.title === GAME);
  expect(game, `${GAME} is seeded`).toBeTruthy();
  return game.id;
}

async function createSession(admin, campaignId, { date, time, duration = 240 }) {
  const res = await admin.post(`/api/campaigns/${campaignId}/sessions`, {
    data: { date, time, duration, timezone: 'Europe/Prague' },
  });
  expect(res.status(), 'create session').toBe(201);
  return (await res.json()).session;
}

async function reply(api, campaignId, sessionId, participation) {
  const res = await api.post(`/api/campaigns/${campaignId}/sessions/${sessionId}/responses`, {
    data: { participation },
  });
  expect(res.status(), 'reply').toBe(201);
}

async function deleteSession(admin, campaignId, sessionId) {
  await admin.delete(`/api/campaigns/${campaignId}/sessions/${sessionId}`);
}

/** YYYY-MM-DD, n days from today in local time (the server uses its local clock too). */
function dateFromToday(n) {
  const d = new Date();
  d.setDate(d.getDate() + n);
  const p = (v) => String(v).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/**
 * Open the Games page calendar for Age of Sojourn. With unanswered suggestions the
 * button carries a focus date, so the calendar opens on that month with that day selected.
 */
async function openCalendar(page) {
  await page.goto('/campaigns');
  await page
    .getByRole('article')
    .filter({ hasText: GAME })
    .getByRole('button', { name: /^Calendar/ })
    .click();
  const modal = page.locator('#calendarModal');
  // Wait for the focus load, so a later "Next month" can't race with it.
  await expect(modal.locator('.calendar-day.is-selected')).toBeVisible();
  return modal;
}

/** Click "Next month" until the calendar shows the month of `ymd`. */
async function goToMonth(page, ymd) {
  const [y, m] = ymd.split('-').map(Number);
  const label = new Date(y, m - 1, 1).toLocaleString('en-US', { month: 'long', year: 'numeric' });
  const modal = page.locator('#calendarModal');
  const current = modal.locator('.current-month');
  for (let i = 0; i < 6 && (await current.textContent()) !== label; i++) {
    // Wait for each month's load: loadSessions has no guard against a stale response.
    const loaded = page.waitForResponse(/\/sessions\?month=/);
    await modal.getByRole('button', { name: 'Next month' }).click();
    await loaded;
  }
  await expect(current).toHaveText(label);
}

/** Test with a `game` fixture that removes every session it created at teardown. */
const test = base.test.extend({
  game: async ({}, use) => {
    const admin = await apiAs(env.ADMIN_USERNAME);
    const id = await sojournId(admin);
    const created = [];
    const players = [];
    await use({
      id,
      async create(opts) {
        const s = await createSession(admin, id, opts);
        created.push(s.id);
        return s;
      },
      async replyAs(user, session, participation) {
        const api = await apiAs(user);
        players.push(api);
        await reply(api, id, session.id, participation);
      },
    });
    for (const sid of created) await deleteSession(admin, id, sid);
    await Promise.all([admin, ...players].map((a) => a.dispose()));
  },
});

module.exports = { test, expect, loginAs, apiAs, sojournId, createSession, reply, deleteSession, dateFromToday, openCalendar, goToMonth };
