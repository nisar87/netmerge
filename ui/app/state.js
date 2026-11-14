let hidTick=0,bootApp='',pollInFlight=null,lastStateAt=0;
function safeToReload(){
  if(settingsIntent.length||document.querySelector('dialog[open]')||document.querySelector('[data-dirty="1"]'))return false;
  // text typed on other pages and not saved yet
  if((typeof dnsDirty!=='undefined'&&dnsDirty)||(typeof prNameDraft!=='undefined'&&prNameDraft)||(typeof prScheduleDirty!=='undefined'&&prScheduleDirty.size)||(typeof shEdit!=='undefined'&&shEdit)||document.querySelector('#usFix[open]'))return false;
  const a=document.activeElement;
  return !(a&&(a.isContentEditable||/^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName)));
}
async function poll(resync){
  if(pollInFlight)return resync ? pollInFlight.then(()=>poll(true)) : pollInFlight;
  if(document.hidden&&!resync&&++hidTick%8)return;
  pollInFlight=(async()=>{
    const epoch=stateEpoch;
    try{
      const next=NM.normalizeState(await api('state'));
      if(epoch!==stateEpoch)return;
      if(bootApp&&next.snap.app&&next.snap.app!==bootApp){
        // NetMerge was updated: load the new window at once, unless the person is in the middle of something
        // (a setting being saved, an open dialog, the cursor in a text field). Then a banner offers it, and the
        // reload happens by itself at the first poll after they are done.
        if(safeToReload()){location.reload();return;}
        const banner=$('#connectionBanner');banner.hidden=false;banner.className='note info';banner.innerHTML='<span>NetMerge was updated. The window will refresh when you finish what you are doing.</span><button class="btn" id="reloadApp">Refresh now</button>';$('#reloadApp').onclick=()=>location.reload();
      }else $('#connectionBanner').hidden=true;
      bootApp ||= next.snap.app || '';S=next;pollFails=0;lastStateAt=Date.now();
      $$('.page').forEach(section=>{section.inert=false;section.removeAttribute('aria-busy');});
      for(const key of ['links','apps','devices','total','sameIsp'])S.snap[key] ||= [];
      S.settings.mode ||= 'fast';
      settingsBase=structuredClone(S.settings);
      local=NM.applyChanges(S.settings,settingsIntent.flatMap(edit=>edit.changes));
      settingsEditSnapshot=structuredClone(local);
      render();
    }catch(error){
      if(epoch!==stateEpoch)return;
      pollFails++;
      const banner=$('#connectionBanner');banner.hidden=false;banner.className='note '+(pollFails>=3?'bad':'warn');
      banner.innerHTML='<span>'+esc(S ? 'Lost contact with NetMerge. Showing the last figures; reconnecting by itself.' : 'Waiting for NetMerge. Open the desktop application to connect.')+'</span><button class="btn" id="retryState">Retry now</button>';
      $('#retryState').onclick=()=>poll(true);
      if(S&&local)render();else $('#power').disabled=true;
      return false;
    }finally{pollInFlight=null;}
  })();
  return pollInFlight;
}
