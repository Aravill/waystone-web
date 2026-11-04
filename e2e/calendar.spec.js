// @ts-check
const { test, expect, loginAs, dateFromToday, openCalendar, goToMonth } = require('./helpers');

test.beforeEach(async ({ page }) => {
  await loginAs(page, 'testy');
});

test('calendar opens on the seeded suggestion with its time and tally', async ({ page }) => {
  const modal = await openCalendar(page);
  await expect(modal.getByRole('heading', { name: 'Age of Sojourn' })).toBeVisible();

  const cell = modal.getByRole('button', { name: /Suggested 18:00-22:00/ });
  await expect(cell.getByText('18:00-22:00')).toBeVisible();
  await expect(cell.locator('.reply-accepted b')).toHaveText('1');
  await expect(cell.locator('.reply-awaiting b')).toHaveText('3');
  await expect(cell.locator('.reply-declined b')).toHaveText('0');
});

test('day panel lists a roster row per player', async ({ page, game }) => {
  const date = dateFromToday(42);
  const session = await game.create({ date, time: '20:30' });
  await game.replyAs('roy', session, 'Accepted');
  await game.replyAs('adam', session, 'Tentative');
  await game.replyAs('simon', session, 'Declined');

  const modal = await openCalendar(page);
  await goToMonth(page, date);
  await modal.getByRole('button', { name: /Suggested 20:30-/ }).click();

  const panel = modal.locator('.calendar-panel');
  for (const [name, said] of [['Roy', 'Accepted'], ['Adam', 'Tentative'], ['Simon', 'Declined'], ['Testy', 'Awaiting']]) {
    await expect(panel.getByText(name, { exact: true })).toBeVisible();
    await expect(panel.locator('.roster-row').filter({ hasText: name })).toContainText(said);
  }
});

test('three of four accepted is a strong candidate', async ({ page, game }) => {
  const date = dateFromToday(49);
  const session = await game.create({ date, time: '17:45' });
  for (const user of ['roy', 'adam', 'simon']) await game.replyAs(user, session, 'Accepted');

  const modal = await openCalendar(page);
  await goToMonth(page, date);
  const cell = modal.getByRole('button', { name: /Suggested 17:45-/ });
  await expect(cell).toHaveAccessibleName(/Strong candidate/);
  await expect(cell.getByText('✦')).toBeVisible();

  await cell.click();
  await expect(modal.locator('.calendar-panel').getByText('Strong candidate: 3 of 4 accepted')).toBeVisible();
});

test('changing a reply updates the tally', async ({ page, game }) => {
  const date = dateFromToday(35);
  const session = await game.create({ date, time: '19:15', duration: 120 });
  await game.replyAs('roy', session, 'Accepted');

  const modal = await openCalendar(page);
  await goToMonth(page, date);
  const cell = modal.getByRole('button', { name: /Suggested 19:15-/ });
  await cell.click();
  const panel = modal.locator('.calendar-panel');

  await panel.getByRole('button', { name: /Aye$/ }).click();
  await expect(panel.getByRole('button', { name: /Aye$/ })).toHaveClass(/selected/);
  await expect(cell.locator('.reply-accepted b')).toHaveText('2');
  await expect(cell.locator('.reply-awaiting b')).toHaveText('2');

  await panel.getByRole('button', { name: /Nay$/ }).click();
  await expect(panel.getByRole('button', { name: /Nay$/ })).toHaveClass(/selected/);
  await expect(cell.locator('.reply-accepted b')).toHaveText('1');
  await expect(cell.locator('.reply-declined b')).toHaveText('1');
  await expect(cell.locator('.reply-awaiting b')).toHaveText('2');
});

// Strong candidate = strictly over 70% of ALL players accepted; awaiting players count in the total.
const thresholdCases = [
  { name: '7 of 10 is exactly 70%: no mark', status: 'Suggested', accepted: 7, tentative: 1, declined: 1, awaiting: 1, strong: false },
  { name: '5 of 7 is over 70%: mark', status: 'Suggested', accepted: 5, tentative: 0, declined: 0, awaiting: 2, strong: true, note: 'Strong candidate: 5 of 7 accepted' },
  { name: '2 of 3 is 67%: no mark (awaiting counts)', status: 'Suggested', accepted: 2, tentative: 0, declined: 0, awaiting: 1, strong: false },
  { name: 'confirmed session: no mark', status: 'Confirmed', accepted: 4, tentative: 0, declined: 0, awaiting: 0, strong: false },
];

for (const c of thresholdCases) {
  test(`strong candidate threshold: ${c.name}`, async ({ page }) => {
    await page.route(/\/api\/campaigns\/[^/]+\/sessions\?month=(\d{4}-\d{2})$/, async (route) => {
      const month = /month=(\d{4}-\d{2})$/.exec(route.request().url())[1];
      await route.fulfill({
        json: [{
          id: 'mock-1',
          campaign_id: 'mock',
          date: `${month}-15`,
          time: '19:00',
          duration: 120,
          status: c.status,
          timezone: 'Europe/Prague',
          accepted_count: c.accepted,
          tentative_count: c.tentative,
          declined_count: c.declined,
          pending_count: c.awaiting,
          current_user_participation: '',
        }],
      });
    });
    await page.route(/\/sessions\/mock-[^/]+\/responses$/, (route) =>
      route.fulfill({ json: { all: [], pending: [], responses: {} } }));

    const modal = await openCalendar(page);
    const cell = modal.getByRole('button', { name: new RegExp(`${c.status} 19:00-`) });
    await expect(cell).toBeVisible();
    await cell.click();
    const panel = modal.locator('.calendar-panel');

    if (c.strong) {
      await expect(cell.getByText('✦')).toBeVisible();
      await expect(panel.getByText(c.note, { exact: true })).toBeVisible();
    } else {
      await expect(cell.getByText('✦')).toBeHidden();
      await expect(panel.getByText(/Strong candidate:/)).toBeHidden();
    }
  });
}
