import {createClient} from './api.mjs';
import {createSettingsWriter,captureChanges,applyChanges} from './store.mjs';
import {normalizeState,contributingLinks} from './format.mjs';
import {createScheduler} from './polling.mjs';

const request = createClient();
// errors in the window go to NetMerge's log (at most a few: one broken screen must not flood it)
let reportedErrors = 0;
function reportError(kind, msg, src, line) {
  if (++reportedErrors > 20) return;
  try {
    fetch('/api/clientlog', {method:'POST', headers:{'X-NetMerge':'1','Content-Type':'application/json'},
      body:JSON.stringify({kind, msg:String(msg||'').slice(0,400), src:String(src||''), line:Number(line)||0})}).catch(()=>{});
  } catch {}
}
window.addEventListener('error', event => reportError('error', event.message || event.error, event.filename, event.lineno));
window.addEventListener('unhandledrejection', event => {
  const reason = event.reason;
  if (reason?.name === 'AbortError') return;
  reportError('promise', reason?.stack || reason?.message || reason, '', 0);
});
const scheduler = createScheduler();
const writer = createSettingsWriter({read:async()=> (await request('state',{fresh:true})).settings,write:body=>request('settings',{body})});
const icon = name => {
  const normalized=name.replace(/([a-z0-9])([A-Z])/g,'$1-$2').toLowerCase();
  const id=({moreHorizontal:'ellipsis','more-horizontal':'ellipsis',clock3:'clock-3'})[normalized]||normalized;
  return `<svg class="icon" aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><use href="assets/icons.svg#${/^[a-z0-9-]+$/.test(id)?id:'circle-help'}"></use></svg>`;
};
function icons(root=document){
  root.querySelectorAll('[data-icon]:not([data-icon-ready])').forEach(el=>{el.innerHTML=icon(el.dataset.icon);el.dataset.iconReady='true';});
  root.querySelectorAll('.scrollbox:not([tabindex]),.table-scroll:not([tabindex]),pre.log:not([tabindex])').forEach(el=>{el.tabIndex=0;el.setAttribute('role','region');el.setAttribute('aria-label',el.id==='logBox'?'Log output':'Scrollable data table');});
}
let dialogTail=Promise.resolve();
function confirm(options) {
  const operation=dialogTail.then(()=>new Promise(resolve=>{
    const dialog=document.querySelector('#confirmDialog');
    document.querySelector('#confirmTitle').textContent=options.title || 'Confirm change';
    document.querySelector('#confirmMessage').textContent=options.message || '';
    const accept=document.querySelector('#confirmAccept');
    accept.textContent=options.confirmLabel || 'Confirm';
    accept.className='btn '+(options.danger?'danger-solid':'pri');
    dialog.returnValue='cancel';
    dialog.addEventListener('close',()=>resolve(dialog.returnValue==='accept'),{once:true});
    dialog.showModal();
  }));
  dialogTail=operation.catch(()=>{});
  return operation;
}
const errors=new Map();
function report(key,error,retry) {
  const el=document.querySelector('#featureError');
  if(error)errors.set(key,{error,retry});else errors.delete(key);
  if(key!=='the application'&&window.NM.activeResource!==key)return;
  const entry=errors.get(key);
  el.hidden=!entry;
  el.replaceChildren();
  if(!entry)return;
  el.className='note bad';
  const message=document.createElement('span');message.textContent=`Could not refresh ${key}. ${error.message || error}`;
  el.append(message);
  if(retry){const button=document.createElement('button');button.className='btn';button.textContent='Retry';button.onclick=retry;el.append(button);}
}
window.NM={request,saveSettings:(base,draft)=>writer.save(base,draft),saveSettingsChanges:writer.saveChanges,settingsTransaction:writer.transaction,captureChanges,applyChanges,normalizeState,contributingLinks,schedule:scheduler.schedule,dispose:scheduler.dispose,icon,icons,confirm,report};
document.querySelectorAll('.page').forEach(page=>{page.inert=true;page.setAttribute('aria-busy','true');});
icons();
new MutationObserver(()=>icons()).observe(document.body,{childList:true,subtree:true});
const files=['app/controller.js','features/overview.js','features/quantum-flow.js','app/navigation.js','features/apps.js','features/speed-test.js','features/protection.js','features/settings.js','features/logs.js','features/blocking.js','features/sharing.js','features/usage.js','features/updates.js','app/state.js','features/presets.js','features/dns.js','features/activity.js','features/history-chart.js','features/backup.js','features/setup.js','app/start.js'];
try {
  for(const file of files)await new Promise((resolve,reject)=>{
    const script=document.createElement('script');script.src=file;script.onload=resolve;script.onerror=()=>reject(Error(`Could not load ${file}`));document.head.append(script);
  });
} catch(error) {
  report('the application',error,()=>location.reload());
}
