#!/usr/bin/env node
// Development-only loopback server. Only ui/ and extension/ are exposed as files.
import { createServer } from 'node:http';
import { readFile, realpath, stat } from 'node:fs/promises';
import { dirname, extname, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { createModel, fail } from '../tests/fixtures/model.mjs';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const LOOPBACK = new Set(['127.0.0.1', 'localhost', '::1', '[::1]']);
const MIME = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8', '.css': 'text/css; charset=utf-8', '.svg': 'image/svg+xml',
  '.json': 'application/json', '.png': 'image/png', '.ico': 'image/x-icon', '.woff2': 'font/woff2', '.txt': 'text/plain; charset=utf-8' };
function reply(res, status, body, headers = {}) {
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff', ...headers });
  res.end(Buffer.isBuffer(body) || typeof body === 'string' ? body : JSON.stringify(body));
}
async function readBody(req) {
  const chunks = []; let length = 0;
  for await (const chunk of req) {
    length += chunk.length; if (length > 1 << 20) fail('Request is too large', 413);
    chunks.push(chunk);
  }
  if (!length) return {};
  try { const value = JSON.parse(Buffer.concat(chunks).toString());
    if (!value || typeof value !== 'object' || Array.isArray(value)) fail('Expected a JSON object');
    return value;
  } catch { fail('Invalid JSON body'); }
}
function verifyLocal(req) {
  let host;
  try { host = new URL(`http://${req.headers.host}`); } catch { fail('forbidden', 403); }
  if (!LOOPBACK.has(host.hostname)) fail('forbidden', 403);
  if (req.headers.origin) {
    if (req.url.startsWith('/ext/')) {
      if (!/^(chrome-extension|moz-extension|safari-web-extension):\/\//.test(req.headers.origin)) fail('forbidden', 403);
      return;
    }
    let origin; try { origin = new URL(req.headers.origin); } catch { fail('forbidden', 403); }
    if (origin.protocol !== 'http:' || !LOOPBACK.has(origin.hostname) || origin.port !== host.port) fail('forbidden', 403);
  }
}
// One stored ZIP entry, fixed DOS time/date and CRC, entirely in memory.
function diagnosticsZip() {
  const name = Buffer.from('fixture-diagnostics.txt');
  const data = Buffer.from('Synthetic NetMerge diagnostics. No logs or settings from this computer.\n');
  let crc = 0xffffffff;
  for (const byte of data) { crc ^= byte; for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ ((crc & 1) ? 0xedb88320 : 0); }
  crc = (crc ^ 0xffffffff) >>> 0;
  const local = Buffer.alloc(30); local.writeUInt32LE(0x04034b50); local.writeUInt16LE(20, 4);
  local.writeUInt16LE(0x21, 12); local.writeUInt32LE(crc, 14); local.writeUInt32LE(data.length, 18);
  local.writeUInt32LE(data.length, 22); local.writeUInt16LE(name.length, 26);
  const central = Buffer.alloc(46); central.writeUInt32LE(0x02014b50); central.writeUInt16LE(20, 4); central.writeUInt16LE(20, 6);
  central.writeUInt16LE(0x21, 14); central.writeUInt32LE(crc, 16); central.writeUInt32LE(data.length, 20);
  central.writeUInt32LE(data.length, 24); central.writeUInt16LE(name.length, 28);
  const end = Buffer.alloc(22); end.writeUInt32LE(0x06054b50); end.writeUInt16LE(1, 8); end.writeUInt16LE(1, 10);
  end.writeUInt32LE(central.length + name.length, 12); end.writeUInt32LE(local.length + name.length + data.length, 16);
  return Buffer.concat([local, name, data, central, name, end]);
}

export function createFixtureServer({ root = ROOT } = {}) {
  let model = createModel(), offline = false, requests = [], faults = [], nextRequest = 1;
  const server = createServer(async (req, res) => {
    let record;
    try {
      verifyLocal(req);
      const url = new URL(req.url, `http://${req.headers.host}`), path = url.pathname;
      if (path === '/__test/health') return reply(res, 200, { fixture: 'netmerge', ready: true });
      if (path.startsWith('/__test/')) {
        if (req.headers['x-netmerge-test'] !== '1') fail('Test controls require X-NetMerge-Test: 1', 403);
        if (path === '/__test/requests' && req.method === 'GET') return reply(res, 200, { requests });
        if (req.method !== 'POST') fail('POST only', 405);
        const body = await readBody(req);
        if (path === '/__test/reset') {
          const scenario = body.scenario || 'default';
          const next = createModel(scenario); model = next; offline = scenario === 'offline';
          requests = []; faults = []; nextRequest = 1;
        } else if (path === '/__test/scenario') {
          const scenario = body.scenario || 'default';
          if (!['offline', 'default'].includes(scenario)) model = createModel(scenario);
          offline = scenario === 'offline';
        } else if (path === '/__test/requests') { requests = []; nextRequest = 1; }
        else if (path === '/__test/state') model.patch(body);
        else if (path === '/__test/fault') {
          if (typeof body.path !== 'string' || !body.path.startsWith('/api/')) fail('A fixture API path is required');
          if (body.status !== undefined && (!Number.isInteger(body.status) || body.status < 200 || body.status > 599)) fail('Invalid HTTP status');
          if (body.delayMs !== undefined && (!Number.isFinite(body.delayMs) || body.delayMs < 0 || body.delayMs > 30000)) fail('delayMs must be 0..30000');
          faults = faults.filter(f => f.path !== body.path || f.method !== body.method);
          if (body.times !== 0) faults.push({ ...body, remaining: body.times ?? Infinity });
        } else fail('Unknown test control', 404);
        return reply(res, 200, { ok: true });
      }
      record = { sequence: nextRequest++, method: req.method, path, query: Object.fromEntries(url.searchParams), body: null,
        headers: Object.fromEntries(['content-type', 'x-netmerge', 'origin'].filter(k => req.headers[k] !== undefined).map(k => [k, req.headers[k]])), status: null };
      requests.push(record);
      if (requests.length > 20000) requests.shift();
      if (path.startsWith('/ext/')) {
        if (!model.data.state.downloadsOn) fail('The download manager is off', 404);
        if (req.headers.origin) {
          res.setHeader('Access-Control-Allow-Origin', req.headers.origin);
          res.setHeader('Vary', 'Origin');
          res.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type');
          res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
          res.setHeader('Access-Control-Allow-Private-Network', 'true');
        }
        if (req.method === 'OPTIONS') { record.status = 204; res.writeHead(204); return res.end(); }
      }
      const api = path.startsWith('/api/') || path.startsWith('/ext/');
      if (api) {
        if (!['GET', 'POST'].includes(req.method)) fail('Method not allowed', 405);
        if (req.method === 'POST') {
          if (path.startsWith('/api/') && req.headers['x-netmerge'] !== '1') fail('forbidden', 403);
          record.body = await readBody(req);
        }
        if (offline) fail('Fixture server is offline', 503);
        const fault = faults.find(f => f.path === path && (!f.method || f.method === req.method) && f.remaining > 0);
        if (fault) fault.remaining--;
        let status = fault?.status || 200, body;
        const headers = {};
        if (fault && 'body' in fault) body = fault.body;
        else if (status >= 400) body = { error: fault?.error || 'Injected fixture failure' };
        else if (path === '/api/diagnostics' && req.method === 'GET') {
          body = diagnosticsZip(); headers['Content-Type'] = 'application/zip'; headers['Content-Disposition'] = 'attachment; filename="netmerge-fixture-diagnostics.zip"';
        } else {
          body = model.dispatch(path, req.method, record.body, url.searchParams, req.headers);
          if (path === '/api/backup') headers['Content-Disposition'] = 'attachment; filename="netmerge-settings-2026-11-09.json"';
        }
        // Snapshot before delaying so later mutations cannot rewrite an earlier GET response.
        body = Buffer.isBuffer(body) ? body : JSON.stringify(body);
        if (fault?.delayMs) await new Promise(resolve => setTimeout(resolve, fault.delayMs));
        record.status = status; return reply(res, status, body, headers);
      }
      if (!['GET', 'HEAD'].includes(req.method)) fail('Method not allowed', 405);
      const extension = path.startsWith('/extension/');
      const directory = await realpath(resolve(root, extension ? 'extension' : 'ui'));
      let relative;
      try { relative = decodeURIComponent(extension ? path.slice('/extension/'.length) : path.slice(1)); } catch { fail('Bad path'); }
      if (!relative) relative = 'index.html';
      if (relative.split(/[\\/]/).some(part => part.startsWith('.')) || relative.includes('\0')) fail('Not found', 404);
      const target = resolve(directory, relative);
      if (!target.startsWith(directory + sep)) fail('Not found', 404);
      let real, contents;
      try { real = await realpath(target); if (!real.startsWith(directory + sep) || !(await stat(real)).isFile()) fail('Not found', 404); contents = await readFile(real); }
      catch { fail('Not found', 404); }
      record.status = 200;
      reply(res, 200, req.method === 'HEAD' ? '' : contents, { 'Content-Type': MIME[extname(real)] || 'application/octet-stream' });
    } catch (error) {
      const status = error.status || 400;
      if (record) record.status = status;
      if (!res.headersSent) reply(res, status, { error: error.message });
      else res.end();
    }
  });
  return server;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const port = Number(process.env.NETMERGE_FIXTURE_PORT || process.env.PORT || 4173);
  const host = process.env.NETMERGE_FIXTURE_HOST || '127.0.0.1';
  if (!LOOPBACK.has(host) || !Number.isInteger(port) || port < 1 || port > 65535) throw new Error('Choose a loopback host and a port from 1 to 65535');
  const server = createFixtureServer();
  server.on('error', error => { process.stderr.write(`Fixture server: ${error.message}\n`); process.exitCode = 1; });
  server.listen(port, host, () => process.stdout.write(`NetMerge fixture server: http://${host.includes(':') ? `[${host}]` : host}:${port}\n`));
  for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => { server.closeAllConnections(); server.close(); });
}
