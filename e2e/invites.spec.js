// @ts-check
const { request } = require('@playwright/test');
const helpers = require('./helpers');
const env = require('./env');

const { expect, apiAs, loginAs } = helpers;

const PASSWORD = 'Sturdy-Pass-42';
const BASE_URL = `http://localhost:${env.PORT}`;

let userCount = 0;

/** An API client with no session, i.e. someone who only has the link. */
const anonymous = () => request.newContext({ baseURL: BASE_URL });

const test = helpers.test.extend({
  // An admin API client plus new users (deleted at teardown) who hold an invite.
  invites: async ({}, use) => {
    const admin = await apiAs(env.ADMIN_USERNAME);
    const userIds = [];
    await use({
      admin,
      async newUser() {
        const username = `invitee${++userCount}`;
        const res = await admin.post('/api/admin/users', { data: { username, name: `Invitee ${userCount}` } });
        expect(res.status(), 'create user').toBe(201);
        const body = await res.json();
        userIds.push(body.user.id);
        return { id: body.user.id, username, token: body.invite_token, body };
      },
      async reset(userId) {
        const res = await admin.post(`/api/admin/users/${userId}/reset-password`);
        expect(res.ok(), 'reset password').toBeTruthy();
        return (await res.json()).invite_token;
      },
      track: (id) => userIds.push(id),
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

const login = async (api, username, password) => (await api.post('/auth/login', { data: { username, password } })).status();

test('a new user sets a password from the invite and is logged in', async ({ page, invites }) => {
  const user = await invites.newUser();
  expect(user.token).toBeTruthy();
  expect(user.body.temp_password).toBeUndefined();

  await page.goto(`/invite/${user.token}`);
  await expect(page.getByText(`Your username is ${user.username}.`)).toBeVisible();
  await expect(page.getByLabel(/current/i)).toHaveCount(0);
  await setPasswordInForm(page, PASSWORD);
  await expect(page).toHaveURL(/\/profile/);

  const me = await (await page.request.get('/auth/current-user')).json();
  expect(me.username).toBe(user.username);
  expect(me.must_change_password).toBe(false);

  const fresh = await anonymous();
  expect(await login(fresh, user.username, PASSWORD)).toBe(200);
  await fresh.dispose();
});

test('the admin page shows the invite link once a user is added', async ({ page, invites }) => {
  await loginAs(page, env.ADMIN_USERNAME);
  await page.goto('/admin/users');
  const username = `invitee-ui${++userCount}`;
  await page.getByPlaceholder('username', { exact: true }).fill(username);
  await page.getByRole('button', { name: 'Add', exact: true }).first().click();

  const url = page.locator('code', { hasText: '/invite/' });
  await expect(url).toBeVisible();
  await expect(page.getByText("Shown once: it can't be retrieved later.")).toBeVisible();
  const link = (await url.textContent()) || '';
  const list = await (await invites.admin.get('/api/admin/users')).json();
  invites.track(list.find((u) => u.username === username).id);

  await page.goto(link);
  await expect(page.getByLabel('New password', { exact: true })).toBeVisible();
  await expect(page.getByText(`Your username is ${username}.`)).toBeVisible();
});

test('a used invite is dead and points to login', async ({ page, invites }) => {
  const user = await invites.newUser();
  const api = await anonymous();
  expect((await api.post(`/api/invite/${user.token}/password`, { data: { new_password: PASSWORD } })).ok()).toBeTruthy();
  expect((await api.post(`/api/invite/${user.token}/password`, { data: { new_password: 'Another-Pass-77' } })).status()).toBe(404);
  await api.dispose();

  await page.goto(`/invite/${user.token}`);
  await expect(page.getByRole('heading', { name: 'Link not valid' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Go to login' })).toBeVisible();
});

test('a reset revokes the password and sessions, and a second reset kills the first invite', async ({ page, invites }) => {
  const user = await invites.newUser();
  const api = await anonymous();
  expect((await api.post(`/api/invite/${user.token}/password`, { data: { new_password: PASSWORD } })).ok()).toBeTruthy();
  // `api` now holds a session for the user.
  expect((await api.get('/auth/current-user')).ok()).toBeTruthy();
  expect((await api.post('/api/profile/setup-complete')).ok()).toBeTruthy();

  const first = await invites.reset(user.id);
  expect((await api.get('/auth/current-user')).status()).toBe(401);
  const fresh = await anonymous();
  expect(await login(fresh, user.username, PASSWORD)).toBe(401);

  const second = await invites.reset(user.id);
  expect((await api.get(`/api/invite/${first}`)).status()).toBe(404);
  expect((await api.post(`/api/invite/${first}/password`, { data: { new_password: PASSWORD } })).status()).toBe(404);

  await page.goto(`/invite/${second}`);
  await setPasswordInForm(page, 'Brand-New-Pass-9');
  await expect(page).toHaveURL(/\/$/);
  expect(await login(fresh, user.username, 'Brand-New-Pass-9')).toBe(200);
  await fresh.dispose();
  await api.dispose();
});

test('blocking kills the invite for good', async ({ invites }) => {
  const user = await invites.newUser();
  const api = await anonymous();
  expect((await api.get(`/api/invite/${user.token}`)).ok()).toBeTruthy();

  expect((await invites.admin.post(`/api/admin/users/${user.id}/block`)).ok()).toBeTruthy();
  expect((await api.get(`/api/invite/${user.token}`)).status()).toBe(404);
  expect((await invites.admin.post(`/api/admin/users/${user.id}/unblock`)).ok()).toBeTruthy();
  expect((await api.get(`/api/invite/${user.token}`)).status()).toBe(404);
  expect((await api.post(`/api/invite/${user.token}/password`, { data: { new_password: PASSWORD } })).status()).toBe(404);
  await api.dispose();
});

test('rejected passwords leave the invite usable', async ({ page, invites }) => {
  const user = await invites.newUser();
  await page.goto(`/invite/${user.token}`);
  await setPasswordInForm(page, PASSWORD, `${PASSWORD}x`);
  await expect(page.getByText('Passwords do not match')).toBeVisible();

  const api = await anonymous();
  expect((await api.post(`/api/invite/${user.token}/password`, { data: { new_password: 'weak' } })).status()).toBe(400);
  expect((await api.get(`/api/invite/${user.token}`)).ok()).toBeTruthy();
  expect((await api.post(`/api/invite/${user.token}/password`, { data: { new_password: PASSWORD } })).ok()).toBeTruthy();
  await api.dispose();
});

test('the invite replaces a session already in the browser', async ({ page, invites }) => {
  const user = await invites.newUser();
  await loginAs(page, 'testy');

  await page.goto(`/invite/${user.token}`);
  await setPasswordInForm(page, PASSWORD);
  await expect(page).toHaveURL(/\/profile/);

  const me = await (await page.request.get('/auth/current-user')).json();
  expect(me.username).toBe(user.username);
});

test('only admins issue invites, and guest links are gone', async ({ invites }) => {
  const user = await invites.newUser();
  const testy = await apiAs('testy');
  expect((await testy.post('/api/admin/users', { data: { username: 'sneaky-user' } })).status()).toBe(403);
  expect((await testy.post(`/api/admin/users/${user.id}/reset-password`)).status()).toBe(403);
  expect((await testy.get(`/api/campaigns/${user.id}/guest-links`)).status()).not.toBe(200);
  await testy.dispose();

  const api = await anonymous();
  expect((await api.get('/api/guest/x')).status()).not.toBe(200);
  expect((await api.get('/api/invite/x')).status()).toBe(404);
  await api.dispose();
});
