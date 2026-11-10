const api = globalThis.browser || globalThis.chrome;
const $ = (id) => document.getElementById(id);

// Keep the existing messages intact; surface transport failures in the popup.
function ask(message) {
  if (globalThis.browser) return Promise.resolve(api.runtime.sendMessage(message)).then((reply) => reply || {});
  return new Promise((resolve, reject) => {
    api.runtime.sendMessage(message, (reply) => {
      const error = api.runtime.lastError;
      if (error) reject(new Error(error.message));
      else resolve(reply || {});
    });
  });
}

function icon(name) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('aria-hidden', 'true');
  const use = document.createElementNS('http://www.w3.org/2000/svg', 'use');
  use.setAttribute('href', '#i-' + name);
  svg.appendChild(use);
  return svg;
}

function feedback(id, message = '', tone = 'muted') {
  const target = typeof id === 'string' ? $(id) : id;
  target.textContent = '';
  target.dataset.tone = tone;
  if (!message) return;
  target.appendChild(icon(tone === 'error' || tone === 'warning' ? 'circle-alert' : tone === 'success' ? 'circle-check' : tone === 'pending' ? 'clock-3' : 'circle-pause'));
  const text = document.createElement('span');
  text.textContent = message;
  target.appendChild(text);
}

function buttonState(button, label, pending = false) {
  button.disabled = pending;
  button.setAttribute('aria-busy', String(pending));
  button.querySelector('span').textContent = label;
  button.querySelector('use').setAttribute('href', '#i-' + (pending ? 'clock-3' : button.dataset.icon));
}

function preserveFocus(control) {
  const wasFocused = document.activeElement === control;
  return () => {
    // Disabling a pending control may blur it. Restore only focus the browser
    // discarded, never focus the user moved to another control while waiting.
    if (wasFocused && document.activeElement === document.body && !control.disabled && control.getClientRects().length) control.focus();
  };
}

function show(id, focus = false) {
  for (const view of document.querySelectorAll('.v')) view.hidden = view.id !== id;
  if (focus) (id === 'v-pair' ? $('code') : $(id).querySelector('h2')).focus();
}

function showDown(message, focus = false, title = 'NetMerge is not running') {
  $('down-title').textContent = title;
  $('down-detail').textContent = message;
  show('v-down', focus);
}

function fmt(bytes) {
  if (!Number.isFinite(bytes) || bytes < 0) return 'Speed unavailable';
  return bytes > 1048576 ? (bytes / 1048576).toFixed(1) + ' MB/s' : Math.round(bytes / 1024) + ' KB/s';
}

function handoffStatus() {
  feedback('handoff-status', $('on').checked ? '' : 'Automatic downloads stay in this browser.');
}

async function paint(focus = false) {
  let ping;
  try { ping = await ask({ type: 'ping' }); }
  catch (error) { return showDown('The extension could not check NetMerge. Try again or reopen this popup.', focus, 'Connection unavailable'); }
  if (ping.status === 404) return show('v-off', focus);
  if (!ping.ok) return showDown('Open NetMerge on this device, then try again.', focus);

  let settings;
  try { settings = await api.storage.local.get(['token', 'localOn', 'ignore', 'types']); }
  catch (error) { return showDown('Your browser preferences could not be loaded. Try again.', focus, 'Preferences unavailable'); }
  if (!settings.token) return show('v-pair', focus);

  let status;
  try { status = await ask({ type: 'refresh' }); }
  catch (error) { return showDown('The extension could not read the connection status. Try again.', focus, 'Connection unavailable'); }
  if (status.status === 404) return show('v-off', focus);
  if (status.status === 0) return showDown('Open NetMerge on this device, then try again.', focus);
  if (!status.ok) return show('v-pair', focus); // NetMerge forgot this browser.

  const data = status.data || {};
  $('stat').textContent = Number.isFinite(data.active)
    ? (data.active > 0 ? data.active + ' downloading · ' + fmt(data.speed) : 'Nothing downloading right now.')
    : 'Download activity unavailable.';
  $('on').checked = settings.localOn !== false && data.enabled !== false;
  $('on').disabled = data.enabled === false;
  $('min').textContent = data.enabled === false
    ? 'Automatic handoff is off in NetMerge. Enable it in Downloads.'
    : Number.isFinite(data.minMB)
      ? 'Files of ' + data.minMB + ' MB and more are sent. Change the limit in NetMerge.'
      : 'The minimum file size is set in NetMerge.';
  handoffStatus();
  $('ign').value = (settings.ignore || []).join('\n');
  $('typ').value = (settings.types || []).join(', ');
  show('v-on', focus);

  try { $('perm').hidden = await api.permissions.contains({ origins: ['<all_urls>'] }); }
  catch (error) {
    $('perm').hidden = false;
    feedback('perm-feedback', 'Could not check site access. Try Allow for all sites.', 'error');
  }
  await paintMedia();
}

