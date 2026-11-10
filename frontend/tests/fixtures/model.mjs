import { isIP } from 'node:net';
import { createFixture, NOW, FAKE_PASSWORD } from './data.mjs';

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
  let speedReads = 0, dnsReads = 0, updateReads = 0;
  const settings = () => data.state.settings;
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
      if (method === 'GET') {
        const resources = { '/api/state': data.state, '/api/block': data.block, '/api/usage': data.usage,
          '/api/share': data.share, '/api/presets': data.presets, '/api/dns': data.dns, '/api/events': data.events,
          '/api/history': data.history, '/api/update': data.update, '/api/ping': { pid: 4173 } };
        if (path in resources) return resources[path];
        if (path === '/api/log') return { text: data.logs[query.get('f')] ?? data.logs.app };
        if (path === '/api/backup') return safeBackup();
        if (path === '/api/legacy-downloads') {
          if (!data.state.legacyDownloads) fail('There is no download list from an earlier NetMerge version', 404);
          return { app: 'netmerge-downloads', version: 1, items: Array.from({ length: data.state.legacyDownloads }, (_, i) => ({ url: `https://files.example.test/old-${i + 1}.zip` })) };
        }
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
          mergeSettings(body, true); break;
        case '/api/quit': break; // Keep the fixture server alive; never signal processes.
        default: fail('Unknown fixture API route', 404);
      }
      sync(); return { ok: true };
    },
  };
}
