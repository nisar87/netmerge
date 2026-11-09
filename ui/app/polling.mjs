/** One in-flight operation per resource. Never schedules a mutation. */
export function createScheduler({setTimer=setTimeout,clearTimer=clearTimeout} = {}) {
  const tasks = new Map();
  function schedule(key,fn,ms,predicate=()=>true) {
    tasks.get(key)?.();
    let stopped=false, timer, failures=0;
    const tick=async()=>{
      if(stopped)return;
      if(predicate()) {
        try {const result=await fn();failures=result===false?Math.min(failures+1,3):0;}catch{failures=Math.min(failures+1,3);}
      }
      if(!stopped)timer=setTimer(tick,Math.min(30000,ms*Math.max(1,2**failures)));
    };
    const dispose=()=>{stopped=true;clearTimer(timer);if(tasks.get(key)===dispose)tasks.delete(key);};
    tasks.set(key,dispose);timer=setTimer(tick,ms);
    return dispose;
  }
  return {schedule,dispose(){for(const stop of [...tasks.values()])stop();}};
}
