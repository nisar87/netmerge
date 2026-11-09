/* ---------- speed test ---------- */
let speedTestPending=false;
function renderTest(){
  const t = S.test, run = t.running;
  $('#testBtn').disabled=run||speedTestPending;$('#testBtn').textContent=run?'Testing…':speedTestPending?'Starting…':t.phase==='Done'?'Test again':'Start test';
  $('#testProg').hidden = !run; $('#testProg').firstElementChild.style.width = ((t.progress||0)*100)+'%';
  $('#testPhase').textContent = run ? `${t.phase}…  ${t.live ? t.live.toFixed(1)+' Mbps' : ''}` : '';
  const rows = (t.results||[]).map(r => ({t:r.label, s:(r.error||('public '+(r.ip||'?')+' · 1 stream '+r.single+' Mbps')), v:r.mbps, c:colorOf(r.name)}));
  if (t.phase==='Done'){
    if (t.combined) rows.push({t:'All together', s:'the most your lines can do at once', v:t.combined, c:'var(--text)'});
    (t.scaling||[]).forEach(p => rows.push({t:'Through NetMerge', s:p.streams+' parallel streams', v:p.mbps, c:'var(--accent)'}));
    if (!(t.scaling||[]).length) rows.push({t:'Through NetMerge', s:'merging was off', v:0, c:'var(--accent)'});
  }
  const max = Math.max(1, ...rows.map(r=>r.v));
  setHTML($('#testBars'), rows.map(r => `<div class="bar-r"><div class="t">${esc(r.t)}<small>${esc(r.s)}</small></div><div class="track"><div class="fill" style="width:${r.v/max*100}%;--c:${r.c}"></div></div><div class="v">${r.v ? r.v.toFixed(1) : '—'} <small style="color:var(--mute);font-size:12px">Mbps</small></div></div>`).join(''));
  let notes = '';
  if (t.warning) notes += `<div class="note warn">${esc(t.warning)}</div>`;
  if (t.phase==='Done' && t.suggest && Object.keys(t.suggest).length > 1){
    const cur = local.links.filter(l => t.suggest[l.name]).every(l => l.weight === t.suggest[l.name]);
    const txt = local.links.filter(l => t.suggest[l.name]).map(l => `${esc(l.label)} ×${t.suggest[l.name]}`).join(', ');
    notes += cur ? `<div class="note info">Shares already match your connection speeds (${txt}).</div>`
      : `<div class="note info"><span>Suggested shares from this test: <b>${txt}</b></span><span class="spacer"></span><button class="btn pri" id="applyW">Apply</button></div>`;
  }
  if (t.phase==='Done' && t.merged && t.combined && t.merged < t.combined*0.6 && S.snap.running)
    notes += `<div class="note warn">NetMerge reached ${t.merged.toFixed(1)} of a possible ${t.combined.toFixed(1)} Mbps. Test servers sometimes limit repeated tests — a real download is the best check.</div>`;
  const nt = $('#testNotes');
  if (nt.dataset.h !== notes){ nt.dataset.h = notes; nt.innerHTML = notes; const b=$('#applyW'); if (b) b.onclick = async () => { try { await api('applyweights',{}); await poll(true); toast('Shares updated'); } catch(e){ toast(e.message,true); } }; }
}
$('#testBtn').onclick=async()=>{if(speedTestPending||S?.test.running)return;speedTestPending=true;renderTest();try{await api('speedtest',{});await poll(true);}catch(e){toast(e.message,true);}finally{speedTestPending=false;if(S)renderTest();}};
