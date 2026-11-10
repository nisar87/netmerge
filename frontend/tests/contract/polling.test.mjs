import test from 'node:test';
import assert from 'node:assert/strict';
import {createScheduler} from '../../../ui/app/polling.mjs';

test('a slow poll has one owner and a replaced owner cannot restart itself',async()=>{
  let id=0,finish;const timers=new Map();
  const scheduler=createScheduler({setTimer:(fn,ms)=>{timers.set(++id,{fn,ms});return id;},clearTimer:id=>timers.delete(id)});
  scheduler.schedule('state',()=>new Promise(resolve=>finish=resolve),1000);
  const first=timers.get(1);timers.delete(1);const running=first.fn();
  assert.equal(timers.size,0);
  scheduler.schedule('state',async()=>{},1000);
  finish();await running;assert.equal(timers.size,1);
  scheduler.dispose();assert.equal(timers.size,0);
});

test('failed reads back off and a successful read resets the delay',async()=>{
  let id=0,okay=false;const timers=new Map();
  const scheduler=createScheduler({setTimer:(fn,ms)=>{timers.set(++id,{fn,ms});return id;},clearTimer:id=>timers.delete(id)});
  scheduler.schedule('state',async()=>okay,1000);
  const run=async()=>{const [key,timer]=timers.entries().next().value;timers.delete(key);await timer.fn();return [...timers.values()][0].ms;};
  assert.equal(await run(),2000);assert.equal(await run(),4000);
  okay=true;assert.equal(await run(),1000);scheduler.dispose();
});
