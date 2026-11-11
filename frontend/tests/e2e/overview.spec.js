import AxeBuilder from '@axe-core/playwright';
import { test, expect, navigate, waitForRead } from '../fixtures/browser.mjs';

test('overview shows live speed per connection, and the merge picture adds them up', async ({ page }) => {
  await navigate(page, 'overview');
  const cards = page.locator('#linkSpeeds .ov-link');
  await expect(cards).toHaveCount(2);
  await expect(cards.nth(0)).toContainText('Home fibre');
  await expect(cards.nth(0).locator('.ov-down b')).toHaveText('16.8');
  await expect(cards.nth(1).locator('.ov-down b')).toHaveText('8.39');
  await expect(cards.nth(0)).toContainText('67%');
  await expect(cards.nth(0)).toContainText('14 ms');
  await expect(page.locator('#mergeFlow')).toHaveAttribute('aria-label', /Combined download speed 25\.2 Mbps\. Home fibre 16\.8, Backup Wi-Fi 8\.39 Mbps/);
  await expect(page.locator('#mergeFlow [data-f="hubValue"]')).toHaveText('25.2');
  await expect(page.locator('#totalUp')).toContainText('3.15');
  await expect(page.locator('#statusLine')).toContainText('2 connections working together');
});

test('each live figure appears once: no speed list or legend rates repeated on Overview, none on Connections', async ({ page }) => {
  await navigate(page, 'overview');
  await expect(page.locator('#connectionSummary')).toHaveCount(0);
  await expect(page.locator('#legend')).toContainText('Home fibre');
  await expect(page.locator('#legend')).not.toContainText('16.8');
  await expect(page.locator('#statusLine')).not.toContainText('Mbps');
  await navigate(page, 'connections');
  const card = page.locator('.lk[data-n="Ethernet"]');
  await expect(card).toContainText('Local 192.0.2.10');
  await expect(card).toContainText('Public 198.51.100.10');
  await expect(card.locator('[data-f=down], canvas.spark')).toHaveCount(0);
  await expect(card).not.toContainText('ms');
  await expect(card).not.toContainText('today');
  await page.getByRole('link', { name: 'Live speed' }).click();
  await expect(page).toHaveURL(/#\/overview$/);
});

test('a disconnected line is shown as such, and merging off is clear', async ({ page, fixture }) => {
  await navigate(page, 'overview');
  const links = (await fixture.read('state')).snap.links;
  await waitForRead(page, 'state', () => fixture.patch({ state: { snap: { links: [links[0], { ...links[1], present: false, down: 0, up: 0 }] } } }));
  const wifi = page.locator('#linkSpeeds .ov-link').filter({ hasText: 'Backup Wi-Fi' });
  await expect(wifi).toHaveClass(/dim/);
  await expect(wifi).toContainText('Disconnected');
  await expect(wifi.locator('.ov-down b')).toHaveText('—');
  await fixture.reset('off');
  await page.evaluate(() => poll(true));
  await expect(page.locator('#statusLine')).toContainText('Merging is off');
  await expect(page.locator('#mergeFlow [data-f="hubLabel"]')).toHaveText('ADAPTERS');
  await expect(page.locator('#uptime')).toHaveText('—');
});

for (const theme of ['light', 'dark']) {
  test(`overview has no serious accessibility problems in ${theme} theme`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: theme });
    await navigate(page, 'overview');
    await expect(page.locator('#linkSpeeds .ov-link')).toHaveCount(2);
    const { violations } = await new AxeBuilder({ page }).include('#pg-overview').withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
    expect(violations.filter(v => ['serious', 'critical'].includes(v.impact))).toEqual([]);
  });
}
