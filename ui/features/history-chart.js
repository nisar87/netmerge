/* ---------- 24-hour history ---------- */
let histAt = 0, histPts = null;
async function loadHistory(){
  if (Date.now() - histAt < 55000) return; histAt = Date.now();
  try { histPts = (await api('history')).points || []; } catch(e){ return; }
  paintHistory();
}
function paintHistory(){
  const box = $('#histBox'); if (!box || !local) return;
  const pts = histPts || [];
  if (pts.length < 3) { setHTML(box, '<div class="hist-empty"><span data-icon="clock"></span><span>Collecting data. The graph fills in as NetMerge runs.</span></div>'); return; }
  // History points are already in Mbps, one per minute.
  const W = 800, H = 200, now = Date.now()/1000, t0 = now - 86400;
  const names = local.links.filter(l => l.enabled).map(l => l.name);
  let raw = 1; pts.forEach(p => names.forEach(n => { raw = Math.max(raw, (p.d||{})[n]||0); }));
  const mx = niceMax(raw * 1.1);
  const X = t => Math.max(0, Math.min(W, (t - t0) / 86400 * W)), Y = v => H - v / mx * (H - 8);
  const stats = names.map(n => { let peak = 0, sum = 0, cnt = 0; pts.forEach(p => { const v = (p.d||{})[n]; if (v == null) return; peak = Math.max(peak, v); sum += v; cnt++; }); return {n, peak, avg: cnt ? sum / cnt : 0}; });
  const series = names.map((n, i) => {
    const col = colorOf(n); let d = '', segs = [], cur = [], prev = 0;
    pts.forEach(p => { const v = (p.d||{})[n]; if (v == null) return; if (cur.length && p.t - prev > 180) { segs.push(cur); cur = []; } cur.push([X(p.t), Y(v)]); prev = p.t; });
    if (cur.length) segs.push(cur);
    const line = segs.map(s => s.map((q, j) => (j ? 'L' : 'M') + q[0].toFixed(1) + ' ' + q[1].toFixed(1)).join(' ')).join(' ');
    const area = segs.filter(s => s.length > 1).map(s => s.map((q, j) => (j ? 'L' : 'M') + q[0].toFixed(1) + ' ' + q[1].toFixed(1)).join(' ') + ` L${s.at(-1)[0].toFixed(1)} ${H} L${s[0][0].toFixed(1)} ${H} Z`).join(' ');
    return `<defs><linearGradient id="hg${i}" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="${col}" stop-opacity=".22"/><stop offset="1" stop-color="${col}" stop-opacity="0"/></linearGradient></defs><path d="${area}" fill="url(#hg${i})"/><path d="${line}" fill="none" stroke="${col}" stroke-width="1.6" stroke-linejoin="round" vector-effect="non-scaling-stroke"/>`;
  }).join('');
  const grid = [0, .25, .5, .75, 1].map(k => `<line x1="0" x2="${W}" y1="${Y(mx*k).toFixed(1)}" y2="${Y(mx*k).toFixed(1)}" vector-effect="non-scaling-stroke"/>`).join('');
  const yLab = [1, .5, 0].map(k => `<span style="top:${(Y(mx*k)/H*100).toFixed(1)}%">${fmt(mx*k)}</span>`).join('');
  const xLab = ['24 h ago','18 h','12 h','6 h','now'].map(t => `<span>${t}</span>`).join('');
  const legend = stats.map(s => `<div class="hist-key" style="--c:${colorOf(s.n)}"><i></i><b>${esc(labelOf(s.n))}</b><span>peak <em class="num">${fmt(s.peak)}</em> · average <em class="num">${fmt(s.avg)}</em> Mbps</span></div>`).join('');
  setHTML(box, `<div class="hist-plot"><svg id="histSvg" viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" role="img" aria-label="Download speed over the last 24 hours. ${esc(stats.map(s => `${labelOf(s.n)}: peak ${fmt(s.peak)} Mbps, average ${fmt(s.avg)} Mbps`).join('; '))}"><g class="hist-grid">${grid}</g>${series}</svg><div class="hist-y" aria-hidden="true">${yLab}</div></div><div class="hist-x" aria-hidden="true">${xLab}</div><div class="hist-legend">${legend}</div>`);
}
