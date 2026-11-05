// @ts-check
const { test, expect } = require('@playwright/test');
const env = require('./env');

test('valid login reaches the app', async ({ page }) => {
  await page.goto('/login.html');
  await page.getByLabel('Username').fill('testy');
  await page.getByLabel('Password').fill(env.DEMO_PASSWORD);
  await page.getByRole('button', { name: 'Enter' }).click();
  await expect(page).toHaveTitle('Waystone - Home');
  await expect(page.getByRole('button', { name: 'Logout' })).toBeVisible();
});

// Only one bad attempt, as simon: the login limiter is per ip+username and no other UI test uses simon.
test('wrong password shows an error', async ({ page }) => {
  await page.goto('/login.html');
  await page.getByLabel('Username').fill('simon');
  await page.getByLabel('Password').fill('not-the-password');
  await page.getByRole('button', { name: 'Enter' }).click();
  await expect(page.getByText('invalid username or password')).toBeVisible();
  await expect(page).toHaveTitle('Login - Waystone');
});

test('login returns to the page that was asked for', async ({ page }) => {
  await page.goto('/campaigns?calendar=not-a-real-game');
  await expect(page).toHaveURL(/\/login\.html\?next=/);
  await page.getByLabel('Username').fill('testy');
  await page.getByLabel('Password').fill(env.DEMO_PASSWORD);
  await page.getByRole('button', { name: 'Enter' }).click();
  await expect(page).toHaveURL(/\/campaigns/);
});

for (const next of ['//evil.example', 'https://evil.example/', '/\\evil.example', '/.//evil.example', 'javascript:alert(1)']) {
  test(`login ignores an off-site return address: ${next}`, async ({ page }) => {
    await page.goto(`/login.html?next=${encodeURIComponent(next)}`);
    await page.getByLabel('Username').fill('testy');
    await page.getByLabel('Password').fill(env.DEMO_PASSWORD);
    await page.getByRole('button', { name: 'Enter' }).click();
    await expect(page.getByRole('button', { name: 'Logout' })).toBeVisible();
    expect(new URL(page.url()).hostname).toBe('localhost');
  });
}

test('a fresh session lasts 30 days and is not reissued on every page', async ({ page, context }) => {
  await page.goto('/login.html');
  await page.getByLabel('Username').fill('testy');
  await page.getByLabel('Password').fill(env.DEMO_PASSWORD);
  await page.getByRole('button', { name: 'Enter' }).click();
  await expect(page.getByRole('button', { name: 'Logout' })).toBeVisible();

  const cookie = (await context.cookies()).find((c) => c.name === 'auth_session');
  const days = (cookie.expires - Date.now() / 1000) / 86400;
  expect(days).toBeGreaterThan(29);
  expect(days).toBeLessThanOrEqual(30);

  const res = await page.request.get('/campaigns');
  expect(res.ok()).toBeTruthy();
  expect(res.headers()['set-cookie']).toBeUndefined();
});

test('a session that dies in an open tab returns to that page after login', async ({ page, context }) => {
  await page.goto('/login.html');
  await page.getByLabel('Username').fill('testy');
  await page.getByLabel('Password').fill(env.DEMO_PASSWORD);
  await page.getByRole('button', { name: 'Enter' }).click();
  await expect(page.getByRole('button', { name: 'Logout' })).toBeVisible();
  await page.goto('/profile?from=test');
  await expect(page.locator('.notif-btn')).toBeVisible();

  await context.clearCookies();
  // Coming back to the tab rechecks the unread count, which finds the dead session.
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
  await expect(page).toHaveURL(/\/login\.html\?next=%2Fprofile%3Ffrom%3Dtest$/);

  await page.getByLabel('Username').fill('testy');
  await page.getByLabel('Password').fill(env.DEMO_PASSWORD);
  await page.getByRole('button', { name: 'Enter' }).click();
  await expect(page).toHaveURL(/\/profile\?from=test$/);
});

test('logging out does not set a return address', async ({ page }) => {
  await page.goto('/login.html');
  await page.getByLabel('Username').fill('testy');
  await page.getByLabel('Password').fill(env.DEMO_PASSWORD);
  await page.getByRole('button', { name: 'Enter' }).click();
  await expect(page.getByRole('button', { name: 'Logout' })).toBeVisible();
  await page.goto('/campaigns');
  await page.getByRole('button', { name: 'Logout' }).click();
  await expect(page).toHaveURL(/\/login\.html$/);
});
