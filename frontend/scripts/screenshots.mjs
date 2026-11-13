// Screenshots of the window for the repository (docs/screenshots), taken from the fixture server: synthetic
// data only, nothing from a real PC. Run: npm run screenshots
import {chromium} from '@playwright/test';
import {mkdir} from 'node:fs/promises';
import {resolve} from 'node:path';
import {createFixtureServer} from './serve.mjs';
import {NOW} from '../tests/fixtures/data.mjs';
const output=resolve(process.env.NETMERGE_SCREENSHOTS||new URL('../../docs/screenshots',import.meta.url).pathname);
await mkdir(output,{recursive:true});
const server=createFixtureServer();await new Promise(done=>server.listen(0,'127.0.0.1',done));
const origin=`http://127.0.0.1:${server.address().port}`;
const browser=await chromium.launch(process.env.NETMERGE_BROWSER_EXECUTABLE?{executablePath:process.env.NETMERGE_BROWSER_EXECUTABLE}:{});
const shots=[
  ['light',1440,900,'overview'],['dark',1440,900,'overview'],['light',390,844,'overview'],
  ['light',1440,900,'connections'],['light',1440,900,'connections/speed-test'],['light',1440,900,'connections/presets'],
  ['light',1440,900,'apps/active'],['light',1440,900,'apps/rules'],['light',1440,900,'usage'],
  ['light',1440,900,'protection/blocking'],['light',1440,900,'protection/dns'],['light',1440,900,'sharing/connect'],
  ['light',1440,900,'activity/events'],['light',1440,900,'settings/general'],['light',1440,900,'settings/advanced'],['light',1440,900,'settings/about'],
  ['dark',1440,900,'connections'],['dark',1440,900,'usage'],
];
const errors=[];
try{
  for(const [theme,width,height,route] of shots){
    await fetch(origin+'/__test/reset',{method:'POST',headers:{'X-NetMerge-Test':'1','Content-Type':'application/json'},body:'{}'});
    await fetch(origin+'/api/share',{method:'POST',headers:{'X-NetMerge':'1','Content-Type':'application/json'},body:'{"enabled":true}'});
    // 90 seconds of live readings for the charts (the test fixture itself keeps only a few)
    const st=await (await fetch(origin+'/api/state',{headers:{'X-NetMerge':'1'}})).json();
    const wave=(base,amp,period,phase)=>Array.from({length:90},(_,i)=>Math.round((base+amp*Math.sin(i/period+phase)+amp*0.4*Math.sin(i/3.1+phase))*131072));
    const links=st.snap.links.map((l,i)=>({...l,history:wave(i?8:16,i?2.5:4,i?7:9,i)}));
    const total=links[0].history.map((v,k)=>links.reduce((a,l)=>a+l.history[k],0));
    // and a month of daily use, newest day first as NetMerge sends it, up to the fixture's "today" (2026-11-09)
    const days=Array.from({length:31},(_,k)=>{const d=new Date(Date.UTC(2026,10,9-k)); const w=1+0.5*Math.sin(k/2.3)+(d.getUTCDay()%6===0?0.6:0);
      return {date:d.toISOString().slice(0,10),links:{Ethernet:[Math.round(2.4e9*w),Math.round(1.6e8*w)],'Wi-Fi':[Math.round(1.1e9*w),Math.round(7e7*w)]}};});
    await fetch(origin+'/__test/state',{method:'POST',headers:{'X-NetMerge-Test':'1','Content-Type':'application/json'},body:JSON.stringify({state:{snap:{links,total}},usage:{days}})});
    const context=await browser.newContext({viewport:{width,height},colorScheme:theme,reducedMotion:'reduce',locale:'en-US',timezoneId:'UTC',deviceScaleFactor:1});
    await context.route('**/*',route=>route.request().url().startsWith(origin)||/^(data|blob):/.test(route.request().url())?route.continue():route.abort());
    const page=await context.newPage();page.on('pageerror',e=>errors.push(`${route}: ${e.message}`));
    await page.clock.install({time:new Date(NOW*1000)});
    await page.goto(origin+'/#/'+route);await page.waitForFunction(()=>document.querySelector('#power')?.disabled===false);
    if(route==='settings/advanced')await page.evaluate(()=>{const d=document.querySelector('details.adv');if(d)d.open=true;});
    // let 90 seconds of the window's own clock pass, so the live charts are full
    await page.clock.runFor(90000);await page.waitForTimeout(800);
    // a window as tall as the page, so the side bar runs the full height
    const full=await page.evaluate(()=>Math.max(document.documentElement.scrollHeight,innerHeight));
    await page.setViewportSize({width,height:full});await page.waitForTimeout(300);
    const name=`${route.replaceAll('/','-')}-${theme}${width<600?'-narrow':''}.png`;
    await page.screenshot({path:resolve(output,name)});
    process.stdout.write(name+'\n');
    await context.close();
  }
}finally{await browser.close();server.closeAllConnections();await new Promise(done=>server.close(done));}
if(errors.length){process.stdout.write(errors.join('\n')+'\n');process.exitCode=1;}
