import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

// Optional real-browser contract suite; no production or test dependency is installed by this file.
// PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs node --test .../downloads-browser.test.mjs
const modulePath=process.env.PLAYWRIGHT_MODULE;
test('Downloads browser workflows, draft retention, paging, keyboard and request contracts', {skip:!modulePath}, async()=>{
  const {chromium}=await import(modulePath);
  const browser=await chromium.launch({headless:true,args:['--no-sandbox']});
  try {
    const page=await browser.newPage({viewport:{width:1440,height:1000}});
    page.setDefaultTimeout(8000);
    const errors=[];page.on('pageerror',e=>errors.push(e.message));
    await page.setContent('<html><body style="margin:32px"><main><div id="dlBody"></div></main></body></html>');
    await page.addStyleTag({content:readFileSync(new URL('../../../ui/downloads.css',import.meta.url),'utf8')});
    await page.evaluate(()=>{
      window.$=s=>document.querySelector(s);window.$$=s=>[...document.querySelectorAll(s)];
      window.esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
      window.bytes=n=>n==null?'—':(n/1048576).toFixed(1)+' MB';window.colorOf=()=> '#1D4ED8';window.labelOf=n=>n;
      window.page='downloads';window.DM_ON=true;window.S={};window.local={};window.go=()=>{};
      window.toasts=[];window.toast=(...x)=>toasts.push(x);window.setHTML=(e,h)=>{e.innerHTML=h;};
      window.calls=[];window.allow=true;window.NM={confirm:async()=>allow,schedule:()=>()=>{}};
      window.snapshot={enabled:true,items:Array.from({length:1000},(_,i)=>({id:String(i),name:i===0?'Installer.exe':'Transfer '+String(i).padStart(4,'0')+'.zip',url:'https://files.example.test/file'+i,category:'Archives',state:i===0?'done':i===1?'downloading':'paused',order:i,size:104857600,done:52428800,speed:1048576,map:[0,50,100]})),speed:1048576,lines:[{name:'Wi-Fi'}],config:{dir:'C:\\Downloads',queues:[{id:'night',name:'Night',maxActive:2,window:{enabled:true,from:'23:00',to:'06:00'},days:[1],paused:false}],window:{enabled:false,from:'22:00',to:'07:00'},limitLines:{},catDirs:{Videos:'C:\\Videos'},siteRules:[],maxActive:3,maxPerLine:8,noTorrents:false,noUpload:false,noNotify:false,unrelated:'preserve me'},ext:{enabled:true,minMB:5,browsers:[{id:'b',name:'Firefox',created:1700000000}]}};
      window.api=async(path,body)=>{
        calls.push({path,body:body&&structuredClone(body)});
        if(window.failID&&body?.id===window.failID)throw Error('Synthetic per-item failure');
        if(body?.action==='config') snapshot.config=structuredClone(body.config);
        if(body?.action==='extConfig') {snapshot.ext.enabled=!body.extOff;snapshot.ext.minMB=body.extMinMB;}
        if(body?.action==='probe')return {ok:true,info:{name:'video.mp4',size:1000,ranges:true},lines:2,lineNames:['Wi-Fi','Ethernet'],dir:'C:\\Downloads',hls:{segments:10,seconds:120,heights:[1080,720],audio:true},mux:false};
        if(body?.action==='extCode')return {code:'DEMO0-DEMO0',expires:Math.floor(Date.now()/1000)+120,repo:'example/netmerge'};
        if(body?.action==='grab')return {links:[{name:'file.zip',url:'https://example.test/file.zip'}]};
        if(body?.action==='torrentInfo')return {name:'Torrent',files:[{name:'a',size:10},{name:'b',size:20}],dir:'C:\\Downloads',size:30};
        if(body?.action==='files')return {files:[{name:'a',size:10}],select:[true]};
        if(body?.action==='add')return {added:[{id:'new'}],skipped:0};
        if(body?.action==='import')return {imported:2,skipped:1};
        if(body?.action==='export')return {file:{items:[]}};
        return structuredClone(snapshot);
      };
    });
    for(const f of ['downloads-more.js','downloads.js'])await page.addScriptTag({content:readFileSync(new URL('../../../ui/'+f,import.meta.url),'utf8')});
    await page.evaluate(()=>loadDL(true));
    assert.equal(await page.locator('tbody tr').count(),100);
    await page.locator('#dlSearch').fill('Transfer');
    await page.evaluate(()=>{window.savedSearch=document.activeElement; snapshot.speed+=100;});
    await page.evaluate(()=>loadDL(true));
    assert.equal(await page.evaluate(()=>savedSearch===document.activeElement),true);
    assert.equal(await page.locator('#dlSearch').inputValue(),'Transfer');
    await page.locator('#dlSearch').fill('');
    await page.locator('.dlck').first().check();
    await page.locator('[data-page="1"]').click();
    assert.match(await page.locator('.dl-bulk').innerText(),/1 selected/);
    await page.locator('[data-page="-1"]').click();
    assert.equal(await page.locator('.dlck').first().isChecked(),true);

    await page.locator('[data-dl-tab=settings]').first().click();
    assert.equal(await page.evaluate(()=>location.hash),'#/downloads/settings');
    await page.locator('#dlDir').fill('D:\\Draft folder');
    await page.evaluate(()=>loadDL(true));
    assert.equal(await page.locator('#dlDir').inputValue(),'D:\\Draft folder');
    await page.locator('[data-dm=notifyTog]').click();
    assert.equal(await page.locator('#dlDir').inputValue(),'D:\\Draft folder');
    await page.locator('#dlDirSave').click();
    await page.waitForFunction(()=>snapshot.config.dir==='D:\\Draft folder');
    assert.equal(await page.evaluate(()=>snapshot.config.unrelated),'preserve me');
    await page.evaluate(()=>{window.allow=false;});
    await page.locator('[data-cfg=afterAll]').selectOption('shutdown');
    assert.equal(await page.locator('[data-cfg=afterAll]').inputValue(),'');
    await page.evaluate(()=>{window.allow=true;});

    await page.locator('[data-dm=folders]').click();
    await page.locator('.fc').first().fill('D:\\Programs draft');
    await page.locator('#fAdd').click();
    assert.equal(await page.locator('.fc').first().inputValue(),'D:\\Programs draft');
    await page.locator('#fSave').click();
    await page.waitForFunction(()=>!document.querySelector('#dlDlg'));

    await page.locator('[data-dl-tab=transfers]').first().click();
    await page.locator('#dlNew').click();
    await page.locator('#dlUrl').fill('https://example.test/video.m3u8');
    await page.locator('#dlAddBtn').click();
    await page.locator('#dlName').fill('My video.mp4');
    await page.locator('#dlQual').selectOption('720');
    assert.equal(await page.locator('#dlName').inputValue(),'My video.mp4');
    await page.locator('#dlLater').click();
    await page.waitForFunction(()=>!document.querySelector('#dlDlg'));
    const add=await page.evaluate(()=>calls.filter(c=>c.body?.action==='add').at(-1).body);
    assert.equal(add.start,false);assert.equal(add.height,720);assert.equal(add.name,'My video.mp4');

    await page.locator('#dlNew').click();
    await page.locator('#dlUrl').fill('https://example.test/file');
    await page.keyboard.press('Escape');
    assert.equal(await page.locator('.dl-confirm').count(),1);
    await page.locator('[data-answer=no]').click();
    assert.equal(await page.locator('#dlUrl').inputValue(),'https://example.test/file');
    await page.keyboard.press('Escape');await page.locator('[data-answer=yes]').click();
    assert.equal(await page.evaluate(()=>document.activeElement.id),'dlNew');

    await page.locator('[data-dl-tab=browser]').first().click();
    await page.locator('#dlExtMin').selectOption('10');
    await page.locator('#dlExtOn').click();
    await page.waitForFunction(()=>snapshot.ext.enabled===false);
    assert.equal(await page.evaluate(()=>snapshot.ext.minMB),10);
    await page.locator('#dlExtCode').click();
    assert.equal(await page.locator('#dlDlg').getAttribute('open'),'');
    assert.match(await page.locator('#dlExtLeft').innerText(),/one use/);
    await page.locator('#dlCancel').click();

    // New codes are generated only on explicit request, and clipboard rejection is local.
    await page.locator('#dlExtCode').click();
    await page.locator('#dlCopyCode').click();
    await page.locator('#dlCancel').click();

    await page.locator('[data-dl-tab=queues]').first().click();
    await page.locator('[data-start-queue=night]').click();
    assert.equal(await page.evaluate(()=>calls.filter(c=>c.body?.action==='startQueue').at(-1).body.queue),'night');
    await page.locator('[data-dm=queues]').first().click();
    await page.locator('.qn').fill('Weeknight');
    await page.locator('.qw').uncheck();
    await page.locator('#qSave').click();
    await page.waitForFunction(()=>!document.querySelector('#dlDlg'));
    assert.equal(await page.evaluate(()=>snapshot.config.queues[0].name),'Weeknight');
    assert.equal(await page.evaluate(()=>snapshot.config.queues[0].window.enabled),false);

    await page.evaluate(()=>{snapshot.items[0].state='done';nmDownloadsTab('transfers');});
    await page.locator('#dlSearch').fill('Installer');
    await page.evaluate(()=>{window.allow=false;});
    const before=await page.evaluate(()=>calls.filter(c=>c.body?.action==='open').length);
    await page.locator('[data-dl=open]').click();
    assert.equal(await page.evaluate(()=>calls.filter(c=>c.body?.action==='open').length),before);
    await page.locator('.dl-name').click();
    await page.locator('[data-dl=delfile]').click();
    await page.locator('[data-answer=no]').click();
    assert.equal(await page.evaluate(()=>calls.some(c=>c.body?.deleteFiles)),false);
    await page.locator('[data-dl=delfile]').click();
    await page.locator('[data-answer=yes]').click();
    await page.waitForFunction(()=>calls.some(c=>c.body?.action==='remove'&&c.body.deleteFiles===true));
    await page.waitForFunction(()=>!document.querySelector('#dlDlg'));

    await page.locator('#dlSearch').fill('Transfer 0002');
    await page.locator('.dl-name').click();
    await page.locator('#pQ').selectOption('night');
    await page.locator('#pC').selectOption('4');
    await page.locator('#pM').fill('https://mirror.test/a');
    await page.locator('#pSave').click();
    await page.waitForFunction(()=>!document.querySelector('#dlDlg'));
    assert.deepEqual(await page.evaluate(()=>calls.filter(c=>['setQueue','setConns','setMirrors'].includes(c.body?.action)).slice(-3).map(c=>c.body)),[
      {action:'setQueue',id:'2',queue:'night'}, {action:'setConns',id:'2',conns:4}, {action:'setMirrors',id:'2',urls:['https://mirror.test/a']}
    ]);
    await page.locator('.dl-name').click();
    await page.locator('#pLink').fill('https://new.test/a');
    await page.locator('#pHeader').fill('X-Example: test');
    await page.locator('#pRelink').click();
    await page.waitForFunction(()=>!document.querySelector('#dlDlg'));
    assert.deepEqual(await page.evaluate(()=>calls.filter(c=>c.body?.action==='relink').at(-1).body),{action:'relink',id:'2',url:'https://new.test/a',header:{'X-Example':'test'}});

    await page.locator('#dlNew').click();
    await page.locator('[data-dm=options]').click();
    await page.locator('#oUrl').fill('https://example.test/a');
    await page.locator('#oLimit').selectOption('2');
    await page.locator('#oHeader').fill('X-Example: test');
    await page.locator('#oLater').click();
    await page.waitForFunction(()=>!document.querySelector('#dlDlg'));
    const options=await page.evaluate(()=>calls.filter(c=>c.body?.action==='add').at(-1).body);
    assert.equal(options.limit,2097152);assert.equal(options.start,false);assert.equal(options.force,false);assert.deepEqual(options.header,{'X-Example':'test'});

    await page.locator('#dlNew').click();
    await page.locator('#dlTorFile').setInputFiles({name:'demo.torrent',mimeType:'application/x-bittorrent',buffer:Buffer.from('fixture')});
    await page.locator('#tpAll').uncheck();
    await page.locator('#tpGo').click();
    assert.match(await page.locator('.dl-notice').innerText(),/Choose at least one file/);
    await page.locator('.tpSel').first().check();
    await page.locator('#tpLater').click();
    await page.waitForFunction(()=>!document.querySelector('#dlDlg'));
    const torrent=await page.evaluate(()=>calls.filter(c=>c.body?.torrent&&c.body.action==='add').at(-1).body);
    assert.deepEqual(torrent.select,[true,false]);assert.equal(torrent.start,false);

    await page.locator('.dl-menu summary').click();
    await page.locator('[data-dm=grab]:visible').click();
    await page.locator('#gUrl').fill('https://example.test/page');
    await page.locator('#gType').selectOption('Archives');
    await page.locator('#gFind').click();
    await page.locator('#gAdd').click();
    await page.waitForFunction(()=>!document.querySelector('#dlDlg'));
    assert.deepEqual(await page.evaluate(()=>calls.filter(c=>c.body?.action==='grab').at(-1).body.types),['zip','rar','7z','tar','gz','tgz','bz2','xz']);

    await page.locator('#dlSearch').fill('');
    await page.evaluate(()=>{dlSel.clear();dlSel.add('2');dlSel.add('3');window.failID='3';});
    await page.evaluate(()=>dlBulk('pause'));
    assert.match(await page.evaluate(()=>toasts.at(-1)[0]),/1 done, 1 could not be changed/);
    await page.evaluate(()=>{window.failID=undefined;});
    await page.locator('#dlSearch').fill('Transfer 0002');
    await page.evaluate(async()=>{snapshot.items[2].size=0;await loadDL(true);});
    assert.equal(await page.locator('.dl-progress-cell [role=progressbar]').count(),0);
    await page.evaluate(async()=>{snapshot.items[2].size=100000;await loadDL(true);});
    assert.equal(await page.locator('.dl-progress-cell [role=progressbar]').count(),1);
    assert.equal(await page.locator('.dl-progress-cell').count(),1);
    await page.locator('#dlSearch').fill('');

    for(const width of [1440,375,320]) {
      await page.setViewportSize({width,height:900});
      await page.evaluate(()=>{document.body.style.margin='16px';nmDownloadsTab('transfers');});
      assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`overflow at ${width}`);
      if(process.env.DOWNLOADS_SCREENSHOTS)await page.screenshot({path:process.env.DOWNLOADS_SCREENSHOTS+`/downloads-${width}.png`});
    }
    await page.setViewportSize({width:1440,height:1000});
    await page.evaluate(()=>{document.documentElement.dataset.theme='dark';nmDownloadsTab('settings');});
    if(process.env.DOWNLOADS_SCREENSHOTS)await page.screenshot({path:process.env.DOWNLOADS_SCREENSHOTS+'/downloads-settings-dark.png'});
    assert.deepEqual(errors,[]);
  } finally {await browser.close();}
});
