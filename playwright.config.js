// @ts-check
const { defineConfig, devices } = require('@playwright/test');
const env = require('./e2e/env');

module.exports = defineConfig({
  testDir: 'e2e',
  workers: 1,
  fullyParallel: false,
  retries: 0,
  forbidOnly: true,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    // Must be localhost: the session cookie is Secure.
    baseURL: `http://localhost:${env.PORT}`,
    locale: 'en-US',
    timezoneId: 'Europe/Prague',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      // 1440 wide avoids the narrow-cell container query that hides .day-time-full.
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
    },
  ],
  webServer: {
    command: 'node e2e/serve.js',
    url: `http://localhost:${env.PORT}/login.html`,
    reuseExistingServer: false,
    timeout: 120_000,
    env: {
      PORT: env.PORT,
      SESSION_SECRET: env.SESSION_SECRET,
      ADMIN_USERNAME: env.ADMIN_USERNAME,
      ADMIN_PASSWORD: env.ADMIN_PASSWORD,
      SEED_DEMO_DATA: '1',
      BACKUP_DIR: '',
      TRUST_PROXY: '',
    },
  },
});
