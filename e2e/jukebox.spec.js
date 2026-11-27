// @ts-check
const base = require('./helpers');
const { expect, loginAs, apiAs } = base;
const env = require('./env');

// Chromium's fake capture device is a beeping tone, so the DM can broadcast an "audio input"
// without a real microphone or a permission prompt. The test checks states, not the sound.
base.test.use({
  launchOptions: {
    args: ['--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream', '--autoplay-policy=no-user-gesture-required'],
  },
});

/** An Ongoing game of its own: testy is the DM, adam a player. Deleted at teardown. */
const test = base.test.extend({
  jgame: async ({}, use) => {
    const testy = await apiAs('testy');
    const adam = await apiAs('adam');
    const admin = await apiAs(env.ADMIN_USERNAME);
    const adamId = (await (await adam.get('/auth/current-user')).json()).user_id;
    const title = `Jukebox E2E ${Date.now()}`;
    const res = await testy.post('/api/campaigns', {
      data: { title, summary: 'A game for the jukebox test', description: 'Test game', type: 'Campaign' },
    });
    expect(res.status(), 'create game').toBe(201);
    const id = (await res.json()).campaign_id;
    expect((await adam.post(`/api/campaigns/${id}/signups`)).status(), 'adam signs up').toBe(201);
    expect((await testy.put(`/api/campaigns/${id}/signups/${adamId}`, { data: { status: 'Accepted' } })).status(), 'accept adam').toBe(200);
    expect((await testy.put(`/api/campaigns/${id}/status`, { data: { status: 'Ongoing' } })).status(), 'set Ongoing').toBe(200);
    await use({ id, title, testy, adam, adamId });
    await admin.delete(`/api/campaigns/${id}`);
    await Promise.all([testy, adam, admin].map((a) => a.dispose()));
  },
});

test('the DM broadcasts, a player listens from the header, and loses access when kicked', async ({ browser, jgame }) => {
  const dmCtx = await browser.newContext({ permissions: ['microphone'] });
  const playerCtx = await browser.newContext();
  try {
    // The DM starts the jukebox from the game page; it opens in a pop-up.
    const dmPage = await dmCtx.newPage();
    await loginAs(dmPage, 'testy');
    await dmPage.goto(`/games/${jgame.id}`);
    const [dmPop] = await Promise.all([
      dmPage.waitForEvent('popup'),
      dmPage.getByRole('button', { name: 'Start jukebox' }).click(),
    ]);
    await dmPop.waitForLoadState();
    await dmPop.getByText('Audio input', { exact: true }).click();
    await dmPop.getByRole('button', { name: 'Start', exact: true }).click();
    await expect(dmPop.getByRole('button', { name: 'Stop' })).toBeVisible();
    // The game counts as live once the server has the stream's first segment.
    await expect.poll(async () => (await (await jgame.adam.get('/api/jukebox/live')).json()).live.map((g) => g.campaign_id), { timeout: 15_000 }).toContain(jgame.id);

    // The player sees the header jukebox light up on another page and listens from its menu.
    const playerPage = await playerCtx.newPage();
    await loginAs(playerPage, 'adam');
    await playerPage.goto('/campaigns');
    const headerBtn = playerPage.getByRole('button', { name: 'Jukebox' });
    await expect(headerBtn).toBeVisible({ timeout: 15_000 });
    await headerBtn.click();
    const row = playerPage.locator('.jukebox').filter({ hasText: jgame.title });
    await expect(row).toBeVisible();
    const [pop] = await Promise.all([
      playerPage.waitForEvent('popup'),
      row.getByRole('button', { name: 'Listen' }).first().click(),
    ]);
    await pop.waitForLoadState();
    await expect(pop.getByText('Live', { exact: false }).first()).toBeVisible({ timeout: 15_000 });

    // Sound actually arrives: the listener's audio element plays forward.
    await expect.poll(() => pop.evaluate(() => document.querySelector('audio')?.currentTime || 0), { timeout: 15_000 }).toBeGreaterThan(0.5);

    // The DM sees who's listening.
    await expect(dmPop.getByText('Adam', { exact: false }).first()).toBeVisible();

    // Mute shows Paused to the listener.
    await dmPop.getByRole('button', { name: 'Mute' }).click();
    await expect(pop.getByText('Paused', { exact: false }).first()).toBeVisible();

    // Kicking the player cuts them off at once.
    expect((await jgame.testy.delete(`/api/campaigns/${jgame.id}/players/${jgame.adamId}`)).status(), 'kick adam').toBeLessThan(300);
    await expect(pop.getByText('You no longer have access to this jukebox')).toBeVisible();
  } finally {
    await dmCtx.close();
    await playerCtx.close();
  }
});
