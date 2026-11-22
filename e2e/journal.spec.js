// @ts-check
const base = require('./helpers');
const { expect, loginAs, apiAs, sojournId, dateFromToday } = base;
const env = require('./env');

const PEOPLE = ['testy', 'roy', 'adam', 'simon', env.ADMIN_USERNAME];
const CODEX = '5b7e9d21-8c3a-4f6e-a1d4-7c2e9b0f3a61'; // the demo codex, "The Karrow Marches"

/** "Jan 2, 2026" for a YYYY-MM-DD date, the way the Session template writes {{date}}. */
function templateDate(ymd) {
  const [y, m, d] = ymd.split('-').map(Number);
  return `${new Date(Date.UTC(y, m - 1, d)).toLocaleString('en-US', { month: 'short', timeZone: 'UTC' })} ${d}, ${y}`;
}

/**
 * Every test gets its own game, so none of them touches the seeded ones. testy is the DM (Edit on the
 * demo codex), roy a player with Edit, adam a player who can only See the codex, simon a logged-in
 * visitor. The game starts without a codex; `world.link()` sets it in the demo codex. Sessions are
 * recorded by hand (always played). At teardown the game and every page and folder it made are removed.
 */
const test = base.test.extend({
  world: async ({}, use) => {
    const api = {};
    const ids = {};
    for (const name of PEOPLE) {
      api[name] = await apiAs(name);
      ids[name] = (await (await api[name].get('/auth/current-user')).json()).user_id;
    }
    const title = `Journal E2E ${Date.now()}`;
    const res = await api.testy.post('/api/campaigns', {
      data: { title, summary: 'A game for the journal tests', description: 'Test game', type: 'Campaign' },
    });
    expect(res.status(), 'create game').toBe(201);
    const id = (await res.json()).campaign_id;
    for (const player of ['roy', 'adam']) {
      expect((await api[player].post(`/api/campaigns/${id}/signups`)).status(), `${player} signs up`).toBe(201);
      expect((await api.testy.put(`/api/campaigns/${id}/signups/${ids[player]}`, { data: { status: 'Accepted' } })).status(), `accept ${player}`).toBe(200);
    }
    const world = {
      id,
      title,
      api,
      ids,
      async setCodex(codexId) {
        const r = await api.testy.put(`/api/campaigns/${id}`, {
          data: { title, summary: 'A game for the journal tests', description: 'Test game', type: 'Campaign', codex_id: codexId },
        });
        expect(r.status(), 'set codex').toBe(200);
      },
      link: () => world.setCodex(CODEX),
      async played(daysAgo) {
        const r = await api.testy.post(`/api/campaigns/${id}/played-sessions`, {
          data: { date: dateFromToday(-daysAgo), start: '19:00', end: '22:00', timezone: 'Europe/Prague', attendee_ids: ['testy', 'roy', 'adam'].map((a) => ids[a]), guests: [] },
        });
        expect(r.status(), 'record session').toBe(201);
        return (await r.json()).session;
      },
      start: (user, sid) => api[user].post(`/api/campaigns/${id}/journal/${sid}`),
      async game(user = 'testy') {
        return (await api[user].get(`/api/campaigns/${id}`)).json();
      },
      async codex(user = 'testy') {
        return (await api[user].get(`/api/codices/c/${CODEX}`)).json();
      },
      async pageOf(user, pageId) {
        const r = await api[user].get(`/api/codices/pages/${pageId}`);
        expect(r.status()).toBe(200);
        return r.json();
      },
    };
    await use(world);

    const admin = api[env.ADMIN_USERNAME];
    const codex = await (await admin.get(`/api/codices/c/${CODEX}`)).json();
    for (const p of codex.pages.filter((p) => p.title.startsWith(`${title}: `))) await admin.delete(`/api/codices/pages/${p.id}`);
    for (const f of codex.folders.filter((f) => f.name.startsWith(`${title}: Campaign Journal`))) await admin.delete(`/api/codices/folders/${f.id}`);
    await admin.delete(`/api/campaigns/${id}`);
    await Promise.all(Object.values(api).map((a) => a.dispose()));
  },
});

