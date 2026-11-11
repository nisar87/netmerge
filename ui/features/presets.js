/* ---------- presets ---------- */
let PR = null;
let prRead=null,prBusy=false,prNameDraft='',prNameRevision=0,prScheduleDraft=null,prScheduleQueued=false;
const prScheduleDirty=new Set(),prScheduleFields={prFrom:'from',prTo:'to',prSc:'preset'};
function prSyncSchedule(){
  const confirmed=PR.schedule||{enabled:false,from:'22:00',to:'06:00',preset:'maxspeed'};
  if(!prScheduleDraft)prScheduleDraft={...confirmed};
  else for(const key of Object.keys(confirmed))if(!prScheduleDirty.has(key))prScheduleDraft[key]=confirmed[key];
}
function prControls(){
  for(const button of $$('#prBody button'))button.disabled=prBusy||button.id==='prRestore'&&!PR.active||!!button.dataset.pa&&button.dataset.pa===PR.active;
}
async function loadPresets(act){
  if(prBusy||(!act&&prRead))return false;
  if(!act){
    const read=prRead=new AbortController();
    try{const next=await NM.request('presets',{signal:read.signal});if(prRead!==read)return;PR=next;NM.report('presets');paintPresets();return true;}
    catch(e){if(read.signal.aborted)return;if(page==='presets')NM.report('presets',e,()=>loadPresets());return false;}
    finally{if(prRead===read)prRead=null;}
  }
  const nameRevision=prNameRevision;
  prBusy=true;prRead?.abort();prRead=null;prControls();
  try{
    PR=await api('presets',act);
    if(act.action==='save'&&nameRevision===prNameRevision)prNameDraft='';
    if(act.action==='schedule')for(const key of Object.keys(act.schedule)){
      if(prScheduleDraft[key]===act.schedule[key])prScheduleDirty.delete(key);
    }
    NM.report('presets');
    await poll(true); // General settings follow the confirmed preset mutation.
    return true;
  }catch(e){if(page==='presets')NM.report('presets',e,()=>loadPresets());throw e;}
  finally{
    prBusy=false;paintPresets();
    if(prScheduleQueued){prScheduleQueued=false;void prSched(prScheduleDraft.enabled);}
  }
}
const PR_ICON = {work:'monitor',gaming:'zap',maxspeed:'gauge',travel:'smartphone',safe:'shield-check'};
const prIcon = id => `<span class="pc-ic">${NM.icon(PR_ICON[id]||'list-checks')}</span>`;
function paintPresets(){
  const box = $('#prBody'); if (!box || !PR) return;
  prSyncSchedule();const sc=prScheduleDraft;
  const active = PR.active || '';
  const def = `<div class="pc${!active?' on':''}">${prIcon('default').replace('list-checks','settings-2')}<h4>Default · my settings${!active?'<span class="badge">On</span>':''}</h4><p>Exactly the settings you had before using any preset (or what an earlier version of NetMerge kept). Nothing is changed.</p><div class="row"><button class="btn" id="prRestore"${!active?' disabled':''}>${!active?'Active':'Use this'}</button></div></div>`;
  const cards = def + PR.presets.map(p => `<div class="pc${p.id===active?' on':''}">${prIcon(p.id)}<h4>${esc(p.name)}${p.id===active?'<span class="badge">On</span>':''}</h4><p>${esc(p.desc)}</p><div class="row"><button class="btn" data-pa="${esc(p.id)}"${p.id===active?' disabled':''}>${p.id===active?'Active':'Use this'}</button>${p.builtin?'':`<button class="btn" data-pd="${esc(p.id)}" aria-label="Delete preset ${esc(p.name)}">Delete</button>`}</div></div>`).join('');
  const opts = PR.presets.map(p => `<option value="${esc(p.id)}"${p.id===sc.preset?' selected':''}>${esc(p.name)}</option>`).join('');
  const html = `<section class="card page-hero"><span class="hero-icon">${NM.icon(PR_ICON[active]||'settings-2')}</span><div class="hero-text"><h2>${active ? 'Preset on: ' + esc((PR.presets.find(p=>p.id===active)||{}).name || active) : 'Your own settings are on'}</h2><p class="hint2">A preset adds to your settings and can be undone at any time. Your own app rules always win over a preset.</p></div></section>
  <div class="pr">${cards}</div>
  <div class="card" style="margin-top:16px"><h3 style="margin:0 0 6px">Save your current settings as a preset</h3><div class="row" style="gap:8px;flex-wrap:wrap"><input class="inp" id="prName" value="${esc(prNameDraft)}" maxlength="40" placeholder="Name, for example Evening" aria-label="Preset name" style="max-width:260px"><button class="btn" id="prSave">Save</button></div></div>
  <div class="card" style="margin-top:16px"><h3 style="margin:0 0 6px">Schedule</h3><div class="hint2" style="margin-bottom:10px">Switch to a preset at set hours, then back to what you had. ${PR.scheduled ? '<b>The schedule is on right now.</b>' : ''}</div><div class="row" style="gap:8px;flex-wrap:wrap;align-items:center"><button class="tog${sc.enabled?' on':''}" id="prScOn" role="switch" aria-checked="${sc.enabled}" aria-label="Use a schedule"></button><span>From</span><input class="inp" type="time" id="prFrom" aria-label="Schedule start" value="${esc(sc.from)}" style="width:120px"><span>to</span><input class="inp" type="time" id="prTo" aria-label="Schedule end" value="${esc(sc.to)}" style="width:120px"><span>use</span><select class="sel" id="prSc" aria-label="Scheduled preset">${opts}</select></div></div>`;
  prControls();
  if (box.contains(document.activeElement) && ['INPUT','SELECT'].includes(document.activeElement.tagName)) return;
  setHTML(box, html);
  prControls();
}
async function prSched(on){
  if(!prScheduleDraft)return;
  prScheduleDraft.enabled=on;prScheduleDirty.add('enabled');
  // Inputs stay editable during a write; coalesce their next explicit changes.
  if(prBusy){prScheduleQueued=true;return;}
  const schedule={...prScheduleDraft};
  try{await loadPresets({action:'schedule',schedule});toast(on?'Schedule saved':'Schedule off');}
  catch(e){toast(e.message,true);}
}
document.addEventListener('input',e=>{
  if(e.target.id==='prName'){prNameDraft=e.target.value;prNameRevision++;}
  const field=prScheduleFields[e.target.id];
  if(field&&prScheduleDraft){prScheduleDraft[field]=e.target.value;prScheduleDirty.add(field);}
});
document.addEventListener('click', async e => {
  const t = e.target.closest('button'); if (!t) return;
  try {
    if (t.dataset.pa) { await loadPresets({action:'apply', id:t.dataset.pa}); toast('Preset is on'); }
    else if(t.dataset.pd){if(await NM.confirm({title:'Delete this preset?',message:'The saved preset will be removed. Current connection settings will remain active.',confirmLabel:'Delete preset',danger:true}))await loadPresets({action:'delete',id:t.dataset.pd});}
    else if (t.id === 'prRestore') { await loadPresets({action:'restore'}); toast('Back to your settings'); }
    else if (t.id === 'prSave') { await loadPresets({action:'save', name:prNameDraft}); toast('Preset saved'); }
    else if (t.id === 'prScOn') { await prSched(!prScheduleDraft.enabled); }
  } catch(err){ toast(err.message || 'Failed', true); }
});
document.addEventListener('change',e=>{
  const field=prScheduleFields[e.target.id];if(!field||!prScheduleDraft)return;
  prScheduleDraft[field]=e.target.value;prScheduleDirty.add(field);
  if(prScheduleDraft.enabled)void prSched(true);
});
NM.schedule('presets',()=>loadPresets(), 8000, ()=>page==='presets'&&!document.hidden);
