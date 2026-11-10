import { test, expect, navigate, mutate } from '../fixtures/browser.mjs';

// Stop background schedules after mounting so each race has an explicit ordering.
// Requests still use the real scripts and synthetic server; no application methods are mocked.
async function openFeature(page, route, selector) {
  await navigate(page, route);
  await expect(page.locator(selector)).toBeVisible();
  await page.evaluate(() => NM.dispose());
}
async function startRead(page, path, loader) {
  const sent = page.waitForRequest(r => new URL(r.url()).pathname === '/api/' + path && r.method() === 'GET');
  await page.evaluate(name => { window.featureRead = window[name](); }, loader);
  return sent;
}
const postResponse = (page, path) => page.waitForResponse(r => new URL(r.url()).pathname === '/api/' + path && r.request().method() === 'POST');
const customDNS = { state: { settings: { builtinDNS: true, dnsProvider: 'custom', dnsCustom: ['192.0.2.1'] } } };

test('focused DNS provider changes reveal dependent controls immediately and retain focus', async ({ page }) => {
  await openFeature(page, 'protection/dns', '#dnsProv');
  await page.locator('#dnsProv').focus();
  await page.locator('#dnsProv').selectOption('custom');
  await expect(page.locator('#dnsC0')).toBeVisible({ timeout: 1000 });
  await expect(page.locator('#dnsProv')).toBeFocused();
  await page.locator('#dnsProv').selectOption('cloudflare');
  await expect(page.locator('#dnsSecure')).toBeVisible({ timeout: 1000 });
  await expect(page.locator('#dnsProv')).toBeFocused();
});

test('DNS testing and refresh preserve custom addresses and an unsubmitted domain', async ({ page, fixture }) => {
  await fixture.patch(customDNS);
  await openFeature(page, 'protection/dns', '#dnsC0');
  await page.locator('#dnsC0').fill('192.0.2.53');
  await page.locator('#dnsDom').fill('draft.example.test');
  await page.locator('#pageTitle').focus();
  await mutate(page, 'dns', { action: 'test' }, () => page.locator('#dnsTest').click());
  await page.evaluate(() => loadDNS());
  await expect(page.locator('#dnsC0')).toHaveValue('192.0.2.53');
  await expect(page.locator('#dnsDom')).toHaveValue('draft.example.test');
  expect((await fixture.read('dns')).dns.custom).toEqual(['192.0.2.1']);
});

for (const status of [200, 500]) {
  test(`DNS ${status === 200 ? 'successful' : 'failed'} save keeps edits made while pending`, async ({ page, fixture }) => {
    await fixture.patch(customDNS);
    await openFeature(page, 'protection/dns', '#dnsC0');
    await page.locator('#dnsC0').fill('192.0.2.53');
    await page.locator('#pageTitle').focus();
    await page.evaluate(() => loadDNS());
    await fixture.fault({ path: '/api/dns', method: 'POST', delayMs: 700, status, times: 1 });
    const saved = postResponse(page, 'dns');
    await page.locator('#dnsApply').click();
    await page.locator('#dnsC0').fill('192.0.2.54');
    await page.locator('#pageTitle').focus();
    await saved;
    await page.evaluate(() => loadDNS());
    await expect(page.locator('#dnsC0')).toHaveValue('192.0.2.54');
    await expect(page.locator('#dnsApply')).toBeEnabled();
    await mutate(page, 'dns', { action: 'set', provider: 'custom', secure: false, custom: ['192.0.2.54'], domains: [] }, () => page.locator('#dnsApply').click());
    expect((await fixture.read('dns')).dns.custom).toEqual(['192.0.2.54']);
  });
}

test('DNS ignores a GET started before a configuration save', async ({ page, fixture }) => {
  await fixture.patch(customDNS);
  await openFeature(page, 'protection/dns', '#dnsC0');
  await page.locator('#dnsC0').fill('192.0.2.53');
  await page.locator('#pageTitle').focus();
  await page.evaluate(() => loadDNS());
  await fixture.fault({ path: '/api/dns', method: 'GET', delayMs: 700, times: 1 });
  await startRead(page, 'dns', 'loadDNS');
  await mutate(page, 'dns', { action: 'set', provider: 'custom', secure: false, custom: ['192.0.2.53'], domains: [] }, () => page.locator('#dnsApply').click());
  await page.evaluate(() => window.featureRead);
  await expect(page.locator('#dnsC0')).toHaveValue('192.0.2.53');
});

