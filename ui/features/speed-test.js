/* One meter, an observed-sample timeline, and progressively disclosed results. */
let speedTestPending=false,speedTestError='',speedWeightsPending=false;
let speedObservedState=null,speedPrevious=null,speedStartedAt=0,speedSamples=[],speedGraphFrozen=null,speedSelected=null;
const speedValid=value=>typeof value==='number'&&Number.isFinite(value)&&value>=0;
const speedNumber=value=>speedValid(value)?value.toFixed(value>=1000?0:1):'—';
const speedPhase=t=>typeof t.phase==='string'?t.phase:'';
const speedMeasuring=t=>t.running&&speedPhase(t).startsWith('Testing ');
function speedCeiling(value,minimum=100){
  const n=Math.max(minimum,value),power=10**Math.floor(Math.log10(n));
  return [1,2,5,10].find(step=>step*power>=n)*power;
}
function speedResult(t){
  if((t.scaling||[]).length)return {value:speedValid(t.merged)?t.merged:null,label:'Best through NetMerge'};
  if(speedValid(t.combined)&&t.combined>0)return {value:t.combined,label:'All connections together'};
  const best=(t.results||[]).filter(r=>!r.error&&speedValid(r.mbps)&&r.mbps>0).sort((a,b)=>b.mbps-a.mbps)[0];
  return best?{value:best.mbps,label:`Best connection · ${best.label||best.name}`}:{value:null,label:'Download speed'};
}

// Observe accepted state objects once, including while another page is open. A redraw,
// failed poll or theme change must never turn an old reading into a new sample.
function observeSpeedTest(){
  if(!S||S===speedObservedState)return;
  speedObservedState=S;
  const t=S.test,previous=speedPrevious;
  const restarted=previous&&S.snap.app!==previous.app;
  const newRun=t.running&&(!previous?.running||(t.progress||0)<previous.progress||t.finished!==previous.finished);
  const unseenResult=!t.running&&previous&&!previous.running&&t.finished!==previous.finished;
  const cleared=!t.running&&!speedPhase(t)&&speedSamples.length>0;
  if(newRun||restarted||unseenResult||cleared){
    speedStartedAt=performance.now();speedSamples=[];speedGraphFrozen=null;speedSelected=null;
  }
  if(speedMeasuring(t)&&speedValid(t.live)){
    speedSamples.push({seconds:(performance.now()-speedStartedAt)/1000,value:t.live,phase:speedPhase(t)});
    if(speedSamples.length>600)speedSamples.shift();
  }
  speedPrevious={running:!!t.running,progress:t.progress||0,finished:t.finished,app:S.snap.app};
}

function renderSpeedMeter(value){
  const max=speedCeiling(Math.max(value||0,...speedSamples.map(p=>p.value)));
  const ratio=speedValid(value)?Math.min(1,value/max):0;
  const ticks=Array.from({length:21},(_,i)=>{
    const angle=Math.PI*(1-i/20),major=i%5===0;
    const x=r=>220+Math.cos(angle)*r,y=r=>230-Math.sin(angle)*r;
    return `<line x1="${x(166)}" y1="${y(166)}" x2="${x(major?156:161)}" y2="${y(major?156:161)}"/>`+
      (major?`<text x="${x(210)}" y="${y(210)}">${(max*i/20).toLocaleString()}</text>`:'');
  }).join('');
  setHTML($('#testMeterTicks'),ticks);
  $('#testMeterFill').style.strokeDashoffset=100-ratio*100;
  $('#testMeterNeedle').style.transform=`rotate(${ratio*180}deg)`;
  $('#testMeterNeedle').toggleAttribute('hidden',!speedValid(value));
  $('#testSpeed').textContent=speedNumber(value);
  $('#testMeter').setAttribute('aria-label',speedValid(value)?`Download speed: ${speedNumber(value)} Mbps. Scale 0 to ${max} Mbps.`:'Download speed: not measured');
}

