import { test, expect } from '@playwright/test';

// The parent fixture server serves the real popup and core.js at this path.
// Only browser APIs are mocked; all DOM, parsing, and event handling is real.
async function openPopup(page, overrides = {}) {
  await page.addInitScript((overrides) => {
    const state = window.popupMock = {
      ping: { ok: true, status: 200 },
      refresh: { ok: true, data: { enabled: true, minMB: 5, active: 0, speed: 0 } },
      stored: { token: 'test-token', localOn: true, ignore: [], types: [] },
      pair: { ok: true }, unpair: { ok: true }, sendMedia: { ok: true },
      media: [], granted: true, grantResult: true, messages: [], writes: [], requests: [],
      pending: {}, hold: [], ...overrides,
    };
    window.chrome = {
      runtime: {
        sendMessage(message, callback) {
          state.messages.push(message);
          const respond = () => {
            if (state.runtimeError) {
              chrome.runtime.lastError = { message: 'Extension service unavailable' };
              callback();
              delete chrome.runtime.lastError;
              return;
            }
            if (message.type === 'pair' && state.pair.ok) state.stored.token = 'new-token';
            if (message.type === 'unpair' && state.unpair.ok) delete state.stored.token;
            callback(message.type === 'media' ? { list: state.media } : state[message.type]);
          };
          if (state.hold.includes(message.type)) state.pending[message.type] = respond;
          else queueMicrotask(respond);
        },
      },
      storage: { local: {
        async get() {
          if (state.readError) throw new Error('Storage unavailable');
          return { ...state.stored };
        },
        async set(values) {
          state.writes.push(values);
          if (state.hold.includes('write')) await new Promise((resolve) => { state.pending.write = resolve; });
          if (state.writeError) throw new Error('Storage unavailable');
          Object.assign(state.stored, values);
        },
      } },
      permissions: {
        async contains() { return state.granted; },
        async request(request) {
          state.requests.push(request);
          if (state.hold.includes('grant')) await new Promise((resolve) => { state.pending.grant = resolve; });
          if (state.grantError) throw new Error('Permission service unavailable');
          state.granted = state.grantResult;
          return state.grantResult;
        },
      },
      tabs: { async query() { return [{ id: 23, url: 'https://example.com/watch' }]; } },
    };
  }, overrides);
  await page.goto('/extension/src/popup.html');
}

async function update(page, values) {
  await page.evaluate((values) => Object.assign(window.popupMock, values), values);
}

async function release(page, type) {
  await page.evaluate((type) => window.popupMock.pending[type](), type);
}

