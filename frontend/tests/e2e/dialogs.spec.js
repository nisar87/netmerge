import AxeBuilder from '@axe-core/playwright';
import {test,expect,navigate} from '../fixtures/browser.mjs';

const cases=[
  ['Setup guide','settings/maintenance',async page=>page.locator('#wizBtn').click()],
  ['Quit confirmation','settings/maintenance',async page=>page.locator('#quit').click()]
];
for(const theme of ['light','dark'])for(const [name,route,open] of cases){
  test(`${name}: keyboard modal and accessibility in ${theme}`,async({page,fixture})=>{
    await page.setViewportSize({width:390,height:844});await page.emulateMedia({colorScheme:theme});
    await navigate(page,route);await open(page);
    const dialog=page.getByRole('dialog');await expect(dialog).toBeVisible();
    await expect.poll(()=>page.evaluate(()=>document.querySelector('dialog[open]')?.contains(document.activeElement))).toBe(true);
    await page.keyboard.press('Tab');
    expect(await page.evaluate(()=>document.querySelector('dialog[open]')?.contains(document.activeElement))).toBe(true);
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
    const result=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
    expect(result.violations).toEqual([]);
  });
}
