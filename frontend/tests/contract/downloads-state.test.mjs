import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

// Run the shipped classic scripts; only the browser and transport boundary are replaced.
function runtime(api = async () => ({items:[], config:{}, lines:[]}), hash='#/downloads/transfers') {
  const document = {addEventListener(){}, querySelector(){return null;}, querySelectorAll(){return [];}, hidden:false};
  const context = vm.createContext({document, api, console, URL, AbortController,
    location:{hash}, setTimeout(){}, clearTimeout(){}, setInterval(){}, clearInterval(){},
    $:()=>null, $$:()=>[], esc:s=>String(s??'').replace(/[&<>"']/g, c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])),
    bytes:n=>`${n} B`, toast(){}, colorOf:()=>'', labelOf:n=>n, page:'downloads', DM_ON:true, S:{}, local:{}, go(){}, setHTML(){}});
  context.window = context; context.addEventListener = ()=>{};
  for (const file of ['downloads-more.js','downloads.js']) vm.runInContext(readFileSync(new URL(`../../../ui/${file}`,import.meta.url),'utf8'),context);
  return {context, run:code=>vm.runInContext(code,context)};
}
const plain = value => JSON.parse(JSON.stringify(value));

test('1000 transfers are paged without losing matches or off-page selection', () => {
  const {run} = runtime();
  assert.equal(run('typeof dlPageItems'), 'function');
  run(`DL={items:Array.from({length:1000},(_,i)=>({id:String(i),name:'file '+i,state:'paused',order:i})),config:{queues:[]}}; dlSel.add('990');`);
  assert.equal(run('dlPageItems().items.length'),100);
  assert.equal(run('dlPageItems().total'),1000);
  run('dlPage=9');
  assert.equal(run('dlPageItems().items[0].id'),'900');
  assert.equal(run("dlSel.has('990')"),true);
  run("dlQ='file 999'");
  assert.deepEqual(plain(run('dlPageItems().items.map(x=>x.id)')),['999']);
  assert.equal(run('dlPage'),0);
});

test('config merges only intended edits into fresh maps and queues', () => {
  const {run} = runtime();
  assert.equal(run('typeof dlMergeConfig'),'function');
  const result = run(`dlMergeConfig(
    {dir:'old',limitLines:{A:10,B:20},queues:[{id:'q',name:'Night',paused:false,window:{enabled:false,from:'22:00'}}]},
    {dir:'mine',limitLines:{B:20},queues:[{id:'q',name:'Weekend',paused:false,window:{enabled:false,from:'22:00'}}]},
    {dir:'external',limitLines:{A:10,B:30,C:40},siteRules:[{site:'example.test',dir:'D'}],queues:[{id:'q',name:'Night',paused:true,window:{enabled:true,from:'23:00'}},{id:'new',name:'New'}],unknown:true})`);
  assert.deepEqual(plain(result),{dir:'mine',limitLines:{B:30,C:40},siteRules:[{site:'example.test',dir:'D'}],queues:[{id:'q',name:'Weekend',paused:true,window:{enabled:true,from:'23:00'}},{id:'new',name:'New'}],unknown:true});
});

test('config writes serialize fresh reads, preserve unrelated changes and inverse flags', async () => {
  let current = {dir:'old', noTorrents:false, noUpload:false, noNotify:false, queues:[{id:'q',name:'Night'}], external:42};
  const writes=[];
  const {run} = runtime(async (path,body)=>{
    assert.equal(path,'downloads');
    if (body) {writes.push(plain(body)); current=body.config;}
    return {items:[], config:plain(current),lines:[]};
  });
  run("DL={items:[],config:{dir:'old',noTorrents:false,noUpload:false,noNotify:false,queues:[]},lines:[]}");
  await run("Promise.all([dlAct({action:'config',config:{...DL.config,noTorrents:true}}),dlAct({action:'config',config:{...DL.config,noNotify:true}})])");
  assert.equal(writes.length,2);
  assert.equal(current.noTorrents,true);
  assert.equal(current.noNotify,true);
  assert.equal(current.noUpload,false);
  assert.equal(current.external,42);
  assert.deepEqual(current.queues,[{id:'q',name:'Night'}]);
});

test('polling deduplicates slow reads, normalizes null lists and tolerates manager off',async()=>{
  let count=0, resolve;
  const {run}=runtime(()=>{count++;return new Promise(r=>resolve=r);});
  const a=run('loadDL(true)'), b=run('loadDL(true)');
  assert.equal(count,1);
  resolve({items:null,lines:null,config:{queues:null,window:null},ext:{browsers:null}});
  await Promise.all([a,b]);
  assert.deepEqual(plain(run('DL.items')),[]);
  const c=run('loadDL(true)'); resolve({enabled:false}); await c;
  assert.equal(run('DL.enabled'),false);
});

test('unknown totals never invent percentages and names render as text',()=>{
  const {run}=runtime();
  run('DL={items:[],config:{queues:[]}}');
  const html=run(`dlRow({id:'x',name:'<img src=x onerror=alert(1)>',state:'downloading',done:20,size:0,url:null})`);
  assert.ok(!html.includes('<img'));
  assert.ok(!html.includes('NaN'));
  assert.ok(!html.includes('aria-valuenow='));
  assert.match(html,/Size unknown/i);
});

test('router accepts only local Downloads tabs and reads initial hash',()=>{
  const {run,context}=runtime(undefined,'#/downloads/settings');
  assert.equal(typeof context.nmDownloadsTab,'function');
  assert.equal(run('dlTab'),'settings');
  context.nmDownloadsTab('browser');
  assert.equal(run('dlTab'),'browser');
  context.nmDownloadsTab('invalid');
  assert.equal(run('dlTab'),'transfers');
});

test('an old poll cannot replace a mutation and failed config POST is never retried',async()=>{
  let resolveOld, reads=0, posts=0;
  const {run}=runtime(async(_path,body)=>{
    if(body) {posts++;throw Error('Save outcome unknown');}
    if(++reads===1)return new Promise(resolve=>{resolveOld=resolve;});
    return {items:[{id:'new',state:'paused'}],config:{dir:'latest',external:9},lines:[]};
  });
  run("DL={items:[],config:{dir:'old'},lines:[]}");
  const poll=run('loadDL(true)');
  const action=run("dlAct({action:'config',config:{dir:'draft'}})");
  await new Promise(resolve=>setImmediate(resolve));
  resolveOld({items:[{id:'obsolete'}],config:{dir:'old'},lines:[]});
  await Promise.all([poll,action]);
  assert.equal(posts,1);
  assert.equal(run('DL.items[0].id'),'new');
  assert.equal(run('DL.config.external'),9);
});

test('new map entries preserve externally added keys and queue deletion preserves other queues',()=>{
  const {run}=runtime();
  assert.deepEqual(plain(run("dlMergeConfig({catDirs:null},{catDirs:{Videos:'D'}},{catDirs:{Music:'M'}})")),{catDirs:{Music:'M',Videos:'D'}});
  assert.deepEqual(plain(run("dlMergeConfig({queues:[{id:'a'}]},{queues:[]},{queues:[{id:'a'},{id:'b'}]})")),{queues:[{id:'b'}]});
});

test('live speed sorting stays stable until an explicit sort/filter change',()=>{
  const {run}=runtime();
  run("dlSort='speed';DL={items:[{id:'a',state:'downloading',speed:20},{id:'b',state:'downloading',speed:10}],config:{queues:[]}};dlPageItems()");
  run('DL.items[1].speed=30');
  assert.deepEqual(plain(run('dlPageItems().items.map(x=>x.id)')),['a','b']);
  run("dlOrderKey=''");
  assert.deepEqual(plain(run('dlPageItems().items.map(x=>x.id)')),['b','a']);
});

test('partial add results and duplicate feedback do not claim all jobs started',()=>{
  const {run}=runtime();
  assert.equal(run('dlAddResult({added:[{id:"a"}],skipped:1},4)'), '1 downloads added, 1 already in your list, 2 could not be added');
  assert.match(run('dlAddResult({duplicate:true},1)'),/No second copy/);
});

test('concurrent browser preferences preserve the latest untouched preference',async()=>{
  let ext={enabled:true,minMB:5,browsers:[]};
  const {run}=runtime(async(_path,body)=>{
    if(body){ext={...ext,enabled:!body.extOff,minMB:body.extMinMB};}
    return {items:[],config:{},lines:[],ext:plain(ext)};
  });
  run('DL={items:[],config:{},lines:[],ext:{enabled:true,minMB:5,browsers:[]}}');
  await run("Promise.all([dlAct({action:'extConfig',extMinMB:10,extOff:false}),dlAct({action:'extConfig',extMinMB:5,extOff:true})])");
  assert.equal(ext.minMB,10);assert.equal(ext.enabled,false);
});
