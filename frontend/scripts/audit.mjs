// Reproducible visual/accessibility evidence. Fixtures contain synthetic data only.
import {chromium} from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {createFixtureServer} from './serve.mjs';
import {NOW} from '../tests/fixtures/data.mjs';
const output=resolve(process.env.NETMERGE_AUDIT_OUTPUT||'/tmp/netmerge-frontend-audit');
await mkdir(output,{recursive:true});
const server=createFixtureServer();await new Promise(done=>server.listen(0,'127.0.0.1',done));
const origin=`http://127.0.0.1:${server.address().port}`;
const browser=await chromium.launch(process.env.NETMERGE_BROWSER_EXECUTABLE?{executablePath:process.env.NETMERGE_BROWSER_EXECUTABLE}:{});
const routes=['overview','connections','connections/speed-test','connections/presets','apps/active','apps/rules','usage','usage/allowances','protection/routing','protection/blocking','protection/dns','sharing/connect','sharing/devices','activity/events','activity/logs','settings/general','settings/appearance','settings/maintenance','settings/advanced'];
const reports=[],errors=[],external=[];
try{
  for(const theme of ['light','dark'])for(const [width,height] of [[375,812],[390,844],[768,1024],[1024,768],[1440,900],[1920,1080]]){
    await fetch(origin+'/__test/reset',{method:'POST',headers:{'X-NetMerge-Test':'1','Content-Type':'application/json'},body:'{}'});
    await fetch(origin+'/api/share',{method:'POST',headers:{'X-NetMerge':'1','Content-Type':'application/json'},body:'{"enabled":true}'});
    const context=await browser.newContext({viewport:{width,height},colorScheme:theme,reducedMotion:'reduce',locale:'en-US',timezoneId:'UTC'});
    await context.route('**/*',route=>{if(route.request().url().startsWith(origin)||/^(data|blob):/.test(route.request().url()))route.continue();else{external.push(route.request().url());route.abort();}});
    const page=await context.newPage();page.on('pageerror',error=>errors.push({theme,width,error:error.stack}));
    await page.clock.setFixedTime(new Date(NOW*1000));
    await page.goto(origin);await page.waitForFunction(()=>document.querySelector('#power')?.disabled===false);
    for(const route of routes){
      await page.evaluate(route=>location.hash='#/'+route,route);
      await page.waitForTimeout(150);
      const result=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
      const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1);
      const report={theme,width,route,overflow,violations:result.violations.map(v=>({id:v.id,impact:v.impact,nodes:v.nodes.map(n=>({target:n.target,summary:n.failureSummary}))}))};reports.push(report);
      if(overflow||report.violations.length)process.stdout.write(JSON.stringify(report)+'\n');
      if(['overview','connections','settings/general','settings/appearance'].includes(route))await page.screenshot({path:resolve(output,`${theme}-${width}-${route.replaceAll('/','-')}.png`),fullPage:true});
    }
    await context.close();process.stdout.write(`${theme} ${width}: ${routes.length} routes inspected\n`);
  }
}finally{
  await browser.close();server.closeAllConnections();await new Promise(done=>server.close(done));
  await writeFile(resolve(output,'audit.json'),JSON.stringify({reports,errors,external},null,2));
}
const failed=reports.some(r=>r.overflow||r.violations.length)||errors.length||external.length;
process.stdout.write(`${reports.length} route/theme/viewport cases; ${errors.length} runtime errors; ${external.length} external requests\n`);
if(failed)process.exitCode=1;
