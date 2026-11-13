/* ---------- Quantum Flow: the moving picture behind the merge diagram ---------- */
// Presentation only, drawn on a canvas over the SVG diagram of features/overview.js. Particles leave each connection
// at a rate that follows that connection's measured download speed (a log scale, so a slow line still shows life and
// a fast one does not flood the picture), travel its stream into the NetMerge hub and on towards the totals, keeping
// their colour all the way: every download still runs over one connection; NetMerge chooses which. Uploads travel the
// other way, as smaller pale sparks. A line that is off, unplugged, without internet or unused sends nothing; with
// merging off nothing moves. When a line drops, its particles fade out (red when it lost internet); when it comes
// back, one bright pulse runs along its stream. Nothing here is invented.
//
// Cost: it draws only while the Overview is on screen and the window is visible, at most 30 frames a second
// (15 with reduced motion), with a fixed cap on particles. If drawing a frame takes too long on a slow PC, it halves
// the particles and drops the trails by itself. Animations: Off stops it completely.
(() => {
  const MAX = 200;
  let box = null, svg = null, canvas = null, g = null, geo = null, raf = 0, last = 0, glow = 0, map = null;
  let cost = 0, quality = 1; // smoothed milliseconds per frame, and 1 = full detail, .5 = light
  const parts = [], pulses = [];
  const sprites = new Map();

  const motion = () => document.documentElement.dataset.motion || 'normal';
  const visible = () => box && box.isConnected && !document.hidden && !document.getElementById('pg-overview')?.hidden && box.clientWidth > 0;
  const hex = c => /^#[0-9a-f]{6}$/i.test(c) ? c : '#67e8f9';

  // a small soft dot per colour, drawn once and stamped many times
  function sprite(color, core = '#ffffff') {
    const k = color + core;
    if (sprites.has(k)) return sprites.get(k);
    const c = document.createElement('canvas'); c.width = c.height = 32;
    const x = c.getContext('2d'), r = x.createRadialGradient(16, 16, 0, 16, 16, 16);
    r.addColorStop(0, core); r.addColorStop(.2, color); r.addColorStop(.48, color + '55'); r.addColorStop(1, color + '00');
    x.fillStyle = r; x.fillRect(0, 0, 32, 32);
    sprites.set(k, c); return c;
  }
  const bez = (p, t) => { const u = 1 - t; return [
    u*u*u*p[0][0] + 3*u*u*t*p[1][0] + 3*u*t*t*p[2][0] + t*t*t*p[3][0],
    u*u*u*p[0][1] + 3*u*u*t*p[1][1] + 3*u*t*t*p[2][1] + t*t*t*p[3][1]]; };
  // particles per second for a measured speed: 0 when nothing flows, about 4.5 at 1 Mbps, about 21 at 100 Mbps
  const rate = m => m > 0.05 ? Math.min(24, 1.6 + 2.9 * Math.log2(1 + m)) : 0;

  // the canvas size and the SVG-to-canvas mapping are measured once per refresh (and on resize), never per frame:
  // measuring forces the browser to lay the page out again
  function measure() { map = box && svg && box.clientWidth > 0 ? size() : null; }
  function size() {
    const r = box.getBoundingClientRect(), dpr = Math.min(2, window.devicePixelRatio || 1);
    const w = Math.round(r.width * dpr), h = Math.round(r.height * dpr);
    if (canvas.width !== w || canvas.height !== h) { canvas.width = w; canvas.height = h; }
    const m = svg.getScreenCTM(); if (!m) return null;
    return {dpr, a: m.a * dpr, d: m.d * dpr, e: (m.e - r.left) * dpr, f: (m.f - r.top) * dpr};
  }

  function frame(now) {
    raf = 0;
    if (!geo || !visible() || motion() === 'off') { clear(); return; }
    const reduced = motion() === 'reduced', step = reduced ? 1000 / 15 : 1000 / 30;
    if (now - last < step - 2) { raf = requestAnimationFrame(frame); return; }
    const dt = Math.min(.1, (now - (last || now)) / 1000); last = now;
    const T = map; if (!T) { measure(); raf = requestAnimationFrame(frame); return; }
    const t0 = performance.now();
    const X = x => x * T.a + T.e, Y = y => y * T.d + T.f;
    const detail = reduced ? .5 : quality;
    // new particles: downloads from the line to the hub, uploads from the hub back to the line
    if (geo.run) geo.streams.forEach((s, i) => {
      if (!s.live) return;
      s.due = (s.due || 0) + rate(s.mbps) * detail * dt;
      s.dueUp = (s.dueUp || 0) + rate(s.up) * .35 * detail * dt;
      while (s.due >= 1 && parts.length < MAX) { s.due -= 1; parts.push({s: i, t: 0, v: .3 + Math.random() * .2, w: Math.random() * .8 - .4, o: false, up: false, die: 0}); }
      while (s.dueUp >= 1 && parts.length < MAX) { s.dueUp -= 1; parts.push({s: i, t: 1, v: .24 + Math.random() * .12, w: Math.random() * .6 - .3, o: false, up: true, die: 0}); }
      if (s.due > 3) s.due = 3; if (s.dueUp > 3) s.dueUp = 3;
    });
    g.setTransform(1, 0, 0, 1, 0, 0); g.clearRect(0, 0, canvas.width, canvas.height);
    const dark = document.documentElement.dataset.theme === 'dark';
    g.globalCompositeOperation = dark ? 'lighter' : 'source-over';
    const at = (q, s, t) => {
      if (q.o) { const a = geo.out[0], b = geo.out[1]; return [a[0] + (b[0] - a[0]) * t, a[1] + q.w * 7 * (1 - t)]; }
      const [x, y] = bez(s.p, t); return [x, y + q.w * 6 * Math.sin(t * Math.PI)];
    };
    let arrived = 0;
    for (let k = parts.length - 1; k >= 0; k--) {
      const q = parts[k], s = geo.streams[q.s];
      if (!s) { parts.splice(k, 1); continue; }
      // a line that stopped carrying traffic: what is on its way fades out where it is
      if (!q.die && !q.o && (!geo.run || !s.live)) { q.die = 1; q.red = s.state === 'down'; }
      if (q.die) { q.die -= dt * 1.6; if (q.die <= 0) { parts.splice(k, 1); continue; } }
      else if (q.up) { q.t -= q.v * dt; if (q.t <= 0) { parts.splice(k, 1); continue; } }
      else {
        q.t += q.v * dt * (q.o ? 2.2 : 1);
        if (!q.o && q.t >= 1) { arrived++; if (geo.out) { q.o = true; q.t = 0; } else { parts.splice(k, 1); continue; } }
        if (q.o && q.t >= 1) { parts.splice(k, 1); continue; }
      }
      const [x, y] = at(q, s, q.t);
      const fade = q.die ? q.die : q.up ? Math.min(1, (1 - q.t) * 5, q.t * 4) : q.o ? 1 - q.t : Math.min(1, q.t * 6);
      const col = q.red ? '#f87171' : s.color;
      const r = (q.up ? 3.6 : q.o ? 5.5 : 6.5) * T.a * (reduced ? .85 : 1);
      const img = q.up ? sprite(dark ? '#e2e8f0' : '#64748b', dark ? '#ffffff' : '#334155') : sprite(col);
      // a short trail behind each download particle (full detail only)
      if (detail >= 1 && !q.up && !q.die) for (let j = 1; j <= 1; j++) {
        const tt = q.t - q.v * .06 * (q.o ? 2.2 : 1); if (tt <= 0) break;
        const [px, py] = at(q, s, tt);
        g.globalAlpha = .3 * fade; const rr = r * .8;
        g.drawImage(img, X(px) - rr, Y(py) - rr, rr * 2, rr * 2);
      }
      g.globalAlpha = (q.up ? .7 : .9) * fade;
      g.drawImage(img, X(x) - r, Y(y) - r, r * 2, r * 2);
    }
    // a line that came back: one bright pulse along its stream
    for (let k = pulses.length - 1; k >= 0; k--) {
      const p = pulses[k], s = geo.streams.find(x => x.name === p.name);
      p.t += dt * 1.1; if (!s || p.t >= 1) { pulses.splice(k, 1); continue; }
      const [x, y] = bez(s.p, p.t), r = 16 * T.a * (1 - p.t * .3);
      g.globalAlpha = .9 * Math.sin(Math.PI * p.t);
      g.drawImage(sprite(s.color), X(x) - r, Y(y) - r, r * 2, r * 2);
    }
    // the hub brightens a little with what arrives (smoothed), and settles when traffic stops
    glow += ((reduced ? 0 : Math.min(1, arrived * 4)) - glow) * Math.min(1, dt * 3);
    if (glow > .02 && !geo.compact) {
      const cx = X(geo.cx), cy = Y(geo.cy), rr = geo.R * T.a * 1.28;
      const hg = g.createRadialGradient(cx, cy, rr * .6, cx, cy, rr);
      const c = hex(getComputedStyle(box).getPropertyValue('--hub-glow').trim());
      hg.addColorStop(0, c + '00'); hg.addColorStop(.72, c + Math.round(glow * 36).toString(16).padStart(2, '0')); hg.addColorStop(1, c + '00');
      g.globalAlpha = 1; g.fillStyle = hg; g.beginPath(); g.arc(cx, cy, rr, 0, Math.PI * 2); g.fill();
    }
    g.globalAlpha = 1; g.globalCompositeOperation = 'source-over';
    // keep an eye on what a frame costs; a slow PC gets lighter detail (and full detail back once it can afford it)
    cost += (performance.now() - t0 - cost) * .1;
    if (quality === 1 && cost > 6) quality = .5; else if (quality < 1 && cost < 2) quality = 1;
    if (parts.length || pulses.length || glow > .02 || (geo.run && geo.streams.some(s => s.live && (s.mbps > 0.05 || s.up > 0.05)))) raf = requestAnimationFrame(frame);
  }

  function clear() {
    parts.length = 0; pulses.length = 0; glow = 0;
    if (g && canvas) { g.setTransform(1, 0, 0, 1, 0, 0); g.clearRect(0, 0, canvas.width, canvas.height); }
  }
  function kick() { if (!raf && visible() && motion() !== 'off') { last = 0; raf = requestAnimationFrame(frame); } }

  window.QF = {
    // called on every refresh of the Overview with the diagram's geometry and the measured speeds
    update(b, s, next) {
      if (b !== box || s !== svg) { box = b; svg = s; canvas = b.querySelector('.qf-canvas'); g = canvas?.getContext('2d'); parts.length = 0; pulses.length = 0; }
      if (!g) return;
      const old = new Map((geo?.streams || []).map(x => [x.name, x]));
      next.streams.forEach(x => {
        x.live = next.run && ['moving', 'idle'].includes(x.state);
        const o = old.get(x.name);
        // keep each stream's emission phase across refreshes so the flow does not stutter every second
        if (o) { x.due = o.due; x.dueUp = o.dueUp; if (x.live && !o.live && geo.run && motion() !== 'off') pulses.push({name: x.name, t: 0}); }
      });
      if (geo && next.streams.map(x => x.name).join() !== geo.streams.map(x => x.name).join()) parts.length = 0;
      geo = next;
      measure();
      kick();
    },
    kick, clear,
    // for tests: particles moving now (downloads and uploads), pulses running, and the detail level
    get count() { return parts.length; },
    get uploads() { return parts.filter(p => p.up && !p.die).length; },
    get pulses() { return pulses.length; },
    get quality() { return quality; },
    get frameMs() { return +cost.toFixed(2); },
  };
  document.addEventListener('visibilitychange', () => { if (document.hidden) { cancelAnimationFrame(raf); raf = 0; } else kick(); });
  addEventListener('hashchange', () => setTimeout(() => { measure(); kick(); }, 50));
  addEventListener('resize', () => { map = null; });
})();
