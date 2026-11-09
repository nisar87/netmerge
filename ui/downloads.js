// Classic entry point shared with downloads-more.js. The parent owns enablement and routing.
let DL = null, dlFilter = 'all', dlCat = '', dlQ = '', dlPrev = {};
let dlTab = (location.hash.match(/^#\/downloads\/(transfers|queues|browser|settings)$/)||[])[1] || 'transfers';
let dlPage = 0, dlRead = null, dlRevision = 0, dlConfigTail = Promise.resolve(), dlLastGood = 0, dlError = '';
let dlOrderKey = '', dlOrder = [], dlStopPolling = null;
const dlPending = new Map(), dlSel = new Set(); let dlVisIds = [];
const DL_CATS = ['Programs','Archives','Videos','Music','Documents','Images','Torrents','Other'];
const DL_RISKY = /\.(exe|msi|bat|cmd|ps1|vbs|js|jar|scr|com|lnk|msix|appx|reg|hta)$/i;
const dlCopy = x => x === undefined ? undefined : JSON.parse(JSON.stringify(x));
const dlEqual = (a,b) => JSON.stringify(a) === JSON.stringify(b);
const dlObject = x => x && typeof x === 'object' && !Array.isArray(x);
const dlRequest = (body, signal, fresh=false) => window.NM && NM.request ? NM.request('downloads', {body,signal,fresh}) : api('downloads', body);

// Three-way merge: apply the user's changes to a just-read full config. Queue identity is stable.
function dlMergeConfig(base, desired, fresh, key = '') {
  if (dlEqual(base, desired)) return dlCopy(fresh);
  if (key === 'queues' && Array.isArray(base) && Array.isArray(desired)) {
    const before = new Map(base.filter(q=>q.id).map(q=>[q.id,q]));
    const after = new Map(desired.filter(q=>q.id).map(q=>[q.id,q]));
    const result = (fresh||[]).filter(q=>!before.has(q.id)||after.has(q.id)).map(q=>after.has(q.id) ? dlMergeConfig(before.get(q.id)||{},after.get(q.id),q) : dlCopy(q));
    for (const q of desired) if (!q.id || (!before.has(q.id) && !result.some(x=>x.id===q.id))) result.push(dlCopy(q));
    return result;
  }
  if ((!base || dlObject(base)) && dlObject(desired)) {
    base=base||{};
    const result = {...(fresh||{})};
    for (const k of new Set([...Object.keys(base),...Object.keys(desired)])) {
      if (dlEqual(base[k],desired[k])) continue;
      if (!Object.hasOwn(desired,k)) delete result[k];
      else result[k] = dlMergeConfig(base[k],desired[k],result[k],k);
    }
    return result;
  }
  return dlCopy(desired);
}
// Inline controls send only explicitly edited fields, even when reverting a pending save.
// Undefined deletes a map entry; dialogs still use the three-way merge above.
function dlMergePatch(fresh, patch) {
  const result={...(fresh||{})};
  for (const [key,value] of Object.entries(patch)) {
    if (value===undefined) delete result[key];
    else result[key]=dlObject(value) ? dlMergePatch(result[key],value) : dlCopy(value);
  }
  return result;
}
const dlConfigEdit=(config,okMsg)=>dlAct({action:'config',config},okMsg,null);
function dlNormalize(r) {
  return {...r, items:r.items||[], lines:r.lines||[], config:{...(r.config||{}),queues:r.config?.queues||[],window:r.config?.window||{}}, ext:r.ext ? {...r.ext,browsers:r.ext.browsers||[]} : null};
}
async function loadDL(force) {
  if (typeof DM_ON !== 'undefined' && !DM_ON) return;
  dlStartPolling();
  if (dlRead) return dlRead;
  const revision = dlRevision;
  dlRead = (async()=>{
    try {
      const result = await dlRequest();
      if (revision !== dlRevision) return;
      DL = dlNormalize(result); dlLastGood = Date.now(); dlError = '';
      const next = {};
      for (const v of DL.items) { if (dlPrev[v.id] && dlPrev[v.id] !== 'done' && v.state === 'done') toast('Finished: '+v.name); next[v.id]=v.state; }
      dlPrev=next; paintDL();
    } catch(e) { if (e.name !== 'AbortError') { dlError=e.message||'Could not refresh downloads.'; paintDL(); } }
  })().finally(()=>{dlRead=null;});
  return dlRead;
}
function dlNotice(message, bad = false) {
  const d = $('#dlDlg');
  if (d) {
    let el=d.querySelector('.dl-notice');
    if (!el) { el=document.createElement('div'); el.className='dl-notice'; d.querySelector('.card').prepend(el); }
    el.setAttribute('role',bad?'alert':'status'); el.textContent=message; el.classList.toggle('bad',bad);
  } else { if (bad) { dlError=message; paintDL(); } toast(message,bad); }
}
async function dlAct(body, okMsg, configBase = DL?.config) {
  const isConfig=body.action==='config', isExt=body.action==='extConfig';
  // Inline reversals are distinct choices; repeated dialog submissions still deduplicate.
  const key = (isConfig&&configBase===null)||isExt ? Symbol() : JSON.stringify(body);
  if (dlPending.has(key)) return dlPending.get(key);
  const base = isConfig ? dlCopy(configBase) : null;
  const desired = isConfig ? structuredClone(body.config) : null;
  const opener = document.activeElement;
  const changed = isConfig ? Object.keys(desired).filter(k=>!base||!dlEqual(base[k],desired[k])) : [];
  const fieldKeys={dlDir:'dir',dlAfterCmd:'afterCmd',dlWinFrom:'window',dlWinTo:'window',dlMax:'maxActive',dlPer:'maxPerLine'};
  const fields = new Map($$('#dlBody [data-dirty]').filter(el=>changed.includes(fieldKeys[el.id]||el.dataset.cfg)).map(el=>[el,el.value]));
  const perform = async()=>{
    const wasDisabled=opener?.disabled, dialog=$('#dlDlg');
    if(dialog)dialog.dlPending=(dialog.dlPending||0)+1;
    if (opener?.tagName==='BUTTON') { opener.disabled=true; opener.setAttribute('aria-busy','true'); }
    try {
      let request=body;
      if (isConfig) {
        const latest=await dlRequest(undefined,undefined,true);
        if (latest.enabled === false || !latest.config) throw new Error('Downloads are off. Enable them before saving.');
        request={action:'config',config:base ? dlMergeConfig(base,desired,latest.config) : dlMergePatch(latest.config,desired)};
      }
      if (isExt) {
        const latest=await dlRequest(undefined,undefined,true);
        if (!latest.ext || latest.enabled===false) throw new Error('Browser preferences are unavailable. Refresh and try again.');
        request={action:'extConfig',extMinMB:body.extMinMB??latest.ext.minMB,extOff:body.extOff??!latest.ext.enabled};
      }
      dlRevision++;
      const r=await dlRequest(request);
      if (r?.items) { DL=dlNormalize(r); dlLastGood=Date.now(); }
      // A read started before this action cannot replace its authoritative result.
      if (dlRead) await dlRead;
      // Drain in-flight reads before resetting: a poll during the move may carry old order.
      if (body.action==='move') dlOrder=[];
      await loadDL(true);
      if (isConfig) for (const [el,value] of fields) if (el.value===value) delete el.dataset.dirty;
      if (isExt && $('#dlExtMin')?.value===String(body.extMinMB)) delete $('#dlExtMin').dataset.dirty;
      paintDL();
      if (okMsg) dlNotice(okMsg);
      return r;
    } catch(e) {
      dlNotice(e.message||'Could not complete this action.',true);
      // POST may have applied: reconcile, but never replay it.
      if (dlRead) await dlRead;
      if (body.action==='move') dlOrder=[];
      await loadDL(true);
      dlNotice(e.message||'Could not confirm this change. Refresh and try again.',true);
      return null;
    } finally { if(dialog)dialog.dlPending--;if (opener?.tagName==='BUTTON') { opener.disabled=wasDisabled; opener.removeAttribute('aria-busy'); } }
  };
  const promise=isConfig||isExt ? dlConfigTail.then(perform,perform) : perform();
  if (isConfig||isExt) dlConfigTail=promise.then(()=>{});
  dlPending.set(key,promise);
  try { return await promise; } finally { dlPending.delete(key); }
}
async function dlAsk(message, title='Confirm action', danger=true, confirmLabel='Continue') {
  // Avoid nested dialogs when confirming an edit inside the native top-layer dialog.
  const d=$('#dlDlg');
  if (!d) return window.NM?.confirm ? NM.confirm({title,message,confirmLabel,danger}) : window.confirm(message);
  if (d.querySelector('.dl-confirm')) return false;
  return new Promise(resolve=>{
    const box=document.createElement('section'); box.className='dl-confirm'; box.setAttribute('role','alertdialog'); box.setAttribute('aria-label',title);
    box.innerHTML=`<h3>${esc(title)}</h3><p>${esc(message)}</p><div class="row"><button class="btn" data-answer="no">Go back</button><button class="btn ${danger?'danger':'pri'}" data-answer="yes">${esc(confirmLabel)}</button></div>`;
    const content=d.querySelector('.card'), oldFocus=document.activeElement;
    content.inert=true; content.hidden=true; d.append(box);
    const done=value=>{box.remove();content.inert=false;content.hidden=false;d.dlCancelConfirm=null;oldFocus?.focus();resolve(value);};
    d.dlCancelConfirm=()=>done(false);
    box.querySelector('[data-answer=no]').onclick=()=>done(false);
    box.querySelector('[data-answer=yes]').onclick=()=>done(true);
    box.querySelector('button').focus();
  });
}
const dlStateText = v => v.state === 'downloading' ? 'Downloading' : v.state === 'queued' ? (v.retryAt > Date.now()/1000 ? 'Trying again soon' : v.startAt > Date.now()/1000 ? 'Scheduled' : 'Waiting') : v.state === 'paused' ? 'Paused' : v.state === 'done' ? 'Finished' : 'Needs attention';
function dlLineShare(v){
  const tot={...(v.bytes||{})};
  (v.lineStats||[]).forEach(l=>tot[l.name]=(tot[l.name]||0)+(l.bytes||0));
  return {tot,sum:Object.values(tot).reduce((a,b)=>a+b,0)};
}
function dlIcon(name) {
  if (window.NM?.icon) return NM.icon(name.replace(/([a-zA-Z])(\d)/g,'$1-$2'));
  // Small local fallback for the existing shell; the new shell supplies its icon subset.
  const paths={Download:'M12 3v12m-5-5 5 5 5-5M4 18v3h16v-3',Plus:'M12 5v14M5 12h14',Pause:'M8 5v14M16 5v14',Play:'m8 4 12 8-12 8Z',RotateCcw:'M4 10a8 8 0 1 1 1 8M4 4v6h6',FolderOpen:'M3 8V4h6l3 4h9l-3 12H3Z',File:'M6 3h8l4 4v14H6ZM14 3v5h4',MoreHorizontal:'M4 12h1m6 0h1m6 0h1',CircleCheck:'m8 12 3 3 5-6',CircleAlert:'M12 7v6m0 4h.01',Clock3:'M12 6v6l4 2',Globe:'M3 12h18M12 3a18 18 0 0 0 0 18 18 18 0 0 0 0-18Z',X:'m6 6 12 12M6 18 18 6',ArrowUp:'M12 20V4m-6 6 6-6 6 6',ArrowDown:'M12 4v16m-6-6 6 6 6-6'};
  const extraIcons={"Trash2":"<path d=\"M10 11v6\" /> <path d=\"M14 11v6\" /> <path d=\"M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6\" /> <path d=\"M3 6h18\" /> <path d=\"M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2\" />","ListChecks":"<path d=\"M13 5h8\" /> <path d=\"M13 12h8\" /> <path d=\"M13 19h8\" /> <path d=\"m3 17 2 2 4-4\" /> <path d=\"m3 7 2 2 4-4\" />","ChevronLeft":"<path d=\"m15 18-6-6 6-6\" />","Pencil":"<path d=\"M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z\" /> <path d=\"m15 5 4 4\" />","ListPlus":"<path d=\"M16 5H3\" /> <path d=\"M11 12H3\" /> <path d=\"M16 19H3\" /> <path d=\"M18 9v6\" /> <path d=\"M21 12h-6\" />","ExternalLink":"<path d=\"M15 3h6v6\" /> <path d=\"M10 14 21 3\" /> <path d=\"M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6\" />","ChevronRight":"<path d=\"m9 18 6-6-6-6\" />","Settings2":"<path d=\"M14 17H5\" /> <path d=\"M19 7h-9\" /> <circle cx=\"17\" cy=\"17\" r=\"3\" /> <circle cx=\"7\" cy=\"7\" r=\"3\" />","Search":"<path d=\"m21 21-4.34-4.34\" /> <circle cx=\"11\" cy=\"11\" r=\"8\" />","Link":"<path d=\"M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71\" /> <path d=\"M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71\" />","Upload":"<path d=\"M12 3v12\" /> <path d=\"m17 8-5-5-5 5\" /> <path d=\"M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4\" />","Unplug":"<path d=\"m19 5 3-3\" /> <path d=\"m2 22 3-3\" /> <path d=\"M6.3 20.3a2.4 2.4 0 0 0 3.4 0L12 18l-6-6-2.3 2.3a2.4 2.4 0 0 0 0 3.4Z\" /> <path d=\"M7.5 13.5 10 11\" /> <path d=\"M10.5 16.5 13 14\" /> <path d=\"m12 6 6 6 2.3-2.3a2.4 2.4 0 0 0 0-3.4l-2.6-2.6a2.4 2.4 0 0 0-3.4 0Z\" />","ArrowUpToLine":"<path d=\"M5 3h14\" /> <path d=\"m18 13-6-6-6 6\" /> <path d=\"M12 7v14\" />"};
  if(extraIcons[name])return `<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">${extraIcons[name]}</svg>`;
  const circle=['CircleCheck','CircleAlert','Clock3','Globe'].includes(name)?'<circle cx="12" cy="12" r="9"/>':'';
  return `<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">${circle}<path d="${paths[name]||paths.File}"/></svg>`;
}
const DL_ACTIONS={pause:['Pause','warn'],resume:['Play','good'],retry:['RotateCcw','action'],open:['ExternalLink','action'],folder:['FolderOpen','action'],remove:['Trash2','bad'],delfile:['Trash2','bad']};
function dlTool(icon,label,attrs='',tone='neutral',hint=label,tag='button'){
  return `<${tag} class="btn dl-icon-button dl-tone-${tone}" aria-label="${esc(label)}" data-tooltip="${esc(hint)}" ${attrs}>${dlIcon(icon)}</${tag}>`;
}
// One hoverable hint for all compact controls, including controls in native dialogs.
let dlHint=null,dlHintOwner=null,dlHintTimer=null;
function dlHideHint(){
  clearTimeout(dlHintTimer);dlHintOwner?.removeAttribute('aria-describedby');dlHintOwner=null;
  if(dlHint)dlHint.hidden=true;
}
function dlPositionHint(){
  if(!dlHintOwner?.isConnected||!dlHintOwner.getClientRects().length||dlHintOwner.disabled)return dlHideHint();
  dlHint.textContent=dlHintOwner.dataset.tooltip;dlHintOwner.setAttribute('aria-describedby','dlActionHint');
  const anchor=dlHintOwner.getBoundingClientRect(),hint=dlHint.getBoundingClientRect();
  dlHint.style.left=Math.max(8,Math.min(innerWidth-hint.width-8,anchor.left+(anchor.width-hint.width)/2))+'px';
  dlHint.style.top=(anchor.top>=hint.height+12?anchor.top-hint.height-8:Math.min(innerHeight-hint.height-8,anchor.bottom+8))+'px';
}
function dlShowHint(owner){
  clearTimeout(dlHintTimer);if(owner.disabled)return;
  if(dlHintOwner!==owner)dlHideHint();
  if(!dlHint){
    dlHint=document.createElement('div');dlHint.id='dlActionHint';dlHint.className='nm-download-tooltip';dlHint.setAttribute('role','tooltip');
    dlHint.onpointerenter=()=>clearTimeout(dlHintTimer);
    dlHint.onpointerleave=()=>{if(!dlHintOwner?.matches(':focus-visible'))dlHideHint();};
  }
  (owner.closest('dialog')||document.body).append(dlHint);
  dlHintOwner=owner;dlHint.hidden=false;dlPositionHint();
}
document.addEventListener('pointerover',e=>{
  const owner=e.target.closest?.('[data-tooltip]');if(owner&&e.pointerType!=='touch')dlShowHint(owner);
});
document.addEventListener('pointerout',e=>{
  if(!dlHintOwner||dlHintOwner.contains(e.relatedTarget)||dlHint?.contains(e.relatedTarget)||dlHintOwner.matches(':focus-visible'))return;
  if(dlHintOwner.contains(e.target))dlHintTimer=setTimeout(dlHideHint,120);
});
document.addEventListener('focusin',e=>{const owner=e.target.closest?.('[data-tooltip]');if(owner)dlShowHint(owner);else dlHideHint();});
document.addEventListener('focusout',e=>{if(dlHintOwner?.contains(e.target))dlHideHint();});
document.addEventListener('keydown',e=>{if(e.key==='Escape'&&dlHintOwner){e.preventDefault();e.stopImmediatePropagation();dlHideHint();}},true);
document.addEventListener('click',dlHideHint,true);
addEventListener('scroll',dlHideHint,true);addEventListener('resize',dlHideHint);
const dlDuration = s => s<60 ? Math.ceil(s)+'s' : s<3600 ? Math.ceil(s/60)+'m' : Math.ceil(s/3600)+'h';
function dlPageItems() {
  const q=dlQ.toLowerCase();
  const items=(DL?.items||[]).filter(v=>(dlFilter==='all'||(dlFilter==='active'&&['downloading','queued','paused'].includes(v.state))||(dlFilter==='done'&&v.state==='done')||(dlFilter==='bad'&&v.state==='error')) && (!dlCat||v.category===dlCat) && dlQueueMatch(v) && (!q||`${v.name||''} ${v.url||''}`.toLowerCase().includes(q)));
  const key=[dlFilter,dlCat,dlQ,dlQf,dlSort].join('\0');
  if (key!==dlOrderKey) {dlOrderKey=key;dlPage=0;dlOrder=[];}
  dlSortItems(items);
  // Keep the established order through telemetry ticks. Explicit moves/filters/sorts reset it.
  const rank=new Map(dlOrder.map((id,i)=>[id,i]));
  items.sort((a,b)=>(rank.get(a.id)??Infinity)-(rank.get(b.id)??Infinity));
  dlOrder=items.map(v=>v.id);
  const total=items.length, pages=Math.max(1,Math.ceil(total/100));
  dlPage=Math.max(0,Math.min(dlPage,pages-1));
  return {items:items.slice(dlPage*100,(dlPage+1)*100),total,pages};
}
function dlRow(v) {
  const known=v.size>0, pct=known?Math.max(0,Math.min(100,(v.done||0)/v.size*100)):null;
  let host=''; try {host=new URL(v.url).hostname;} catch(_) {}
  const name=v.name||v.url||'Download', state=dlStateText(v);
  const icon=v.state==='downloading'?'Download':v.state==='done'?'CircleCheck':v.state==='error'?'CircleAlert':v.state==='paused'?'Pause':'Clock3';
  const action=v.state==='done'?'open':v.state==='paused'?'resume':v.state==='error'?'retry':'pause';
  const actionLabel={open:'Open',resume:'Resume',retry:'Retry',pause:'Pause'}[action];
  const [actionIcon,tone]=DL_ACTIONS[action];
  return `<tr class="dlrow${dlSel.has(v.id)?' picked':''}" data-key="${esc(v.id)}" data-state="${esc(v.state)}">
    <td><input type="checkbox" class="dlck" data-id="${esc(v.id)}" ${dlSel.has(v.id)?'checked':''} aria-label="Select ${esc(name)}"></td>
    <td class="dl-file"><span class="dlic">${dlIcon(v.kind==='torrent'?'Globe':'File')}</span><div><button class="dl-name" data-dm="props" data-id="${esc(v.id)}">${esc(name)}</button><span class="dl-meta">${esc(host|| (v.kind==='torrent'?'Torrent':v.category||'Download'))}${v.queue?' · '+esc(dlQueueName(v.queue)):''}</span></div></td>
    <td class="dl-progress-cell"><div class="dl-status ${v.state==='error'||v.verify==='bad'?'bad':v.state==='done'?'good':''}">${dlIcon(icon)}<span>${esc(state)}</span><span class="dl-percent">${known?Math.floor(pct)+'%':''}</span></div>
      ${known?`<div class="dlprog" role="progressbar" aria-label="Progress for ${esc(name)}" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${Math.floor(pct)}"><div style="width:${pct}%"></div></div>`:'<span class="dl-meta">Size unknown</span>'}
      <span class="dl-meta dl-detail">${esc(v.verify==='bad'?'Checksum does not match. Do not open this file.':v.verify==='checking'?'Checking checksum…':v.verify==='ok'?'Checksum matches':v.note||v.err||'')}</span></td>
    <td class="dl-size">${bytes(v.done||0)}<span class="dl-meta">${known?'of '+bytes(v.size):'total unknown'}</span></td>
    <td class="dl-speed">${v.state==='downloading'&&Number.isFinite(v.speed)?bytes(v.speed)+'/s':'—'}<span class="dl-meta">${v.state==='downloading'&&known&&v.speed>0?dlDuration(Math.max(0,(v.size-(v.done||0))/v.speed))+' left':'—'}</span></td>
    <td><div class="dl-actions">${dlTool(actionIcon,actionLabel+' '+name,`data-dl="${action}" data-id="${esc(v.id)}"`,tone,action==='open'?'Open file':actionLabel+' download')}${dlTool('MoreHorizontal','Details and actions for '+name,`data-dm="props" data-id="${esc(v.id)}"`,'neutral','Details and actions')}</div></td></tr>`;
}
const LIM_OPTS=[0,0.5,1,2,5,10,20,50];
function limSel(key, cur=0) {
  const value=cur/1048576, opts=[...new Set([...LIM_OPTS,value])].sort((a,b)=>a-b);
  return `<select class="sel" data-cfg="${esc(key)}" aria-label="${esc(key.startsWith('line:')?'Speed limit for '+labelOf(key.slice(5)):key==='uploadLimit'?'Torrent upload speed limit':'Overall download speed limit')}">${opts.map(n=>`<option value="${n}"${n===value?' selected':''}>${n?n+' MB/s':'No limit'}</option>`).join('')}</select>`;
}
// Patch existing nodes, retaining focus, text selection, disclosures and unsaved form values.
function dlMorph(root, html) {
  const fresh=document.createElement(root.tagName); fresh.innerHTML=html;
  function patch(old,next) {
    if (old.nodeType!==next.nodeType||old.nodeName!==next.nodeName) {const replacement=next.cloneNode(true);old.replaceWith(replacement);return replacement;}
    if (old.nodeType===3) {if(old.nodeValue!==next.nodeValue)old.nodeValue=next.nodeValue;return old;}
    const protectedField=old===document.activeElement||old.hasAttribute('data-dirty');
    const skip=name=>(protectedField&&['value','checked','selected','data-dirty'].includes(name))||(old.tagName==='DETAILS'&&name==='open');
    for(const a of [...old.attributes]) if(!next.hasAttribute(a.name)&&!skip(a.name))old.removeAttribute(a.name);
    for(const a of [...next.attributes]) if(!skip(a.name)&&old.getAttribute(a.name)!==a.value)old.setAttribute(a.name,a.value);
    if(old.matches('input,textarea,select')&&protectedField){if(old.classList.contains('dlck'))old.checked=next.checked;return old;}
    children(old,next);
    if(old.matches('input,textarea,select')) {old.value=next.value;if(old.type==='checkbox')old.checked=next.checked;}
    return old;
  }
  function children(old,next) {
    const key=n=>n.nodeType===1?(n.getAttribute('data-key')||n.id):'';
    const keyed=new Map([...old.childNodes].filter(key).map(n=>[key(n),n]));
    let cursor=old.firstChild;
    for(const n of [...next.childNodes]) {
      let match=key(n)?keyed.get(key(n)):cursor;
      if(match&&key(match)!==key(n))match=null;
      if(!match){match=n.cloneNode(true);old.insertBefore(match,cursor);}
      else {if(match!==cursor)old.insertBefore(match,cursor);match=patch(match,n);}
      cursor=match.nextSibling;
    }
    while(cursor){const next=cursor.nextSibling;cursor.remove();cursor=next;}
  }
  children(root,fresh);
}
function dlShell() {
  const box=$('#dlBody'); if (!box) return null;
  box.classList.add('nm-downloads');
  if (!box.querySelector('.dl-tabs')) box.innerHTML=`<nav class="dl-tabs" aria-label="Downloads sections">${['transfers','queues','browser','settings'].map(t=>`<a href="#/downloads/${t}" data-dl-tab="${t}">${t[0].toUpperCase()+t.slice(1)}</a>`).join('')}</nav><div id="dlBanners"></div>${['transfers','queues','browser','settings'].map(t=>`<section id="dlPanel-${t}" class="dl-panel" aria-label="${t}" hidden></section>`).join('')}`;
  return box;
}
window.nmDownloadsTab=function(tab) {
  dlTab=['transfers','queues','browser','settings'].includes(tab)?tab:'transfers';
  paintDL();
};
function dlTransfers() {
  const items=DL.items, result=dlPageItems(); dlVisIds=result.items.map(v=>v.id);
  const available=new Set(items.map(v=>v.id));for(const id of dlSel)if(!available.has(id))dlSel.delete(id);
  const count=state=>items.filter(v=>v.state===state).length;
  const chip=(id,label,n)=>`<button class="btn ${dlFilter===id?'selected':''}" data-f="${id}" aria-pressed="${dlFilter===id}">${label} <span>${n}</span></button>`;
  return `<div class="dl-heading"><div><h2>Transfers</h2><p><strong>${Number.isFinite(DL.speed)?bytes(DL.speed)+'/s':'—'}</strong> · ${count('downloading')} active · ${count('queued')} waiting</p></div><button class="btn pri" id="dlNew">${dlIcon('Plus')} Add download</button></div>
    <div class="dl-toolbar"><label class="dl-search">${dlIcon('Search')}<span class="dl-sr">Search downloads</span><input class="inp" id="dlSearch" type="search" placeholder="Search downloads" value="${esc(dlQ)}"></label><select class="sel" id="dlCat" aria-label="File type"><option value="">All types</option>${DL_CATS.map(c=>`<option${c===dlCat?' selected':''}>${c}</option>`).join('')}</select>${dlSortSelect()}<details class="dl-menu">${dlTool('MoreHorizontal','List actions','','neutral','List actions','summary')}<div class="dl-menu-body"><button class="btn dl-tone-warn" data-dl="pauseAll">${dlIcon('Pause')} Pause all</button><button class="btn dl-tone-good" data-dl="resumeAll">${dlIcon('Play')} Resume all</button><button class="btn dl-tone-bad" data-dl="clear" ${count('done')?'':'disabled'}>${dlIcon('Trash2')} Clear finished</button><button class="btn dl-tone-action" data-dm="grab">${dlIcon('Link')} Find links on a page</button><button class="btn dl-tone-action" data-dm="export">${dlIcon('Download')} Export list</button><button class="btn dl-tone-action" data-dm="import">${dlIcon('Upload')} Import list</button></div></details></div>
    <div class="dl-filters" aria-label="Transfer status">${chip('all','All',items.length)}${chip('active','In progress',count('downloading')+count('queued')+count('paused'))}${chip('done','Finished',count('done'))}${chip('bad','Needs attention',count('error'))}</div>
    <div class="dl-bulk" role="group" aria-label="Selected downloads" ${dlSel.size?'':'hidden'}><div class="dl-selection-count"><strong>${dlSel.size} selected</strong><span>${result.items.length} visible</span></div><div class="dl-bulk-actions" role="group" aria-label="Transfer actions">${dlTool('Pause','Pause selected downloads','data-bulk="pause"','warn','Pause selected')}${dlTool('Play','Resume selected downloads','data-bulk="resume"','good','Resume selected')}${dlTool('Trash2','Remove selected downloads','data-bulk="remove"','bad','Remove selected')}</div><label class="dl-queue-picker">${dlIcon('ListPlus')}<span class="dl-sr">Move selection to queue</span><select class="sel" id="dlBulkQ"><option value="__choose__" selected>Move to queue…</option>${dlQueueOpts('__choose__')}</select></label><div class="dl-selection-actions">${dlTool('ListChecks','Select this page','data-bulk="all"','action')}${dlTool('X','Clear selection','data-bulk="none"')}</div></div>
    <div class="dl-table-wrap"><table class="dl-table"><caption class="dl-sr">Downloads. Select a filename for details and all actions.</caption><thead><tr><th scope="col"><span class="dl-sr">Select</span></th><th scope="col">File</th><th scope="col">Progress</th><th scope="col">Size</th><th scope="col">Speed / ETA</th><th scope="col"><span class="dl-sr">Actions</span></th></tr></thead><tbody>${result.items.map(dlRow).join('')}</tbody></table><div class="dlempty" ${result.total?'hidden':''}>${dlIcon('Download')}<h3>${items.length?'No matching downloads':'Your downloads will appear here'}</h3><p>${items.length?'Try another search or clear your filters.':'Add a file link, magnet link or torrent to get started.'}</p><button class="btn" id="${items.length?'dlResetFilters':'dlEmptyAdd'}">${items.length?'Clear filters':'Add download'}</button></div></div>
    <div class="dl-pagination"><span>${result.total?dlPage*100+1:0}–${Math.min((dlPage+1)*100,result.total)} of ${result.total} downloads</span><div>${dlTool('ChevronLeft','Previous page',`data-page="-1" ${dlPage?'':'disabled'}`)}<span>Page ${dlPage+1} of ${result.pages}</span>${dlTool('ChevronRight','Next page',`data-page="1" ${dlPage+1<result.pages?'':'disabled'}`)}</div></div>`;
}
const dlSetting=(title,hint,control)=>`<div class="dnsrow"><div><b>${title}</b><div class="d">${hint}</div></div><div class="dl-control">${control}</div></div>`;
const dlToggle=(id,on,label,extra='')=>`<button class="tog${on?' on':''}" ${id?`id="${id}"`:''} ${extra} role="switch" aria-checked="${!!on}" aria-label="${esc(label)}"><span></span></button>`;
const dlNumber=(id,values,value,label)=>`<select class="sel" id="${id}" aria-label="${label}">${[...new Set([...values,value])].filter(x=>x!==undefined).map(x=>`<option ${x===value?'selected':''}>${x}</option>`).join('')}</select>`;
function dlSettings() {
  const c=DL.config;
  const group=(id,title,html)=>`<section class="dl-settings-group" id="dlSettings-${id}" data-key="${id}"><h3>${title}</h3>${html}</section>`;
  return `<div class="dl-heading"><div><h2>Download settings</h2><p>Folders, limits and automation for your downloads.</p></div></div><nav class="dl-jumps" aria-label="Settings groups">${[['files','Files & folders'],['speed','Speed & connections'],['schedule','Schedule & calls'],['torrent','Torrents & streams'],['automation','Automation'],['tools','Lists & tools']].map(([id,t])=>`<button class="btn" data-jump="${id}">${t}</button>`).join('')}</nav>
  ${group('files','Files & folders',
    dlSetting('Save downloads in','The default folder for new downloads.',`<input class="inp" id="dlDir" aria-label="Download folder" value="${esc(c.dir||'')}"><button class="btn" id="dlDirSave">Save</button>`)+
    dlSetting('Sort into folders by type','Videos, Programs, Documents and other file types.',dlToggle('dlCatTog',c.useCategories,'Sort into folders'))+
    dlSetting('Folders for types and websites','Choose a different destination for each type or website.','<button class="btn" data-dm="folders">Choose folders</button>')+
    dlSetting('If a file already exists','Keep both, replace the old file, or skip the new download.',`<select class="sel" data-cfg="onExists" aria-label="If a file already exists">${[['rename','Keep both'],['overwrite','Replace the old file'],['skip','Skip the download']].map(([v,t])=>`<option value="${v}" ${(c.onExists||'rename')===v?'selected':''}>${t}</option>`).join('')}</select>`))}
  ${group('speed','Speed & connections',
    dlSetting('Downloads at the same time','Recommended: 3.',dlNumber('dlMax',[1,2,3,4,5,6,8,10],c.maxActive,'Simultaneous downloads'))+
    dlSetting('Connections per line','Sites may reduce this automatically. Recommended: 8.',dlNumber('dlPer',[2,4,6,8,12,16],c.maxPerLine,'Connections per line'))+
    dlSetting('Overall speed limit','Keep bandwidth available for other activities.',limSel('limitAll',c.limitAll))+
    DL.lines.map(l=>dlSetting(esc(labelOf(l.name)),'Use this connection and optionally limit its speed.',limSel('line:'+l.name,c.limitLines?.[l.name]||0)+dlToggle('',!(c.excludeLines||[]).includes(l.name),'Use '+labelOf(l.name)+' for downloads',`data-ex="${esc(l.name)}"`))).join('')+
    dlSetting('Use the mobile line','When off, mobile is still used if it is the only connection.',dlToggle('dlMobTog',c.allowMobile,'Use mobile line')))}
  ${group('schedule','Schedule & calls',
    dlSetting('Download hours','Outside these hours, downloads wait.',`<input type="time" class="inp" id="dlWinFrom" value="${esc(c.window.from||'22:00')}" aria-label="Download from"><input type="time" class="inp" id="dlWinTo" value="${esc(c.window.to||'07:00')}" aria-label="Download until">${dlToggle('dlWinTog',c.window.enabled,'Restrict download hours')}`)+
    dlSetting('During a call','Keeping the call’s line free requires merging to be on.',`<select class="sel" data-cfg="callMode" aria-label="During a call">${[['avoid','Keep the call’s line free'],['pause','Pause all downloads'],['off','Do nothing']].map(([v,t])=>`<option value="${v}" ${c.callMode===v?'selected':''}>${t}</option>`).join('')}</select>`)+
    dlSetting('When everything is finished','Runs once. Shutdown gives you one minute to cancel.',`<select class="sel" data-cfg="afterAll" aria-label="When everything is finished">${[['','Nothing'],['sleep','Put the PC to sleep'],['shutdown','Shut the PC down']].map(([v,t])=>`<option value="${v}" ${(c.afterAll||'')===v?'selected':''}>${t}</option>`).join('')}</select>`)+
    dlSetting('Named queues','Set separate hours, days and concurrency.','<a class="btn" href="#/downloads/queues" data-dl-tab="queues">Open queues</a>'))}
  ${group('torrent','Torrents & streams',
    dlSetting('Torrents and magnet links','Allow .torrent files and magnet links.',dlToggle('',!c.noTorrents,'Allow torrents','data-dm="torTog"'))+
    dlSetting('Share while downloading','Send downloaded pieces to peers. Never uses the mobile line.',dlToggle('',!c.noUpload,'Share while downloading','data-dm="upTog"'))+
    dlSetting('Upload speed limit','For torrent uploads only.',limSel('uploadLimit',c.uploadLimit))+
    dlSetting('ffmpeg',DL.ffmpeg?'Found. Separate video and audio tracks are joined into one file.':'Not found. Separate video and audio tracks are saved as two files. Place ffmpeg next to NetMerge or in its tools folder.',`<span class="dl-status">${dlIcon(DL.ffmpeg?'CircleCheck':'CircleAlert')}${DL.ffmpeg?'Ready':'Not installed'}</span>`))}
  ${group('automation','Automation',
    dlSetting('Notify when a download finishes','Show a notification from NetMerge.',dlToggle('',!c.noNotify,'Notification when finished','data-dm="notifyTog"'))+
    dlSetting('Run a program after each download','Optional. Use <code>%f</code> for the file, for example <code>"C:\\Tools\\scan.exe" "%f"</code>.',`<input class="inp" id="dlAfterCmd" aria-label="Program to run after a download" value="${esc(c.afterCmd||'')}"><button class="btn" data-dm="afterSave">Save</button>`)+
    dlSetting('Learned website limits',`${DL.knownSites||0} sites remembered. Some websites accept only one address per file.`,`<button class="btn" id="dlForget" ${DL.knownSites?'':'disabled'}>Forget</button>`))}
  ${group('tools','Lists & tools',dlSetting('Download list','Import an exported list or a text file with one link per line.','<button class="btn" data-dm="export">Export list</button><button class="btn" data-dm="import">Import list</button>')+dlSetting('Find links on a web page','Filter by file type, then choose the links to download.','<button class="btn" data-dm="grab">Find links</button>')+'<p class="dl-help">Patterns: <code>photo[001-120].jpg</code>, <code>[a-f]</code>, or <code>[1-100:5]</code>. Up to 500 links per submission.</p>')}`;
}
function dlQueuesPanel() {
  const c=DL.config, qs=[{id:'',name:'Main queue',maxActive:c.maxActive,window:c.window,days:[]},...c.queues];
  return `<div class="dl-heading"><div><h2>Queues</h2><p>Organize downloads with their own hours and limits.</p></div><button class="btn pri" data-dm="queues">Manage queues</button></div><div class="dl-queue-list">${qs.map(q=>`<section class="dl-queue" data-key="${esc(q.id||'main')}"><div><h3>${esc(q.name)}</h3><p>${(q.window||{}).enabled?esc(q.window.from+'–'+q.window.to):'Any time'} · ${q.days?.length?q.days.map(d=>DL_DAYS[d]).join(', '):'Every day'}</p><span class="dl-meta">${DL.items.filter(v=>(v.queue||'')===q.id).length} downloads · ${q.maxActive?q.maxActive+' at once':'General concurrency limit'} · ${q.paused?'Paused':'Ready'}</span></div><div class="dl-actions">${dlTool('Play','Start '+q.name,`data-start-queue="${esc(q.id)}"`,'good','Start queue')}${q.id?dlTool('Pencil','Edit '+q.name,'data-dm="queues"','action','Edit queue'):dlTool('Settings2','Download settings','href="#/downloads/settings" data-dl-tab="settings"','neutral','Download settings','a')}</div></section>`).join('')}</div>`;
}
function dlBrowser() {
  const ext=DL.ext;
  return `<div class="dl-heading"><div><h2>Browser connection</h2><p>Send downloads from Chrome, Edge, Brave or Firefox.</p></div><button class="btn pri" id="dlExtCode">${dlIcon('Plus')} Connect a browser</button></div><section class="dl-settings-group"><h3>Download handoff</h3>${ext?dlSetting('Send downloads from the browser','Turn off to let browsers download by themselves.',dlToggle('dlExtOn',ext.enabled,'Send downloads from the browser'))+dlSetting('Minimum file size','Smaller downloads stay in the browser.',`<select class="sel" id="dlExtMin" aria-label="Minimum browser file size">${[...new Set([0,1,5,10,25,50,100,ext.minMB])].filter(n=>n!==undefined).map(n=>`<option value="${n}" ${ext.minMB===n?'selected':''}>${n?n+' MB':'Everything'}</option>`).join('')}</select>`):'<p>Browser connection information is unavailable. Refresh to try again.</p>'}</section><section class="dl-settings-group"><h3>Connected browsers</h3>${ext?.browsers.length?ext.browsers.map(b=>`<div class="dl-queue" data-key="${esc(b.id)}"><div><b>${esc(b.name)}</b><p>Connected ${new Date(b.created*1000).toLocaleDateString()}${b.lastSeen?' · Last used '+new Date(b.lastSeen*1000).toLocaleString():''}</p></div>${dlTool('Unplug','Disconnect '+b.name,`data-extrm="${esc(b.id)}"`,'bad','Disconnect browser')}</div>`).join(''):'<div class="dlempty">'+dlIcon('Globe')+'<h3>No browsers connected</h3><p>Connect a browser to send file links and detected media to NetMerge.</p></div>'}</section>`;
}
function paintDL() {
  if ((typeof page!=='undefined'&&page!=='downloads') || !DM_ON) return;
  const box=dlShell(); if(!box)return;
  if(!DL) {
    if(dlError)dlMorph($('#dlBanners'),`<div class="dl-banner bad"><span>${esc(dlError)}</span><button class="btn" id="dlRefresh">Retry refresh</button></div>`);
    return;
  }
  if(DL.enabled===false)return;
  const focused=box.contains(document.activeElement)?document.activeElement:null;
  const stale=dlLastGood&&Date.now()-dlLastGood>5000;
  dlMorph($('#dlBanners'),`${DL.powerAt&&Date.now()/1000-DL.powerAt<70?'<div class="dl-banner bad"><b>The PC is about to shut down because downloads finished.</b><button class="btn" data-dl="cancelPower">Cancel shutdown</button></div>':''}${dlError||stale?`<div class="dl-banner bad"><span>${esc(dlError||'Downloads may be out of date.')} Last updated ${dlLastGood?new Date(dlLastGood).toLocaleTimeString():'never'}.</span><button class="btn" id="dlRefresh">Retry refresh</button></div>`:''}${DL.hold?`<div class="dl-banner">${dlIcon('Clock3')}<span>${esc(DL.hold)}</span></div>`:!DL.lines.length?'<div class="dl-banner">'+dlIcon('CircleAlert')+'<span>No connection available. Downloads will start when a connection is available.</span></div>':''}`);
  for(const link of box.querySelectorAll('.dl-tabs a')) {if(link.dataset.dlTab===dlTab)link.setAttribute('aria-current','page');else link.removeAttribute('aria-current');}
  for(const tab of ['transfers','queues','browser','settings']) $('#dlPanel-'+tab).hidden=tab!==dlTab;
  dlMorph($('#dlPanel-'+dlTab),({transfers:dlTransfers,queues:dlQueuesPanel,browser:dlBrowser,settings:dlSettings})[dlTab]());
  if(focused&&!document.contains(focused))$('#dlSearch')?.focus();
  const details=$('#dlDlg [data-details-id]'), current=details&&DL.items.find(v=>v.id===details.dataset.detailsId);
  if(current){
    details.querySelector('[data-detail-state]').textContent=dlStateText(current)+(current.note?' · '+current.note:'');
    const control=details.querySelector('[data-dl=pause],[data-dl=resume],[data-dl=retry]');
    if(control&&current.state!=='done'){const action=current.state==='paused'?'resume':current.state==='error'?'retry':'pause';if(control.dataset.dl!==action){control.dataset.dl=action;control.className='btn dl-tone-'+DL_ACTIONS[action][1];control.innerHTML=dlIcon(DL_ACTIONS[action][0])+'<span>'+({pause:'Pause',resume:'Resume',retry:'Retry'}[action])+'</span>';}}
    if($('#pRelink')){$('#pRelink').disabled=current.state==='downloading';$('#pRelinkNote').textContent='Keeps downloaded data.'+(current.state==='downloading'?' Pause the download before replacing its link.':'');}
  }
  if(dlHintOwner)dlPositionHint();
}

let dlOpener=null, dlDialogSerial=0;
function dlFormValues(d) {
  return [...d.querySelectorAll('input:not([type=file]),textarea,select')].map(el=>el.type==='checkbox'?el.checked:el.value);
}
async function dlClose(force) {
  const d=$('#dlDlg'); if(!d)return;
  if(d.dlCancelConfirm){d.dlCancelConfirm();return;}
  if(d.dlPending>0){dlNotice('Finishing your request. You can close this dialog when it completes.');return;}
  if(force!==true && (d.dlDirty || !dlEqual(d.dlInitial,dlFormValues(d))) && !await dlAsk('Discard your unsaved changes?','Discard changes',true,'Discard'))return;
  dlDialogSerial++;
  d.close();d.remove();
  const opener=dlOpener;dlOpener=null;
  if(opener&&document.contains(opener))opener.focus();else $('#dlNew')?.focus();
}
function dlDialog(html) {
  let d=$('#dlDlg');
  if(!d) {
    dlOpener=document.activeElement;d=document.createElement('dialog');d.id='dlDlg';d.className='nm-download-dialog';document.body.appendChild(d);
    d.addEventListener('cancel',e=>{e.preventDefault();dlClose();});
    d.addEventListener('input',()=>{d.dlDirty=true;});
    d.addEventListener('change',()=>{d.dlDirty=true;});
  }
  const hadDirty=d.dlDirty;
  d.innerHTML=html;
  const card=d.querySelector('.card');
  if(card) {
    d.setAttribute('aria-label',card.getAttribute('aria-label')||card.querySelector('h3')?.textContent||'Download');
    card.removeAttribute('role');card.removeAttribute('aria-modal');card.setAttribute('tabindex','-1');
    const close=document.createElement('button');close.className='btn dl-dialog-close';close.setAttribute('aria-label','Close dialog');close.innerHTML=dlIcon('X');close.onclick=()=>dlClose();card.prepend(close);
  }
  d.dlInitial=dlFormValues(d);d.dlDirty=hadDirty||false;
  if(!d.open)d.showModal();
  const first=d.querySelector('input:not([type=hidden]),textarea,select');(first||card)?.focus();
  dlDialogSerial++;
}
function dlAddDialog(text='') {
  dlDialog(`<div class="card" aria-label="Add download"><h3>Add download</h3><p class="hint2">Add a file link, magnet link, or torrent. Multiple links go on separate lines.</p><label class="dl-field">Links<textarea class="inp" id="dlUrl" rows="4" spellcheck="false" placeholder="https://example.com/file.zip">${esc(text)}</textarea></label><div class="row"><button class="btn" id="dlPaste">Paste</button><button class="btn" id="dlTorBtn">Choose torrent file</button><input type="file" id="dlTorFile" accept=".torrent,application/x-bittorrent" hidden><button class="btn" data-dm="options">More options</button></div><p class="hint2">Numbered series: <code>photo[001-120].jpg</code>. Up to 500 links per submission.</p><div class="dl-dialog-actions"><button class="btn" id="dlCancel">Cancel</button><button class="btn" id="dlAddLater">Add to queue</button><button class="btn pri" id="dlAddBtn">Continue</button></div></div>`);
  $('#dlCancel').onclick=dlClose;
}

async function dlExtCode() {
  const opener=document.activeElement; if(opener?.tagName==='BUTTON')opener.disabled=true;
  let r;try{r=await dlRequest({action:'extCode'});}catch(e){dlNotice(e.message,true);return;}finally{if(opener?.tagName==='BUTTON')opener.disabled=false;}
  const base=/^[\w.-]+\/[\w.-]+$/.test(r.repo||'')?`https://github.com/${r.repo}/releases/latest/download/`:'';
  const file=(name,label)=>base?`<a class="btn" href="${esc(base+name)}" target="_blank" rel="noopener noreferrer">${label}</a>`:'';
  dlDialog(`<div class="card" aria-label="Connect a browser"><h3>Connect a browser</h3><p class="hint2">Install the extension, then enter this one-use code. The extension is installed manually.</p><ol class="dl-pair-steps"><li><b>Download the extension</b><div class="row" style="margin-top:8px">${file('NetMerge-extension-chrome-edge.zip','Chrome / Edge / Brave')+file('NetMerge-extension-firefox.zip','Firefox')||'Get NetMerge-extension-chrome-edge.zip or NetMerge-extension-firefox.zip from the NetMerge release page.'}</div></li><li><b>Chrome, Edge, Brave</b><p>Unzip it into a folder you will keep. Open <code>chrome://extensions</code> (Edge: <code>edge://extensions</code>), switch on Developer mode, choose Load unpacked and select the folder.</p><button class="btn" id="dlCopyExt">Copy Chrome extensions address</button><p><b>Firefox:</b> open <code>about:debugging#/runtime/this-firefox</code>, choose Load Temporary Add-on and select the ZIP. Firefox forgets this temporary installation when it closes.</p></li><li><b>Enter the code</b><p>Pin NetMerge using the puzzle-piece menu, open the extension and enter:</p></li></ol><div class="dl-pair-code" id="dlExtCodeBox">${esc(r.code)}</div><p class="hint2" id="dlExtLeft"></p><div class="row"><button class="btn" id="dlCopyCode">Copy code</button><button class="btn" id="dlNewCode" hidden>Generate new code</button></div><p class="hint2">The Browser tab controls which file sizes are handed off. Right-click a link and choose Download with NetMerge to send it manually. When NetMerge is closed, your browser downloads as usual.</p><div class="dl-dialog-actions"><button class="btn pri" id="dlCancel">Done</button></div></div>`);
  $('#dlCancel').onclick=dlClose;
  $('#dlCopyExt').onclick=async()=>{try{await navigator.clipboard.writeText('chrome://extensions');dlNotice('Address copied. Paste it into a new browser tab.');}catch(_){dlNotice('Copy chrome://extensions from the instructions above.',true);}};
  $('#dlCopyCode').onclick=async()=>{try{await navigator.clipboard.writeText(r.code);dlNotice('Pairing code copied.');}catch(_){dlNotice('Copy the pairing code shown above, then paste it into the extension.',true);}};
  $('#dlNewCode').onclick=dlExtCode;
  const token=dlDialogSerial;
  const tick=()=>{if(token!==dlDialogSerial||!$('#dlExtLeft'))return;const seconds=r.expires-Math.floor(Date.now()/1000);if(seconds<=0){$('#dlExtLeft').textContent='This code has expired. Generate a new code to connect.';$('#dlCopyCode').disabled=true;$('#dlNewCode').hidden=false;return;}$('#dlExtLeft').textContent=`Valid for ${Math.floor(seconds/60)}:${String(seconds%60).padStart(2,'0')}, one use only.`;setTimeout(tick,1000);};tick();
}

function dlAddResult(r, requested) {
  if (r.duplicate) return 'Already in your list. No second copy was added.';
  const added=(r.added||[]).length, skipped=r.skipped||0;
  const rejected=requested==null?null:Math.max(0,requested-added-skipped);
  return `${added} downloads added${skipped?`, ${skipped} already in your list`:''}${rejected?`, ${rejected} could not be added`:''}`;
}
async function dlSubmit(start=true) {
  const ta=$('#dlUrl'); if(!ta)return;
  const links=dlRowsOf(ta.value);
  if(!links.length){dlNotice('Paste a link first.',true);ta.focus();return;}
  if(links.length>1||/^magnet:|\.torrent(\?|#|$)/i.test(links[0])) {
    const r=await dlAct({action:'add',urls:links,start});
    if(r){const msg=dlAddResult(r,links.some(s=>/\[/.test(s))?null:links.length);await dlClose(true);toast(msg,!!r.duplicate);}
    return;
  }
  const token=dlDialogSerial;const button=$('#dlAddBtn');button.disabled=true;button.textContent='Checking…';
  try {const p=await dlRequest({action:'probe',url:links[0]});if(token===dlDialogSerial){dlShowProbe(links[0],p);if(!start)$('#dlLater').focus();}}
  catch(e){if(token===dlDialogSerial)dlNotice(e.message||'Could not check this link. Try More options to add it directly.',true);}
  finally{if(button.isConnected){button.disabled=false;button.textContent='Continue';}}
}
function dlShowProbe(link, p){
  const info = p.info || {};
  const fine = p.ok;
  const hls = fine && p.hls;
  const can = hls ? p.lines > 1 : fine && info.ranges && info.size > 0 && p.lines > 1 && !p.locked;
  const audioNote = hls && p.hls.audio ? (p.mux ? ' The sound is a separate track and is joined to the picture with ffmpeg.' : ' The sound is a separate track. ffmpeg was not found on this PC, so you get two files (picture and sound).') : '';
  const why = hls ? (p.lines > 1 ? `The video comes in ${p.hls.segments} parts. They are downloaded over all ${p.lines} connections and joined into one file.` : `The video comes in ${p.hls.segments} parts and is downloaded over your one connection.`) : !fine ? '' : !info.ranges ? 'This website does not allow splitting the file, so one connection is used.' : info.size <= 0 ? 'The size is not known, so one connection is used.' : p.lines < 2 ? 'Only one connection is available right now.' : p.locked ? 'This website only accepts one address per file, so one line is used.' : `The file can be split over all ${p.lines} connections.`;
  dlDialog(`<div class="card" role="dialog" aria-modal="true" aria-label="Add download" style="max-width:560px"><h3 style="margin:0">Add this download</h3>
    ${fine ? '' : `<div class="note bad" style="margin:12px 0">${esc(p.error || 'The link could not be checked.')} You can still add it.</div>`}
    ${info.warning === 'html' ? '<div class="note warn" style="margin:12px 0"><b>This looks like a web page, not a file.</b> The site may want you to sign in first. Download the link from your browser instead if it fails.</div>' : ''}
    <div style="margin:12px 0;display:grid;gap:10px"><label class="hint2">Name<input class="inp" id="dlName" value="${esc(info.name || '')}" style="width:100%;margin-top:4px"></label>
    <label class="hint2">Save in<input class="inp" id="dlFolder" value="${esc(p.dir || '')}" style="width:100%;margin-top:4px"></label>
    <details class="hint2"><summary style="cursor:pointer">More options</summary><div style="display:grid;gap:10px;margin-top:10px">
      <label>Start at (leave empty to start now)<input type="time" class="inp" id="dlTime" style="width:100%;margin-top:4px"></label>
      <label>Checksum, to check the file is not damaged (SHA-256, SHA-1 or MD5 from the website)<input class="inp" id="dlHash" placeholder="Optional" style="width:100%;margin-top:4px"></label>
      ${dlExtraAddFields('dl')}
      ${(p.lineNames||[]).length > 1 ? `<div>Use these connections<div style="display:flex;gap:14px;flex-wrap:wrap;margin-top:6px">${p.lineNames.map(n => `<label style="display:flex;gap:6px;align-items:center"><input type="checkbox" class="dlLn" value="${esc(n)}" checked> ${esc(labelOf(n))}</label>`).join('')}</div></div>` : ''}
    </div></details></div>
    ${p.duplicate ? (dlDupStopped(p) ? '<div class="note warn" style="margin:0 0 10px"><b>This link is already in your list but stopped.</b> Downloading it now restarts that copy (with its name and folder).</div>' : '<div class="note warn" style="margin:0 0 10px"><b>This link is already in your list.</b> Adding it again makes a second copy.</div>') : ''}
    ${fine ? `<div class="hint2" style="margin-bottom:6px">${hls ? '<b>Video stream</b> · about ' + (p.hls.seconds < 90 ? Math.round(p.hls.seconds) + ' sec' : Math.round(p.hls.seconds/60) + ' min') + (p.hls.quality ? ' · ' + esc(p.hls.quality) : '') + ' · ' : (info.size > 0 ? '<b>' + bytes(info.size) + '</b> · ' : '')}${hls ? '' : esc(info.type || 'unknown type')}</div>${hls && (p.hls.heights||[]).length > 1 ? `<label class="hint2" style="display:block;margin:6px 0">Picture quality <select class="sel" id="dlQual" style="min-width:0;margin-left:6px"><option value="0"${!p.height?' selected':''}>Best (${p.hls.heights[0]}p)</option>${p.hls.heights.slice(1).map(h => `<option value="${h}"${p.height===h?' selected':''}>${h}p</option>`).join('')}</select></label>` : ''}<div class="hint2" style="${can?'color:var(--good)':''}">${esc(why + audioNote)}</div>` : ''}
    <div class="row" style="justify-content:flex-end;gap:8px;margin-top:16px"><button class="btn" id="dlCancel">Cancel</button><button class="btn" id="dlLater">Add, start later</button><button class="btn pri" id="dlGo">Download now</button></div></div>`);
  if ($('#dlQual')) $('#dlQual').onchange = async e => {
    const h=+e.target.value, token=dlDialogSerial;
    const saved=[...$('#dlDlg').querySelectorAll('input,textarea,select')].filter(el=>el.id!=='dlQual').map(el=>({id:el.id,cls:el.className,value:el.value,checked:el.checked}));
    const open=[...$('#dlDlg').querySelectorAll('details')].map(el=>el.open);
    e.target.disabled=true;
    try { const q=await dlRequest({action:'probe',url:link,height:h,header:dlReadHeaders('dlHeader')});if(token!==dlDialogSerial)return;q.height=h;dlShowProbe(link,q);
      for(const savedEl of saved){const el=savedEl.id?document.getElementById(savedEl.id):[...$('#dlDlg').querySelectorAll('input')].find(x=>x.className===savedEl.cls&&x.value===savedEl.value);if(el){el.value=savedEl.value;el.checked=savedEl.checked;}}
      [...$('#dlDlg').querySelectorAll('details')].forEach((el,i)=>el.open=open[i]);$('#dlQual').focus();
    } catch(err){if(token===dlDialogSerial){e.target.disabled=false;dlNotice(err.message,true);}}
  };
  $('#dlGo').onclick = () => dlConfirm(link, p, true);
  $('#dlLater').onclick = () => dlConfirm(link, p, false);
  $('#dlCancel').onclick = dlClose;

}
function dlDupStopped(p){ return p.dupState === 'error' || p.dupState === 'paused'; }
async function dlConfirm(link, p, start){
  const name = $('#dlName').value.trim(), folder = $('#dlFolder').value.trim();
  const body = {action:'add', url: link, start};
  if (name && name !== (p.info||{}).name) body.name = name; // only a name the person changed is kept over the server's
  if (folder && folder !== p.dir) body.dir = folder;
  if (p.duplicate && !dlDupStopped(p)) body.force = true;
  if (p.height) body.height = p.height;
  const tm = $('#dlTime') && $('#dlTime').value;
  if (tm) { const [h, mi] = tm.split(':').map(Number); const d = new Date(); d.setHours(h, mi, 0, 0); if (d.getTime() <= Date.now()) d.setDate(d.getDate() + 1); body.startAt = Math.floor(d.getTime()/1000); }
  const hs = $('#dlHash') && $('#dlHash').value.trim(); if (hs) body.hash = hs;
  const boxes = $$('.dlLn'); if (boxes.length) { const on = boxes.filter(b => b.checked).map(b => b.value); if (!on.length) return dlNotice('Choose at least one connection', true); if (on.length < boxes.length) body.lines = on; }
  try {dlReadExtraAdd(body,'dl');} catch(e){return dlNotice(e.message,true);}
  const r = await dlAct(body);
  if (r) { dlClose(true); toast(dlAddResult(r,1),!!r.duplicate); }
}
document.addEventListener('keydown', e => { if (e.key === 'Enter' && (e.ctrlKey||e.metaKey) && e.target.id === 'dlUrl') { e.preventDefault(); dlSubmit(); } });
document.addEventListener('change', async e => {
  const t = e.target;
  if (t.classList && t.classList.contains('dlck')) { t.checked ? dlSel.add(t.dataset.id) : dlSel.delete(t.dataset.id); return paintDL(); }
  if (t.id === 'dlBulkQ' && t.value !== '__choose__') { const q=t.value,ids=[...dlSel];let bad=0;t.disabled=true;for(const id of ids)if(await dlAct({action:'setQueue',id,queue:q})===null)bad++;t.disabled=false;t.value='__choose__';toast(`${ids.length-bad} moved${bad?`, ${bad} could not be changed`:''}`,!!bad); }
});
async function dlBulk(a){
  if (a === 'all') { dlVisIds.forEach(id => dlSel.add(id)); return paintDL(); }
  if (a === 'none') { dlSel.clear(); return paintDL(); }
  const ids = [...dlSel]; if (!ids.length) return;
  if (a === 'remove') {
    const loose = ids.filter(id => { const v = DL.items.find(x => x.id === id); return v && v.state !== 'done'; }).length;
    if (!await dlAsk(`Remove ${ids.length} download${ids.length===1?'':'s'} from the list?` + (loose ? `\n\n${loose} of them are not finished: what was downloaded so far will be deleted. Finished files stay on your disk.` : '\n\nFinished files stay on your disk.'))) return;
  }
  let bad = 0;
  for (const id of ids) { const r = await dlAct({action: a, id}); if (r === null) bad++; }
  dlSel.clear(); paintDL();
  toast(bad ? `${ids.length - bad} done, ${bad} could not be changed` : `${ids.length} ${a === 'remove' ? 'removed' : a === 'pause' ? 'paused' : 'resumed'}`, !!bad);
}
document.addEventListener('keydown', e => {
  if (typeof page === 'undefined' || page !== 'downloads' || $('dialog[open]')) return;
  const tag = (e.target.tagName || '').toLowerCase(); if (tag === 'input' || tag === 'textarea' || tag === 'select' || e.target.isContentEditable || dlTab !== 'transfers') return;
  const menu=$('.dl-menu[open]');
  if(e.key==='Escape'&&menu){e.preventDefault();menu.open=false;menu.querySelector('summary').focus();return;}
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'a') { e.preventDefault(); dlVisIds.forEach(id => dlSel.add(id)); paintDL(); }
  else if (e.key === 'Delete' && dlSel.size) { e.preventDefault(); dlBulk('remove'); }
  else if (e.key === 'Escape' && dlSel.size) { dlSel.clear(); paintDL(); }
});
document.addEventListener('paste', e => {
  if (typeof page === 'undefined' || page !== 'downloads' || $('dialog[open]')) return;
  const tag = (e.target.tagName || '').toLowerCase(); if (tag === 'input' || tag === 'textarea' || tag === 'select' || e.target.isContentEditable || !DM_ON) return;
  const txt = ((e.clipboardData && e.clipboardData.getData('text')) || '').trim();
  if (!/^(https?:|ftp:|magnet:)/i.test(txt) || txt.length > 20000) return;
  e.preventDefault(); dlAddDialog(txt);
});
document.addEventListener('click', async e => {
  const t = e.target.closest('button'); if (!t) return;
  if (t.dataset.bulk) return dlBulk(t.dataset.bulk);
  if (t.id === 'dlAddBtn') return dlSubmit(true);
  if (t.id === 'dlAddLater') return dlSubmit(false);
  if (t.id === 'dlTorBtn') { const f = $('#dlTorFile'); if (f) f.click(); return; }
  if (t.id === 'dlPaste') { try { const s = await navigator.clipboard.readText(); if (s) { $('#dlUrl').value = s.trim(); $('#dlUrl').focus(); } } catch(err){ dlNotice('Press Ctrl+V in the Links field instead.', true); $('#dlUrl')?.focus(); } return; }
  if (t.dataset.f && page === 'downloads') { dlFilter = t.dataset.f; return paintDL(); }
  if (t.id === 'dlDirSave') return void dlConfigEdit({dir: $('#dlDir').value.trim()}, 'Folder saved');
  if (t.id === 'dlCatTog') return void dlConfigEdit({useCategories: !DL.config.useCategories});
  if (t.id === 'dlWinTog') return void dlConfigEdit({window: {enabled: !DL.config.window.enabled}});
  if (t.dataset.ex !== undefined) { const ex = new Set(DL.config.excludeLines||[]); ex.has(t.dataset.ex) ? ex.delete(t.dataset.ex) : ex.add(t.dataset.ex); return void dlConfigEdit({excludeLines: [...ex]}); }
  if (t.id === 'dlForget') return void dlAct({action:'forgetHosts'}, 'Forgotten');
  if (t.id === 'dlExtCode') return dlExtCode();
  if (t.id === 'dlExtOn') return void dlAct({action:'extConfig', extOff: DL.ext.enabled});
  if (t.dataset.extrm) { if (!await dlAsk('Disconnect this browser? Its extension stops sending downloads until you connect it again.')) return; return void dlAct({action:'extRevoke', id: t.dataset.extrm}, 'Disconnected'); }
  if (t.id === 'dlMobTog') return void dlConfigEdit({allowMobile: !DL.config.allowMobile});
  const a = t.dataset.dl; if (!a) return;
  const id = t.dataset.id, v = DL && DL.items.find(x => x.id === id);
  if (a === 'delfile') { if (!await dlAsk('Delete "' + (v ? v.name : 'this file') + '" from your computer too? This cannot be undone.')) return; const r=await dlAct({action:'remove',id,deleteFiles:true},'File deleted');if(r)dlClose(true);return; }
  if (a === 'open' && v && DL_RISKY.test(v.name) && !await dlAsk('"' + v.name + '" is a program. Only open it if you trust where it came from. Open it?')) return;
  if (a === 'remove' && !await dlAsk(v && v.state !== 'done' ? 'Remove this unfinished download? Its partial file will be deleted.' : 'Remove this download from the list? The finished file stays on your computer.', 'Remove download', true, 'Remove')) return;
  if (a === 'clear' && !await dlAsk('Clear finished downloads from the list? Finished files stay on your computer.', 'Clear finished', false, 'Clear')) return;
  if (a === 'move') return void dlAct({action:'move', id, move: t.dataset.mv});
  if (a === 'cancelPower') return void dlAct({action:'cancelPower'}, 'Shutdown cancelled');
  const result=await dlAct({action:a,id});if(result&&a==='remove')dlClose(true);
});
async function dlAddTorrentFile(f){
  if (!f) return;
  if (f.size > 16e6) return dlNotice('That file is too big to be a torrent', true);
  let buf;try{buf=new Uint8Array(await f.arrayBuffer());}catch(e){return dlNotice('Could not read the torrent file. Choose it again.',true);}let bin = ''; for (let i = 0; i < buf.length; i += 32768) bin += String.fromCharCode.apply(null, buf.subarray(i, i + 32768));
  const torrent = btoa(bin);
  const token=dlDialogSerial;let p; try { p = await dlRequest({action:'torrentInfo', torrent}); } catch(e){ return dlNotice(e.message, true); }
  if(token!==dlDialogSerial)return;
  const fl = p.files || [];
  dlDialog(`<div class="card" role="dialog" aria-modal="true" aria-label="Add torrent" style="max-width:640px;max-height:92vh;overflow:auto"><h3 style="margin:0;word-break:break-all">${esc(p.name)}</h3>
    <div class="hint2" style="margin:4px 0 10px">${bytes(p.size)} · ${fl.length} file${fl.length===1?'':'s'}${p.private ? ' · private torrent (only its own tracker is used)' : ''}</div>
    ${p.duplicate ? '<div class="note warn" style="margin:0 0 10px"><b>This torrent is already in your list.</b></div>' : ''}
    ${fl.length > 1 ? `<div class="row" style="gap:10px;margin-bottom:6px"><label style="display:flex;gap:6px;align-items:center"><input type="checkbox" id="tpAll" checked> All files</label><span class="hint2" id="tpSum"></span></div>` : ''}
    <div class="scrollbox" style="max-height:260px;overflow:auto;font-size:13px">${fl.map((x,i) => `<label style="display:flex;gap:8px;align-items:center;padding:3px 0"><input type="checkbox" class="tpSel" data-i="${i}" checked> <span style="flex:1;word-break:break-all">${esc(x.name)}</span><span class="hint2">${bytes(x.size)}</span></label>`).join('')}</div>
    <label class="hint2" style="display:block;margin-top:10px">Save in<input class="inp" id="tpDir" value="${esc(p.dir)}" style="width:100%;margin-top:4px"></label>
    <div class="row" style="justify-content:flex-end;gap:8px;margin-top:16px"><button class="btn" id="dlCancel">Cancel</button><button class="btn" id="tpLater">Add, start later</button><button class="btn pri" id="tpGo">Download now</button></div></div>`);
  const sum = () => { const on = $$('.tpSel').filter(b => b.checked); const sz = on.reduce((a,b) => a + fl[+b.dataset.i].size, 0); if ($('#tpSum')) $('#tpSum').textContent = `${on.length} chosen · ${bytes(sz)}`; if ($('#tpAll')) $('#tpAll').checked = on.length === fl.length; };
  sum();
  $$('.tpSel').forEach(b => b.onchange = sum);
  if ($('#tpAll')) $('#tpAll').onchange = e => { $$('.tpSel').forEach(b => b.checked = e.target.checked); sum(); };
  const go = async start => {
    const sel = $$('.tpSel').map(b => b.checked); if (!sel.some(Boolean)) return dlNotice('Choose at least one file', true);
    const body = {action:'add', torrent, start, force: !!p.duplicate};
    if (!sel.every(Boolean)) body.select = sel;
    const dir = $('#tpDir').value.trim(); if (dir && dir !== p.dir) body.dir = dir;
    const r = await dlAct(body, start ? 'Torrent added' : 'Added'); if (r) dlClose(true);
  };
  $('#tpGo').onclick = () => go(true); $('#tpLater').onclick = () => go(false); $('#dlCancel').onclick = dlClose;
}
document.addEventListener('change', async e => {
  if (!DL) return;
  const t = e.target;
  if (t.id === 'dlTorFile') { const f = t.files && t.files[0]; t.value = ''; return void dlAddTorrentFile(f); }
  if(t.dataset&&t.dataset.lim){
    const dialog=t.closest('#dlDlg'),value=t.value;
    const index=dialog?[...dialog.querySelectorAll('input:not([type=file]),textarea,select')].indexOf(t):-1;
    if(await dlAct({action:'limit',id:t.dataset.lim,limitBytes:+value*1048576})!==null&&dialog&&index>=0){
      dialog.dlInitial[index]=value;dialog.dlDirty=!dlEqual(dialog.dlInitial,dlFormValues(dialog));
    }
    return;
  }
  if (t.dataset && t.dataset.cfg) {
    const k = t.dataset.cfg, c = {};
    if (k === 'limitAll') c.limitAll = +t.value * 1048576;
    else if (k === 'uploadLimit') c.uploadLimit = +t.value * 1048576;
    else if (k.startsWith('line:')) c.limitLines = {[k.slice(5)]: +t.value ? +t.value * 1048576 : undefined};
    else if (k === 'afterAll') { if (t.value === 'shutdown' && !await dlAsk('Shut the PC down when all downloads are finished? You get one minute to cancel.')) { t.value=DL.config.afterAll||'';delete t.dataset.dirty;paintDL();return; } c.afterAll = t.value; }
    else c[k] = t.value;
    return void dlConfigEdit(c);
  }
  if (t.id === 'dlWinFrom' || t.id === 'dlWinTo') return void dlConfigEdit({window: t.id==='dlWinFrom' ? {from: t.value||'22:00'} : {to: t.value||'07:00'}});
  if (t.id === 'dlExtMin') return void dlAct({action:'extConfig', extMinMB: +t.value});
  if (e.target.id === 'dlCat') { dlCat = e.target.value; paintDL(); }
  if (e.target.id === 'dlMax') dlConfigEdit({maxActive: +e.target.value});
  if (e.target.id === 'dlPer') dlConfigEdit({maxPerLine: +e.target.value});
});
document.addEventListener('input', e => { if (e.target.id === 'dlSearch') { dlQ = e.target.value; paintDL(); } });
window.addEventListener('dragover', e => { if (page === 'downloads' && DM_ON && e.dataTransfer && [...(e.dataTransfer.types||[])].some(t => t === 'text/uri-list' || t === 'text/plain' || t === 'Files')) e.preventDefault(); });
window.addEventListener('drop', async e => {
  const dt = e.dataTransfer; if (!dt || page !== 'downloads' || !DM_ON) return; e.preventDefault();
  if ($('dialog[open]')) return;
  let text = dt.getData('text/uri-list') || dt.getData('text/plain');
  if (dt.files && dt.files[0] && /\.torrent$/i.test(dt.files[0].name)) { go('downloads'); await loadDL(true); return void dlAddTorrentFile(dt.files[0]); }
  if (!text && dt.files && dt.files[0] && /\.(txt|csv|lst|m3u8?)$/i.test(dt.files[0].name) && dt.files[0].size < 1e6) text = await dt.files[0].text();
  const links = (text||'').split(/\s*\n\s*/).map(s => s.trim()).filter(s => /^(https?|ftp):\/\/|^magnet:/i.test(s));
  if (!links.length) return;
  await loadDL(true); dlAddDialog(links.join('\n'));
});

function renderDlCard(){
  const c = $('#dlCard'), d = S && S.downloads; if (!c) return;
  if (!d || (!d.active && !d.queued)) { c.hidden = true; return; }
  c.hidden = false;
  setHTML(c, `<div class="row" style="gap:12px"><div style="flex:1;min-width:0"><h3 style="margin:0 0 6px">Downloading now <span class="hint2" style="font-weight:400">${bytes(d.speed)}/s${d.queued ? ' · ' + d.queued + ' waiting' : ''}</span></h3>
    ${(d.items||[]).map(i => `<div class="hint2" style="margin-top:3px"><b style="color:var(--text)">${esc(i.name)}</b> · ${i.size>0?Math.round(i.done/i.size*100)+'%':bytes(i.done)} · ${bytes(i.speed)}/s</div>`).join('')}</div>
    <button class="btn" id="dlOpen">Open Downloads</button></div>`);
  $('#dlOpen').onclick = () => go('downloads');
}


// Local routes are links: normal browser Back/Forward and modified clicks remain available.
document.addEventListener('click', async e=>{
  for(const menu of $$('.dl-menu[open]'))if(!menu.contains(e.target)||e.target.closest('button'))menu.open=false;
  const link=e.target.closest('[data-dl-tab]');
  if(link&&!e.ctrlKey&&!e.metaKey&&!e.shiftKey&&!e.altKey) {e.preventDefault();location.hash='#/downloads/'+link.dataset.dlTab;nmDownloadsTab(link.dataset.dlTab);return;}
  const t=e.target.closest('button');if(!t||!DL)return;
  if(t.id==='dlNew'||t.id==='dlEmptyAdd')return dlAddDialog();
  if(t.id==='dlRefresh')return loadDL(true);
  if(t.dataset.page) {dlPage+=+t.dataset.page;paintDL();$('#dlSearch')?.focus();return;}
  if(t.id==='dlResetFilters') {dlFilter='all';dlCat='';dlQ='';dlQf='';dlPage=0;paintDL();return;}
  if(t.dataset.jump) {const target=$('#dlSettings-'+t.dataset.jump);target.setAttribute('tabindex','-1');target.focus();target.scrollIntoView({block:'start'});return;}
  if(t.dataset.startQueue!==undefined)return dlAct({action:'startQueue',queue:t.dataset.startQueue},'Queue started');
});
document.addEventListener('input',e=>{if(e.target.closest('#dlBody')&&e.target.matches('input:not([type=checkbox]),textarea,select')&&e.target.id!=='dlSearch')e.target.dataset.dirty='true';});
window.addEventListener('hashchange',()=>{if(/^#\/downloads(?:\/|$)/.test(location.hash)){nmDownloadsTab(location.hash.split('/')[2]);loadDL(true);}});
function dlStartPolling() {
  if(dlStopPolling)return;
  const visible=()=>typeof page!=='undefined'&&page==='downloads'&&DM_ON&&!document.hidden;
  if(window.NM?.schedule)dlStopPolling=NM.schedule('downloads',()=>loadDL(false),1000,visible);
  else {let stopped=false,timer;const tick=async()=>{if(stopped)return;if(visible())await loadDL(false);if(!stopped)timer=setTimeout(tick,1000);};timer=setTimeout(tick,1000);dlStopPolling=()=>{stopped=true;clearTimeout(timer);};}
}
window.nmDownloadsDispose=()=>{dlStopPolling?.();dlStopPolling=null;dlRevision++;};
window.addEventListener('pagehide',()=>nmDownloadsDispose());
window.addEventListener('pageshow',()=>dlStartPolling());
document.addEventListener('visibilitychange',()=>{if(!document.hidden&&page==='downloads'&&DM_ON)loadDL(true);});
dlStartPolling();
