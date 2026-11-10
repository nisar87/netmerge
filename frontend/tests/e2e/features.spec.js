import { test, expect, navigate, waitForRead, mutate, enableDownloads } from '../fixtures/browser.mjs';
import { createFixture, FAKE_PASSWORD, NOW } from '../fixtures/data.mjs';

const BASE = createFixture();
const downloadAction = (page, action, id) => page.locator(`[data-dl="${action}"][data-id="${id}"]`);

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

test('single download probes metadata then submits a distinct add request', async ({ page, fixture }) => {
  await enableDownloads(fixture); await navigate(page, 'downloads/transfers');
  const url = 'https://downloads.example.test/new-test.zip';
  await page.locator('#dlNew').click();
  await page.locator('#dlUrl').fill(url);
  await mutate(page, 'downloads', { action: 'probe', url }, () => page.locator('#dlAddBtn').click());
  await expect(page.getByRole('dialog')).toBeVisible();
  await page.locator('#dlName').fill('named-fixture.zip');
  await mutate(page, 'downloads', { action: 'add', url, start: false, name: 'named-fixture.zip', conns: 0, limit: 0, queue: '' }, () => page.locator('#dlLater').click());
  await expect(page.getByRole('dialog')).toHaveCount(0);
  const added = (await fixture.read('downloads')).items.find(i => i.url === url);
  expect(added).toMatchObject({ name: 'named-fixture.zip', state: 'paused' });
  await expect(page.getByText('named-fixture.zip', { exact: true })).toBeVisible();
});

test('download dialog Escape restores focus and does not queue anything', async ({ page, fixture }) => {
  await enableDownloads(fixture); await navigate(page, 'downloads/transfers');
  await page.locator('#dlNew').click();
  await page.locator('#dlUrl').fill('https://downloads.example.test/cancelled.zip');
  await page.locator('#dlAddBtn').click();
  await expect(page.locator('#dlName')).toBeVisible();
  await page.keyboard.press('Escape');
  await page.getByRole('alertdialog', { name: 'Discard changes' }).getByRole('button', { name: 'Discard', exact: true }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.locator('#dlNew')).toBeFocused();
  expect((await fixture.requests()).filter(r => r.body?.action === 'add')).toEqual([]);
});

test('download lifecycle actions preserve byte units and distinguish remove from delete-file', async ({ page, fixture }) => {
  await enableDownloads(fixture); await navigate(page, 'downloads/transfers');
  const id = 'fixture-0001';
  await mutate(page, 'downloads', { action: 'pause', id }, () => downloadAction(page, 'pause', id).click());
  await page.locator(`.dl-name[data-id="${id}"]`).click();
  await mutate(page, 'downloads', { action: 'limit', id, limitBytes: 1048576 }, () => page.locator(`[data-lim="${id}"]`).selectOption('1'));
  await mutate(page, 'downloads', { action: 'resume', id }, () => page.locator('#dlDlg').locator(`[data-dl="resume"][data-id="${id}"]`).click());
  await page.locator('#dlCancel').click();
  await expect(page.locator('#dlDlg')).toHaveCount(0);
  await mutate(page, 'downloads', { action: 'retry', id: 'fixture-0004' }, () => downloadAction(page, 'retry', 'fixture-0004').click());
  await page.locator('.dl-name[data-id="fixture-0003"]').click();
  await mutate(page, 'downloads', { action: 'remove', id: 'fixture-0003', deleteFiles: true }, async () => {
    await downloadAction(page, 'delfile', 'fixture-0003').click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Continue', exact: true }).click();
  });
  expect((await fixture.read('downloads')).items.some(i => i.id === 'fixture-0003')).toBe(false);
});

for (const [action, field] of [['notifyTog', 'noNotify'], ['torTog', 'noTorrents'], ['upTog', 'noUpload']]) {
  test(`download ${field} toggle preserves complete queues, rules and limits`, async ({ page, fixture }) => {
    await enableDownloads(fixture); await navigate(page, 'downloads/settings');
    const config = (await fixture.read('downloads')).config;
    await mutate(page, 'downloads', { action: 'config', config: { ...config, [field]: true } }, () => page.locator(`[data-dm="${action}"]`).click());
    expect((await fixture.read('downloads')).config).toEqual({ ...config, [field]: true });
  });
}

test('download folder draft survives unrelated refresh and saving retains complete config', async ({ page, fixture }) => {
  await enableDownloads(fixture); await navigate(page, 'downloads/settings');
  await page.locator('#dlDir').fill('C:\\Fixture\\Draft folder');
  await page.locator('#pageTitle').click();
  await waitForRead(page, 'downloads', () => fixture.patch({ downloads: { knownSites: 3 } }));
  await expect(page.locator('#dlDir')).toHaveValue('C:\\Fixture\\Draft folder');
  const config = (await fixture.read('downloads')).config;
  await mutate(page, 'downloads', { action: 'config', config: { ...config, dir: 'C:\\Fixture\\Draft folder' } }, () => page.locator('#dlDirSave').click());
});

