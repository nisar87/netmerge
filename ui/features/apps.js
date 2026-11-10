/* ---------- apps ---------- */
function targetOptions(sel, withDefault){
  const o = [];
  if (withDefault) o.push(['', 'Automatic']);
  o.push(['fast','Combine · fastest'], ['stable','Combine · steady']);
  local.links.forEach(l => o.push(['link:'+l.name, 'Only '+l.label]));
  o.push(['bypass','Skip NetMerge']);
  return o.map(([v,t]) => `<option value="${esc(v)}"${v===sel?' selected':''}>${esc(t)}</option>`).join('');
}
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
      <tr><td class="proc">${esc(r.process)}</td><td><select class="sel" data-process="${esc(r.process)}" aria-label="Rule for ${esc(r.process)}">${targetOptions(r.target, false)}</select></td><td><button class="x" data-del="${esc(r.process)}" title="Remove rule" aria-label="Remove rule">✕</button></td></tr>`).join('')
      : '<tr><td colspan="3" class="empty">No rules yet — apps follow the Overview setting.</td></tr>'));
    // Focus preserves this DOM through polling; resolve the process in the latest rules.
    rt.querySelectorAll('select').forEach(s => s.onchange = () => {
      const rule=local.appRules.find(r=>r.process===s.dataset.process);
      if (rule) { rule.target=s.value; saveSoon('Rule updated'); }
      else toast('This app rule was removed in another window.',true);
      s.blur(); renderApps();
    });
    rt.querySelectorAll('[data-del]').forEach(b => b.onclick = () => {
      const index=local.appRules.findIndex(r=>r.process===b.dataset.del);
      if (index>=0) { local.appRules.splice(index,1); saveSoon('Rule removed'); }
      b.blur(); renderApps();
    });
  }
  const tb = $('#appsTbl'), apps = S.snap.apps || [], P = pal();
  if (document.activeElement && tb.contains(document.activeElement)) return;
  setHTML(tb, '<tr><th>App</th><th>Connections used</th><th class="r">Download</th><th class="r">Upload</th><th>Rule</th></tr>' +
    (!S.snap.running ? '<tr><td colspan="5" class="empty">Turn merging on to see which apps are using the internet.</td></tr>' :
     !apps.length ? '<tr><td colspan="5" class="empty">No traffic right now.</td></tr>' :
     apps.map(a => {
       const tot = Object.values(a.links).reduce((x,y)=>x+y,0) || 1;
       const split = Object.entries(a.links).map(([n,c]) => `<i title="${esc(n==='bypass'?'Skip NetMerge':labelOf(n))}: ${c}" style="width:${c/tot*100}%;background:${n==='bypass'?'var(--dim)':colorOf(n)}"></i>`).join('');
       const names = Object.keys(a.links).map(n => n==='bypass' ? 'Skip' : labelOf(n)).join(' + ');
       const rule = local.appRules.find(r => r.process === a.process)?.target || '';
       return `<tr><td class="proc">${esc(a.process)}</td><td><div class="split" aria-hidden="true">${split}</div><div style="font-size:12px;color:var(--dim);margin-top:3px">${esc(names)} · ${a.conns} conn</div></td><td class="r num">${fmtM(a.down)} Mbps</td><td class="r num">${fmtM(a.up)} Mbps</td>
         <td>${a.process==='system'?'':`<select class="sel" data-p="${esc(a.process)}" aria-label="Rule for ${esc(a.process)}">${targetOptions(rule, true)}</select>`}</td></tr>`;
     }).join('')));
  tb.querySelectorAll('select').forEach(s => s.onchange = () => {
    const p = s.dataset.p, i = local.appRules.findIndex(r => r.process === p);
    if (!s.value){ if (i>=0) local.appRules.splice(i,1); } else if (i>=0) local.appRules[i].target = s.value; else local.appRules.push({process:p, target:s.value});
    $('#rulesTbl').dataset.h = ''; saveSoon(`Rule for ${p} saved`); s.blur();
  });
}
$('#appAdd').onclick = () => {
  let p = $('#appInput').value.trim().toLowerCase(); if (!p) return toast('Type an app name first', true);
  if (!p.endsWith('.exe')) p += '.exe';
  const t = $('#appTarget').value, i = local.appRules.findIndex(r => r.process === p);
  if (i>=0) local.appRules[i].target = t; else local.appRules.push({process:p, target:t});
  $('#appInput').value=''; $('#rulesTbl').dataset.h=''; renderApps(); saveSoon(`Rule for ${p} added`);
};
$('#appInput').onkeydown = e => { if (e.key==='Enter') $('#appAdd').click(); };
