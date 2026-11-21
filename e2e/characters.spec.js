// @ts-check
const base = require('./helpers');
const { expect, loginAs, apiAs } = base;
const env = require('./env');

/**
 * testy runs a D&D 5e game of its own, roy signs up and is accepted. The game is deleted at teardown
 * (which also deletes its assignments), then roy's character.
 */
const test = base.test.extend({
  world: async ({}, use) => {
    const testy = await apiAs('testy');
    const roy = await apiAs('roy');
    const admin = await apiAs(env.ADMIN_USERNAME);
    const world = { id: '', characterIds: /** @type {string[]} */ ([]) };
    try {
      const royId = (await (await roy.get('/auth/current-user')).json()).user_id;
      const res = await testy.post('/api/campaigns', {
        data: { title: `Characters E2E ${Date.now()}`, summary: 'A game for the characters test', description: 'Test game', type: 'Campaign', system: 'D&D 5e' },
      });
      expect(res.status(), 'create game').toBe(201);
      world.id = (await res.json()).campaign_id;
      expect((await roy.post(`/api/campaigns/${world.id}/signups`)).status(), 'roy signs up').toBe(201);
      expect((await testy.put(`/api/campaigns/${world.id}/signups/${royId}`, { data: { status: 'Accepted' } })).status(), 'accept roy').toBe(200);
      await use(world);
    } finally {
      if (world.id) expect((await admin.delete(`/api/campaigns/${world.id}`)).ok(), 'delete game').toBe(true);
      for (const cid of world.characterIds) expect((await roy.delete(`/api/characters/${cid}`)).ok(), 'delete character').toBe(true);
      await Promise.all([testy.dispose(), roy.dispose(), admin.dispose()]);
    }
  },
});

test('create a character, choose it on the game page, then see the badge and the company entry', async ({ page, world }) => {
  const name = `Kael E2E ${Date.now()}`;
  await loginAs(page, 'roy');

  // Create the character on /characters.
  await page.goto('/characters');
  await page.getByRole('button', { name: '+ New character' }).click();
  await page.locator('#newName').fill(name);
  await page.locator('#newSystem').selectOption({ label: 'D&D 5e' });
  await page.getByRole('button', { name: 'Create' }).click();
  const row = page.locator('.char-row', { hasText: name });
  await expect(row).toBeVisible();
  const href = await row.locator('.char-name').getAttribute('href');
  const characterId = /** @type {string} */ (href).split('/').pop();
  world.characterIds.push(/** @type {string} */ (characterId));

  // Roy is an accepted player without a character: the game page nudges, and the picker takes it.
  await page.goto(`/games/${world.id}`);
  await expect(page.locator('.pending-notice')).toContainText('Choose your character');
  await page.getByRole('button', { name: 'Choose your character' }).click();
  const picker = page.locator('#pickerModal');
  await expect(picker).toBeVisible();
  await picker.locator('.picker-option', { hasText: name }).click();
  await picker.getByRole('button', { name: 'Confirm' }).click();

  // The company panel shows "as <name>", linking to the character.
  const link = page.locator('.roster-character a', { hasText: name });
  await expect(link).toBeVisible();
  await expect(link).toHaveAttribute('href', `/characters/${characterId}`);
  await expect(page.locator('.pending-notice')).toBeHidden();

  // The character page carries the game's badge.
  await page.goto(`/characters/${characterId}`);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(name);
  await expect(page.locator('.char-game').first()).toHaveAttribute('href', `/games/${world.id}`);
});

test('a character links to its Codices page, and only Codices links are accepted', async ({ page, world }, testInfo) => {
  const origin = /** @type {string} */ (testInfo.project.use.baseURL);
  const roy = await apiAs('roy');
  try {
    const systems = await (await roy.get('/api/systems')).json();
    const systemId = systems.find((/** @type {{name: string}} */ s) => s.name === 'D&D 5e').id;
    const res = await roy.post('/api/characters', { data: { name: `Codex E2E ${Date.now()}`, system_id: systemId } });
    expect(res.status()).toBe(201);
    const id = (await res.json()).character_id;
    world.characterIds.push(id);
    const put = (/** @type {string} */ codexUrl) => roy.put(`/api/characters/${id}`, { data: { name: 'Codex E2E', system_id: systemId, codex_url: codexUrl } });

    for (const bad of ['https://evil.example/codices/x', 'https://waystone.aravill.org.evil.com/codices/x', 'http://waystone.aravill.org/codices/x', 'https://waystone.aravill.org/games/x', 'javascript:alert(1)']) {
      const r = await put(bad);
      expect(r.status(), bad).toBe(400);
      expect((await r.json()).error).toContain('Codices page');
    }

    // The owner sets it in the edit form; the page then shows a button to it.
    await loginAs(page, 'roy');
    await page.goto(`/characters/${id}`);
    await expect(page.locator('.char-codex-link')).toHaveCount(0);
    await page.getByRole('button', { name: 'Edit' }).click();
    await page.locator('#editCodex').fill('https://evil.example/codices/x');
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.locator('.game-form-error')).toContainText('Codices page');
    await page.locator('#editCodex').fill(`${origin}/codices/c/x`);
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.locator('.char-codex-link')).toHaveAttribute('href', `${origin}/codices/c/x`);

    // Someone else cannot change it.
    const testy = await apiAs('testy');
    expect((await testy.put(`/api/characters/${id}`, { data: { name: 'x', system_id: systemId, codex_url: '' } })).status()).toBe(403);
    await testy.dispose();

    // Clearing removes the button.
    expect((await put('')).status()).toBe(200);
    await page.reload();
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
    await expect(page.locator('.char-codex-link')).toHaveCount(0);
  } finally {
    await roy.dispose();
  }
});
