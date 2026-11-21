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

// A 1x1 PNG.
const TINY_PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64');

test.describe('Codices pictures and tags', () => {
  let editor;
  let codex;
  let pageId;
  let pageUrl;

  test.beforeEach(async () => {
    editor = await apiAs('roy');
    const home = await (await editor.get('/api/codices')).json();
    codex = home.codices.find((c) => c.can_edit);
    expect(codex, 'roy can edit a codex').toBeTruthy();
    const res = await editor.post('/api/codices/pages', { data: { title: 'E2E pictures page', codex_id: codex.id } });
    expect(res.status(), 'create page').toBe(201);
    pageId = (await res.json()).page.id;
    pageUrl = `/codices/p/${pageId}/e2e-pictures-page`;
  });

  test.afterEach(async () => {
    await editor.delete(`/api/codices/pages/${pageId}`);
    const tags = await (await editor.get(`/api/codices/c/${codex.id}/tags`)).json();
    for (const t of tags.filter((x) => x.name === 'e2e-tag')) await editor.delete(`/api/codices/tags/${t.id}`);
    await editor.dispose();
  });

  test('an uploaded picture goes into the text, shows as a figure and opens in the lightbox', async ({ page }) => {
    await loginAs(page, 'roy');
    await page.goto(`${pageUrl}/edit`);
    await page.locator('input[type=file]').setInputFiles({ name: 'tiny.png', mimeType: 'image/png', buffer: TINY_PNG });
    await expect(page.locator('.cm-content')).toContainText('![[tiny.png|]]');
    await expect(page.locator('.cx-thumbs li')).toHaveCount(1);
    await page.getByRole('button', { name: 'Set tiny.png as the cover' }).click();
    await expect(page.locator('.cx-flag')).toBeVisible();
    await page.getByRole('button', { name: 'Save changes' }).click();
    await page.waitForURL(/saved=/);

    await expect(page.locator('.codex-text figure img')).toBeVisible();
    await expect(page.locator('.cx-cover img')).toBeVisible();
    await page.locator('.codex-text button.cx-img-btn').click();
    await expect(page.locator('dialog.cx-lightbox')).toBeVisible();
    await expect(page.locator('.cx-lightbox figcaption')).toContainText('tiny.png · 1×1');
    await page.keyboard.press('Escape');
    await expect(page.locator('dialog.cx-lightbox')).toBeHidden();

    // A see-only user can look at it but not change it.
    const viewer = await apiAs('adam');
    const list = await viewer.get(`/api/codices/pages/${pageId}/pictures`);
    expect(list.ok()).toBeTruthy();
    const [img] = await list.json();
    expect((await viewer.get(img.url)).status()).toBe(200);
    expect((await viewer.delete(`/api/codices/pages/${pageId}/pictures/${img.id}`)).status()).toBe(403);
    await viewer.dispose();
  });

  test('tags: add them on a page, click one to list its pages', async ({ page }) => {
    await loginAs(page, 'roy');
    await page.goto(pageUrl);
    await page.getByRole('button', { name: 'Edit tags' }).click();
    await page.getByLabel('Add a tag').fill('e2e-tag');
    await page.keyboard.press('Enter');
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    const chip = page.locator('.cx-prop a.cx-chip--tag', { hasText: 'e2e-tag' });
    await expect(chip).toBeVisible();
    await chip.click();
    await expect(page).toHaveURL(/\?tag=e2e-tag/);
    await expect(page.locator('#tagged').getByRole('link', { name: 'E2E pictures page' })).toBeVisible();
  });
});

