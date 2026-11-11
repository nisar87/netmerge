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

/* The merge picture: each connection flows into one ring. The ring is split by each line's share of the download right now. */
function renderMergeFlow(sn, run){
  const box = $('#mergeFlow'); if (!box) return;
  const shown = (sn.links || []).filter(l => l.present && l.enabled);
  const compact = box.clientWidth > 0 && box.clientWidth < 520;
  const key = (compact ? 'c|' : 'w|') + shown.map(l => l.name).join('|');
  const n = shown.length;
  const W = compact ? 280 : 720, rowH = 74, H = compact ? 280 : Math.max(280, n * rowH + 40);
  const cx = compact ? 140 : 560, cy = H/2, R = compact ? 104 : 106, ring = 14;
  if (box.dataset.key !== key) {
    box.dataset.key = key;
    const y0 = cy - (n - 1) * rowH / 2;
    const nodes = compact ? '' : shown.map((l, i) => {
      const y = y0 + i * rowH;
      return `<g class="mf-node" data-n="${esc(l.name)}" style="--c:${colorOf(l.name)}">
        <path class="mf-ribbon" d="M196 ${y} C 340 ${y}, 370 ${cy}, ${cx - R - 4} ${cy}"/>
        <path class="mf-flow" d="M196 ${y} C 340 ${y}, 370 ${cy}, ${cx - R - 4} ${cy}"/>
        <rect class="mf-card" x="0" y="${y - 28}" width="196" height="56" rx="12"/>
        <rect class="mf-icon" x="10" y="${y - 18}" width="36" height="36" rx="9"/>
        ${svgIcon(l.kind, 18, y - 10, 20)}
        <text class="mf-name" x="56" y="${y - 4}">${esc(shortLabel(l.label || l.name, 17))}</text>
        <text class="mf-state" x="56" y="${y + 14}" data-f="state"></text>
      </g>`;
    }).join('');
    const C = 2 * Math.PI * R;
    const segs = shown.map(l => `<circle class="mf-seg" data-n="${esc(l.name)}" cx="${cx}" cy="${cy}" r="${R}" stroke="${colorOf(l.name)}" stroke-width="${ring}" stroke-dasharray="0 ${C}" transform="rotate(-90 ${cx} ${cy})"/>`).join('');
    box.innerHTML = `<svg viewBox="0 0 ${W} ${H}" aria-hidden="true" focusable="false">
      ${nodes}
      <circle class="mf-glow" cx="${cx}" cy="${cy}" r="${R + 18}"/>
      <circle class="mf-track" cx="${cx}" cy="${cy}" r="${R}" stroke-width="${ring}"/>
      ${segs}
      <circle class="mf-core" cx="${cx}" cy="${cy}" r="${R - ring/2 - 8}"/>
      <text class="mf-hub-label" x="${cx}" y="${cy - 30}" text-anchor="middle" data-f="hubLabel"></text>
      <text class="mf-hub-value" x="${cx}" y="${cy + 14}" text-anchor="middle" data-f="hubValue"></text>
      <text class="mf-hub-unit" x="${cx}" y="${cy + 36}" text-anchor="middle">Mbps down</text>
    </svg>`;
  }
  const svg = box.querySelector('svg'); if (!svg) return;
  const f = (root, k) => root.querySelector(`[data-f="${k}"]`);
  const total = shown.reduce((a, l) => a + (Number.isFinite(l.down) ? l.down : 0), 0);
  const C = 2 * Math.PI * R, gap = shown.length > 1 ? 3 : 0;
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
  f(svg, 'hubLabel').textContent = run ? 'COMBINED' : 'ADAPTERS';
  f(svg, 'hubValue').textContent = fmtM(sn.totalDown);
  const parts = shown.map(l => `${l.label || l.name} ${fmtM(l.down)}`).join(', ');
  box.setAttribute('aria-label', `${run ? 'Combined' : 'Adapter'} download speed ${fmtM(sn.totalDown)} Mbps${parts ? '. ' + parts + ' Mbps' : ''}.`);
}
