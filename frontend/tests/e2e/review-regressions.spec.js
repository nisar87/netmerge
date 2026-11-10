import { test, expect, navigate, mutate } from '../fixtures/browser.mjs';

const appRules = [
  { process: 'first.exe', target: 'fast' },
  { process: 'editing.exe', target: 'stable' },
  { process: 'last.exe', target: 'fast' },
];
async function openRules(page, fixture) {
  await fixture.patch({ state: { settings: { appRules } } });
  await navigate(page, 'apps/rules');
  await expect(page.locator('#rulesTbl select')).toHaveCount(3);
  await page.evaluate(() => NM.dispose());
}

test('focused app rule still edits its process after an earlier rule is removed', async ({ page, fixture }) => {
  await openRules(page, fixture);
  const selector = page.locator('#rulesTbl').getByRole('combobox', { name: 'Rule for editing.exe', exact: true });
  await selector.focus();
  await fixture.patch({ state: { settings: { appRules: appRules.slice(1) } } });
  await page.evaluate(() => poll(true));
  await expect(selector).toBeFocused();
  const settings = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...settings, appRules: [
    { process: 'editing.exe', target: 'bypass' }, appRules[2],
  ] }, () => selector.selectOption('bypass'));
});

test('a focused rule removed elsewhere does not edit its replacement or recreate itself', async ({ page, fixture }) => {
  await openRules(page, fixture);
  const selector = page.locator('#rulesTbl').getByRole('combobox', { name: 'Rule for editing.exe', exact: true });
  await selector.focus();
  await fixture.patch({ state: { settings: { appRules: [appRules[0], appRules[2]] } } });
  await page.evaluate(() => poll(true));
  await fixture.clear();
  await selector.selectOption('bypass');
  await expect(page.locator('#rulesTbl select')).toHaveCount(2);
  expect((await fixture.read('state')).settings.appRules).toEqual([appRules[0], appRules[2]]);
  expect((await fixture.requests()).filter(r => r.path === '/api/settings' && r.method === 'POST')).toEqual([]);
});

test('focused remove button keeps its process identity after polling shifts the list', async ({ page, fixture }) => {
  await openRules(page, fixture);
  const button = page.locator('#rulesTbl tr').filter({ hasText: 'editing.exe' }).getByRole('button', { name: 'Remove rule', exact: true });
  await button.focus();
  await fixture.patch({ state: { settings: { appRules: appRules.slice(1) } } });
  await page.evaluate(() => poll(true));
  const settings = (await fixture.read('state')).settings;
  await mutate(page, 'settings', { ...settings, appRules: [appRules[2]] }, () => button.click());
});
