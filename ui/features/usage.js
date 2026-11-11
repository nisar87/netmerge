/* ---------- usage ---------- */
let U = null;
async function loadUsage(){try{const next=await api('usage');U={...next,links:next.links||[],days:next.days||[]};NM.report('usage');renderUsage();}catch(error){if(page==='usage')NM.report('usage',error,loadUsage);}}
// One unit for everything on screen, and totals are added up from the values that are SHOWN, so the numbers
// always add up exactly (LAN + Wi-Fi + … = Total, download + upload = connection total).
function mkFmt(maxBytes){
  const G = maxBytes >= 1e9, div = G ? 1e7 : 1e6;
  return { q: b => Math.round((b||0)/div), show: n => n === 0 ? '—' : G ? (n/100).toFixed(2)+' GB' : n.toLocaleString()+' MB', zero: G ? '0.00 GB' : '0 MB',
           cell: n => n === 0 ? '—' : G ? (n/100).toFixed(2) : n.toLocaleString(), unit: G ? 'GB' : 'MB' };
}
function usForecast(l){
  const DAY = 864e5, s = new Date(U.start+'T00:00:00'), e = new Date(U.end+'T00:00:00'), now = new Date();
  const el = (now - s)/DAY, tot = (e - s)/DAY;
  if (el < 0.5 || l.used <= 0) return '';
  const rate = l.used/el, proj = rate*tot, q = l.quotaGB*1e9;
  if (q > 0 && l.used < q && proj > q){
    const d = new Date(s.getTime() + q/rate*DAY);
    return `<div class="s fc bad">At this pace it reaches the limit around ${d.toLocaleDateString(undefined,{day:'numeric',month:'short'})}</div>`;
  }
  const g = proj >= 1e9 ? (proj/1e9).toFixed(1)+' GB' : Math.round(proj/1e6)+' MB';
  return `<div class="s fc">${q > 0 && l.used < q ? 'On track: ' : ''}about ${g} by renewal at this pace</div>`;
}
function renderUsage(){
  if (!U || !local) return;
  const P = pal(), L = U.links, shown = L.filter(l => l.enabled || l.used > 0);
  // ----- cards (this allowance period): a ring per connection, then the period total
  const F = mkFmt(shown.reduce((a,l)=>a+l.used,0));
  let tD = 0, tU = 0;
  const allUsed = shown.reduce((a,l)=>a+l.used,0);
  const ring = (pct, col, inner) => { const C = 2*Math.PI*30, v = Math.max(0, Math.min(100, pct)); return `<svg class="us-ring" viewBox="0 0 72 72" aria-hidden="true"><circle cx="36" cy="36" r="30" class="us-ring-track"/><circle cx="36" cy="36" r="30" stroke="${col}" stroke-dasharray="${(v/100*C).toFixed(1)} ${C.toFixed(1)}" transform="rotate(-90 36 36)" class="us-ring-val"/><text x="36" y="40" text-anchor="middle">${inner}</text></svg>`; };
  const cards = shown.map(l => {
    const d = F.q(l.rx), u = F.q(l.tx); tD += d; tU += u;
    const col = P[L.indexOf(l)%5], quota = l.quotaGB > 0, pct = quota ? Math.min(100, l.used/(l.quotaGB*1e9)*100) : allUsed > 0 ? l.used/allUsed*100 : 0;
    const tone = quota && pct >= 100 ? 'var(--error)' : quota && pct >= 80 ? 'var(--warning)' : col;
    return `<div class="us${l.out?' out':''}" style="--c:${col}"><div class="us-top">${ring(pct, tone, Math.round(pct)+'%')}<div class="us-main"><div class="n">${esc(l.label)}</div><div class="v">${d+u === 0 ? F.zero : F.show(d+u)}</div><div class="s us-du"><span>↓ ${F.show(d)}</span><span>↑ ${F.show(u)}</span></div></div></div>
      <div class="s us-cap">${quota ? `${pct.toFixed(0)}% of ${l.quotaGB} GB allowance${l.out ? ' · left out of the merge' : ''}` : `${Math.round(pct)}% of all data · no limit set`}</div>${usForecast(l)}</div>`;
  }).join('');
  const dayTot = d => shown.reduce((a,l) => { const v = d.links?.[l.name] || [0,0]; return a + v[0] + v[1]; }, 0);
  const wk = U.days.slice(0,7).map(dayTot), pw = U.days.slice(7,14).map(dayTot);
  const wkSum = wk.reduce((a,b)=>a+b,0), pwSum = pw.reduce((a,b)=>a+b,0), top = Math.max(...wk, 0);
  const topDay = top > 0 ? U.days[wk.indexOf(top)].date : '';
  const spark = wk.slice().reverse(), sMax = Math.max(...spark, 1);
  const wkBars = spark.map((v,i) => `<rect x="${i*14}" y="${(36 - v/sMax*34).toFixed(1)}" width="9" height="${Math.max(1.5, v/sMax*34).toFixed(1)}" rx="2"/>`).join('');
  const wkCard = wkSum > 0 ? `<div class="us us-week"><div class="n">Last 7 days</div><div class="v">${F.show(F.q(wkSum))}</div><svg class="us-wk" viewBox="0 0 94 36" preserveAspectRatio="none" aria-hidden="true">${wkBars}</svg><div class="s">about ${F.show(F.q(wkSum/7))} a day · busiest ${esc(topDay)}</div>${pwSum > 0 ? `<div class="s us-trend ${wkSum >= pwSum ? 'up' : 'down'}">${wkSum >= pwSum ? '▲ Up' : '▼ Down'} ${Math.abs(Math.round((wkSum/pwSum-1)*100))}% on the week before</div>` : ''}</div>` : '';
  const split = shown.map(l => `<i style="width:${allUsed > 0 ? (l.used/allUsed*100).toFixed(2) : 0}%;background:${P[L.indexOf(l)%5]}" title="${esc(l.label)}"></i>`).join('');
  const totCard = `<div class="us tot"><div class="n">This period · all connections</div><div class="v">${tD+tU === 0 ? F.zero : F.show(tD+tU)}</div><div class="us-split" aria-hidden="true">${split}</div><div class="s us-du"><span>↓ ${F.show(tD)}</span><span>↑ ${F.show(tU)}</span></div><div class="s">${esc(U.start)} to ${esc(U.end)} · renews on day ${U.reset}</div></div>`;
  setHTML($('#usSum'), totCard + cards + wkCard);
  $('#usPeriod').textContent = '· last 31 days';
  // ----- daily chart: one stacked bar per day, oldest on the left
  { const days = U.days.slice(0, 31).slice().reverse(), W = 760, H = 180, n = Math.max(days.length, 1), bw = W / 31;
    const dmax = Math.max(1, ...days.map(dayTot)), top = niceMax(dmax/1e9*1.1), Y = gb => H - gb/top*(H-6);
    const today = new Date(Date.now()-new Date().getTimezoneOffset()*60000).toISOString().slice(0,10);
    const bars = days.map((d,i) => { let y = H; const x = (31 - n + i)*bw + bw*.18, w = bw*.64;
      const segs = shown.map(l => { const v = d.links?.[l.name] || [0,0], gb = (v[0]+v[1])/1e9; if (gb <= 0) return ''; const h = H - Y(gb); y -= h; return `<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${w.toFixed(1)}" height="${Math.max(h, .8).toFixed(1)}" fill="${P[L.indexOf(l)%5]}"/>`; }).join('');
      return `<g class="us-bar${d.date===today?' today':''}"><title>${esc(d.date===today?'Today':d.date)}: ${(dayTot(d)/1e9).toFixed(2)} GB</title><rect class="us-hit" x="${((31-n+i)*bw).toFixed(1)}" y="0" width="${bw.toFixed(1)}" height="${H}"/>${segs}</g>`; }).join('');
    const grid = [0,.5,1].map(k => `<line x1="0" x2="${W}" y1="${Y(top*k).toFixed(1)}" y2="${Y(top*k).toFixed(1)}"/>`).join('');
    const yl = [1,.5,0].map(k => `<span style="top:${(Y(top*k)/H*100).toFixed(1)}%">${fmt(top*k)} GB</span>`).join('');
    const first = days[0]?.date || '', last = days.at(-1)?.date || '';
    setHTML($('#usChart'), days.length ? `<div class="us-plot"><svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" role="img" aria-label="Data used per day, last ${days.length} days. The table below has every value."><g class="hist-grid">${grid}</g>${bars}</svg><div class="hist-y" aria-hidden="true">${yl}</div></div><div class="hist-x" aria-hidden="true"><span>${esc(first)}</span><span>${esc(last===today?'Today':last)}</span></div>` : '<div class="hist-empty">No days recorded yet.</div>');
    setHTML($('#usLegend'), shown.map(l => `<div><i style="background:${P[L.indexOf(l)%5]}"></i>${esc(l.label)}</div>`).join(''));
  }
  // ----- daily table: download / upload for every connection, then the total
  let maxDay = 0;
  U.days.forEach(d => { let t = 0; shown.forEach(l => { const v = d.links?.[l.name] || [0,0]; t += v[0]+v[1]; }); maxDay = Math.max(maxDay, t); });
  const T = mkFmt(Math.max(maxDay, 1e9));   // table always in GB
  const grp = shown.map((l,i) => `<th class="num grp" colspan="2" style="--c:${P[L.indexOf(l)%5]}">${esc(l.label)}</th>`).join('');
  const head = `<tr><th rowspan="2">Date</th>${grp}<th class="num grp tot" colspan="3">Total</th></tr>
    <tr class="sub">${shown.map(()=>'<th class="num">↓ Down</th><th class="num">↑ Up</th>').join('')}<th class="num tot">↓ Down</th><th class="num">↑ Up</th><th class="num">All</th></tr>`;
  const loc = new Date(Date.now()-new Date().getTimezoneOffset()*60000).toISOString().slice(0,10);
  const rows = U.days.map(d => {
    let sd = 0, su = 0;
    const cells = shown.map(l => { const v = d.links?.[l.name] || [0,0], a = T.q(v[0]), b = T.q(v[1]); sd += a; su += b; return `<td class="num">${T.cell(a)}</td><td class="num">${T.cell(b)}</td>`; }).join('');
    return `<tr class="${d.date===loc?'today':''}"><td>${d.date===loc?'Today':esc(d.date)}</td>${cells}<td class="num tot">${T.cell(sd)}</td><td class="num">${T.cell(su)}</td><td class="num"><b>${T.cell(sd+su)}</b></td></tr>`;
  }).join('');
  setHTML($('#usTbl'), head + rows);
  $('#usUnit').textContent = '· all values in GB';
  // allowance controls: rebuilt only when the set of connections changes, so typing is never interrupted
  const q = $('#usQuota'), keys = local.links.map(l=>l.name).join('|');
  if (q.dataset.keys !== keys){
    q.dataset.keys = keys;
    q.innerHTML = local.links.map(l => `<div class="qrow"><b>${esc(l.label)}</b><input class="inp" type="number" min="0" step="1" inputmode="numeric" placeholder="No limit" data-n="${esc(l.name)}" aria-label="Monthly allowance for ${esc(l.label)} in GB"> <span style="color:var(--mute)">GB per month</span></div>`).join('') +
      `<div class="qrow"><b>Renews on day</b><select class="sel" id="qReset" aria-label="Allowance renewal day" style="min-width:90px">${Array.from({length:28},(_,i)=>`<option>${i+1}</option>`).join('')}</select>
       <b style="min-width:0;margin-left:12px">When used up</b><select class="sel" id="qAct" aria-label="When allowance is used up"><option value="warn">Only warn me</option><option value="leave">Leave it out of the merge</option></select></div>`;
    q.querySelectorAll('input[data-n]').forEach(inp => inp.onchange = () => {
      const l = local.links.find(x => x.name === inp.dataset.n); l.quotaGB = Math.max(0, parseFloat(inp.value) || 0);
      saveSoon('Allowance saved'); setTimeout(loadUsage, 900);
    });
    $('#qReset').onchange = e => { local.quotaReset = +e.target.value; saveSoon('Saved'); setTimeout(loadUsage, 900); };
    $('#qAct').onchange = e => { local.quotaAction = e.target.value; saveSoon('Saved'); setTimeout(loadUsage, 900); };
  }
  q.querySelectorAll('input[data-n]').forEach(inp => { if (document.activeElement !== inp){ const l = local.links.find(x => x.name === inp.dataset.n); inp.value = l && l.quotaGB > 0 ? l.quotaGB : ''; } });
  // correction tool: the lists are rebuilt only when the connections or days change, so nothing a person is typing is lost
  { const fl = $('#fxLink'), fd = $('#fxDay'), k = local.links.map(l=>l.name).join('|') + '#' + U.days.map(d=>d.date).join('|');
    if (fl.dataset.keys !== k){
      fl.dataset.keys = k;
      fl.innerHTML = local.links.map(l => `<option value="${esc(l.name)}">${esc(l.label)}</option>`).join('');
      fd.innerHTML = U.days.map(d => `<option value="${esc(d.date)}">${esc(d.date)}</option>`).join('');
      const fill = () => { const d = U.days.find(x => x.date === fd.value), v = (d && d.links && d.links[fl.value]) || [0,0]; $('#fxDown').value = Math.round(v[0]/1e6); $('#fxUp').value = Math.round(v[1]/1e6); $('#fxSave').disabled = !fl.value || !fd.value; };
      fl.onchange = fd.onchange = fill; fill();
      $('#fxSave').onclick = async () => {
        if (!fl.value || !fd.value || !$('#fxDown').reportValidity() || !$('#fxUp').reportValidity()) return;
        try { await api('usage', {action:'set', link: fl.value, day: fd.value, downMB: +$('#fxDown').value || 0, upMB: +$('#fxUp').value || 0}); toast('Number corrected'); loadUsage(); }
        catch(e){ toast(e.message, true); }
      };
    }
  }
  if (document.activeElement !== $('#qReset')) $('#qReset').value = local.quotaReset || 1;
  if (document.activeElement !== $('#qAct')) $('#qAct').value = local.quotaAction || 'warn';
}
NM.schedule('usage',()=>loadUsage(), 5000, ()=>page==='usage'&&!document.hidden);
