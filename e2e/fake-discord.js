// A tiny fake of the Discord REST API, for the E2E server (DISCORD_API_BASE points here).
// It records every message the app posts; GET /_messages lists them and DELETE /_messages clears them.
const http = require('node:http');
const env = require('./env');

const BOT_ID = '900000000000000001';
const VIEW = 1n << 10n;
const SEND = 1n << 11n;
const EMBED = 1n << 14n;
const CHANNEL_OK = '700000000000000001';
const CHANNEL_HIDDEN = '700000000000000002';

const messages = [];
const linkedUsers = new Set();

function send(res, status, body) {
  const data = body === undefined ? '' : JSON.stringify(body);
  res.writeHead(status, { 'Content-Type': 'application/json' });
  res.end(data);
}

function readBody(req) {
  return new Promise((resolve) => {
    const chunks = [];
    req.on('data', (c) => chunks.push(c));
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
  });
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, 'http://localhost');
  const path = url.pathname;
  const auth = req.headers.authorization || '';
  const body = await readBody(req);
  let m;

  if (path === '/_health') return send(res, 200, { ok: true });
  if (path === '/_messages') {
    if (req.method === 'DELETE') {
      messages.length = 0;
      return send(res, 200, { ok: true });
    }
    return send(res, 200, messages);
  }
  if (path === '/oauth2/token' && req.method === 'POST') {
    const code = new URLSearchParams(body).get('code') || '';
    return send(res, 200, { access_token: 'tok-' + code, token_type: 'Bearer' });
  }
  if (path === '/users/@me' && req.method === 'GET') {
    if (auth.startsWith('Bot ')) return send(res, 200, { id: BOT_ID, username: 'waystone-bot' });
    if (auth.startsWith('Bearer tok-')) {
      const id = auth.slice('Bearer tok-'.length);
      linkedUsers.add(id);
      return send(res, 200, { id, username: 'player' + id.slice(-4), global_name: null });
    }
    return send(res, 401, { code: 0, message: '401: Unauthorized' });
  }
  if ((m = path.match(/^\/guilds\/(\d+)$/)) && req.method === 'GET') {
    return send(res, 200, {
      id: m[1], name: 'Test Guild', owner_id: '1',
      roles: [{ id: m[1], permissions: String(VIEW | SEND | EMBED) }],
    });
  }
  if ((m = path.match(/^\/guilds\/(\d+)\/members\/(\d+)$/)) && req.method === 'GET') {
    if (m[2] === BOT_ID || linkedUsers.has(m[2])) return send(res, 200, { user: { id: m[2] }, roles: [] });
    return send(res, 404, { code: 10007, message: 'Unknown Member' });
  }
  if ((m = path.match(/^\/guilds\/(\d+)\/channels$/)) && req.method === 'GET') {
    return send(res, 200, [
      { id: CHANNEL_OK, name: 'game-night', type: 0, position: 0, permission_overwrites: [] },
      {
        id: CHANNEL_HIDDEN, name: 'staff-only', type: 0, position: 1,
        permission_overwrites: [{ id: m[1], type: 0, allow: '0', deny: String(VIEW) }],
      },
      { id: '700000000000000003', name: 'Voice', type: 2, position: 2, permission_overwrites: [] },
    ]);
  }
  if (path === '/users/@me/channels' && req.method === 'POST') {
    const rid = JSON.parse(body || '{}').recipient_id;
    return send(res, 200, { id: 'dm-' + rid, type: 1 });
  }
  if ((m = path.match(/^\/channels\/([^/]+)\/messages$/)) && req.method === 'POST') {
    messages.push({ channel_id: m[1], ...JSON.parse(body || '{}') });
    return send(res, 200, { id: String(messages.length) });
  }
  return send(res, 404, { code: 0, message: '404: Not Found' });
});

server.listen(Number(env.DISCORD_PORT), () => {
  console.log(`fake discord on :${env.DISCORD_PORT}`);
});
for (const sig of ['SIGINT', 'SIGTERM']) process.on(sig, () => process.exit(0));
