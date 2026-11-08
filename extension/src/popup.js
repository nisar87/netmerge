const api = globalThis.browser || globalThis.chrome;
const $ = (id) => document.getElementById(id);
const ask = (m) => new Promise((res) => api.runtime.sendMessage(m, (r) => res(r || {})));
function show(id) { for (const v of document.querySelectorAll('.v')) v.hidden = v.id !== id; }
function fmt(b) { return b > 1048576 ? (b / 1048576).toFixed(1) + ' MB/s' : Math.round(b / 1024) + ' KB/s'; }

async function paint() {
  const ping = await ask({ type: 'ping' });
  if (ping.status === 404) return show('v-off'); // NetMerge runs, but its download manager is switched off
  if (!ping.ok) return show('v-down');
  const s = await api.storage.local.get(['token', 'localOn', 'ignore']);
  if (!s.token) return show('v-pair');
  const st = await ask({ type: 'refresh' });
  if (!st.ok) return show('v-pair'); // the token was refused: NetMerge forgot this browser
  show('v-on');
  const d = st.data;
  $('stat').textContent = d.active ? d.active + ' downloading · ' + fmt(d.speed) : 'Connected. Nothing downloading now.';
  $('on').checked = s.localOn !== false && d.enabled !== false;
  $('on').disabled = d.enabled === false;
  $('min').textContent = d.enabled === false ? 'Switched off in NetMerge.' : 'Files of ' + d.minMB + ' MB and more are sent (change this in NetMerge).';
  $('ign').value = (s.ignore || []).join('\n');
  try { const has = await api.permissions.contains({ origins: ['<all_urls>'] }); $('perm').hidden = has; } catch (e) { $('perm').hidden = true; }
}

$('retry').onclick = paint;
$('retry2').onclick = paint;
$('pair').onclick = async () => {
  $('perr').textContent = '';
  const r = await ask({ type: 'pair', code: $('code').value, name: /Edg\//.test(navigator.userAgent) ? 'Edge' : /Firefox\//.test(navigator.userAgent) ? 'Firefox' : 'Chrome' });
  if (r.ok) paint(); else $('perr').textContent = r.error || 'Could not connect';
};
$('code').onkeydown = (e) => { if (e.key === 'Enter') $('pair').click(); };
$('on').onchange = () => api.storage.local.set({ localOn: $('on').checked });
$('ign').onchange = async () => {
  const list = $('ign').value.split(/[\s,;]+/).map((s) => s.replace(/^https?:\/\//, '').replace(/\/.*$/, '').trim().toLowerCase()).filter(Boolean).slice(0, 200);
  await api.storage.local.set({ ignore: list });
};
$('grant').onclick = async () => { try { await api.permissions.request({ origins: ['<all_urls>'] }); } catch (e) { } paint(); };
$('unpair').onclick = async () => { await ask({ type: 'unpair' }); paint(); };
paint();
