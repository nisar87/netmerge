/* ---------- DNS ---------- */
let D = null, dnsDraft = null, dnsDirty = false, dnsBusy = false;
let dnsRead = null, dnsRevision = 0, dnsDomainDraft = '';
const DNSPROV = {windows:'Windows default (recommended)', builtin:'NetMerge built-in (Cloudflare + Google)', custom:'My own servers'};
function dnsConfigDraft(config){return {provider:config.provider,secure:!!config.secure,custom:[...(config.custom||[]),'',''].slice(0,2),domains:[...(config.domains||[])]};}
async function loadDNS(force){
  if (dnsBusy || dnsRead) return;
  const read=dnsRead=new AbortController();
  try{
    const next=await NM.request('dns',{signal:read.signal});
    if(dnsRead!==read)return;
    D=next;NM.report('DNS');
    if(!dnsDraft||!dnsDirty)dnsDraft=dnsConfigDraft(D.dns);
    paintDNS();
  }catch(e){if(read.signal.aborted)return;if(page==='dns')NM.report('DNS',e,()=>loadDNS(false));return false;}
  finally{if(dnsRead===read)dnsRead=null;}
}
async function dnsAct(body){
  if(dnsBusy)return false;
  const revision=dnsRevision;
  dnsBusy=true;dnsRead?.abort();dnsRead=null;dnsControls();
  try {
    D=await api('dns',body);
    // A test does not apply configuration. A save acknowledges only its own draft.
    if(body.action!=='test'&&revision===dnsRevision){dnsDirty=false;dnsDraft=dnsConfigDraft(D.dns);}
  }catch(e){toast(e.message||'Failed',true);return false;}
  finally{dnsBusy=false;paintDNS();}
  await poll(true);await loadDNS(true);return true;
}
function provName(id){ return DNSPROV[id] || (D.providers.find(p => p.id === id)||{}).name || id; }
function dnsControls(){
  if(!D||!dnsDraft)return;
  const dr=dnsDraft,cur=D.dns;
  const changed=dr.provider!==cur.provider||dr.secure!==!!cur.secure||JSON.stringify(dr.domains)!==JSON.stringify(cur.domains||[])||JSON.stringify(dr.custom.filter(Boolean))!==JSON.stringify(cur.custom||[]);
  const apply=$('#dnsApply');if(apply)apply.disabled=dnsBusy||!changed;
  for(const id of ['dnsReset','dnsUseRec']){const button=$('#'+id);if(button)button.disabled=dnsBusy;}
  const test=$('#dnsTest');if(test)test.disabled=dnsBusy||!!D.test?.running;
}
function paintDNS(force=false){
  const box = $('#dnsBody'); if (!box || !D || !dnsDraft) return;
  const cur = D.dns, dr = dnsDraft, T = D.test || {};
  dnsControls();
  const typing = box.contains(document.activeElement) && ['INPUT'].includes(document.activeElement.tagName);
  if (typing && !force) return;
  const prov = D.providers.find(p => p.id === dr.provider);
  const choice = ['windows', ...D.providers.map(p => p.id), 'custom', ...(cur.provider === 'builtin' ? ['builtin'] : [])];
  const opts = choice.map(id => `<option value="${esc(id)}"${id===dr.provider?' selected':''}>${esc(provName(id))}</option>`).join('');
  const isReal = prov || dr.provider === 'custom';
  const desc = dr.provider === 'windows' ? 'Windows and your router keep looking up names, exactly as before. Safest, and the only choice that always works with a company VPN.'
    : dr.provider === 'builtin' ? 'The pair NetMerge has always used when this was switched on. You can choose a specific provider instead.'
    : dr.provider === 'custom' ? 'Your own servers, for example a Pi-hole or a DNS from your company. Up to two IPv4 addresses.' : (prov ? prov.desc : '');
  const status = cur.provider === 'windows' ? 'Windows default' : esc(provName(cur.provider)) + (cur.secure && cur.provider !== 'custom' && cur.provider !== 'builtin' ? ' · encrypted' : '');
  const chips = dr.domains.map((d,i) => `<span class="tag">${esc(d)}<button class="x" data-dx="${i}" aria-label="Remove ${esc(d)}">×</button></span>`).join('');
  const rowsT = (T.rows||[]);
  const lineN = T.lineNames || {};
  const head = (T.lines||[]).map(l => `<th class="n">${esc(lineN[l]||l)}</th>`).join('');
  const bestId = T.rec && T.rec.provider ? (T.rec.provider + (T.rec.provider==='custom'?'':(T.rec.secure?':doh':':plain'))) : '';
  const cell = s => !s ? '<td class="n">–</td>' : s.median < 0 ? '<td class="n" style="color:var(--bad)">no answer</td>' : `<td class="n">${Math.round(s.median)} ms</td>`;
  const tbl = rowsT.length ? `<table class="dnstbl"><thead><tr><th>Server</th>${head}<th class="n">Typical</th><th class="n">Failed</th><th>Notes</th></tr></thead><tbody>${rowsT.map(r => `<tr class="${r.id===bestId?'best':''}"><td>${esc(r.label)}${r.id===bestId?' <b>· recommended</b>':''}</td>${(T.lines||[]).map(l => cell(r.lines[l])).join('')}<td class="n"><b>${r.median<0?'–':Math.round(r.median)+' ms'}</b></td><td class="n">${r.median<0?'100':Math.round(r.fail)}%</td><td>${r.hijack?'<span style="color:var(--warn)">Redirects names that do not exist</span>':''}</td></tr>`).join('')}</tbody></table>` : '';
  const rec = T.rec ? `<div class="dnsrec"><b>${esc(T.rec.text)}</b>${(T.rec.notes||[]).map(n => `<div class="hint2" style="margin-top:6px">${esc(n)}</div>`).join('')}${T.rec.provider ? `<div class="row" style="margin-top:10px"><button class="btn" id="dnsUseRec">Use this</button></div>` : ''}</div>` : '';
  const prog = T.running ? `<div class="dnsprog"><div style="width:${T.total ? Math.round(T.progress/T.total*100) : 0}%"></div></div><div class="hint2">Testing every server through each connection… ${T.total ? Math.round(T.progress/T.total*100) : 0}%</div>` : '';
  const html = `
  ${cur.fallback ? `<div class="dnswarn"><b>NetMerge went back to the Windows DNS.</b> The servers you chose stopped answering (or the engine would not start with them). Run the test below to find servers that work on your connections.</div>` : ''}
  <section class="card page-hero"><span class="hero-icon">${NM.icon('globe')}</span><div class="hero-text"><h2>Using now: ${status}</h2><p class="hint2">DNS turns website names into addresses. It does not change how your lines are merged.</p></div></section>
  <div class="card" style="margin-top:16px"><h3 style="margin:0 0 10px">Choose DNS servers</h3>
    <div class="dnsrow"><div><b>Servers</b><div class="d">${esc(desc)}</div></div><select class="sel" id="dnsProv" aria-label="DNS servers" style="min-width:250px">${opts}</select></div>
    ${prov ? `<div class="dnsrow"><div><b>Encrypt lookups (DNS-over-HTTPS)</b><div class="d">Your internet provider can no longer see which websites you look up. The first lookup is a little slower. If a network blocks it, NetMerge goes back to Windows DNS by itself within a few minutes.</div></div><button class="tog${dr.secure?' on':''}" id="dnsSecure" role="switch" aria-checked="${dr.secure}" aria-label="Encrypt lookups"></button></div>` : ''}
    ${dr.provider === 'custom' ? `<div class="dnsrow"><div><b>Addresses</b><div class="d">For example 9.9.9.9 or 192.168.1.2</div></div><div class="row" style="gap:8px"><input class="inp" id="dnsC0" value="${esc(dr.custom[0]||'')}" placeholder="First" style="width:140px" aria-label="First DNS address"><input class="inp" id="dnsC1" value="${esc(dr.custom[1]||'')}" placeholder="Second (optional)" style="width:140px" aria-label="Second DNS address"></div></div>` : ''}
    ${isReal ? `<div class="dnsrow"><div><b>Company or VPN names</b><div class="d">Names that only your company's DNS knows (for example intranet.mycompany.com). They keep using the Windows DNS so they still open. Leave empty if you have no company VPN.</div></div><div class="row" style="gap:8px"><input class="inp" id="dnsDom" value="${esc(dnsDomainDraft)}" placeholder="mycompany.com" style="width:170px" aria-label="Company domain"><button class="btn" id="dnsDomAdd">Add</button></div></div><div class="listchips" style="margin:4px 0 8px">${chips}</div>
    <div class="dnswarn"><b>Working from a company network or VPN?</b> Private company websites and shared drives often stop opening with a different DNS. Add the company domain above, or keep the Windows default.</div>` : ''}
    <div class="row" style="gap:8px;margin-top:14px;flex-wrap:wrap"><button class="btn" id="dnsApply"${(dr.provider===cur.provider && dr.secure===!!cur.secure && JSON.stringify(dr.domains)===JSON.stringify(cur.domains) && JSON.stringify(dr.custom.filter(Boolean))===JSON.stringify(cur.custom))?' disabled':''} style="background:var(--accent);color:var(--accent-ink);border-color:var(--accent)">Apply</button>${cur.provider !== 'windows' ? '<button class="btn" id="dnsReset">Back to Windows default</button>' : ''}<span class="hint2">Applying restarts merging for a moment.</span></div>
  </div>
  <div class="card" style="margin-top:16px"><h3 style="margin:0 0 4px">Find the best DNS for my connections</h3>
    <div class="hint2">Sends real lookups through each connection to every provider (plain and encrypted), times them and checks for redirects. Takes about half a minute. Nothing is changed until you press Use this.</div>
    <div class="row" style="margin-top:10px"><button class="btn" id="dnsTest"${T.running?' disabled':''}>${T.running ? 'Testing…' : (T.rows && T.rows.length ? 'Test again' : 'Start the test')}</button>${T.done ? `<span class="hint2">Last run ${ago(T.done)}</span>` : ''}</div>
    ${prog}${T.error ? `<div class="dnswarn">${esc(T.error)}</div>` : ''}${rec}${tbl}
  </div>`;
  setHTML(box,html,{force});dnsControls();
}
document.addEventListener('input', e => {
  if (!dnsDraft) return;
  if(e.target.id==='dnsDom')dnsDomainDraft=e.target.value;
  if (e.target.id === 'dnsC0') { dnsDraft.custom[0] = e.target.value; dnsDirty = true; dnsRevision++; }
  if (e.target.id === 'dnsC1') { dnsDraft.custom[1] = e.target.value; dnsDirty = true; dnsRevision++; }
  dnsControls();
});
document.addEventListener('change', e => {
  if (e.target.id === 'dnsProv' && dnsDraft) { dnsDraft.provider = e.target.value; if (!D.providers.find(p=>p.id===dnsDraft.provider)) dnsDraft.secure = false; dnsDirty = true; dnsRevision++;paintDNS(true); }
});
document.addEventListener('click', async e => {
  const t = e.target.closest('button'); if (!t || !D) return;
  if (t.id === 'dnsSecure') { dnsDraft.secure = !dnsDraft.secure; dnsDirty = true; dnsRevision++;paintDNS(true); }
  else if (t.dataset.dx !== undefined) { dnsDraft.domains.splice(+t.dataset.dx, 1); dnsDirty = true; dnsRevision++;paintDNS(true); }
  else if (t.id === 'dnsDomAdd') { const v = $('#dnsDom').value.trim(); if (!/^([a-z0-9-]+\.)+[a-z]{2,}$/i.test(v.replace(/^\*\./,''))) return toast('Enter a website address like mycompany.com', true); dnsDraft.domains.push(v.replace(/^\*\./,'').toLowerCase()); dnsDomainDraft='';dnsDirty = true; dnsRevision++;paintDNS(true); }
  else if (t.id === 'dnsApply') { if (await dnsAct({action:'set', provider: dnsDraft.provider, secure: dnsDraft.secure, custom: dnsDraft.custom.filter(Boolean), domains: dnsDraft.domains})) toast('DNS applied'); }
  else if (t.id === 'dnsReset') { if (await dnsAct({action:'reset'})) toast('Back to Windows DNS'); }
  else if (t.id === 'dnsTest') { await dnsAct({action:'test'}); }
  else if (t.id === 'dnsUseRec') { const r = D.test.rec; if (await dnsAct({action:'set', provider: r.provider, secure: r.secure, custom: D.dns.custom, domains: D.dns.domains})) toast('DNS applied'); }
});
NM.schedule('dns',()=>loadDNS(false), 1500, ()=>page==='dns'&&!document.hidden);