test('sharing keeps the rename identity, draft and caret when addresses change', async ({ page, fixture }) => {
  await fixture.patch({ state: { settings: { share: { enabled: true } } } });
  await openFeature(page, 'sharing/devices', '[data-ren]');
  const id = await page.locator('[data-ren]').first().getAttribute('data-ren');
  await page.locator('[data-ren]').first().click();
  await page.locator('#shNm').fill('Unsent phone name');
  await page.locator('#shNm').evaluate(el => el.setSelectionRange(2, 6));
  await fixture.patch({ share: { addresses: [{ iface: 'Wi-Fi', ip: '192.0.2.11' }] } });
  await page.evaluate(() => loadShare());
  await expect(page.locator('#shNm')).toHaveValue('Unsent phone name');
  await expect(page.locator('#shNm')).toBeFocused();
  expect(await page.locator('#shNm').evaluate(el => [el.selectionStart, el.selectionEnd])).toEqual([2, 6]);
  await page.locator('#pageTitle').focus();
  await page.evaluate(() => loadShare());
  await expect(page.locator('#shNm')).toHaveValue('Unsent phone name');
  await mutate(page, 'share', { name: { id, name: 'Unsent phone name' } }, () => page.locator('#shNmOk').click());
  await expect(page.locator('#shNm')).toHaveCount(0);
  await expect(page.locator('#shDev')).toContainText('Unsent phone name');
});

test('sharing retains a failed rename and newer edits during a successful retry', async ({ page, fixture }) => {
  await fixture.patch({ state: { settings: { share: { enabled: true } } } });
  await openFeature(page, 'sharing/devices', '[data-ren]');
  await page.locator('[data-ren]').first().click();
  await page.locator('#shNm').fill('First name');
  await fixture.fault({ path: '/api/share', method: 'POST', status: 500, times: 1 });
  const failed = postResponse(page, 'share');
  await page.locator('#shNmOk').click(); await failed;
  await page.locator('#pageTitle').focus();
  await page.evaluate(() => loadShare());
  await expect(page.locator('#shNm')).toHaveValue('First name');
  await fixture.fault({ path: '/api/share', method: 'POST', delayMs: 700, times: 1 });
  const saved = postResponse(page, 'share');
  await page.locator('#shNmOk').click();
  await page.locator('#shNm').fill('Newer name');
  await page.locator('#pageTitle').focus();
  await saved;
  await page.evaluate(() => loadShare());
  await expect(page.locator('#shNm')).toHaveValue('Newer name');
  await page.locator('#shNmNo').click();
  await expect(page.locator('#shNm')).toHaveCount(0);
  await expect(page.locator('#shDev')).toContainText('First name');
});

for (const [input, save, field] of [['blkMine', 'blkSaveMine', 'blocklist'], ['blkAllow', 'blkSaveAllow', 'allowlist']]) {
  test(`blocking preserves newer ${field} edits through delayed saves and stale reads`, async ({ page, fixture }) => {
    await openFeature(page, 'protection/blocking', '#blkMine');
    await page.evaluate(() => loadBlock());
    await fixture.fault({ path: '/api/block', method: 'GET', delayMs: 900, times: 1 });
    await startRead(page, 'block', 'loadBlock');
    await page.locator('#' + input).fill('first.example.test');
    await fixture.fault({ path: '/api/block', method: 'POST', delayMs: 500, times: 1 });
    const saved = postResponse(page, 'block');
    await page.locator('#' + save).click();
    await page.locator('#' + input).fill('newer.example.test');
    await page.locator('#pageTitle').focus();
    await saved;
    await page.evaluate(() => window.featureRead);
    await page.evaluate(() => loadBlock());
    await expect(page.locator('#' + input)).toHaveValue('newer.example.test');
    await mutate(page, 'block', { [field]: 'newer.example.test' }, () => page.locator('#' + save).click());
    expect((await fixture.read('block'))[field]).toBe('newer.example.test');
  });
}

