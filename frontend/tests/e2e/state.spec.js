import {test,expect,navigate,mutate,waitForRead} from '../fixtures/browser.mjs';

test('an old state poll cannot replace a confirmed settings write',async({page,fixture})=>{
  await navigate(page,'settings/general');await expect(page.locator('#autoTog')).toHaveAttribute('aria-checked','false');
  const before=(await fixture.read('state')).settings;await fixture.clear();
  await fixture.fault({path:'/api/state',method:'GET',delayMs:1500,times:1});
  const oldRequest=await page.waitForRequest(r=>new URL(r.url()).pathname==='/api/state');
  await expect.poll(async()=>(await fixture.requests()).filter(r=>r.path==='/api/state').length).toBe(1);
  await mutate(page,'settings',{...before,autoStart:true},()=>page.locator('#autoTog').click());
  await (await oldRequest.response()).finished();
  // The request body is complete; allow its DOM continuation to run before inspecting it.
  await page.waitForTimeout(30);
  expect(await page.evaluate(()=>local.autoStart)).toBe(true);
  await expect(page.locator('#autoTog')).toHaveAttribute('aria-checked','true');
});

test('failed settings save retains the draft and retries only on explicit action',async({page,fixture})=>{
  await navigate(page,'settings/general');await expect(page.locator('#autoTog')).toHaveAttribute('aria-checked','false');
  await fixture.fault({path:'/api/settings',method:'POST',status:500,error:'Synthetic settings failure',times:1});
  await fixture.clear();await page.locator('#autoTog').click();
  await expect(page.locator('#saveStatus')).toContainText('could not be saved');
  await waitForRead(page,'state');
  await expect(page.locator('#autoTog')).toHaveAttribute('aria-checked','true');
  expect((await fixture.requests()).filter(r=>r.path==='/api/settings'&&r.method==='POST')).toHaveLength(1);
  await page.locator('#retrySettings').click();await expect(page.locator('#saveStatus')).toBeHidden();
  expect((await fixture.read('state')).settings.autoStart).toBe(true);
});

test('a rapid reversal while a save is pending preserves the final choice',async({page,fixture})=>{
  await navigate(page,'settings/general');await expect(page.locator('#autoTog')).toHaveAttribute('aria-checked','false');
  await fixture.fault({path:'/api/settings',method:'POST',delayMs:1200,times:1});
  const first=page.waitForRequest(r=>new URL(r.url()).pathname==='/api/settings'&&r.method()==='POST');
  await page.locator('#autoTog').click();await first;await page.locator('#autoTog').click();
  await expect.poll(async()=>(await fixture.read('state')).settings.autoStart,{timeout:6000}).toBe(false);
  await expect(page.locator('#autoTog')).toHaveAttribute('aria-checked','false');
});

test('adapter discovery preserves a rename being typed and its focus',async({page,fixture})=>{
  await navigate(page,'connections');const input=page.getByRole('textbox',{name:'Name for Ethernet',exact:true});
  await input.fill('Unsaved connection name');
  const current=await fixture.read('state');
  await fixture.patch({state:{snap:{links:[...current.snap.links,{...current.snap.links[0],name:'USB',label:'USB'}]},settings:{links:[...current.settings.links,{...current.settings.links[0],name:'USB',label:'USB'}]}}});
  await waitForRead(page,'state');await expect(input).toHaveValue('Unsaved connection name');await expect(input).toBeFocused();
});

test('first-run setup remains open and retryable through a failed pending save',async({page,fixture})=>{
  await fixture.patch({state:{settings:{setupDone:false}}});await navigate(page,'overview');
  await expect(page.locator('#wiz')).toBeVisible();
  await fixture.fault({path:'/api/settings',method:'POST',status:500,error:'Setup save failed',delayMs:1400,times:1});
  await page.locator('#wizSkip').click();
  await expect(page.locator('#wiz')).toBeVisible();await expect(page.locator('#wizError')).toHaveText('Setup save failed');
  await page.locator('#wizSkip').click();await expect(page.locator('#wiz')).toHaveCount(0);
  expect((await fixture.read('state')).settings.setupDone).toBe(true);
});

test('restore waits for earlier settings work and a cancelled file can be selected again',async({page,fixture})=>{
  await navigate(page,'settings/general');await expect(page.locator('#autoTog')).toHaveAttribute('aria-checked','false');
  const backup={...(await fixture.read('state')).settings,mode:'stable',noCallProtect:true};
  const file={name:'settings.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(backup))};
  await fixture.clear();await fixture.fault({path:'/api/state',method:'GET',delayMs:1800,times:1});
  const pendingRead=page.waitForRequest(r=>new URL(r.url()).pathname==='/api/state');
  await page.locator('#autoTog').click();await pendingRead;
  await navigate(page,'settings/maintenance');
  await page.locator('#restFile').setInputFiles(file);
  await page.locator('#confirmDialog').getByRole('button',{name:'Cancel',exact:true}).click();
  await expect(page.locator('#restFile')).toHaveValue('');
  await page.locator('#restFile').setInputFiles(file);
  await page.locator('#confirmDialog').getByRole('button',{name:'Restore settings',exact:true}).click();
  await expect(page.getByRole('dialog',{name:'Restoring settings…'})).toHaveCount(0);
  await expect.poll(async()=>(await fixture.read('state')).settings.mode).toBe('stable');
  await waitForRead(page,'state');
  expect((await fixture.read('state')).settings.noCallProtect).toBe(true);
  const writes=(await fixture.requests()).filter(r=>r.method==='POST'&&['/api/settings','/api/restore'].includes(r.path));
  expect(writes.map(r=>r.path)).toEqual(['/api/settings','/api/restore']);
  expect(await page.evaluate(()=>local.mode)).toBe('stable');
});
