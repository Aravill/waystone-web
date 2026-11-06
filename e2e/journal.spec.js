// @ts-check
const base = require('./helpers');
const { expect, loginAs, apiAs, dateFromToday } = base;
const env = require('./env');

const PEOPLE = ['testy', 'roy', 'adam', 'simon', env.ADMIN_USERNAME];

/**
 * Every test gets its own game, so none of them touches the seeded ones. testy is the DM, roy and
 * adam are players, simon is a logged-in visitor and the admin is a site admin who is not a member.
 * Sessions are recorded by hand (always played), which keeps the dates fixed. The game is deleted at teardown.
 */
const test = base.test.extend({
  world: async ({}, use) => {
    const api = {};
    const ids = {};
    for (const name of PEOPLE) {
      api[name] = await apiAs(name);
      ids[name] = (await (await api[name].get('/auth/current-user')).json()).user_id;
    }
    const res = await api.testy.post('/api/campaigns', {
      data: { title: `Journal E2E ${Date.now()}`, summary: 'A game for the journal tests', description: 'Test game', type: 'Campaign' },
    });
    expect(res.status(), 'create game').toBe(201);
    const id = (await res.json()).campaign_id;
    for (const player of ['roy', 'adam']) {
      expect((await api[player].post(`/api/campaigns/${id}/signups`)).status(), `${player} signs up`).toBe(201);
      expect((await api.testy.put(`/api/campaigns/${id}/signups/${ids[player]}`, { data: { status: 'Accepted' } })).status(), `accept ${player}`).toBe(200);
    }

    const world = {
      id,
      api,
      ids,
      /** Record a played session `daysAgo` days back. `start` '' records one with no time. */
      async played(daysAgo, { attendees = ['testy', 'roy', 'adam'], start = '19:00', end = '22:00' } = {}) {
        const r = await api.testy.post(`/api/campaigns/${id}/played-sessions`, {
          data: { date: dateFromToday(-daysAgo), start, end: start ? end : '', timezone: 'Europe/Prague', attendee_ids: attendees.map((a) => ids[a]), guests: [] },
        });
        expect(r.status(), 'record session').toBe(201);
        return (await r.json()).session;
      },
      /** A calendar session `daysFromNow` days away (negative: in the past), as the DM; Suggested unless confirmed. */
      async calendar(daysFromNow, { confirm = false } = {}) {
        const r = await api.testy.post(`/api/campaigns/${id}/sessions`, {
          data: { date: dateFromToday(daysFromNow), time: '18:00', duration: 120, timezone: 'Europe/Prague' },
        });
        expect(r.status(), 'create session').toBe(201);
        const session = (await r.json()).session;
        if (confirm) await world.setStatus(session, 'Confirmed');
        return session;
      },
      async setStatus(session, status) {
        const r = await api.testy.put(`/api/campaigns/${id}/sessions/${session.id}`, { data: { status } });
        expect(r.status(), `set ${status}`).toBe(200);
      },
      api_url: (sid, tail = '') => `/api/campaigns/${id}/journal/${sid}${tail}`,
      page_url: (sid, tail = '') => `/games/${id}/journal/${sid}${tail}`,
      /** PUT an entry as `user`. A new entry omits entryId/base. */
      save(user, sid, { entryId = '', base = 0, name = '', body, note = '', mergedWith = 0 }) {
        return api[user].put(world.api_url(sid), {
          data: { entry_id: entryId, base_revision: base, name, body, note, merged_with: mergedWith },
        });
      },
      /** Write an entry as `user` and return its entry id and revision number. */
      async write(user, sid, opts) {
        const r = await world.save(user, sid, opts);
        expect(r.ok(), `${user} saves: ${r.status()}`).toBeTruthy();
        const data = await r.json();
        return { entryId: data.entry_id, number: data.revision.number };
      },
      async entry(sid, user = 'testy') {
        const r = await api[user].get(world.api_url(sid));
        expect(r.status()).toBe(200);
        return r.json();
      },
      async revisions(sid, user = 'testy') {
        const r = await api[user].get(world.api_url(sid, '/revisions'));
        expect(r.status()).toBe(200);
        return r.json();
      },
    };
    await use(world);

    await api[env.ADMIN_USERNAME].delete(`/api/campaigns/${id}`); // also removes sessions and entries
    await Promise.all(Object.values(api).map((a) => a.dispose()));
  },
});

