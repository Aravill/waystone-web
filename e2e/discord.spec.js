// Discord: a player links an account through the (fake) OAuth flow, the admin picks a channel
// for Age of Sojourn, and confirming a session posts to the channel and DMs the player.
const { test, expect, loginAs, apiAs, sojournId, dateFromToday } = require('./helpers');
const env = require('./env');

const FAKE = `http://localhost:${env.DISCORD_PORT}`;
const CHANNEL = '700000000000000001';
const DISCORD_USER = '800000000000000042';
const ALL_EVENTS = ['session_confirmed', 'session_cancelled', 'new_poll', 'session_reminder', 'signups_opened', 'status_changed'];

test('link, pick a channel, confirm a session: channel post and DM', async ({ page }) => {
  const admin = await apiAs(env.ADMIN_USERNAME);
  const testy = await apiAs('testy');
  const id = await sojournId(admin);
  let sessionId = '';

  try {
    // The player links through the OAuth round trip against the fake.
    const start = await testy.get('/auth/discord/start', { maxRedirects: 0 });
    expect(start.status()).toBe(302);
    const state = new URL(start.headers()['location']).searchParams.get('state');
    expect(state).toBeTruthy();
    const back = await testy.get(`/auth/discord/callback?code=${DISCORD_USER}&state=${state}`, { maxRedirects: 0 });
    expect(back.headers()['location']).toContain('discord=linked');
    const prof = await (await testy.get('/api/profile/discord')).json();
    expect(prof.linked).toBe(true);

    // The admin sets the server and picks the game's channel in the UI.
    await loginAs(page, env.ADMIN_USERNAME);
    await page.goto('/admin/discord');
    await page.locator('#dcGuild').fill(env.DISCORD_GUILD);
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await expect(page.locator('.message.success')).toBeVisible();
    const select = page.locator('tr[data-game="Age of Sojourn"] select');
    await expect(select.locator(`option[value="${CHANNEL}"]`)).toHaveCount(1);
    await expect(select.locator('option[value="700000000000000002"]')).toHaveCount(0);
    const saved = page.waitForResponse((r) => r.url().includes('/api/admin/discord/games/') && r.request().method() === 'PUT');
    await select.selectOption(CHANNEL);
    expect((await saved).ok()).toBeTruthy();
    expect((await (await testy.get('/api/profile/discord')).json()).in_guild).toBe(true);

    // Confirm a session.
    await fetch(`${FAKE}/_messages`, { method: 'DELETE' });
    const created = await admin.post(`/api/campaigns/${id}/sessions`, {
      data: { date: dateFromToday(3), time: '19:00', duration: 240, timezone: 'Europe/Prague' },
    });
    expect(created.status()).toBe(201);
    sessionId = (await created.json()).session.id;
    const confirm = await admin.put(`/api/campaigns/${id}/sessions/${sessionId}`, { data: { status: 'Confirmed' } });
    expect(confirm.ok()).toBeTruthy();

    await expect
      .poll(async () => (await (await fetch(`${FAKE}/_messages`)).json()).map((m) => m.channel_id + '|' + m.content), { timeout: 10_000 })
      .toEqual(expect.arrayContaining([
        expect.stringMatching(new RegExp(`^${CHANNEL}\\|.*Age of Sojourn.*session confirmed`)),
        expect.stringMatching(new RegExp(`^dm-${DISCORD_USER}\\|.*confirmed the session for`)),
      ]));

    // Both open the session on the calendar; the DM's button also carries the player's own notification.
    const sent = await (await fetch(`${FAKE}/_messages`)).json();
    const button = (re) => sent.find((m) => re.test(m.channel_id) && m.content.includes('confirmed')).components[0].components[0];
    const path = `http://localhost:${env.PORT}/games/${id}/calendar?date=${dateFromToday(3)}&session=${sessionId}`;
    expect(button(/^7/)).toEqual({ type: 2, style: 5, label: 'Open the session', url: path });
    const { notifications } = await (await testy.get('/api/notifications?limit=50')).json();
    const own = notifications.find((n) => n.session_id === sessionId && n.type === 'session_confirmed');
    expect(button(/^dm-/).url).toBe(`${path}&notif=${own.id}`);
    expect(own.url).toBe(button(/^dm-/).url.replace(/^https?:\/\/[^/]+/, ''));
  } finally {
    if (sessionId) await admin.delete(`/api/campaigns/${id}/sessions/${sessionId}`);
    await admin.put(`/api/admin/discord/games/${id}`, { data: { channel_id: '' } });
    await testy.delete('/api/profile/discord');
    await admin.put('/api/admin/discord', { data: { guild_id: '', invite_url: '', events: ALL_EVENTS } });
    await Promise.all([admin.dispose(), testy.dispose()]);
  }
});