async function paintMedia() {
  const box = $('mlist');
  box.textContent = '';
  feedback('media-feedback');
  $('media').hidden = true;
  try {
    const tabs = await api.tabs.query({ active: true, currentWindow: true });
    if (!tabs[0]) return;
    const reply = await ask({ type: 'media', tab: tabs[0].id });
    const list = reply.list || [];
    $('media').hidden = !list.length;
    $('media-count').textContent = list.length > 8 ? '8 of ' + list.length : String(list.length);
    for (const media of list.slice(0, 8)) {
      const row = document.createElement('li');
      row.className = 'media-row';
      const kind = media.kind === 'stream' ? 'Stream' : media.kind === 'audio' ? 'Audio' : 'Video';
      const mediaIcon = icon(media.kind === 'stream' ? 'radio' : media.kind === 'audio' ? 'music' : 'film');
      mediaIcon.classList.add('media-icon');
      let name = media.url.split('?')[0].split('/').pop() || media.url;
      // A malformed escape in one URL must not hide every detected file.
      try { name = decodeURIComponent(name); } catch (error) { /* Keep the original filename. */ }
      const description = document.createElement('div');
      const label = document.createElement('span');
      label.className = 'media-name';
      label.textContent = name;
      const meta = document.createElement('span');
      meta.className = 'media-meta';
      meta.textContent = kind + (media.size > 0 ? ' · ' + (media.size / 1048576).toFixed(1) + ' MB' : ' · Size unknown');
      description.append(label, meta);

      const button = document.createElement('button');
      button.className = 'primary';
      button.dataset.icon = 'download';
      button.append(icon('download'), document.createElement('span'));
      buttonState(button, 'Download');
      button.setAttribute('aria-label', 'Download ' + name);
      const notice = document.createElement('p');
      notice.className = 'feedback';
      notice.setAttribute('role', 'status');
      notice.setAttribute('aria-atomic', 'true');
      button.onclick = async () => {
        const restoreFocus = preserveFocus(button);
        buttonState(button, 'Sending…', true);
        button.setAttribute('aria-label', 'Sending ' + name);
        feedback(notice, 'Sending to NetMerge…', 'pending');
        try {
          const result = await ask({ type: 'sendMedia', url: media.url, referrer: tabs[0].url || '' });
          if (!result.ok) throw new Error('Send failed');
          buttonState(button, result.duplicate ? 'In app' : 'Added');
          button.disabled = true;
          button.dataset.complete = 'true';
          button.querySelector('use').setAttribute('href', '#i-circle-check');
          button.setAttribute('aria-label', (result.duplicate ? 'Already in NetMerge: ' : 'Added to NetMerge: ') + name);
          feedback(notice, result.duplicate ? 'Already in NetMerge.' : 'Added to NetMerge.', 'success');
        } catch (error) {
          buttonState(button, 'Retry');
          button.setAttribute('aria-label', 'Retry downloading ' + name);
          feedback(notice, 'Could not send. Check NetMerge is running, then retry.', 'error');
        } finally { restoreFocus(); }
      };
      row.append(mediaIcon, description, button, notice);
      box.appendChild(row);
    }
  } catch (error) {
    $('media').hidden = false;
    $('media-count').textContent = '';
    feedback('media-feedback', 'Could not check this page for media. Reopen the popup to try again.', 'error');
  }
}

for (const id of ['retry', 'retry2']) {
  $(id).onclick = async () => {
    buttonState($(id), 'Checking…', true);
    try { await paint(true); }
    finally { buttonState($(id), 'Retry'); }
  };
}

