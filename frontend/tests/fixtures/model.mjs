import { isIP } from 'node:net';
import { createFixture, downloadItem, NOW, FAKE_PASSWORD, FAKE_TOKEN } from './data.mjs';

const clone = value => structuredClone(value);
export function fail(message, status = 400) { throw Object.assign(new Error(message), { status }); }
export function patch(target, changes) {
  for (const [key, value] of Object.entries(changes)) {
    if (['__proto__', 'constructor', 'prototype'].includes(key)) fail('Invalid patch key');
    if (value && !Array.isArray(value) && typeof value === 'object' && target[key] && !Array.isArray(target[key])) patch(target[key], value);
    else target[key] = clone(value);
  }
  return target;
}
const settingsFields = ['appRules', 'bypassIPs', 'stableDomains', 'vpnLink', 'autoStart', 'mergeOnLaunch',
  'builtinDNS', 'debugLog', 'classicBalance', 'noUpdateCheck', 'noCallProtect', 'quotaReset', 'quotaAction'];
const restoreFields = ['appRules', 'bypassIPs', 'stableDomains', 'mergeOnLaunch', 'builtinDNS', 'dnsProvider',
  'dnsSecure', 'dnsCustom', 'dnsDomains', 'noCallProtect', 'noUpdateCheck', 'classicBalance', 'quotaReset', 'quotaAction'];