test.describe('the seeded journal', () => {
  test('the Sojourn entries are pages in the demo codex with their revisions and scribes', async ({ page }) => {
    const testy = await apiAs('testy');
    const sojourn = await sojournId(testy);
    const game = await (await testy.get(`/api/campaigns/${sojourn}`)).json();
    const written = game.played_sessions.filter((s) => s.journal);
    expect(written.map((s) => s.journal.revisions).sort()).toEqual([1, 3]);
    expect(game.journal_pending).toBe(0);

    const codex = await (await testy.get(`/api/codices/c/${CODEX}`)).json();
    const folder = codex.folders.find((f) => f.name === 'Age of Sojourn: Campaign Journal');
    expect(folder, 'the journal folder').toBeTruthy();
    expect(folder.parent_id).toBe('');
    expect(folder.default_template).toBe('builtin:session');
    for (const s of written) {
      expect(s.journal).toMatchObject({ codex_id: CODEX, can_open: true });
      const inFolder = codex.pages.find((p) => p.id === s.journal.page_id);
      expect(inFolder).toMatchObject({ folder_id: folder.id, kind: 'lore' });
      expect(inFolder.title).toBe(`Age of Sojourn: ${s.date}` + (s.journal.revisions === 3 ? ' – The Dark Waystone' : ' – Ash on the Wind'));
    }

    const three = written.find((s) => s.journal.revisions === 3);
    const payload = await (await testy.get(`/api/codices/pages/${three.journal.page_id}`)).json();
    expect(payload.page.revision_count).toBe(3);
    expect(payload.page.author_user.display_name).toBe('Roy');
    expect(payload.page.amender_users.map((u) => u.display_name)).toEqual(['Adam', 'Testy']);
    expect(payload.session).toMatchObject({ number: three.number, date: three.date, played: true, campaign_title: 'Age of Sojourn' });
    const revs = await (await testy.get(`/api/codices/pages/${three.journal.page_id}/revisions`)).json();
    expect(revs.revisions.map((r) => r.number)).toEqual([3, 2, 1]);
    expect(revs.revisions.at(-1).note).toBe('First draft');
    await testy.dispose();

    // Read in the browser: "Session N" under the journal eyebrow, and the tree shows the date only.
    await loginAs(page, 'testy');
    await page.goto(`/codices/p/${three.journal.page_id}/journal`);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(`Session ${three.number}`);
    await expect(page.locator('.cx-kicker', { hasText: 'Campaign journal' })).toContainText('Campaign journal · Age of Sojourn');
    await expect(page.locator('.cx-jmeta')).toContainText(three.date);
    await expect(page.locator('.codex-folder').filter({ hasText: 'Age of Sojourn: Campaign Journal' }).getByRole('link', { name: `${three.date} – The Dark Waystone` })).toBeVisible();
  });
});

test.describe('writing an entry', () => {
  test('a player with Edit starts it from the game page and lands in the editor with the Session template', async ({ page, world }) => {
    await world.link();
    const s = await world.played(5);
    await loginAs(page, 'roy');
    await page.goto(`/games/${world.id}`);
    await page.getByRole('button', { name: /Write the entry/ }).first().click();
    await expect(page).toHaveURL(/\/codices\/p\/[^/]+\/[^/]+\/edit$/);
    await expect(page.locator('.cm-content')).toContainText('Who was there');

    const game = await world.game();
    const journal = game.played_sessions[0].journal;
    expect(journal).toMatchObject({ title: `${world.title}: ${s.date}`, codex_id: CODEX, revisions: 1 });
    const payload = await world.pageOf('roy', journal.page_id);
    expect(payload.page.body).toContain(`**Date:** ${templateDate(s.date)}`);
    expect(payload.page.folder_path.map((f) => f.name)).toEqual([`${world.title}: Campaign Journal`]);
    expect(payload.session).toMatchObject({ number: 1, date: s.date, played: true, session_id: s.id });
    expect(payload.tags.map((t) => t.name)).toContain('session');

    // A second request finds the same page; the game page now links to it.
    const again = await world.start('testy', s.id);
    expect(again.status()).toBe(200);
    expect((await again.json()).page.id).toBe(journal.page_id);
    await page.goto(`/games/${world.id}`);
    await expect(page.locator('.entry-link')).toHaveText('Unnamed entry');
    await expect(page.getByRole('button', { name: /Write the entry/ })).toHaveCount(0);
    await page.locator('.entry-link').click();
    await expect(page).toHaveURL(/\/codices\/p\/[^/]+\/[^/]+$/);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Session 1');
  });

  test('a player without Edit sees no Write button and gets 403', async ({ page, world }) => {
    await world.link();
    const s = await world.played(5);
    await loginAs(page, 'adam');
    await page.goto(`/games/${world.id}`);
    await expect(page.locator('.game-row')).toHaveCount(1);
    await expect(page.getByRole('button', { name: /Write the entry/ })).toHaveCount(0);
    const r = await world.start('adam', s.id);
    expect(r.status()).toBe(403);
    expect((await world.game()).played_sessions[0].journal).toBeNull();
  });

  test('a Suggested session cannot have an entry (409)', async ({ world }) => {
    await world.link();
    const r = await world.api.testy.post(`/api/campaigns/${world.id}/sessions`, {
      data: { date: dateFromToday(3), time: '18:00', duration: 120, timezone: 'Europe/Prague' },
    });
    const session = (await r.json()).session;
    const started = await world.start('roy', session.id);
    expect(started.status()).toBe(409);
    expect((await started.json()).code).toBe('session_not_played');
  });

  test('a game without a codex shows the DM a hint and answers 409', async ({ page, world }) => {
    const s = await world.played(5);
    expect((await world.start('roy', s.id)).status()).toBe(409);
    const r = await world.start('testy', s.id);
    expect((await r.json()).code).toBe('no_codex');
    await loginAs(page, 'testy');
    await page.goto(`/games/${world.id}`);
    await expect(page.locator('.game-chronicle').getByText('Link a codex in Edit game to keep a journal.')).toBeVisible();
    await expect(page.getByRole('button', { name: /Write the entry/ })).toHaveCount(0);
    await loginAs(page, 'roy');
    await page.goto(`/games/${world.id}`);
    await expect(page.locator('.game-chronicle').getByText('Link a codex in Edit game to keep a journal.')).toBeHidden();
  });
});