$('pair-form').onsubmit = async (event) => {
  event.preventDefault();
  if ($('pair').disabled) return;
  feedback('perr');
  if (!$('code').value.trim()) {
    feedback('perr', 'Enter the pairing code from NetMerge.', 'error');
    $('code').setAttribute('aria-invalid', 'true');
    $('code').focus();
    return;
  }
  $('code').setAttribute('aria-invalid', 'false');
  $('code').readOnly = true;
  buttonState($('pair'), 'Connecting…', true);
  feedback('pair-progress', 'Connecting to NetMerge…', 'pending');
  try {
    const result = await ask({ type: 'pair', code: $('code').value, name: /Edg\//.test(navigator.userAgent) ? 'Edge' : /Firefox\//.test(navigator.userAgent) ? 'Firefox' : 'Chrome' });
    if (result.ok) {
      $('code').value = '';
      await paint(true);
    } else {
      feedback('perr', typeof result.error === 'string' && result.error ? result.error : 'Could not connect. Check the code or get a new one in NetMerge.', 'error');
      $('code').setAttribute('aria-invalid', 'true');
    }
  } catch (error) {
    feedback('perr', 'The extension could not connect. Check NetMerge is running, then try again.', 'error');
  } finally {
    $('code').readOnly = false;
    buttonState($('pair'), 'Connect');
    feedback('pair-progress');
    if (!$('v-pair').hidden) $('code').focus();
  }
};
$('code').oninput = () => {
  $('code').setAttribute('aria-invalid', 'false');
  feedback('perr');
};

$('on').onchange = async () => {
  const restoreFocus = preserveFocus($('on'));
  const checked = $('on').checked;
  $('on').disabled = true;
  feedback('on-feedback', 'Saving…', 'pending');
  try {
    await api.storage.local.set({ localOn: checked });
    feedback('on-feedback', 'Saved.', 'success');
  } catch (error) {
    $('on').checked = !checked;
    feedback('on-feedback', 'Could not save. Try switching again.', 'error');
  } finally {
    $('on').disabled = false;
    handoffStatus();
    restoreFocus();
  }
};

async function saveRules(id, key, parse) {
  const field = $(id);
  if (field.readOnly) return;
  const retry = $(id + '-retry');
  const hadRetryFocus = document.activeElement === retry;
  field.readOnly = true;
  retry.disabled = true;
  feedback(id + '-feedback', 'Saving…', 'pending');
  try {
    await api.storage.local.set({ [key]: parse(field.value) });
    field.setAttribute('aria-invalid', 'false');
    feedback(id + '-feedback', 'Saved.', 'success');
    if (hadRetryFocus) field.focus();
    retry.hidden = true;
  } catch (error) {
    field.setAttribute('aria-invalid', 'true');
    feedback(id + '-feedback', 'Could not save. Your changes are still here; retry below.', 'error');
    retry.hidden = false;
  } finally {
    field.readOnly = false;
    retry.disabled = false;
  }
}

// Preserve the existing normalization, limits, and exact storage keys.
$('ign').onchange = () => saveRules('ign', 'ignore', (text) => text.split(/[\s,;]+/).map((site) => site.replace(/^https?:\/\//, '').replace(/\/.*$/, '').trim().toLowerCase()).filter(Boolean).slice(0, 200));
$('typ').onchange = () => saveRules('typ', 'types', NMCore.parseTypes);
$('ign-retry').onclick = () => $('ign').onchange();
$('typ-retry').onclick = () => $('typ').onchange();

$('grant').onclick = async () => {
  const restoreFocus = preserveFocus($('grant'));
  buttonState($('grant'), 'Requesting…', true);
  feedback('perm-feedback', 'Waiting for browser permission…', 'pending');
  try {
    // Invoke directly in the click handler to retain the browser's user gesture.
    const granted = await api.permissions.request({ origins: ['<all_urls>'] });
    if (granted) {
      $('perm').hidden = true;
      $('connected-title').focus();
      await paintMedia();
    } else feedback('perm-feedback', 'Permission was not granted. You can allow access again when ready.', 'warning');
  } catch (error) {
    feedback('perm-feedback', 'Could not request site access. Try again.', 'error');
  } finally {
    buttonState($('grant'), 'Allow for all sites');
    restoreFocus();
  }
};

$('unpair').onclick = async () => {
  const restoreFocus = preserveFocus($('unpair'));
  buttonState($('unpair'), 'Disconnecting…', true);
  feedback('disconnect-feedback', 'Disconnecting this browser…', 'pending');
  try {
    const result = await ask({ type: 'unpair' });
    if (!result.ok) throw new Error('Disconnect failed');
    feedback('disconnect-feedback');
    await paint(true);
  } catch (error) {
    feedback('disconnect-feedback', 'Could not disconnect. Try again.', 'error');
  } finally {
    buttonState($('unpair'), 'Disconnect');
    restoreFocus();
  }
};

paint();
