import { test, expect, navigate, waitForRead, mutate } from '../fixtures/browser.mjs';
import { createFixture, FAKE_PASSWORD, NOW } from '../fixtures/data.mjs';

const BASE = createFixture();

for (const [id, key, route, value] of [
  ['autoTog', 'autoStart', 'settings/general', true],
  ['launchTog', 'mergeOnLaunch', 'settings/general', false],
  ['callTog', 'noCallProtect', 'settings/general', true],
  ['updTog', 'noUpdateCheck', 'settings/maintenance', true],
  ['diagTog', 'noDiagnostics', 'settings/maintenance', true],
  ['debugTog', 'debugLog', 'settings/advanced', true],
]) {
  test(`${id} sends complete settings and maps ${key} correctly`, async ({ page, fixture }) => {
    await navigate(page, route);
    await expect(page.locator(`#${id}`)).toHaveAttribute('aria-checked', String(['noCallProtect', 'noUpdateCheck', 'noDiagnostics'].includes(key) ? !BASE.state.settings[key] : BASE.state.settings[key]));
    const before = (await fixture.read('state')).settings;
    await mutate(page, 'settings', { ...before, [key]: value }, () => page.locator(`#${id}`).click());
    expect((await fixture.read('state')).settings).toEqual({ ...before, [key]: value });
  });
}

test('settings writes read current state and preserve concurrent changes to other sections', async ({ page, fixture }) => {
  await navigate(page, 'settings/general');
  await expect(page.locator('#autoTog')).toHaveAttribute('aria-checked', 'false');
  const links = structuredClone(BASE.state.settings.links);
  links[1] = { ...links[1], weight: 5, role: 'backup', quotaGB: 125 };
  await fixture.patch({ state: { settings: { links, stableDomains: ['external-edit.example.test'], noUpdateCheck: true } } });
  const fresh = (await fixture.read('state')).settings;
  await fixture.clear();
  await mutate(page, 'settings', { ...fresh, autoStart: true }, () => page.locator('#autoTog').click());
  const requests = await fixture.requests();
  const writeIndex = requests.findIndex(r => r.method === 'POST' && r.path === '/api/settings');
  expect(requests.slice(0, writeIndex).some(r => r.path === '/api/state' && r.method === 'GET')).toBe(true);
  expect((await fixture.read('state')).settings.links).toEqual(links);
});

test('routing edits retain a dirty input across polling and save the entire settings object', async ({ page, fixture }) => {
  await navigate(page, 'protection/routing');
  await page.locator('#domInput').fill('draft.example.test');
  await page.locator('#pageTitle').click();
  await waitForRead(page, 'state', () => fixture.patch({ state: { snap: { uptime: 3601, totalDown: 5242880 } } }));
  await expect(page.locator('#domInput')).toHaveValue('draft.example.test');
  const before = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...before, stableDomains: [...before.stableDomains, 'draft.example.test'] }, () => page.locator('#domAdd').click());
  expect((await fixture.read('state')).settings.stableDomains).toContain('draft.example.test');
});

test('app rules preserve unrelated settings and use the exact process and target contract', async ({ page, fixture }) => {
  await navigate(page, 'apps/rules');
  await page.locator('#appInput').fill('fixture-player.exe');
  await page.locator('#appTarget').selectOption('link:Wi-Fi');
  const before = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...before, appRules: [...before.appRules, { process: 'fixture-player.exe', target: 'link:Wi-Fi' }] }, () => page.locator('#appAdd').click());
  await expect(page.locator('#rulesTbl')).toContainText('fixture-player.exe');
});

test('allowance settings preserve adapter identity, role and weights', async ({ page, fixture }) => {
  await navigate(page, 'usage/allowances');
  await expect(page.locator('#qReset')).toBeVisible();
  const before = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...before, quotaAction: 'leave' }, () => page.locator('#qAct').selectOption('leave'));
  const fresh = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...fresh, quotaReset: 15 }, () => page.locator('#qReset').selectOption('15'));
  expect((await fixture.read('state')).settings.links).toEqual(before.links);
});

test('speed test and share application use empty action bodies and confirmed results', async ({ page, fixture }) => {
  await navigate(page, 'connections/speed-test');
  await mutate(page, 'speedtest', {}, () => page.locator('#testBtn').click());
  await page.locator('#testDetails > summary').click();
  await expect(page.locator('#applyW')).toBeVisible({ timeout: 10_000 });
  await mutate(page, 'applyweights', {}, () => page.locator('#applyW').click());
  const state = await fixture.read('state');
  expect(state.test.running).toBe(false);
  expect(state.settings.links.map(l => l.weight)).toEqual([4, 1]);
});