test.describe('Codices templates and copying', () => {
  let editor;
  let codex;
  const made = []; // page ids to remove
  let hadTags; // names of the tags the codex had before the test: only new ones are removed

  test.beforeEach(async () => {
    editor = await apiAs('roy');
    const home = await (await editor.get('/api/codices')).json();
    codex = home.codices.find((c) => c.can_edit);
    expect(codex, 'roy can edit a codex').toBeTruthy();
    hadTags = (await (await editor.get(`/api/codices/c/${codex.id}/tags`)).json()).map((t) => t.name);
    // adam is the see-only viewer: set that up here instead of relying on the seed or other tests.
    const admin = await apiAs(env.ADMIN_USERNAME);
    const adam = (await access(admin)).users.find((u) => u.username === 'adam');
    await setRight(admin, adam.id, 'codices.see', true);
    await setLevel(admin, codex.id, adam.id, 'see');
    await admin.dispose();
  });

  test.afterEach(async () => {
    for (const id of made.splice(0)) await editor.delete(`/api/codices/pages/${id}`);
    const templates = await (await editor.get(`/api/codices/c/${codex.id}/templates`)).json();
    for (const t of templates.filter((x) => x.name.startsWith('E2E'))) await editor.delete(`/api/codices/templates/${t.id}`);
    const tags = await (await editor.get(`/api/codices/c/${codex.id}/tags`)).json();
    for (const t of tags.filter((x) => (x.name === 'session' || x.name === 'e2e-tpl-tag') && !hadTags.includes(x.name))) await editor.delete(`/api/codices/tags/${t.id}`);
    await editor.dispose();
  });

  test('a page made from the Session template is copied with a new history', async ({ page }) => {
    await loginAs(page, 'roy');
    await page.goto(`/codices/c/${codex.id}`);
    await page.getByRole('button', { name: '+ New page' }).click();
    await page.getByRole('radio', { name: /Session/ }).click();
    await expect(page.locator('.cx-tpl-preview')).toContainText('Summary');
    await expect(page.locator('.cx-np-tags')).toHaveText('Adds tags: session');
    await page.getByLabel('Title').fill('E2E session notes');
    await page.getByLabel('Title').press('Enter');
    await page.waitForURL(/\/edit$/);
    const id = page.url().match(/\/p\/([^/]+)\//)[1];
    made.push(id);

    // The editor opens on the template's text; the tag is already on the page.
    await expect(page.locator('.cm-content')).toContainText('## What happened');
    const created = await (await editor.get(`/api/codices/pages/${id}`)).json();
    expect(created.page.body).toContain('**Date:** ');
    expect(created.page.body).not.toContain('{{date}}');
    expect(created.tags.map((t) => t.name)).toEqual(['session']);

    // More > Copy page… into the same codex under a new title.
    await page.goto(`/codices/p/${id}/e2e-session-notes`);
    await page.getByRole('button', { name: /More/ }).click();
    await page.getByRole('menuitem', { name: 'Copy page…' }).click();
    await expect(page.getByLabel('Title')).toHaveValue('E2E session notes (copy)');
    await page.getByLabel('Title').fill('E2E session notes, take two');
    await page.getByRole('button', { name: 'Copy page', exact: true }).click();
    await page.waitForURL(/\/p\/[^/]+\/e2e-session-notes-take-two/);
    await expect(page.getByRole('status')).toHaveText('Copied.');
    const copyId = page.url().match(/\/p\/([^/]+)\//)[1];
    made.push(copyId);

    const copy = await (await editor.get(`/api/codices/pages/${copyId}`)).json();
    expect(copy.page.body).toBe(created.page.body);
    expect(copy.page.current_revision).toBe(1);
    expect(copy.tags.map((t) => t.name)).toEqual(['session']);
    const revs = await (await editor.get(`/api/codices/pages/${copyId}/revisions`)).json();
    expect(revs.revisions).toHaveLength(1);
    expect(revs.revisions[0].note).toBe('Copied from E2E session notes');
  });

  test('a codex template is used by the New page dialog; a see-only user cannot change it', async ({ page }) => {
    const res = await editor.post(`/api/codices/c/${codex.id}/templates`, {
      data: { name: 'E2E tavern', description: 'An inn', body: '_About {{title}}._\n\n## Rooms\n', tags: ['e2e-tpl-tag'] },
    });
    expect(res.status(), 'create template').toBe(201);
    await loginAs(page, 'roy');
    await page.goto(`/codices/c/${codex.id}`);
    await page.getByRole('button', { name: '+ New page' }).click();
    await page.getByRole('radio', { name: /E2E tavern/ }).click();
    await page.getByLabel('Title').fill('E2E Prancing Pony');
    await expect(page.locator('.cx-tpl-preview')).toContainText('About E2E Prancing Pony.');
    await page.getByRole('button', { name: 'Create page' }).click();
    await page.waitForURL(/\/edit$/);
    made.push(page.url().match(/\/p\/([^/]+)\//)[1]);
    await expect(page.locator('.cm-content')).toContainText('## Rooms');

    const viewer = await apiAs('adam');
    expect((await viewer.get(`/api/codices/c/${codex.id}/templates`)).ok()).toBeTruthy();
    const t = (await (await editor.get(`/api/codices/c/${codex.id}/templates`)).json()).find((x) => x.name === 'E2E tavern');
    expect((await viewer.put(`/api/codices/templates/${t.id}`, { data: { name: 'Hijacked' } })).status()).toBe(403);
    expect((await viewer.delete(`/api/codices/templates/${t.id}`)).status()).toBe(403);
    await viewer.dispose();
  });
});

test.describe('Codices graph', () => {
  test('the graph lists the codex pages; find, card and Open page work; no See means 404', async ({ page }) => {
    const admin = await apiAs(env.ADMIN_USERNAME);
    const data = await access(admin);
    const codex = data.codices[0];
    const adam = data.users.find((u) => u.username === 'adam');
    const corvin = data.users.find((u) => u.username === USER);
    await setRight(admin, adam.id, 'codices.see', true);
    await setLevel(admin, codex.id, adam.id, 'see');
    await setLevel(admin, codex.id, corvin.id, 'none');

    // The API follows the codex's See right.
    const stranger = await apiAs(USER);
    expect((await stranger.get(`/api/codices/c/${codex.id}/graph`)).status()).toBe(404);
    await stranger.dispose();
    await admin.dispose();

    await loginAs(page, 'adam');
    const graph = await (await page.request.get(`/api/codices/c/${codex.id}/graph`)).json();
    const pages = graph.nodes.filter((n) => !n.missing);
    expect(pages.length).toBeGreaterThan(1);
    await page.goto(`/codices/c/${codex.id}/graph`);
    await expect(page.locator('#gvSvg .node')).toHaveCount(graph.nodes.length);
    await expect(page.locator('#gvSvg .node.missing')).toHaveCount(graph.nodes.length - pages.length);

    // The filters are folded on narrow screens.
    if (!(await page.locator('#gvSettings').evaluate((el) => el.open))) await page.locator('#gvSettings summary').click();
    await page.getByLabel('Find a page').fill('Karrow Ford');
    await page.getByLabel('Find a page').press('Enter');
    await expect(page.locator('#gvCardTitle')).toHaveText('Karrow Ford');
    await expect(page.locator('#gvCardCreate')).toBeHidden();
    await page.getByRole('link', { name: 'Open page' }).click();
    await expect(page).toHaveURL(/\/codices\/p\/[^/]+\/karrow-ford$/);
    await expect(page.locator('h1.cx-title')).toHaveText('Karrow Ford');
    // The page's Local graph opens the full graph from this page.
    await expect(page.locator('#localSvg .node').first()).toBeAttached();
    await page.locator('.local-graph').click();
    await expect(page).toHaveURL(/\/graph\?from=/);
    await expect(page.locator('#gvSvg .node.current')).toHaveCount(1);
  });
});
