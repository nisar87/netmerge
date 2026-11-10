// Download manager: the extra dialogs (add with options, page link grabber, properties, queues, folders, import/export).
// Loaded together with downloads.js, only when the manager is on.

const DL_TYPES = {'All files':[], 'Videos':['mp4','mkv','avi','mov','webm','m4v','wmv','flv','ts'], 'Music':['mp3','flac','wav','aac','ogg','m4a','opus'], 'Archives':['zip','rar','7z','tar','gz','tgz','bz2','xz'], 'Programs':['exe','msi','dmg','iso','apk','deb','rpm'], 'Documents':['pdf','doc','docx','xls','xlsx','ppt','pptx','txt','epub','csv'], 'Images':['jpg','jpeg','png','gif','webp','bmp','svg','tif'], 'Torrents':['torrent','magnet']};
const DL_DAYS = ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'];
let dlSort = 'default', dlQf = '';
const dlQueueName = id => { const q = ((DL && DL.config.queues) || []).find(x => x.id === id); return q ? q.name : 'Main queue'; };
const dlQueueOpts = (cur, withMain = true) => (withMain ? [`<option value=""${!cur ? ' selected' : ''}>Main queue</option>`] : []).concat(((DL && DL.config.queues) || []).map(q => `<option value="${esc(q.id)}"${q.id === cur ? ' selected' : ''}>${esc(q.name)}</option>`)).join('');
const dlRowsOf = text => text.split(/\s*\n\s*/).map(s => s.trim()).filter(Boolean);

function dlSortSelect(){
  return `<select class="sel" id="dlSort" style="min-width:0" aria-label="Sort"><option value="default"${dlSort==='default'?' selected':''}>Sort: active first</option>${[['name','Name'],['size','Size (largest first)'],['added','Newest first'],['speed','Speed']].map(o => `<option value="${o[0]}"${dlSort===o[0]?' selected':''}>Sort: ${o[1]}</option>`).join('')}</select>`
    + ((DL.config.queues||[]).length ? `<select class="sel" id="dlQf" style="min-width:0" aria-label="Queue"><option value="">All queues</option><option value="main"${dlQf==='main'?' selected':''}>Main queue</option>${DL.config.queues.map(q => `<option value="${esc(q.id)}"${dlQf===q.id?' selected':''}>${esc(q.name)}</option>`).join('')}</select>` : '');
}

function dlSortItems(vis){
  const order = {downloading:0, queued:1, paused:2, error:3, done:4};
  const by = {
    default: (a,b) => (order[a.state] - order[b.state]) || ((b.finished||0) - (a.finished||0)) || a.order - b.order,
    name: (a,b) => (a.name||'').localeCompare(b.name||''),
    size: (a,b) => (b.size||0) - (a.size||0),
    added: (a,b) => (b.added||0) - (a.added||0),
    speed: (a,b) => (b.speed||0) - (a.speed||0),
  };
  return vis.sort(by[dlSort] || by.default);
}
function dlQueueMatch(v){ return !dlQf || (dlQf === 'main' ? !v.queue : v.queue === dlQf); }

function dlSegBar(v){
  if (!v.map || !v.map.length || v.state === 'done') return '';
  return `<p class="hint2">${v.map.length} parts · ${v.map.filter(p=>p>=100).length} complete</p><div class="dlseg" aria-hidden="true">${v.map.map(p => `<i style="opacity:${0.12 + Math.max(0,Math.min(100,p))/100*0.88}"></i>`).join('')}</div>`;
}

