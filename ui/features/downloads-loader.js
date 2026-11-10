/* Optional module: no download-manager assets or requests until enabled. */
let DM_ON=false,DM_LOAD=null,dmSwitching=false;
const dmScripts=new Map();
function dmLoad(){
  if(DM_LOAD)return DM_LOAD;
  if(!document.querySelector('link[data-downloads]')){
    const link=document.createElement('link');link.rel='stylesheet';link.href='downloads.css';link.dataset.downloads='true';document.head.append(link);
  }
  const load=src=>{
    if(dmScripts.has(src))return dmScripts.get(src);
    const promise=new Promise((resolve,reject)=>{
      const script=document.createElement('script');script.src=src;
      script.onload=resolve;script.onerror=()=>{dmScripts.delete(src);script.remove();reject(Error('Could not load the Downloads workspace.'));};document.head.append(script);
    });dmScripts.set(src,promise);return promise;
  };
  DM_LOAD=load('downloads-more.js').then(()=>load('downloads.js')).catch(error=>{DM_LOAD=null;throw error;});
  return DM_LOAD;
}
function renderDownloadsOff(){
  const box=$('#dlBody');if(box.dataset.off==='true')return;
  box.dataset.off='true';
  box.innerHTML=`<section class="card downloads-off"><div class="empty-icon">${NM.icon('download')}</div><h2>Give every download a faster lane</h2><p>The optional download manager brings transfers, queues and browser capture into one workspace. Turn it on when you are ready.</p><button class="btn pri" id="enableDownloads">Enable download manager ${NM.icon('arrow-up-right')}</button><ul><li>${NM.icon('network')}Use your connections together</li><li>${NM.icon('clock')}Organize and schedule queues</li><li>${NM.icon('link')}Connect your browser</li></ul></section>`;
  $('#enableDownloads').onclick=()=>dmSwitch(true);
}
function dmApply(on){
  const toggle=$('#dmTog');toggle.classList.toggle('on',on);toggle.setAttribute('aria-checked',on);toggle.disabled=dmSwitching;
  if(on===DM_ON)return;
  DM_ON=on;
  if(on){
    $('#dlBody').dataset.off='false';
    dmLoad().then(()=>{if(page==='downloads'){window.nmDownloadsTab?.(currentRoute.split('/')[1]||'transfers');if(typeof loadDL==='function')loadDL(true);}}).catch(error=>NM.report('downloads',error,()=>{DM_ON=false;dmApply(true);}));
  }else{ $('#dlCard').hidden=true;if(page==='downloads')renderDownloadsOff(); }
}
async function dmSwitch(on){
  if(dmSwitching)return;
  if(!on&&!await NM.confirm({title:'Turn off Downloads?',message:'Active transfers will stop. They can continue when you enable the download manager again.',confirmLabel:'Turn off'}))return;
  dmSwitching=true;$('#dmTog').disabled=true;if($('#enableDownloads'))$('#enableDownloads').disabled=true;
  try{const result=await api('downloads/switch',{on});if(S)S.downloadsOn=!!result.enabled;dmApply(!!result.enabled);toast(result.enabled?'Download manager enabled':'Download manager off');}
  catch(error){toast(error.message,true);}
  finally{dmSwitching=false;$('#dmTog').disabled=false;if($('#enableDownloads'))$('#enableDownloads').disabled=false;}
}
$('#dmTog').onclick=()=>dmSwitch(!DM_ON);
