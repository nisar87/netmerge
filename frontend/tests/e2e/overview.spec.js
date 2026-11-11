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
  await expect(page.locator('#mergeFlow')).toHaveAttribute('aria-label', /Combined download speed 25\.2 Mbps\. Home fibre 16\.8, Backup Wi-Fi 8\.39 Mbps/);
  await expect(page.locator('#mergeFlow [data-f="hubValue"]')).toHaveText('25.2');
  await expect(page.locator('#totalUp')).toContainText('3.15');
  await expect(page.locator('#statusLine')).toContainText('2 connections working together');
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
  await expect(page.locator('#blkTop .hbars li')).toHaveCount(1);
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
