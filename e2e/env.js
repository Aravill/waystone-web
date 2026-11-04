// Throwaway values for the local test server only (see playwright.config.js).
module.exports = {
  PORT: '18080',
  ADMIN_USERNAME: 'admin',
  ADMIN_PASSWORD: 'e2e-admin-password-1',
  SESSION_SECRET: 'e2e-session-secret-0123456789-abcdefghij',
  // Public demo password, config.InitialPasswords.
  DEMO_PASSWORD: 'ohgoditsacat',
};
