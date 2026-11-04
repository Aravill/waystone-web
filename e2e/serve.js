// Starts the app against a fresh throwaway database. Playwright's webServer runs
// this instead of `go run` directly because the DB can't be deleted safely from
// globalSetup (the server is already up by then).
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const root = path.resolve(__dirname, '..');
const tmp = path.join(__dirname, '.tmp');

fs.rmSync(tmp, { force: true, recursive: true, maxRetries: 10, retryDelay: 200 });
fs.mkdirSync(tmp, { recursive: true });

const child = spawn('go', ['run', 'main.go'], {
  cwd: root,
  stdio: 'inherit',
  env: { ...process.env, DB_PATH: path.join(tmp, 'waystone.db') },
});

for (const sig of ['SIGINT', 'SIGTERM']) {
  process.on(sig, () => child.kill(sig));
}
child.on('exit', (code) => process.exit(code ?? 1));