test('presets save, apply, restore and schedule through their action-specific endpoint', async ({ page, fixture }) => {
  await navigate(page, 'connections/presets');
  await page.locator('#prName').fill('Fixture evening');
  await mutate(page, 'presets', { action: 'save', name: 'Fixture evening' }, () => page.locator('#prSave').click());
  await mutate(page, 'presets', { action: 'apply', id: 'safe' }, () => page.locator('[data-pa="safe"]').click());
  expect((await fixture.read('state')).settings.mode).toBe('stable');
  await mutate(page, 'presets', { action: 'restore' }, () => page.locator('#prRestore').click());
  expect((await fixture.read('state')).settings.mode).toBe('fast');
  await mutate(page, 'presets', { action: 'schedule', schedule: { enabled: true, from: '22:00', to: '06:00', preset: 'work' } }, () => page.locator('#prScOn').click());
});

test('blocking keeps unsaved text through resource refresh and saves only the chosen list', async ({ page, fixture }) => {
  await navigate(page, 'protection/blocking');
  const draft = 'blocked.example.test\ntracker.example.test';
  await page.locator('#blkMine').fill(draft);
  await page.locator('#pageTitle').click();
  await waitForRead(page, 'block', () => fixture.patch({ block: { today: 24 } }));
  await expect(page.locator('#blkMine')).toHaveValue(draft);
  await mutate(page, 'block', { blocklist: draft }, () => page.locator('#blkSaveMine').click());
  expect((await fixture.read('block')).allowlist).toBe(BASE.block.allowlist);
  await mutate(page, 'block', { enabled: false }, () => page.locator('#blkTog').click());
  await mutate(page, 'block/update', {}, () => page.locator('#blkUpdate').click());
});

test('custom DNS sends explicit fields and retains draft addresses while polling', async ({ page, fixture }) => {
  await navigate(page, 'protection/dns');
  await page.locator('#dnsProv').selectOption('custom');
  await page.locator('#dnsC0').fill('192.0.2.53');
  await page.locator('#dnsC1').fill('192.0.2.54');
  await page.locator('#pageTitle').click();
  await waitForRead(page, 'dns');
  await expect(page.locator('#dnsC0')).toHaveValue('192.0.2.53');
  await page.locator('#dnsDom').fill('corp.example.test');
  await page.locator('#dnsDomAdd').click();
  await mutate(page, 'dns', { action: 'set', provider: 'custom', secure: false, custom: ['192.0.2.53', '192.0.2.54'], domains: ['corp.example.test'] }, () => page.locator('#dnsApply').click());
  expect((await fixture.read('state')).settings.dnsDomains).toEqual(['corp.example.test']);
  await mutate(page, 'dns', { action: 'reset' }, () => page.locator('#dnsReset').click());
  await expect(page.locator('#dnsProv')).toHaveValue('windows');
});

test('sharing enables and rotates only explicit fake credentials', async ({ page, fixture }) => {
  await navigate(page, 'sharing/connect');
  await mutate(page, 'share', { enabled: true }, () => page.locator('#shTog').click());
  await expect(page.locator('#shTog')).toHaveAttribute('aria-checked', 'true');
  const rotate = page.locator('#shNew');
  if (!(await rotate.isVisible())) await page.locator('details').filter({ has: rotate }).locator('summary').click();
  await mutate(page, 'share', { newPass: true }, async () => {
    await rotate.click();
    await page.getByRole('dialog', { name: 'Create a new sharing password?' }).getByRole('button', { name: 'Create new password', exact: true }).click();
  });
  expect((await fixture.read('share')).pass).toBe(`${FAKE_PASSWORD}-1`);
  expect(page.url()).not.toContain('FAKE-TEST');
  expect(await page.evaluate(() => JSON.stringify({ ...localStorage }))).not.toContain('FAKE-TEST');
});

test('manual log refresh keeps the selected source and renders log markup as text', async ({ page, fixture }) => {
  await fixture.patch({ logs: { engine: '<img src=x onerror="window.fixtureInjection=true">\nSynthetic engine log' } });
  await navigate(page, 'activity/logs');
  await waitForRead(page, 'log', () => page.locator('#logSel [data-v="engine"]').click());
  await expect(page.locator('#logBox')).toContainText('<img src=x');
  await expect(page.locator('#logBox img')).toHaveCount(0);
  await waitForRead(page, 'log', () => page.locator('#logRefresh').click());
  expect((await fixture.requests()).filter(r => r.path === '/api/log').at(-1).query).toEqual({ f: 'engine' });
});

test('maintenance uses inert update actions and a sanitized backup download', async ({ page, fixture }) => {
  await navigate(page, 'settings/maintenance');
  await mutate(page, 'update', { action: 'check' }, () => page.locator('#updCheck').click());
  const download = page.waitForEvent('download');
  await page.locator('a[href="/api/backup"]').click();
  expect((await download).suggestedFilename()).toBe('netmerge-settings-2026-11-09.json');
  await expect.poll(async () => (await fixture.read('update')).checked).toBe(NOW);
});

