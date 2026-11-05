// @ts-check
const { request } = require('@playwright/test');
const helpers = require('./helpers');
const env = require('./env');

const { expect, apiAs, loginAs, createSession, deleteSession, dateFromToday } = helpers;

// Thorns has no seeded players, so these players can't skew the Age of Sojourn calendar tests.
const GAME = 'Thorns of Anevion';
const GAME_DM = 'mira';
const PASSWORD = 'Sturdy-Pass-42';
const BASE_URL = `http://localhost:${env.PORT}`;

let playerCount = 0;

/** An API client with no session, i.e. someone who only has the link. */
const anonymous = () => request.newContext({ baseURL: BASE_URL });

const test = helpers.test.extend({
  // The game, plus new players (deleted at teardown) who haven't set a password.
  thorns: async ({}, use) => {
    const admin = await apiAs(env.ADMIN_USERNAME);
    const games = await (await admin.get('/api/campaigns')).json();
    const id = games.find((c) => c.title === GAME).id;
    const userIds = [];
    await use({
      id,
      admin,
      async newPlayer() {
        const username = `newplayer${++playerCount}`;
        const res = await admin.post('/api/admin/users', { data: { username, name: `Player ${playerCount}` } });
        expect(res.status(), 'create user').toBe(201);
        const { user, temp_password } = await res.json();
        userIds.push(user.id);
        const added = await admin.post(`/api/admin/campaigns/${id}/players`, { data: { user_id: user.id } });
        expect(added.ok(), 'add player').toBeTruthy();
        return { id: user.id, username, name: `Player ${playerCount}`, tempPassword: temp_password };
      },
      generateLink: (userId) => admin.post(`/api/campaigns/${id}/players/${userId}/guest-link`, { data: {} }),
      async link(userId) {
        const res = await admin.post(`/api/campaigns/${id}/players/${userId}/guest-link`, { data: {} });
        expect(res.ok(), 'generate link').toBeTruthy();
        return (await res.json()).token;
      },
    });
    for (const uid of userIds) await admin.delete(`/api/admin/users/${uid}`);
    await admin.dispose();
  },
});

async function setPasswordInForm(page, password, confirm = password) {
  await page.getByLabel('New password', { exact: true }).fill(password);
  await page.getByLabel('Confirm new password').fill(confirm);
  await page.getByRole('button', { name: 'Set password' }).click();
}

test('a new player sets a password from the link and replies on the calendar', async ({ page, thorns }) => {
  const player = await thorns.newPlayer();
  const token = await thorns.link(player.id);
  const dm = await apiAs(GAME_DM);
  const session = await createSession(dm, thorns.id, { date: dateFromToday(3), time: '18:00' });

  try {
    await page.goto(`/r/${token}`);
    await expect(page.getByText(`Your username is ${player.username}.`)).toBeVisible();
    await expect(page.getByText(GAME)).toBeVisible();
    await expect(page.getByLabel(/current/i)).toHaveCount(0);
    await setPasswordInForm(page, PASSWORD);

    const modal = page.locator('#calendarModal');
    await expect(modal).toBeVisible();
    await expect(modal).toContainText(GAME);
    const aye = modal.getByRole('button', { name: 'Aye' });
    await aye.click();
    await expect(aye).toHaveClass(/selected/);
  } finally {
    await deleteSession(dm, thorns.id, session.id);
    await dm.dispose();
  }

  const me = await (await page.request.get('/auth/current-user')).json();
  expect(me.username).toBe(player.username);
  expect(me.must_change_password).toBe(false);

  const fresh = await anonymous();
  expect((await fresh.post('/auth/login', { data: { username: player.username, password: player.tempPassword } })).status()).toBe(401);
  expect((await fresh.post('/auth/login', { data: { username: player.username, password: PASSWORD } })).ok()).toBeTruthy();
  await fresh.dispose();
});

