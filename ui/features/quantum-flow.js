/* ---------- Quantum Flow: the moving picture behind the merge diagram ---------- */
// Presentation only, drawn on a canvas under the SVG diagram of features/overview.js. Particles leave each
// connection at a rate that follows that connection's measured download speed (a log scale, so a slow line still
// shows life and a fast one does not flood the picture), travel its stream into the NetMerge hub and on towards the
// totals, keeping their colour all the way: every download still runs over one connection; NetMerge chooses which.
// A line that is off, down or not used shows no particles; with merging off nothing moves. Nothing here is invented.
//
// Cost: it draws only while the Overview is on screen and the window is visible, at most 30 frames a second
// (15 with reduced motion), with a fixed cap on particles, and stops completely with animations turned off.
(() => {
  const MAX = 240;
  let box = null, svg = null, canvas = null, g = null, geo = null, raf = 0, last = 0, acc = 0, glow = 0;
  const parts = [];
  const sprites = new Map();

  const motion = () => document.documentElement.dataset.motion || 'normal';
  const visible = () => box && box.isConnected && !document.hidden && !document.getElementById('pg-overview')?.hidden && box.clientWidth > 0;

  // a small soft dot per colour, drawn once and stamped many times
  function sprite(color) {
    if (sprites.has(color)) return sprites.get(color);
    const c = document.createElement('canvas'); c.width = c.height = 32;
    const x = c.getContext('2d'), r = x.createRadialGradient(16, 16, 0, 16, 16, 16);
    r.addColorStop(0, '#ffffff'); r.addColorStop(.18, color); r.addColorStop(.45, color + '66'); r.addColorStop(1, color + '00');
    x.fillStyle = r; x.fillRect(0, 0, 32, 32);
    sprites.set(color, c); return c;
  }
  const bez = (p, t) => { const u = 1 - t; return [
    u*u*u*p[0][0] + 3*u*u*t*p[1][0] + 3*u*t*t*p[2][0] + t*t*t*p[3][0],
    u*u*u*p[0][1] + 3*u*u*t*p[1][1] + 3*u*t*t*p[2][1] + t*t*t*p[3][1]]; };
  // particles per second for a measured speed: 0 when nothing flows, about 6 at 1 Mbps, about 26 at 100 Mbps
  const rate = m => m > 0.05 ? Math.min(30, 2 + 3.6 * Math.log2(1 + m)) : 0;

  // the canvas size and the SVG-to-canvas mapping are measured once per refresh (and on resize), never per frame:
  // measuring forces the browser to lay the page out again
  let map = null;
  function measure() { map = box && svg && box.clientWidth > 0 ? size() : null; }
  function size() {
    const r = box.getBoundingClientRect(), dpr = Math.min(2, window.devicePixelRatio || 1);
    const w = Math.round(r.width * dpr), h = Math.round(r.height * dpr);
    if (canvas.width !== w || canvas.height !== h) { canvas.width = w; canvas.height = h; }
    // the SVG keeps its aspect ratio inside the box: map its coordinates onto the canvas
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
    const X = p => p[0] * T.a + T.e, Y = p => p[1] * T.d + T.f;
    // new particles
    if (geo.run) geo.streams.forEach((s, i) => {
      s.due = (s.due || 0) + rate(s.mbps) * (reduced ? .5 : 1) * dt;
      while (s.due >= 1 && parts.length < MAX) { s.due -= 1; parts.push({s: i, t: 0, v: .32 + Math.random() * .22, w: Math.random() * .7 - .35, o: false}); }
      if (s.due > 3) s.due = 3;
    });
    g.setTransform(1, 0, 0, 1, 0, 0); g.clearRect(0, 0, canvas.width, canvas.height);
    const dark = document.documentElement.dataset.theme === 'dark';
    g.globalCompositeOperation = dark ? 'lighter' : 'source-over';
    let arrived = 0;
    for (let k = parts.length - 1; k >= 0; k--) {
      const q = parts[k], s = geo.streams[q.s];
      if (!s) { parts.splice(k, 1); continue; }
      q.t += q.v * dt * (q.o ? 2.2 : 1);
      if (!q.o && q.t >= 1) { if (geo.out) { q.o = true; q.t = 0; arrived++; } else { parts.splice(k, 1); arrived++; continue; } }
      if (q.o && q.t >= 1) { parts.splice(k, 1); continue; }
      let x, y;
      if (q.o) { const a = geo.out[0], b = geo.out[1]; x = a[0] + (b[0] - a[0]) * q.t; y = a[1] + q.w * 6; }
      else { [x, y] = bez(s.p, q.t); y += q.w * 5 * Math.sin(q.t * Math.PI); }
      const fade = q.o ? 1 - q.t : Math.min(1, q.t * 6);
      const r = (q.o ? 5.5 : 6.5) * T.a * (reduced ? .85 : 1);
      g.globalAlpha = .85 * fade;
      g.drawImage(sprite(s.color), X([x, 0]) - r, Y([0, y]) - r, r * 2, r * 2);
    }
    // the hub brightens a little with what arrives (smoothed), and settles when traffic stops
    glow += ((reduced ? 0 : Math.min(1, arrived * 4)) - glow) * Math.min(1, dt * 3);
    if (glow > .02 && !geo.compact) {
      const cx = X([geo.cx, 0]), cy = Y([0, geo.cy]), rr = geo.R * T.a * 1.25;
      const hg = g.createRadialGradient(cx, cy, rr * .55, cx, cy, rr);
      const c = getComputedStyle(box).getPropertyValue('--hub-glow').trim() || '#67e8f9';
      hg.addColorStop(0, c + '00'); hg.addColorStop(.7, c + Math.round(glow * 40).toString(16).padStart(2, '0')); hg.addColorStop(1, c + '00');
      g.globalAlpha = 1; g.fillStyle = hg; g.beginPath(); g.arc(cx, cy, rr, 0, Math.PI * 2); g.fill();
    }
    g.globalAlpha = 1; g.globalCompositeOperation = 'source-over';
    if (parts.length || glow > .02 || (geo.run && geo.streams.some(s => s.mbps > 0.05))) raf = requestAnimationFrame(frame);
  }

  function clear() {
    parts.length = 0; glow = 0;
    if (g && canvas) { g.setTransform(1, 0, 0, 1, 0, 0); g.clearRect(0, 0, canvas.width, canvas.height); }
  }
  function kick() { if (!raf && visible() && motion() !== 'off') { last = 0; raf = requestAnimationFrame(frame); } }

  window.QF = {
    // called on every refresh of the Overview with the diagram's geometry and the measured speeds
    update(b, s, next) {
      if (b !== box || s !== svg) { box = b; svg = s; canvas = b.querySelector('.qf-canvas'); g = canvas?.getContext('2d'); parts.length = 0; }
      if (!g) return;
      const names = new Map((geo?.streams || []).map((x, i) => [x.name, i]));
      // keep each stream's emission phase across refreshes so the flow does not stutter every second
      next.streams.forEach(x => { const old = geo?.streams[names.get(x.name)]; if (old) x.due = old.due; });
      if (geo && next.streams.map(x => x.name).join() !== geo.streams.map(x => x.name).join()) parts.length = 0;
      geo = next;
      measure();
      if (!next.run) parts.length = 0;
      kick();
    },
    kick, clear,
    // for tests: how many particles are moving now
    get count() { return parts.length; },
  };
  document.addEventListener('visibilitychange', () => { if (document.hidden) { cancelAnimationFrame(raf); raf = 0; } else kick(); });
  addEventListener('hashchange', () => setTimeout(() => { measure(); kick(); }, 50));
  addEventListener('resize', () => { map = null; });
})();
