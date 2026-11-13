// Measures the window's own cost in a browser (fixture data, nothing from a real PC): main-thread busy time,
// script time, animation frames, layout work and JavaScript memory while the Overview runs live for a while,
// and memory after fast navigation through every page. Run: node scripts/perf.mjs [seconds] > result.json
import {chromium} from '@playwright/test';
import {createFixtureServer} from './serve.mjs';

const seconds = Number(process.argv[2] || 30);
const server = createFixtureServer(); await new Promise(done => server.listen(0, '127.0.0.1', done));
const origin = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch(process.env.NETMERGE_BROWSER_EXECUTABLE ? {executablePath: process.env.NETMERGE_BROWSER_EXECUTABLE} : {});
const post = (path, body) => fetch(origin + path, {method: 'POST', headers: {'X-NetMerge-Test': '1', 'Content-Type': 'application/json'}, body: JSON.stringify(body)});
const routes = ['overview', 'connections', 'connections/speed-test', 'connections/presets', 'apps/active', 'apps/rules', 'usage', 'protection/blocking', 'protection/dns', 'sharing/connect', 'activity/events', 'settings/general', 'settings/appearance', 'settings/maintenance', 'settings/advanced', 'settings/about'];
const out = {seconds};
try {
  for (const theme of ['dark', 'light']) {
    await post('/__test/reset', {});
    await post('/api/share', {enabled: true}).catch(() => {});
    const st = await (await fetch(origin + '/api/state', {headers: {'X-NetMerge': '1'}})).json();
    const ctx = await browser.newContext({viewport: {width: 1440, height: 900}, colorScheme: theme, deviceScaleFactor: 1});
    const page = await ctx.newPage();
    const errors = []; page.on('pageerror', e => errors.push(e.message));
    const cdp = await ctx.newCDPSession(page); await cdp.send('Performance.enable');
    await page.goto(origin + '/#/overview'); await page.waitForFunction(() => document.querySelector('#power')?.disabled === false);
    if (process.env.PERF_CSS) await page.addStyleTag({content: process.env.PERF_CSS});
    await page.waitForTimeout(2000);
    // live data: every second the lines' speeds change, as they would on a real PC
    let tick = 0;
    const feeder = setInterval(() => {
      tick++;
      const links = st.snap.links.map((l, i) => ({...l, down: Math.round((i ? 1.1e6 : 2.1e6) * (1 + 0.4 * Math.sin(tick / 3 + i))), history: Array.from({length: 90}, (_, k) => Math.round((i ? 1e6 : 2e6) * (1 + 0.4 * Math.sin((k + tick) / 7 + i))))}));
      post('/__test/state', {state: {snap: {links, totalDown: links.reduce((a, l) => a + l.down, 0)}}}).catch(() => {});
    }, 1000);
    const metrics = async () => Object.fromEntries((await cdp.send('Performance.getMetrics')).metrics.map(m => [m.name, m.value]));
    await cdp.send('HeapProfiler.collectGarbage');
    const a = await metrics();
    await page.evaluate(() => { window.__frames = 0; const f = () => { window.__frames++; requestAnimationFrame(f); }; requestAnimationFrame(f); });
    await page.waitForTimeout(seconds * 1000);
    const frames = await page.evaluate(() => window.__frames);
    await cdp.send('HeapProfiler.collectGarbage');
    const b = await metrics();
    clearInterval(feeder);
    const wall = b.Timestamp - a.Timestamp;
    const r = {
      mainThreadBusyPercent: +(100 * (b.TaskDuration - a.TaskDuration) / wall).toFixed(2),
      scriptPercent: +(100 * (b.ScriptDuration - a.ScriptDuration) / wall).toFixed(2),
      layoutsPerSecond: +((b.LayoutCount - a.LayoutCount) / wall).toFixed(1),
      styleRecalcsPerSecond: +((b.RecalcStyleCount - a.RecalcStyleCount) / wall).toFixed(1),
      framesPerSecond: +(frames / wall).toFixed(1),
      heapStartMB: +(a.JSHeapUsedSize / 1e6).toFixed(2), heapEndMB: +(b.JSHeapUsedSize / 1e6).toFixed(2),
      domNodes: b.Nodes,
    };
    // fast navigation through every page, three times, then memory again
    const nav0 = Date.now();
    for (let k = 0; k < 3; k++) for (const route of routes) { await page.evaluate(h => { location.hash = h; }, '#/' + route); await page.waitForTimeout(60); }
    r.navigationMsPerPage = +((Date.now() - nav0) / (routes.length * 3)).toFixed(0);
    await page.evaluate(h => { location.hash = h; }, '#/overview'); await page.waitForTimeout(1500);
    await cdp.send('HeapProfiler.collectGarbage');
    r.heapAfterNavigationMB = +((await metrics()).JSHeapUsedSize / 1e6).toFixed(2);
    // hidden window: what it costs while minimized
    await page.evaluate(() => { Object.defineProperty(document, 'hidden', {configurable: true, get: () => true}); Object.defineProperty(document, 'visibilityState', {configurable: true, get: () => 'hidden'}); document.dispatchEvent(new Event('visibilitychange')); });
    const h0 = await metrics(); await page.waitForTimeout(5000); const h1 = await metrics();
    r.hiddenMainThreadBusyPercent = +(100 * (h1.TaskDuration - h0.TaskDuration) / (h1.Timestamp - h0.Timestamp)).toFixed(2);
    r.errors = errors;
    out[theme] = r;
    await ctx.close();
  }
} finally { await browser.close(); server.closeAllConnections(); await new Promise(done => server.close(done)); }
process.stdout.write(JSON.stringify(out, null, 2) + '\n');