const PARAS = 'Alpha one.\n\nBeta two.\n\nGamma three.';
const heading = (page) => page.getByRole('heading', { level: 1 });
const message = (page) => page.locator('.journal-message');

test.describe('merging', () => {
  test('changes to different paragraphs merge on their own and record merged_with', async ({ page, world }) => {
    const s = await world.played(5);
    const first = await world.write('roy', s.id, { name: 'The Ascent', body: PARAS });
    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id, '/edit'));
    await expect(page.locator('#fBody')).toHaveValue(PARAS);

    // Adam saves revision 2 while Roy is still writing.
    await world.write('adam', s.id, { entryId: first.entryId, base: 1, name: 'The Ascent', body: 'Alpha one.\n\nBeta two.\n\nGamma three, as Adam saw it.' });

    await page.locator('#fBody').fill('Alpha one, as Roy saw it.\n\nBeta two.\n\nGamma three.');
    await page.getByRole('button', { name: 'Save changes' }).click();

    await expect(page).toHaveURL(world.page_url(s.id));
    await expect(message(page)).toHaveText(/Merged with Adam's changes and saved as revision 3/);
    await expect(page.locator('.game-text')).toContainText('as Roy saw it');
    await expect(page.locator('.game-text')).toContainText('as Adam saw it');

    const { revisions } = await world.revisions(s.id);
    expect(revisions.map((r) => r.number)).toEqual([1, 2, 3]);
    expect(revisions[2]).toMatchObject({ merged_with: 2, author_id: world.ids.roy });
    expect(revisions[2].body).toBe('Alpha one, as Roy saw it.\n\nBeta two.\n\nGamma three, as Adam saw it.');
  });

  test('the same paragraph and a different name open "Combine your changes", and the result saves', async ({ page, world }) => {
    const s = await world.played(5);
    const first = await world.write('roy', s.id, { name: 'The Ascent', body: PARAS });
    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id, '/edit'));
    await expect(page.locator('#fBody')).toHaveValue(PARAS);

    await world.write('adam', s.id, { entryId: first.entryId, base: 1, name: 'The Summit', body: 'Alpha one.\n\nBeta two, in Adam\'s words.\n\nGamma three.' });

    await page.locator('#fName').fill('The Climb');
    await page.locator('#fBody').fill('Alpha one.\n\nBeta two, in Roy\'s words.\n\nGamma three.');
    await page.getByRole('button', { name: 'Save changes' }).click();

    await expect(heading(page)).toHaveText('Combine your changes');
    await expect(page.locator('.merge-summary')).toContainText('2 of 2 conflicts still to decide');
    await expect(page.locator('.conflict-head').nth(0)).toContainText('Session name');
    await expect(page.locator('.conflict-head').nth(1)).toContainText('Paragraph');
    const save = page.getByRole('button', { name: 'Save merged entry' });
    await expect(save).toBeDisabled();

    await page.locator('.conflict').nth(0).getByRole('button', { name: 'Keep yours' }).click();
    await expect(save).toBeDisabled();
    await page.locator('.conflict').nth(1).getByRole('button', { name: "Keep Adam's" }).click();
    await expect(page.locator('.merge-summary')).toContainText('All conflicts decided');
    await expect(save).toBeEnabled();
    await expect(page.locator('.merge-result')).toContainText("in Adam's words");
    await save.click();

    await expect(page).toHaveURL(world.page_url(s.id));
    await expect(message(page)).toHaveText(/Merged with Adam's changes and saved as revision 3/);
    const e = (await world.entry(s.id)).entry;
    expect(e).toMatchObject({ name: 'The Climb', current_revision: 3 });
    expect(e.body).toBe("Alpha one.\n\nBeta two, in Adam's words.\n\nGamma three.");
    const { revisions } = await world.revisions(s.id);
    expect(revisions[2].merged_with).toBe(2);
  });

  test('a first draft that meets an existing entry offers "Keep both"', async ({ page, world }) => {
    const s = await world.played(5);
    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id, '/edit'));
    await page.locator('#fBody').fill('Roy wrote this.');

    await world.write('adam', s.id, { name: 'The Gate', body: 'Adam wrote this.' });
    await page.getByRole('button', { name: 'Save entry' }).click();

    await expect(heading(page)).toHaveText('Combine your changes');
    await page.getByRole('button', { name: 'Keep both' }).click();
    await page.getByRole('button', { name: 'Save merged entry' }).click();

    await expect(page).toHaveURL(world.page_url(s.id));
    const e = (await world.entry(s.id)).entry;
    expect(e).toMatchObject({ name: 'The Gate', body: 'Roy wrote this.\n\nAdam wrote this.', current_revision: 2 });
  });

  test('a stale base is refused with the latest revision', async ({ world }) => {
    const s = await world.played(5);
    const first = await world.write('roy', s.id, { body: PARAS });
    await world.write('adam', s.id, { entryId: first.entryId, base: 1, body: PARAS + '\n\nDelta four.' });

    const r = await world.save('roy', s.id, { entryId: first.entryId, base: 1, body: 'Something else.' });
    expect(r.status()).toBe(409);
    const data = await r.json();
    expect(data).toMatchObject({ code: 'stale_base', entry_id: first.entryId });
    expect(data.latest).toMatchObject({ number: 2, author_id: world.ids.adam, body: PARAS + '\n\nDelta four.' });
    expect(data.latest.author_user.display_name).toBe('Adam');

    // merged_with, when given, must name the base
    const bad = await world.save('roy', s.id, { entryId: first.entryId, base: 2, body: 'x', mergedWith: 1 });
    expect(bad.status()).toBe(400);
  });

  test('a merge too long to save keeps the merge screen open', async ({ page, world }) => {
    const s = await world.played(5);
    const first = await world.write('roy', s.id, { body: 'Alpha.' });
    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id, '/edit'));
    await expect(page.locator('#fBody')).toHaveValue('Alpha.');

    await world.write('adam', s.id, { entryId: first.entryId, base: 1, body: 'X'.repeat(12000) });
    await page.locator('#fBody').fill('Y'.repeat(12000));
    await page.getByRole('button', { name: 'Save changes' }).click();

    await expect(heading(page)).toHaveText('Combine your changes');
    await page.getByRole('button', { name: 'Combine by hand' }).click(); // starts from both versions: 24000 characters
    await page.getByRole('button', { name: 'Save merged entry' }).click();
    await expect(page.locator('.merge-notice')).toContainText('at most 20000 characters');
    await expect(heading(page)).toHaveText('Combine your changes');

    await page.locator('.conflict-text').fill('Short and sweet.');
    await page.getByRole('button', { name: 'Save merged entry' }).click();
    await expect(page).toHaveURL(world.page_url(s.id));
    expect((await world.entry(s.id)).entry.body).toBe('Short and sweet.');
  });
});

