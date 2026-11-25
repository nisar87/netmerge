/* ---------- protection ---------- */
function chipList(el, arr, key, show = v => v){
  const k = JSON.stringify(arr); if (el.dataset.k === k) return; el.dataset.k = k;
  el.innerHTML = arr.length ? arr.map((v,i) => `<span class="tag"${show(v) !== v ? ` title="${esc(v)}"` : ''}>${esc(show(v))}<button class="x" data-i="${i}" title="Remove" aria-label="Remove ${esc(v)}">✕</button></span>`).join('') : '<span style="color:var(--dim);font-size:13px">Nothing added.</span>';
  el.querySelectorAll('.x').forEach(b => b.onclick = () => { local[key].splice(b.dataset.i,1); renderRules(); saveSoon('Removed'); });
}
function renderRules(){
  chipList($('#ipList'), local.bypassIPs, 'bypassIPs', ipShown);
  chipList($('#domList'), local.stableDomains, 'stableDomains');
  const v = $('#vpnLink'); if (document.activeElement !== v) setHTML(v, `<option value="">Automatic (wired first)</option>` + local.links.map(l => `<option value="${esc(l.name)}"${local.vpnLink===l.name?' selected':''}>${esc(l.label)}</option>`).join(''));
}
const addTo = (inp, key, test, msg) => { const v = $(inp).value.trim(); if (!v) return; if (!test(v)) return toast('That does not look right', true); local[key].push(v); $(inp).value=''; renderRules(); saveSoon(msg); };
/* Addresses that skip NetMerge. Windows and the engine take ranges in the network form 198.51.100.0/24; people
   write 198.51.100.x. Both are accepted: an x or * at the end becomes the matching range, and whole ranges are
   shown back the way people write them. A wildcard in the middle (198.x.100.5) is not one range, so it is refused. */
function ipParse(text){
  const v = text.trim().toLowerCase().replace(/\s+/g, '');
  const octet = p => /^\d{1,3}$/.test(p) && Number(p) <= 255;
  let m = v.match(/^([^/]+)\/(\d{1,2})$/);
  if (m) {
    const parts = m[1].split('.'), bits = Number(m[2]);
    if (parts.length !== 4 || !parts.every(octet)) return {error: 'Write an address as four numbers from 0 to 255, like 203.0.113.5'};
    if (bits > 32) return {error: 'The number after / is 0 to 32 (24 means the last part can be anything)'};
    return {value: `${parts.join('.')}/${bits}`};
  }
  const parts = v.split('.');
  if (parts.length !== 4) return {error: 'Write an address as four numbers, like 203.0.113.5, or a range like 198.51.100.x'};
  const wild = parts.map(p => p === 'x' || p === '*');
  const first = wild.indexOf(true);
  if (first === -1) return parts.every(octet) ? {value: parts.join('.')} : {error: 'Each part of an address is a number from 0 to 255'};
  if (first === 0) return {error: 'That would be every address on the internet. Keep at least the first part, like 198.x.x.x'};
  if (wild.slice(first).some(w => !w)) return {error: 'Put x only at the end, like 198.51.100.x. 198.x.100.5 is not one range of addresses'};
  if (!parts.slice(0, first).every(octet)) return {error: 'Each part of an address is a number from 0 to 255'};
  return {value: `${[...parts.slice(0, first), ...Array(4 - first).fill('0')].join('.')}/${first * 8}`, wildcard: true};
}
// a whole range written the way people write it (198.51.100.0/24 → 198.51.100.x); anything else as it is
function ipShown(v){
  const m = /^(\d+)\.(\d+)\.(\d+)\.(\d+)\/(8|16|24|32)$/.exec(v); if (!m) return v;
  const bits = Number(m[5]), keep = bits / 8, parts = m.slice(1, 5);
  if (parts.slice(keep).some(p => p !== '0')) return v;
  return [...parts.slice(0, keep), ...Array(4 - keep).fill('x')].join('.');
}
$('#ipAdd').onclick = () => {
  const inp = $('#ipInput'), text = inp.value.trim(); if (!text) return;
  const r = ipParse(text);
  if (r.error) return toast(r.error, true);
  const same = v => v === r.value || v === r.value + '/32' || v + '/32' === r.value;
  if (local.bypassIPs.some(same)) { inp.value = ''; return toast(`${ipShown(r.value)} is already on the list`); }
  local.bypassIPs.push(r.value); inp.value = ''; renderRules();
  const shown = ipShown(r.value);
  saveSoon(r.wildcard ? `Addresses ${shown.replace(/x/g, '0')} to ${shown.replace(/x/g, '255')} will skip NetMerge` : 'Address will skip NetMerge');
};
$('#domAdd').onclick = () => addTo('#domInput','stableDomains', v => /^[a-z0-9.-]+\.[a-z]{2,}$/i.test(v.replace(/^https?:\/\//,'').replace(/\/.*$/,'')), 'Website kept on one connection');
$('#ipInput').onkeydown = e => { if (e.key==='Enter') $('#ipAdd').click(); };
$('#domInput').onkeydown = e => { if (e.key==='Enter') $('#domAdd').click(); };
$('#vpnLink').onchange = e => { local.vpnLink = e.target.value; saveSoon('VPN connection set'); };
