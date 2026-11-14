/* ---------- blocking ---------- */
let B = null, blockBusy = false;
let blockRead=null;
const blockRevisions={blocklist:0,allowlist:0};
function renderBlockBadge(){
  const b = $('#blkBadge'), n = S.block?.enabled ? S.block.today : 0;
  b.hidden = !n; b.textContent = n > 999 ? (n/1000).toFixed(1)+'k' : n;
}
async function loadBlock(){
  if(blockBusy||blockRead)return;
  const read=blockRead=new AbortController();
  try{const next=await NM.request('block',{signal:read.signal});if(blockRead!==read)return;B=next;NM.report('blocking');renderBlock();}
  catch(error){if(read.signal.aborted)return;if(page==='block')NM.report('blocking',error,loadBlock);return false;}
  finally{if(blockRead===read)blockRead=null;}
}
function blockControls(){
  for(const button of $$('#blkTog,#blkUpdate,#blkSaveMine,#blkSaveAllow,#blkPacks button,#blkRecent button'))button.disabled=blockBusy;
}
const ago = t => agoText(t);
function renderBlock(){
  if (!B || !S) return;
  const on = B.enabled, run = S.snap.running;
  $('#blkTog').classList.toggle('on', on); $('#blkTog').setAttribute('aria-checked', on);
  $('#blkToday').textContent = (S.block?.today ?? B.today).toLocaleString(); $('#blkTotal').textContent = (S.block?.total ?? B.total).toLocaleString();
  setHTML($('#blkHint'), !on ? '<div class="note warn" style="margin:10px 0 0">Blocking is off.</div>' : !run ? '<div class="note info" style="margin:10px 0 0">Blocking works while merging is on. Turn merging on to start blocking.</div>' : '');
  $('.blk-hero .hero-icon')?.classList.toggle('good', on && run);
  setHTML($('#blkPacks'), B.packs.map(p => {
    const meta = p.builtin ? `${p.count.toLocaleString()} sites` : p.updating ? 'Updating…' : p.error ? `<span style="color:var(--bad)" title="${esc(p.error)}">Update failed</span>` : p.updated ? `${p.count.toLocaleString()} sites · ${ago(p.updated)}` : (p.enabled ? 'Downloading…' : '');
    return `<div class="pack"><div class="t"><b>${esc(p.name)}</b><span>${esc(p.desc)}</span></div><div class="m">${meta}</div>${p.builtin ? '' : `<button class="tog${p.enabled?' on':''}" data-pack="${esc(p.id)}" role="switch" aria-checked="${p.enabled}" aria-label="${esc(p.name)}"></button>`}</div>`;
  }).join(''));
  $('#blkPacks').querySelectorAll('[data-pack]').forEach(b => b.onclick = () => blockPost({packs:{[b.dataset.pack]: !B.packs.find(p=>p.id===b.dataset.pack).enabled}}, 'Saved'));
  // one list: the count column carries the bar that shows how often each website was refused
  const rec = (B.recent || []).slice(0, 20);
  const recMax = Math.max(1, ...rec.map(r => r.count));
  setHTML($('#blkRecent'), rec.length ? '<tr><th>Website</th><th>App</th><th class="r">Times</th><th>Last</th><th></th></tr>' + rec.map(r => `<tr><td class="proc"><span class="blk-host" title="${esc(r.host)}">${esc(r.host)}</span></td><td>${esc(r.app || '')}</td><td class="r num"><span class="blk-count">${r.count.toLocaleString()}<span class="hb-track" aria-hidden="true"><i style="width:${(r.count/recMax*100).toFixed(1)}%"></i></span></span></td><td style="color:var(--dim);white-space:nowrap">${ago(r.time)}</td><td class="r"><button class="btn sm" data-allow="${esc(r.host)}">Allow</button></td></tr>`).join('') : '<tr><td class="empty" colspan="5">No blocked connections yet.</td></tr>');
  $('#blkRecent').querySelectorAll('[data-allow]').forEach(b => b.onclick = () => blockPost({domain:b.dataset.allow},b.dataset.allow+' is now allowed','block/allow'));
  fillLists(false);
  blockControls();
}
function fillLists(force){
  for (const [id, key] of [['#blkMine','blocklist'],['#blkAllow','allowlist']]) {
    const el = $(id); if (force || (document.activeElement !== el && el.dataset.dirty !== '1')) { if (el.value !== B[key]) el.value = B[key]; }
  }
}
$('#blkMine').oninput = e => {e.target.dataset.dirty='1';blockRevisions.blocklist++;};
$('#blkAllow').oninput = e => {e.target.dataset.dirty='1';blockRevisions.allowlist++;};
async function blockPost(body,msg,path='block'){
  if(blockBusy)return false;
  blockBusy=true;blockRead?.abort();blockRead=null;blockControls();
  try{const next=await api(path,body);if(path!=='block/update')B=next;toast(msg);renderBlock();poll(true);return true;}
  catch(e){toast(e.message,true);return false;}
  finally{blockBusy=false;blockControls();}
}
async function saveBlockList(id,key,msg){
  const input=$('#'+id),revision=blockRevisions[key],value=input.value;
  if(await blockPost({[key]:value},msg)){
    if(blockRevisions[key]===revision)input.dataset.dirty='';
    fillLists(false);
  }
}
$('#blkTog').onclick = () => B && blockPost({enabled: !B.enabled}, B.enabled ? 'Blocking is off' : 'Blocking is on');
$('#blkUpdate').onclick=async()=>{if(await blockPost({},'Updating lists…','block/update'))setTimeout(loadBlock,1500);};
$('#blkSaveMine').onclick=()=>saveBlockList('blkMine','blocklist','Block list saved');
$('#blkSaveAllow').onclick=()=>saveBlockList('blkAllow','allowlist','Allow list saved');
$('#alerts').addEventListener('click', async e => {
  if (e.target.id !== 'fixConf') return;
  e.target.disabled = true; e.target.textContent = 'Working…';
  try { await api('fixconflict', {}); toast('Closed. You can turn merging on now.'); } catch(err){ toast(err.message, true); }
  poll(true);
});
NM.schedule('block',()=>loadBlock(), 3000, ()=>page==='block'&&!document.hidden);
