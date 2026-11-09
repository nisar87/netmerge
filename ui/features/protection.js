/* ---------- protection ---------- */
function chipList(el, arr, key){
  const k = JSON.stringify(arr); if (el.dataset.k === k) return; el.dataset.k = k;
  el.innerHTML = arr.length ? arr.map((v,i) => `<span class="tag">${esc(v)}<button class="x" data-i="${i}" title="Remove" aria-label="Remove ${esc(v)}">✕</button></span>`).join('') : '<span style="color:var(--dim);font-size:13px">Nothing added.</span>';
  el.querySelectorAll('.x').forEach(b => b.onclick = () => { local[key].splice(b.dataset.i,1); renderRules(); saveSoon('Removed'); });
}
function renderRules(){
  chipList($('#ipList'), local.bypassIPs, 'bypassIPs');
  chipList($('#domList'), local.stableDomains, 'stableDomains');
  const v = $('#vpnLink'); if (document.activeElement !== v) setHTML(v, `<option value="">Automatic (wired first)</option>` + local.links.map(l => `<option value="${esc(l.name)}"${local.vpnLink===l.name?' selected':''}>${esc(l.label)}</option>`).join(''));
}
const addTo = (inp, key, test, msg) => { const v = $(inp).value.trim(); if (!v) return; if (!test(v)) return toast('That does not look right', true); local[key].push(v); $(inp).value=''; renderRules(); saveSoon(msg); };
$('#ipAdd').onclick = () => addTo('#ipInput','bypassIPs', v => /^\d{1,3}(\.\d{1,3}){3}(\/\d{1,2})?$/.test(v), 'Address will skip NetMerge');
$('#domAdd').onclick = () => addTo('#domInput','stableDomains', v => /^[a-z0-9.-]+\.[a-z]{2,}$/i.test(v.replace(/^https?:\/\//,'').replace(/\/.*$/,'')), 'Website kept on one connection');
$('#ipInput').onkeydown = e => { if (e.key==='Enter') $('#ipAdd').click(); };
$('#domInput').onkeydown = e => { if (e.key==='Enter') $('#domAdd').click(); };
$('#vpnLink').onchange = e => { local.vpnLink = e.target.value; saveSoon('VPN connection set'); };
