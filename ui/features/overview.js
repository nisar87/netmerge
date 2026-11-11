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
   download against a scale that follows the top speed since NetMerge opened; the thin inner ring shows each line's share. */
let gaugeTop = 10;
function renderMergeFlow(sn, run){
  const box = $('#mergeFlow'); if (!box) return;
  const shown = (sn.links || []).filter(l => l.present && l.enabled);
  const compact = box.clientWidth > 0 && box.clientWidth < 520;
  const n = shown.length;
  const W = compact ? 300 : 720, rowH = 74, H = compact ? 290 : Math.max(300, n * rowH + 40);
  const cx = compact ? 150 : 560, cy = H/2 - 6, R = 110, arcW = 16, inner = R - 40;
  // scale: a round number above the fastest reading of this session, so the needle has room to move
  const now = mbps(sn.totalDown) || 0, peak = Math.max(mbps(sn.peakDown) || 0, now);
  const top = Math.max(10, niceMax(peak * 1.15));
  const key = (compact ? 'c|' : 'w|') + shown.map(l => l.name).join('|') + '|' + top;
  const P = (r, deg) => { const a = deg * Math.PI / 180; return [cx + r * Math.cos(a), cy + r * Math.sin(a)]; };
  const arc = r => { const [x1, y1] = P(r, 135), [x2, y2] = P(r, 405); return `M${x1.toFixed(1)} ${y1.toFixed(1)} A${r} ${r} 0 1 1 ${x2.toFixed(1)} ${y2.toFixed(1)}`; };
  if (box.dataset.key !== key) {
    box.dataset.key = key; gaugeTop = top;
    const y0 = cy - (n - 1) * rowH / 2;
    const nodes = compact ? '' : shown.map((l, i) => {
      const y = y0 + i * rowH;
      return `<g class="mf-node" data-n="${esc(l.name)}" style="--c:${colorOf(l.name)}">
        <path class="mf-ribbon" d="M196 ${y} C 330 ${y}, 360 ${cy}, ${cx - R - 30} ${cy}"/>
        <path class="mf-flow" d="M196 ${y} C 330 ${y}, 360 ${cy}, ${cx - R - 30} ${cy}"/>
        <rect class="mf-card" x="0" y="${y - 28}" width="196" height="56" rx="12"/>
        <rect class="mf-icon" x="10" y="${y - 18}" width="36" height="36" rx="9"/>
        ${svgIcon(l.kind, 18, y - 10, 20)}
        <text class="mf-name" x="56" y="${y - 4}">${esc(shortLabel(l.label || l.name, 17))}</text>
        <text class="mf-state" x="56" y="${y + 14}" data-f="state"></text>
      </g>`;
    }).join('');
    const ticks = Array.from({length: 41}, (_, k) => { const deg = 135 + 270 * k / 40, major = k % 10 === 0, mid = k % 5 === 0;
      const [x1, y1] = P(R - arcW/2 - 5, deg), [x2, y2] = P(R - arcW/2 - (major ? 15 : mid ? 11 : 8), deg);
      return `<line class="mf-tick${major ? ' major' : ''}" x1="${x1.toFixed(1)}" y1="${y1.toFixed(1)}" x2="${x2.toFixed(1)}" y2="${y2.toFixed(1)}"/>`; }).join('');
    const labels = [0, .25, .5, .75, 1].map(k => { const [x, y] = P(R + 22, 135 + 270 * k); return `<text class="mf-scale" x="${x.toFixed(1)}" y="${(y + 4).toFixed(1)}" text-anchor="middle">${fmt(top * k).replace(/\.0+$/, '')}</text>`; }).join('');
    const C = 2 * Math.PI * inner;
    const segs = shown.map(l => `<circle class="mf-seg" data-n="${esc(l.name)}" cx="${cx}" cy="${cy}" r="${inner}" stroke="${colorOf(l.name)}" stroke-width="5" stroke-dasharray="0 ${C}" transform="rotate(-90 ${cx} ${cy})"/>`).join('');
    const [nx1] = P(inner + 8, 0), [nx2] = P(R + arcW/2 + 4, 0);
    box.innerHTML = `<svg viewBox="0 0 ${W} ${H}" aria-hidden="true" focusable="false">
      ${nodes}
      <circle class="mf-glow" cx="${cx}" cy="${cy}" r="${R - arcW/2 - 2}"/>
      <path class="mf-track" d="${arc(R)}" stroke-width="${arcW}"/>
      <path class="mf-arc" data-f="arc" d="${arc(R)}" stroke-width="${arcW}" pathLength="100" stroke-dasharray="0 100"/>
      ${ticks}${labels}
      <circle class="mf-inner-track" cx="${cx}" cy="${cy}" r="${inner}" stroke-width="5"/>
      ${segs}
      <g class="mf-needle" data-f="needle" style="transform-origin:${cx}px ${cy}px;transform:rotate(135deg)"><line x1="${nx1.toFixed(1)}" y1="${cy}" x2="${nx2.toFixed(1)}" y2="${cy}"/><circle cx="${nx2.toFixed(1)}" cy="${cy}" r="4"/></g>
      <text class="mf-hub-label" x="${cx}" y="${cy - 26}" text-anchor="middle" data-f="hubLabel"></text>
      <text class="mf-hub-value" x="${cx}" y="${cy + 14}" text-anchor="middle" data-f="hubValue"></text>
      <text class="mf-hub-unit" x="${cx}" y="${cy + 34}" text-anchor="middle">Mbps down</text>
      <text class="mf-tier" x="${cx}" y="${cy + R * .78}" text-anchor="middle" data-f="tier"></text>
      <text class="mf-of" x="${cx}" y="${cy + R * .78 + 16}" text-anchor="middle">scale 0–${fmt(top).replace(/\.0+$/, '')} Mbps</text>
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
    const node = svg.querySelector(`.mf-node[data-n="${CSS.escape(l.name)}"]`);
    if (node) {
      const working = isWorking(l, run), moving = working && mbps(l.down) > 0.3;
      node.classList.toggle('idle', !working); node.classList.toggle('moving', moving);
      node.querySelector('.mf-ribbon').style.strokeWidth = (4 + Math.round(share * 22)) + 'px';
      f(node, 'state').textContent = statusOf(l, run)[0];
    }
  });
  svg.classList.toggle('off', !run);
  const p = run ? Math.max(0, Math.min(1, now / gaugeTop)) : 0, col = speedColor(p);
  f(svg, 'arc').setAttribute('stroke-dasharray', `${(p * 100).toFixed(2)} 100`);
  svg.style.setProperty('--speed', col);
  f(svg, 'needle').style.transform = `rotate(${(135 + 270 * p).toFixed(1)}deg)`;
  f(svg, 'hubLabel').textContent = run ? 'COMBINED' : 'ADAPTERS';
  f(svg, 'hubValue').textContent = fmtM(sn.totalDown);
  f(svg, 'tier').textContent = !run ? 'Merging off' : speedTier(p);
  const parts = shown.map(l => `${l.label || l.name} ${fmtM(l.down)}`).join(', ');
  box.setAttribute('aria-label', `${run ? 'Combined' : 'Adapter'} download speed ${fmtM(sn.totalDown)} Mbps of a ${fmt(gaugeTop).replace(/\.0+$/, '')} Mbps scale${run ? ', ' + speedTier(p).toLowerCase() : ''}${parts ? '. ' + parts + ' Mbps' : ''}.`);
}