test.describe('the editor keeps the draft', () => {
  test('a player removed while writing sees a notice, keeps the text and cannot save', async ({ page, world }) => {
    const s = await world.played(5);
    await loginAs(page, 'adam');
    await page.goto(world.page_url(s.id, '/edit'));
    await page.locator('#fBody').fill('Adam was here.');
    expect((await world.api.testy.delete(`/api/campaigns/${world.id}/players/${world.ids.adam}`)).status()).toBe(200);

    await page.getByRole('button', { name: 'Save entry' }).click();
    await expect(page.getByRole('alert')).toContainText('no longer write this journal');
    await expect(page.locator('#fBody')).toHaveValue('Adam was here.');
    await expect(page.getByRole('button', { name: 'Save entry' })).toBeDisabled();
    expect((await world.api.adam.get(world.api_url(s.id))).status()).toBe(200);
  });

  test('a deleted game keeps the draft on the page', async ({ page, world }) => {
    const s = await world.played(5);
    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id, '/edit'));
    await page.locator('#fBody').fill('Roy was here.');
    expect((await world.api[env.ADMIN_USERNAME].delete(`/api/campaigns/${world.id}`)).status()).toBe(200);

    await page.getByRole('button', { name: 'Save entry' }).click();
    await expect(page.getByRole('alert')).toContainText('no longer exists');
    await expect(page.locator('#fBody')).toHaveValue('Roy was here.');
  });

  test('an expired login keeps the draft and does not leave the page', async ({ page, world }) => {
    const s = await world.played(5);
    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id, '/edit'));
    await page.locator('#fBody').fill('Written just before the cookie went.');
    await page.context().clearCookies();

    await page.getByRole('button', { name: 'Save entry' }).click();
    await expect(page.getByRole('alert')).toContainText('login has expired');
    await expect(page.locator('#fBody')).toHaveValue('Written just before the cookie went.');
    await expect(page).toHaveURL(world.page_url(s.id, '/edit'));
  });

  test('an entry deleted while editing keeps the text, and saving writes a new entry', async ({ page, world }) => {
    const s = await world.played(5);
    await world.write('adam', s.id, { body: PARAS });
    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id, '/edit'));
    await page.locator('#fBody').fill('Roy kept writing.');
    expect((await world.api.testy.delete(world.api_url(s.id))).status()).toBe(200);

    await page.getByRole('button', { name: 'Save changes' }).click();
    await expect(page.getByRole('alert')).toContainText('was deleted while you were editing');
    await expect(page.locator('#fBody')).toHaveValue('Roy kept writing.');

    await page.getByRole('button', { name: 'Save entry' }).click();
    await expect(page).toHaveURL(world.page_url(s.id));
    const e = (await world.entry(s.id)).entry;
    expect(e).toMatchObject({ author_id: world.ids.roy, body: 'Roy kept writing.', current_revision: 1 });
  });

  test('leaving with unsaved text asks first', async ({ page, world }) => {
    const s = await world.played(5);
    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id, '/edit'));
    await page.locator('#fBody').click();
    await page.keyboard.type('Not saved yet.');

    const dialogs = [];
    page.on('dialog', async (d) => {
      dialogs.push(d.type());
      await d.dismiss();
    });
    await page.close({ runBeforeUnload: true });
    await expect.poll(() => dialogs).toEqual(['beforeunload']);
  });
});