test('unreachable app can be retried with keyboard and returns focus to pairing', async ({ page }) => {
  await openPopup(page, { ping: { ok: false, status: 0 }, stored: {} });
  await expect(page.locator('#v-down')).toBeVisible();
  await update(page, { ping: { ok: true, status: 200 }, hold: ['ping'] });
  await page.getByRole('button', { name: 'Retry', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('#retry')).toBeDisabled();
  await expect(page.locator('#retry')).toContainText('Checking');
  await release(page, 'ping');
  await expect(page.getByLabel('Pairing code')).toBeFocused();
  await expect(page.locator('#v-pair')).toContainText('Downloads > Browser > Connect a browser');
});

test('manager off has a distinct state and retry', async ({ page }) => {
  await openPopup(page, { ping: { ok: false, status: 404 } });
  await expect(page.locator('#v-off')).toBeVisible();
  await expect(page.locator('#v-down')).toBeHidden();
  await update(page, { ping: { ok: true, status: 200 } });
  await page.locator('#retry2').click();
  await expect(page.locator('#v-on')).toBeVisible();
});

test('pairing validates, prevents duplicate submissions, preserves errors as text, and connects', async ({ page }) => {
  const error = 'Code expired. <img src=x onerror=alert(1)> Get a new code.';
  await openPopup(page, { stored: {}, pair: { ok: false, error }, hold: ['pair'] });
  const code = page.getByLabel('Pairing code');
  await page.getByRole('button', { name: 'Connect', exact: true }).click();
  await expect(code).toBeFocused();
  await expect(code).toHaveAttribute('aria-invalid', 'true');
  expect(await page.evaluate(() => popupMock.messages.filter((m) => m.type === 'pair'))).toHaveLength(0);
  await code.fill('ABCDE-12345');
  await code.press('Enter');
  await expect(page.locator('#pair')).toBeDisabled();
  await expect(page.locator('#pair')).toContainText('Connecting');
  await code.press('Enter');
  expect(await page.evaluate(() => popupMock.messages.filter((m) => m.type === 'pair'))).toEqual([
    { type: 'pair', code: 'ABCDE-12345', name: 'Chrome' },
  ]);
  await release(page, 'pair');
  await expect(page.locator('#perr')).toHaveText(error);
  await expect(page.locator('#perr img')).toHaveCount(0);
  await expect(code).toHaveValue('ABCDE-12345');
  await expect(page.locator('#pair')).toBeEnabled();
  await update(page, { pair: { ok: true }, hold: [] });
  await code.press('Enter');
  await expect(page.locator('#v-on')).toBeVisible();
  await expect(page.locator('#connected-title')).toBeFocused();
});

test('connected active status, local handoff, and advanced preferences retain storage contracts', async ({ page }) => {
  await openPopup(page, { refresh: { ok: true, data: { enabled: true, minMB: 12, active: 2, speed: 3145728 } } });
  await expect(page.locator('#stat')).toContainText('2 downloading');
  await expect(page.locator('#stat')).toContainText('3.0 MB/s');
  await expect(page.locator('#min')).toContainText('12 MB');
  await expect(page.locator('#ign')).toBeHidden();
  await page.getByRole('switch', { name: 'Send big downloads to NetMerge' }).uncheck();
  await expect(page.locator('#handoff-status')).toContainText('stay in this browser');
  await page.locator('summary').click();
  await page.getByLabel('Keep these sites in the browser').fill('https://EXAMPLE.com/path; sub.example.org\nexample.net');
  await page.getByLabel('Always send these file types').fill('.PDF; epub, bad! mp4');
  await page.getByLabel('Always send these file types').press('Tab');
  await expect(page.locator('#typ-feedback')).toContainText('Saved');
  expect(await page.evaluate(() => popupMock.writes)).toEqual([
    { localOn: false },
    { ignore: ['example.com', 'sub.example.org', 'example.net'] },
    { types: ['pdf', 'epub', 'mp4'] },
  ]);
});

test('globally disabled handoff explains why the switch is unavailable', async ({ page }) => {
  await openPopup(page, { refresh: { ok: true, data: { enabled: false, minMB: 5, active: 0, speed: 0 } } });
  await expect(page.getByRole('switch')).toBeDisabled();
  await expect(page.locator('#min')).toContainText('off in NetMerge');
  await expect(page.locator('#handoff-status')).toContainText('stay in this browser');
  expect(await page.evaluate(() => popupMock.writes)).toEqual([]);
});

test('failed preference saves keep entered text and restore the confirmed switch value', async ({ page }) => {
  await openPopup(page, { writeError: true, hold: ['write'] });
  await page.getByRole('switch').focus();
  await page.keyboard.press('Space');
  await expect(page.getByRole('switch')).toBeDisabled();
  await release(page, 'write');
  await expect(page.getByRole('switch')).toBeChecked();
  await expect(page.locator('#on-feedback')).toContainText('Could not save');
  await expect(page.getByRole('switch')).toBeFocused();
  await update(page, { hold: [] });
  await page.locator('summary').click();
  await page.locator('#ign').fill('example.com');
  await page.locator('#ign').press('Tab');
  await expect(page.locator('#ign-feedback')).toContainText('Could not save');
  await expect(page.locator('#ign')).toHaveValue('example.com');
  await update(page, { writeError: false });
  await page.getByRole('button', { name: 'Retry saving sites' }).click();
  await expect(page.locator('#ign-feedback')).toContainText('Saved');
  expect(await page.evaluate(() => popupMock.stored.ignore)).toEqual(['example.com']);
});

test('permissions denial stays actionable and retries the exact existing permission', async ({ page }) => {
  await openPopup(page, { granted: false, grantResult: false, hold: ['grant'] });
  await expect(page.locator('#perm')).toBeVisible();
  expect(await page.evaluate(() => popupMock.requests)).toEqual([]);
  await page.getByRole('button', { name: 'Allow for all sites' }).click();
  await expect(page.locator('#grant')).toBeDisabled();
  await release(page, 'grant');
  await expect(page.locator('#perm-feedback')).toContainText('not granted');
  await expect(page.locator('#grant')).toBeEnabled();
  await expect(page.locator('#grant')).toBeFocused();
  await update(page, { grantResult: true, hold: [] });
  await page.locator('#grant').click();
  await expect(page.locator('#perm')).toBeHidden();
  expect(await page.evaluate(() => popupMock.requests)).toEqual([
    { origins: ['<all_urls>'] }, { origins: ['<all_urls>'] },
  ]);
});

test('media names are safe text, malformed escapes render, and failed handoff can retry', async ({ page }) => {
  const url = 'https://example.com/%E0%A4%A.mp4?token=test';
  await openPopup(page, {
    media: [{ url, kind: 'video', size: 2097152 }, { url: 'https://example.com/%3Cimg%3E.mp3', kind: 'audio', size: 0 }],
    sendMedia: { ok: false }, hold: ['sendMedia'],
  });
  await expect(page.locator('#mlist li')).toHaveCount(2);
  await expect(page.locator('#mlist')).toContainText('%E0%A4%A.mp4');
  await expect(page.locator('#mlist')).toContainText('<img>.mp3');
  await expect(page.locator('#mlist img')).toHaveCount(0);
  const first = page.locator('#mlist li').first();
  await first.getByRole('button').click();
  await expect(first.getByRole('button')).toBeDisabled();
  await release(page, 'sendMedia');
  await expect(first).toContainText('Could not send');
  await expect(first.getByRole('button')).toBeEnabled();
  await expect(first.getByRole('button')).toBeFocused();
  await update(page, { sendMedia: { ok: true, duplicate: true }, hold: [] });
  await first.getByRole('button').click();
  await expect(first).toContainText('Already in NetMerge');
  await expect(first.getByRole('button')).toBeDisabled();
  expect(await page.evaluate(() => popupMock.messages.filter((m) => m.type === 'sendMedia'))).toEqual([
    { type: 'sendMedia', url, referrer: 'https://example.com/watch' },
    { type: 'sendMedia', url, referrer: 'https://example.com/watch' },
  ]);
});

test('media preserves the eight item limit and confirms a successful handoff', async ({ page }) => {
  await openPopup(page, { media: Array.from({ length: 10 }, (_, i) => ({ url: `https://example.com/stream${i}.m3u8`, kind: 'stream', size: -1 })) });
  await expect(page.locator('#mlist li')).toHaveCount(8);
  await expect(page.locator('#media-count')).toHaveText('8 of 10');
  const first = page.locator('#mlist li').first();
  await first.getByRole('button').click();
  await expect(first).toContainText('Added to NetMerge');
});

test('disconnect reports a failure and only changes views after a successful reply', async ({ page }) => {
  await openPopup(page, { unpair: { ok: false }, hold: ['unpair'] });
  await page.getByRole('button', { name: 'Disconnect', exact: true }).click();
  await expect(page.locator('#unpair')).toBeDisabled();
  await release(page, 'unpair');
  await expect(page.locator('#disconnect-feedback')).toContainText('Could not disconnect');
  await expect(page.locator('#v-on')).toBeVisible();
  await expect(page.locator('#unpair')).toBeFocused();
  await update(page, { unpair: { ok: true }, hold: [] });
  await page.locator('#unpair').click();
  await expect(page.locator('#v-pair')).toBeVisible();
  await expect(page.getByLabel('Pairing code')).toBeFocused();
});

for (const action of ['sendMedia', 'unpair']) {
  test(`${action} failure respects focus moved to another control while pending`, async ({ page }) => {
    await openPopup(page, {
      media: [{ url: 'https://example.com/video.mp4', kind: 'video', size: 2097152 }],
      [action]: { ok: false }, hold: [action],
    });
    const button = action === 'sendMedia' ? page.locator('#mlist button').first() : page.locator('#unpair');
    await button.focus();
    await page.keyboard.press('Enter');
    await expect(button).toBeDisabled();
    await page.locator('summary').focus();
    await release(page, action);
    await expect(button).toBeEnabled();
    await expect(page.locator('summary')).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page.locator('#ign')).toBeVisible();
  });
}

