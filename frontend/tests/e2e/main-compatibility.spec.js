import AxeBuilder from '@axe-core/playwright';
import {test,expect,navigate,enableDownloads,mutate} from '../fixtures/browser.mjs';

for(const state of ['paused','error','downloading']){
  test(`adding a ${state} duplicate preserves main's restart and filename contract`,async({page,fixture})=>{
    await enableDownloads(fixture);
    const snapshot=await fixture.read('downloads');
    const item=snapshot.items[1];item.state=state;
    await fixture.patch({downloads:{items:snapshot.items}});
    await navigate(page,'downloads/transfers');
    await page.getByRole('button',{name:'Add download',exact:true}).click();
    await page.locator('#dlUrl').fill(item.url);
    await page.getByRole('button',{name:'Continue',exact:true}).click();
    const stopped=state!=='downloading';
    await expect(page.getByRole('dialog')).toContainText(stopped?'restarts that copy':'makes a second copy');
    await mutate(page,'downloads',{action:'add',url:item.url,start:true,conns:0,limit:0,queue:'',...(stopped?{}:{force:true})},
      ()=>page.getByRole('button',{name:'Download now',exact:true}).click());
    await expect(page.locator('#toast')).toHaveText(stopped?'1 stopped download restarted':'1 downloads added');
    await expect(page.locator('#toast')).not.toHaveClass(/bad/);
    const current=await fixture.read('downloads');
    expect(current.items).toHaveLength(stopped?5:6);
    if(stopped){
      const restarted=current.items.find(i=>i.id===item.id);
      expect(restarted.state).toBe('queued');
      expect(restarted.name).toBe(item.name);expect(restarted.path).toBe(item.path);
    }
  });
}

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