function renderSpeedGraph(){
  const t=S.test,points=speedGraphFrozen||speedSamples,chart=$('#testChart');
  const width=Math.round(chart.clientWidth);
  if(!width)return;
  const height=chart.clientHeight,left=42,right=12,top=20,bottom=26;
  const peak=Math.max(0,...points.map(p=>p.value)),max=speedCeiling(peak,10);
  const end=Math.max(10,Math.ceil((points.at(-1)?.seconds||0)/10)*10);
  const x=s=>left+s/end*(width-left-right),y=v=>height-bottom-v/max*(height-top-bottom);
  chart.setAttribute('viewBox',`0 0 ${width} ${height}`);
  let html=Array.from({length:3},(_,i)=>{
    const value=max*i/2,py=y(value);
    return `<line class="speed-chart-grid" x1="${left}" y1="${py}" x2="${width-right}" y2="${py}"/><text x="${left-8}" y="${py+4}" text-anchor="end">${value.toLocaleString()}</text>`;
  }).join('');
  html+=`<text x="${left-8}" y="10" text-anchor="end">Mbps</text>`;
  for(let i=0;i<4;i++)html+=`<text x="${x(end*i/3)}" y="${height-5}" text-anchor="${i===0?'start':i===3?'end':'middle'}">${Math.round(end*i/3)}s</text>`;
  const segments=[];
  points.forEach((point,i)=>{
    const prev=points[i-1];
    if(!prev||point.phase!==prev.phase||point.seconds-prev.seconds>3)segments.push([]);
    segments.at(-1).push(point);
  });
  for(const segment of segments){
    const line=segment.map((p,i)=>`${i?'L':'M'}${x(p.seconds).toFixed(2)},${y(p.value).toFixed(2)}`).join(' ');
    html+=`<path class="speed-chart-area" d="${line} L${x(segment.at(-1).seconds)},${y(0)} L${x(segment[0].seconds)},${y(0)} Z"/>`;
    html+=`<path data-speed-line class="speed-chart-line" d="${line}"/>`;
    if(segment.length===1)html+=`<circle class="speed-chart-point" cx="${x(segment[0].seconds)}" cy="${y(segment[0].value)}" r="3"/>`;
  }
  const selected=points.length?Math.min(speedSelected??points.length-1,points.length-1):0,p=points[selected];
  if(p)html+=`<line class="speed-chart-cursor" x1="${x(p.seconds)}" x2="${x(p.seconds)}" y1="${top}" y2="${y(0)}"/><circle class="speed-chart-point" cx="${x(p.seconds)}" cy="${y(p.value)}" r="4"/>`;
  setHTML(chart,html);
  const empty=$('#testGraphEmpty');empty.hidden=points.length>0;
  setHTML(empty,`<span>${t.running?'Waiting for live download readings…':speedPhase(t)==='Done'?'No live samples were recorded in this window. Your results are in Test details.':'Start a test to see live throughput.'}</span>`);
  $('#testGraphSummary').textContent=points.length?`${speedGraphFrozen?'Paused · ':''}Peak ${speedNumber(peak)} Mbps`:'Waiting for readings';
  const pause=$('#testChartPause');pause.disabled=!points.length||(!t.running&&!speedGraphFrozen);
  pause.setAttribute('aria-label',speedGraphFrozen?'Resume graph':'Pause graph');pause.title=pause.getAttribute('aria-label');
  setHTML(pause,NM.icon(speedGraphFrozen?'play':'pause'));
  $('#testInspector').hidden=!p;
  if(p){
    const label=`${p.seconds.toFixed(1)}s · ${p.phase} · ${speedNumber(p.value)} Mbps`;
    $('#testSample').max=points.length-1;$('#testSample').value=selected;
    $('#testSample').setAttribute('aria-valuetext',label);$('#testSampleValue').textContent=label;
  }
}

function renderSpeedDetails(t){
  const results=t.results||[],scaling=t.scaling||[],done=speedPhase(t)==='Done';
  $('#testDetailsHint').textContent=results.length?`${results.length} connection${results.length===1?'':'s'} measured · ${scaling.length} parallel-stream results`:'Connection speeds, parallel streams and recommendations';
  const rows=results.map(r=>`<tr><td><span class="speed-result-label${r.error?' has-error':''}">${NM.icon(r.error?'circle-alert':'network')}<span>${esc(r.label||r.name)}</span></span><small>${r.ip?`Public IP · ${esc(r.ip)}`:'Public IP unavailable'}</small>${r.error?`<small class="speed-result-error">${esc(r.error)}</small>`:''}</td><td class="speed-value">${speedNumber(r.single)}</td><td class="speed-value">${speedNumber(r.mbps)}</td></tr>`).join('');
  setHTML($('#testResults'),results.length?`<table class="tbl"><caption class="sr-only">Connection download speeds in Mbps</caption><thead><tr><th scope="col">Connection</th><th scope="col">1 stream · Mbps</th><th scope="col">8 streams · Mbps</th></tr></thead><tbody>${rows}</tbody></table>${done?`<div class="speed-combined"><span>All connections together</span><strong>${speedNumber(t.combined)} Mbps</strong></div>`:''}`:`<p class="hint2">${t.running?'Connection results will appear as each stage finishes.':done?'No connection results were returned. Check your enabled connections and try again.':'Start a test to compare your connections.'}</p>`);
  setHTML($('#testScaling'),scaling.length?`<table class="tbl"><caption class="sr-only">NetMerge download speeds by parallel stream count</caption><thead><tr><th scope="col">Parallel streams</th><th scope="col">Download · Mbps</th></tr></thead><tbody>${scaling.map(p=>`<tr><td>${esc(p.streams)} stream${p.streams===1?'':'s'}</td><td class="speed-value">${speedNumber(p.mbps)}</td></tr>`).join('')}</tbody></table>`:`<p class="hint2">${done?'Merging was off during this test. Turn it on and test again to measure NetMerge.':t.running?'Parallel-stream results will appear when the test finishes.':'The test compares each connection, all connections together, then parallel streams through NetMerge.'}</p>`);
  let notes='';
  if(done&&t.suggest&&Object.keys(t.suggest).length>1){
    const links=local.links.filter(l=>t.suggest[l.name]),current=links.every(l=>l.weight===t.suggest[l.name]);
    const text=links.map(l=>`${esc(l.label)} ×${esc(t.suggest[l.name])}`).join(', ');
    notes=current?`<div class="note info"><span>Shares already match your connection speeds (${text}).</span></div>`:`<div class="note info"><span>Suggested shares from this test: <b>${text}</b>. Shares are used by Classic balancing and by “steady sites” mode; Smart balancing finds the speeds by itself.</span><button class="btn pri" id="applyW"${speedWeightsPending?' disabled':''}>${speedWeightsPending?'Applying…':'Apply shares'}</button></div>`;
  }
  setHTML($('#testNotes'),notes);
  const apply=$('#applyW');if(apply)apply.onclick=async()=>{
    if(speedWeightsPending)return;
    speedWeightsPending=true;renderTest();
    try{await api('applyweights',{});await poll(true);toast('Shares updated');}
    catch(e){toast(e.message,true);}
    finally{speedWeightsPending=false;if(S)renderTest();}
  };
  $('#testSampleCount').textContent=`(${speedSamples.length})`;
  setHTML($('#testSamples tbody'),speedSamples.map(p=>`<tr><td>${p.seconds.toFixed(1)}s</td><td>${esc(p.phase)}</td><td>${speedNumber(p.value)}</td></tr>`).join(''));
}

