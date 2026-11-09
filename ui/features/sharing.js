/* ---------- share ---------- */
let SH = null, shAddrSel = '', shKey = '', shPlat = 'android';
let shRead=null,shBusy=false,shEdit=null;
async function loadShare(){
  if(shBusy||shRead)return;
  const read=shRead=new AbortController();
  try{const next=await NM.request('share',{signal:read.signal});if(shRead!==read)return;SH=next;NM.report('sharing');renderShare();}
  catch(error){if(read.signal.aborted)return;if(page==='share')NM.report('sharing',error,loadShare);return false;}
  finally{if(shRead===read)shRead=null;}
}
async function shareSet(body,msg){
  if(shBusy)return false;
  shBusy=true;shRead?.abort();shRead=null;renderShare();
  try{SH=await api('share',body);toast(msg);return true;}
  catch(e){if(body.name&&shEdit?.id===body.name.id)shEdit.error=e.message;toast(e.message,true);return false;}
  finally{shBusy=false;renderShare();}
}
async function copyText(t){try{await navigator.clipboard.writeText(t);toast('Copied');}catch(e){toast('Could not copy. Select the text and press Ctrl+C.',true);}}
function shIP(){ const l = (SH.addresses||[]); return (l.find(a => a.ip === shAddrSel) || l.find(a => a.ip.startsWith('192.168.1.')) || l[0] || {}).ip || ''; }
function shLink(){
  const ip = shIP(); if (!ip) return '';
  // standard Shadowsocks link (SIP002): every proxy app can import it
  const ui = btoa((SH.cipher||'aes-128-gcm')+':'+SH.pass).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');
  return `ss://${ui}@${ip}:${SH.ssPort||8388}#NetMerge`;
}
function drawQR(cv, text){
  const q = new QRLib.QRCode(-1, QRLib.ECL.M); q.addData(text); q.make();
  const n = q.getModuleCount(), quiet = 2, sz = cv.width, cell = Math.floor(sz/(n+quiet*2)), off = Math.floor((sz-cell*n)/2), g = cv.getContext('2d');
  g.fillStyle = '#fff'; g.fillRect(0,0,sz,sz); g.fillStyle = '#111';
  for (let r=0;r<n;r++) for (let c=0;c<n;c++) if (q.isDark(r,c)) g.fillRect(off+c*cell, off+r*cell, cell, cell);
}
const APPS = {
  android: { name:'Android', apps:'Hiddify or v2rayNG (both free)', how:'Open the app, tap <b>+</b> and choose <b>Scan QR code</b> (in v2rayNG: <b>Import config from QR code</b>).' },
  iphone:  { name:'iPhone',  apps:'Hiddify, Streisand or Shadowrocket', how:'Open the app, go to its add-profile screen and choose <b>Scan QR code</b>.' }
};
function buildShare(){
  // Keep the live device editor when connection details change around it.
  const devices=$('#shDev'),active=document.activeElement;
  const focus=$('#shBody').contains(active)?active.id:null;
  const selection=focus&&active.tagName==='INPUT'?[active.selectionStart,active.selectionEnd,active.selectionDirection]:null;
  const manualOpen=$('#shManual')?.open;
  const rows = (SH.addresses||[]).map(a => `<div class="shk"><span>${esc(a.iface)}</span><code>${esc(a.ip)}</code><button class="btn sm" data-copy="${esc(a.ip)}">Copy</button></div>`).join('') || '<div class="shk"><span>No network address found</span></div>';
  $('#shBody').innerHTML = `
    <div class="shgrid">
      <div class="card shsteps2">
        <div class="stp"><i>1</i><div><b>Get a free app on your phone</b>
          <div class="seg" id="shPlat" style="margin:8px 0 6px"><button data-v="android">Android</button><button data-v="iphone">iPhone</button></div>
          <div class="hint2" id="shApps"></div></div></div>
        <div class="stp"><i>2</i><div><b>Scan this code</b>
          <div class="hint2" id="shHow"></div></div></div>
        <div class="qrbox"><canvas id="qr-hid" width="260" height="260" aria-label="QR code to scan with your phone"></canvas></div>
        <div class="qrmeta">
          <label class="hint2" for="shAddr">PC address in the code</label>
          <select class="sel" id="shAddr">${(SH.addresses||[]).map(a => `<option value="${esc(a.ip)}">${esc(a.ip)} · ${esc(a.iface)}</option>`).join('')}</select>
          <button class="btn" id="shCopyLink">Copy link instead</button>
        </div>
        <div class="stp"><i>3</i><div><b>Connect</b>
          <div class="hint2">Tap the connect button in the app and allow the VPN request. Your phone is now using every connection.</div></div></div>
      </div>
      <div style="display:flex;flex-direction:column;gap:16px;min-width:0">
        <div class="card"><h3 style="margin:0 0 8px">Connected now</h3><div id="shDev"></div></div>
        <div class="card"><h3 style="margin:0 0 6px">Good to know</h3>
          <ul class="tips">
            <li>The phone and this PC must be on the <b>same network</b> (same Wi-Fi or router).</li>
            <li>NetMerge must be <b>merging</b> for sharing to work.</li>
            <li>The code contains your password. If anyone else has seen it, make a new one below.</li>
            <li>Only one VPN app can run on the phone at a time.</li>
          </ul></div>
        <details class="card adv" id="shManual"><summary>Manual setup &amp; password</summary>
          <div class="hint2" style="margin:8px 0">Use these if you prefer to type the details, or want to set the phone's Wi-Fi proxy by hand (Settings → Wi-Fi → network → Proxy → Manual).</div>
          ${rows}
          <div class="shk"><span>Proxy port</span><code>${SH.port}</code><button class="btn sm" data-copy="${SH.port}">Copy</button></div>
          <div class="shk"><span>User name</span><code>${esc(SH.user)}</code><button class="btn sm" data-copy="${esc(SH.user)}">Copy</button></div>
          <div class="shk"><span>Password</span><code>${esc(SH.pass)}</code><button class="btn sm" data-copy="${esc(SH.pass)}">Copy</button></div>
          <div style="margin-top:12px"><button class="btn" id="shNew">Make a new password</button></div>
        </details>
      </div>
    </div>`;
  if(devices)$('#shDev').replaceWith(devices);
  if(manualOpen)$('#shManual').open=true;
  $('#shBody').querySelectorAll('[data-copy]').forEach(b => b.onclick = () => copyText(b.dataset.copy));
  $('#shNew').onclick=async()=>{if(await NM.confirm({title:'Create a new sharing password?',message:'Connected devices will need to scan the new code or enter the new password.',confirmLabel:'Create new password'}))shareSet({newPass:true},'New password created. Reconnect your devices.');};
  $('#shCopyLink').onclick = () => copyText(shLink());
  $$('#shPlat button').forEach(b => b.onclick = () => { shPlat = b.dataset.v; paintShare(); });
  const sa = $('#shAddr'); sa.onchange = () => { shAddrSel = sa.value; paintShare(); };
  return ()=>{const target=focus&&document.getElementById(focus);if(target){target.focus({preventScroll:true});if(selection)target.setSelectionRange(...selection);}};
}
// the parts that change without rebuilding the page (so open sections and the selected address stay as they are)
function paintShare(){
  const A = APPS[shPlat];
  $$('#shPlat button').forEach(b => b.classList.toggle('sel', b.dataset.v === shPlat));
  $('#shApps').innerHTML = `Install <b>${A.apps}</b> from the ${shPlat==='iphone'?'App Store':'Play Store'}.`;
  $('#shHow').innerHTML = A.how;
  const sa = $('#shAddr'); if (sa && document.activeElement !== sa) sa.value = shIP();
  const l = shLink(), cv = $('#qr-hid');
  if (cv && l && cv.dataset.l !== l){ cv.dataset.l = l; try { drawQR(cv, l); } catch(e){} }
  if(shEdit&&$('#shNm')){shareEditorControls();return;}
  const dev=[...(SH.devices||[])];
  // A temporarily disconnected device must not take its unsaved name with it.
  if(shEdit&&!dev.some(d=>d.id===shEdit.id))dev.push(shEdit.device);
  const icon=d=>NM.icon('smartphone');
  setHTML($('#shDev'), dev.length ? dev.map(d => {
    const sub = [d.ip, d.mac, d.vendor || (d.private ? 'private address' : ''), `${d.conns} connection${d.conns===1?'':'s'}`].filter(Boolean).join(' · ');
    return `<div class="dev"><div class="dic">${icon(d)}</div><div style="flex:1;min-width:0">
      <div class="dname"><b>${esc(d.name || 'Unnamed device')}</b><button class="lnk" data-ren="${esc(d.id)}" data-cur="${esc(d.source==='you'?d.name:'')}">${d.source==='you'?'Rename':'Name it'}</button></div>
      <div class="hint2">${esc(sub)}</div></div>
      <div class="num">${fmtM(d.down)}<small> Mbps ↓</small></div></div>`; }).join('')
    : '<div class="hint2">No device connected yet. Scan the code on your phone and connect.</div>');
  if(shEdit)mountShareEditor();
}
function shareEditorControls(){
  for(const id of ['shNmOk','shNmNo']){const button=$('#'+id);if(button)button.disabled=shBusy;}
  const error=$('#shNmError');if(error)error.textContent=shEdit?.error||'';
}
function mountShareEditor(){
  const edit=shEdit,button=$$('#shDev [data-ren]').find(b=>b.dataset.ren===edit.id);if(!button)return;
  button.closest('.dname').innerHTML=`<input class="inp" id="shNm" aria-label="Device name" maxlength="40" placeholder="e.g. My phone" value="${esc(edit.value)}" style="flex:1;min-width:0"><button class="btn sm" id="shNmOk">Save</button><button class="btn sm" id="shNmNo">Cancel</button><span id="shNmError" role="status"></span>`;
  delete $('#shDev').dataset.h;
  const input=$('#shNm');
  input.oninput=()=>{edit.value=input.value;edit.revision++;edit.error='';shareEditorControls();};
  const done=async save=>{
    if(shBusy)return;
    const revision=edit.revision;
    if(save&&!await shareSet({name:{id:edit.id,name:edit.value}},'Saved'))return;
    if(shEdit===edit&&(!save||revision===edit.revision)){
      shEdit=null;renderShare();
      $$('#shDev [data-ren]').find(b=>b.dataset.ren===edit.id)?.focus({preventScroll:true});
    }
  };
  $('#shNmOk').onclick=()=>done(true);$('#shNmNo').onclick=()=>done(false);
  input.onkeydown=event=>{if(event.key==='Enter'||event.key==='Escape'){event.preventDefault();done(event.key==='Enter');}};
  shareEditorControls();
}
document.addEventListener('click',e=>{
  const button=e.target.closest?.('[data-ren]');if(!button||shBusy)return;
  if(shEdit){$('#shNm')?.focus();return;}
  const device=SH.devices.find(d=>d.id===button.dataset.ren);if(!device)return;
  shEdit={id:device.id,device:{...device},value:button.dataset.cur,revision:0,error:''};
  mountShareEditor();$('#shNm').focus();$('#shNm').select();
});
function renderShare(){
  if (!SH) return;
  const tog = $('#shTog'); tog.classList.toggle('on', SH.enabled); tog.setAttribute('aria-checked', SH.enabled);tog.disabled=shBusy;
  const n = (SH.devices||[]).length;
  $('#shStatus').innerHTML = !SH.enabled ? 'Let a phone or tablet use all your connections together.'
    : !SH.running ? '<span class="dot warn"></span>Turn merging on to start sharing'
    : n ? `<span class="dot good"></span>Sharing · ${n} device${n===1?'':'s'} connected`
    : '<span class="dot good"></span>Sharing is on · waiting for a device';
  if (!SH.enabled){
    shKey = '';
    $('#shBody').innerHTML = `<div class="card shoff"><div class="big">${NM.icon('share-2')}</div><h3>Use your merged internet on your phone</h3>
      <div class="hint2" style="max-width:520px;margin:6px auto 16px">Turn sharing on, scan one QR code with a free app, and your phone can use the connections managed by NetMerge. Blocking follows your protection settings.</div>
      <button class="btn primary" id="shOn">Turn on sharing</button></div>`;
    $('#shOn').onclick = () => shareSet({enabled:true}, 'Sharing on');
    $('#shOn').disabled=shBusy;
    return;
  }
  setHTML($('#shWarn'), (SH.publicNets||[]).length ? `<div class="note warn"><b>Windows treats ${esc(SH.publicNets.join(' and '))} as a Public network.</b> Only share on a network you trust (your home), not in a café or hotel.</div>` : '');
  const key = [SH.pass, SH.port, SH.ssPort, (SH.addresses||[]).map(a=>a.ip).join(',')].join('|');
  let restoreFocus;
  if (key !== shKey || !$('#shAddr')){ shKey = key; restoreFocus=buildShare(); }
  paintShare();
  $('#shNew').disabled=shBusy;
  const grid=$('#shBody .shgrid'),device=$('#shDev').closest('.card');
  const devicesOnly=routeView==='devices';grid.classList.toggle('devices-only',devicesOnly);
  grid.querySelector('.shsteps2').hidden=devicesOnly;
  [...device.parentElement.children].forEach(child=>{child.hidden=devicesOnly ? child!==device : child===device;});
  restoreFocus?.();
}
$('#shTog').onclick = () => shareSet({enabled: !(SH && SH.enabled)}, (SH && SH.enabled) ? 'Sharing off' : 'Sharing on');
NM.schedule('share',()=>loadShare(), 2500, ()=>page==='share'&&!document.hidden);
