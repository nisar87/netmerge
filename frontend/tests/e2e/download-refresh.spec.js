import {test,expect,navigate,enableDownloads} from '../fixtures/browser.mjs';
import {downloadItem,NOW} from '../fixtures/data.mjs';

for(const sort of ['added','default']){
  test(`a newly observed transfer appears on page one with ${sort} sorting`,async({page,fixture})=>{
    await enableDownloads(fixture);
    const items=Array.from({length:100},(_,i)=>downloadItem(i+1,'paused'));
    await fixture.patch({downloads:{items}});
    await navigate(page,'downloads/transfers');
    await expect(page.locator('.dlrow')).toHaveCount(100);
    await page.locator('#dlSort').selectOption(sort);
    await page.evaluate(()=>NM.dispose());
    await page.getByRole('checkbox',{name:'Select fixture-file-0001.zip',exact:true}).check();
    await fixture.patch({downloads:{items:[...items,downloadItem(101,'queued',{added:NOW})]}});
    await page.evaluate(()=>loadDL(true));
    await expect(page.locator('.dlrow').first()).toHaveAttribute('data-key','fixture-0101');
    await expect(page.locator('.dlrow')).toHaveCount(100);
    await expect(page.getByRole('checkbox',{name:'Select fixture-file-0001.zip',exact:true})).toBeChecked();
    await page.evaluate(()=>loadDL(true));
    await expect(page.locator('.dlrow').first()).toHaveAttribute('data-key','fixture-0101');
    await page.getByRole('button',{name:'Next page',exact:true}).click();
    await expect(page.locator('.dlrow')).toHaveCount(1);
    await expect(page.locator('.dlrow')).toHaveAttribute('data-key','fixture-0100');
  });
}

test('Retry refresh recovers a failed first Downloads load without automatic polling',async({page,fixture})=>{
  await enableDownloads(fixture);
  await fixture.fault({path:'/api/downloads',method:'GET',status:503,error:'Temporary download refresh failure'});
  await navigate(page,'downloads/transfers');
  const retry=page.getByRole('button',{name:'Retry refresh',exact:true});
  await expect(retry).toBeVisible();
  await page.evaluate(async()=>{NM.dispose();if(dlRead)await dlRead;});
  expect(await page.evaluate(()=>DL)).toBeNull();
  await fixture.fault({path:'/api/downloads',method:'GET',times:0});
  await fixture.clear();
  await retry.click();
  await expect.poll(async()=>(await fixture.requests()).filter(r=>r.path==='/api/downloads'&&r.method==='GET').length,{timeout:2000}).toBe(1);
  await expect(page.locator('.dlrow')).toHaveCount(5);
  await expect(retry).toHaveCount(0);
});