function renderTest(){
  const t=S.test,run=!!t.running,done=speedPhase(t)==='Done',result=speedResult(t);
  const value=run?(speedMeasuring(t)&&speedValid(t.live)?t.live:null):done?result.value:null;
  const button=$('#testBtn');button.disabled=run||speedTestPending;
  setHTML(button,NM.icon(run||speedTestPending?'activity':done?'rotate-ccw':'play')+`<span>${run?'Testing…':speedTestPending?'Starting…':done?'Test again':'Start test'}</span>`);
  renderSpeedMeter(value);
  $('#testSpeedScope').textContent=run?'Live download speed':done?result.label:'Download speed';
  const phase=run?speedPhase(t):speedTestPending?'Starting your test…':done?'Results from your latest test':'Ready when you are';
  if($('#testPhase').textContent!==phase)$('#testPhase').textContent=phase;
  const status=$('#testStatus'),state=run||speedTestPending?'running':done?(result.value>0?'done':'warning'):'idle';
  status.dataset.state=state;
  setHTML(status,NM.icon(state==='running'?'activity':state==='done'?'circle-check':state==='warning'?'circle-alert':'circle-dot')+`<span>${state==='running'?'Test in progress':state==='done'?'Test complete':state==='warning'?'No speed measured':'Ready to test'}</span>`);
  const progress=Math.round(Math.min(1,Math.max(0,t.progress||0))*100);
  $('#testProg').hidden=!run;$('#testProg').setAttribute('aria-valuenow',progress);
  $('#testProg').firstElementChild.style.transform=`scaleX(${progress/100})`;
  let warnings='';
  if(speedTestError)warnings+=`<div class="note bad">${NM.icon('circle-alert')}<span>Could not start the speed test. ${esc(speedTestError)} Try again.</span></div>`;
  if(t.warning)warnings+=`<div class="note warn">${NM.icon('circle-alert')}<span>${esc(t.warning)}</span></div>`;
  if(done&&!run&&!(result.value>0))warnings+=`<div class="note warn">${NM.icon('circle-alert')}<span>No speed was measured. Check your connections and the errors in Test details, then try again.</span></div>`;
  if(done&&t.merged&&t.combined&&t.merged<t.combined*.6&&S.snap.running)warnings+=`<div class="note warn">${NM.icon('circle-alert')}<span>NetMerge reached ${speedNumber(t.merged)} of a possible ${speedNumber(t.combined)} Mbps. Test servers sometimes limit repeated tests — a real download is the best check.</span></div>`;
  setHTML($('#testWarnings'),warnings);
  renderSpeedGraph();renderSpeedDetails(t);
}
$('#testBtn').onclick=async()=>{
  if(speedTestPending||S?.test.running)return;
  speedTestPending=true;speedTestError='';renderTest();
  try{await api('speedtest',{});await poll(true);}
  catch(e){speedTestError=e.message;toast(e.message,true);}
  finally{speedTestPending=false;if(S)renderTest();}
};
$('#testChartPause').onclick=()=>{speedGraphFrozen=speedGraphFrozen?null:speedSamples.slice();speedSelected=null;renderSpeedGraph();};
$('#testSample').oninput=event=>{speedSelected=Number(event.target.value);renderSpeedGraph();};
new ResizeObserver(()=>{if(S&&page==='test')renderSpeedGraph();}).observe($('#testChart'));
