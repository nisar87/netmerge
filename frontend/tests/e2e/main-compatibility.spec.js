import AxeBuilder from '@axe-core/playwright';
import {test,expect,navigate,mutate} from '../fixtures/browser.mjs';

test('usage corrections survive polling and send the stable release API contract',async({page,fixture})=>{
  await navigate(page,'usage');
  await page.locator('#usFix summary').click();
  await expect(page.locator('#fxDay')).toHaveValue('2026-11-09');
  await page.getByRole('spinbutton',{name:'Download usage in MB',exact:true}).fill('123');
  await page.getByRole('spinbutton',{name:'Upload usage in MB',exact:true}).fill('45');
  await page.evaluate(()=>loadUsage());
  await expect(page.locator('#fxDown')).toHaveValue('123');
  await expect(page.locator('#fxUp')).toHaveValue('45');
  expect((await new AxeBuilder({page}).include('#usFix').withTags(['wcag2a','wcag2aa','wcag21aa']).analyze()).violations).toEqual([]);
  await mutate(page,'usage',{action:'set',link:'Ethernet',day:'2026-11-09',downMB:123,upMB:45},()=>page.locator('#fxSave').click());
  await expect(page.locator('#toast')).toHaveText('Number corrected');
  expect((await fixture.read('usage')).days[0].links.Ethernet).toEqual([123e6,45e6]);
});

test('usage correction waits for a connection and day when initial lists are empty',async({page,fixture})=>{
  await fixture.reset('null');
  await navigate(page,'usage');
  await page.evaluate(async()=>{await poll(true);await loadUsage();});
  await page.locator('#usFix summary').click();
  await expect(page.locator('#fxSave')).toBeDisabled();
  await expect(page.locator('#featureError')).toBeHidden();
  await fixture.reset();
  await page.evaluate(async()=>{await poll();await loadUsage();});
  await expect(page.locator('#fxSave')).toBeEnabled();
  await expect(page.locator('#fxDown')).toHaveValue('2000');
});
