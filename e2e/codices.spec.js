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

test.describe('Codices folder default templates', () => {
  let editor;
  let codex;
  let folder;
  const made = [];
  const folderOf = async (id) => (await (await editor.get(`/api/codices/c/${codex.id}`)).json()).folders.find((f) => f.id === id);

  test.beforeEach(async () => {
    editor = await apiAs('roy');
    codex = (await (await editor.get('/api/codices')).json()).codices.find((c) => c.can_edit);
    expect(codex, 'roy can edit a codex').toBeTruthy();
    const res = await editor.post(`/api/codices/c/${codex.id}/folders`, { data: { name: 'E2E Folder defaults' } });
    expect(res.status(), 'create folder').toBe(201);
    folder = (await res.json()).folder;
  });

  test.afterEach(async () => {
    for (const id of made.splice(0)) await editor.delete(`/api/codices/pages/${id}`);
    await editor.delete(`/api/codices/folders/${folder.id}`);
    const templates = await (await editor.get(`/api/codices/c/${codex.id}/templates`)).json();
    for (const t of templates.filter((x) => x.name.startsWith('E2E'))) await editor.delete(`/api/codices/templates/${t.id}`);
    await editor.dispose();
  });

  test('a folder default is set from the folder menu and preselected by New page in it', async ({ page }) => {
    await loginAs(page, 'roy');
    await page.goto(`/codices/c/${codex.id}`);
    await page.getByRole('button', { name: 'Actions for folder E2E Folder defaults' }).click();
    await page.getByRole('menuitem', { name: /New pages start from/ }).click();
    await page.getByLabel('New pages here start from').selectOption({ label: 'Character' });
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await expect(page.getByRole('status')).toHaveText('Folder saved');
    expect((await folderOf(folder.id)).default_template).toBe('builtin:character');

    await page.getByRole('button', { name: 'Actions for folder E2E Folder defaults' }).click();
    await page.getByRole('menuitem', { name: '+ Page in E2E Folder defaults' }).click();
    const character = page.getByRole('radio', { name: /Character/ });
    await expect(character).toHaveAttribute('aria-checked', 'true');
    await expect(character.locator('.cx-tpl-def')).toHaveText('Folder default');
    await expect(page.locator('.cx-np-pick')).toContainText('start from Character');

    // Another folder brings its own choice along until a card is clicked.
    await page.getByLabel('Folder', { exact: true }).selectOption('');
    await expect(page.getByRole('radio', { name: /Blank/ })).toHaveAttribute('aria-checked', 'true');
    await expect(page.locator('.cx-tpl-def:visible')).toHaveCount(0);
    await page.getByLabel('Folder', { exact: true }).selectOption(folder.id);
    await expect(character).toHaveAttribute('aria-checked', 'true');
    await page.getByRole('radio', { name: /Session/ }).click();
    await page.getByLabel('Folder', { exact: true }).selectOption('');
    await page.getByLabel('Folder', { exact: true }).selectOption(folder.id);
    await expect(page.getByRole('radio', { name: /Session/ })).toHaveAttribute('aria-checked', 'true');
  });

  test('a missing link on a page in that folder starts from the folder default', async ({ page }) => {
    expect((await editor.put(`/api/codices/folders/${folder.id}`, { data: { default_template: 'builtin:character' } })).ok()).toBeTruthy();
    const res = await editor.post('/api/codices/pages', { data: { title: 'E2E Linker', codex_id: codex.id, folder_id: folder.id, body: 'Meet [[E2E Nobody Yet]].' } });
    expect(res.status(), 'create page').toBe(201);
    const created = (await res.json()).page;
    made.push(created.id);
    await loginAs(page, 'roy');
    await page.goto(`/codices/p/${created.id}/e2e-linker`);
    await page.locator('a.wikilink--missing', { hasText: 'E2E Nobody Yet' }).click(); // the link in the text, not the Wanted pages button
    await expect(page.getByLabel('Title')).toHaveValue('E2E Nobody Yet');
    await expect(page.getByRole('radio', { name: /Character/ })).toHaveAttribute('aria-checked', 'true');
    await expect(page.locator('.cx-tpl-def:visible')).toHaveText('Folder default');
  });

  test('a default must exist in the codex, and deleting its template clears it', async () => {
    expect((await editor.put(`/api/codices/folders/${folder.id}`, { data: { default_template: 'nope' } })).status()).toBe(400);
    const res = await editor.post(`/api/codices/c/${codex.id}/templates`, { data: { name: 'E2E default', description: '', body: '## A\n', tags: [] } });
    expect(res.status(), 'create template').toBe(201);
    const t = (await res.json()).template;
    const set = await editor.put(`/api/codices/folders/${folder.id}`, { data: { default_template: t.id } });
    expect(set.ok()).toBeTruthy();
    expect((await folderOf(folder.id)).default_template).toBe(t.id);
    expect((await editor.delete(`/api/codices/templates/${t.id}`)).ok()).toBeTruthy();
    expect((await folderOf(folder.id)).default_template).toBe('');
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
    const keys = graph.links.map((l) => l.source + '|' + l.target);
    expect(new Set(keys).size, 'links are not repeated').toBe(keys.length);
    // ?around= gives just a page, its neighbours and the links among them, without excerpts.
    const hub = graph.nodes.find((n) => n.title === 'Karrow Ford');
    const near = await (await page.request.get(`/api/codices/c/${codex.id}/graph?around=${hub.id}`)).json();
    expect(near.nodes.length).toBeGreaterThan(1);
    expect(near.nodes.length).toBeLessThan(graph.nodes.length);
    expect(near.nodes.every((n) => n.excerpt === '')).toBe(true);
    expect(near.nodes.map((n) => n.id)).toContain(hub.id);
    expect((await page.request.get(`/api/codices/c/${codex.id}/graph?around=missing:nope`)).status()).toBe(404);
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

test.describe('Codices local drafts', () => {
  let editor;
  let pageId;
  let editUrl;

  test.beforeEach(async () => {
    editor = await apiAs('roy');
    const home = await (await editor.get('/api/codices')).json();
    const codex = home.codices.find((c) => c.can_edit);
    expect(codex, 'roy can edit a codex').toBeTruthy();
    const res = await editor.post('/api/codices/pages', { data: { title: 'E2E drafts page', codex_id: codex.id } });
    expect(res.status(), 'create page').toBe(201);
    pageId = (await res.json()).page.id;
    editUrl = `/codices/p/${pageId}/e2e-drafts-page/edit`;
  });

  test.afterEach(async () => {
    await editor.delete(`/api/codices/pages/${pageId}`);
    await editor.dispose();
  });

  async function typeAndWait(page, text) {
    await page.locator('.cm-content').click();
    await page.keyboard.type(text);
    await expect(page.getByText('Draft kept on this device')).toBeVisible();
  }

  test('a draft is restored after a reload and gone after saving', async ({ page }) => {
    await loginAs(page, 'roy');
    await page.goto(editUrl);
    await typeAndWait(page, 'Half written thought');

    await page.reload();
    await expect(page.getByText('You have unsaved changes from')).toBeVisible();
    await page.getByRole('button', { name: 'Restore them' }).click();
    await expect(page.locator('.cm-content')).toContainText('Half written thought');
    await expect(page.getByText('You have unsaved changes from')).toBeHidden();

    await page.getByRole('button', { name: 'Save changes' }).click();
    await page.waitForURL(/saved=/);
    await page.goto(editUrl);
    await expect(page.locator('.cm-content')).toContainText('Half written thought');
    await expect(page.getByText('You have unsaved changes from')).toBeHidden();
  });

  test('a draft on an older revision opens the conflict panel on save', async ({ page }) => {
    await loginAs(page, 'roy');
    await page.goto(editUrl);
    await typeAndWait(page, 'My side of the story');

    const cur = await (await editor.get(`/api/codices/pages/${pageId}`)).json();
    const saved = await editor.put(`/api/codices/pages/${pageId}`, { data: { base_revision: cur.page.current_revision, body: 'Someone else wrote this', note: '' } });
    expect(saved.status(), 'other save').toBe(201);

    await page.reload();
    await page.getByRole('button', { name: 'Restore them' }).click();
    await expect(page.locator('.cm-content')).toContainText('My side of the story');
    await page.getByRole('button', { name: 'Save changes' }).click();
    await expect(page.locator('.conflict')).toBeVisible();
    await expect(page.locator('.conflict .diff')).toContainText('Someone else wrote this');
  });

  test('Cancel asks before dropping changes and clears the draft', async ({ page }) => {
    await loginAs(page, 'roy');
    await page.goto(editUrl);
    await typeAndWait(page, 'Not wanted');
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByText('Discard your changes?')).toBeVisible();
    await page.getByRole('button', { name: 'Discard changes' }).click();
    await page.waitForURL(new RegExp(`/codices/p/${pageId}/e2e-drafts-page$`));
    await page.goto(editUrl);
    await expect(page.getByText('You have unsaved changes from')).toBeHidden();
  });
});

test.describe('Codices tree drag and drop', () => {
  let editor;
  let codex;
  const pages = [];
  const folders = []; // children first, so each delete finds an empty parent

  async function makeFolder(name, parentId = '') {
    const res = await editor.post(`/api/codices/c/${codex.id}/folders`, { data: { name, parent_id: parentId } });
    expect(res.status(), 'create folder').toBe(201);
    const id = (await res.json()).folder.id;
    folders.unshift(id);
    return id;
  }
  async function folderOf(id) {
    return (await (await editor.get(`/api/codices/pages/${id}`)).json()).page.folder_id;
  }
  async function parentOf(id) {
    const data = await (await editor.get(`/api/codices/c/${codex.id}`)).json();
    return data.folders.find((f) => f.id === id).parent_id;
  }

  test.beforeEach(async () => {
    editor = await apiAs('roy');
    const home = await (await editor.get('/api/codices')).json();
    codex = home.codices.find((c) => c.can_edit);
    expect(codex, 'roy can edit a codex').toBeTruthy();
  });

  test.afterEach(async () => {
    for (const id of pages.splice(0)) await editor.delete(`/api/codices/pages/${id}`);
    for (const id of folders.splice(0)) await editor.delete(`/api/codices/folders/${id}`);
    await editor.dispose();
  });

  test('a page dropped on a folder moves there and Undo moves it back; a folder goes to the top level; a folder into its child sends nothing', async ({ page }) => {
    const tag = Date.now().toString(36);
    const target = await makeFolder(`E2E drop target ${tag}`);
    const outer = await makeFolder(`E2E outer ${tag}`, target);
    const inner = await makeFolder(`E2E inner ${tag}`, outer);
    const made = await editor.post('/api/codices/pages', { data: { title: `E2E dragged ${tag}`, codex_id: codex.id } });
    expect(made.status(), 'create page').toBe(201);
    const pageId = (await made.json()).page.id;
    pages.push(pageId);

    await loginAs(page, 'roy');
    await page.goto(`/codices/c/${codex.id}`);
    const tree = page.locator('.codex-tree');
    const row = (id) => tree.locator(`details[data-id="${id}"] > summary`);
    // Step by step: the Top level strip appears (and the tree shifts down) a moment after the drag starts.
    const drag = async (from, to) => {
      await from.hover();
      await page.mouse.down();
      await page.mouse.move(10, 10);
      await expect(page.locator('.codex-root-drop')).toBeVisible();
      await to.hover();
      await to.hover({ position: { x: 20, y: 8 } });
      await page.mouse.up();
    };

    // 1. A page onto a folder, then Undo.
    await drag(tree.getByRole('link', { name: `E2E dragged ${tag}` }), row(target));
    const toast = page.getByRole('status').filter({ hasText: 'Moved' });
    await expect(toast).toContainText(`Moved E2E dragged ${tag} to E2E drop target ${tag}.`);
    expect(await folderOf(pageId)).toBe(target);
    await toast.getByRole('button', { name: 'Undo' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'back.' })).toBeVisible();
    expect(await folderOf(pageId)).toBe('');

    // 2. A folder onto the Top level strip.
    await expect(row(outer)).toBeVisible();
    await drag(row(outer), page.locator('.codex-root-drop'));
    await expect(page.getByRole('status').filter({ hasText: 'to the top level.' })).toBeVisible();
    expect(await parentOf(outer)).toBe('');

    // 3. A folder onto its own child: refused on the client, no move request.
    const moves = [];
    page.on('request', (req) => req.method() === 'PUT' && moves.push(req.url()));
    await expect(row(inner)).toBeVisible();
    await drag(row(outer), row(inner));
    await page.waitForTimeout(300);
    expect(moves).toEqual([]);
    expect(await parentOf(outer)).toBe('');
    expect(await parentOf(inner)).toBe(outer);
  });
});

test.describe('Codices live lists', () => {
  let editor;
  let codex;
  const ids = {};

  async function make(title, body, tags) {
    const res = await editor.post('/api/codices/pages', { data: { title, body, codex_id: codex.id } });
    expect(res.status(), 'create ' + title).toBe(201);
    const id = (await res.json()).page.id;
    if (tags) expect((await editor.put(`/api/codices/pages/${id}/tags`, { data: { names: tags } })).ok()).toBeTruthy();
    return id;
  }

  test.beforeEach(async () => {
    editor = await apiAs('roy');
    const home = await (await editor.get('/api/codices')).json();
    codex = home.codices.find((c) => c.can_edit);
    expect(codex, 'roy can edit a codex').toBeTruthy();
    ids.bard = await make('E2E Bard', 'Lives in [[E2E Hub]].', ['e2e-npc']);
    ids.alchemist = await make('E2E Alchemist', 'Sells potions.', ['e2e-npc']);
    ids.cook = await make('E2E Cook', 'Cooks.', []);
    ids.hub = await make('E2E Hub', '{{list tag:e2e-npc sort:title}}', []);
  });

  test.afterEach(async () => {
    for (const id of Object.values(ids)) await editor.delete(`/api/codices/pages/${id}`);
    const tags = await (await editor.get(`/api/codices/c/${codex.id}/tags`)).json();
    for (const t of tags.filter((x) => x.name === 'e2e-npc')) await editor.delete(`/api/codices/tags/${t.id}`);
    await editor.dispose();
  });

  test('a list shows the tagged pages in order, and a newly tagged page joins it', async ({ page }) => {
    await loginAs(page, 'roy');
    await page.goto(`/codices/p/${ids.hub}/e2e-hub`);
    const rows = page.locator('.codex-text .cx-live li a.wikilink');
    await expect(rows).toHaveText(['E2E Alchemist', 'E2E Bard']);
    await expect(page.locator('.cx-live-h')).toContainText('tag e2e-npc');
    await expect(page.locator('.cx-live-h')).toContainText('2 pages');

    expect((await editor.put(`/api/codices/pages/${ids.cook}/tags`, { data: { names: ['e2e-npc'] } })).ok()).toBeTruthy();
    await page.reload();
    await expect(rows).toHaveText(['E2E Alchemist', 'E2E Bard', 'E2E Cook']);
  });

  test('links:here lists only the pages that link to this one', async ({ page }) => {
    const res = await editor.get(`/api/codices/pages/${ids.hub}`);
    const { page: hub } = await res.json();
    const save = await editor.put(`/api/codices/pages/${ids.hub}`, { data: { base_revision: hub.current_revision, body: '{{list tag:e2e-npc links:here}}'+String.fromCharCode(10,10)+'{{list tag:e2e-npc bogus:1}}', note: '' } });
    expect(save.ok(), 'save hub').toBeTruthy();
    await loginAs(page, 'roy');
    await page.goto(`/codices/p/${ids.hub}/e2e-hub`);
    const first = page.locator('.codex-text .cx-live').first();
    await expect(first.locator('li a.wikilink')).toHaveText(['E2E Bard']);
    await expect(first.locator('.cx-live-h')).toContainText('linking here');
    await expect(page.locator('.codex-text .cx-live').nth(1)).toContainText('Unknown list option: bogus:1');
  });

  test('a list adds nothing to Linked from', async ({ page }) => {
    const backlinks = async (id) => (await (await editor.get(`/api/codices/pages/${id}`)).json()).backlinks;
    expect(await backlinks(ids.hub)).toHaveLength(1); // the Bard's own [[E2E Hub]]
    expect(await backlinks(ids.bard)).toEqual([]);
    expect(await backlinks(ids.alchemist)).toEqual([]);
    await loginAs(page, 'roy');
    await page.goto(`/codices/p/${ids.alchemist}/e2e-alchemist`);
    await expect(page.locator('.cx-box', { hasText: 'Linked from' })).toContainText('No other page links here yet.');
  });
});

test.describe('Codices unlinked mentions', () => {
  let editor;
  let codex;
  const ids = [];

  test.beforeEach(async () => {
    editor = await apiAs('roy');
    const home = await (await editor.get('/api/codices')).json();
    codex = home.codices.find((c) => c.can_edit);
    expect(codex, 'roy can edit a codex').toBeTruthy();
  });

  test.afterEach(async () => {
    for (const id of ids.splice(0)) await editor.delete(`/api/codices/pages/${id}`);
    await editor.dispose();
  });

  async function makePage(title, body) {
    const res = await editor.post('/api/codices/pages', { data: { title, body, codex_id: codex.id } });
    expect(res.status(), 'create ' + title).toBe(201);
    const id = (await res.json()).page.id;
    ids.push(id);
    return id;
  }

  test('a plain mention can be linked; links and URLs are not mentions; See sees no button', async ({ page }) => {
    const bId = await makePage('E2E Mention Source', 'We met at the Old Mill at dawn. Old Mill again.');
    const cId = await makePage('E2E Mention Safe', 'Already [[Old Mill]], see https://example.com/old-mill and `old mill`.');
    const aId = await makePage('Old Mill', 'A mill.');

    // The API lists B (two mentions) and leaves out the page with only links, URLs and code.
    const list = await (await editor.get(`/api/codices/pages/${aId}/mentions`)).json();
    expect(list.total).toBe(1);
    expect(list.mentions[0].page.id).toBe(bId);
    expect(list.mentions[0].count).toBe(2);
    expect(list.mentions[0].snippets[0].match).toBe('Old Mill');
    expect(list.can_edit).toBe(true);
    expect(list.mentions.some((m) => m.page.id === cId)).toBe(false);

    await loginAs(page, 'roy');
    await page.goto(`/codices/p/${aId}/old-mill`);
    const box = page.locator('.cx-box--mentions');
    await expect(box.locator('li')).toHaveCount(1);
    await expect(box.locator('mark.cx-mention').first()).toHaveText('Old Mill');
    await expect(page.locator('[aria-label="Linked from"]')).not.toContainText('E2E Mention Source');
    await box.getByRole('button', { name: /^Link 2/ }).click();
    await expect(page.locator('.message, [role=status], .toast').filter({ hasText: 'Linked 2 mentions in E2E Mention Source' }).first()).toBeVisible();
    await expect(box.locator('li')).toHaveCount(0);
    await expect(box.locator('.cx-box-empty').first()).toHaveText('No plain mentions of this title.');
    await expect(page.locator('[aria-label="Linked from"]')).toContainText('E2E Mention Source');

    const b = await (await editor.get(`/api/codices/pages/${bId}`)).json();
    expect(b.page.body).toBe('We met at the [[Old Mill]] at dawn. [[Old Mill]] again.');
    const c = await (await editor.get(`/api/codices/pages/${cId}`)).json();
    expect(c.page.body).toContain('https://example.com/old-mill');

    // Spelled differently, the text is kept as the label.
    const dId = await makePage('E2E Mention Lower', 'down by the  old mill, a ruin.');
    const d = await (await editor.get(`/api/codices/pages/${dId}`)).json();
    const res = await editor.post(`/api/codices/pages/${aId}/mentions/link`, { data: { from_page_id: dId, base_revision: d.page.current_revision } });
    expect(res.ok()).toBeTruthy();
    const d2 = await (await editor.get(`/api/codices/pages/${dId}`)).json();
    expect(d2.page.body).toBe('down by the  [[Old Mill|old mill]], a ruin.');
    // A stale base is refused; nothing left to link is 200 with linked 0.
    const stale = await editor.post(`/api/codices/pages/${aId}/mentions/link`, { data: { from_page_id: dId, base_revision: 1 } });
    expect(stale.ok()).toBeTruthy();
    expect((await stale.json()).linked).toBe(0);

    // A See-only user gets the list without Link buttons, and the POST is refused.
    const admin = await apiAs(env.ADMIN_USERNAME);
    const data = await access(admin);
    const user = data.users.find((u) => u.username === USER);
    await setRight(admin, user.id, 'codices.see', true);
    await setLevel(admin, codex.id, user.id, 'see');
    const eId = await makePage('E2E Mention Viewer', 'Back to the old mill.');
    try {
      await loginAs(page, USER);
      await page.goto(`/codices/p/${aId}/old-mill`);
      await expect(box.locator('li')).toHaveCount(1);
      await expect(box.getByRole('button')).toHaveCount(0);
      const e = await (await page.request.get(`/api/codices/pages/${eId}`)).json();
      const denied = await page.request.post(`/api/codices/pages/${aId}/mentions/link`, { data: { from_page_id: eId, base_revision: e.page.current_revision } });
      expect(denied.status()).toBe(403);
    } finally {
      await admin.dispose();
    }
  });
});

test.describe('Codices quick notes and the Inbox', () => {
  let editor;
  let codex;
  const pages = []; // page ids to remove

  test.beforeEach(async () => {
    editor = await apiAs('roy');
    const home = await (await editor.get('/api/codices')).json();
    codex = home.codices.find((c) => c.can_edit);
    expect(codex, 'roy can edit a codex').toBeTruthy();
    // adam is the see-only viewer.
    const admin = await apiAs(env.ADMIN_USERNAME);
    const adam = (await access(admin)).users.find((u) => u.username === 'adam');
    await setRight(admin, adam.id, 'codices.see', true);
    await setLevel(admin, codex.id, adam.id, 'see');
    await admin.dispose();
  });

  test.afterEach(async () => {
    for (const id of pages.splice(0)) await editor.delete(`/api/codices/pages/${id}`);
    for (const user of ['roy', 'testy']) {
      const api = await apiAs(user);
      const { notes } = await (await api.get(`/api/codices/c/${codex.id}/notes`)).json();
      for (const n of notes) await api.delete(`/api/codices/notes/${n.id}`);
      await api.dispose();
    }
    await editor.dispose();
  });

  async function addNote(api, body) {
    const res = await api.post(`/api/codices/c/${codex.id}/notes`, { data: { body } });
    expect(res.status(), 'add note').toBe(201);
    return (await res.json()).note;
  }

  test('n opens the note box; saving puts it in the inbox with a count', async ({ page }) => {
    await loginAs(page, 'roy');
    await page.goto(`/codices/c/${codex.id}`);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(codex.name);
    await expect(page.locator('.cx-nav-group .cx-badge')).toBeHidden();
    await page.keyboard.press('n');
    await expect(page.locator('#dNote')).toBeFocused();
    await page.locator('#dNote').fill('E2E first note');
    await page.locator('#dNote').press('Control+Enter');
    await expect(page.getByRole('status')).toContainText('In your inbox.');
    await expect(page.locator('.cx-nav-group .cx-badge')).toHaveText('1');
    await page.getByRole('button', { name: 'Open inbox' }).click();
    await expect(page).toHaveURL(new RegExp(`/codices/c/${codex.id}/inbox$`));
    await expect(page.locator('.note-t')).toHaveText('E2E first note');
    // The draft is gone once saved.
    await page.keyboard.press('n');
    await expect(page.locator('#dNote')).toHaveValue('');
    // Typing an n in a field does not open the box.
    await page.keyboard.press('Escape');
    await page.getByRole('searchbox').click();
    await page.keyboard.type('n');
    await expect(page.locator('#dNote')).toBeHidden();
  });

  test('a note filed as a new page takes its first line as the title and leaves the inbox', async ({ page }) => {
    await addNote(editor, 'E2E inbox idea [x]\nsecond line');
    await loginAs(page, 'roy');
    await page.goto(`/codices/c/${codex.id}/inbox`);
    await page.locator('.note').getByRole('button', { name: 'New page' }).click();
    await expect(page.getByLabel('Title')).toHaveValue('E2E inbox idea x');
    await expect(page.locator('.cx-np-note')).toHaveText('E2E inbox idea [x]\nsecond line');
    await page.getByRole('button', { name: 'Create page' }).click();
    await page.waitForURL(/\/edit$/);
    const id = page.url().match(/\/p\/([^/]+)\//)[1];
    pages.push(id);
    const created = await (await editor.get(`/api/codices/pages/${id}`)).json();
    expect(created.page.title).toBe('E2E inbox idea x');
    expect(created.page.body).toBe('E2E inbox idea [x]\nsecond line');
    expect((await (await editor.get(`/api/codices/c/${codex.id}/notes`)).json()).notes).toEqual([]);
    // Deleting a note has an Undo that brings it back.
    await addNote(editor, 'E2E to delete');
    await page.goto(`/codices/c/${codex.id}/inbox`);
    await page.getByRole('button', { name: 'Delete' }).click();
    await expect(page.locator('.note')).toHaveCount(0);
    await page.getByRole('button', { name: 'Undo' }).click();
    await expect(page.locator('.note-t')).toHaveText('E2E to delete');
  });

  test('a note added to an existing page is a new revision with a signed paragraph', async ({ page }) => {
    const res = await editor.post('/api/codices/pages', { data: { title: 'E2E inbox target', codex_id: codex.id, body: 'Hello there.' } });
    expect(res.status(), 'create page').toBe(201);
    const id = (await res.json()).page.id;
    pages.push(id);
    await addNote(editor, 'E2E appended thought');
    await loginAs(page, 'roy');
    await page.goto(`/codices/c/${codex.id}/inbox`);
    await page.getByRole('button', { name: 'Add to page…' }).click();
    await page.locator('#dApPick').fill('E2E inbox target');
    await page.getByRole('option', { name: /E2E inbox target/ }).click();
    await expect(page.locator('.cx-np-note')).toContainText('E2E appended thought');
    await page.getByRole('button', { name: 'Add to page', exact: true }).click();
    await expect(page.getByRole('status')).toContainText('Added to E2E inbox target.');
    await expect(page.locator('.note')).toHaveCount(0);

    const after = await (await editor.get(`/api/codices/pages/${id}`)).json();
    expect(after.page.current_revision).toBe(2);
    expect(after.page.body).toMatch(/^Hello there\.\n\n_[A-Z][a-z]{2} \d{1,2}, \d{4} · .+_\nE2E appended thought$/);
    const revs = await (await editor.get(`/api/codices/pages/${id}/revisions`)).json();
    expect(revs.revisions[0].note).toBe('Added a quick note');
    // Filing it again is a 404: the note is gone.
    expect((await editor.post(`/api/codices/notes/none/append`, { data: { page_id: id } })).status()).toBe(404);
  });

  test('a see-only user gets no note box and a 403 on POST', async ({ page }) => {
    await loginAs(page, 'adam');
    await page.goto(`/codices/c/${codex.id}`);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(codex.name);
    await expect(page.locator('.cx-note')).toBeHidden();
    await page.keyboard.press('n');
    await expect(page.locator('#dNote')).toBeHidden();
    const post = await page.request.post(`/api/codices/c/${codex.id}/notes`, { data: { body: 'nope' } });
    expect(post.status()).toBe(403);
    expect((await page.request.get(`/api/codices/c/${codex.id}/notes`)).status()).toBe(200);
  });

  test('notes are private: others do not see, change or delete them', async () => {
    const note = await addNote(editor, 'E2E private note');
    const other = await apiAs('testy');
    expect((await (await other.get(`/api/codices/c/${codex.id}/notes`)).json()).notes).toEqual([]);
    expect((await other.delete(`/api/codices/notes/${note.id}`)).status()).toBe(404);
    expect((await other.put(`/api/codices/notes/${note.id}`, { data: { body: 'mine now' } })).status()).toBe(404);
    expect((await other.post(`/api/codices/notes/${note.id}/append`, { data: { page_id: 'x' } })).status()).toBe(404);
    await other.dispose();
    const mine = (await (await editor.get(`/api/codices/c/${codex.id}/notes`)).json()).notes;
    expect(mine.map((n) => n.body)).toEqual(['E2E private note']);
    expect((await editor.post(`/api/codices/c/${codex.id}/notes`, { data: { body: '   ' } })).status()).toBe(400);
  });

  test('the game page offers the note box when its codex is editable', async ({ page }) => {
    const gameId = await helpers.sojournId(editor);
    await loginAs(page, 'roy');
    await page.goto(`/games/${gameId}`);
    await expect(page.getByRole('button', { name: 'Quick note' }).first()).toBeVisible();
    await page.keyboard.press('n');
    await page.locator('#noteText').fill('E2E from the game');
    await page.locator('#noteText').press('Control+Enter');
    await expect(page.getByRole('status')).toContainText('In your inbox.');
    const { notes } = await (await editor.get(`/api/codices/c/${codex.id}/notes`)).json();
    expect(notes.map((n) => n.body)).toContain('E2E from the game');
  });
});

test.describe('Codices link from selection', () => {
  let editor;
  let codex;
  const ids = [];

  test.beforeEach(async () => {
    editor = await apiAs('roy');
    const home = await (await editor.get('/api/codices')).json();
    codex = home.codices.find((c) => c.can_edit);
    expect(codex, 'roy can edit a codex').toBeTruthy();
  });

  test.afterEach(async () => {
    for (const id of ids.splice(0)) await editor.delete(`/api/codices/pages/${id}`);
    await editor.dispose();
  });

  async function makePage(title) {
    const res = await editor.post('/api/codices/pages', { data: { title, codex_id: codex.id } });
    expect(res.status(), 'create ' + title).toBe(201);
    const id = (await res.json()).page.id;
    ids.push(id);
    return id;
  }

  const editUrl = (id) => `/codices/p/${id}/x/edit`;

  test('Ctrl+K links the selection to a page by title, another case or alias', async ({ page }) => {
    const target = await makePage('E2E Link Target');
    const aliased = await editor.put(`/api/codices/pages/${target}/aliases`, { data: { aliases: ['E2E Nick'] } });
    expect(aliased.ok(), 'set alias').toBeTruthy();
    const host = await makePage('E2E Link Host');
    await loginAs(page, 'roy');
    await page.goto(editUrl(host));
    const content = page.locator('.cm-content');
    await content.click();
    // One word group per line; each is selected from the line start and linked.
    await page.keyboard.type('E2E Nick\ne2e link target\nE2E Link Target\nE2E Unwritten');
    const linkLine = async (up, text, viaButton) => {
      await page.keyboard.press('Control+Home');
      for (let i = 0; i < up; i++) await page.keyboard.press('ArrowDown');
      await page.keyboard.press('Home');
      await page.keyboard.down('Shift');
      for (let i = 0; i < text.length; i++) await page.keyboard.press('ArrowRight');
      await page.keyboard.up('Shift');
      if (viaButton) await page.getByRole('button', { name: 'Link', exact: true }).click();
      else await page.keyboard.press('Control+k');
    };
    await linkLine(0, 'E2E Nick');
    await linkLine(1, 'e2e link target', true);
    await linkLine(2, 'E2E Link Target');
    await linkLine(3, 'E2E Unwritten');
    const lines = (await content.innerText()).split('\n').filter((l) => l.trim());
    expect(lines).toEqual(['[[E2E Link Target|E2E Nick]]', '[[E2E Link Target|e2e link target]]', '[[E2E Link Target]]', '[[E2E Unwritten]]']);
  });

  test('after saving, the missing pages dialog creates only the ticked ones from their template', async ({ page }) => {
    const host = await makePage('E2E Missing Host');
    await loginAs(page, 'roy');
    await page.goto(editUrl(host));
    await page.locator('.cm-content').click();
    await page.keyboard.type('Meet [[E2E Born Here]] and [[E2E Not Born]].');
    await page.getByRole('button', { name: 'Save changes' }).click();
    const dlg = page.getByRole('dialog');
    await expect(dlg.getByRole('heading', { name: 'Create these 2 pages?' })).toBeVisible();
    await expect(dlg.getByLabel('Create E2E Born Here')).toBeChecked();
    await dlg.getByLabel('Create E2E Not Born').uncheck();
    await dlg.getByLabel('Template for E2E Born Here').selectOption({ label: 'Character' });
    await dlg.getByRole('button', { name: 'Save and create' }).click();
    await page.waitForURL(/created=1/);

    const list = await (await editor.get(`/api/codices/c/${codex.id}`)).json();
    const born = list.pages.find((p) => p.title === 'E2E Born Here');
    expect(born, 'ticked page exists').toBeTruthy();
    ids.push(born.id);
    expect(list.pages.some((p) => p.title === 'E2E Not Born')).toBe(false);
    const full = await (await editor.get(`/api/codices/pages/${born.id}`)).json();
    const builtin = await (await editor.get('/api/codices/templates/builtin')).json();
    const character = builtin.templates.find((t) => t.id === 'builtin:character');
    expect(character.body.trim().length).toBeGreaterThan(0);
    expect(full.page.body.trim()).toBe(character.body.split('{{title}}').join('E2E Born Here').split('{{date}}').join(builtin.today).trim());
  });

  test('a save with no new missing links goes straight to the page', async ({ page }) => {
    await makePage('E2E Exists Already');
    const host = await makePage('E2E Plain Host');
    await loginAs(page, 'roy');
    await page.goto(editUrl(host));
    await page.locator('.cm-content').click();
    await page.keyboard.type('See [[E2E Exists Already]] and a picture ![[nothing.png]].');
    await page.getByRole('button', { name: 'Save changes' }).click();
    await page.waitForURL(/saved=/);
    await expect(page.getByRole('heading', { name: /Create (this|these)/ })).toHaveCount(0);
  });
});

test.describe('Codices / menu', () => {
  let editor;
  let codex;
  const ids = [];

  test.beforeEach(async () => {
    editor = await apiAs('roy');
    const home = await (await editor.get('/api/codices')).json();
    codex = home.codices.find((c) => c.can_edit);
    expect(codex, 'roy can edit a codex').toBeTruthy();
  });

  test.afterEach(async () => {
    for (const id of ids.splice(0)) await editor.delete(`/api/codices/pages/${id}`);
    await editor.dispose();
  });

  async function makePage(title, body) {
    const res = await editor.post('/api/codices/pages', { data: { title, codex_id: codex.id, body: body || '' } });
    expect(res.status(), 'create ' + title).toBe(201);
    const id = (await res.json()).page.id;
    ids.push(id);
    return id;
  }

  test('/per then Enter inserts the Person block', async ({ page }) => {
    const id = await makePage('E2E Slash Person');
    await loginAs(page, 'roy');
    await page.goto(`/codices/p/${id}/x/edit`);
    const content = page.locator('.cm-content');
    await content.click();
    await page.keyboard.type('Intro');
    await page.keyboard.press('Enter');
    await page.keyboard.type('/per');
    await expect(page.locator('.cm-tooltip-autocomplete')).toBeVisible();
    await page.waitForTimeout(200); // the popup ignores Enter for its first 75 ms
    await page.keyboard.press('Enter');
    await page.keyboard.type('Mira');
    const lines = (await content.innerText()).split('\n').map((l) => l.trim()).filter(Boolean);
    expect(lines).toEqual(['Intro', '**Name:** Mira', '**Role:**', '**Wants:**', '**Lives in:** [[ ]]']);
  });

  test('/Session then Enter inserts that template headings', async ({ page }) => {
    const id = await makePage('E2E Slash Session');
    await loginAs(page, 'roy');
    await page.goto(`/codices/p/${id}/x/edit`);
    const content = page.locator('.cm-content');
    await content.click();
    await page.keyboard.type('/Session');
    await expect(page.locator('.cm-tooltip-autocomplete')).toContainText('Session headings');
    await page.waitForTimeout(200); // the popup ignores Enter for its first 75 ms
    await page.keyboard.press('Enter');
    const text = await content.innerText();
    expect(text).toContain('## Summary');
    expect(text).toContain('## What happened');
    expect(text).not.toContain('/Session');
    expect(text).not.toContain('Notes from one game session');
  });

  test('URLs and and/or open no slash menu, and the Insert / button does', async ({ page }) => {
    const id = await makePage('E2E Slash Quiet');
    await loginAs(page, 'roy');
    await page.goto(`/codices/p/${id}/x/edit`);
    const content = page.locator('.cm-content');
    await content.click();
    await page.keyboard.type('See https://example.com/x and and/or then');
    await page.waitForTimeout(300);
    await expect(page.locator('.cm-tooltip-autocomplete')).toHaveCount(0);
    await page.getByRole('button', { name: 'Insert /' }).click();
    await expect(page.locator('.cm-tooltip-autocomplete')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.locator('.cm-tooltip-autocomplete')).toHaveCount(0);
  });

  test('a line that is only --- renders as a rule', async ({ page }) => {
    const nl = String.fromCharCode(10);
    const id = await makePage('E2E Slash Divider', ['Above', '---', 'Below'].join(nl));
    await loginAs(page, 'roy');
    await page.goto(`/codices/p/${id}/x`);
    await expect(page.locator('.codex-text hr')).toHaveCount(1);
    await expect(page.locator('.codex-text')).not.toContainText('---');
  });
});
