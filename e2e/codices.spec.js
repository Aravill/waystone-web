// @ts-check
const helpers = require('./helpers');
const env = require('./env');

const { test, expect, apiAs, loginAs } = helpers;

const USER = 'corvin';

async function access(admin) {
  const res = await admin.get('/api/admin/access');
  expect(res.ok(), 'GET /api/admin/access').toBeTruthy();
  return res.json();
}

async function setRight(admin, userId, action, on) {
  const res = await admin.put(`/api/admin/access/users/${userId}`, { data: { action, on } });
  expect(res.ok(), `set ${action}`).toBeTruthy();
}

async function setLevel(admin, codexId, userId, level) {
  const res = await admin.put(`/api/admin/access/codices/${codexId}/users/${userId}`, { data: { level } });
  expect(res.ok(), `set level ${level}`).toBeTruthy();
}

test.describe('Codices access rights', () => {
  let admin;
  let user;
  let codex;

  test.beforeEach(async () => {
    admin = await apiAs(env.ADMIN_USERNAME);
    const data = await access(admin);
    user = data.users.find((u) => u.username === USER);
    codex = data.codices[0];
    expect(user, `${USER} is seeded`).toBeTruthy();
    expect(codex, 'a codex is seeded').toBeTruthy();
    // Start from nothing: new users get no rights.
    for (const action of user.rights) await setRight(admin, user.id, action, false);
    await setLevel(admin, codex.id, user.id, 'none');
  });

  test.afterEach(async () => {
    // The seed gives demo users See Codices; put it back for any later test.
    await setRight(admin, user.id, 'codices.see', true);
    await setLevel(admin, codex.id, user.id, 'see');
    await admin.dispose();
  });

  test('a user without rights cannot reach Codices; See is read-only; Edit saves', async ({ page }) => {
    await loginAs(page, USER);
    expect((await page.request.get('/api/codices')).status()).toBe(404);
    await page.goto('/codices');
    await expect(page).toHaveURL(/\/$/);

    // See Codices alone shows nothing: no codex is granted yet.
    await setRight(admin, user.id, 'codices.see', true);
    const home = await (await page.request.get('/api/codices')).json();
    expect(home.codices).toEqual([]);
    expect(home.can_create).toBe(false);
    expect((await page.request.get(`/api/codices/c/${codex.id}`)).status()).toBe(404);

    // See on the codex: readable, not writable.
    await setLevel(admin, codex.id, user.id, 'see');
    const view = await (await page.request.get(`/api/codices/c/${codex.id}`)).json();
    expect(view.can_edit).toBe(false);
    const post = await page.request.post('/api/codices/pages', { data: { title: 'Forbidden page', codex_id: codex.id } });
    expect(post.status()).toBe(403);
    await page.goto(`/codices/c/${codex.id}`);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(codex.name);
    await expect(page.getByRole('button', { name: '+ New page' })).toBeHidden();

    // Edit on the codex: the same request now succeeds.
    await setLevel(admin, codex.id, user.id, 'edit');
    const ok = await page.request.post('/api/codices/pages', { data: { title: 'Access rights test page', codex_id: codex.id } });
    expect(ok.status()).toBe(201);
    const id = (await ok.json()).page.id;
    expect((await page.request.delete(`/api/codices/pages/${id}`)).ok()).toBeTruthy();
  });

  test('the Access rights page lists the user and keeps changes', async ({ page }) => {
    await loginAs(page, env.ADMIN_USERNAME);
    await page.goto('/admin/access');
    await expect(page.getByRole('heading', { name: 'Access rights' })).toBeVisible();
    // The user is in both tables: the module checkboxes and the per-codex grid.
    const moduleRow = page.locator('section.card').first().locator('tr', { hasText: USER });
    const gridRow = page.locator('.access-grid tbody tr', { hasText: USER });
    await expect(moduleRow).toBeVisible();
    await gridRow.locator('select').first().selectOption('edit');
    await expect(page.getByRole('status')).toHaveText('Saved');
    const saved = page.waitForResponse((r) => r.url().includes('/api/admin/access/users/') && r.request().method() === 'PUT');
    await moduleRow.getByRole('checkbox').first().check();
    expect((await saved).ok()).toBeTruthy();

    await page.reload();
    await expect(gridRow.locator('select').first()).toHaveValue('edit');
    await expect(moduleRow.getByRole('checkbox').first()).toBeChecked();
  });
});