test.describe('after the entry exists', () => {
  test('an old journal URL redirects (303) to the codex page, or to the game when there is none', async ({ world }) => {
    await world.link();
    const s = await world.played(5);
    const none = await world.api.testy.get(`/games/${world.id}/journal/${s.id}`, { maxRedirects: 0 });
    expect(none.status()).toBe(303);
    expect(none.headers().location).toBe(`/games/${world.id}`);

    const pageId = (await (await world.start('testy', s.id)).json()).page.id;
    for (const tail of ['', '/edit', '/history']) {
      const r = await world.api.testy.get(`/games/${world.id}/journal/${s.id}${tail}`, { maxRedirects: 0 });
      expect(r.status(), tail).toBe(303);
      expect(r.headers().location).toBe(`/codices/p/${pageId}/journal${tail}`);
    }
  });

  test('deleting a by-hand session keeps the page as an ordinary page', async ({ page, world }) => {
    await world.link();
    const s = await world.played(5);
    const pageId = (await (await world.start('roy', s.id)).json()).page.id;
    expect((await world.pageOf('roy', pageId)).session).not.toBeNull();

    await loginAs(page, 'testy');
    await page.goto(`/games/${world.id}`);
    await page.locator('.game-row .game-entry').first().click();
    await page.locator('.game-panel').getByRole('button', { name: 'Delete' }).click();
    const dialog = page.getByRole('dialog', { name: 'Delete session' });
    await expect(dialog).toContainText('journal page stays in the codex as an ordinary page');
    await dialog.getByRole('button', { name: 'Delete', exact: true }).click();
    await expect(page.locator('.game-row')).toHaveCount(0);

    const kept = await world.pageOf('roy', pageId);
    expect(kept.session).toBeNull();
    expect(kept.page.title).toBe(`${world.title}: ${s.date}`);
    const game = await world.game();
    expect(game.played_sessions).toEqual([]);
    expect(game.journal_cancelled).toEqual([]);
  });

  test('changing the game\'s codex leaves the old page linked, and the next entry gets a new folder', async ({ world }) => {
    await world.link();
    const older = await world.played(9);
    const newer = await world.played(5);
    const first = (await (await world.start('roy', older.id)).json()).page.id;

    await world.setCodex('');
    let game = await world.game();
    expect(game.codex).toBeNull();
    const row = game.played_sessions.find((x) => x.id === older.id);
    expect(row.journal).toMatchObject({ page_id: first, codex_id: CODEX, can_open: true });
    expect((await world.start('testy', newer.id)).status()).toBe(409);
    expect((await world.pageOf('roy', first)).session).toMatchObject({ session_id: older.id });

    // Linked again, the old folder is forgotten: the next entry makes a second one.
    await world.link();
    const second = (await (await world.start('roy', newer.id)).json()).page.id;
    const folders = (await world.codex()).folders.filter((f) => f.name.startsWith(`${world.title}: Campaign Journal`));
    expect(folders.map((f) => f.name).sort()).toEqual([`${world.title}: Campaign Journal`, `${world.title}: Campaign Journal (2)`]);
    const pages = (await world.codex()).pages;
    expect(pages.find((p) => p.id === first).folder_id).not.toBe(pages.find((p) => p.id === second).folder_id);
    game = await world.game();
    expect(game.played_sessions.every((x) => x.journal)).toBe(true);
  });

  test('renaming the game renames its folder while it has the generated name, and not page titles', async ({ world }) => {
    await world.link();
    const s = await world.played(5);
    const pageId = (await (await world.start('roy', s.id)).json()).page.id;
    const renamed = `${world.title} Reborn`;
    const r = await world.api.testy.put(`/api/campaigns/${world.id}`, {
      data: { title: renamed, summary: 'A game for the journal tests', description: 'Test game', type: 'Campaign' },
    });
    expect(r.status()).toBe(200);
    const codex = await world.codex();
    expect(codex.folders.map((f) => f.name)).toContain(`${renamed}: Campaign Journal`);
    expect(codex.pages.find((p) => p.id === pageId).title).toBe(`${world.title}: ${s.date}`);
    // Put the name back so teardown finds the folder.
    await world.api.testy.put(`/api/campaigns/${world.id}`, {
      data: { title: world.title, summary: 'A game for the journal tests', description: 'Test game', type: 'Campaign' },
    });
  });
});