test.describe('permissions', () => {
  test('a visitor reads but cannot write, a player who missed the session can', async ({ page, world }) => {
    const s = await world.played(5, { attendees: ['testy', 'adam'] }); // roy was not there
    const { entryId } = await world.write('adam', s.id, { body: PARAS });

    const visitor = await world.entry(s.id, 'simon');
    expect(visitor).toMatchObject({ can_edit: false, can_delete: false });
    expect(visitor.entry.body).toBe(PARAS);
    expect((await world.revisions(s.id, 'simon')).revisions).toHaveLength(1);
    expect((await world.save('simon', s.id, { entryId, base: 1, body: 'x' })).status()).toBe(403);
    expect((await world.api.simon.post(world.api_url(s.id, '/revisions/1/restore'), { data: { base_revision: 1 } })).status()).toBe(403);
    expect((await world.api.simon.delete(world.api_url(s.id))).status()).toBe(403);

    const r = await world.save('roy', s.id, { entryId, base: 1, body: PARAS + '\n\nDelta four.' });
    expect(r.status()).toBe(201);

    await loginAs(page, 'simon');
    await page.goto(world.page_url(s.id));
    await expect(page.locator('.game-text')).toContainText('Delta four.');
    await expect(page.getByRole('link', { name: 'Edit', exact: true })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Delete' })).toHaveCount(0);
    await page.goto(world.page_url(s.id, '/edit'));
    await expect(page.getByText("Only the game's DM and players can write the journal.")).toBeVisible();
  });

  test('a removed player cannot edit but keeps the credit', async ({ world }) => {
    const s = await world.played(5);
    const { entryId } = await world.write('roy', s.id, { body: PARAS });
    await world.write('adam', s.id, { entryId, base: 1, body: PARAS + '\n\nDelta four.' });
    expect((await world.api.testy.delete(`/api/campaigns/${world.id}/players/${world.ids.adam}`)).status()).toBe(200);

    expect((await world.save('adam', s.id, { entryId, base: 2, body: 'More.' })).status()).toBe(403);
    expect((await world.api.adam.post(world.api_url(s.id, '/revisions/1/restore'), { data: { base_revision: 2 } })).status()).toBe(403);
    const e = (await world.entry(s.id, 'adam')).entry;
    expect(e.amender_users.map((u) => u.display_name)).toEqual(['Adam']);
    expect(e.scribes.map((c) => [c.user.display_name, c.revisions])).toEqual([['Roy', 1], ['Adam', 1]]);
    expect((await world.entry(s.id, 'adam')).can_edit).toBe(false);
  });

  test('the DM and an admin can delete an entry, a player cannot', async ({ world }) => {
    const s = await world.played(5);
    const body = { body: PARAS };
    await world.write('roy', s.id, body);

    expect((await world.api.roy.delete(world.api_url(s.id))).status()).toBe(403);
    expect((await world.entry(s.id, 'roy')).can_delete).toBe(false);
    expect((await world.entry(s.id, 'testy')).can_delete).toBe(true);
    expect((await world.api.testy.delete(world.api_url(s.id))).status()).toBe(200);
    expect((await world.entry(s.id)).entry).toBeNull();
    expect((await world.api.testy.delete(world.api_url(s.id))).status()).toBe(404);

    await world.write('roy', s.id, body);
    const admin = world.api[env.ADMIN_USERNAME];
    expect((await world.entry(s.id, env.ADMIN_USERNAME)).can_delete).toBe(true);
    expect((await admin.delete(world.api_url(s.id))).status()).toBe(200);
  });

  test('an admin who is not a member can read and delete, but not edit or restore', async ({ world }) => {
    const admin = world.api[env.ADMIN_USERNAME];
    const s = await world.played(5);
    const { entryId } = await world.write('roy', s.id, { body: PARAS });
    await world.write('roy', s.id, { entryId, base: 1, body: PARAS + '\n\nDelta four.' });

    expect(await world.entry(s.id, env.ADMIN_USERNAME)).toMatchObject({ can_edit: false, can_delete: true });
    expect((await world.save(env.ADMIN_USERNAME, s.id, { entryId, base: 2, body: 'x' })).status()).toBe(403);
    expect((await admin.post(world.api_url(s.id, '/revisions/1/restore'), { data: { base_revision: 2 } })).status()).toBe(403);
    expect((await world.revisions(s.id, env.ADMIN_USERNAME)).revisions).toHaveLength(2);
  });
});

test.describe('sessions that happened', () => {
  test('a Suggested or a future Confirmed session cannot have an entry, and offers no Write link', async ({ page, world }) => {
    const suggested = await world.calendar(30);
    const confirmed = await world.calendar(40, { confirm: true });
    await world.played(5);
    for (const s of [suggested, confirmed]) {
      const r = await world.save('roy', s.id, { body: PARAS });
      expect(r.status()).toBe(409);
      expect((await r.json()).code).toBe('session_not_played');
      expect(await world.entry(s.id, 'roy')).toMatchObject({ can_edit: false, entry: null, session: { played: false, number: null } });
    }

    await loginAs(page, 'roy');
    await page.goto(`/games/${world.id}`);
    await expect(page.locator('.game-row')).toHaveCount(1); // only the played session
    for (const s of [suggested, confirmed]) {
      await page.goto(world.page_url(s.id));
      await expect(page.getByText('This session has not been played, so it has no journal entry.')).toBeVisible();
      await expect(page.getByRole('link', { name: /Write the entry/ })).toHaveCount(0);
    }
  });

  test('a session recorded by hand, even without a time, can have an entry', async ({ page, world }) => {
    const s = await world.played(5, { start: '' });
    expect(s.time).toBe('');
    await world.write('roy', s.id, { name: 'No clock', body: PARAS });

    await loginAs(page, 'roy');
    await page.goto(`/games/${world.id}`);
    await expect(page.locator('.game-row').first().locator('.entry-link')).toHaveText('No clock');
    await expect(page.locator('.game-count')).toContainText('1 session played · 1 in the journal');
    await page.goto(world.page_url(s.id));
    await expect(heading(page)).toContainText('No clock');
  });

  test('cancelling a session with an entry leaves the entry read-only and reachable', async ({ page, world }) => {
    const s = await world.calendar(-3, { confirm: true });
    await world.played(9); // an older played session, so the cancelled one would have been number 2
    const { entryId } = await world.write('roy', s.id, { name: 'Lost to the weather', body: PARAS });
    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id, '/edit'));
    await page.locator('#fBody').fill(PARAS + '\n\nA late thought.');

    await world.setStatus(s, 'Cancelled');
    await page.getByRole('button', { name: 'Save changes' }).click();
    await expect(page.getByRole('alert')).toContainText('no longer marked as played');
    await expect(page.locator('#fBody')).toHaveValue(PARAS + '\n\nA late thought.');
    await expect(page.getByRole('button', { name: 'Save changes' })).toBeDisabled();

    expect((await world.save('roy', s.id, { entryId, base: 1, body: 'x' })).status()).toBe(409);
    expect(await world.entry(s.id, 'roy')).toMatchObject({ can_edit: false, session: { played: false }, entry: { name: 'Lost to the weather' } });

    await page.goto(`/games/${world.id}`);
    await expect(page.locator('.game-row')).toHaveCount(1);
    await expect(page.locator('.game-count')).toContainText('1 session played · 0 in the journal');
    const line = page.locator('.entry-cancelled');
    await expect(line).toContainText('Entries on cancelled sessions');
    await line.getByRole('link', { name: /Lost to the weather/ }).click();
    await expect(page.getByText('This session was cancelled, so its entry can be read but not changed.')).toBeVisible();
    await expect(page.getByRole('link', { name: 'Edit', exact: true })).toHaveCount(0);

    // the DM can still delete it
    expect((await world.api.testy.delete(world.api_url(s.id))).status()).toBe(200);
  });
});