test('runtime and storage failures expose a recovery action', async ({ page }) => {
  await openPopup(page, { runtimeError: true });
  await expect(page.locator('#v-down')).toBeVisible();
  await expect(page.locator('#down-detail')).toContainText('extension');
  await update(page, { runtimeError: false, readError: true });
  await page.locator('#retry').click();
  await expect(page.locator('#down-detail')).toContainText('preferences');
  await update(page, { readError: false });
  await page.locator('#retry').click();
  await expect(page.locator('#v-on')).toBeVisible();
});

for (const theme of ['light', 'dark']) {
  test(`${theme} theme fits compact windows and exposes visible keyboard focus`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: theme, reducedMotion: 'reduce' });
    await page.setViewportSize({ width: 320, height: 480 });
    await openPopup(page, { granted: false, media: [{ url: `https://example.com/${'long-name-'.repeat(20)}.mp4`, kind: 'video', size: 2048000 }] });
    await expect(page.locator('#v-on')).toBeVisible();
    const toggle = page.getByRole('switch');
    await toggle.focus();
    expect(await toggle.evaluate((el) => getComputedStyle(el).outlineStyle)).not.toBe('none');
    await page.locator('summary').click();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    const color = await page.locator('body').evaluate((el) => getComputedStyle(el).backgroundColor);
    expect(color).toBe(theme === 'dark' ? 'rgb(11, 17, 32)' : 'rgb(245, 247, 251)');
    await page.getByRole('button', { name: 'Disconnect', exact: true }).focus();
    const bounds = await page.locator('#unpair').boundingBox();
    expect(bounds.y).toBeGreaterThanOrEqual(0);
    expect(bounds.y + bounds.height).toBeLessThanOrEqual(480);
  });
}
