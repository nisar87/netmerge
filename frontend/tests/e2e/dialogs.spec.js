import AxeBuilder from '@axe-core/playwright';
import {test,expect,navigate,enableDownloads} from '../fixtures/browser.mjs';

const cases=[
  ['Add download','downloads/transfers',async page=>page.locator('#dlNew').click()],
  ['File details','downloads/transfers',async page=>page.locator('.dl-name[data-id="fixture-0002"]').click()],
  ['Download options','downloads/transfers',async page=>{await page.locator('#dlNew').click();await page.locator('[data-dm="options"]').click();}],
  ['Download probe','downloads/transfers',async page=>{await page.locator('#dlNew').click();await page.locator('#dlUrl').fill('https://downloads.example.test/a.zip');await page.locator('#dlAddBtn').click();await expect(page.locator('#dlName')).toBeVisible();}],
  ['Manage queues','downloads/queues',async page=>page.getByRole('button',{name:'Manage queues',exact:true}).click()],
  ['Choose folders','downloads/settings',async page=>page.locator('[data-dm="folders"]').click()],
  ['Find links','downloads/transfers',async page=>{await page.locator('.dl-menu summary').click();await page.locator('[data-dm="grab"]').click();}],
  ['Connect browser','downloads/browser',async page=>page.locator('#dlExtCode').click()],
  ['Setup guide','settings/maintenance',async page=>page.locator('#wizBtn').click()],
  ['Quit confirmation','settings/maintenance',async page=>page.locator('#quit').click()]
];
for(const theme of ['light','dark'])for(const [name,route,open] of cases){
  test(`${name}: keyboard modal and accessibility in ${theme}`,async({page,fixture})=>{
    await page.setViewportSize({width:390,height:844});await page.emulateMedia({colorScheme:theme});
    await enableDownloads(fixture);await navigate(page,route);await open(page);
    const dialog=page.getByRole('dialog');await expect(dialog).toBeVisible();
    await expect.poll(()=>page.evaluate(()=>document.querySelector('dialog[open]')?.contains(document.activeElement))).toBe(true);
    await page.keyboard.press('Tab');
    expect(await page.evaluate(()=>document.querySelector('dialog[open]')?.contains(document.activeElement))).toBe(true);
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
    const result=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
    expect(result.violations).toEqual([]);
  });
}
