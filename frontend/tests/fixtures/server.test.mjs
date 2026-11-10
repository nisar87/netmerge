import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { request as httpRequest } from 'node:http';
import { after, before, beforeEach, test } from 'node:test';
import { createFixtureServer } from '../../scripts/serve.mjs';
import { createFixture, FAKE_PASSWORD } from './data.mjs';

let server, origin;
before(async () => {
  server = createFixtureServer();
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  origin = `http://127.0.0.1:${server.address().port}`;
});
after(async () => { if (server) { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); } });
async function call(path, body, headers = {}) {
  const response = await fetch(origin + path, { method: body === undefined ? 'GET' : 'POST',
    headers: { 'X-NetMerge': '1', 'X-NetMerge-Test': '1', 'Content-Type': 'application/json', ...headers },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
  return { status: response.status, body: await response.json() };
}
beforeEach(() => call('/__test/reset', {}));

test('serves actual UI assets, never arbitrary repository files', async () => {
  for (const [path, file] of [['/', '../../../ui/index.html'], ['/app/boot.mjs', '../../../ui/app/boot.mjs']]) {
    const response = await fetch(origin + path);
    assert.equal(response.status, 200);
    assert.equal(await response.text(), await readFile(new URL(file, import.meta.url), 'utf8'));
  }
  for (const path of ['/api.go', '/.git/config', '/frontend/tests/fixtures/data.mjs', '/missing.js', '/downloads.js', '/extension/src/popup.html', '/../api.go']) {
    assert.equal((await fetch(origin + path)).status, 404, path);
  }
});
test('test routes are gated and mutations enforce the production header and local origin', async () => {
  assert.equal((await fetch(origin + '/__test/requests')).status, 403);
  assert.equal((await fetch(origin + '/api/power', { method: 'POST', body: '{"on":false}' })).status, 403);
  assert.equal((await call('/api/power', { on: false }, { Origin: 'https://example.test' })).status, 403);
  assert.equal((await call('/__test/reset', {}, { Origin: 'https://example.test' })).status, 403);
  const response = await fetch(origin + '/api/power', { method: 'POST', headers: { 'X-NetMerge': '1' }, body: '{' });
  assert.equal(response.status, 400);
  const hostileHost = await new Promise(resolve => {
    const req = httpRequest(origin + '/api/state', { headers: { Host: 'example.test:4173' } }, res => { res.resume(); resolve(res.statusCode); });
    req.end();
  });
  assert.equal(hostileHost, 403);
});
test('acknowledgement shapes, fake side effects and reset are deterministic; the download manager is gone', async () => {
  const baseline = (await call('/api/state')).body;
  assert.equal(baseline.legacyDownloads, 0); assert.equal(baseline.downloads, undefined); assert.equal(baseline.downloadsOn, undefined);
  assert.equal((await call('/api/downloads')).status, 404);
  assert.equal((await call('/api/legacy-downloads')).status, 404);
  assert.deepEqual((await call('/api/power', { on: false })).body, { ok: true });
  assert.equal((await call('/api/state')).body.snap.running, false);
  assert.deepEqual((await call('/api/quit', {})).body, { ok: true });
  assert.equal((await call('/api/ping')).status, 200);
  await call('/__test/reset', {});
  assert.deepEqual((await call('/api/state')).body, baseline);
});
test('all existing GETs return real-shaped data and unknown routes fail', async () => {
  for (const path of ['state', 'block', 'usage', 'share', 'presets', 'dns', 'events', 'history', 'log?f=engine', 'update', 'ping']) {
    const r = await call(`/api/${path}`); assert.equal(r.status, 200, path); assert.equal(typeof r.body, 'object', path);
  }
  assert.equal((await call('/api/not-a-contract')).status, 404);
  assert.equal((await call('/api/downloads/switch', { on: true })).status, 404);
});
test('complete settings writes merge by adapter name and preserve server-owned fields', async () => {
  const settings = createFixture().state.settings;
  settings.links.reverse(); settings.links[0].label = 'Edited Wi-Fi'; settings.links[0].weight = 4;
  settings.autoStart = true; settings.noCallProtect = true; settings.share.pass = 'must-not-replace';
  assert.deepEqual((await call('/api/settings', settings)).body, { ok: true });
  const after = (await call('/api/state')).body;
  assert.equal(after.settings.links[0].name, 'Ethernet');
  assert.equal(after.settings.links[1].label, 'Edited Wi-Fi');
  assert.equal(after.settings.links[1].weight, 4);
  assert.equal(after.settings.share.pass, FAKE_PASSWORD);
  assert.equal(after.snap.links[1].label, 'Edited Wi-Fi');
  assert.equal(after.settings.autoStart, true);
  assert.equal((await call('/api/usage')).body.links[1].label, 'Edited Wi-Fi');
});
test('captures exact mutation bodies, headers, order and failures but not test controls', async () => {
  await call('/api/power', { on: false });
  await call('/api/rescan', {});
  await call('/api/no-such-action', {});
  const requests = (await call('/__test/requests')).body.requests;
  assert.deepEqual(requests.map(r => [r.method, r.path, r.body, r.status]), [
    ['POST', '/api/power', { on: false }, 200], ['POST', '/api/rescan', {}, 200], ['POST', '/api/no-such-action', {}, 404],
  ]);
  assert.equal(requests[0].headers['x-netmerge'], '1');
  assert.deepEqual(requests.map(r => r.sequence), [1, 2, 3]);
  await call('/__test/requests', {});
  assert.deepEqual((await call('/__test/requests')).body, { requests: [] });
});
test('fault scenarios and recovery retain in-memory edits', async () => {
  await call('/api/power', { on: false });
  await call('/__test/scenario', { scenario: 'offline' });
  assert.equal((await call('/api/state')).status, 503);
  await call('/__test/scenario', { scenario: 'default' });
  assert.equal((await call('/api/state')).body.snap.running, false);
  await call('/__test/fault', { path: '/api/power', method: 'POST', status: 500, times: 1 });
  assert.equal((await call('/api/power', { on: true })).status, 500);
  assert.equal((await call('/api/state')).body.snap.running, false);
  assert.equal((await call('/api/power', { on: true })).status, 200);
  for (const [scenario, count] of [['one-connection', 1], ['empty', 0]]) {
    await call('/__test/reset', { scenario }); assert.equal((await call('/api/state')).body.snap.links.length, count);
  }
  await call('/__test/reset', { scenario: 'null' });
  assert.equal((await call('/api/state')).body.snap.links, null);
  assert.equal((await call('/__test/reset', { scenario: 'typo' })).status, 400);
});
test('patches replace arrays and delayed reads retain their original snapshot', async () => {
  await call('/__test/state', { state: { settings: { stableDomains: ['dirty.example.test'] } } });
  assert.deepEqual((await call('/api/state')).body.settings.stableDomains, ['dirty.example.test']);
  await call('/__test/fault', { path: '/api/state', delayMs: 100, times: 1 });
  const pending = call('/api/state');
  // Wait for server receipt, not a wall-clock guess.
  while (!(await call('/__test/requests')).body.requests.some(r => r.path === '/api/state' && r.status === null)) await new Promise(resolve => setTimeout(resolve, 2));
  await call('/api/power', { on: false });
  assert.equal((await pending).body.snap.running, true);
  assert.equal((await call('/api/state')).body.snap.running, false);
});
test('DNS, presets, blocking and sharing mutations reconcile with state', async () => {
  await call('/api/dns', { action: 'set', provider: 'custom', secure: true, custom: ['192.0.2.53'], domains: ['corp.example.test'] });
  assert.equal((await call('/api/state')).body.settings.dnsProvider, 'custom');
  assert.equal((await call('/api/dns')).body.dns.secure, false);
  await call('/api/presets', { action: 'apply', id: 'safe' });
  assert.equal((await call('/api/state')).body.settings.mode, 'stable');
  await call('/api/presets', { action: 'restore' });
  assert.equal((await call('/api/state')).body.settings.mode, 'fast');
  await call('/api/block', { enabled: false });
  assert.equal((await call('/api/state')).body.block.enabled, false);
  await call('/api/share', { enabled: true });
  await call('/api/share', { name: { id: '02:00:00:00:00:01', name: 'Renamed fixture phone' } });
  assert.equal((await call('/api/share')).body.devices[0].name, 'Renamed fixture phone');
  assert.equal((await call('/api/state')).body.settings.share.enabled, true);
});
test('backup is sanitized, restore is inert and diagnostics is a real ZIP', async () => {
  const backup = await fetch(origin + '/api/backup');
  assert.match(backup.headers.get('content-disposition'), /attachment/);
  const data = await backup.json();
  assert.ok(!JSON.stringify(data).includes(FAKE_PASSWORD)); assert.equal(data.secret, ''); assert.equal(data.share.pass, '');
  assert.equal(data.deviceNames, undefined); assert.equal(data.downloadConfig, undefined);
  assert.deepEqual((await call('/api/restore', { ...data, mode: 'stable' })).body, { ok: true });
  assert.equal((await call('/api/state')).body.settings.mode, 'stable');
  const diagnostics = await fetch(origin + '/api/diagnostics');
  assert.equal(diagnostics.headers.get('content-type'), 'application/zip');
  assert.equal(Buffer.from(await diagnostics.arrayBuffer()).subarray(0, 4).toString('hex'), '504b0304');
});

test('the old download list is offered in the format Pullwell imports', async () => {
  await call('/__test/state', { state: { legacyDownloads: 2 } });
  assert.equal((await call('/api/state')).body.legacyDownloads, 2);
  const list = (await call('/api/legacy-downloads')).body;
  assert.equal(list.app, 'netmerge-downloads'); assert.equal(list.version, 1); assert.equal(list.items.length, 2);
});
