/* ---------- logs ---------- */
let logRead=null;
async function loadLog(){
  logRead?.abort();
  const source=logWhich,read=logRead=new AbortController();
  try{
    const j=await NM.request('log?f='+source,{signal:read.signal});
    if(logRead!==read||source!==logWhich)return;
    const b=$('#logBox'),follow=b.scrollTop+b.clientHeight>=b.scrollHeight-24;
    b.textContent=j.text||(source==='app'?'(empty)':'(empty — turn on Detailed logging in Settings to record this)');
    if(follow)b.scrollTop=b.scrollHeight;
  }catch(e){if(!read.signal.aborted){toast(e.message,true);return false;}}
  finally{if(logRead===read)logRead=null;}
}
$$('#logSel button').forEach(b => b.onclick = () => { logWhich = b.dataset.v; $$('#logSel button').forEach(x=>x.classList.toggle('sel',x===b)); loadLog(); });
$('#logRefresh').onclick = loadLog;

/* ---------- mode ---------- */
$('#mode').onchange = e => { local.mode = e.target.value; render(); e.target.blur(); saveSoon(local.mode.startsWith('link:') ? `Everything now uses ${labelOf(local.mode.slice(5))} only` : local.mode==='fast' ? 'Combining all connections · fastest' : 'Combining all connections · steady sites'); };
