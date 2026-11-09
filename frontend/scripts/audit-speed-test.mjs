// Reproduce speed-test screenshots and layout/a11y checks with synthetic API readings.
import { chromium } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { createFixtureServer } from './serve.mjs';
import { NOW } from '../tests/fixtures/data.mjs';

const output=resolve(process.env.NETMERGE_AUDIT_OUTPUT||'/tmp/netmerge-speedtest-review/audit');
await mkdir(output,{recursive:true});
const server=createFixtureServer();
await new Promise(r=>server.listen(0,'127.0.0.1',r));
const origin=`http://127.0.0.1:${server.address().port}`;
const browser=await chromium.launch(process.env.NETMERGE_BROWSER_EXECUTABLE?{executablePath:process.env.NETMERGE_BROWSER_EXECUTABLE}:{});
const report=[],errors=[];
const control=async(path,body)=>{
  const response=await fetch(origin+'/__test/'+path,{method:'POST',headers:{'Content-Type':'application/json','X-NetMerge-Test':'1'},body:JSON.stringify(body)});
  if(!response.ok)throw Error(await response.text());
};
const results=[{name:'Ethernet',label:'Ethernet',mbps:86.4,single:42.8,ip:'192.0.2.10'},
  {name:'Wi-Fi',label:'Wi-Fi',mbps:34.6,single:24.2,ip:'192.0.2.20'}];
const completed={running:false,phase:'Done',progress:1,live:0,results,combined:118.2,merged:112.4,
  scaling:[{streams:1,mbps:42.8},{streams:2,mbps:68.2},{streams:4,mbps:96.5},{streams:8,mbps:112.4},{streams:16,mbps:109.1},{streams:32,mbps:110.6}],
  suggest:{Ethernet:3,'Wi-Fi':1},finished:'2026-11-09T12:00:00Z',warning:''};
try {
  for(const theme of ['light','dark']){
    await control('reset',{});
    const context=await browser.newContext({viewport:{width:1440,height:1000},colorScheme:theme,reducedMotion:'reduce'});
    const page=await context.newPage();
    page.on('pageerror',e=>errors.push(e.message));
    await page.clock.install({time:new Date(NOW*1000)});
    await page.goto(origin+'/#/connections/speed-test');
    await page.waitForFunction(()=>document.querySelector('#testMeterTicks').children.length>0);
    await page.evaluate(()=>NM.dispose());
    const check=async(state,width)=>{
      await page.setViewportSize({width,height:1000});
      await page.evaluate(()=>renderSpeedGraph());
      const violations=(await new AxeBuilder({page}).include('#pg-test').analyze()).violations;
      const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth);
      report.push({theme,state,width,overflow,violations:violations.map(v=>({id:v.id,nodes:v.nodes.map(n=>n.target)}))});
      if((width===1440&&(state==='idle'||state==='live'))||(state==='details'&&width===390)){
        await page.evaluate(()=>{document.activeElement.blur();window.scrollTo({top:0,behavior:'instant'});});
        await page.screenshot({path:resolve(output,`${state}-${theme}-${width}.png`),fullPage:true});
      }
    };
    for(const width of [375,390,768,1024,1440,1920])await check('idle',width);
    const stages=[
      ['Testing Ethernet (8 streams)',[24,43,57,66,70,81,86,82,87,85,86,86.4]],
      ['Testing Wi-Fi (8 streams)',[12,22,31,33,35,34,36,34.6]],
      ['Testing all connections together',[43,70,94,109,114,118,117,118.2]],
      ['Testing through NetMerge (8 streams)',[39,65,82,96,105,111,112,112.4]],
    ];
    for(const [index,[phase,readings]] of stages.entries())for(const live of readings){
      await page.clock.fastForward(1000);
      await control('state',{state:{test:{running:true,phase,live,progress:(index+1)/5,results:index?results:[]}}});
      await page.evaluate(()=>poll(true));
    }
    for(const width of [375,390,768,1024,1440,1920])await check('live',width);
    await control('state',{state:{test:completed}});await page.evaluate(()=>poll(true));
    await page.locator('#testDetails > summary').click();
    for(const width of [375,390,768,1024,1440,1920])await check('details',width);
    await page.setViewportSize({width:1440,height:1000});
    await page.evaluate(()=>{document.activeElement.blur();window.scrollTo({top:0,behavior:'instant'});});
    await page.screenshot({path:resolve(output,`details-${theme}-1440.png`),fullPage:true});
    await context.close();
  }
  await writeFile(resolve(output,'audit.json'),JSON.stringify({synthetic:true,report,errors},null,2));
  const failures=report.filter(r=>r.overflow||r.violations.length);
  console.log(JSON.stringify({cases:report.length,failures,errors,output},null,2));
  if(failures.length||errors.length)process.exitCode=1;
}finally{await browser.close();await new Promise(r=>server.close(r));}
