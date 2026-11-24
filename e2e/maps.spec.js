// @ts-check
const { test, expect, loginAs, apiAs } = require('./helpers');

const CODEX = '5b7e9d21-8c3a-4f6e-a1d4-7c2e9b0f3a61'; // the demo codex: Roy edits it, Adam only sees it

/** Clicks the map at fractions of its stage. */
async function clickMap(page, x, y) {
  const box = await page.locator('#cxMap').boundingBox();
  await page.mouse.click(box.x + box.width * x, box.y + box.height * y);
}
const tile = (page, name) => page.locator(`.mp-marker[aria-label^="${name}"] .mp-tile`);

test.describe('Codices maps', () => {
  let roy;
  let made = [];

  test.beforeEach(async () => {
    roy = await apiAs('roy');
    made = [];
  });

  test.afterEach(async () => {
    for (const id of made) await roy.delete(`/api/codices/pages/${id}`);
    await roy.dispose();
  });

  async function newMap(title, image) {
    const res = await roy.post('/api/codices/pages', { data: { title, codex_id: CODEX, kind: 'map' } });
    expect(res.status()).toBe(201);
    const id = (await res.json()).page.id;
    made.push(id);
    if (image) {
      const up = await roy.post(`/api/codices/pages/${id}/images`, { data: image, headers: { 'Content-Type': 'image/png' } });
      expect(up.status()).toBe(201);
    }
    return id;
  }

  test('a new map gets its image, pins, a move with undo, a scale and the ruler', async ({ page }) => {
    await loginAs(page, 'roy');
    await page.goto(`/codices/c/${CODEX}`);
    await page.getByRole('button', { name: '+ New page' }).first().click();
    await page.getByLabel('Title').fill('E2E Map');
    await page.getByRole('radio', { name: /^Map/ }).click();
    await page.getByRole('button', { name: 'Create page' }).click();

    // The empty map asks for its image.
    await expect(page.getByText("Add the map's image")).toBeVisible();
    made.push(page.url().split('/p/')[1].split('/')[0]);
    await page.locator('input[type=file][accept*="image/png"]').setInputFiles({ name: 'm.png', mimeType: 'image/png', buffer: await page.screenshot() });
    await expect(page.locator('.leaflet-image-layer')).toBeVisible();

    // A plain pin: name, description, icon and colour; its preview says who added it.
    await page.getByRole('button', { name: 'Add pin' }).click();
    await clickMap(page, 0.5, 0.5);
    await page.getByLabel('Name').fill('Goblin camp');
    await page.getByLabel('Description').fill('Thirty tents.');
    await page.getByRole('radio', { name: 'Enemy camp or base' }).click();
    await page.getByRole('radio', { name: 'Green' }).click();
    await page.locator('.modal-open .submit-btn').click();
    const card = page.locator('#cxMapCard');
    await expect(card).toContainText('Thirty tents.');
    await expect(card).toContainText('added by Roy');
    await expect(page.locator('.mp-marker .mp-label')).toHaveText('Goblin camp');
    await expect(page.locator('.mp-marker .mp-pin')).toHaveClass(/mp-c-green/);

    // Move with the keyboard, then undo it from the toast.
    const pinX = async () => (await (await page.request.get(`/api/codices/pages/${made[0]}/map`)).json()).map.pins[0].x;
    const x = await pinX();
    await card.getByRole('button', { name: 'Move' }).click();
    for (let i = 0; i < 5; i++) await page.keyboard.press('Shift+ArrowRight');
    await page.keyboard.press('Enter');
    await expect(page.locator('.message')).toContainText('Moved “Goblin camp”.');
    expect(await pinX()).toBeGreaterThan(x);
    await page.locator('.message').getByRole('button', { name: 'Undo' }).click();
    await expect(page.locator('.message')).toContainText('Undone.');
    expect(await pinX()).toBeCloseTo(x, 6);

    // No scale, no ruler. A line of a fifth of the width is 10 leagues, so half the width is 25.
    await expect(page.getByRole('button', { name: 'Ruler' })).toBeHidden();
    await page.getByRole('button', { name: 'More page actions' }).click();
    await page.getByRole('menuitem', { name: 'Set scale…' }).click();
    await clickMap(page, 0.2, 0.8);
    await clickMap(page, 0.4, 0.8);
    await page.getByRole('button', { name: 'Set distance' }).click();
    await page.getByLabel('Distance').fill('10');
    await page.getByLabel('Unit').fill('leagues');
    await page.locator('.modal-open .submit-btn').click();
    await expect(page.locator('.mp-scale')).toContainText('leagues');
    await page.getByRole('button', { name: 'Ruler' }).click();
    await clickMap(page, 0.2, 0.3);
    await clickMap(page, 0.7, 0.3);
    await expect(page.locator('.mp-banner p')).toContainText(/Total 2(4\.9|5)\d? leagues|Total 25\.1 leagues/);
    await page.keyboard.press('Escape');
    await expect(page.locator('.mp-banner')).toBeHidden();

    // History lists the changes.
    await page.locator('.cx-actions').getByRole('link', { name: /History/ }).click();
    await expect(page.locator('.mp-hist li').first()).toContainText('Roy moved “Goblin camp”');
    await expect(page.locator('.mp-hist')).toHaveCount(2);
  });

  test('a linked pin shows its page, follows a rename and turns plain when the page goes', async ({ page }) => {
    await loginAs(page, 'roy');
    await page.goto('/');
    const map = await newMap('E2E Linked Map', await page.screenshot());
    const res = await roy.post('/api/codices/pages', { data: { title: 'E2E Harbour', codex_id: CODEX, kind: 'lore', body: 'Ships and gulls.' } });
    const target = (await res.json()).page.id;
    made.push(target);

    await page.goto(`/codices/p/${map}/x`);
    await page.getByRole('button', { name: 'Add pin' }).click();
    await clickMap(page, 0.4, 0.5);
    await page.locator('#dPinPick').fill('E2E Harb');
    await page.keyboard.press('Enter');
    await page.locator('.modal-open .submit-btn').click();
    const card = page.locator('#cxMapCard');
    await expect(card).toContainText('Ships and gulls.');
    await card.getByRole('link', { name: 'Open page' }).click();

    // The page says which map it is on; pins are not backlinks.
    const onMap = page.locator('section[aria-label="On the map"] a');
    await expect(onMap).toHaveText('E2E Linked Map');
    await expect(page.locator('section[aria-label="Linked from"]')).not.toContainText('E2E Linked Map');
    await onMap.click();
    await expect(card).toContainText('E2E Harbour');

    await roy.put(`/api/codices/pages/${target}/title`, { data: { title: 'E2E Old Harbour' } });
    await page.reload();
    await expect(page.locator('.mp-marker .mp-label')).toHaveText('E2E Old Harbour');

    await roy.delete(`/api/codices/pages/${target}`);
    await page.reload();
    await tile(page, 'E2E Old Harbour').click();
    await expect(card).toContainText('E2E Old Harbour');
    await expect(card.getByRole('link', { name: 'Open page' })).toBeHidden();
  });

  test('a see-only user looks, measures and filters but changes nothing', async ({ page }) => {
    await loginAs(page, 'adam');
    await page.goto(`/codices/c/${CODEX}`);
    await page.getByRole('link', { name: /Map of the Karrow/ }).first().click();
    await tile(page, 'Karrow Ford').click();
    const card = page.locator('#cxMapCard');
    await expect(card.getByRole('link', { name: 'Open page' })).toBeVisible();
    await expect(card.getByRole('button', { name: 'Edit' })).toBeHidden();
    await expect(page.getByRole('button', { name: 'Add pin' })).toBeHidden();
    await expect(page.getByRole('button', { name: 'Ruler' })).toBeVisible();
    await page.keyboard.press('Escape');

    await page.getByRole('button', { name: /^Dragon/ }).click();
    await expect(tile(page, 'Hollow Tor')).toHaveCount(0);
    await page.locator('.mp-list').getByRole('button', { name: /Hollow Tor/ }).click();
    await expect(card).toContainText('Something large nests here.');

    const id = page.url().split('/p/')[1].split('/')[0];
    const res = await page.request.post(`/api/codices/pages/${id}/pins`, { data: { x: 0.5, y: 0.5, name: 'No', icon: 'place', color: 'red' } });
    expect(res.status()).toBe(403);
  });
});
