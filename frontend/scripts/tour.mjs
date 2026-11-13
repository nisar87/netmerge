// Records the product tour video for the repository (docs/media): the real window, served by the fixture server,
// with made-up connection data that changes every second the way a real PC's would. Nothing comes from a real PC.
// Captions are an overlay added for the video only. Frames come from Chromium's screencast (high-quality JPEG),
// assembled by ffmpeg at their real timing.
// Run: node scripts/tour.mjs [out.mp4] [width] [height]   (needs ffmpeg)
import {chromium} from '@playwright/test';
import {mkdtemp, writeFile, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import {execFileSync} from 'node:child_process';
import {createFixtureServer} from './serve.mjs';

const out = resolve(process.argv[2] || new URL('../../docs/media/netmerge-tour.mp4', import.meta.url).pathname);
const width = Number(process.argv[3] || 1440), height = Number(process.argv[4] || 900);
const script = process.env.TOUR || 'full';
const server = createFixtureServer(); await new Promise(done => server.listen(0, '127.0.0.1', done));
const origin = `http://127.0.0.1:${server.address().port}`;
const post = (path, body) => fetch(origin + path, {method: 'POST', headers: {'X-NetMerge-Test': '1', 'Content-Type': 'application/json'}, body: JSON.stringify(body)});
await post('/__test/reset', {});
await fetch(origin + '/api/share', {method: 'POST', headers: {'X-NetMerge': '1', 'Content-Type': 'application/json'}, body: '{"enabled":true}'});
const st = await (await fetch(origin + '/api/state', {headers: {'X-NetMerge': '1'}})).json();

// ---- live data: two lines with believable, changing speeds ----
let t = 0, mode = 'ok';
const base = [46e6 / 8, 23e6 / 8];
const wave = (i, k) => Math.max(0, Math.round(base[i] * (1 + .32 * Math.sin(k / 4.3 + i * 2.1) + .12 * Math.sin(k / 1.7 + i))));
const hist = st.snap.links.map((l, i) => Array.from({length: 90}, (_, k) => wave(i, k - 90)));
let peak = 0, totalRx = 0;
function push() {
  t++;
  const links = st.snap.links.map((l, i) => {
    let down = wave(i, t), x = {...l, healthy: true, present: true};
    if (i === 1 && mode === 'down') { x.healthy = false; down = 0; }
    hist[i].push(down); hist[i].shift();
    return {...x, down, up: Math.round(down * .09), history: [...hist[i]], delay: i ? 24 : 12};
  });
  const totalDown = links.reduce((a, l) => a + l.down, 0); peak = Math.max(peak, totalDown); totalRx += totalDown;
  return post('/__test/state', {state: {snap: {links, totalDown, totalUp: links.reduce((a, l) => a + l.up, 0), peakDown: peak}}});
}
// a month of daily use for the Usage page
const days = Array.from({length: 31}, (_, k) => { const d = new Date(Date.UTC(2026, 10, 9 - k)); const w = 1 + .5 * Math.sin(k / 2.3) + (d.getUTCDay() % 6 === 0 ? .6 : 0);
  return {date: d.toISOString().slice(0, 10), links: {Ethernet: [Math.round(2.4e9 * w), Math.round(1.6e8 * w)], 'Wi-Fi': [Math.round(1.1e9 * w), Math.round(7e7 * w)]}}; });
await post('/__test/state', {usage: {days}});
for (let k = 0; k < 3; k++) await push();

const browser = await chromium.launch(process.env.NETMERGE_BROWSER_EXECUTABLE ? {executablePath: process.env.NETMERGE_BROWSER_EXECUTABLE} : {});
const context = await browser.newContext({viewport: {width, height}, colorScheme: 'dark', reducedMotion: 'no-preference', deviceScaleFactor: 1, locale: 'en-US'});
await context.route('**/*', route => route.request().url().startsWith(origin) || /^(data|blob):/.test(route.request().url()) ? route.continue() : route.abort());
const page = await context.newPage();
const errors = []; page.on('pageerror', e => errors.push(e.message));
await page.goto(origin + '/#/overview'); await page.waitForFunction(() => document.querySelector('#power')?.disabled === false);
const feeder = setInterval(() => push().catch(() => {}), 1000);
await page.waitForTimeout(2500);

// caption overlay (video only)
await page.addStyleTag({content: `#tourCap{position:fixed;left:50%;bottom:34px;transform:translate(-50%,12px);z-index:99999;pointer-events:none;
  padding:14px 26px;border-radius:16px;font:600 21px "Segoe UI Variable Display","Segoe UI",system-ui,sans-serif;letter-spacing:-.2px;
  color:#f1f6ff;background:rgb(8 14 28 / .82);border:1px solid rgb(103 232 249 / .35);box-shadow:0 18px 48px -12px rgb(0 0 0 / .6),0 0 30px -10px rgb(103 232 249 / .5);
  opacity:0;transition:opacity .45s,transform .45s cubic-bezier(.2,.8,.2,1);white-space:nowrap}
  #tourCap.show{opacity:1;transform:translate(-50%,0)} #tourCap small{display:block;font:500 14px "Segoe UI",system-ui,sans-serif;color:#a3b2c9;margin-top:3px;text-align:center}`});
await page.evaluate(() => { const d = document.createElement('div'); d.id = 'tourCap'; document.body.append(d); });
const caption = async (text, sub = '') => { await page.evaluate(([a, b]) => { const c = document.getElementById('tourCap'); c.classList.remove('show'); setTimeout(() => { c.innerHTML = a + (b ? `<small>${b}</small>` : ''); if (a) c.classList.add('show'); }, a ? 260 : 0); }, [text, sub]); };
const go = async route => { await page.evaluate(h => { location.hash = h; window.scrollTo({top: 0}); }, '#/' + route); await page.waitForTimeout(400); };
const scroll = async y => { await page.evaluate(top => window.scrollTo({top, behavior: 'smooth'}), y); };
const wait = ms => page.waitForTimeout(ms);

// ---- recording ----
const dir = await mkdtemp(join(tmpdir(), 'nm-tour-'));
const cdp = await context.newCDPSession(page);
const frames = [];
cdp.on('Page.screencastFrame', async f => { const name = join(dir, `f${String(frames.length).padStart(5, '0')}.jpg`); frames.push({name, ts: f.metadata.timestamp}); await writeFile(name, Buffer.from(f.data, 'base64')); cdp.send('Page.screencastFrameAck', {sessionId: f.sessionId}).catch(() => {}); });
await cdp.send('Page.startScreencast', {format: 'jpeg', quality: 92, maxWidth: width, maxHeight: height, everyNthFrame: 1});
// the screencast sends a frame only when something changes; a moving cursor-free page still repaints at the animation's rate

if (script === 'short') {
  // a short loop of the Overview, for the README preview
  await wait(9000);
} else {
  await caption('NetMerge combines your internet connections', 'Ethernet, Wi-Fi and phone hotspots, working together on Windows'); await wait(5200);
  await caption('Each line flows into the hub at its real, measured speed', 'Downloads flow in · uploads flow back as small sparks'); await wait(6000);
  mode = 'down'; await caption('Wi-Fi loses internet', 'NetMerge leaves it out at once, so nothing slows down'); await wait(6000);
  mode = 'ok'; await caption('…and it comes back by itself', 'No clicks needed'); await wait(5000);
  await caption('Every line, live', 'Speed, upload, response time and share of the traffic'); await scroll(520); await wait(4200);
  await scroll(1020); await wait(3800);
  await go('connections'); await caption('Choose how your lines work together'); await wait(4500);
  // the speed test, as it runs: each line alone, then everything through NetMerge
  await go('connections/speed-test'); await caption('A speed test made for merged lines', 'Each line alone, all together, and through NetMerge'); await wait(1500);
  const phases = ['Testing Ethernet alone', 'Testing Wi-Fi alone', 'Testing all lines together', 'Testing through NetMerge with 8 downloads'];
  for (let k = 0; k <= 16; k++) {
    const live = [44, 47, 46, 23, 24, 22, 61, 66, 68, 67, 64, 66, 65, 67, 66, 68, 66][k];
    await post('/__test/state', {state: {test: {running: true, phase: phases[Math.min(3, Math.floor(k / 4.5))], progress: k / 17, live, results: [], scaling: [], finished: '0001-01-01T00:00:00Z'}}});
    await wait(420);
  }
  const results = [{name: 'Ethernet', label: 'Home fibre', mbps: 47.2, single: 41.8}, {name: 'Wi-Fi', label: 'Backup Wi-Fi', mbps: 23.6, single: 19.4}];
  await post('/__test/state', {state: {test: {running: false, phase: 'Done', progress: 1, live: 0, results, combined: 69.4, merged: 67.1,
    scaling: [{streams: 1, mbps: 41.5}, {streams: 4, mbps: 63.8}, {streams: 8, mbps: 67.1}, {streams: 16, mbps: 66.2}], suggest: {}, finished: new Date().toISOString()}}});
  await wait(3200);
  await go('usage'); await caption('Data per line, with allowances and a forecast'); await wait(4800);
  await go('protection/blocking'); await caption('Blocks ads and trackers for every app'); await wait(4300);
  await go('sharing/connect'); await caption('Share the merged internet with your phone', 'Scan one QR code with a free app'); await wait(4800);
  await go('settings/appearance'); await caption('Dark or light, with gentle animations'); await wait(2200);
  await page.click('.theme-preview[data-v="light"]'); await wait(2600);
  await go('overview'); await caption('NetMerge', 'Free and open source · github.com/nisar87/netmerge'); await wait(5500);
  await caption(''); await wait(600);
}

await cdp.send('Page.stopScreencast');
clearInterval(feeder);
await browser.close(); server.closeAllConnections(); await new Promise(done => server.close(done));
if (errors.length) { console.error(errors.join('\n')); process.exitCode = 1; }

// ---- assemble at the frames' real timing, then a steady 30 frames a second ----
const list = frames.map((f, i) => `file '${f.name}'\nduration ${Math.max(.001, ((frames[i + 1]?.ts ?? f.ts + .5) - f.ts)).toFixed(4)}`).join('\n') + `\nfile '${frames.at(-1).name}'\n`;
await writeFile(join(dir, 'list.txt'), list);
const args = ['-loglevel', 'error', '-y', '-f', 'concat', '-safe', '0', '-i', join(dir, 'list.txt'), '-vf', 'fps=30,format=yuv420p', '-c:v', 'libx264', '-preset', 'slow', '-crf', process.env.CRF || '19', '-movflags', '+faststart', out];
execFileSync('ffmpeg', args, {stdio: 'inherit'});
await rm(dir, {recursive: true, force: true});
console.log(`${out}: ${frames.length} frames`);