/* ----- add with options ----- */
function dlReadHeaders(id) {
  const text=document.getElementById(id)?.value.trim();if(!text)return undefined;
  const header={};
  for(const line of text.split(/\r?\n/)) {if(!line.trim())continue;const at=line.indexOf(':');if(at<1)throw new Error('Use one header per line: Header-Name: value.');const name=line.slice(0,at).trim();if(!/^[!#$%&'*+.^_`|~\w-]+$/.test(name))throw new Error('A header name is invalid.');header[name]=line.slice(at+1).trim();}
  return header;
}
function dlExtraAddFields(prefix) {
  return `<label class="dl-field">Queue<select class="sel" id="${prefix}Queue">${dlQueueOpts('')}</select></label>
    <label class="dl-field">Speed limit<select class="sel" id="${prefix}Limit">${[0,.5,1,2,5,10,20,50].map(n=>`<option value="${n}">${n?n+' MB/s':'No limit'}</option>`).join('')}</select></label>
    <label class="dl-field">Connections per line<select class="sel" id="${prefix}Conns"><option value="0">Automatic</option>${[1,2,4,6,8,12,16].map(n=>`<option>${n}</option>`).join('')}</select></label>
    <label class="dl-field">Mirrors (one address per line)<textarea class="inp" id="${prefix}Mirrors" rows="2" spellcheck="false"></textarea></label>
    <label class="dl-field">Request headers (Header-Name: value)<textarea class="inp" id="${prefix}Header" rows="2" spellcheck="false" autocomplete="off"></textarea></label>
    <div class="row"><label class="dl-field">User name<input class="inp" id="${prefix}User" autocomplete="off"></label><label class="dl-field">Password<input type="password" class="inp" id="${prefix}Pass" autocomplete="new-password"></label></div>`;
}
function dlReadExtraAdd(body,prefix) {
  const value=suffix=>document.getElementById(prefix+suffix)?.value;
  body.queue=value('Queue')||'';body.conns=+(value('Conns')||0);body.limit=+(value('Limit')||0)*1048576;
  const mirrors=dlRowsOf(value('Mirrors')||'');if(mirrors.length)body.mirrors=mirrors;
  const header=dlReadHeaders(prefix+'Header');if(header)body.header=header;
  if(value('User')){body.user=value('User');body.pass=value('Pass')||'';}
}
function dlOptions() {
  const text=$('#dlUrl')?.value||'';
  dlDialog(`<div class="card" aria-label="Add with options"><h3>Add with options</h3><div class="dl-form">
    <label class="dl-field">Links (one per line)<textarea class="inp" id="oUrl" rows="3" spellcheck="false">${esc(text)}</textarea></label>
    <label class="dl-field">Name (single link only)<input class="inp" id="oName"></label>
    <label class="dl-field">Save in<input class="inp" id="oDir" placeholder="${esc(DL.config.dir||'Default download folder')}"></label>
    <label class="dl-field">Start at (leave empty to start now)<input type="time" class="inp" id="oTime"></label>
    <label class="dl-field">Checksum (SHA-256, SHA-1 or MD5)<input class="inp" id="oHash"></label>
    ${dlExtraAddFields('o')}
    <fieldset><legend>Use these connections</legend>${DL.lines.length?DL.lines.map(l=>`<label class="dl-check"><input type="checkbox" class="oLine" value="${esc(l.name)}" checked>${esc(labelOf(l.name))}</label>`).join(''):'<p>Connections will be selected when available.</p>'}</fieldset>
    <label class="dl-field">Stream height (0 chooses best)<input class="inp" id="oHeight" type="number" min="0" step="1" value="0"></label>
    <label class="dl-check"><input type="checkbox" id="oForce">Add another copy if already in the list</label>
    <p class="hint2">Patterns: <code>[001-020]</code>, <code>[a-f]</code>, <code>[1-100:5]</code>. Maximum 500 links. Passwords and headers are used only for these downloads.</p></div>
    <div class="dl-dialog-actions"><button class="btn" id="dlCancel">Cancel</button><button class="btn" id="oLater">Add to queue</button><button class="btn pri" id="oGo">Download now</button></div></div>`);
  $('#dlCancel').onclick=dlClose;
  const submit=async start=>{
    const urls=dlRowsOf($('#oUrl').value);if(!urls.length){$('#oUrl').focus();return dlNotice('Paste a link first.',true);}
    const body={action:'add',urls,start,force:$('#oForce').checked};
    for(const [id,key] of [['oName','name'],['oDir','dir'],['oHash','hash']]){const value=$('#'+id).value.trim();if(value)body[key]=value;}
    body.height=Math.max(0,+$('#oHeight').value||0);
    const time=$('#oTime').value;if(time){const d=new Date(),[h,m]=time.split(':').map(Number);d.setHours(h,m,0,0);if(d.getTime()<=Date.now())d.setDate(d.getDate()+1);body.startAt=Math.floor(d.getTime()/1000);}
    const lines=$$('.oLine');if(lines.length){body.lines=lines.filter(el=>el.checked).map(el=>el.value);if(!body.lines.length)return dlNotice('Choose at least one connection.',true);}
    try {dlReadExtraAdd(body,'o');} catch(e){return dlNotice(e.message,true);}
    const r=await dlAct(body);if(r){await dlClose(true);toast(dlAddResult(r,urls.some(s=>/\[/.test(s))?null:urls.length),!!r.duplicate&&!r.restarted);}
  };
  $('#oGo').onclick=()=>submit(true);$('#oLater').onclick=()=>submit(false);
}

/* ----- find links on a web page ----- */
let dlGrabbed = [];
function dlGrab(){
  dlDialog(`<div class="card" role="dialog" aria-modal="true" aria-label="Find links on a page" style="max-width:700px;max-height:90vh;overflow:auto"><h3 style="margin:0">Find links on a web page</h3>
    <div class="hint2" style="margin:6px 0 12px">Type the address of a page. NetMerge lists the files it links to, and you choose which to download.</div>
    <div class="dlbox"><input class="inp" id="gUrl" placeholder="https://…" style="flex:1;min-width:240px" spellcheck="false" aria-label="Page address"><select class="sel" id="gType" style="min-width:0" aria-label="Kind of file">${Object.keys(DL_TYPES).map(k => `<option>${k}</option>`).join('')}</select><button class="btn pri" id="gFind">Find</button></div>
    <div id="gOut" style="margin-top:12px"></div>
    <div class="row" style="justify-content:flex-end;gap:8px;margin-top:14px"><button class="btn" id="dlCancel">Close</button><button class="btn pri" id="gAdd" disabled>Download selected</button></div></div>`);
  $('#dlCancel').onclick = dlClose;
  $('#gFind').onclick = async () => {
    const u = $('#gUrl').value.trim(); if (!u) return;
    const token=dlDialogSerial;$('#gFind').disabled=true;$('#gAdd').disabled=true;
    $('#gOut').innerHTML = '<div class="hint2">Looking…</div>';
    try {
      const r = await api('downloads', {action:'grab', url: u, types: DL_TYPES[$('#gType').value]});
      if(token!==dlDialogSerial)return;
      dlGrabbed = r.links || [];
      $('#gOut').innerHTML = dlGrabbed.length ? `<div class="row" style="gap:10px;margin-bottom:6px"><label style="display:flex;gap:6px;align-items:center"><input type="checkbox" id="gAll" checked> All (${dlGrabbed.length})</label></div><div class="scrollbox" style="max-height:300px;overflow:auto">${dlGrabbed.map((g,i) => `<label style="display:flex;gap:8px;align-items:center;padding:3px 0"><input type="checkbox" class="gSel" data-i="${i}" checked><span style="min-width:0;overflow-wrap:anywhere" title="${esc(g.url)}">${esc(g.name)}<span class="hint2"> · ${esc(g.url)}</span></span></label>`).join('')}</div>` : '<div class="hint2">No matching links found on that page.</div>';
      $('#gAdd').disabled = !dlGrabbed.length;
      const all = $('#gAll'); if (all) all.onchange = () => $$('.gSel').forEach(c => c.checked = all.checked);
    } catch(e){ if(token===dlDialogSerial)dlNotice(e.message,true); } finally {if(token===dlDialogSerial)$('#gFind').disabled=false;}
  };
  $('#gUrl').onkeydown = e => { if (e.key === 'Enter') $('#gFind').click(); };
  $('#gAdd').onclick = async () => {
    const urls = $$('.gSel').filter(c => c.checked).map(c => dlGrabbed[+c.dataset.i].url); if (!urls.length) return;
    const r = await dlAct({action:'add', urls}, null);
    if (r) { dlClose(true); toast(dlAddResult(r,urls.length),!!r.duplicate&&!r.restarted); }
  };
  setTimeout(() => $('#gUrl') && $('#gUrl').focus(), 30);
}

/* ----- properties of one download ----- */
async function dlProps(id) {
  const initial=DL.items.find(v=>v.id===id);if(!initial)return;
  let v=initial;
  if(v.kind==='torrent'&&v.nFiles){const r=await dlAct({action:'files',id});if(r?.files)v={...v,files:r.files,select:r.select,trackers:r.trackers,priv:r.private,pieceLen:r.pieceLen};}
  const edit=v.state!=='done',name=v.name||'Download',sh=dlLineShare(v);
  const action=(a,label,extra='')=>{
    const attrs=`data-dl="${a}" data-id="${esc(id)}" ${extra}`;
    if(a==='move')return dlTool(extra.includes('top')?'ArrowUpToLine':extra.includes('down')?'ArrowDown':'ArrowUp',label,attrs,'action');
    const [icon,tone]=DL_ACTIONS[a];
    return `<button class="btn dl-tone-${tone}" ${attrs}>${dlIcon(icon)}<span>${label}</span></button>`;
  };
  const limit=(v.limit||0)/1048576;
  dlDialog(`<div class="card dl-properties" data-details-id="${esc(id)}" aria-label="Download details"><h3>${esc(name)}</h3><p class="dl-path">${esc(v.url||'')}</p>
    <div class="dl-properties-actions">${v.state==='done'?action('open','Open file')+action('folder','Show in folder'):action(v.state==='paused'?'resume':v.state==='error'?'retry':'pause',v.state==='paused'?'Resume':v.state==='error'?'Retry':'Pause')}${action('remove','Remove from list')}${v.state==='done'?action('delfile','Delete file from computer'):''}</div>
    <details open class="dl-detail-group"><summary>Summary</summary><dl class="dl-facts"><dt>Status</dt><dd data-detail-state>${esc(dlStateText(v))}${v.note?' · '+esc(v.note):''}</dd><dt>Size</dt><dd>${v.size>0?bytes(v.size):'Size unknown'} · ${bytes(v.done||0)} downloaded</dd><dt>Saved as</dt><dd>${esc(v.path||'Not decided yet')}</dd><dt>Added</dt><dd>${v.added?new Date(v.added*1000).toLocaleString():'—'}</dd>${v.finished?`<dt>Finished</dt><dd>${new Date(v.finished*1000).toLocaleString()}</dd>`:''}${v.err?`<dt>Error</dt><dd class="bad">${esc(v.err)} ${esc(v.detail||'')}</dd>`:''}${v.verify?`<dt>Checksum</dt><dd class="${v.verify==='bad'?'bad':''}">${v.verify==='ok'?'Matches':v.verify==='bad'?'Does not match. Do not open this file.':'Checking…'}</dd>`:''}${v.kind==='torrent'?`<dt>Peers</dt><dd>${v.peers||0} connected · ${v.seeds||0} seeds${v.swarmSeeds?' · '+v.swarmSeeds+' swarm seeds':''}</dd><dt>Uploaded</dt><dd>${bytes(v.up||0)}${v.done>0?' · ratio '+((v.up||0)/v.done).toFixed(2):''}</dd><dt>Pieces</dt><dd>${v.pieceLen?bytes(v.pieceLen)+' each':'Metadata may still be loading'}${v.priv?' · private torrent':''}</dd><dt>Trackers</dt><dd>${(v.trackers||[]).map(esc).join('<br>')||'Not available yet'}</dd>`:''}</dl></details>
    <details class="dl-detail-group"><summary>Connections & parts</summary>${v.map?.length?dlSegBar(v):'<p>No part map available.</p>'}<ul class="dl-connections">${Object.entries(sh.tot).map(([line,size])=>`<li>${esc(labelOf(line))}: ${bytes(size)}${sh.sum?' · '+Math.round(size/sh.sum*100)+'%':''}</li>`).join('')}</ul>${(v.lineStats||[]).map(l=>`<p>${esc(labelOf(l.name))} · ${esc(l.status||'Unknown')} · ${Number.isFinite(l.speed)?bytes(l.speed)+'/s':'—'} · ${bytes(l.bytes||0)}</p>`).join('')}</details>
    ${v.kind==='torrent'?`<details class="dl-detail-group" open><summary>Files</summary><div class="scrollbox">${(v.files||[]).map((f,i)=>`<label class="dl-check"><input type="checkbox" class="tfSel" ${!v.select||v.select[i]?'checked':''} ${edit?'':'disabled'}><span>${esc(f.name)}</span><span class="hint2">${bytes(f.size)}</span></label>`).join('')||'<p>Torrent metadata is not available yet. Open details again after it arrives.</p>'}</div>${edit&&v.files?.length?'<button class="btn" id="tfSave">Download selected files</button>':''}</details>`:''}
    <details class="dl-detail-group"><summary>Activity</summary><div class="scrollbox">${(v.log||[]).slice().reverse().map(l=>`<p><time>${new Date(l.t*1000).toLocaleTimeString()}</time> ${esc(l.s)}</p>`).join('')||'<p>No activity yet.</p>'}</div></details>
    ${edit?`<details class="dl-detail-group" open><summary>Options</summary><div class="dl-form"><label class="dl-field">Queue<select class="sel" id="pQ">${dlQueueOpts(v.queue)}</select></label><label class="dl-field">Speed limit<select class="sel" data-lim="${esc(id)}" aria-label="Speed limit for ${esc(name)}">${[...new Set([0,.5,1,2,5,10,20,50,limit])].sort((a,b)=>a-b).map(n=>`<option value="${n}" ${n===limit?'selected':''}>${n?n+' MB/s':'No limit'}</option>`).join('')}</select></label>${['queued','paused'].includes(v.state)?`<div class="row">${action('move','Move up','data-mv="up"')}${action('move','Move down','data-mv="down"')}${action('move','Download first','data-mv="top"')}</div>`:''}${v.kind!=='torrent'?`<label class="dl-field">Connections per line<select class="sel" id="pC"><option value="0">Automatic</option>${[1,2,4,6,8,12,16].map(n=>`<option ${v.conns===n?'selected':''}>${n}</option>`).join('')}</select></label><label class="dl-field">Mirrors (one per line)<textarea class="inp" id="pM" rows="2">${esc((v.mirrors||[]).join('\n'))}</textarea></label>`:''}<button class="btn pri" id="pSave">Save options</button>${v.kind!=='torrent'?`<label class="dl-field">Replace an expired link<input class="inp" id="pLink" placeholder="https://…"></label><label class="dl-field">Headers for the new link (Header-Name: value)<textarea class="inp" id="pHeader" rows="2" autocomplete="off"></textarea></label><p class="hint2" id="pRelinkNote">Keeps downloaded data.${v.state==='downloading'?' Pause the download before replacing its link.':''}</p><button class="btn" id="pRelink" ${v.state==='downloading'?'disabled':''}>Use this link</button>`:''}</div></details>`:''}
    <div class="dl-dialog-actions"><button class="btn" id="dlCancel">Close</button></div></div>`);
  $('#dlCancel').onclick=dlClose;
  if($('#tfSave'))$('#tfSave').onclick=async()=>{const select=$$('.tfSel').map(e=>e.checked);if(!select.some(Boolean))return dlNotice('Choose at least one file.',true);if(await dlAct({action:'select',id,select},'File selection saved'))dlClose(true);};
  if(!edit)return;
  $('#pSave').onclick=async()=>{
    // Capture every draft before the first request; partial failure keeps all edits visible.
    const queue=$('#pQ').value,conns=+($('#pC')?.value||0),urls=dlRowsOf($('#pM')?.value||'');
    if(await dlAct({action:'setQueue',id,queue})===null)return;
    if(v.kind!=='torrent'){if(await dlAct({action:'setConns',id,conns})===null)return;if(await dlAct({action:'setMirrors',id,urls})===null)return;}
    dlClose(true);toast('Download options saved');
  };
  if($('#pRelink'))$('#pRelink').onclick=async()=>{const url=$('#pLink').value.trim();if(!url){$('#pLink').focus();return dlNotice('Paste a replacement link.',true);}let header;try{header=dlReadHeaders('pHeader');}catch(e){return dlNotice(e.message,true);}if(await dlAct({action:'relink',id,url,...(header?{header}:{})},'Link changed'))dlClose(true);};
}

/* ----- queues ----- */
function dlQueues(){
  const base=dlCopy(DL.config), qs=dlCopy(base.queues||[]).map(q=>({...q,window:q.window||{enabled:false,from:'23:00',to:'06:00'}}));
  const draw = () => {
    dlDialog(`<div class="card" role="dialog" aria-modal="true" aria-label="Queues" style="max-width:700px;max-height:92vh;overflow:auto"><h3 style="margin:0">Queues</h3>
      <div class="hint2" style="margin:6px 0 12px">A queue is a group of downloads with its own hours. Downloads in no queue (the main queue) follow your general settings. Put a download in a queue from its Properties, or when you add it.</div>
      ${qs.length ? qs.map((q,i) => `<div class="card" style="margin:0 0 10px;padding:12px;background:var(--inset)">
        <div class="row" style="gap:8px;flex-wrap:wrap"><input class="inp qn" data-i="${i}" value="${esc(q.name)}" style="flex:1;min-width:150px" aria-label="Queue name" maxlength="40">
          <label class="hint2">At once <select class="sel qm" data-i="${i}" style="min-width:0">${[0,1,2,3,4,6,8,10].map(n => `<option value="${n}"${q.maxActive===n?' selected':''}>${n || 'general limit'}</option>`).join('')}</select></label>
          <button class="btn" data-q="del" data-i="${i}">Delete</button></div>
        <div class="row" style="gap:8px;flex-wrap:wrap;margin-top:8px"><label style="display:flex;gap:6px;align-items:center" class="hint2"><input type="checkbox" class="qw" data-i="${i}"${q.window.enabled?' checked':''}> Only between</label>
          <input type="time" class="inp qf" data-i="${i}" value="${esc(q.window.from)}" aria-label="Start time for ${esc(q.name)}" style="min-width:0"> <span class="hint2">and</span> <input type="time" class="inp qt" data-i="${i}" value="${esc(q.window.to)}" aria-label="End time for ${esc(q.name)}" style="min-width:0"></div>
        <div class="row" style="gap:10px;flex-wrap:wrap;margin-top:8px"><span class="hint2">Days</span>${DL_DAYS.map((d,k) => `<label style="display:flex;gap:4px;align-items:center" class="hint2"><input type="checkbox" class="qd" data-i="${i}" data-k="${k}"${(q.days||[]).includes(k)?' checked':''}>${d}</label>`).join('')}<span class="hint2">(none ticked = every day)</span></div>
        <div class="row" style="gap:12px;margin-top:8px"><label style="display:flex;gap:6px;align-items:center" class="hint2"><input type="checkbox" class="qp" data-i="${i}"${q.paused?' checked':''}> Paused</label>
          <button class="btn" data-q="start" data-i="${i}" data-id="${esc(q.id)}"${q.id?'':' disabled'}>Start all in this queue</button></div></div>`).join('') : '<div class="hint2">No queues yet.</div>'}
      <div class="row" style="gap:8px;margin-top:10px"><button class="btn" data-q="add"${qs.length>=12?' disabled':''}>Add a queue</button></div>
      <div class="row" style="justify-content:flex-end;gap:8px;margin-top:14px"><button class="btn" id="dlCancel">Cancel</button><button class="btn pri" id="qSave">Save</button></div></div>`);
    $('#dlCancel').onclick = dlClose;
    $('#qSave').onclick = async () => { sync(); const r = await dlAct({action:'config', config: {...base, queues: qs}}, 'Queues saved', base); if (r !== null) dlClose(true); };
    $$('[data-q]').forEach(b => b.onclick = async () => {
      sync();
      if (b.dataset.q === 'add') {$('#dlDlg').dlDirty=true;qs.push({id:'', name:'Queue ' + (qs.length + 1), maxActive:0, window:{enabled:false, from:'23:00', to:'06:00'}, days:[], paused:false});}
      else if (b.dataset.q === 'del') { if (!await dlAsk('Delete this queue? Its downloads move to the main queue.')) return; qs.splice(+b.dataset.i, 1);$('#dlDlg').dlDirty=true; }
      else if (b.dataset.q === 'start') { await dlAct({action:'startQueue', queue: b.dataset.id}, 'Started'); return; }
      draw();
    });
  };
  const sync = () => {
    $$('.qn').forEach(e => qs[+e.dataset.i].name = e.value.trim() || 'Queue');
    $$('.qm').forEach(e => qs[+e.dataset.i].maxActive = +e.value);
    $$('.qw').forEach(e => qs[+e.dataset.i].window.enabled = e.checked);
    $$('.qf').forEach(e => qs[+e.dataset.i].window.from = e.value || '23:00');
    $$('.qt').forEach(e => qs[+e.dataset.i].window.to = e.value || '06:00');
    $$('.qp').forEach(e => qs[+e.dataset.i].paused = e.checked);
    qs.forEach(q => q.days = []); $$('.qd').forEach(e => { if (e.checked) qs[+e.dataset.i].days.push(+e.dataset.k); });
  };
  draw();
}

/* ----- folders by type and by website ----- */
function dlFolders(){
  const base=dlCopy(DL.config), cfg=dlCopy(base), rules=dlCopy(cfg.siteRules||[]);
  const draw = () => {
    dlDialog(`<div class="card" role="dialog" aria-modal="true" aria-label="Folders" style="max-width:680px;max-height:92vh;overflow:auto"><h3 style="margin:0">Folders for types and websites</h3>
      <div class="hint2" style="margin:6px 0 12px">A folder you choose while adding a download always wins. Then a website's folder, then the folder for the type, then your main folder (${esc(cfg.dir)}).</div>
      <h4 style="margin:6px 0">By type</h4>
      <div style="display:grid;gap:6px">${DL_CATS.map(c => `<label class="row" style="gap:10px"><span style="width:100px">${c}</span><input class="inp fc" data-c="${c}" value="${esc((cfg.catDirs||{})[c]||'')}" placeholder="(main folder / ${c})" style="flex:1" spellcheck="false"></label>`).join('')}</div>
      <h4 style="margin:14px 0 6px">By website</h4>
      <div style="display:grid;gap:6px">${rules.map((r,i) => `<div class="row" style="gap:8px"><input class="inp fs" data-i="${i}" value="${esc(r.site)}" aria-label="Website" placeholder="example.com" style="width:180px" spellcheck="false"><input class="inp fd" data-i="${i}" value="${esc(r.dir)}" aria-label="Website folder" placeholder="Folder" style="flex:1" spellcheck="false"><button class="btn" data-fr="${i}">Remove</button></div>`).join('') || '<div class="hint2">No websites yet.</div>'}</div>
      <div class="row" style="margin-top:8px"><button class="btn" id="fAdd">Add a website</button></div>
      <div class="row" style="justify-content:flex-end;gap:8px;margin-top:14px"><button class="btn" id="dlCancel">Cancel</button><button class="btn pri" id="fSave">Save</button></div></div>`);
    $('#dlCancel').onclick = dlClose;
    $('#fAdd').onclick = () => { sync(); rules.push({site:'', dir:''});$('#dlDlg').dlDirty=true; draw(); };
    $$('[data-fr]').forEach(b => b.onclick = () => { sync(); rules.splice(+b.dataset.fr, 1);$('#dlDlg').dlDirty=true; draw(); });
    $('#fSave').onclick = async () => {
      sync(); const catDirs = {}; $$('.fc').forEach(e => { if (e.value.trim()) catDirs[e.dataset.c] = e.value.trim(); });
      const r = await dlAct({action:'config', config: {...base, catDirs, siteRules: rules.filter(x => x.site.trim() && x.dir.trim())}}, 'Saved', base); if (r !== null) dlClose(true);
    };
  };
  const sync = () => { cfg.catDirs={};$$('.fc').forEach(e=>{if(e.value.trim())cfg.catDirs[e.dataset.c]=e.value;}); $$('.fs').forEach(e => rules[+e.dataset.i].site = e.value); $$('.fd').forEach(e => rules[+e.dataset.i].dir = e.value); };
  draw();
}

/* ----- save / load the list ----- */
async function dlExport(){
  const r = await dlAct({action:'export'}); if (!r || !r.file) return;
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob([JSON.stringify(r.file, null, 2)], {type:'application/json'}));
  a.download = 'netmerge-downloads-' + new Date().toISOString().slice(0,10) + '.json'; document.body.appendChild(a); a.click(); a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 4000);
}
function dlImport(){
  const f = document.createElement('input'); f.type = 'file'; f.accept = '.json,.txt,.lst,.csv';
  f.onchange = async () => {
    const file = f.files[0]; if (!file) return; if (file.size > 5e6) return toast('That file is too big', true);
    const r = await dlAct({action:'import', text: await file.text()});
    if (r) toast(`${r.imported} downloads added` + (r.skipped ? `, ${r.skipped} skipped` : ''));
  };
  f.click();
}

document.addEventListener('click', async e => {
  const t = e.target.closest('button'); if (!t || !t.dataset.dm || !DL) return;
  const a = t.dataset.dm;
  if (a === 'options') return dlOptions();
  if (a === 'grab') return dlGrab();
  if (a === 'queues') return dlQueues();
  if (a === 'folders') return dlFolders();
  if (a === 'props') return dlProps(t.dataset.id);
  if (a === 'export') return dlExport();
  if (a === 'import') return dlImport();
  if (a === 'torTog') return void dlConfigEdit({noTorrents: !DL.config.noTorrents});
  if (a === 'upTog') return void dlConfigEdit({noUpload: !DL.config.noUpload});
  if (a === 'notifyTog') return void dlConfigEdit({noNotify: !DL.config.noNotify});
  if (a === 'afterSave') {
    const v = $('#dlAfterCmd').value.trim();
    if (v && !await dlAsk('Run this after every finished download?\n\n' + v + '\n\nOnly use a program you trust.')) return;
    return void dlConfigEdit({afterCmd: v}, v ? 'Saved' : 'Cleared');
  }
});
document.addEventListener('change', e => {
  if (e.target.id === 'dlSort') { dlSort = e.target.value; dlOrderKey='';paintDL(); }
  if (e.target.id === 'dlQf') { dlQf = e.target.value; paintDL(); }
});
