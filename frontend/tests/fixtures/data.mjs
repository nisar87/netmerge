// Synthetic data shaped by the Go JSON types. Nothing is read from a live installation.
export const NOW = Date.parse('2026-11-09T12:00:00Z') / 1000;
export const FAKE_PASSWORD = 'FAKE-TEST-ONLY-password';
const clone = value => structuredClone(value);
const window = { enabled: false, from: '23:00', to: '06:00' };

export function createFixture(scenario = 'default') {
  const links = [
    { name: 'Ethernet', label: 'Home fibre', enabled: true, weight: 3, quotaGB: 0, role: '' },
    { name: 'Wi-Fi', label: 'Backup Wi-Fi', enabled: true, weight: 2, quotaGB: 100, role: 'busy' },
  ];
  const settings = {
    links, mode: 'fast', appRules: [{ process: 'mstsc.exe', target: 'stable' }],
    bypassIPs: ['192.0.2.0/24'], stableDomains: ['accounts.example.test'], vpnLink: '',
    autoStart: false, mergeOnLaunch: true, builtinDNS: false, seenApps: ['browser.exe', 'mstsc.exe'],
    secret: '', debugLog: false, tunMTU: 1500,
    block: { enabled: true, packs: { ads: true, pro: false, malware: false } }, stackChosen: false,
    share: { enabled: false, port: 1080, ssPort: 8388, user: 'fixture-user', pass: FAKE_PASSWORD },
    deviceNames: { '02:00:00:00:00:01': 'Fixture phone' }, quotaReset: 1, quotaAction: 'warn',
    setupDone: true, noCallProtect: false, noUpdateCheck: false, preset: '', customPresets: [],
    presetSchedule: { enabled: false, from: '22:00', to: '06:00', preset: 'work' },
    schedActive: false, schedPrev: '', dnsProvider: '', dnsSecure: false, dnsCustom: [], dnsDomains: [],
    dnsFallback: false, classicBalance: false,
  };
  const snapLinks = links.map(({ quotaGB, ...link }, i) => ({
    ...link, present: true, kind: i ? 'wifi' : 'ethernet', desc: `Synthetic ${i ? 'wireless' : 'wired'} adapter`,
    ip: `192.0.2.${10 + i}`, linkRate: i ? '300 Mbps' : '1 Gbps', down: (2 - i) * 1048576,
    up: (2 - i) * 131072, healthy: true, delay: 14 + i * 8, conns: 8 - i * 3,
    history: [1048576, 2097152, 1572864, 2097152], totalRx: 8e9, totalTx: 1e9,
    publicIP: i ? '203.0.113.20' : '198.51.100.10', slots: i ? 4 : 6,
    useDay: 2e9, useMonth: i ? 80e9 : 9e9, quotaOut: false, standby: false, slow: false,
    quality: { score: 94, label: 'Good', loss: 0, jitter: 3, avg: 16 }, portal: false,
  }));
  const events = [{ time: NOW - 60, kind: 'good', text: 'Fixture connections ready' },
    { time: NOW - 300, kind: 'info', text: 'Synthetic session started' }];
  const state = {
    snap: { running: true, version: 'nmengine-0.0.0-fixture', app: '0.0.0-fixture', error: '', uptime: 3600,
      links: snapLinks, apps: [{ process: 'browser.exe', down: 2097152, up: 131072, conns: 8,
        links: { Ethernet: 5, 'Wi-Fi': 3 }, rule: '' }], totalDown: 3145728, totalUp: 393216,
      peakDown: 6291456, conns: 13, total: [1048576, 3145728, 2097152, 3145728], devices: [],
      smart: true, activity: clone(events), sameIsp: [] },
    settings,
    test: { running: false, phase: '', progress: 0, live: 0, results: [], combined: 0, merged: 0,
      scaling: [], suggest: {}, finished: '0001-01-01T00:00:00Z', warning: '' },
    adapters: snapLinks.map((l, i) => ({ name: l.name, index: i + 1, desc: l.desc, ip: l.ip,
      gateway: '192.0.2.1', kind: l.kind, linkRate: l.linkRate, up: true })),
    block: { enabled: true, today: 12, total: 120 }, conflict: '', legacyDownloads: 0,
  };
  const presets = [
    { id: 'work', name: 'Work from home', desc: 'Keep meetings steady.', builtin: true, mode: 'fast', noCalls: false, block: false, mobile: 'busy', rules: [{ process: 'Teams.exe', target: 'stable' }] },
    { id: 'gaming', name: 'Gaming', desc: 'Keep games steady.', builtin: true, mode: 'fast', noCalls: false, mobile: 'busy', rules: [{ process: 'Discord.exe', target: 'stable' }] },
    { id: 'maxspeed', name: 'Max speed', desc: 'Use every connection.', builtin: true, mode: 'fast', noCalls: true, mobile: '' },
    { id: 'travel', name: 'Travel / limited data', desc: 'Save mobile data.', builtin: true, mode: 'fast', noCalls: false, mobile: 'backup' },
    { id: 'safe', name: 'Safe browsing', desc: 'Keep websites steady.', builtin: true, mode: 'stable', noCalls: false, block: true, mobile: '' },
  ];
  const providers = [
    ['cloudflare', 'Cloudflare (1.1.1.1)', ['1.1.1.1', '1.0.0.1']],
    ['google', 'Google (8.8.8.8)', ['8.8.8.8', '8.8.4.4']],
    ['quad9', 'Quad9 (9.9.9.9)', ['9.9.9.9', '149.112.112.112']],
    ['opendns', 'OpenDNS', ['208.67.222.222', '208.67.220.220']],
    ['adguard', 'AdGuard DNS', ['94.140.14.14', '94.140.15.15']],
  ].map(([id, name, plain]) => ({ id, name, desc: 'Synthetic provider metadata; no requests are sent.', plain, doh: plain.map(ip => `https://${ip}/dns-query`) }));
  const fixture = { state,
    block: { enabled: true, packs: [{ id: 'starter', name: 'Starter list', desc: 'Built-in fixture list', enabled: true, builtin: true, count: 100 },
      ...['ads', 'pro', 'malware'].map(id => ({ id, name: id === 'ads' ? 'Ads & trackers' : id === 'pro' ? 'Ads, trackers & more' : 'Malware & phishing', desc: 'Synthetic pack', enabled: id === 'ads', count: 200, updated: NOW - 86400, error: '', updating: false }))],
      blocklist: 'blocked.example.test', allowlist: 'allowed.example.test', today: 12, total: 120,
      recent: [{ time: NOW - 10, host: 'ads.example.test', app: 'browser.exe', count: 3 }], top: [{ host: 'ads.example.test', count: 3 }] },
    share: { ...settings.share, cipher: 'aes-128-gcm', addresses: [{ iface: 'Ethernet', ip: '192.0.2.10' }],
      running: true, publicNets: [], devices: [{ id: '02:00:00:00:00:01', mac: '02:00:00:00:00:01', name: 'Fixture phone', source: 'you', host: '', vendor: '', private: true, ip: '192.0.2.30', conns: 2, down: 1024, up: 128 }] },
    usage: { links: links.map((l, i) => ({ name: l.name, label: l.label, enabled: l.enabled, quotaGB: l.quotaGB, used: i ? 80e9 : 9e9, rx: i ? 72e9 : 8e9, tx: i ? 8e9 : 1e9, out: false })),
      days: [{ date: '2026-11-09', links: { Ethernet: [2e9, 1e8], 'Wi-Fi': [1e9, 5e7] } }], start: '2026-11-01', end: '2026-12-01', reset: 1, action: 'warn' },
    presets: { presets, active: '', schedule: clone(settings.presetSchedule), scheduled: false },
    dns: { dns: { provider: 'windows', secure: false, custom: [], domains: [], fallback: false }, providers,
      test: { running: false, progress: 0, total: 0, done: 0, lines: [], lineNames: {}, rows: [] } },
    // The production history records one sample per minute, already in Mbps.
    events: { events }, history: { points: Array.from({ length: 1440 }, (_, i) => ({ t: NOW - (1439 - i) * 60, d: { Ethernet: 16 + 5 * Math.sin(i / 41), 'Wi-Fi': 8 + 3 * Math.sin(i / 31) }, u: { Ethernet: 2, 'Wi-Fi': 1 } })) },
    update: { current: '0.0.0-fixture', latest: '0.0.0-fixture', notes: 'Synthetic release notes.', page: 'https://example.test/releases', available: false, checked: NOW - 60, error: '', status: 'idle', progress: 0, repo: 'example/netmerge-fixture', auto: true },
    logs: { app: '2026-11-09 12:00:00 Fixture ready\n', engine: 'Fixture engine: running\n', rates: 'Ethernet down=2097152 up=262144\n' },
  };
  if (scenario === 'one-connection') {
    state.snap.links.splice(1); settings.links.splice(1); state.adapters.splice(1);
    fixture.usage.links.splice(1);
    state.snap.totalDown = state.snap.links[0].down; state.snap.totalUp = state.snap.links[0].up;
    state.snap.conns = state.snap.links[0].conns;
  } else if (scenario === 'null') {
    for (const key of ['links', 'apps', 'devices', 'activity', 'sameIsp', 'total']) state.snap[key] = null;
    for (const key of ['links', 'appRules', 'bypassIPs', 'stableDomains', 'seenApps', 'customPresets']) settings[key] = null;
    state.adapters = null; state.test.results = null; state.test.scaling = null; state.test.suggest = null;
    fixture.events.events = null; fixture.history.points = null;
    fixture.usage.links = null; fixture.usage.days = null;
    fixture.block.recent = null; fixture.block.top = null;
    fixture.share.addresses = null; fixture.share.devices = null; fixture.share.publicNets = null;
  } else if (scenario === 'empty') {
    state.snap.links = []; settings.links = []; state.adapters = []; state.snap.apps = [];
    state.snap.totalDown = 0; state.snap.totalUp = 0; state.snap.conns = 0;
    fixture.usage.links = [];
  } else if (scenario === 'off') {
    state.snap.running = false; state.snap.totalDown = 0; state.snap.totalUp = 0; state.snap.conns = 0;
    for (const link of state.snap.links) Object.assign(link, { healthy: null, down: 0, up: 0, conns: 0 });
  } else if (!['default', 'offline'].includes(scenario)) throw new Error(`Unknown fixture scenario: ${scenario}`);
  return fixture;
}