test.describe('history', () => {
  test('restore adds a revision and keeps the others, and a no-change save adds none', async ({ page, world }) => {
    const s = await world.played(5);
    const { entryId } = await world.write('roy', s.id, { name: 'One', body: PARAS });
    await world.write('adam', s.id, { entryId, base: 1, name: 'Two', body: 'Totally different.' });

    // Restoring needs the current revision as its base
    const stale = await world.api.roy.post(world.api_url(s.id, '/revisions/1/restore'), { data: { base_revision: 1 } });
    expect(stale.status()).toBe(409);
    expect((await stale.json()).code).toBe('stale_base');

    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id, '/history'));
    await page.getByRole('button', { name: /^Revision 1/ }).click();
    await expect(page.getByText('This is the first version of the entry')).toBeVisible();
    await page.getByRole('button', { name: 'Restore this version' }).click();
    await expect(page.getByRole('dialog', { name: 'Restore revision 1?' })).toBeVisible();
    await page.getByRole('button', { name: 'Restore', exact: true }).click();
    await expect(message(page)).toHaveText(/Restored revision 1 as revision 3/);
    await expect(page.getByRole('button', { name: /^Revision 3/ })).toContainText('Restored revision 1');

    const { revisions } = await world.revisions(s.id);
    expect(revisions.map((r) => r.number)).toEqual([1, 2, 3]);
    expect(revisions[2]).toMatchObject({ restored_from: 1, name: 'One', body: PARAS, author_id: world.ids.roy });
    expect(revisions[1].body).toBe('Totally different.');

    // Saving what is already there, or restoring what is already there, adds nothing
    const same = await world.save('roy', s.id, { entryId, base: 3, name: 'One', body: PARAS.replace(/\n\n/g, '\r\n  \r\n\r\n') + '\n' });
    expect(same.status()).toBe(200);
    expect((await same.json()).created).toBe(false);
    const again = await world.api.roy.post(world.api_url(s.id, '/revisions/1/restore'), { data: { base_revision: 3 } });
    expect(again.status()).toBe(200);
    expect((await world.revisions(s.id)).revisions).toHaveLength(3);
  });

  test('only the latest 200 revisions are kept; numbering continues and credit survives', async ({ page, world }) => {
    test.setTimeout(120_000);
    const s = await world.played(5);
    const { entryId } = await world.write('roy', s.id, { name: 'Long story', body: 'Revision 1' });
    await world.write('adam', s.id, { entryId, base: 1, name: 'Long story', body: 'Revision 2' });
    for (let n = 3; n <= 212; n++) {
      const r = await world.save('roy', s.id, { entryId, base: n - 1, name: 'Long story', body: `Revision ${n}` });
      expect(r.status(), `revision ${n}`).toBe(201);
    }

    const { revisions, pruned, current_revision } = await world.revisions(s.id);
    expect(pruned).toBe(true);
    expect(current_revision).toBe(212);
    expect(revisions).toHaveLength(200);
    expect(revisions[0].number).toBe(13);
    expect(revisions.at(-1)).toMatchObject({ number: 212, body: 'Revision 212' });

    // Revisions 1 (Roy's, the author's) and 2 (Adam's only one) are gone, yet both are credited
    const e = (await world.entry(s.id)).entry;
    expect(e).toMatchObject({ current_revision: 212, revision_count: 200, author_id: world.ids.roy });
    expect(e.amender_users.map((u) => u.display_name)).toEqual(['Adam']);
    expect(Object.fromEntries(e.scribes.map((c) => [c.user.display_name, c.revisions]))).toEqual({ Roy: 211, Adam: 1 });

    // Saving still works, and a stale client whose base was pruned is told to merge
    expect((await world.save('adam', s.id, { entryId, base: 212, name: 'Long story', body: 'Revision 213' })).status()).toBe(201);
    expect((await world.save('adam', s.id, { entryId, base: 2, body: 'From a very old base' })).status()).toBe(409);

    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id, '/history'));
    await expect(page.getByText('Only the latest 200 revisions are kept.')).toBeVisible();
    await page.getByRole('button', { name: /^Revision 15 / }).click();
    await expect(page.locator('.detail')).toContainText('Against revision 14');
    await page.getByRole('button', { name: /^Revision 14 / }).click(); // the oldest kept revision has nothing before it
    await expect(page.locator('.detail')).toContainText('the oldest revision still kept');
  });
});

