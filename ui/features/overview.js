/* ---------- overview: merge picture, live speed by connection, activity ---------- */
// Presentation only. Every number comes from the existing /api/state reading; nothing here changes what NetMerge does.
const SVGNS = 'http://www.w3.org/2000/svg';
const svgIcon = (kind, x, y, size) => `<svg x="${x}" y="${y}" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><use href="assets/icons.svg#${({ethernet:'cable',wifi:'wifi',mobile:'smartphone'})[kind]||'cable'}"></use></svg>`;
const shortLabel = (s, n) => { s = String(s ?? ''); return s.length > n ? s.slice(0, n - 1) + '…' : s; };
const isWorking = (l, run) => run && l.present && l.enabled && l.healthy === true && !l.standby && !l.slow && !l.portal && !l.quotaOut;

function renderActivity(sn){
  const a = (sn.activity || []).slice(0, 6);
  setHTML($('#actList'), a.length ? '<ol class="ov-timeline">' + a.map(e => { const d = new Date(e.time*1000); return `<li class="${esc(e.kind||'info')}"><span class="ov-tl-dot" aria-hidden="true"></span><time>${d.toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'})}</time><span>${esc(e.text)}</span></li>`; }).join('') + '</ol>' : '<div class="hint2">Nothing yet. NetMerge writes here whenever it changes something and says why.</div>');
}

function renderStatus(sn, run){
  const st = statusLine(sn, run);
  const tone = st[0] || 'off';
  setHTML($('#statusLine'), `<span class="ov-pill ${tone}"><span class="dot ${st[0]}"></span>${run ? 'Merging on' : 'Merging off'}</span><div><h2 id="ovStatusTitle">${esc(st[1])}</h2><p class="hint2">${esc(st[2])}</p></div>`);
}

function renderOverview(sn, run){
  renderStatus(sn, run);
  renderActivity(sn);
  const links = sn.links || [];
  const act = NM.contributingLinks(sn);
  $('#totalUp').innerHTML = `${fmtM(sn.totalUp)}<small>Mbps</small>`;
  $('#peak').innerHTML = `${fmtM(sn.peakDown)}<small>Mbps</small>`;
  $('#session').textContent = bytes(links.reduce((a,l)=>a+(l.totalRx||0),0));
  $('#uptime').textContent = run ? dur(sn.uptime) : '—';
  $('#contribHint').textContent = run ? `${act.length} of ${links.filter(l=>l.present).length} connection${links.length===1?'':'s'} contributing` : 'Merging is off';
  renderMergeFlow(sn, run);
  // The legend is a colour key only: exact rates are in the connection cards and in the table below.
  setHTML($('#legend'), links.filter(l=>l.present && l.enabled).map(l => `<div><i style="background:${colorOf(l.name)}"></i>${esc(l.label || l.name)}</div>`).join(''));
  if(!chartPaused) drawChart();
  setHTML($('#chartTable'),'<thead><tr><th scope="col">Connection</th><th scope="col">Download (Mbps)</th><th scope="col">Upload (Mbps)</th><th scope="col">Status</th></tr></thead><tbody>'+links.map(l=>`<tr><th scope="row">${esc(l.label || l.name)}</th><td>${fmtM(l.down)}</td><td>${fmtM(l.up)}</td><td>${esc(statusOf(l,sn.running)[0])}</td></tr>`).join('')+'</tbody>');
}

/* The merge picture: each connection flows into one speedometer. The needle and the coloured arc show the combined
   download against a scale that follows the top speed since NetMerge opened; the thin inner ring shows each line's share.
   Every connection NetMerge knows is drawn, also one that is unplugged, switched off or without internet, so a line
   going down or coming back changes its look in place instead of the picture being rebuilt. */
