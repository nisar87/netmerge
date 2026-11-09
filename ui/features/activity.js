/* ---------- history ---------- */
let HI = null, hiFilter = 'all';
async function loadHistoryEvents(){ try { HI = (await api('events')).events || []; } catch(e){if(page==='history')NM.report('activity',e,loadHistoryEvents);return;}NM.report('activity');paintHistoryEvents(); }
function paintHistoryEvents(){
  const box = $('#hiBox'); if (!box || !HI) return;
  const ev = HI.filter(e => hiFilter === 'all' || (hiFilter === 'problems' ? (e.kind === 'warn' || e.kind === 'bad') : (e.kind === 'info' || e.kind === 'good')));
  if (!ev.length) { setHTML(box, '<div class="hint2">Nothing here yet. Connection changes, outages, presets and starts and stops show up as they happen.</div>'); return; }
  let day = '', html = '';
  const today = new Date().toDateString(), yest = new Date(Date.now()-864e5).toDateString();
  ev.forEach(e => {
    const d = new Date(e.time*1000), ds = d.toDateString();
    if (ds !== day) { day = ds; html += `<div class="hi-day">${ds === today ? 'Today' : ds === yest ? 'Yesterday' : d.toLocaleDateString(undefined, {weekday:'long', day:'numeric', month:'long'})}</div>`; }
    html += `<div class="hi-row"><time>${d.toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'})}</time><i class="${esc(e.kind)}"></i><span>${esc(e.text)}</span></div>`;
  });
  setHTML(box, html);
}
$$('#hiSel button').forEach(b => b.onclick = () => { hiFilter = b.dataset.v; $$('#hiSel button').forEach(x=>x.classList.toggle('sel', x===b)); paintHistoryEvents(); });
$('#hiRefresh').onclick = loadHistoryEvents;
$('#seeAll').onclick = () => go('history');
NM.schedule('history',()=>loadHistoryEvents(), 5000, ()=>page==='history'&&!document.hidden);
