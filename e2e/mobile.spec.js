// @ts-check
const { test, expect, loginAs, openCalendar } = require('./helpers');

test.use({ viewport: { width: 375, height: 812 } });

test('calendar shows the start time and roster on a phone', async ({ page }) => {
  await loginAs(page, 'testy');
  const modal = await openCalendar(page);

  const cell = modal.getByRole('button', { name: /Suggested 18:00-22:00/ });
  await expect(cell.locator('.day-time')).toHaveText('18:00');
  await expect(modal.locator('.calendar-panel .roster-name').first()).toBeVisible();
});
