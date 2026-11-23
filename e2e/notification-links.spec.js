// Notification links: every notification opens the place of its event, marks itself read however it
// is opened, and says so when the event is gone or not visible.
const base = require('./helpers');
const { expect, loginAs, apiAs, dateFromToday } = base;
const env = require('./env');

/** testy runs a game, roy plays in it and adam follows it. The game is deleted at teardown. */
const test = base.test.extend({
  world: async ({}, use) => {
    const api = { testy: await apiAs('testy'), roy: await apiAs('roy'), adam: await apiAs('adam'), admin: await apiAs(env.ADMIN_USERNAME) };
    let id = '';
    try {
      const royId = (await (await api.roy.get('/auth/current-user')).json()).user_id;
      const res = await api.testy.post('/api/campaigns', {
        data: { title: `Links E2E ${Date.now()}`, summary: 'A game for the links test', description: 'Test game', type: 'Campaign', system: '' },
      });
      id = (await res.json()).campaign_id;
      expect((await api.roy.post(`/api/campaigns/${id}/signups`)).status()).toBe(201);
      expect((await api.testy.put(`/api/campaigns/${id}/signups/${royId}`, { data: { status: 'Accepted' } })).status()).toBe(200);
      expect((await api.adam.put(`/api/campaigns/${id}/follow`)).ok()).toBe(true);
      await use({
        id,
        api,
        /** The user's newest notification of this game (of a type). */
        async latest(user, type) {
          const { notifications } = await (await api[user].get('/api/notifications?limit=50')).json();
          const n = notifications.find((x) => x.campaign_id === id && x.type === type);
          expect(n, `${user} has ${type}`).toBeTruthy();
          return n;
        },
        async suggest(...days) {
          const dates = days.map((d) => ({ date: dateFromToday(d), time: '19:00', duration: 120 }));
          const r = await api.testy.post(`/api/campaigns/${id}/polls`, { data: { timezone: 'Europe/Prague', dates } });
          expect(r.status(), await r.text()).toBe(201);
          return r.json();
        },
        setStatus: (sid, status) => api.testy.put(`/api/campaigns/${id}/sessions/${sid}`, { data: { status } }),
      });
    } finally {
      if (id) expect((await api.admin.delete(`/api/campaigns/${id}`)).ok(), 'delete game').toBe(true);
      await Promise.all(Object.values(api).map((a) => a.dispose()));
    }
  },
});

const heading = (ymd) => new Date(ymd + 'T12:00:00').toLocaleDateString('en-US', { weekday: 'long', month: 'short', day: 'numeric' });

test('a poll opens on the calendar from the inbox, in a new tab too, and is marked read', async ({ page, context, world }) => {
  const poll = await world.suggest(62, 65);
  const n = await world.latest('roy', 'session_suggested');
  await loginAs(page, 'roy');
  await page.goto('/characters');
  await page.getByRole('button', { name: 'Inbox' }).click();
  const row = page.locator('a.notif-item', { hasText: 'suggested 2 dates for "Links E2E' }).first();
  await expect(row).toHaveAttribute('href', n.url);

  // Ctrl+click opens a tab and leaves this page alone.
  const [tab] = await Promise.all([context.waitForEvent('page'), row.click({ modifiers: ['ControlOrMeta'] })]);
  await expect(tab.locator('.calendar-day.is-selected')).toBeVisible();
  await expect(tab.locator('.calendar-panel .panel-heading')).toHaveText(new RegExp(heading(dateFromToday(62))));
  await expect(tab.locator('.poll-card.is-poll .panel-session')).toHaveCount(2);
  expect(new URL(tab.url()).searchParams.has('notif')).toBe(false);
  expect(page.url()).toContain('/characters');
  await expect.poll(async () => (await world.latest('roy', 'session_suggested')).read_at).not.toBe('');

  // Settled on another date: the old link opens the confirmed session.
  expect((await world.setStatus(poll.sessions[1].id, 'Confirmed')).ok()).toBe(true);
  await page.goto(n.url);
  await expect(page.locator('.calendar-panel .panel-heading')).toHaveText(new RegExp(heading(dateFromToday(65))));
  await expect(page.locator('.panel-status-tag')).toHaveText('Confirmed');

  // Cancelled: the same day, shown as cancelled.
  expect((await world.setStatus(poll.sessions[1].id, 'Cancelled')).ok()).toBe(true);
  await page.goto((await world.latest('roy', 'session_cancelled')).url);
  await expect(page.locator('.panel-status-tag')).toHaveText('Cancelled');

  // A withdrawn poll is gone, and the calendar says so.
  const gone = await world.suggest(70, 72);
  expect((await world.api.testy.delete(`/api/campaigns/${world.id}/polls/${gone.poll_id}`)).ok()).toBe(true);
  await page.goto((await world.latest('roy', 'session_cancelled')).url);
  await expect(page.locator('.message')).toHaveText('This poll is no longer open.');
});

test('a follower lands on the game page, with the session or the reason', async ({ page, world }) => {
  const poll = await world.suggest(62, 65);
  expect((await world.setStatus(poll.sessions[0].id, 'Confirmed')).ok()).toBe(true);
  const other = await world.suggest(70, 72);

  await loginAs(page, 'adam');
  await page.goto((await world.latest('adam', 'session_confirmed')).url);
  await expect(page).toHaveURL(new RegExp(`/games/${world.id}$`));
  await expect(page.locator('#gamePanel .panel-heading')).toHaveText(new RegExp(heading(dateFromToday(62))));
  await expect.poll(async () => (await world.latest('adam', 'session_confirmed')).read_at).not.toBe('');

  // A poll from a Discord channel: only its players can see it.
  await page.goto(`/games/${world.id}/calendar?poll=${other.poll_id}`);
  await expect(page.locator('.message')).toHaveText("Only the game's DM and players can see its polls.");

  expect((await world.setStatus(poll.sessions[0].id, 'Cancelled')).ok()).toBe(true);
  await page.goto((await world.latest('adam', 'session_cancelled')).url);
  await expect(page.locator('.message')).toContainText("That session isn't shown here.");
});

test('a logged-out click comes back after the login; sign-ups open their dialog', async ({ page, world }) => {
  await page.goto((await world.latest('testy', 'signup_created')).url);
  await expect(page).toHaveURL(/login\.html\?next=/);
  await page.locator('input[name="username"], #username').first().fill('testy');
  await page.locator('input[type="password"]').first().fill(env.DEMO_PASSWORD);
  await page.keyboard.press('Enter');
  await expect(page.locator('#signupsModal')).toHaveClass(/modal-open/);
  await expect.poll(async () => (await world.latest('testy', 'signup_created')).read_at).not.toBe('');

  // The player's acceptance opens the game page.
  await loginAs(page, 'roy');
  await page.goto((await world.latest('roy', 'signup_accepted')).url);
  await expect(page).toHaveURL(new RegExp(`/games/${world.id}$`));
  await expect(page.locator('h1.game-title')).toBeVisible();
});
