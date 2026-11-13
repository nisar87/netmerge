import { test, expect, navigate, waitForRead, mutate } from '../fixtures/browser.mjs';
import { createFixture, FAKE_PASSWORD, NOW } from '../fixtures/data.mjs';

const BASE = createFixture();

for (const [id, key, route, value] of [
  ['autoTog', 'autoStart', 'settings/general', true],
  ['launchTog', 'mergeOnLaunch', 'settings/general', false],
  ['callTog', 'noCallProtect', 'settings/general', true],
  ['updTog', 'noUpdateCheck', 'settings/maintenance', true],
  ['debugTog', 'debugLog', 'settings/advanced', true],
]) {
  test(`${id} sends complete settings and maps ${key} correctly`, async ({ page, fixture }) => {
    await navigate(page, route);
    await expect(page.locator(`#${id}`)).toHaveAttribute('aria-checked', String(['noCallProtect', 'noUpdateCheck'].includes(key) ? !BASE.state.settings[key] : BASE.state.settings[key]));
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

test('network engine choice is saved and the running engine is named', async ({ page, fixture }) => {
  await navigate(page, 'settings/advanced');
  await expect(page.locator('#engSel')).toHaveValue('');
  await expect(page.locator('#engineVer')).toHaveText('mihomo fixture-engine (running)');
  const before = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...before, engine: 'netmerge' }, () => page.locator('#engSel').selectOption('netmerge'));
  expect((await fixture.read('state')).settings.engine).toBe('netmerge');
  await fixture.patch({ state: { snap: { version: 'nmengine-3.17.0', ownEngine: true } } });
  await expect(page.locator('#engineVer')).toHaveText('NetMerge engine 3.17.0 (running)');
  await fixture.patch({ state: { snap: { version: '1.19.33', ownEngine: false, fellBack: true } } });
  await expect(page.locator('#engineVer')).toContainText('did not start');
});

test('settings has an About tab linking to nisarworks.com and the source', async ({ page }) => {
  await navigate(page, 'settings/about');
  await expect(page.locator('#aboutCard')).toBeVisible();
  await expect(page.locator('#aboutCard')).toContainText('Made by Nisar Ahmad');
  await expect(page.locator('#aboutSite')).toHaveAttribute('href', 'https://nisarworks.com/');
  await expect(page.locator('#aboutSite')).toHaveAttribute('target', '_blank');
  await expect(page.locator('#aboutRepo')).toHaveAttribute('href', 'https://github.com/nisar87/netmerge');
});
