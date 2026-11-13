import AxeBuilder from '@axe-core/playwright';
import { test, expect, navigate, waitForRead, mutate } from '../fixtures/browser.mjs';

test('overview shows live speed per connection, and the merge picture adds them up', async ({ page }) => {
  await navigate(page, 'overview');
  const cards = page.locator('#links .lk');
  await expect(cards).toHaveCount(2);
  const fibre = page.locator('.lk[data-n="Ethernet"]');
  await expect(fibre.locator('[data-f=down]')).toHaveText('16.8');
  await expect(page.locator('.lk[data-n="Wi-Fi"] [data-f=down]')).toHaveText('8.39');
  await expect(fibre.locator('[data-f=tshare]')).toHaveText('67%');
  await expect(fibre.locator('[data-f=ping]')).toHaveText('14 ms');
  await expect(fibre.locator('[data-f=line]')).toHaveAttribute('d', /^M/);
  await expect(page.locator('#mergeFlow')).toHaveAttribute('aria-label', /Combined download speed 25\.2 Mbps of a 80 Mbps scale, light use\. Home fibre 16\.8, Backup Wi-Fi 8\.39 Mbps/);
  await expect(page.locator('#mergeFlow [data-f="hubValue"]')).toHaveText('25.2');
  await expect(page.locator('#totalUp')).toContainText('3.15');
  await expect(page.locator('#statusLine')).toContainText('2 connections working together');
});

test('the speedometer needle, arc and colour follow the real speed', async ({ page, fixture }) => {
  await navigate(page, 'overview');
  const needle = page.locator('#mergeFlow [data-f="needle"]'), arc = page.locator('#mergeFlow [data-f="arc"]');
  // 25.2 of an 80 Mbps scale (session peak 50.3 Mbps)
  await expect(arc).toHaveAttribute('stroke-dasharray', /^31\.\d+ 100$/);
  await expect(page.locator('#mergeFlow [data-f="tier"]')).toHaveText('Light use');
  const links = (await fixture.read('state')).snap.links;
  await waitForRead(page, 'state', () => fixture.patch({ state: { snap: { totalDown: 9e6, links: links.map(l => ({ ...l, down: 4.5e6 })) } } }));
  await expect(page.locator('#mergeFlow [data-f="hubValue"]')).toHaveText('72.0');
  await expect(page.locator('#mergeFlow [data-f="tier"]')).toHaveText('Near top speed');
  // 72 Mbps passes the old peak, so the scale grows to 100 Mbps: 72% of the arc
  await expect(arc).toHaveAttribute('stroke-dasharray', '72.00 100');
  await expect(page.locator('#mergeFlow .mf-of')).toHaveText('scale 0–100 Mbps');
  await expect.poll(() => needle.evaluate(n => n.style.transform)).toBe('rotate(329.4deg)');
  await expect(page.locator('.lk[data-n="Ethernet"] .mini-gauge')).toHaveAttribute('data-tier', /.+/);
});

test('each connection is shown once: its card on Overview holds the live figures and its controls', async ({ page, fixture }) => {
  await navigate(page, 'overview');
  await expect(page.locator('#legend')).not.toContainText('16.8');
  await expect(page.locator('#statusLine')).not.toContainText('Mbps');
  const fibre = page.locator('.lk[data-n="Ethernet"]');
  await fibre.locator('.lk-settings summary').click();
  await expect(fibre).toContainText('Local 192.0.2.10');
  await expect(fibre).toContainText('Public 198.51.100.10');
  const before = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...before, links: before.links.map(l => l.name === 'Ethernet' ? { ...l, role: 'backup' } : l) },
    () => fibre.getByRole('combobox', { name: 'When to use this connection' }).selectOption('backup'));
  // the Connections page has the traffic mode and counts, not the connections again
  await navigate(page, 'connections');
  await expect(page.locator('#pg-connections .lk')).toHaveCount(0);
  await expect(page.locator('#pg-connections')).not.toContainText('Home fibre · ');
  await expect(page.locator('#connFound')).toContainText('2found');
  await page.getByRole('link', { name: 'See them live on Overview' }).click();
  await expect(page).toHaveURL(/#\/overview$/);
});

test('traffic mode pictures change the same setting as before', async ({ page, fixture }) => {
  await navigate(page, 'connections');
  await expect(page.locator('#modePick .mode-opt.sel')).toContainText('fastest');
  const before = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...before, mode: 'stable' }, () => page.getByRole('button', { name: 'Combine all · steady sites' }).click());
  await expect(page.locator('#modePick .mode-opt.sel')).toContainText('steady sites');
  const now = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...now, mode: 'link:Wi-Fi' }, () => page.locator('#modePick').getByRole('button', { name: 'Backup Wi-Fi' }).click());
  await expect(page.locator('#modeDesc')).toContainText('Backup Wi-Fi only');
});

test('a disconnected line is shown as such, and merging off is clear', async ({ page, fixture }) => {
  await navigate(page, 'overview');
  const links = (await fixture.read('state')).snap.links;
  await waitForRead(page, 'state', () => fixture.patch({ state: { snap: { links: [links[0], { ...links[1], present: false, down: 0, up: 0 }] } } }));
  const wifi = page.locator('.lk[data-n="Wi-Fi"]');
  await expect(wifi).toHaveClass(/dim/);
  await expect(wifi.locator('[data-f=status]')).toHaveText('Disconnected');
  await expect(wifi.locator('[data-f=down]')).toHaveText('—');
  await expect(wifi.locator('[data-f=q]')).toBeHidden();
  await fixture.reset('off');
  await page.evaluate(() => poll(true));
  await expect(page.locator('#statusLine')).toContainText('Merging is off');
  await expect(page.locator('#mergeFlow [data-f="hubLabel"]')).toHaveText('ADAPTERS');
  await expect(page.locator('#uptime')).toHaveText('—');
});

