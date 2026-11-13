/* ---------- settings ---------- */
function renderSettings(){
  $('#callTog').classList.toggle('on', !local.noCallProtect); $('#callTog').setAttribute('aria-checked', !local.noCallProtect);
  [['#autoTog','autoStart'],['#launchTog','mergeOnLaunch'],['#debugTog','debugLog']].forEach(([id,k]) => { $(id).classList.toggle('on', !!local[k]); $(id).setAttribute('aria-checked', !!local[k]); });
  $$('#theme button,.theme-preview').forEach(b=>{const selected=b.dataset.v===document.documentElement.dataset.preference;b.classList.toggle('sel',selected);b.setAttribute('aria-pressed',selected);});
  if (document.activeElement !== $('#balSel')) $('#balSel').value = local.classicBalance ? 'classic' : 'smart';
  const v = S.snap.version || '';
  $('#engineVer').textContent = v ? 'Built into NetMerge, version ' + v.replace(/^nmengine-/, '') + ' (running)' : 'Not running';
}
$('#autoTog').onclick = () => { local.autoStart = !local.autoStart; renderSettings(); saveSoon(local.autoStart ? 'Will start with Windows' : 'Will not start with Windows'); };
$('#launchTog').onclick = () => { local.mergeOnLaunch = !local.mergeOnLaunch; renderSettings(); saveSoon('Saved'); };
$('#balSel').onchange = e => { local.classicBalance = e.target.value === 'classic'; saveSoon(local.classicBalance ? 'Classic balancing' : 'Smart balancing'); };
$('#callTog').onclick = () => { local.noCallProtect = !local.noCallProtect; renderSettings(); saveSoon(local.noCallProtect ? 'Call protection off' : 'Call protection on'); };
$('#debugTog').onclick = () => { local.debugLog = !local.debugLog; renderSettings(); saveSoon(local.debugLog ? 'Detailed logging on' : 'Detailed logging off'); };
document.addEventListener('click', async e => {
  const b = e.target.closest && e.target.closest('[data-forget]'); if (!b) return;
  const lbl = (local.links.find(x => x.name === b.dataset.forget) || {}).label || b.dataset.forget;
  if (!await NM.confirm({title:'Forget '+lbl+'?',message:'This connection and its routing rules will be removed. Its usage history stays. Reconnecting it adds it as a new connection.',confirmLabel:'Forget connection',danger:true})) return;
  try { await api('linkforget', {name: b.dataset.forget}); await poll(true); toast('Connection forgotten'); } catch (err) { toast(err.message, true); }
});
async function scanConnections(){const buttons=[$('#rescan'),$('#rescanConnections')];buttons.forEach(b=>b.disabled=true);try{await api('rescan',{});await poll(true);toast('Connections scanned');}catch(error){toast(error.message,true);}finally{buttons.forEach(b=>b.disabled=false);}}
$('#rescan').onclick=scanConnections;$('#rescanConnections').onclick=scanConnections;
$('#quit').onclick = async () => {
  if(!await NM.confirm({title:'Quit NetMerge?',message:'Merging will stop and your normal internet will be restored.',confirmLabel:'Quit NetMerge',danger:true}))return;
  try{await api('quit',{});}catch(error){toast(error.message,true);return;}NM.dispose();
  document.body.innerHTML = '<div style="display:grid;place-items:center;height:100vh;text-align:center;color:var(--mute);font-size:16px"><div><div style="font-size:22px;color:var(--text);margin-bottom:8px">NetMerge has stopped</div>Your normal internet is restored. You can close this window.</div></div>';
};
