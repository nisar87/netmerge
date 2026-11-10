/* First-run checklist uses a native modal, with explicit readiness and optional preferences. */
let wizOpen=false,wizSkip=false,wizBusy=false;
function wizSteps(){
  const present=S.snap.links.filter(link=>link.present&&link.enabled).length;
  const engine=(!!S.snap.version || !!UP && !UP.engineDl) && !/not installed/i.test(S.snap.error||'');
  return [
    [present>=2,present>=2?`${present} connections found`:present===1?'One connection found':'No connection found',present>=2?'Your connections are ready to combine.':'Connect a second network, such as Ethernet and Wi-Fi.'],
    [engine,'Network engine',engine?'Installed and ready.':UP?.engineDl==='downloading'?'Downloading the engine. This is needed only once.':UP?.engineDl==='error'?'The engine could not be downloaded. Open Maintenance to retry.':'Waiting for the engine to report its version.'],
    [!!local.autoStart,'Start with Windows','Optional. Open NetMerge quietly when you sign in.'],
    [!!local.mergeOnLaunch,'Merge automatically','Optional. Start merging when NetMerge opens.']
  ];
}
function renderWizard(force){
  if(force){wizOpen=true;wizSkip=false;}
  if(!local||!S)return;
  let dialog=$('#wiz');
  if(!(wizOpen||(!local.setupDone&&!wizSkip))){if(dialog){dialog.close();dialog.remove();}return;}
  if(!dialog){
    dialog=document.createElement('dialog');dialog.id='wiz';dialog.className='wiz';dialog.setAttribute('aria-labelledby','wizTitle');
    dialog.innerHTML='<div class="card"><div class="eyebrow">LET’S GET CONNECTED</div><h2 id="wizTitle">Welcome to NetMerge</h2><p class="hint2">A quick check before your connections work together.</p><ol id="wizSteps"></ol><p class="hint2" id="wizError" role="status"></p><div class="dialog-actions"><button class="btn" id="wizSkip">Set up later</button><button class="btn pri" id="wizDone">Done</button></div></div>';
    dialog.addEventListener('cancel',event=>{event.preventDefault();if(!wizBusy){wizOpen=false;wizSkip=true;dialog.close();dialog.remove();}});
    document.body.append(dialog);dialog.showModal();
    $('#wizSkip').onclick=()=>finishSetup(false);$('#wizDone').onclick=()=>finishSetup(true);
  }
  const steps=wizSteps();
  setHTML($('#wizSteps'),steps.map((step,index)=>`<li><i class="${step[0]?'ok':''}">${step[0]?NM.icon('check'):index+1}</i><div><b>${esc(step[1])}</b><p class="hint2">${esc(step[2])}</p>${index>=2?`<button class="btn sm" id="wizOption${index}" data-setup-option="${index}" aria-pressed="${step[0]}" aria-disabled="${wizBusy}">${step[0]?'Turn off':'Turn on'}</button>`:''}</div></li>`).join(''));
  const ready=steps[0][0]&&steps[1][0];
  $('#wizDone').textContent=ready&&!S.snap.running?'Start merging':'Done';$('#wizDone').disabled=wizBusy;$('#wizSkip').disabled=wizBusy;
}
async function finishSetup(start){
  if(wizBusy)return;wizOpen=true;wizBusy=true;renderWizard();
  try{
    local.setupDone=true;await api('settings',local);
    const steps=wizSteps();if(start&&steps[0][0]&&steps[1][0]&&!S.snap.running)await api('power',{on:true});
    wizOpen=false;wizSkip=true;const dialog=$('#wiz');dialog?.close();dialog?.remove();poll(true);
  }catch(error){$('#wizError').textContent=error.message;}
  finally{wizBusy=false;if($('#wiz'))renderWizard();}
}
document.addEventListener('click',async event=>{
  const option=event.target.closest('[data-setup-option]');
  if(option&&!wizBusy){
    const key=option.dataset.setupOption==='2'?'autoStart':'mergeOnLaunch';wizBusy=true;option.disabled=true;local[key]=!local[key];
    try{await api('settings',local);}catch(error){$('#wizError').textContent=error.message;}finally{wizBusy=false;renderWizard();}
  }
  if(event.target.closest('#wizBtn'))renderWizard(true);
});