test('usage, apps, blocking and activity draw their pictures from the same readings', async ({ page }) => {
  await navigate(page, 'usage');
  await expect(page.locator('#usSum .us-ring')).toHaveCount(2);
  await expect(page.locator('#usChart svg .us-bar')).toHaveCount(1);
  await expect(page.locator('#usTbl')).toContainText('3.15');
  await navigate(page, 'apps/active');
  await expect(page.locator('#appsStats')).toContainText('apps online');
  await expect(page.locator('#appsTbl .app-av')).toHaveCount(1);
  await navigate(page, 'protection/blocking');
  await expect(page.locator('#blkTop')).toHaveCount(0);
  await expect(page.locator('#blkRecent .hb-track')).toHaveCount(1);
  await expect(page.locator('#blkRecent')).toContainText('ads.example.test');
  await expect(page.locator('#blkToday')).toHaveText('12');
  await navigate(page, 'activity/events');
  await expect(page.locator('#hiStats')).toContainText('in total');
});

for (const theme of ['light', 'dark']) {
  test(`overview and connections have no serious accessibility problems in ${theme} theme`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: theme });
    for (const route of ['overview', 'connections']) {
      await navigate(page, route);
      if (route === 'overview') await expect(page.locator('#links .lk')).toHaveCount(2);
      const { violations } = await new AxeBuilder({ page }).include('#main').withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
      expect(violations.filter(v => ['serious', 'critical'].includes(v.impact))).toEqual([]);
    }
  });
}

test('wording follows the real behaviour of the chosen mode and balancing', async ({ page, fixture }) => {
  await navigate(page, 'overview');
  await expect(page.locator('#statusLine')).toContainText('most spare speed');
  await expect(page.locator('#peak + .ov-kpi-foot')).toHaveText('Since NetMerge opened');
  const s = (await fixture.read('state')).settings;
  await waitForRead(page, 'state', () => fixture.patch({ state: { settings: { ...s, mode: 'stable', links: s.links.map(l => l.name === 'Wi-Fi' ? { ...l, role: 'busy' } : l) } } }));
  await page.evaluate(() => poll(true));
  await expect(page.locator('#statusLine')).toContainText('by the shares you set');
  await page.locator('.lk[data-n="Wi-Fi"] .lk-settings summary').click();
  await expect(page.locator('.lk[data-n="Wi-Fi"] [data-f=roleHint]')).toContainText('Works only with Smart balancing');
});

test('Quantum Flow moves only with real traffic, and the animation setting is honoured', async ({ page, fixture }) => {
  await navigate(page, 'overview');
  // traffic on both lines: particles flow
  await expect.poll(() => page.evaluate(() => window.QF.count), { timeout: 8000 }).toBeGreaterThan(5);
  // hovering a source names its real readings
  await expect(page.locator('#mergeFlow .mf-node[data-n="Ethernet"] title')).toContainText(/Mbps down/);
  // merging off: nothing moves
  await waitForRead(page, 'state', () => fixture.patch({ state: { snap: { running: false } } }));
  await expect.poll(() => page.evaluate(() => window.QF.count)).toBe(0);
  await waitForRead(page, 'state', () => fixture.patch({ state: { snap: { running: true } } }));
  await expect.poll(() => page.evaluate(() => window.QF.count), { timeout: 8000 }).toBeGreaterThan(0);
  // a line with no internet is drawn broken and sends nothing
  const links = (await fixture.read('state')).snap.links;
  await waitForRead(page, 'state', () => fixture.patch({ state: { snap: { links: links.map(l => ({ ...l, healthy: false, down: 0 })) } } }));
  await expect(page.locator('#mergeFlow .mf-node.down')).toHaveCount(2);
  await expect.poll(() => page.evaluate(() => window.QF.count), { timeout: 8000 }).toBe(0);
  await waitForRead(page, 'state', () => fixture.patch({ state: { snap: { links } } }));
  // Settings → Appearance → Animations: Off stops the picture and is remembered
  await navigate(page, 'settings/appearance');
  await page.locator('#motion button[data-v="normal"]').waitFor();
  await expect(page.locator('#motion button[data-v="normal"]')).toHaveAttribute('aria-pressed', 'true');
  await page.locator('#motion button[data-v="off"]').click();
  await expect(page.locator('html')).toHaveAttribute('data-motion', 'off');
  expect(await page.evaluate(() => localStorage.getItem('nm-motion'))).toBe('off');
  await navigate(page, 'overview');
  await page.waitForTimeout(1500);
  expect(await page.evaluate(() => window.QF.count)).toBe(0);
  await expect(page.locator('#mergeFlow .qf-canvas')).toBeHidden();
  await navigate(page, 'settings/appearance');
  await page.locator('#motion button[data-v="normal"]').click();
  // the tests run with the browser's reduced-motion preference on, so Normal resolves to Reduced here
  await expect(page.locator('html')).toHaveAttribute('data-motion-pref', 'normal');
  await expect(page.locator('html')).not.toHaveAttribute('data-motion', 'off');
});

test('Windows "animation effects off" turns Normal into Reduced, and a narrow window draws no streams', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await navigate(page, 'overview');
  await expect(page.locator('html')).toHaveAttribute('data-motion', 'reduced');
  await page.setViewportSize({ width: 390, height: 844 });
  await page.waitForTimeout(2500);
  expect(await page.evaluate(() => window.QF.count)).toBe(0);
});
