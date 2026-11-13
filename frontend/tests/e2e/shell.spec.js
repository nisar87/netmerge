import AxeBuilder from '@axe-core/playwright';
import { test, expect, navigate, waitForRead, mutate } from '../fixtures/browser.mjs';

const routes = [
  ['overview', 'overview', /overview/i],
  ['connections', 'connections', /connections/i],
  ['connections/speed-test', 'connections', /connections|speed test/i],
  ['connections/presets', 'connections', /connections|presets/i],
  ['apps/active', 'apps', /apps/i], ['apps/rules', 'apps', /apps|rules/i],
  ['usage', 'usage', /usage/i], ['usage/allowances', 'usage', /usage|allowances/i],
  ['protection/routing', 'rules', /protection|routing/i],
  ['protection/blocking', 'rules', /protection|blocking/i], ['protection/dns', 'rules', /protection|dns/i],
  ['sharing/connect', 'share', /sharing|connect/i], ['sharing/devices', 'share', /sharing|devices/i],
  ['activity/events', 'history', /activity|events/i], ['activity/logs', 'history', /activity|logs/i],
  ['settings/general', 'settings', /settings|general/i], ['settings/appearance', 'settings', /settings|appearance/i],
  ['settings/maintenance', 'settings', /settings|updates|backup/i], ['settings/advanced', 'settings', /settings|advanced/i], ['settings/about', 'settings', /settings|about/i],
];

for (const [route, nav, heading] of routes) {
  test(`deep link and refresh: #/${route}`, async ({ page, fixture }) => {
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

test('the download manager moved to Pullwell: no Downloads page, old links land on its note in Settings', async ({ page, fixture }) => {
  await navigate(page, 'overview');
  await expect(page.locator('[data-p="downloads"]')).toHaveCount(0);
  await page.goto('/#/downloads/transfers');
  await expect(page.locator('#pageTitle')).toHaveText(/settings|general/i);
  await expect(page.locator('#pwRow')).toBeVisible();
  await expect(page.locator('#pwRow').getByRole('link', { name: 'Get Pullwell', exact: true })).toHaveAttribute('href', 'https://github.com/nisar87/pullwell/releases/latest');
  await expect(page.locator('#pwSave')).toBeHidden(); // nothing to move for someone who never used it
  await fixture.patch({ state: { legacyDownloads: 3 } });
  await waitForRead(page, 'state');
  await expect(page.locator('#pwSave')).toBeVisible();
  await expect(page.locator('#pwSave')).toHaveAttribute('href', '/api/legacy-downloads');
  const requests = await fixture.requests();
  expect(requests.filter(r => /\/api\/downloads|downloads(?:-more)?\.(?:js|css)/.test(r.path))).toEqual([]);
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

test('after an update the window refreshes by itself, but waits while something is being typed', async ({ page, fixture }) => {
  await navigate(page, 'protection/blocking');
  await page.evaluate(() => { window.oldWindow = true; });
  // typing in a list that is not saved yet: the refresh must wait
  await page.locator('#blkMine').fill('typed.example.test');
  await fixture.patch({ state: { snap: { app: '9.9.9-fixture' } } });
  await waitForRead(page, 'state');
  await expect(page.locator('#connectionBanner')).toContainText('NetMerge was updated');
  expect(await page.evaluate(() => window.oldWindow)).toBe(true);
  // nothing unsaved: the next poll loads the new window
  await page.evaluate(() => { document.activeElement?.blur(); document.querySelector('#blkMine').dataset.dirty = ''; });
  await expect.poll(() => page.evaluate(() => window.oldWindow === undefined), { timeout: 10_000 }).toBe(true);
  await expect(page.locator('#connectionBanner')).toBeHidden();
  await expect(page.locator('#blkMine')).toBeVisible();
});
