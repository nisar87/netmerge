/* ---------- updates ---------- */
let UP = null, updShownErr = '';
async function loadUpdate(){ try { UP = await api('update'); renderUpdate(); } catch(e){return false;} }
function updAgo(t){ if(!t) return 'never'; const m = Math.round((Date.now()/1000 - t)/60); return m < 1 ? 'just now' : m < 60 ? m+' min ago' : Math.round(m/60)+' h ago'; }
function renderUpdate(){
  if (!UP) return;
  const busy = UP.status === 'downloading' || UP.status === 'installing' || UP.status === 'checking';
  let b = '';
  if (UP.available || UP.status === 'downloading' || UP.status === 'installing'){
    const notes = (UP.notes||'').trim().split('\n').filter(Boolean).slice(0,6).map(l => '<li>'+esc(l.replace(/^[-*#\s]+/,''))+'</li>').join('');
    b = `<div class="note updb"><div style="flex:1;min-width:0"><b>NetMerge ${esc(UP.latest)} is available</b> <span class="hint2">(you have ${esc(UP.current)})</span>
      ${notes ? `<details id="updNotes" style="margin-top:6px"><summary id="updNotesSummary" class="hint2" style="cursor:pointer">What's new</summary><ul class="tips" style="margin-top:6px">${notes}</ul></details>` : ''}
      ${UP.status === 'downloading' ? `<div class="share-bar" style="margin-top:8px"><div style="width:${UP.progress}%"></div></div>` : ''}</div>
      <button class="btn primary" id="updGo" ${busy?'disabled':''}>${UP.status==='downloading' ? 'Downloading '+UP.progress+'%' : UP.status==='installing' ? 'Restarting…' : 'Update now'}</button></div>`;
  }
  setHTML($('#updBanner'), b);
  const g = $('#updGo'); if (g) g.onclick = async () => { try { UP = await api('update',{action:'install'}); renderUpdate(); toast('Downloading the update…'); } catch(e){ toast(e.message, true); } };
  $('#appVer').textContent = 'v'+UP.current;
  $('#updInfo').textContent = UP.status === 'checking' ? 'Checking…' : UP.error ? UP.error : UP.available ? `Version ${UP.latest} is available.` : UP.checked ? `You have the latest version. Checked ${updAgo(UP.checked)}.` : 'Not checked yet.';
  $('#updCheck').disabled = busy;
  $('#updTog').classList.toggle('on', UP.auto); $('#updTog').setAttribute('aria-checked', UP.auto);
}
$('#updCheck').onclick = async () => { try { UP = await api('update',{action:'check'}); renderUpdate(); if (!UP.error) toast(UP.available ? 'Update found' : 'You are up to date'); } catch(e){ toast(e.message, true); } };
$('#updTog').onclick = () => { local.noUpdateCheck = !local.noUpdateCheck; if (UP) { UP.auto = !local.noUpdateCheck; renderUpdate(); } saveSoon(local.noUpdateCheck ? 'Automatic update checks off' : 'Automatic update checks on'); };
NM.schedule('update',loadUpdate,4000,()=>!document.hidden); setTimeout(loadUpdate, 800);

