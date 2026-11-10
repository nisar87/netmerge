import { test as base, expect } from '@playwright/test';
import { NOW } from './data.mjs';

export { expect };
export const test = base.extend({
  fixture: async ({ request }, use) => {
    const control = async (path, body) => {
      const response = body === undefined
        ? await request.get(`/__test/${path}`, { headers: { 'X-NetMerge-Test': '1' } })
        : await request.post(`/__test/${path}`, { headers: { 'X-NetMerge-Test': '1' }, data: body });
      expect(response.ok(), await response.text()).toBeTruthy();
      return response.json();
    };
    const api = {
      reset: (scenario = 'default') => control('reset', { scenario }),
      scenario: scenario => control('scenario', { scenario }),
      patch: changes => control('state', changes),
      fault: options => control('fault', options),
      requests: async () => (await control('requests')).requests,
      clear: () => control('requests', {}),
      read: async path => { const r = await request.get(`/api/${path}`); expect(r.ok()).toBeTruthy(); return r.json(); },
      post: async (path, data) => { const r = await request.post(`/api/${path}`, { headers: { 'X-NetMerge': '1' }, data }); expect(r.ok(), await r.text()).toBeTruthy(); return r.json(); },
    };
    await api.reset(); await use(api);
  },
  _determinism: [async ({ context, page, baseURL, fixture }, use, testInfo) => {
    const errors = [], external = [];
    page.on('pageerror', error => errors.push(error.stack || error.message));
    // The real page runs unchanged. Only external network is blocked, never API calls.
    await context.route('**/*', route => {
      const url = new URL(route.request().url());
      if (url.origin === new URL(baseURL).origin || ['data:', 'blob:'].includes(url.protocol)) return route.continue();
      external.push(url.href); return route.abort('blockedbyclient');
    });
    await page.clock.setFixedTime(new Date(NOW * 1000));
    await use();
    if (testInfo.status !== testInfo.expectedStatus || errors.length || external.length) {
      await testInfo.attach('fixture-requests', { body: JSON.stringify(await fixture.requests(), null, 2), contentType: 'application/json' });
      await testInfo.attach('runtime-errors', { body: JSON.stringify({ errors, external }, null, 2), contentType: 'application/json' });
    }
    expect(errors, 'Uncaught browser errors').toEqual([]);
    expect(external, 'The local UI must not fetch external assets').toEqual([]);
  }, { auto: true }],
});

export async function navigate(page, route) {
  if (page.url() === 'about:blank') await page.goto(`/#/${route}`);
  else await page.evaluate(hash => { location.hash = hash; }, `#/${route}`);
  await expect(page).toHaveURL(new RegExp(`#/${route.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`));
  await expect(page.locator('#pageTitle')).toBeVisible();
}
export async function waitForRead(page, path, action = async () => {}) {
  const response = page.waitForResponse(r => new URL(r.url()).pathname === `/api/${path}` && r.request().method() === 'GET' && r.ok());
  await action(); await response;
}
export async function mutate(page, path, expected, action) {
  const pending = page.waitForRequest(r => new URL(r.url()).pathname === `/api/${path}` && r.method() === 'POST');
  await action(); const request = await pending;
  expect(request.postDataJSON()).toEqual(expected);
  expect(request.headers()['x-netmerge']).toBe('1');
  expect(request.headers()['content-type']).toContain('application/json');
  const response = await request.response();
  expect(response?.ok(), response && await response.text()).toBeTruthy();
  return response.json();
}
