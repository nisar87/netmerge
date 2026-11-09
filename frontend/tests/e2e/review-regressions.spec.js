import { test, expect, navigate, enableDownloads, mutate } from '../fixtures/browser.mjs';

async function openDownloads(page, fixture, tab, selector) {
  await enableDownloads(fixture);
  await navigate(page, 'downloads/' + tab);
  await expect(page.locator(selector).first()).toBeVisible();
  await page.evaluate(() => NM.dispose());
}

for (const values of [['4', '3'], ['4', '3', '4']]) {
  test(`download settings keep pending choices ${values.join(' → ')}`, async ({ page, fixture }) => {
    await openDownloads(page, fixture, 'settings', '#dlMax');
    await fixture.clear();
    await fixture.fault({ path: '/api/downloads', method: 'POST', delayMs: 900, times: 1 });
    const sent = page.waitForRequest(r => r.method() === 'POST' && r.postDataJSON()?.action === 'config');
    await page.locator('#dlMax').selectOption(values[0]);
    await sent;
    for (const value of values.slice(1)) await page.locator('#dlMax').selectOption(value);
    await page.evaluate(() => dlConfigTail);
    const writes = (await fixture.requests()).filter(r => r.body?.action === 'config');
    expect(writes.map(r => r.body.config.maxActive)).toEqual(values.map(Number));
    expect((await fixture.read('downloads')).config.maxActive).toBe(Number(values.at(-1)));
    await page.locator('#pageTitle').focus();
    await page.evaluate(() => loadDL(true));
    await expect(page.locator('#dlMax')).toHaveValue(values.at(-1));
  });
}

test('browser size reversal preserves an external handoff change', async ({ page, fixture }) => {
  await openDownloads(page, fixture, 'browser', '#dlExtMin');
  await fixture.patch({ downloads: { ext: { enabled: false } } });
  await fixture.clear();
  await fixture.fault({ path: '/api/downloads', method: 'POST', delayMs: 900, times: 1 });
  const sent = page.waitForRequest(r => r.method() === 'POST' && r.postDataJSON()?.action === 'extConfig');
  await page.locator('#dlExtMin').selectOption('10');
  await sent;
  await page.locator('#dlExtMin').selectOption('5');
  await page.evaluate(() => dlConfigTail);
  const writes = (await fixture.requests()).filter(r => r.body?.action === 'extConfig');
  expect(writes.map(r => r.body)).toEqual([
    { action: 'extConfig', extMinMB: 10, extOff: true },
    { action: 'extConfig', extMinMB: 5, extOff: true },
  ]);
  expect((await fixture.read('downloads')).ext).toMatchObject({ minMB: 5, enabled: false });
});

test('pending per-line limit reset keeps unrelated external limits', async ({ page, fixture }) => {
  await openDownloads(page, fixture, 'settings', '[data-cfg="line:Ethernet"]');
  await fixture.patch({ downloads: { config: { limitLines: { 'Wi-Fi': 5242880 } } } });
  await fixture.fault({ path: '/api/downloads', method: 'POST', delayMs: 900, times: 1 });
  const sent = page.waitForRequest(r => r.method() === 'POST' && r.postDataJSON()?.action === 'config');
  await page.locator('[data-cfg="line:Ethernet"]').selectOption('1');
  await sent;
  await page.locator('[data-cfg="line:Ethernet"]').selectOption('0');
  await page.evaluate(() => dlConfigTail);
  expect((await fixture.read('downloads')).config.limitLines).toEqual({ 'Wi-Fi': 5242880 });
});

const appRules = [
  { process: 'first.exe', target: 'fast' },
  { process: 'editing.exe', target: 'stable' },
  { process: 'last.exe', target: 'fast' },
];
async function openRules(page, fixture) {
  await fixture.patch({ state: { settings: { appRules } } });
  await navigate(page, 'apps/rules');
  await expect(page.locator('#rulesTbl select')).toHaveCount(3);
  await page.evaluate(() => NM.dispose());
}

test('focused app rule still edits its process after an earlier rule is removed', async ({ page, fixture }) => {
  await openRules(page, fixture);
  const selector = page.locator('#rulesTbl').getByRole('combobox', { name: 'Rule for editing.exe', exact: true });
  await selector.focus();
  await fixture.patch({ state: { settings: { appRules: appRules.slice(1) } } });
  await page.evaluate(() => poll(true));
  await expect(selector).toBeFocused();
  const settings = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...settings, appRules: [
    { process: 'editing.exe', target: 'bypass' }, appRules[2],
  ] }, () => selector.selectOption('bypass'));
});