let gaugeTop = 10;
// what a line is doing, for the picture: one word per state, shown as a class on its stream
function flowState(l, run){
  if (!l.present) return 'gone';
  if (!l.enabled) return 'off';
  if (!run) return 'ready';
  if (l.healthy === false) return 'down';
  if (l.portal || l.quotaOut) return 'held';
  if (l.standby || l.slow) return 'standby';
  if (l.healthy == null) return 'checking';
  return mbps(l.down) > 0.3 ? 'moving' : 'idle';
}
function renderMergeFlow(sn, run){
  const box = $('#mergeFlow'); if (!box) return;
  const all = sn.links || [];
  const shown = all.filter(l => l.present && l.enabled);
  const compact = box.clientWidth > 0 && box.clientWidth < 520;
  const n = all.length;
  const W = compact ? 300 : 780, rowH = 82, H = compact ? 290 : Math.max(330, n * rowH + 48);
  const cx = compact ? 150 : 556, cy = H/2 - 4, R = 118, arcW = 14, inner = R - 40;
  // scale: a round number above the fastest reading of this session, so the needle has room to move
  const now = mbps(sn.totalDown) || 0, peak = Math.max(mbps(sn.peakDown) || 0, now);
  const top = Math.max(10, niceMax(peak * 1.15));
  const key = (compact ? 'c|' : 'w|') + all.map(l => l.name).join('|') + '|' + top;
  const P = (r, deg) => { const a = deg * Math.PI / 180; return [cx + r * Math.cos(a), cy + r * Math.sin(a)]; };
  const arc = r => { const [x1, y1] = P(r, 135), [x2, y2] = P(r, 405); return `M${x1.toFixed(1)} ${y1.toFixed(1)} A${r} ${r} 0 1 1 ${x2.toFixed(1)} ${y2.toFixed(1)}`; };
  // each stream ends on the hub's rim at its own angle, so the lines visibly converge instead of piling into one point
  const y0 = cy - (n - 1) * rowH / 2, hubR = R + 24;
  const stream = i => {
    const y = y0 + i * rowH, spread = n > 1 ? (i - (n - 1) / 2) / Math.max(1, n - 1) : 0;
    const [ex, ey] = P(hubR, 180 - spread * Math.min(16, 6 * n)), x0 = 200, dx = ex - x0;
    return [[x0, y], [x0 + dx * .48, y], [ex - dx * .38, ey + (ey - cy) * .15], [ex, ey]];
  };
  const d = q => `M${q[0][0].toFixed(1)} ${q[0][1].toFixed(1)} C ${q[1][0].toFixed(1)} ${q[1][1].toFixed(1)}, ${q[2][0].toFixed(1)} ${q[2][1].toFixed(1)}, ${q[3][0].toFixed(1)} ${q[3][1].toFixed(1)}`;
  if (box.dataset.key !== key) {
    box.dataset.key = key; gaugeTop = top;
    const nodes = compact ? '' : all.map((l, i) => {
      const q = stream(i), y = q[0][1], gid = `mfg${i}`;
      return `<g class="mf-node" data-n="${esc(l.name)}" style="--c:${colorOf(l.name)}"><title data-f="title"></title>
        <linearGradient id="${gid}" gradientUnits="userSpaceOnUse" x1="${q[0][0]}" y1="0" x2="${q[3][0].toFixed(1)}" y2="0"><stop offset="0" class="mf-g-a"/><stop offset="1" class="mf-g-b"/></linearGradient>
        <path class="mf-ribbon-glow" d="${d(q)}"/>
        <path class="mf-ribbon" d="${d(q)}" stroke="url(#${gid})"/>
        <path class="mf-flow" d="${d(q)}"/>
        <rect class="mf-card" x="0" y="${y - 30}" width="200" height="60" rx="14"/>
        <rect class="mf-icon" x="11" y="${y - 19}" width="38" height="38" rx="10"/>
        ${svgIcon(l.kind, 20, y - 10, 20)}
        <circle class="mf-dot" cx="187" cy="${y - 15}" r="4"/>
        <text class="mf-name" x="60" y="${y - 4}">${esc(shortLabel(l.label || l.name, 16))}</text>
        <text class="mf-state" x="60" y="${y + 15}" data-f="state"></text>
      </g>`;
    }).join('');
    // fewer, clearer ticks: a long one every quarter of the scale, short ones in between
    const ticks = Array.from({length: 21}, (_, k) => { const deg = 135 + 270 * k / 20, major = k % 5 === 0;
      const [x1, y1] = P(R - arcW/2 - 6, deg), [x2, y2] = P(R - arcW/2 - (major ? 15 : 10), deg);
      return `<line class="mf-tick${major ? ' major' : ''}" x1="${x1.toFixed(1)}" y1="${y1.toFixed(1)}" x2="${x2.toFixed(1)}" y2="${y2.toFixed(1)}"/>`; }).join('');
    const labels = [0, .25, .5, .75, 1].map(k => { const [x, y] = P(R + 22, 135 + 270 * k); return `<text class="mf-scale" x="${x.toFixed(1)}" y="${(y + 4).toFixed(1)}" text-anchor="middle">${fmt(top * k).replace(/\.0+$/, '')}</text>`; }).join('');
    const C = 2 * Math.PI * inner;
    const segs = shown.map(l => `<circle class="mf-seg" data-n="${esc(l.name)}" cx="${cx}" cy="${cy}" r="${inner}" stroke="${colorOf(l.name)}" stroke-width="4" stroke-dasharray="0 ${C}" transform="rotate(-90 ${cx} ${cy})"/>`).join('');
    const [nx1] = P(inner + 8, 0), [nx2] = P(R + arcW/2 + 4, 0);
    // the combined traffic leaves the hub towards the totals on the right (the streams keep their own colours:
    // each connection still carries its own downloads; NetMerge shares them out, it does not fuse one download)
    const out = compact ? '' : `<linearGradient id="mfOutGrad" gradientUnits="userSpaceOnUse" x1="${cx + hubR}" y1="0" x2="${W}" y2="0"><stop offset="0" class="mf-out-a"/><stop offset="1" class="mf-out-b"/></linearGradient><path class="mf-out" d="M${cx + hubR} ${cy} H ${W}" stroke="url(#mfOutGrad)"/>`;
    const logo = `<g class="mf-logo" transform="translate(${cx - 11} ${cy - 70}) scale(.69)"><path d="M6 9c7 0 7 7 13 7M6 23c7 0 7-7 13-7h7" fill="none" stroke-width="2.8" stroke-linecap="round"/><circle cx="26" cy="16" r="2.4"/></g>`;
    box.innerHTML = `<canvas class="qf-canvas" aria-hidden="true"></canvas><svg viewBox="0 0 ${W} ${H}" aria-hidden="true" focusable="false">
      <defs><radialGradient id="mfHalo"><stop offset="0" class="mf-halo-in"/><stop offset="1" class="mf-halo-out"/></radialGradient>
        <filter id="mfSoft" x="-20%" y="-60%" width="140%" height="220%"><feGaussianBlur stdDeviation="7"/></filter></defs>
      <circle class="mf-halo" cx="${cx}" cy="${cy}" r="${R + 54}" fill="url(#mfHalo)"/>
      ${out}${nodes}
      <circle class="mf-glow" cx="${cx}" cy="${cy}" r="${R - arcW/2 - 2}"/>
      <path class="mf-track" d="${arc(R)}" stroke-width="${arcW}"/>
      <path class="mf-arc" data-f="arc" d="${arc(R)}" stroke-width="${arcW}" pathLength="100" stroke-dasharray="0 100"/>
      ${ticks}${labels}
      <circle class="mf-inner-track" cx="${cx}" cy="${cy}" r="${inner}" stroke-width="4"/>
      ${segs}
      <g class="mf-needle" data-f="needle" style="transform-origin:${cx}px ${cy}px;transform:rotate(135deg)"><line x1="${nx1.toFixed(1)}" y1="${cy}" x2="${nx2.toFixed(1)}" y2="${cy}"/><circle cx="${nx2.toFixed(1)}" cy="${cy}" r="4"/></g>
      ${logo}
      <text class="mf-hub-label" x="${cx}" y="${cy - 30}" text-anchor="middle" data-f="hubLabel"></text>
      <text class="mf-hub-value" x="${cx}" y="${cy + 16}" text-anchor="middle" data-f="hubValue"></text>
      <text class="mf-hub-unit" x="${cx}" y="${cy + 38}" text-anchor="middle">Mbps down</text>
      <text class="mf-tier" x="${cx}" y="${cy + R * .8}" text-anchor="middle" data-f="tier"></text>
      <text class="mf-of" x="${cx}" y="${cy + R * .8 + 16}" text-anchor="middle">scale 0–${fmt(top).replace(/\.0+$/, '')} Mbps</text>
    </svg>`;
  }
  const svg = box.querySelector('svg'); if (!svg) return;
  const f = (root, k) => root.querySelector(`[data-f="${k}"]`);
  const total = shown.reduce((a, l) => a + (Number.isFinite(l.down) ? l.down : 0), 0);
  const C = 2 * Math.PI * inner, gap = shown.length > 1 ? 3 : 0;
  let offset = 0;
  shown.forEach(l => {
    const share = total > 0 ? (l.down || 0) / total : 0;
    const seg = svg.querySelector(`.mf-seg[data-n="${CSS.escape(l.name)}"]`);
    if (seg) { const len = Math.max(0, share * C - gap); seg.setAttribute('stroke-dasharray', `${len.toFixed(1)} ${C.toFixed(1)}`); seg.setAttribute('stroke-dashoffset', (-offset).toFixed(1)); }
    offset += share * C;
  });
  all.forEach(l => {
    const node = svg.querySelector(`.mf-node[data-n="${CSS.escape(l.name)}"]`); if (!node) return;
    const state = flowState(l, run), share = total > 0 && l.present && l.enabled ? (l.down || 0) / total : 0;
    if (node.dataset.state !== state) node.dataset.state = state;
    // kept for the earlier classes the styles and tests know
    const working = isWorking(l, run);
    node.classList.toggle('idle', !working); node.classList.toggle('moving', state === 'moving');
    node.classList.toggle('down', state === 'down');
    const w = (5 + Math.round(share * 20)) + 'px';
    node.querySelector('.mf-ribbon').style.strokeWidth = w; node.querySelector('.mf-ribbon-glow').style.strokeWidth = w;
    f(node, 'state').textContent = statusOf(l, run)[0];
    f(node, 'title').textContent = `${l.label || l.name}: ${statusOf(l, run)[0]}${l.present ? ` · ${fmtM(l.down)} Mbps down, ${fmtM(l.up)} up · ${Math.round(share * 100)}% of the download` : ''}`;
  });
  svg.classList.toggle('off', !run);
  svg.classList.toggle('busy', run && total > 0);
  // a narrow window shows the speedometer alone: no streams to draw
  if (window.QF) QF.update(box, svg, {W, H, cx, cy, R, compact, run, out: compact ? null : [[cx + hubR, cy], [W, cy]], streams: compact ? [] : all.map((l, i) => {
    const working = run && isWorking(l, run);
    return {name: l.name, color: colorOf(l.name), state: flowState(l, run),
      mbps: working ? Math.max(0, mbps(l.down) || 0) : 0, up: working ? Math.max(0, mbps(l.up) || 0) : 0, p: stream(i)};
  })});
  const p = run ? Math.max(0, Math.min(1, now / gaugeTop)) : 0, col = speedColor(p);
  f(svg, 'arc').setAttribute('stroke-dasharray', `${(p * 100).toFixed(2)} 100`);
  svg.style.setProperty('--speed', col);
  f(svg, 'needle').style.transform = `rotate(${(135 + 270 * p).toFixed(1)}deg)`;
  f(svg, 'hubLabel').textContent = run ? 'COMBINED' : 'ADAPTERS';
  tweenNumber(f(svg, 'hubValue'), mbps(sn.totalDown), !run);
  f(svg, 'tier').textContent = !run ? 'Merging off' : speedTier(p);
  const parts = shown.map(l => `${l.label || l.name} ${fmtM(l.down)}`).join(', ');
  box.setAttribute('aria-label', `${run ? 'Combined' : 'Adapter'} download speed ${fmtM(sn.totalDown)} Mbps of a ${fmt(gaugeTop).replace(/\.0+$/, '')} Mbps scale${run ? ', ' + speedTier(p).toLowerCase() : ''}${parts ? '. ' + parts + ' Mbps' : ''}.`);
}

/* the combined speed glides to its new value in about half a second; a state change (merging off), reduced or no
   animation, or a value that is not a number shows at once */
function tweenNumber(el, to, instant){
  const text = fmt(to);
  if (!Number.isFinite(to) || instant || document.documentElement.dataset.motion !== 'normal' || document.hidden) { el._t = null; el.textContent = text; el._v = to; return; }
  const from = Number.isFinite(el._v) ? el._v : to; el._v = to;
  if (Math.abs(from - to) < 0.005 || !el.textContent) { el.textContent = text; return; }
  // a few steps, not one per frame: each change of the text makes the browser lay out the picture again
  const t0 = performance.now(), job = {};
  el._t = job;
  let shown = 0;
  const step = now => {
    if (el._t !== job) return;
    const k = Math.min(1, (now - t0) / 480), e = 1 - Math.pow(1 - k, 3);
    if (k >= 1 || now - shown >= 80) { shown = now; el.textContent = k >= 1 ? text : fmt(from + (to - from) * e); }
    if (k < 1) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}
