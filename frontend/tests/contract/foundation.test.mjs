import test from 'node:test';
import assert from 'node:assert/strict';
import {createClient} from '../../../ui/app/api.mjs';
import {createSettingsWriter, captureChanges, applyChanges} from '../../../ui/app/store.mjs';
import {contributingLinks, toMbps, normalizeState} from '../../../ui/app/format.mjs';

test('POST uses existing header and body; requests are never replayed',async()=>{
  const calls=[];const request=createClient(async(...args)=>{calls.push(args);return new Response('{"error":"no"}',{status:409})});
  await assert.rejects(request('power',{body:{on:true}}),/no/);
  assert.equal(calls.length,1);assert.equal(calls[0][0],'/api/power');assert.equal(calls[0][1].headers['X-NetMerge'],'1');
  assert.equal(calls[0][1].method,'POST');assert.deepEqual(JSON.parse(calls[0][1].body),{on:true});
});
test('concurrent GET reads share one request; errors include HTTP status',async()=>{
  let done,count=0;const request=createClient(()=>{count++;return new Promise(r=>done=r)});
  const a=request('state'),b=request('state');done(new Response('{}'));await Promise.all([a,b]);assert.equal(count,1);
  const bad=createClient(async()=>new Response('not json',{status:503}));await assert.rejects(bad('state'),/503/);
});
test('editing a connection label preserves newly changed weights, roles and unrelated settings',()=>{
  const base={links:[{name:'Ethernet',label:'LAN',weight:1,role:'',quotaGB:0}],appRules:[],autoStart:true};
  const draft=structuredClone(base);draft.links[0].label='Cable';
  const latest={...base,links:[{...base.links[0],weight:5,role:'backup',quotaGB:50},{name:'Wi-Fi',label:'Wireless',weight:2}],autoStart:false};
  const merged=applyChanges(latest,captureChanges(base,draft));
  assert.equal(merged.links[0].label,'Cable');assert.equal(merged.links[0].weight,5);assert.equal(merged.links[0].role,'backup');assert.equal(merged.links.length,2);assert.equal(merged.autoStart,false);
});
test('rule deletion/edit preserves rules added since the draft snapshot',()=>{
  const base={appRules:[{process:'a.exe',target:'stable'}]};
  const merged=applyChanges({appRules:[...base.appRules,{process:'b.exe',target:'bypass'}]},captureChanges(base,{appRules:[]}));
  assert.deepEqual(merged.appRules,[{process:'b.exe',target:'bypass'}]);
});
test('settings writes serialize and read authoritative state before each complete write',async()=>{
  let state={autoStart:true,noCallProtect:false,links:[]},active=0,max=0;
  const writer=createSettingsWriter({read:async()=>structuredClone(state),write:async body=>{active++;max=Math.max(max,active);await new Promise(r=>setTimeout(r,5));state=body;active--;}});
  const base=structuredClone(state);await Promise.all([writer.save(base,{...base,autoStart:false}),writer.save(base,{...base,noCallProtect:true})]);
  assert.equal(max,1);assert.equal(state.autoStart,false);assert.equal(state.noCallProtect,true);
});
test('a failed settings write does not poison the queue',async()=>{
  let fail=true,state={autoStart:true};const writer=createSettingsWriter({read:async()=>state,write:async s=>{if(fail){fail=false;throw Error('failed')}state=s}});
  await assert.rejects(writer.save(state,{autoStart:false}));await writer.save(state,{autoStart:false});assert.equal(state.autoStart,false);
});
test('live rate conversion and contribution states preserve unknowns',()=>{
  assert.equal(toMbps(1_000_000),8);assert.equal(toMbps(null),null);
  assert.deepEqual(contributingLinks({running:false,links:[{enabled:true,present:true}]}),[]);
  const healthy={enabled:true,present:true,healthy:true};
  assert.equal(contributingLinks({running:true,links:[healthy,{...healthy,standby:true},{...healthy,healthy:null},{...healthy,quotaOut:true}]}).length,1);
  const n=normalizeState({snap:{links:null},settings:{links:null},test:null});assert.deepEqual(n.snap.links,[]);assert.deepEqual(n.settings.links,[]);assert.equal(n.snap.totalDown,undefined);
});

test('authoritative reads can bypass a pending stale poll',async()=>{
  const completions=[];let calls=0;
  const request=createClient(()=>{calls++;return new Promise(resolve=>completions.push(resolve));});
  const poll=request('state'),fresh=request('state',{fresh:true});
  assert.equal(calls,2);
  completions[1](new Response('{"revision":2}'));assert.equal((await fresh).revision,2);
  completions[0](new Response('{"revision":1}'));await poll;
});
