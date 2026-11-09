import AxeBuilder from '@axe-core/playwright';
import { test, expect, navigate, waitForRead, mutate, enableDownloads } from '../fixtures/browser.mjs';

const routes = [
  ['overview', 'overview', /overview/i],
  ['connections', 'connections', /connections/i],
  ['connections/speed-test', 'connections', /connections|speed test/i],
  ['connections/presets', 'connections', /connections|presets/i],
  ['downloads/transfers', 'downloads', /downloads|transfers/i],
  ['downloads/queues', 'downloads', /downloads|queues/i],
  ['downloads/browser', 'downloads', /downloads|browser/i],
  ['downloads/settings', 'downloads', /downloads|settings/i],
  ['apps/active', 'apps', /apps/i], ['apps/rules', 'apps', /apps|rules/i],
  ['usage', 'usage', /usage/i], ['usage/allowances', 'usage', /usage|allowances/i],
  ['protection/routing', 'rules', /protection|routing/i],
  ['protection/blocking', 'rules', /protection|blocking/i], ['protection/dns', 'rules', /protection|dns/i],
  ['sharing/connect', 'share', /sharing|connect/i], ['sharing/devices', 'share', /sharing|devices/i],
  ['activity/events', 'history', /activity|events/i], ['activity/logs', 'history', /activity|logs/i],
  ['settings/general', 'settings', /settings|general/i], ['settings/appearance', 'settings', /settings|appearance/i],
  ['settings/maintenance', 'settings', /settings|updates|backup/i], ['settings/advanced', 'settings', /settings|advanced/i],
];

for (const [route, nav, heading] of routes) {
  test(`deep link and refresh: #/${route}`, async ({ page, fixture }) => {
    await enableDownloads(fixture);
    await navigate(page, route);
    await expect(page.locator('#pageTitle')).toHaveText(heading);
    await expect(page).toHaveTitle(/^NetMerge · /);
    await expect(page.locator(`[data-p="${nav}"][aria-current="page"]`)).toBeVisible();
    await page.reload();
    await expect(page).toHaveURL(new RegExp(`#/${route}$`));
    await expect(page.locator('#pageTitle')).toHaveText(heading);
    await expect(page).toHaveTitle(/^NetMerge · /);
  });
}

test('top-level navigation, browser Back/Forward and heading focus', async ({ page }) => {
  await navigate(page, 'overview');
  for (const [nav, route] of [['connections', 'connections'], ['apps', 'apps/active'], ['rules', 'protection/routing']]) {
    await page.locator(`[data-p="${nav}"]`).click();
    await expect(page).toHaveURL(new RegExp(`#/${route}$`));
    await expect(page.locator('#pageTitle')).toBeFocused();
  }
  await page.goBack(); await expect(page).toHaveURL(/#\/apps\/active$/);
  await page.goForward(); await expect(page).toHaveURL(/#\/protection\/routing$/);
});

test('Downloads remains discoverable while off without loading its API or feature assets', async ({ page, fixture }) => {
  await navigate(page, 'overview');
  await page.locator('[data-p="downloads"]').click();
  await expect(page).toHaveURL(/#\/downloads\/transfers$/);
  await expect(page.locator('#pg-downloads').getByRole('button', { name: /enable download manager/i })).toBeVisible();
  // Observe another real state polling cycle, so a hidden eager loader is caught.
  await waitForRead(page, 'state');
  const requests = await fixture.requests();
  expect(requests.filter(r => r.path === '/api/downloads')).toEqual([]);
  expect(requests.filter(r => /\/(downloads(?:-more)?\.(?:js|css)|features\/downloads\/)/.test(r.path))).toEqual([]);
  expect(requests.filter(r => r.method === 'POST')).toEqual([]);
  await navigate(page, 'settings/general');
  await expect(page.locator('#dmTog')).toHaveAttribute('aria-checked', 'false');
  await mutate(page, 'downloads/switch', { on: true }, () => page.locator('#dmTog').click());
  await expect(page.locator('#dmTog')).toHaveAttribute('aria-checked', 'true');
  await navigate(page, 'downloads/transfers');
  await expect(page.locator('#dlSearch')).toBeVisible();
  expect((await fixture.read('downloads')).enabled).toBe(true);
});

test('null Go slices, one connection and an empty adapter list do not crash navigation', async ({ page, fixture }) => {
  for (const scenario of ['null', 'one-connection', 'empty']) {
    await fixture.reset(scenario);
    await page.goto('/#/overview');
    await expect(page.locator('#pageTitle')).toHaveText(/overview/i);
    for (const route of ['connections', 'apps/active', 'usage', 'protection/blocking', 'activity/events']) {
      await navigate(page, route);
      await expect(page.locator('#pageTitle')).not.toHaveText('');
    }
  }
});

test('lost contact is visible and polling recovers without a page reload', async ({ page, fixture }) => {
  await navigate(page, 'overview');
  await expect(page.locator('#power')).toHaveAttribute('aria-checked', 'true');
  await fixture.scenario('offline');
  await expect(page.getByText(/lost contact|reconnect|unreachable|unable to connect|connection lost/i).first()).toBeVisible({ timeout: 15_000 });
  await fixture.patch({ state: { snap: { running: false } } });
  await fixture.scenario('default');
  await expect(page.locator('#power')).toHaveAttribute('aria-checked', 'false', { timeout: 15_000 });
  await expect(page.getByText(/lost contact|unreachable|connection lost/i).first()).not.toBeVisible();
});

test('failed power mutation does not report a confirmed success or retry itself', async ({ page, fixture }) => {
  await navigate(page, 'overview');
  await expect(page.locator('#power')).toHaveAttribute('aria-checked', 'true');
  await fixture.fault({ path: '/api/power', method: 'POST', status: 500, error: 'Fixture power failed', times: 1 });
  await fixture.clear(); await page.locator('#power').click();
  await expect(page.getByText('Fixture power failed', { exact: false }).first()).toBeVisible();
  await waitForRead(page, 'state');
  await expect(page.locator('#power')).toHaveAttribute('aria-checked', 'true');
  expect((await fixture.requests()).filter(r => r.path === '/api/power')).toHaveLength(1);
});

test('theme and usable navigation survive unavailable browser storage', async ({ page }) => {
  await page.addInitScript(() => {
    for (const method of ['getItem', 'setItem', 'removeItem']) Storage.prototype[method] = () => { throw new DOMException('Storage denied', 'SecurityError'); };
  });
  await navigate(page, 'settings/appearance');
  await page.locator('#theme [data-v="dark"]').click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await page.locator('[data-p="overview"]').click();
  await expect(page).toHaveURL(/#\/overview$/);
});

for (const width of [375, 768, 1440]) {
  test(`shell reflows and has no serious accessibility violations at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await navigate(page, 'overview');
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);
    const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
    expect(violations.filter(v => ['serious', 'critical'].includes(v.impact))).toEqual([]);
    if (width < 768) {
      const menu = page.locator('#menuBtn');
      await menu.click();
      await expect(page.locator('[data-p="connections"]')).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(menu).toBeFocused();
    }
  });
}

for (const [route, destination] of [['connections', 'connections/presets'], ['protection/routing', 'protection/dns'], ['settings/general', 'settings/appearance']]) {
  test(`section links navigate from ${route} to ${destination}`, async ({ page }) => {
    await navigate(page, route);
    await page.locator(`#subnav a[href="#/${destination}"]`).click();
    await expect(page).toHaveURL(new RegExp(`#/${destination}$`));
    await expect(page.locator('#pageTitle')).toBeFocused();
  });
}