test('the engine is named as built in, with its version', async ({ page, fixture }) => {
  await navigate(page, 'settings/advanced');
  await expect(page.locator('#engineVer')).toHaveText('Built into NetMerge, version 0.0.0-fixture (running)');
  await expect(page.locator('#engSel')).toHaveCount(0);
  await fixture.patch({ state: { snap: { version: '' } } });
  await expect(page.locator('#engineVer')).toHaveText('Not running');
});

test('settings has an About tab linking to nisarworks.com and the source', async ({ page }) => {
  await navigate(page, 'settings/about');
  await expect(page.locator('#aboutCard')).toBeVisible();
  await expect(page.locator('#aboutCard')).toContainText('Made by Nisar Ahmad');
  await expect(page.locator('#aboutSite')).toHaveAttribute('href', 'https://nisarworks.com/');
  await expect(page.locator('#aboutSite')).toHaveAttribute('target', '_blank');
  await expect(page.locator('#aboutRepo')).toHaveAttribute('href', 'https://github.com/nisar87/netmerge');
});

test('clicking a theme picture chooses that theme', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' });
  await navigate(page, 'settings/appearance');
  const html = page.locator('html');
  await page.locator('.theme-preview[data-v="dark"]').click();
  await expect(html).toHaveAttribute('data-theme', 'dark');
  await expect(page.locator('.theme-preview[data-v="dark"]')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#theme button[data-v="dark"]')).toHaveAttribute('aria-pressed', 'true');
  await page.locator('.theme-preview[data-v="light"]').click();
  await expect(html).toHaveAttribute('data-theme', 'light');
  await expect(page.locator('.theme-preview[data-v="dark"]')).toHaveAttribute('aria-pressed', 'false');
  await page.locator('.theme-preview[data-v="auto"]').click();
  await expect(html).toHaveAttribute('data-preference', 'auto');
  await expect(page.locator('.theme-preview[data-v="auto"]')).toHaveAttribute('aria-pressed', 'true');
  await page.locator('#theme button[data-v="dark"]').click();
  await expect(page.locator('.theme-preview[data-v="dark"]')).toHaveAttribute('aria-pressed', 'true');
});

test('the setup guide has a Send diagnostics checkbox, on by default, that saves the choice', async ({ page, fixture }) => {
  await navigate(page, 'settings/maintenance');
  await page.locator('#wizBtn').click();
  const box = page.locator('#wizDiag');
  await expect(box).toBeChecked();
  const before = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...before, noDiagnostics: true }, () => box.uncheck());
  expect((await fixture.read('state')).settings.noDiagnostics).toBe(true);
  await mutate(page, 'settings', { ...before, noDiagnostics: false }, () => box.check());
  expect((await fixture.read('state')).settings.noDiagnostics).toBe(false);
});

test('a script error in the window is sent to the log', async ({ page, fixture }) => {
  await navigate(page, 'overview');
  await page.evaluate(() => window.dispatchEvent(new ErrorEvent('error', { message: 'test failure in the window', filename: 'http://127.0.0.1/app/x.js', lineno: 3 })));
  await expect.poll(async () => JSON.stringify((await fixture.requests()).filter(r => String(r.path || r.url).includes('clientlog')))).toContain('test failure in the window');
});

test('addresses that skip NetMerge accept a range written with x, and explain what they refuse', async ({ page, fixture }) => {
  await navigate(page, 'protection/routing');
  const input = page.locator('#ipInput'), add = page.locator('#ipAdd'), toast = page.locator('#toast');
  const before = (await fixture.read('state')).settings.bypassIPs || [];
  // a wildcard in the middle is not one range: refused with an explanation, nothing saved
  await input.fill('198.x.100.5'); await add.click();
  await expect(toast).toContainText('Put x only at the end');
  await input.fill('300.1.2.3'); await add.click();
  await expect(toast).toContainText('0 to 255');
  expect((await fixture.read('state')).settings.bypassIPs || []).toEqual(before);
  // 198.51.100.x is saved as the range 198.51.100.0/24 and shown back as written
  await mutate(page, 'settings', { ...(await fixture.read('state')).settings, bypassIPs: [...before, '198.51.100.0/24'] }, async () => { await input.fill('198.51.100.x'); await add.click(); });
  expect((await fixture.read('state')).settings.bypassIPs).toContain('198.51.100.0/24');
  await expect(toast).toContainText('198.51.100.0 to 198.51.100.255');
  await expect(page.locator('#ipList .tag', { hasText: '198.51.100.x' })).toHaveAttribute('title', '198.51.100.0/24');
  // the same range again, written another way, is not added twice
  await input.fill('198.51.100.0/24'); await add.click();
  await expect(toast).toContainText('already on the list');
  // * works like x, and a single address stays a single address
  await mutate(page, 'settings', { ...(await fixture.read('state')).settings, bypassIPs: [...before, '198.51.100.0/24', '10.20.0.0/16'] }, async () => { await input.fill('10.20.*.*'); await add.click(); });
  await expect(page.locator('#ipList .tag', { hasText: '10.20.x.x' })).toBeVisible();
});
