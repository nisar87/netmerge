/* ---------- usage ---------- */
let U = null;
async function loadUsage(){try{U=await api('usage');NM.report('usage');renderUsage();}catch(error){if(page==='usage')NM.report('usage',error,loadUsage);}}
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
  // ----- cards (this allowance period)
  const F = mkFmt(shown.reduce((a,l)=>a+l.used,0));
  let tD = 0, tU = 0;
  const cards = shown.map(l => {
    const d = F.q(l.rx), u = F.q(l.tx); tD += d; tU += u;
    const pct = l.quotaGB > 0 ? Math.min(100, l.used/(l.quotaGB*1e9)*100) : 0;
    return `<div class="us${l.out?' out':''}" style="--c:${P[L.indexOf(l)%5]}"><div class="n">${esc(l.label)}</div><div class="v">${d+u === 0 ? F.zero : F.show(d+u)}</div>
      <div class="s">↓ ${F.show(d)} &nbsp;·&nbsp; ↑ ${F.show(u)}</div>
      ${l.quotaGB > 0 ? `<div class="share-bar" style="margin-top:8px"><div style="width:${pct}%"></div></div><div class="s">${pct.toFixed(0)}% of ${l.quotaGB} GB${l.out ? ' · left out of the merge' : ''}</div>` : '<div class="s">No limit set</div>'}${usForecast(l)}</div>`;
  }).join('');
  const dayTot = d => shown.reduce((a,l) => { const v = d.links?.[l.name] || [0,0]; return a + v[0] + v[1]; }, 0);
  const wk = U.days.slice(0,7).map(dayTot), pw = U.days.slice(7,14).map(dayTot);
  const wkSum = wk.reduce((a,b)=>a+b,0), pwSum = pw.reduce((a,b)=>a+b,0), top = Math.max(...wk, 0);
  const topDay = top > 0 ? U.days[wk.indexOf(top)].date : '';
  const wkCard = wkSum > 0 ? `<div class="us"><div class="n">Last 7 days</div><div class="v">${F.show(F.q(wkSum))}</div><div class="s">about ${F.show(F.q(wkSum/7))} a day</div><div class="s">Busiest: ${esc(topDay)} (${F.show(F.q(top))})</div>${pwSum > 0 ? `<div class="s">${wkSum >= pwSum ? 'Up' : 'Down'} ${Math.abs(Math.round((wkSum/pwSum-1)*100))}% on the week before</div>` : ''}</div>` : '';
  setHTML($('#usSum'), cards + wkCard + `<div class="us tot"><div class="n">Total · ${shown.map(l=>esc(l.label)).join(' + ')}</div><div class="v">${tD+tU === 0 ? F.zero : F.show(tD+tU)}</div><div class="s">↓ ${F.show(tD)} &nbsp;·&nbsp; ↑ ${F.show(tU)}</div><div class="s">${esc(U.start)} to ${esc(U.end)} · renews on day ${U.reset}</div></div>`);
  $('#usPeriod').textContent = '· last 31 days, newest first';
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
  { const fl = $('#fxLink'), fd = $('#fxDay'), k = local.links.map(l=>l.name).join('|') + '#' + U.days[0].date;
    if (fl.dataset.keys !== k){
      fl.dataset.keys = k;
      fl.innerHTML = local.links.map(l => `<option value="${esc(l.name)}">${esc(l.label)}</option>`).join('');
      fd.innerHTML = U.days.map(d => `<option value="${esc(d.date)}">${esc(d.date)}</option>`).join('');
      const fill = () => { const d = U.days.find(x => x.date === fd.value), v = (d && d.links && d.links[fl.value]) || [0,0]; $('#fxDown').value = Math.round(v[0]/1e6); $('#fxUp').value = Math.round(v[1]/1e6); };
      fl.onchange = fd.onchange = fill; fill();
      $('#fxSave').onclick = async () => {
        try { await api('usage', {action:'set', link: fl.value, day: fd.value, downMB: +$('#fxDown').value || 0, upMB: +$('#fxUp').value || 0}); toast('Number corrected'); loadUsage(); }
        catch(e){ toast(e.message, true); }
      };
    }
  }
  if (document.activeElement !== $('#qReset')) $('#qReset').value = local.quotaReset || 1;
  if (document.activeElement !== $('#qAct')) $('#qAct').value = local.quotaAction || 'warn';
}
NM.schedule('usage',()=>loadUsage(), 5000, ()=>page==='usage'&&!document.hidden);