test('a used link is dead and points to login', async ({ page, thorns }) => {
  const player = await thorns.newPlayer();
  const token = await thorns.link(player.id);

  const api = await anonymous();
  expect((await api.post(`/api/guest/${token}/password`, { data: { new_password: PASSWORD } })).ok()).toBeTruthy();
  expect((await api.post(`/api/guest/${token}/password`, { data: { new_password: 'Another-Pass-77' } })).status()).toBe(404);
  await api.dispose();

  await page.goto(`/r/${token}`);
  await expect(page.getByRole('heading', { name: 'Link not valid' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Go to login' })).toBeVisible();
});

test('changing the password the normal way also kills the link', async ({ thorns }) => {
  const player = await thorns.newPlayer();
  const token = await thorns.link(player.id);

  const api = await anonymous();
  expect((await api.post('/auth/login', { data: { username: player.username, password: player.tempPassword } })).ok()).toBeTruthy();
  const changed = await api.post('/auth/change-password', {
    data: { current_password: player.tempPassword, new_password: PASSWORD },
  });
  expect(changed.ok()).toBeTruthy();
  expect((await api.get(`/api/guest/${token}`)).status()).toBe(404);
  await api.dispose();
});

test('an admin reset kills the link until a new one is generated', async ({ thorns }) => {
  const player = await thorns.newPlayer();
  const token = await thorns.link(player.id);
  expect((await thorns.admin.post(`/api/admin/users/${player.id}/reset-password`)).ok()).toBeTruthy();

  const api = await anonymous();
  expect((await api.get(`/api/guest/${token}`)).status()).toBe(404);
  expect((await api.post(`/api/guest/${token}/password`, { data: { new_password: PASSWORD } })).status()).toBe(404);

  const fresh = await thorns.link(player.id);
  expect((await api.get(`/api/guest/${fresh}`)).ok()).toBeTruthy();
  await api.dispose();
});

test('rejected passwords leave the link usable', async ({ page, thorns }) => {
  const player = await thorns.newPlayer();
  const token = await thorns.link(player.id);

  await page.goto(`/r/${token}`);
  await setPasswordInForm(page, PASSWORD, `${PASSWORD}x`);
  await expect(page.getByText('Passwords do not match')).toBeVisible();

  const api = await anonymous();
  expect((await api.post(`/api/guest/${token}/password`, { data: { new_password: 'weak' } })).status()).toBe(400);
  expect((await api.post(`/api/guest/${token}/password`, { data: { new_password: player.tempPassword } })).status()).toBe(400);
  expect((await api.get(`/api/guest/${token}`)).ok()).toBeTruthy();
  await api.dispose();
});

test('the link replaces a session already in the browser', async ({ page, thorns }) => {
  const player = await thorns.newPlayer();
  const token = await thorns.link(player.id);
  await loginAs(page, 'testy');

  await page.goto(`/r/${token}`);
  await setPasswordInForm(page, PASSWORD);
  await expect(page.locator('#calendarModal')).toBeVisible();

  const me = await (await page.request.get('/auth/current-user')).json();
  expect(me.username).toBe(player.username);
});

test('links are refused for players with a password and for admins', async ({ thorns }) => {
  const player = await thorns.newPlayer();
  const token = await thorns.link(player.id);
  const api = await anonymous();
  expect((await api.post(`/api/guest/${token}/password`, { data: { new_password: PASSWORD } })).ok()).toBeTruthy();
  expect((await thorns.generateLink(player.id)).status()).toBe(409);

  const promoted = await thorns.newPlayer();
  const promotedToken = await thorns.link(promoted.id);
  expect((await thorns.admin.post(`/api/admin/users/${promoted.id}/make-admin`)).ok()).toBeTruthy();
  expect((await api.get(`/api/guest/${promotedToken}`)).status()).toBe(404);
  expect((await api.post(`/api/guest/${promotedToken}/password`, { data: { new_password: PASSWORD } })).status()).toBe(404);
  expect((await thorns.generateLink(promoted.id)).status()).toBe(409);
  await api.dispose();
});

test('the DM panel offers links only to players who need setup', async ({ page, thorns }) => {
  const player = await thorns.newPlayer();
  await loginAs(page, GAME_DM);

  const openPanel = async () => {
    await page.goto(`/campaigns?calendar=${encodeURIComponent(thorns.id)}`);
    const row = page.locator('#calendarModal .guest-link-row').filter({ hasText: player.name });
    await expect(row).toBeVisible();
    return row;
  };

  let row = await openPanel();
  await row.getByRole('button', { name: 'Generate' }).click();
  await expect(row.getByRole('button', { name: 'Copy' })).toBeVisible();

  const dm = await apiAs(GAME_DM);
  const { links, needs_setup } = await (await dm.get(`/api/campaigns/${thorns.id}/guest-links`)).json();
  await dm.dispose();
  expect(needs_setup).toContain(player.id);

  const api = await anonymous();
  expect((await api.post(`/api/guest/${links[player.id].token}/password`, { data: { new_password: PASSWORD } })).ok()).toBeTruthy();
  await api.dispose();

  row = await openPanel();
  await expect(row).toContainText('no link needed');
  await expect(row.getByRole('button')).toHaveCount(0);
});
