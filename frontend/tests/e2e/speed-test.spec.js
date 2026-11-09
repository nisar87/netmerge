import AxeBuilder from '@axe-core/playwright';
import { test, expect, navigate, mutate } from '../fixtures/browser.mjs';

const completed = {
  running: false, phase: 'Done', progress: 1, live: 0, combined: 120, merged: 112.4,
  results: [
    { name: 'Ethernet', label: 'Ethernet', mbps: 90, single: 45, ip: '192.0.2.10' },
    { name: 'Wi-Fi', label: 'Wi-Fi', mbps: 30, single: 22, ip: '192.0.2.20' },
  ],
  scaling: [{ streams: 1, mbps: 45 }, { streams: 8, mbps: 112.4 }],
  suggest: { Ethernet: 4, 'Wi-Fi': 1 }, finished: '2026-11-09T12:00:00Z', warning: '',
};
async function open(page) {
  await navigate(page, 'connections/speed-test');
  await expect(page.locator('#testBtn')).toBeEnabled();
  await page.evaluate(() => NM.dispose());
}
async function receive(page, fixture, test) {
  await fixture.patch({ state: { test } });
  await page.evaluate(() => poll(true));
}
const running = (live, phase = 'Testing Ethernet (8 streams)', progress = 0.3) => ({
  running: true, phase, progress, live, finished: '0001-01-01T00:00:00Z', results: [], scaling: [], combined: 0, merged: 0, suggest: {},
});

test('idle meter is unmeasured, graph is empty and details are collapsed', async ({ page }) => {
  await open(page);
  await expect(page.locator('#testSpeed')).toHaveText('—');
  await expect(page.locator('#testMeter')).toHaveAttribute('aria-label', /not measured/i);
  await expect(page.locator('#testGraphEmpty')).toContainText('Start a test');
  await expect(page.locator('#testDetails')).not.toHaveAttribute('open', '');
  await expect(page.locator('#testChart [data-speed-line]')).toHaveCount(0);
});

test('received samples survive navigation, do not duplicate on redraw and reset for another test', async ({ page, fixture }) => {
  await open(page);
  await receive(page, fixture, running(42));
  await receive(page, fixture, running(75));
  await expect(page.locator('#testSpeed')).toHaveText('75.0');
  await expect(page.locator('#testProg')).toHaveAttribute('aria-valuenow', '30');
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(2);
  await page.evaluate(() => { render(); renderTest(); });
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(2);
  await navigate(page, 'overview');
  await receive(page, fixture, running(87));
  await navigate(page, 'connections/speed-test');
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(3);
  await receive(page, fixture, running(110, 'Testing all connections together', 0.55));
  await expect(page.locator('#testChart [data-speed-line]')).toHaveCount(2);
  await receive(page, fixture, completed);
  await expect(page.locator('#testSpeed')).toHaveText('112.4');
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(4);
  await receive(page, fixture, running(0, 'Preparing', 0));
  await expect(page.locator('#testSpeed')).toHaveText('—');
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(0);
});

test('completed test exposes comparisons and exact share action behind keyboard disclosure', async ({ page, fixture }) => {
  await fixture.patch({ state: { test: completed } });
  await open(page);
  await expect(page.locator('#testSpeed')).toHaveText('112.4');
  await expect(page.locator('#testSpeedScope')).toHaveText('Best through NetMerge');
  await expect(page.locator('#testGraphEmpty')).toContainText('No live samples');
  await page.locator('#testDetails > summary').focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('#testResults')).toContainText('192.0.2.10');
  await expect(page.locator('#testResults')).toContainText('45.0');
  await expect(page.locator('#testScaling')).toContainText('112.4');
  await page.evaluate(() => poll(true));
  await expect(page.locator('#testDetails > summary')).toBeFocused();
  await expect(page.locator('#testDetails')).toHaveAttribute('open', '');
  await mutate(page, 'applyweights', {}, () => page.locator('#applyW').click());
  await expect(page.locator('#testNotes')).toContainText('already match');
  expect((await fixture.read('state')).settings.links.map(l => l.weight)).toEqual([4, 1]);
});

test('skipped merge, failed connections and warnings stay truthful and visible', async ({ page, fixture }) => {
  await fixture.patch({ state: { test: { ...completed, scaling: [], merged: 0, warning: 'Both adapters use the same internet line.' } } });
  await open(page);
  await expect(page.locator('#testSpeed')).toHaveText('120.0');
  await expect(page.locator('#testSpeedScope')).toHaveText('All connections together');
  await expect(page.locator('#testWarnings')).toContainText('same internet line');
  await page.locator('#testDetails > summary').click();
  await expect(page.locator('#testScaling')).toContainText('Merging was off');
  await receive(page, fixture, { ...completed, combined: 0, merged: 0, scaling: [], results: [{ name: 'Ethernet', label: '<offline>', mbps: 0, single: 0, error: 'No data received' }], suggest: {} });
  await expect(page.locator('#testSpeed')).toHaveText('—');
  await expect(page.locator('#testStatus')).toContainText('No speed measured');
  await expect(page.locator('#testWarnings')).toContainText('No speed was measured');
  await expect(page.locator('#testResults')).toContainText('<offline>');
  await expect(page.locator('#testResults')).toContainText('No data received');
});