export function createModel(scenario = 'default') {
  let data = createFixture(scenario), sequence = 1001, presetSequence = 1, passwordSequence = 0;
  let speedReads = 0, dnsReads = 0, updateReads = 0, paired = false, pairingCode = '';
  const settings = () => data.state.settings;
  const dl = () => data.downloads;
  function sync() {
    const s = settings(), sn = data.state.snap;
    for (const link of sn.links || []) {
      const configured = (s.links || []).find(l => l.name === link.name);
      if (configured) { const { quotaGB, ...visible } = configured; Object.assign(link, visible); }
    }
    data.block.enabled = s.block?.enabled ?? false;
    for (const pack of data.block.packs || []) if (!pack.builtin) pack.enabled = !!s.block?.packs?.[pack.id];
    data.state.block = { enabled: data.block.enabled, today: data.block.today, total: data.block.total };
    Object.assign(data.share, s.share, { running: sn.running });
    for (const device of data.share.devices || []) if (s.deviceNames?.[device.id]) device.name = s.deviceNames[device.id];
    for (const link of data.usage.links || []) {
      const configured = (s.links || []).find(l => l.name === link.name);
      if (configured) Object.assign(link, { label: configured.label, quotaGB: configured.quotaGB, enabled: configured.enabled });
      link.out = s.quotaAction === 'leave' && link.quotaGB > 0 && link.used >= link.quotaGB * 1e9;
    }
    data.usage.reset = s.quotaReset; data.usage.action = s.quotaAction;
    data.dns.dns = { provider: s.builtinDNS ? s.dnsProvider || 'builtin' : 'windows', secure: s.dnsSecure,
      custom: s.dnsCustom || [], domains: s.dnsDomains || [], fallback: s.dnsFallback };
    Object.assign(data.presets, { active: s.preset, schedule: s.presetSchedule ?? null, scheduled: s.schedActive });
    data.update.auto = !s.noUpdateCheck;
    dl().enabled = data.state.downloadsOn;
    dl().active = (dl().items || []).filter(i => i.state === 'downloading').length;
    dl().speed = (dl().items || []).reduce((total, i) => total + (i.state === 'downloading' ? i.speed : 0), 0);
    data.state.downloads = data.state.downloadsOn ? { active: dl().active, speed: dl().speed,
      queued: (dl().items || []).filter(i => i.state === 'queued').length,
      items: (dl().items || []).filter(i => i.state === 'downloading').slice(0, 3).map(({ name, done, size, speed }) => ({ name, done, size, speed })),
      total: (dl().items || []).length } : null;
  }
  function mergeSettings(body, restore = false) {
    const s = settings();
    if (!Array.isArray(body.links) && !restore) fail('bad settings');
    for (const link of s.links || []) {
      const incoming = (body.links || []).find(l => l.name === link.name);
      if (incoming) for (const key of ['label', 'enabled', 'weight', 'quotaGB', 'role']) if (key in incoming) link[key] = clone(incoming[key]);
    }
    for (const key of restore ? restoreFields : settingsFields) if (key in body) s[key] = clone(body[key]);
    if (body.mode === 'fast' || body.mode === 'stable' || (!restore && (s.links || []).some(l => body.mode === `link:${l.name}`))) s.mode = body.mode;
    if (!restore) {
      s.setupDone ||= body.setupDone === true;
      if (['', 'mixed', 'system', 'gvisor'].includes(body.tunStack) && body.tunStack !== s.tunStack) {
        s.tunStack = body.tunStack; s.stackChosen = true;
      }
    }
  }
  function safeBackup() {
    const output = clone(settings());
    output.secret = ''; if (output.share) output.share.pass = ''; delete output.deviceNames;
    if (data.state.downloadsOn) output.downloadConfig = clone(dl().config);
    return output;
  }
  function presetBackup() {
    const s = settings();
    return { mode: s.mode, noCallProtect: s.noCallProtect, appRules: clone(s.appRules), blockEnabled: s.block.enabled,
      roles: Object.fromEntries((s.links || []).map(l => [l.name, l.role])) };
  }
  function restorePreset() {
    const s = settings(), b = s.presetBackup;
    if (b) {
      s.mode = b.mode; s.noCallProtect = b.noCallProtect; s.appRules = clone(b.appRules); s.block.enabled = b.blockEnabled;
      for (const link of s.links || []) link.role = b.roles[link.name] || '';
    }
    s.preset = ''; delete s.presetBackup;
  }
  function presets(body) {
    const s = settings(), list = data.presets.presets;
    switch (body.action) {
      case 'apply': {
        const p = list.find(p => p.id === body.id); if (!p) fail('unknown preset', 404);
        s.presetBackup ||= presetBackup();
        const backup = clone(s.presetBackup); restorePreset(); s.presetBackup = backup;
        s.preset = p.id; s.mode = p.mode; s.noCallProtect = p.noCalls;
        if ('block' in p) s.block.enabled = p.block;
        s.appRules = [...(backup.appRules || []), ...(p.rules || []).filter(r => !(backup.appRules || []).some(b => b.process === r.process))];
        for (const link of s.links || []) if (data.state.snap.links?.find(l => l.name === link.name)?.kind === 'mobile') link.role = p.mobile;
        break;
      }
      case 'restore': restorePreset(); break;
      case 'save': {
        const name = body.name?.trim(); if (!name || name.length > 40) fail('Give the preset a name (up to 40 letters)');
        const p = { id: `my-fixture-${presetSequence++}`, name, desc: 'Your own preset.', builtin: false,
          mode: s.mode, noCalls: s.noCallProtect, mobile: '', block: s.block.enabled, rules: clone(s.appRules) };
        if ((s.customPresets || []).length < 12) { (s.customPresets ||= []).push(p); list.push(p); }
        break;
      }
      case 'delete':
        s.customPresets = (s.customPresets || []).filter(p => p.id !== body.id);
        data.presets.presets = list.filter(p => p.builtin || p.id !== body.id);
        if (s.preset === body.id) s.preset = '';
        break;
      case 'schedule':
        if (body.schedule && (!/^([01]\d|2[0-3]):[0-5]\d$/.test(body.schedule.from) || !/^([01]\d|2[0-3]):[0-5]\d$/.test(body.schedule.to) || !list.some(p => p.id === body.schedule.preset))) fail('Choose valid hours and a preset');
        s.presetSchedule = clone(body.schedule ?? null); break;
      default: fail('unknown action');
    }
    sync(); return data.presets;
  }
  function dns(body) {
    const s = settings();
    switch (body.action) {
      case 'test':
        if (data.dns.test.running) fail('A test is already running', 409);
        dnsReads = 2;
        Object.assign(data.dns.test, { running: true, progress: 0, total: 2, done: 0 }); break;
      case 'reset': s.builtinDNS = false; s.dnsProvider = ''; s.dnsFallback = false; break;
      case 'set': {
        const custom = (body.custom || []).filter(ip => isIP(ip) === 4);
        if (body.provider === 'custom' && !custom.length) fail('Enter at least one valid IPv4 address');
        if (!['custom', 'windows', ...data.dns.providers.map(p => p.id)].includes(body.provider)) fail('Unknown DNS provider');
        s.builtinDNS = body.provider !== 'windows'; s.dnsProvider = s.builtinDNS ? body.provider : ''; s.dnsFallback = false;
        if (s.builtinDNS) { s.dnsSecure = !!body.secure && body.provider !== 'custom'; s.dnsCustom = custom;
          s.dnsDomains = (body.domains || []).map(d => d.replace(/^\*\./, '').trim().toLowerCase()); }
        break;
      }
      default: fail('unknown action');
    }
    sync(); return data.dns;
  }
  function getItem(id) { return dl().items?.find(item => item.id === id) || fail('download not found'); }
  function cleanURL(url) {
    try {
      const parsed = new URL(url);
      if (!['http:', 'https:', 'ftp:', 'magnet:'].includes(parsed.protocol)) fail('Unsupported download URL');
      return parsed.href;
    } catch { fail('Enter a valid download link'); }
  }
  function add(body, restartStopped = false) {
    let urls = [...(body.url ? [body.url] : []), ...(body.urls || [])];
    if (!urls.length && body.torrent) urls = [''];
    if (!urls.length) fail('paste at least one link');
    urls = urls.flatMap(url => {
      const match = url.match(/\[(\d+)-(\d+)\]/);
      if (!match) return [url];
      const start = Number(match[1]), end = Number(match[2]);
      if (end < start || end - start >= 500) fail('too many links at once (500 at most)');
      return Array.from({ length: end - start + 1 }, (_, i) => url.replace(match[0], String(start + i).padStart(match[1].length, '0')));
    });
    if (urls.length > 500) fail('too many links at once (500 at most)');
    const added = []; let skipped = 0, restarted = 0;
    for (const raw of urls) {
      let url;
      try { url = body.torrent ? raw : cleanURL(raw); } catch (error) { if (urls.length === 1) throw error; continue; }
      const duplicate = dl().items?.findLast(item => item.url === url);
      if (!body.force && duplicate) {
        if (restartStopped && body.start !== false && !body.startAt && ['paused', 'error'].includes(duplicate.state)) {
          Object.assign(duplicate, { state: 'queued', err: '', detail: '', retryAt: 0 }); restarted++;
        } else skipped++;
        continue;
      }
      const torrent = !!body.torrent || url.startsWith('magnet:') || /\.torrent($|\?)/i.test(url);
      if (torrent && dl().config.noTorrents) fail('Torrents are disabled');
      const name = (urls.length === 1 && body.name) || (torrent ? 'fixture-torrent' : decodeURIComponent(new URL(url).pathname.split('/').pop()) || 'download.bin');
      const item = downloadItem(sequence++, body.start === false ? 'paused' : 'queued', {
        url, name, done: 0, bytes: {}, added: NOW, lines: body.lines || [], dir: body.dir || '',
        path: `${body.dir || dl().config.dir}\\${name}`, limit: body.limit || 0, startAt: body.startAt || 0,
        hash: body.hash || '', queue: body.queue || '', mirrors: body.mirrors || [], conns: body.conns || 0,
        height: body.height || 0, web: !!body.fromWeb, ...(torrent ? { kind: 'torrent', nFiles: 2, select: body.select || [true, true], category: 'Torrents' } : {}),
      });
      (dl().items ||= []).push(item); added.push(item);
    }
    if (!added.length && !skipped && !restarted) fail('none of the links could be used');
    return { ...(added.length ? { added } : { duplicate: true }), skipped, restarted, items: dl().items, config: dl().config };
  }
  const torrentFiles = () => [{ name: 'fixture-document.txt', size: 1024 }, { name: 'fixture-image.png', size: 2048 }];
  function downloads(body) {
    if (!data.state.downloadsOn) fail('The download manager is off. Turn it on in Settings.', 409);
    const items = () => dl().items || [];
    switch (body.action) {
      case 'probe': {
        const url = cleanURL(body.url), name = new URL(url).pathname.split('/').pop() || 'download.bin';
        const stream = /\.(m3u8|mpd)($|\?)/i.test(url);
        const hls = stream ? { segments: 24, seconds: 120, quality: `${body.height || 1080}p`, kind: 'mp4', audio: true, heights: [1080, 720, 480] } : null;
        return { ok: true, info: { url, finalUrl: url, name: stream ? name.replace(/\.(m3u8|mpd)$/i, '.mp4') : name, type: stream ? 'video stream' : 'application/octet-stream', size: stream ? -1 : 64 * 1048576,
          ranges: !stream, etag: 'fixture-etag', lastMod: 'Mon, 09 Nov 2026 12:00:00 GMT' }, hls,
          mux: stream && dl().ffmpeg, dir: dl().config.dir, category: 'Archives', lines: dl().lines.length, locked: false,
          duplicate: items().some(i => i.url === url), dupState: items().findLast(i => i.url === url)?.state || '', lineNames: dl().lines.map(l => l.name) };
      }
      case 'add': return add(body, true);
      case 'torrentInfo':
        if (!body.torrent) fail('Choose a torrent file');
        return { ok: true, name: 'fixture-torrent', size: 3072, files: torrentFiles(), private: false, trackers: 1, dir: dl().config.dir, duplicate: false };
      case 'files': { const item = getItem(body.id); return { files: torrentFiles(), select: item.select ?? null }; }
      case 'select': getItem(body.id).select = clone(body.select); break;
      case 'pause': Object.assign(getItem(body.id), { state: 'paused', speed: 0, lineStats: [] }); break;
      case 'resume': case 'retry': {
        const item = getItem(body.id);
        if (item.state !== 'done') Object.assign(item, { state: 'queued', speed: 0, err: '', detail: '', retryAt: 0 });
        break;
      }
      case 'remove': getItem(body.id); dl().items = items().filter(i => i.id !== body.id); break;
      case 'clear': dl().items = items().filter(i => i.state !== 'done'); break;
      case 'pauseAll': case 'resumeAll':
        for (const item of items()) if (item.state !== 'done') Object.assign(item, { state: body.action === 'pauseAll' ? 'paused' : 'queued', speed: 0, lineStats: [] });
        break;
      case 'open': case 'folder': getItem(body.id); break; // Deliberately inert: never open a path.
      case 'move': {
        getItem(body.id); const from = items().findIndex(i => i.id === body.id);
        const to = { up: Math.max(0, from - 1), down: Math.min(items().length - 1, from + 1), top: 0, bottom: items().length - 1 }[body.move];
        if (to === undefined) fail('unknown move');
        const [item] = dl().items.splice(from, 1); dl().items.splice(to, 0, item);
        dl().items.forEach((item, i) => item.order = i); break;
      }
      case 'limit': getItem(body.id).limit = Math.max(0, Number(body.limitBytes) || 0); break;
      case 'setQueue': getItem(body.id).queue = body.queue || ''; break;
      case 'setConns': getItem(body.id).conns = Number(body.conns) || 0; break;
      case 'setMirrors': getItem(body.id).mirrors = (body.urls || []).map(cleanURL); break;
      case 'relink': {
        const item = getItem(body.id); if (item.state === 'downloading') fail('Pause the download first');
        item.url = cleanURL(body.url); item.err = ''; item.detail = ''; break;
      }
      case 'cancelPower': dl().powerAt = 0; dl().config.afterAll = ''; break;
      case 'forgetHosts': dl().knownSites = 0; break;
      case 'config':
        if (!body.config || typeof body.config !== 'object') fail('missing settings');
        dl().config = clone(body.config);
        for (const queue of dl().config.queues || []) queue.id ||= `queue-fixture-${sequence++}`;
        for (const item of items()) if (item.queue && !dl().config.queues?.some(q => q.id === item.queue)) item.queue = '';
        break;
      case 'startQueue':
        for (const item of items()) if (item.queue === body.queue && item.state !== 'done') Object.assign(item, { state: 'queued', startAt: 0 });
        break;
      case 'extCode': pairingCode = 'TEST1-TEST2'; return { code: pairingCode, expires: NOW + 300, repo: 'example/netmerge-fixture' };
      case 'extRevoke': dl().ext.browsers = (dl().ext.browsers || []).filter(b => b.id !== body.id); paired = false; break;
      case 'extConfig': dl().ext.enabled = !body.extOff; dl().ext.minMB = Math.max(0, Number(body.extMinMB) || 0); break;
      case 'grab': {
        const url = cleanURL(body.url);
        return { page: url, links: ['zip', 'pdf', 'mp4'].filter(ext => !body.types?.length || body.types.includes(ext)).map(ext => ({ url: new URL(`fixture.${ext}`, url).href, name: `fixture.${ext}`, ext })) };
      }
      case 'export': return { file: { app: 'netmerge-downloads', version: 1, queues: clone(dl().config.queues),
        items: items().map(i => ({ url: i.url, name: i.name, dir: i.dir, queue: i.queue, mirrors: i.mirrors, hash: i.hash, finished: i.state === 'done' })) } };
      case 'import': {
        let parsed;
        try { parsed = JSON.parse(body.text); } catch { parsed = { items: String(body.text || '').split(/\r?\n/).filter(Boolean).map(url => ({ url })) }; }
        if (String(body.text || '').trim().startsWith('{') && parsed.app !== 'netmerge-downloads') fail('that is not a NetMerge download list');
        if (!Array.isArray(parsed.items) || !parsed.items.length) fail('No downloads in this file');
        let imported = 0, skipped = 0;
        for (const q of parsed.queues || []) if (!dl().config.queues.some(existing => existing.id === q.id)) dl().config.queues.push(clone(q));
        for (const item of parsed.items) {
          try { const result = add({ ...item, start: !item.finished }); imported += result.added?.length || 0; skipped += result.skipped || 0; } catch { skipped++; }
        }
        if (!imported) fail('nothing could be added (the links are already in the list or not valid)');
        return { imported, skipped, items: dl().items, config: dl().config };
      }
      default: fail('unknown action');
    }
    sync(); return dl();
  }
  function progress(path) {
    if (path === '/api/state' && speedReads > 0 && --speedReads === 0) {
      const results = (data.state.snap.links || []).map((l, i) => ({ name: l.name, label: l.label, mbps: 80 - i * 60, single: 40 - i * 10, ip: l.publicIP }));
      Object.assign(data.state.test, { running: false, phase: 'Done', progress: 1, live: 0, results,
        combined: 100, merged: 94, scaling: [{ streams: 1, mbps: 40 }, { streams: 4, mbps: 94 }],
        suggest: Object.fromEntries(results.map((r, i) => [r.name, i ? 1 : 4])), finished: new Date(NOW * 1000).toISOString() });
    }
    if (path === '/api/dns' && dnsReads > 0 && --dnsReads === 0) {
      const names = (settings().links || []).map(l => l.name);
      Object.assign(data.dns.test, { running: false, progress: 2, total: 2, done: NOW, lines: names,
        lineNames: Object.fromEntries((settings().links || []).map(l => [l.name, l.label])),
        rows: [{ id: 'cloudflare-plain', label: 'Cloudflare', provider: 'cloudflare', kind: 'plain', median: 12, fail: 0, hijack: false,
          lines: Object.fromEntries(names.map(n => [n, { median: 12, p90: 15, fail: 0, hijack: false, n: 12 }])) }],
        rec: { provider: 'cloudflare', secure: false, median: 12, current: 25, text: 'Cloudflare answered fastest in this fixture.', notes: [] } });
    }
    if (path === '/api/update' && updateReads > 0 && --updateReads === 0) Object.assign(data.update, { status: 'idle', checked: NOW, engineDl: '', enginePct: 100 });
  }
  return {
    get data() { sync(); return data; },
    patch(changes) { patch(data, changes); sync(); },
    dispatch(path, method, body, query, headers) {
      sync(); if (method === 'GET') progress(path);
      if (path.startsWith('/ext/')) {
        if (!data.state.downloadsOn) fail('The download manager is off', 404);
        if (path === '/ext/ping') return { app: 'netmerge', version: data.state.snap.app };
        if (path === '/ext/pair' && method === 'POST') {
          if (!pairingCode || String(body.code || body.Code).replaceAll('-', '') !== pairingCode.replaceAll('-', '')) fail('Invalid pairing code', 403);
          pairingCode = ''; paired = true;
          dl().ext.browsers.push({ id: 'fixture-paired', name: body.name || body.Name || 'Fixture browser', created: NOW, lastSeen: NOW });
          return { token: FAKE_TOKEN };
        }
        if (!paired || headers.authorization !== `Bearer ${FAKE_TOKEN}`) fail('Not paired', 401);
        if (path === '/ext/status') return { version: data.state.snap.app, enabled: dl().ext.enabled, minMB: dl().ext.minMB, active: dl().active, speed: dl().speed, lines: dl().lines.length };
        if (path === '/ext/unpair' && method === 'POST') { paired = false; return { ok: true }; }
        if (path === '/ext/add' && method === 'POST') { const r = add({ url: body.url, name: body.filename, fromWeb: true }); return r.duplicate ? { ok: true, duplicate: true } : { ok: true, id: r.added[0].id }; }
        fail('Unknown extension route', 404);
      }
      if (method === 'GET') {
        const resources = { '/api/state': data.state, '/api/block': data.block, '/api/usage': data.usage,
          '/api/share': data.share, '/api/presets': data.presets, '/api/dns': data.dns, '/api/events': data.events,
          '/api/history': data.history, '/api/update': data.update, '/api/ping': { pid: 4173 },
          '/api/downloads/switch': { enabled: data.state.downloadsOn },
          '/api/downloads': data.state.downloadsOn ? dl() : { enabled: false } };
        if (path in resources) return resources[path];
        if (path === '/api/log') return { text: data.logs[query.get('f')] ?? data.logs.app };
        if (path === '/api/backup') return safeBackup();
        fail('Unknown fixture API route', 404);
      }
      if (method !== 'POST') fail('Method not allowed', 405);
      switch (path) {
        case '/api/power':
          if (typeof body.on !== 'boolean') fail('on must be a boolean');
          data.state.snap.running = body.on; break;
        case '/api/settings': mergeSettings(body); break;
        case '/api/rescan': break;
        case '/api/usage': {
          if (body.action !== 'set' || ![body.downMB, body.upMB].every(n => Number.isFinite(n) && n >= 0 && n <= 1e7)) fail('bad request');
          const link = data.usage.links?.find(l => l.name === body.link), day = data.usage.days?.find(d => d.date === body.day);
          if (!link || !day) fail('Choose a known connection and day');
          day.links ||= {};
          const old = day.links[body.link] || [0, 0], next = [Math.trunc(body.downMB * 1e6), Math.trunc(body.upMB * 1e6)];
          day.links[body.link] = next;
          if (day.date >= data.usage.start && day.date < data.usage.end) {
            link.rx += next[0] - old[0]; link.tx += next[1] - old[1]; link.used = link.rx + link.tx;
          }
          sync(); return data.usage;
        }
        case '/api/speedtest':
          speedReads = 2; Object.assign(data.state.test, { running: true, phase: 'Testing fixture connections', progress: 0.5, live: 48 }); break;
        case '/api/applyweights':
          for (const link of settings().links || []) if (data.state.test.suggest?.[link.name]) link.weight = data.state.test.suggest[link.name];
          break;
        case '/api/linkforget': {
          if (data.state.snap.links?.some(l => l.name === body.name && l.present)) fail('Unplug the connection before forgetting it');
          settings().links = (settings().links || []).filter(l => l.name !== body.name);
          data.state.snap.links = (data.state.snap.links || []).filter(l => l.name !== body.name);
          settings().appRules = (settings().appRules || []).filter(r => r.target !== `link:${body.name}`);
          if (settings().vpnLink === body.name) settings().vpnLink = '';
          if (settings().mode === `link:${body.name}`) settings().mode = 'fast';
          break;
        }
        case '/api/block':
          if ('enabled' in body) settings().block.enabled = !!body.enabled;
          Object.assign(settings().block.packs, body.packs || {});
          for (const key of ['blocklist', 'allowlist']) if (key in body) data.block[key] = body[key];
          sync(); return data.block;
        case '/api/block/allow':
          if (!body.domain) fail('Enter a domain');
          data.block.allowlist = [...new Set([...data.block.allowlist.split('\n'), body.domain])].filter(Boolean).join('\n'); return data.block;
        case '/api/block/update': for (const pack of data.block.packs || []) if (!pack.builtin) Object.assign(pack, { updated: NOW, updating: false, error: '' }); break;
        case '/api/fixconflict': data.state.conflict = ''; break;
        case '/api/share':
          if ('enabled' in body) settings().share.enabled = !!body.enabled;
          if (body.newPass) settings().share.pass = `${FAKE_PASSWORD}-${++passwordSequence}`;
          if (body.name) (settings().deviceNames ||= {})[body.name.id] = body.name.name;
          sync(); return data.share;
        case '/api/presets': return presets(body);
        case '/api/dns': return dns(body);
        case '/api/update':
          if (!['check', 'install', 'engine'].includes(body.action)) fail('unknown action');
          updateReads = 2;
          if (body.action === 'engine') data.update.engineDl = 'downloading';
          else data.update.status = body.action === 'check' ? 'checking' : 'installing';
          return data.update;
        case '/api/restore':
          if (!body.mode && !body.links?.length && !body.appRules) fail('That does not look like a NetMerge settings file');
          mergeSettings(body, true); if (body.downloadConfig) dl().config = clone(body.downloadConfig); break;
        case '/api/quit': break; // Keep the fixture server alive; never signal processes.
        case '/api/downloads/switch':
          if (typeof body.on !== 'boolean') fail('on must be a boolean');
          if (!body.on) for (const item of dl().items || []) if (item.state === 'downloading') Object.assign(item, { state: 'paused', speed: 0, lineStats: [] });
          data.state.downloadsOn = body.on; sync(); return { enabled: body.on };
        case '/api/downloads': return downloads(body);
        default: fail('Unknown fixture API route', 404);
      }
      sync(); return { ok: true };
    },
  };
}