test('queue configuration and start preserve their distinct mutation contracts', async ({ page, fixture }) => {
  await enableDownloads(fixture); await navigate(page, 'downloads/queues');
  const config = (await fixture.read('downloads')).config;
  await page.getByRole('button', { name: 'Manage queues', exact: true }).click();
  await page.getByRole('textbox', { name: 'Queue name', exact: true }).fill('Fixture overnight');
  const queues = config.queues.map(q => ({ ...q, name: 'Fixture overnight' }));
  await mutate(page, 'downloads', { action: 'config', config: { ...config, queues } }, () => page.locator('#qSave').click());
  await navigate(page, 'downloads/transfers'); await navigate(page, 'downloads/queues');
  await mutate(page, 'downloads', { action: 'startQueue', queue: 'night' }, () => page.locator('[data-start-queue="night"]').click());
});

test('browser pairing and interception settings use inverse extOff and no config overwrite', async ({ page, fixture }) => {
  await enableDownloads(fixture); await navigate(page, 'downloads/browser');
  await mutate(page, 'downloads', { action: 'extConfig', extMinMB: 25, extOff: false }, () => page.locator('#dlExtMin').selectOption('25'));
  await mutate(page, 'downloads', { action: 'extConfig', extMinMB: 25, extOff: true }, () => page.locator('#dlExtOn').click());
  await expect(page.locator('#dlExtOn')).toHaveAttribute('aria-checked', 'false');
  await mutate(page, 'downloads', { action: 'extCode' }, () => page.locator('#dlExtCode').click());
  await expect(page.locator('#dlExtCodeBox')).toHaveText('TEST1-TEST2');
  await page.keyboard.press('Escape');
  await mutate(page, 'downloads', { action: 'extRevoke', id: 'fixture-browser' }, async () => {
    await page.locator('[data-extrm="fixture-browser"]').click();
    await page.locator('#confirmDialog').getByRole('button', { name: 'Continue', exact: true }).click();
  });
  expect((await fixture.read('downloads')).config).toEqual(BASE.downloads.config);
});

test('1000 transfers render bounded rows and search retains text and selection on updates', async ({ page, fixture }) => {
  await fixture.reset('populated1000'); await navigate(page, 'downloads/transfers');
  const search = page.locator('#dlSearch'); await expect(search).toBeVisible();
  const selects = page.getByRole('checkbox', { name: /^Select fixture-file-/ });
  await expect.poll(() => selects.count()).toBeGreaterThan(0);
  expect(await selects.count()).toBeLessThanOrEqual(100);
  await search.fill('fixture-file-0999');
  await expect(page.getByText('fixture-file-0999.zip', { exact: true })).toBeVisible();
  await search.evaluate(input => input.setSelectionRange(3, 9));
  await waitForRead(page, 'downloads', () => fixture.patch({ downloads: { knownSites: 99 } }));
  await expect(search).toHaveValue('fixture-file-0999');
  expect(await search.evaluate(input => [input.selectionStart, input.selectionEnd])).toEqual([3, 9]);
  expect(await selects.count()).toBe(1);
});

test('a manager disabled externally recovers to its off state without a null-items crash', async ({ page, fixture }) => {
  await enableDownloads(fixture); await navigate(page, 'downloads/transfers');
  await expect(page.locator('#dlSearch')).toBeVisible();
  await fixture.post('downloads/switch', { on: false });
  await expect(page.locator('#dlSearch')).not.toBeVisible({ timeout: 10_000 });
  await navigate(page, 'downloads/transfers');
  await expect(page.locator('#pg-downloads').getByRole('button', { name: /enable download manager/i })).toBeVisible();
  await navigate(page, 'settings/general');
  await mutate(page, 'downloads/switch', { on: true }, () => page.locator('#dmTog').click());
  await navigate(page, 'downloads/transfers');
  await expect(page.locator('#dlSearch')).toBeVisible();
});

test('a failed download save retains the draft and can be retried explicitly', async ({ page, fixture }) => {
  await enableDownloads(fixture); await navigate(page, 'downloads/settings');
  await page.locator('#dlDir').fill('C:\\Fixture\\Retry folder');
  await fixture.fault({ path: '/api/downloads', method: 'POST', status: 500, times: 1, error: 'Fixture configuration failed' });
  await fixture.clear(); await page.locator('#dlDirSave').click();
  await expect(page.getByText('Fixture configuration failed', { exact: false }).first()).toBeVisible();
  await waitForRead(page, 'downloads');
  await expect(page.locator('#dlDir')).toHaveValue('C:\\Fixture\\Retry folder');
  expect((await fixture.requests()).filter(r => r.body?.action === 'config')).toHaveLength(1);
  const config = (await fixture.read('downloads')).config;
  await mutate(page, 'downloads', { action: 'config', config: { ...config, dir: 'C:\\Fixture\\Retry folder' } }, () => page.locator('#dlDirSave').click());
});

test('maintenance uses inert update actions and a sanitized backup download', async ({ page, fixture }) => {
  await navigate(page, 'settings/maintenance');
  await mutate(page, 'update', { action: 'check' }, () => page.locator('#updCheck').click());
  const download = page.waitForEvent('download');
  await page.locator('a[href="/api/backup"]').click();
  expect((await download).suggestedFilename()).toBe('netmerge-settings-2026-11-09.json');
  await expect.poll(async () => (await fixture.read('update')).checked).toBe(NOW);
});