test('a focused rule removed elsewhere does not edit its replacement or recreate itself', async ({ page, fixture }) => {
  await openRules(page, fixture);
  const selector = page.locator('#rulesTbl').getByRole('combobox', { name: 'Rule for editing.exe', exact: true });
  await selector.focus();
  await fixture.patch({ state: { settings: { appRules: [appRules[0], appRules[2]] } } });
  await page.evaluate(() => poll(true));
  await fixture.clear();
  await selector.selectOption('bypass');
  await expect(page.locator('#rulesTbl select')).toHaveCount(2);
  expect((await fixture.read('state')).settings.appRules).toEqual([appRules[0], appRules[2]]);
  expect((await fixture.requests()).filter(r => r.path === '/api/settings' && r.method === 'POST')).toEqual([]);
});

test('focused remove button keeps its process identity after polling shifts the list', async ({ page, fixture }) => {
  await openRules(page, fixture);
  const button = page.locator('#rulesTbl tr').filter({ hasText: 'editing.exe' }).getByRole('button', { name: 'Remove rule', exact: true });
  await button.focus();
  await fixture.patch({ state: { settings: { appRules: appRules.slice(1) } } });
  await page.evaluate(() => poll(true));
  const settings = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...settings, appRules: [appRules[2]] }, () => button.click());
});

for (const [label, move, expected] of [
  ['Move up', 'up', ['fixture-0002', 'fixture-0001', 'fixture-0003']],
  ['Move down', 'down', ['fixture-0001', 'fixture-0003', 'fixture-0002']],
  ['Download first', 'top', ['fixture-0002', 'fixture-0001', 'fixture-0003']],
]) {
  test(`${label} updates the retained transfer order immediately`, async ({ page, fixture }) => {
    await enableDownloads(fixture);
    const downloads = await fixture.read('downloads');
    await fixture.patch({ downloads: { items: downloads.items.slice(0, 3).map(item => ({ ...item, state: 'paused', finished: 0 })) } });
    await openDownloads(page, fixture, 'transfers', '.dlrow');
    await page.locator('[data-key="fixture-0002"] [data-dm="props"]').first().click();
    await mutate(page, 'downloads', { action: 'move', id: 'fixture-0002', move }, () => page.getByRole('button', { name: label, exact: true }).click());
    await expect.poll(() => page.locator('.dlrow').evaluateAll(rows => rows.map(row => row.dataset.key))).toEqual(expected);
    await page.keyboard.press('Escape');
    await page.evaluate(() => loadDL(true));
    expect(await page.locator('.dlrow').evaluateAll(rows => rows.map(row => row.dataset.key))).toEqual(expected);
  });
}

test('a poll started during a move cannot restore the previous display order', async ({ page, fixture }) => {
  await enableDownloads(fixture);
  const downloads = await fixture.read('downloads');
  await fixture.patch({ downloads: { items: downloads.items.slice(0, 2).map(item => ({ ...item, state: 'paused', finished: 0 })) } });
  await openDownloads(page, fixture, 'transfers', '.dlrow');
  await page.locator('[data-key="fixture-0002"] [data-dm="props"]').first().click();

  // Hold the POST before it reaches the server. Capture an old GET while that
  // mutation is pending, then deliver the old GET after the POST succeeds.
  let releaseMove, releaseRead, sawMove, sawRead, holdRead = true;
  const moveGate = new Promise(resolve => { releaseMove = resolve; });
  const readGate = new Promise(resolve => { releaseRead = resolve; });
  const moveStarted = new Promise(resolve => { sawMove = resolve; });
  const readCaptured = new Promise(resolve => { sawRead = resolve; });
  await page.route('**/api/downloads', async route => {
    if (route.request().method() === 'POST') {
      sawMove(); await moveGate; await route.continue();
    } else if (holdRead) {
      holdRead = false;
      const response = await route.fetch();
      sawRead(); await readGate; await route.fulfill({ response });
    } else await route.continue();
  });
  try {
    await page.getByRole('button', { name: 'Move up', exact: true }).click();
    await moveStarted;
    await page.evaluate(() => { window.moveRead = loadDL(true); });
    await readCaptured;
    releaseMove();
    await expect.poll(() => page.evaluate(() => DL.items.find(item => item.id === 'fixture-0002').order)).toBe(0);
    releaseRead();
    await page.evaluate(() => window.moveRead);
    await expect(page.getByRole('button', { name: 'Move up', exact: true })).toBeEnabled();
    const expected = ['fixture-0002', 'fixture-0001'];
    expect((await fixture.read('downloads')).items.map(item => item.id)).toEqual(expected);
    expect(await page.locator('.dlrow').evaluateAll(rows => rows.map(row => row.dataset.key))).toEqual(expected);
    await page.evaluate(() => loadDL(true));
    expect(await page.locator('.dlrow').evaluateAll(rows => rows.map(row => row.dataset.key))).toEqual(expected);
  } finally { releaseMove(); releaseRead(); }
});
