let hidTick=0,bootApp='',pollInFlight=null,lastStateAt=0;
async function poll(resync){
  if(pollInFlight)return resync ? pollInFlight.then(()=>poll(true)) : pollInFlight;
  if(document.hidden&&!resync&&++hidTick%8)return;
  pollInFlight=(async()=>{
    const epoch=stateEpoch;
    try{
      const next=NM.normalizeState(await api('state'));
      if(epoch!==stateEpoch)return;
      if(bootApp&&next.snap.app&&next.snap.app!==bootApp){
        const banner=$('#connectionBanner');banner.hidden=false;banner.className='note info';banner.innerHTML='<span>NetMerge was updated. Reload when you are ready to use the new version.</span><button class="btn" id="reloadApp">Reload</button>';$('#reloadApp').onclick=()=>location.reload();
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
      banner.innerHTML='<span>'+esc(S ? 'Connection lost. Showing the last received values; reconnecting automatically.' : 'Waiting for NetMerge. Open the desktop application to connect.')+'</span><button class="btn" id="retryState">Retry now</button>';
      $('#retryState').onclick=()=>poll(true);
      if(S&&local)render();else $('#power').disabled=true;
      return false;
    }finally{pollInFlight=null;}
  })();
  return pollInFlight;
}