test('presets retain unfocused name and schedule drafts when the scheduled preset changes', async ({ page, fixture }) => {
  await openFeature(page, 'connections/presets', '#prName');
  await page.locator('#prName').fill('Evening draft');
  await page.locator('#prFrom').fill('19:30');
  await page.locator('#prSc').selectOption('gaming');
  await page.locator('#pageTitle').focus();
  await fixture.patch({ state: { settings: { schedActive: true, preset: 'work' } } });
  await page.evaluate(() => loadPresets());
  await expect(page.locator('#prName')).toHaveValue('Evening draft');
  await expect(page.locator('#prFrom')).toHaveValue('19:30');
  await expect(page.locator('#prSc')).toHaveValue('gaming');
  await mutate(page, 'presets', { action: 'schedule', schedule: { enabled: true, from: '19:30', to: '06:00', preset: 'gaming' } }, () => page.locator('#prScOn').click());
  await expect(page.locator('#prName')).toHaveValue('Evening draft');
});

test('saving a preset clears only the submitted name revision and ignores older reads', async ({ page, fixture }) => {
  await openFeature(page, 'connections/presets', '#prName');
  await fixture.fault({ path: '/api/presets', method: 'GET', delayMs: 900, times: 1 });
  await startRead(page, 'presets', 'loadPresets');
  await page.locator('#prName').fill('First preset');
  await fixture.fault({ path: '/api/presets', method: 'POST', delayMs: 500, times: 1 });
  const saved = postResponse(page, 'presets');
  await page.locator('#prSave').click();
  await page.locator('#prName').fill('Next preset draft');
  await page.locator('#pageTitle').focus();
  await saved;
  await page.evaluate(() => window.featureRead);
  await expect(page.locator('#prName')).toHaveValue('Next preset draft');
  await expect(page.locator('#prBody h4').filter({ hasText: 'First preset' })).toHaveCount(1);
});

test('schedule edits made during a save are serialized and retain the newest values', async ({ page, fixture }) => {
  await openFeature(page, 'connections/presets', '#prScOn');
  await fixture.fault({ path: '/api/presets', method: 'POST', delayMs: 700, times: 1 });
  const saved = postResponse(page, 'presets');
  await page.locator('#prScOn').click();
  await page.locator('#prFrom').fill('19:30');
  await page.locator('#prTo').fill('05:00');
  await page.locator('#pageTitle').focus();
  await saved;
  await expect.poll(async () => (await fixture.read('presets')).schedule).toEqual({ enabled: true, from: '19:30', to: '05:00', preset: 'work' });
  await page.evaluate(() => loadPresets());
  await expect(page.locator('#prFrom')).toHaveValue('19:30');
  await expect(page.locator('#prTo')).toHaveValue('05:00');
});

test('a delayed previous log source cannot overwrite the selected source', async ({ page, fixture }) => {
  await openFeature(page, 'activity/logs', '#logBox');
  await expect(page.locator('#logBox')).toContainText('Fixture ready');
  await fixture.fault({ path: '/api/log', method: 'GET', delayMs: 700, times: 1 });
  await startRead(page, 'log', 'loadLog');
  await page.locator('#logSel [data-v="engine"]').click();
  await expect(page.locator('#logBox')).toContainText('Fixture engine');
  await page.evaluate(() => window.featureRead);
  await expect(page.locator('#logBox')).toContainText('Fixture engine');
  await expect(page.locator('#logSel [data-v="engine"]')).toHaveClass(/sel/);
});

test('update progress preserves the release-notes disclosure and keyboard focus', async ({ page, fixture }) => {
  await fixture.patch({ update: { available: true, status: 'downloading', progress: 5, latest: '0.1.0-fixture' } });
  await openFeature(page, 'settings/maintenance', '#updBanner details');
  await page.locator('#updBanner summary').click();
  await expect(page.locator('#updBanner summary')).toBeFocused();
  await fixture.patch({ update: { progress: 6 } });
  await page.evaluate(() => loadUpdate());
  await expect(page.locator('#updBanner details')).toHaveAttribute('open', '');
  await expect(page.locator('#updBanner summary')).toBeFocused();
  await expect(page.locator('#updGo')).toContainText('6%');
});