test.describe('deleting', () => {
  test('deleting an entry leaves the session, and someone can write a new one', async ({ page, world }) => {
    const s = await world.played(5);
    await world.write('roy', s.id, { name: 'Gone soon', body: PARAS });
    await world.write('roy', s.id, { entryId: (await world.entry(s.id)).entry.id, base: 1, name: 'Gone soon', body: 'Second.' });

    await loginAs(page, 'testy');
    await page.goto(world.page_url(s.id));
    await page.getByRole('button', { name: 'Delete' }).click();
    const dialog = page.getByRole('dialog', { name: /Delete the entry for session 1\?/ });
    await expect(dialog).toContainText('all 2 of its revisions');
    await dialog.getByRole('button', { name: 'Delete entry' }).click();

    await expect(page).toHaveURL(`/games/${world.id}`);
    await expect(page.locator('#message')).toHaveText(/Entry for session 1 deleted/);
    await expect(page.locator('.game-row')).toHaveCount(1);
    await expect(page.locator('.game-row .entry-none')).toHaveText('No entry yet');
    expect((await world.api.testy.get(world.api_url(s.id, '/revisions'))).status()).toBe(404);

    const again = await world.write('adam', s.id, { body: 'A fresh start.' });
    expect(again.number).toBe(1);
    expect((await world.entry(s.id)).entry.author_id).toBe(world.ids.adam);
  });

  test('deleting a by-hand session deletes its entry, and the game page warns first', async ({ page, world }) => {
    const s = await world.played(5);
    const { entryId } = await world.write('roy', s.id, { body: PARAS });
    await world.write('adam', s.id, { entryId, base: 1, body: PARAS + '\n\nDelta four.' });

    await loginAs(page, 'testy');
    await page.goto(`/games/${world.id}`);
    await page.locator('.game-row .game-entry').first().click();
    await page.locator('.game-panel').getByRole('button', { name: 'Delete' }).click();
    const dialog = page.getByRole('dialog', { name: 'Delete session' });
    await expect(dialog).toContainText('journal entry is deleted too, with 2 revisions');
    await dialog.getByRole('button', { name: 'Delete', exact: true }).click();
    await expect(page.locator('.game-row')).toHaveCount(0);

    expect((await world.api.testy.get(world.api_url(s.id))).status()).toBe(404);
    const game = await (await world.api.testy.get(`/api/campaigns/${world.id}`)).json();
    expect(game.played_sessions).toEqual([]);
    expect(game.journal_cancelled).toEqual([]);
  });
});

