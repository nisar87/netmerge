import AxeBuilder from '@axe-core/playwright';
import {test,expect,navigate,enableDownloads,mutate} from '../fixtures/browser.mjs';

async function selectTransfer(page,fixture){
  await enableDownloads(fixture);await navigate(page,'downloads/transfers');
  await page.getByRole('checkbox',{name:'Select fixture-file-0001.zip',exact:true}).check();
}

test('compact bulk actions explain themselves on focus and Escape dismisses the hint before selection',async({page,fixture})=>{
  await selectTransfer(page,fixture);
  const pause=page.getByRole('button',{name:'Pause selected downloads',exact:true});
  await pause.focus();
  await expect(page.getByRole('tooltip')).toHaveText('Pause selected');
  await page.keyboard.press('Escape');
  await expect(page.getByRole('tooltip')).toBeHidden();
  await expect(pause).toBeFocused();
  await expect(page.locator('.dl-bulk')).toContainText('1 selected');
  await mutate(page,'downloads',{action:'pause',id:'fixture-0001'},()=>page.keyboard.press('Enter'));
  await expect(page.locator('.dl-bulk')).toBeHidden();
});

test('bulk icons preserve page selection, queue movement and removal confirmation',async({page,fixture})=>{
  await selectTransfer(page,fixture);
  await page.getByRole('button',{name:'Select this page',exact:true}).click();
  await expect(page.locator('.dl-bulk')).toContainText('5 selected');
  await expect(page.locator('#dlBulkQ')).toHaveValue('__choose__');
  await page.getByRole('button',{name:'Clear selection',exact:true}).click();
  await expect(page.locator('.dl-bulk')).toBeHidden();
  await page.getByRole('checkbox',{name:'Select fixture-file-0002.zip',exact:true}).check();
  await mutate(page,'downloads',{action:'setQueue',id:'fixture-0002',queue:''},()=>page.locator('#dlBulkQ').selectOption(''));
  await expect(page.locator('#dlBulkQ')).toHaveValue('__choose__');
  await page.getByRole('button',{name:'Remove selected downloads',exact:true}).click();
  await expect(page.getByRole('dialog')).toContainText('Remove 1 download');
  await page.keyboard.press('Escape');
  await expect(page.getByRole('checkbox',{name:'Select fixture-file-0002.zip',exact:true})).toBeChecked();
  expect((await fixture.requests()).filter(r=>r.body?.action==='remove')).toHaveLength(0);
});

test('icon tooltip remains available on hover and a live state change updates its action',async({page,fixture})=>{
  await enableDownloads(fixture);await navigate(page,'downloads/transfers');
  const action=page.locator('[data-key="fixture-0002"] [data-dl]');
  await action.hover();
  await expect(page.getByRole('tooltip')).toHaveText('Resume download');
  await page.getByRole('tooltip').hover();
  await expect(page.getByRole('tooltip')).toBeVisible();
  await page.mouse.move(0,0);await expect(page.getByRole('tooltip')).toBeHidden();
  await action.focus();
  await fixture.post('downloads',{action:'resume',id:'fixture-0002'});
  await expect(action).toHaveAccessibleName('Pause fixture-file-0002.zip');
  await expect(page.getByRole('tooltip')).toHaveText('Pause download');
  await expect(action).toBeFocused();
});

test('every rendered action icon resolves to a local symbol, including numbered icon names',async({page,fixture})=>{
  await selectTransfer(page,fixture);
  for(const route of ['downloads/transfers','downloads/queues','downloads/browser']){
    await navigate(page,route);
    const missing=await page.locator('#dlBody').evaluate(async root=>{
      const sprite=new DOMParser().parseFromString(await (await fetch('/assets/icons.svg')).text(),'image/svg+xml');
      return [...root.querySelectorAll('use')].map(use=>use.getAttribute('href').split('#')[1]).filter(id=>!sprite.getElementById(id));
    });
    expect(missing).toEqual([]);
  }
});

for(const theme of ['light','dark'])for(const width of [375,768,1440]){
  test(`selected download controls are accessible without overflow in ${theme} at ${width}px`,async({page,fixture})=>{
    await page.setViewportSize({width,height:900});await page.emulateMedia({colorScheme:theme});
    await selectTransfer(page,fixture);
    await page.getByRole('button',{name:'Select this page',exact:true}).click();
    await page.getByRole('button',{name:'Remove selected downloads',exact:true}).focus();
    await expect(page.getByRole('tooltip')).toHaveText('Remove selected');
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
    expect((await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze()).violations).toEqual([]);
  });
}
