const $ = s => document.querySelector(s), $$ = s => [...document.querySelectorAll(s)];
const ICONS = {ethernet:NM.icon('cable'),wifi:NM.icon('wifi'),mobile:NM.icon('smartphone')};
const PAGES = {
  overview:['Overview','A clear view of your connected world.'],
  connections:['Connections','Make every connection work for you.'],
  dns:['DNS','Which servers look up website names'],
  history:['History','What happened and when'],
  presets:['Presets','One click for gaming, work, travel and more'],
  apps:['Apps','Decide which connection each app uses'],
  test:['Speed test','Measure what your connections can really do'],
  usage:['Usage','Data used by each connection, day by day'],
  share:['Share','Let your phone use the merged internet'],
  rules:['Protection','Keep VPNs, servers and logins working perfectly'],
  block:['Blocking','Stop ads, trackers and unsafe sites for every app'],
  settings:['Settings','Startup, appearance and maintenance'],
  log:['Logs','What NetMerge has been doing']
};
let S = null, local = null, saveTimer = null, busyPower = false, page = 'overview', logWhich = 'app', pollFails = 0, hover = -1, chartGeo = null;

/* ---------- helpers ---------- */
const mbps = b => typeof b === 'number' && Number.isFinite(b) ? b*8/1e6 : NaN;
const fmt = v => typeof v !== 'number' || !Number.isFinite(v) ? '—' : v >= 100 ? v.toFixed(0) : v >= 10 ? v.toFixed(1) : v.toFixed(2);
const fmtM = b => fmt(mbps(b));
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const bytes = b => b >= 1e9 ? (b/1e9).toFixed(2)+' GB' : b >= 1e6 ? (b/1e6).toFixed(1)+' MB' : (b/1e3).toFixed(0)+' KB';
const dur = s => { if (!s) return '—'; const h=Math.floor(s/3600), m=Math.floor(s%3600/60); return h ? `${h}h ${m}m` : m ? `${m}m ${s%60}s` : `${s}s`; };
const css = v => getComputedStyle(document.documentElement).getPropertyValue(v).trim();
const pal = () => [1,2,3,4,5].map(i => css('--l'+i));
const linkColors=new Map();
const colorOf=name=>{if(!linkColors.has(name))linkColors.set(name,linkColors.size%5);return pal()[linkColors.get(name)];};
const labelOf = name => (local?.links||[]).find(l => l.name === name)?.label || name;
const setHTML = (el,html,{force=false}={}) => {
  if (!el || el.dataset.h === html) return;
  // Live refreshes must not replace an editor or close a disclosure the user opened.
  const focus = document.activeElement;
  if(!force&&el.contains(focus)&&/^(INPUT|TEXTAREA|SELECT)$/.test(focus.tagName))return;
  const key=el.contains(focus) ? focus.id || focus.dataset.focusKey : null;
  const open=[...el.querySelectorAll('details[open]')].map(x=>x.id).filter(Boolean);
  el.dataset.h=html; el.innerHTML=html;
  open.forEach(id=>el.querySelector('#'+CSS.escape(id))?.setAttribute('open',''));
  if(key)el.querySelector('#'+CSS.escape(key))?.focus({preventScroll:true});
};

let toastT;
function toast(msg, bad){ const t=$('#toast'); t.textContent=msg; t.className='toast show'+(bad?' bad':''); clearTimeout(toastT); toastT=setTimeout(()=>t.className='toast',2800); }