test.describe('numbering', () => {
  test('recording an older session renumbers the title, but the link still opens the same entry', async ({ page, world }) => {
    const s = await world.played(5);
    await world.write('roy', s.id, { name: 'Steady', body: PARAS });
    await loginAs(page, 'roy');
    await page.goto(world.page_url(s.id));
    await expect(heading(page)).toHaveText(/\b1: Steady/);
    await expect(page.locator('.journal-page .modal-eyebrow').first()).toContainText('Session 1');

    await world.played(12); // happened earlier, so session 1 becomes session 2
    await page.goto(world.page_url(s.id));
    await expect(heading(page)).toHaveText(/\b2: Steady/);
    await expect(page.locator('.game-text')).toContainText('Alpha one.');
    await expect(page.getByRole('navigation', { name: 'Breadcrumb' })).toContainText('Session 2');
  });
});

test.describe('navigation', () => {
  test('the game page has a breadcrumb and no close cross in its header', async ({ page, world }) => {
    await loginAs(page, 'roy');
    await page.goto(`/games/${world.id}`);
    const crumb = page.getByRole('navigation', { name: 'Breadcrumb' });
    await expect(crumb.getByRole('link', { name: 'Games' })).toHaveAttribute('href', `/campaigns#campaign-${world.id}`);
    await expect(crumb.locator('[aria-current="page"]')).toContainText('Journal E2E');
    await expect(page.locator('.game-close')).toHaveCount(0);
    await expect(page.locator('.game-sheet-head').getByText('×')).toHaveCount(0);
  });

  test('change password shows the main nav, and hides it on a forced change', async ({ page }) => {
    await loginAs(page, 'testy');
    await page.goto('/change-password');
    const nav = page.getByRole('navigation', { name: 'Main' });
    await expect(nav.getByRole('link', { name: 'Profile' })).toHaveAttribute('aria-current', 'page');
    await expect(page.getByRole('navigation', { name: 'Breadcrumb' })).toContainText('Profile');

    // A forced change: the same page with must_change_password set on the current user
    await page.route('**/auth/current-user', async (route) => {
      const user = await (await route.fetch()).json();
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ...user, must_change_password: true }) });
    });
    await page.goto('/change-password');
    await expect(page.getByText('You are using a temporary password.')).toBeVisible();
    await expect(nav).toBeHidden();
    await expect(page.getByRole('navigation', { name: 'Breadcrumb' })).toBeHidden();
  });
});
