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
    const select = page.getByLabel('Channel for Age of Sojourn', { exact: true });
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

// Discord events: the admin turns them on for Age of Sojourn (voice channel and cover set), the DM
// confirms a session, and the bot creates the event before the channel post that links to it.
// Cancelling deletes the event. Starting and ending by the minute ticker is checked by hand.
const VOICE = '700000000000000003';
// A 1x1 PNG
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==', 'base64');

test('discord events: confirm creates an event, cancel deletes it', async ({ page }) => {
  const admin = await apiAs(env.ADMIN_USERNAME);
  const id = await sojournId(admin);
  let sessionId = '';

  try {
    expect((await admin.put('/api/admin/discord', { data: { guild_id: env.DISCORD_GUILD, invite_url: '', events: ALL_EVENTS } })).ok()).toBeTruthy();
    expect((await admin.put(`/api/admin/discord/games/${id}`, { data: { channel_id: CHANNEL } })).ok()).toBeTruthy();

    // Events can't go on without a voice channel.
    const refused = await admin.put(`/api/admin/discord/games/${id}`, { data: { events: true } });
    expect(refused.status()).toBe(400);
    expect((await refused.json()).field).toBe('voice_channel_id');

    const cover = await admin.post(`/api/admin/discord/games/${id}/cover`, { data: PNG, headers: { 'Content-Type': 'image/png' } });
    expect(cover.ok()).toBeTruthy();

    // The admin picks the voice channel and turns events on in the page.
    await loginAs(page, env.ADMIN_USERNAME);
    await page.goto('/admin/discord');
    const row = page.locator('tr[data-game="Age of Sojourn"]');
    await expect(row.locator('.dc-cover img')).toBeVisible();
    let saved = page.waitForResponse((r) => r.url().includes('/api/admin/discord/games/') && r.request().method() === 'PUT');
    await row.getByLabel('Voice channel for Age of Sojourn').selectOption(VOICE);
    expect((await saved).ok()).toBeTruthy();
    saved = page.waitForResponse((r) => r.url().includes('/api/admin/discord/games/') && r.request().method() === 'PUT');
    await row.getByLabel('Discord events for Age of Sojourn').check();
    expect((await saved).ok()).toBeTruthy();

    // The game page says so.
    await page.goto(`/games/${id}`);
    await expect(page.getByText('Discord: posts to #game-night · events in 🔊Voice')).toBeVisible();

    // Confirm a session.
    await fetch(`${FAKE}/_messages`, { method: 'DELETE' });
    await fetch(`${FAKE}/_events`, { method: 'DELETE' });
    const created = await admin.post(`/api/campaigns/${id}/sessions`, {
      data: { date: dateFromToday(4), time: '19:00', duration: 180, timezone: 'Europe/Prague' },
    });
    expect(created.status()).toBe(201);
    sessionId = (await created.json()).session.id;
    expect((await admin.put(`/api/campaigns/${id}/sessions/${sessionId}`, { data: { status: 'Confirmed' } })).ok()).toBeTruthy();

    await expect.poll(async () => (await (await fetch(`${FAKE}/_events`)).json()).length, { timeout: 10_000 }).toBe(1);
    const [event] = await (await fetch(`${FAKE}/_events`)).json();
    expect(event).toMatchObject({ guild_id: env.DISCORD_GUILD, channel_id: VOICE, name: 'Age of Sojourn', entity_type: 2, privacy_level: 2, status: 1 });
    expect(event.image).toMatch(/^data:image\/png;base64,/);
    expect(event.description).toContain(`/games/${id}/calendar?`);
    expect(new Date(event.scheduled_end_time) - new Date(event.scheduled_start_time)).toBe(180 * 60 * 1000);

    const link = `https://discord.com/events/${env.DISCORD_GUILD}/${event.id}`;
    await expect
      .poll(async () => (await (await fetch(`${FAKE}/_messages`)).json()).filter((m) => m.channel_id === CHANNEL).map((m) => m.content), { timeout: 10_000 })
      .toEqual(expect.arrayContaining([expect.stringMatching(/session confirmed/)]));
    const post = (await (await fetch(`${FAKE}/_messages`)).json()).find((m) => m.channel_id === CHANNEL && m.content.includes('session confirmed'));
    expect(post.content.split('\n')[1]).toBe(link);

    // Cancelling deletes it.
    expect((await admin.put(`/api/campaigns/${id}/sessions/${sessionId}`, { data: { status: 'Cancelled' } })).ok()).toBeTruthy();
    await expect.poll(async () => (await (await fetch(`${FAKE}/_events`)).json())[0].deleted, { timeout: 10_000 }).toBe(true);
  } finally {
    if (sessionId) await admin.delete(`/api/campaigns/${id}/sessions/${sessionId}`);
    await admin.put(`/api/admin/discord/games/${id}`, { data: { events: false, voice_channel_id: '', channel_id: '' } });
    await admin.delete(`/api/admin/discord/games/${id}/cover`);
    await admin.put('/api/admin/discord', { data: { guild_id: '', invite_url: '', events: ALL_EVENTS } });
    await admin.dispose();
  }
});