test('pausing the graph preserves readings while the meter continues, and resumes without another request', async ({ page, fixture }) => {
  await open(page);
  await receive(page, fixture, running(40));
  await receive(page, fixture, running(80));
  await page.getByRole('button', { name: 'Pause graph' }).click();
  const frozen = await page.locator('#testChart').innerHTML();
  await receive(page, fixture, running(140));
  await expect(page.locator('#testSpeed')).toHaveText('140.0');
  expect(await page.locator('#testChart').innerHTML()).toBe(frozen);
  await page.getByRole('button', { name: 'Resume graph' }).click();
  await expect(page.locator('#testGraphSummary')).toContainText('140.0');
  await page.locator('#testSample').focus();
  await page.keyboard.press('Home');
  await expect(page.locator('#testSampleValue')).toContainText('40.0 Mbps');
  expect((await fixture.requests()).filter(r => r.method === 'POST')).toEqual([]);
});

test('failed start keeps the previous result and offers a retry without duplicate posts', async ({ page, fixture }) => {
  await fixture.patch({ state: { test: completed } });
  await open(page);
  await fixture.fault({ path: '/api/speedtest', method: 'POST', status: 500, delayMs: 400, times: 1 });
  await page.locator('#testBtn').click();
  await expect(page.locator('#testBtn')).toBeDisabled();
  await expect(page.locator('#testWarnings')).toContainText('Could not start');
  await expect(page.locator('#testBtn')).toBeEnabled();
  await expect(page.locator('#testSpeed')).toHaveText('112.4');
  expect((await fixture.requests()).filter(r => r.method === 'POST' && r.path === '/api/speedtest')).toHaveLength(1);
  await mutate(page, 'speedtest', {}, () => page.locator('#testBtn').click());
  await page.evaluate(() => poll(true));
  await expect(page.locator('#testWarnings')).not.toContainText('Could not start');
});

test('missing readings stay unknown and a measured zero stays zero', async ({ page, fixture }) => {
  await open(page);
  await receive(page, fixture, running(null));
  await expect(page.locator('#testSpeed')).toHaveText('—');
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(0);
  await receive(page, fixture, running(0));
  await expect(page.locator('#testSpeed')).toHaveText('0.0');
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(1);
  await receive(page, fixture, running(1420));
  await expect(page.locator('#testMeter')).toHaveAttribute('aria-label', /Scale 0 to 2000 Mbps/);
  await expect(page.locator('#testSpeed')).toHaveText('1420');
});

test('failed polls add no samples and an unobserved completed test clears old history', async ({ page, fixture }) => {
  await open(page);
  await receive(page, fixture, running(40));
  await fixture.fault({ path: '/api/state', method: 'GET', status: 503, times: 1 });
  await page.evaluate(() => poll(true));
  await expect(page.locator('#connectionBanner')).toContainText('Connection lost');
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(1);
  await receive(page, fixture, completed);
  await expect(page.locator('#connectionBanner')).toBeHidden();
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(1);
  await receive(page, fixture, { ...completed, finished: '2026-11-09T12:10:00Z', merged: 130 });
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(0);
  await expect(page.locator('#testSpeed')).toHaveText('130.0');
  await expect(page.locator('#testGraphEmpty')).toBeVisible();
  await receive(page, fixture, running(45));
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(1);
  await receive(page, fixture, { running: false, phase: '', live: 0 });
  await expect(page.locator('#testSamples tbody tr')).toHaveCount(0);
  await expect(page.locator('#testSpeed')).toHaveText('—');
});

for (const theme of ['light', 'dark']) for (const width of [390, 1440]) {
  test(`speed test is accessible and fits ${theme} at ${width}px`, async ({ page, fixture }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.emulateMedia({ colorScheme: theme, reducedMotion: 'reduce' });
    await fixture.patch({ state: { test: completed } });
    await open(page);
    await page.locator('#testDetails > summary').click();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    expect((await new AxeBuilder({ page }).include('#pg-test').analyze()).violations).toEqual([]);
    await receive(page, fixture, running(612.5));
    await expect(page.locator('#testSpeed')).toHaveText('612.5');
    expect((await new AxeBuilder({ page }).include('#pg-test').analyze()).violations).toEqual([]);
    expect(await page.locator('#testMeterFill').evaluate(el => parseFloat(getComputedStyle(el).transitionDuration))).toBeLessThan(0.001);
  });
}
