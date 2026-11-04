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
