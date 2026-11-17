/* ---------- apps ---------- */
const appMemo = new Map();   // process → last reading and when it was seen (rows stay two minutes after going quiet)
let appSort = (() => { try { return localStorage.getItem('nm-app-sort') === 'down' ? 'down' : 'name'; } catch { return 'name'; } })();
function targetOptions(sel, withDefault){
  const o = [];
  if (withDefault) o.push(['', 'Automatic']);
  o.push(['fast','Combine · fastest'], ['stable','Combine · steady']);
  local.links.forEach(l => o.push(['link:'+l.name, 'Only '+l.label]));
  o.push(['bypass','Skip NetMerge']);
  return o.map(([v,t]) => `<option value="${esc(v)}"${v===sel?' selected':''}>${esc(t)}</option>`).join('');
}
// a small coloured initial for each app, always the same colour for the same name
const appAvatar = name => { let h = 0; for (const ch of String(name)) h = (h*31 + ch.charCodeAt(0)) >>> 0; return `<span class="app-av" style="--c:var(--l${h%5+1})" aria-hidden="true">${esc(String(name).replace(/\.exe$/i,'').slice(0,1).toUpperCase() || '?')}</span>`; };
function renderApps(){
  const at = $('#appTarget');
  if (!at.dataset.init || at.dataset.links !== local.links.map(l=>l.name).join('|')){
    at.dataset.init = 1; at.dataset.links = local.links.map(l=>l.name).join('|');
    const pref = local.links.find(l => l.name !== local.links[0]?.name) || local.links[0];
    at.innerHTML = targetOptions(pref ? 'link:'+pref.name : 'fast', false);
  }
  setHTML($('#seenApps'), (local.seenApps||[]).map(a => `<option value="${esc(a)}">`).join(''));
  const rt = $('#rulesTbl');
  if (!(document.activeElement && rt.contains(document.activeElement))){
    setHTML(rt, '<tr><th>App</th><th>Uses</th><th style="width:44px"></th></tr>' + (local.appRules.length ? local.appRules.map(r => `
      <tr><td class="proc"><span class="app-name">${appAvatar(r.process)}${esc(r.process)}</span></td><td><select class="sel" data-process="${esc(r.process)}" aria-label="Rule for ${esc(r.process)}">${targetOptions(r.target, false)}</select></td><td><button class="x" data-del="${esc(r.process)}" title="Remove rule" aria-label="Remove rule">✕</button></td></tr>`).join('')
      : '<tr><td colspan="3" class="empty">No rules yet — apps follow the traffic mode.</td></tr>'));
    // Focus preserves this DOM through polling; resolve the process in the latest rules.
    rt.querySelectorAll('select').forEach(s => s.onchange = () => {
      const rule=local.appRules.find(r=>r.process===s.dataset.process);
      if (rule) { rule.target=s.value; saveSoon(`Rule updated. ${rule.process}'s open connections reconnect under it now`); }
      else toast('This app rule was removed in another window.',true);
      s.blur(); renderApps();
    });
    rt.querySelectorAll('[data-del]').forEach(b => b.onclick = () => {
      const index=local.appRules.findIndex(r=>r.process===b.dataset.del);
      if (index>=0) { local.appRules.splice(index,1); saveSoon('Rule removed'); }
      b.blur(); renderApps();
    });
  }
  const tb = $('#appsTbl'), live = S.snap.apps || [], P = pal();
  // Rows must stay where they are while you work with them: the list is in a fixed order (A–Z unless you choose
  // "Most download"), and an app whose connections all closed stays listed, as quiet, for two minutes instead of
  // vanishing and making the rows below it jump.
  const now = Date.now();
  if (!S.snap.running) appMemo.clear();
  live.forEach(a => appMemo.set(a.process, {a, seen: now}));
  for (const [p, m] of appMemo) if (now - m.seen > 120000) appMemo.delete(p);
  const q = ($('#appSearch')?.value || '').trim().toLowerCase();
  const liveNames = new Set(live.map(a => a.process));
  let apps = [...appMemo.values()].map(m => liveNames.has(m.a.process) ? m.a : {...m.a, down: 0, up: 0, conns: 0, links: {}, quiet: true});
  const total = apps.length;
  if (q) apps = apps.filter(a => a.process.toLowerCase().includes(q) || a.process.toLowerCase().replace(/\.exe$/, '').includes(q));
  apps.sort(appSort === 'down' ? (x, y) => (y.down || 0) - (x.down || 0) || x.process.localeCompare(y.process) : (x, y) => (x.process === 'system') - (y.process === 'system') || x.process.localeCompare(y.process));
  { const down = apps.reduce((a,x)=>a+(x.down||0),0), stat = (v,l) => `<div class="hero-stat"><b>${v}</b><span>${l}</span></div>`;
    setHTML($('#appsStats'), stat(S.snap.running ? live.length : '—', 'apps online') + stat(S.snap.running ? fmtM(down) : '—', 'Mbps down') + stat((local.appRules||[]).length, (local.appRules||[]).length === 1 ? 'rule' : 'rules')); }
  const maxDown = Math.max(1, ...apps.map(a => a.down || 0));
  if (document.activeElement && tb.contains(document.activeElement)) return;
  setHTML(tb, '<tr><th>App</th><th>Connections used</th><th class="r">Download</th><th class="r">Upload</th><th>Rule</th></tr>' +
    (!S.snap.running ? '<tr><td colspan="5" class="empty">Turn merging on to see which apps are using the internet.</td></tr>' :
     !apps.length ? `<tr><td colspan="5" class="empty">${q && total ? `No app matches “${esc(q)}”.` : 'No traffic right now.'}</td></tr>` :
     apps.map(a => {
       if (a.quiet) { const rule = local.appRules.find(r => r.process === a.process)?.target || '';
         return `<tr class="quiet"><td class="proc"><span class="app-name">${appAvatar(a.process)}${esc(a.process)}</span></td><td><div class="app-lines">Quiet · no open connections</div></td><td class="r num">—</td><td class="r num">—</td><td>${a.process==='system'?'':`<select class="sel" data-p="${esc(a.process)}" aria-label="Rule for ${esc(a.process)}">${targetOptions(rule, true)}</select>`}</td></tr>`; }
       const tot = Object.values(a.links).reduce((x,y)=>x+y,0) || 1;
       const split = Object.entries(a.links).map(([n,c]) => `<i title="${esc(n==='bypass'?'Skip NetMerge':labelOf(n))}: ${c}" style="width:${c/tot*100}%;background:${n==='bypass'?'var(--dim)':colorOf(n)}"></i>`).join('');
       const names = Object.keys(a.links).map(n => n==='bypass' ? 'Skip' : labelOf(n)).join(' + ');
       const rule = local.appRules.find(r => r.process === a.process)?.target || '';
       return `<tr><td class="proc"><span class="app-name">${appAvatar(a.process)}${esc(a.process)}</span></td><td><div class="split" aria-hidden="true">${split}</div><div class="app-lines">${esc(names)} · ${a.conns} open</div></td><td class="r num"><span class="app-rate"><span><b>${fmtM(a.down)}</b> Mbps</span><span class="app-bar" aria-hidden="true"><i style="width:${((a.down||0)/maxDown*100).toFixed(1)}%"></i></span></span></td><td class="r num">${fmtM(a.up)} Mbps</td>
         <td>${a.process==='system'?'':`<select class="sel" data-p="${esc(a.process)}" aria-label="Rule for ${esc(a.process)}">${targetOptions(rule, true)}</select>`}</td></tr>`;
     }).join('')));
  tb.querySelectorAll('select').forEach(s => s.onchange = () => {
    const p = s.dataset.p, i = local.appRules.findIndex(r => r.process === p);
    if (!s.value){ if (i>=0) local.appRules.splice(i,1); } else if (i>=0) local.appRules[i].target = s.value; else local.appRules.push({process:p, target:s.value});
    $('#rulesTbl').dataset.h = ''; saveSoon(`Rule for ${p} saved. Its open connections reconnect under it now`); s.blur();
  });
}
$('#appAdd').onclick = () => {
  let p = $('#appInput').value.trim().toLowerCase(); if (!p) return toast('Type an app name first', true);
  if (!p.endsWith('.exe')) p += '.exe';
  const t = $('#appTarget').value, i = local.appRules.findIndex(r => r.process === p);
  if (i>=0) local.appRules[i].target = t; else local.appRules.push({process:p, target:t});
  $('#appInput').value=''; $('#rulesTbl').dataset.h=''; renderApps(); saveSoon(`Rule for ${p} added. If it is running, its open connections reconnect under it now`);
};
$('#appInput').onkeydown = e => { if (e.key==='Enter') $('#appAdd').click(); };

// search and order of the "Using the internet now" list
$('#appSearch').oninput = () => renderApps();
$('#appSearch').onkeydown = e => { if (e.key === 'Escape') { e.target.value = ''; renderApps(); } };
function markAppSort(){ $$('#appSort button').forEach(b => { b.classList.toggle('sel', b.dataset.v === appSort); b.setAttribute('aria-pressed', b.dataset.v === appSort); }); }
$$('#appSort button').forEach(b => b.onclick = () => { appSort = b.dataset.v; try { localStorage.setItem('nm-app-sort', appSort); } catch {} markAppSort(); renderApps(); });
markAppSort();