let settingsBase=null,settingsEditSnapshot=null,settingsIntent=[],settingsPending=0,settingsRevision=0,settingsUnsaved=false,stateEpoch=0;
function recordSettingsIntent(draft){
  const changes=NM.captureChanges(settingsEditSnapshot||settingsBase||S?.settings||draft,draft);
  if(changes.length)settingsIntent.push({revision:++settingsRevision,changes});
  settingsEditSnapshot=structuredClone(draft);settingsUnsaved=settingsIntent.length>0;
}
async function api(path, body){
  if(path!=='settings'||body===undefined){
    if(body===undefined)return NM.request(path);
    stateEpoch++;try{return await NM.request(path,{body});}finally{stateEpoch++;}
  }
  recordSettingsIntent(body);
  const edits=settingsIntent.slice(),revision=edits.at(-1)?.revision||0;
  settingsPending++;stateEpoch++;
  try {
    const next=await NM.saveSettingsChanges(edits.flatMap(edit=>edit.changes));
    settingsIntent=settingsIntent.filter(edit=>edit.revision>revision);
    settingsBase=structuredClone(next);
    local=NM.applyChanges(next,settingsIntent.flatMap(edit=>edit.changes));
    settingsEditSnapshot=structuredClone(local);settingsUnsaved=settingsIntent.length>0;
    const status=$('#saveStatus');status.hidden=true;
    return next;
  } catch(error) {
    settingsUnsaved=true;
    const status=$('#saveStatus');status.hidden=false;status.className='note bad';
    status.innerHTML='<span>Your changes could not be saved. They are still here.</span><button class="btn" id="retrySettings">Retry save</button>';
    $('#retrySettings').onclick=()=>saveSoon();
    throw error;
  } finally {settingsPending--;stateEpoch++;}
}
function saveSoon(msg){
  recordSettingsIntent(local);
  clearTimeout(saveTimer);
  saveTimer=setTimeout(async()=>{
    saveTimer=null;
    try{await api('settings',local);toast(msg || 'Saved');}
    catch(error){toast(error.message,true);}
  },350);
}

