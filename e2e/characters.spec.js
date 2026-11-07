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
    const royId = (await (await roy.get('/auth/current-user')).json()).user_id;
    const res = await testy.post('/api/campaigns', {
      data: { title: `Characters E2E ${Date.now()}`, summary: 'A game for the characters test', description: 'Test game', type: 'Campaign', system: 'D&D 5e' },
    });
    expect(res.status(), 'create game').toBe(201);
    const id = (await res.json()).campaign_id;
    expect((await roy.post(`/api/campaigns/${id}/signups`)).status(), 'roy signs up').toBe(201);
    expect((await testy.put(`/api/campaigns/${id}/signups/${royId}`, { data: { status: 'Accepted' } })).status(), 'accept roy').toBe(200);

    const world = { id, characterIds: /** @type {string[]} */ ([]) };
    await use(world);

    await admin.delete(`/api/campaigns/${id}`);
    for (const cid of world.characterIds) await roy.delete(`/api/characters/${cid}`);
    await Promise.all([testy.dispose(), roy.dispose(), admin.dispose()]);
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
