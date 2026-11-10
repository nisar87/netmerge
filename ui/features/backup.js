/* ---------- settings backup ---------- */
$('#restBtn').onclick = () => $('#restFile').click();
$('#restFile').onchange = async e => {
  const f = e.target.files[0]; if (!f) return;
  let progress;
  try {
    const backup=JSON.parse(await f.text());
    if(!await NM.confirm({title:'Restore settings?',message:'Settings from '+f.name+' will replace your current configuration and unsaved changes. Save a backup first if you want to keep it.',confirmLabel:'Restore settings'}))return;
    clearTimeout(saveTimer);saveTimer=null;
    progress=document.createElement('dialog');progress.className='nm-dialog';progress.setAttribute('aria-labelledby','restoreTitle');progress.setAttribute('aria-busy','true');
    progress.innerHTML='<h2 id="restoreTitle" tabindex="-1" autofocus>Restoring settings…</h2><p>Finishing any earlier save before applying your backup.</p>';
    progress.addEventListener('cancel',event=>event.preventDefault());document.body.append(progress);progress.showModal();
    await NM.settingsTransaction(()=>api('restore',backup));
    settingsIntent=[];settingsUnsaved=false;settingsEditSnapshot=null;settingsBase=null;local=null;
    $('#saveStatus').hidden=true;await poll(true);toast('Settings restored');
  }
  catch(err){ toast(err.message || 'Not a NetMerge settings file', true); }
  finally{if(progress){progress.close();progress.remove();}e.target.value='';}
};