/* ---------- theme ---------- */
function applyTheme(preference){
  if(!['auto','light','dark'].includes(preference))preference='auto';
  document.documentElement.dataset.preference=preference;
  document.documentElement.dataset.theme=preference==='auto' ? (matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light') : preference;
  try{localStorage.setItem('nm-theme',preference);}catch{}
  $$('#theme button').forEach(b=>{b.classList.toggle('sel',b.dataset.v===preference);b.setAttribute('aria-pressed',b.dataset.v===preference);});
  if(S)render();
}
$$('#theme button').forEach(b=>b.onclick=()=>applyTheme(b.dataset.v));
$('#quickTheme').onclick=()=>applyTheme(document.documentElement.dataset.theme==='dark'?'light':'dark');
matchMedia('(prefers-color-scheme: dark)').addEventListener('change',()=>{if(document.documentElement.dataset.preference==='auto')applyTheme('auto');});

/* ---------- power ---------- */
async function togglePower(){
  if (!S || busyPower) return;
  const want = !S.snap.running;
  $('#power').disabled=true;
  busyPower = true; $('#power').classList.add('busy'); $('#powerText').textContent = want ? 'Starting…' : 'Stopping…';
  try { await api('power', {on:want}); toast(want ? 'Merging is on' : 'Merging is off'); }
  catch(e){ toast(e.message, true); }
  busyPower = false; $('#power').disabled=false; $('#power').classList.remove('busy'); poll(true);
}
$('#power').onclick = togglePower;


/* ---------- render ---------- */
function activeLinks(){ return NM.contributingLinks(S.snap); }

function render(){
  observeSpeedTest();
  updateHeld(S.snap);
  const sn = S.snap, run = sn.running, act = activeLinks();
  const P = pal();

  // header + sidebar status
  $('#sidebarVersion').textContent=S.snap.app ? 'v'+S.snap.app : '';
  $('#sideDot').classList.toggle('on',run);
  const p = $('#power'); p.disabled=busyPower || pollFails>=3; p.classList.toggle('is-on', run); p.setAttribute('aria-checked', run);
  if (!busyPower) $('#powerText').textContent = run ? 'Merging on' : 'Merging off';
  $('#sfTitle').textContent = run ? `${act.length} connection${act.length===1?'':'s'} contributing` : 'Merging is off';
  $('#sfText').textContent = run ? `${fmtM(sn.totalDown)} Mbps right now` : 'Turn it on to combine your internet.';

  // alerts
  const al = [];

  if (S.conflict) al.push(`<div class="note bad"><span style="flex:1">${esc(S.conflict)}</span><button class="btn" id="fixConf">Close it for me</button></div>`);
  if (sn.error && !run && sn.error !== S.conflict) al.push(`<div class="note bad">${esc(sn.error)}</div>`);
  setHTML($('#alerts'), al.join(''));
  renderBlockBadge();
  if (page === 'block') renderBlock();

  { const n = S.legacyDownloads || 0; $('#pwOld').hidden = !n; $('#pwSave').hidden = !n; }
  if (page === 'overview') { renderOverview(sn,run,act,P); loadHistory(); }
  if (page === 'connections') { renderConnectionMode(); renderLinks(sn,act,P); }
  renderWizard();
  if (page === 'apps') renderApps();
  if (page === 'test') renderTest();
  if (page === 'rules') renderRules();
  if (page === 'settings') renderSettings();
}

function statusLine(sn, run){
  if (!run) return ['', 'Merging is off', 'Turn it on to combine your connections.'];
  const ls = sn.links.filter(l => l.enabled && l.present);
  const bad = ls.filter(l => l.healthy === false), out = ls.filter(l => l.quotaOut), gone = sn.links.filter(l => l.enabled && !l.present);
  const names = a => a.map(l => l.label).join(' and ');
  if (gone.length) return ['warn', `${names(gone)} ${gone.length>1?'are':'is'} disconnected`, `Using ${ls.length} connection${ls.length===1?'':'s'} for now. Plug it back in and it joins automatically.`];
  if (bad.length) return ['bad', `${names(bad)} ${bad.length>1?'have':'has'} no internet`, 'It is left out until it works again, so nothing slows down.'];
  if (out.length) return ['warn', `${names(out)} used up its allowance`, 'It is left out of the merge until the allowance renews.'];
  if ((sn.sameIsp||[]).length > 1) return ['warn', `${sn.sameIsp.join(' and ')} look like the same provider`, 'They probably leave through the same provider or router, so merging cannot add speed. Two different providers are needed.'];
  const active=NM.contributingLinks(sn);
  if (ls.some(l=>l.healthy==null)) return ['warn','Checking connection health','Waiting for a confirmed health reading.'];
  if (active.length < 2) return ['warn',active.length ? 'One connection is contributing' : 'No connections are contributing','Check Connections for availability, standby roles and allowances.'];
  const n = (S.share && S.share.devices) || 0;
  return ['good', `${active.length} connections working together`, 'Each new connection goes to the line with the most spare speed.'];
}
function renderConnectionMode(){
  const sn=S.snap;
  $('#connHint').textContent = sn.links.length ? 'NetMerge shares the work between your connections by itself. Turn one off to leave it out.' : '';
  const ms = $('#mode');
  if (document.activeElement !== ms) {
    const opts = [['fast','Combine all · fastest'], ['stable','Combine all · steady sites']].concat(local.links.map(l => ['link:'+l.name, 'Only '+l.label + (S.snap.links.find(x=>x.name===l.name)?.present === false ? ' (not connected)' : '')]));
    setHTML(ms, opts.map(([v,t]) => `<option value="${esc(v)}"${v===local.mode?' selected':''}>${esc(t)}</option>`).join(''));
  }
  $('#modeDesc').textContent = local.mode.startsWith('link:')
    ? `Everything uses ${labelOf(local.mode.slice(5))} only. The other connection stays ready as a backup if it drops. Apps with their own rule keep that rule.`
    : local.mode === 'stable'
    ? 'Each website stays on one line. Fewer sign-in problems, slightly lower top speed.'
    : 'Uses all your lines at once, sharing the work by speed. Best for downloads and speed tests.';

}

/* values that should never blink in and out: keep the last good reading and smooth the traffic share */
const held = {};
function holdFor(name){ return held[name] ||= {delay:0, publicIP:'', ema:0, share:null, activeAt:0}; }
function updateHeld(sn){
  const now = Date.now();
  sn.links.forEach(l => {
    const h = holdFor(l.name);
    if (l.delay) h.delay = l.delay;
    if (l.publicIP) h.publicIP = l.publicIP;
    h.ema = h.ema*0.8 + (l.present && Number.isFinite(l.down) ? l.down : 0)*0.2;
    if (mbps(l.down) > 0.3 || mbps(l.up) > 0.3) h.activeAt = now;
  });
  const tot = sn.links.reduce((a,l) => a + holdFor(l.name).ema, 0);
  if (tot > 12500) sn.links.forEach(l => holdFor(l.name).share = Math.round(holdFor(l.name).ema/tot*100));
}
function statusOf(l, run){
  if (!l.present) return ['Disconnected', 'var(--warn)'];
  if (!l.enabled) return ['Off', 'var(--dim)'];
  if (!run) return ['Ready', 'var(--mute)'];
  if (l.quotaOut) return ['Allowance used up', 'var(--warn)'];
  if (l.standby) return ['Standing by', 'var(--info)'];
  if (l.slow) return ['Too slow to help: standing by', 'var(--info)'];
  if (l.portal) return ['Needs sign-in', 'var(--warn)'];
  if (l.healthy === false) return ['No internet', 'var(--bad)'];
  if (l.healthy == null) return ['Checking health', 'var(--mute)'];
  if (Date.now() - holdFor(l.name).activeAt < 5000) return ['Active', 'var(--good)'];
  return ['Online · idle', 'var(--info)'];
}

function renderLinks(sn, act, P){
  const box = $('#links'), run = sn.running;
  $('#connectionsEmpty').hidden=sn.links.length>0;
  const keys = sn.links.map(l=>l.name).join('|');
  if (box.dataset.keys !== keys){
    box.dataset.keys = keys;
    const existing=new Map([...box.children].map(card=>[card.dataset.n,card]));
    const focused=document.activeElement,selection=focused?.tagName==='INPUT'?[focused.selectionStart,focused.selectionEnd]:null;
    const next=document.createElement('div');
    next.innerHTML = sn.links.map(l => `
      <div class="lk" data-n="${esc(l.name)}">
        <div class="lk-h">
          <div class="ico">${ICONS[l.kind]||ICONS.ethernet}</div>
          <div style="flex:1;min-width:0">
            <input class="lbl" value="${esc(l.label)}" spellcheck="false" aria-label="Name for ${esc(l.name)}" title="Click to rename">
            <div class="desc" data-f="desc"></div>
          </div>
          <button class="tog" data-f="tog" role="switch" aria-label="Use ${esc(l.label || l.name)}"></button>
        </div>
        <div class="meta"><span class="chip" data-f="status"></span><span data-f="tags" style="display:contents"></span></div>
        <div data-f="missing"></div>
        <div class="share">
          <div class="share-h"><span title="NetMerge decides by itself how many newly opened websites and downloads use this line, based on its speed right now.">Share of the work<b data-f="pct"></b></span>
            <div class="step" data-f="step"><button data-f="minus" aria-label="Give this connection a smaller share">−</button><b class="num" data-f="w"></b><button data-f="plus" aria-label="Give this connection a bigger share">+</button></div></div>
          <div class="share-bar"><div data-f="bar"></div></div>
        </div>
        <details class="role connection-detail"><summary>Connection preferences</summary><label>When to use this line<select class="sel" data-f="role" aria-label="When to use this connection"><option value="">Always</option><option value="busy">Only when the others are busy</option><option value="backup">Only if the others fail</option></select></label><div class="hint2" data-f="roleHint"></div><div class="hint2 slowHint" data-f="slowHint" hidden>This line is much slower or less steady than the others. <button class="btn" data-f="slowBtn" style="padding:2px 8px;font-size:12px">Use it only when the others are busy</button></div></details>
      </div>`).join('');
    let cursor=box.firstElementChild;
    for(const child of [...next.children]){const node=existing.get(child.dataset.n)||child;if(node!==cursor)box.insertBefore(node,cursor);cursor=node.nextElementSibling;}
    while(cursor){const nextNode=cursor.nextElementSibling;cursor.remove();cursor=nextNode;}
    if(focused?.isConnected&&box.contains(focused)&&document.activeElement!==focused){focused.focus({preventScroll:true});if(selection)focused.setSelectionRange(...selection);}
    box.querySelectorAll('.lk').forEach(card => {
      const name = card.dataset.n, L = () => local.links.find(x => x.name === name);
      const lbl = card.querySelector('.lbl');
      lbl.onchange = e => { L().label = e.target.value.trim() || name; saveSoon('Renamed'); };
      lbl.onkeydown = e => { if (e.key==='Enter') e.target.blur(); };
      card.querySelector('[data-f=tog]').onclick = () => {
        const l = L(); if (l.enabled && local.links.filter(x=>x.enabled).length===1) return toast('At least one connection must stay on', true);
        l.enabled = !l.enabled; saveSoon(l.enabled ? `${l.label} added to the merge` : `${l.label} left out of the merge`); render();
      };
      card.querySelector('[data-f=slowBtn]').onclick = () => { const l = L(); l.role = 'busy'; saveSoon(`${l.label} will join when the others are busy`); };
      card.querySelector('[data-f=role]').onchange = e => { const l = L(); l.role = e.target.value; saveSoon(l.role === 'backup' ? `${l.label} is now a backup line` : l.role === 'busy' ? `${l.label} will join when the others are busy` : `${l.label} is used all the time`); };
      card.querySelector('[data-f=minus]').onclick = () => { const l=L(); if (l.weight>1){ l.weight--; saveSoon('Share updated'); render(); } };
      card.querySelector('[data-f=plus]').onclick = () => { const l=L(); if (l.weight<5){ l.weight++; saveSoon('Share updated'); render(); } };
    });
  }
  const totalW = local.links.filter(l => l.enabled && sn.links.find(s=>s.name===l.name)?.present).reduce((a,l)=>a+l.weight,0) || 1;
  const totalDown = sn.links.reduce((a,l)=>a+(l.present?l.down:0),0) || 1;
  sn.links.forEach((l,i) => {
    const card = box.querySelector(`.lk[data-n="${CSS.escape(l.name)}"]`); if (!card) return;
    const ll = local.links.find(x=>x.name===l.name) || l, c = colorOf(l.name);
    card.style.setProperty('--c', c);
    card.classList.toggle('off', !ll.enabled || !l.present);
    const f = n => card.querySelector(`[data-f=${n}]`);
    const [st, sc] = statusOf({...l, enabled: ll.enabled}, run);
    f('status').textContent = st; f('status').style.setProperty('--c', sc);
    f('tog').classList.toggle('on', ll.enabled); f('tog').setAttribute('aria-checked', ll.enabled);
    f('desc').textContent = l.present ? `${l.desc}${l.linkRate ? ' · '+l.linkRate : ''}` : 'Not connected';
    const h = holdFor(l.name), on = l.present && ll.enabled;
    const tag = (txt, title, dim) => `<span class="tag${dim ? ' dim' : ''}" title="${title}">${txt}</span>`;
    // Live figures (speed, response time, share of traffic) are on Overview; usage is on Usage. Here: who the line is.
    setHTML(f('tags'), [
      tag('Local ' + (l.present ? esc(l.ip) : '—'), 'Address on your network', !l.present),
      tag(h.publicIP ? 'Public ' + esc(h.publicIP) : 'Public address…', 'Internet address (what websites see)', !h.publicIP)
    ].join(''));
    f('missing').innerHTML = l.present ? '' : '<div class="warn-line">Unplugged or no internet. It rejoins by itself when it comes back. <button class="btn" style="margin-left:8px;padding:2px 10px" data-forget="' + esc(l.name) + '">Forget it</button></div>';
    f('w').textContent = '×'+ll.weight;
    { const r = f('role'); if (document.activeElement !== r) r.value = ll.role || ''; f('roleHint').textContent = ll.role === 'busy' ? 'Saves this line: it joins only when the other lines are nearly full.' : ll.role === 'backup' ? 'Kept in reserve: it is used only if the other lines stop working.' : 'Used all the time, together with the other lines.'; }
    { const poor = run && on && !ll.role && (l.slow || (l.quality && l.quality.label === 'Poor')); f('slowHint').hidden = !poor; }
    f('minus').disabled = ll.weight <= 1; f('plus').disabled = ll.weight >= 5;
    const smart = !!sn.smart && !local.mode.startsWith('link:') && local.mode !== 'stable';
    f('step').style.display = smart ? 'none' : '';
    const single = local.mode.startsWith('link:'), pct = single ? (local.mode === 'link:'+l.name ? 100 : 0) : smart ? ((ll.enabled && l.present) ? Math.round((l.slots||0)/8*100) : 0) : ((ll.enabled && l.present) ? Math.round(ll.weight/totalW*100) : 0);
    f('pct').textContent = (ll.enabled && l.present) ? pct+'%' : '—';
    f('bar').style.width = pct+'%';
  });
}

function setupCanvas(c){
  const r = c.getBoundingClientRect(), d = devicePixelRatio || 1;
  if (c.width !== Math.round(r.width*d) || c.height !== Math.round(r.height*d)){ c.width = Math.round(r.width*d); c.height = Math.round(r.height*d); }
  const g = c.getContext('2d'); g.setTransform(d,0,0,d,0,0); g.clearRect(0,0,r.width,r.height);
  return [g, r.width, r.height];
}
function niceMax(v){ for (const s of [1,2,5,10,20,25,40,50,80,100,150,200,250,400,500,800,1000,2000]) if (v<=s) return s; return Math.ceil(v/1000)*1000; }

function drawChart(){
  if(chartPaused)return;
  const c = $('#chart'); const [g,w,h] = setupCanvas(c); if(w<10||h<10)return;
  const P = pal(), links = S.snap.links.filter(l => l.enabled && l.history && l.history.length);
  const n = 90, padT = 30, padB = 24, padL = 12, padR = 78;
  const len = Math.max(0, ...links.map(l => l.history.length));
  const stack = []; let acc = new Array(len).fill(0);
  links.forEach(l => { const off = len - l.history.length; const idx = S.snap.links.findIndex(s=>s.name===l.name);
    const top = acc.map((a,i) => a + (i>=off ? l.history[i-off] : 0)); stack.push({l, col:colorOf(l.name), base:acc, top}); acc = top; });
  const maxM = niceMax(mbps(Math.max(...acc, 125000)) * 1.12);
  const X = i => padL + (i + n - len)/(n-1) * (w-padL-padR), Y = b => h - padB - (mbps(b)/maxM) * (h-padT-padB);
  g.font = '11px ' + css('--mono'); g.textAlign = 'left';
  for (let k=0;k<=4;k++){ const v = maxM*k/4, yy = Math.round(h - padB - (k/4)*(h-padT-padB)) + .5;
    g.strokeStyle = css('--line'); g.lineWidth = 1; g.beginPath(); g.moveTo(padL, yy); g.lineTo(w-padR, yy); g.stroke();
    g.fillStyle = css('--dim'); g.fillText(fmt(v)+' Mbps', w-padR+8, yy+4); }
  chartGeo = {len, X, Y, w, h, padL, padR, stack, acc, n};
  if (!len) return;
  stack.forEach(({col,base,top}) => {
    g.beginPath(); top.forEach((v,i)=> i? g.lineTo(X(i),Y(v)) : g.moveTo(X(i),Y(v)));
    for (let i=len-1;i>=0;i--) g.lineTo(X(i), Y(base[i]));
    g.closePath(); const gr = g.createLinearGradient(0,padT,0,h); gr.addColorStop(0,col+'77'); gr.addColorStop(1,col+'1c'); g.fillStyle = gr; g.fill();
    g.beginPath(); top.forEach((v,i)=> i? g.lineTo(X(i),Y(v)) : g.moveTo(X(i),Y(v))); g.strokeStyle = col; g.lineWidth = 1.7; g.lineJoin='round'; g.stroke();
  });
  const i = hover >= 0 ? Math.min(hover, len-1) : len-1;
  g.fillStyle = css('--text'); g.beginPath(); g.arc(X(i), Y(acc[i]||0), 3.4, 0, 7); g.fill();
  if (hover >= 0){ g.strokeStyle = css('--line2'); g.setLineDash([4,4]); g.beginPath(); g.moveTo(X(i)+.5, padT-6); g.lineTo(X(i)+.5, h-padB); g.stroke(); g.setLineDash([]); }
}
const cv = $('#chart');
cv.addEventListener('mousemove', e => {
  if (!chartGeo || !chartGeo.len) return;
  const r = cv.getBoundingClientRect(), x = e.clientX - r.left, G = chartGeo;
  const idx = Math.round((x - G.padL)/(G.w-G.padL-G.padR)*(G.n-1)) - (G.n - G.len);
  if (idx < 0 || idx >= G.len) { hover = -1; $('#tip').hidden = true; drawChart(); return; }
  hover = idx; drawChart();
  const tip = $('#tip');
  tip.innerHTML = G.stack.map(s => `<div><span><i style="background:${s.col}"></i>${esc(s.l.label)}</span><b class="num">${fmtM(s.l.history[idx-(G.len-s.l.history.length)]||0)}</b></div>`).join('') + `<div class="t"><span>Total</span><b class="num">${fmtM(G.acc[idx])} Mbps</b></div>`;
  tip.hidden = false;
  const tw = tip.offsetWidth; tip.style.left = Math.min(Math.max(8, G.X(idx) - tw/2), G.w - tw - 8) + 'px'; tip.style.top = '34px';
});
cv.addEventListener('mouseleave', () => { hover = -1; $('#tip').hidden = true; if (S) drawChart(); });


let chartPaused=false;
$('#chartPause').onclick=()=>{
  chartPaused=!chartPaused;$('#chartPause').setAttribute('aria-pressed',chartPaused);$('#chartPause').setAttribute('aria-label',chartPaused?'Resume chart updates':'Pause chart updates');$('#chartPause').innerHTML=NM.icon(chartPaused?'play':'pause');
  $('.live-label').innerHTML=chartPaused?'Paused · 90 seconds':'<i></i> Live · 90 seconds';if(!chartPaused&&S)drawChart();
};
cv.addEventListener('keydown',e=>{
  if(!['ArrowLeft','ArrowRight','Home','End'].includes(e.key)||!chartGeo?.len)return;
  e.preventDefault();
  hover=e.key==='Home'?0:e.key==='End'?chartGeo.len-1:Math.max(0,Math.min(chartGeo.len-1,(hover<0?chartGeo.len-1:hover)+(e.key==='ArrowLeft'?-1:1)));
  const readout=$('#chartReadout');readout.setAttribute('aria-live','polite');readout.textContent=`${chartGeo.len-1-hover} seconds ago: ${fmtM(chartGeo.acc[hover])} Mbps combined download`;
  if(!chartPaused)drawChart();
});
