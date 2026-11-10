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
  if (pts.length < 3) { setHTML(box, '<div class="hint2">Collecting data. The graph fills in as NetMerge runs.</div>'); return; }
  const W = 800, H = 150, now = Date.now()/1000, t0 = now - 86400;
  const names = local.links.filter(l => l.enabled).map(l => l.name);
  let mx = 1; pts.forEach(p => names.forEach(n => { mx = Math.max(mx, (p.d||{})[n]||0); }));
  const X = t => Math.max(0, (t - t0) / 86400 * W), Y = v => H - 6 - v / mx * (H - 14);
  const lines = names.map(n => {
    let d = '', prev = 0;
    pts.forEach(p => { const v = (p.d||{})[n]; if (v == null) return; d += (p.t - prev > 180 || !d ? 'M' : 'L') + X(p.t).toFixed(1) + ' ' + Y(v).toFixed(1) + ' '; prev = p.t; });
    return `<path d="${d}" fill="none" stroke="${colorOf(n)}" stroke-width="1.6" stroke-linejoin="round"/>`;
  }).join('');
  const lab = ['24 h ago','18 h','12 h','6 h','now'].map((t,i) => `<span>${t}</span>`).join('');
  setHTML(box, `<svg id="histSvg" viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" role="img" aria-label="Download speed over the last 24 hours"><line x1="0" x2="${W}" y1="${H-6}" y2="${H-6}" stroke="var(--line)"/>${lines}</svg><div class="row" style="justify-content:space-between;font-size:12px;color:var(--mute)">${lab}<span style="position:absolute"></span></div><div class="hint2">Highest point: ${fmtM(mx*1e6/8)} Mbps</div>`);
}
