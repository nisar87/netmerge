/* Isolated visual reference. All data is illustrative; no fetch or backend access. */
const $=s=>document.querySelector(s);
const $$=s=>[...document.querySelectorAll(s)];
let themeChoice='light';
const osTheme=matchMedia('(prefers-color-scheme: dark)');
function setTheme(choice){
  themeChoice=choice;
  const actual=choice==='auto'?(osTheme.matches?'dark':'light'):choice;
  document.documentElement.dataset.theme=actual;
  $('#theme-toggle span').textContent=actual==='light'?'Dark theme':'Light theme';
  $$('[data-theme-choice]').forEach(b=>b.setAttribute('aria-pressed',String(b.dataset.themeChoice===choice)));
}
$('#theme-toggle').onclick=()=>setTheme(document.documentElement.dataset.theme==='light'?'dark':'light');
$$('[data-theme-choice]').forEach(b=>b.onclick=()=>setTheme(b.dataset.themeChoice));
osTheme.addEventListener('change',()=>{if(themeChoice==='auto')setTheme('auto')});
function showView(name){
  $$('.view').forEach(v=>v.hidden=v.id!==name);
  $$('.nav[data-view]').forEach(b=>{b.classList.toggle('active',b.dataset.view===name);if(b.dataset.view===name)b.setAttribute('aria-current','page');else b.removeAttribute('aria-current')});
  $('#crumb').textContent=name[0].toUpperCase()+name.slice(1);
  $('.sidebar').classList.remove('open');$('#menu').setAttribute('aria-expanded','false');
  document.title=`NetMerge — ${name} design study`;
  if(name==='overview')drawTrafficChart();
  $('#'+name+' h1').focus({preventScroll:true});
  window.scrollTo({top:0,behavior:'instant'});
}
$$('[data-view]').forEach(b=>{
  b.onclick=()=>showView(b.dataset.view);
  if(b.classList.contains('nav'))b.setAttribute('aria-label',b.dataset.view[0].toUpperCase()+b.dataset.view.slice(1));
});
$('#menu').setAttribute('aria-expanded','false');
$('#menu').onclick=()=>{$('.sidebar').classList.toggle('open');$('#menu').setAttribute('aria-expanded',String($('.sidebar').classList.contains('open')))};
function drawTrafficChart(){
  const chart=$('#traffic-chart'),width=Math.max(280,chart.clientWidth),right=width-8;
  chart.setAttribute('viewBox',`0 0 ${width} 180`);
  const grid=[0,1,2,3].map(i=>`<line x1="34" x2="${right}" y1="${14+i*42}" y2="${14+i*42}" stroke="var(--line)"/><text x="0" y="${18+i*42}">${150-i*50}</text>`).join('');
  function path(base,amp,phase){return Array.from({length:61},(_,i)=>{
    const value=i===60?base:base+Math.sin(i*.52+phase)*amp+Math.sin(i*1.7)*amp*.28;
    return `${i?'L':'M'}${(34+i*(right-34)/60).toFixed(1)},${(140-value/150*126).toFixed(1)}`;
  }).join(' ')}
  const p1=path(84.2,20,0),p2=path(42.2,11,2);
  chart.innerHTML=`<defs><linearGradient id="chart-fill" x1="0" y1="0" x2="0" y2="1"><stop stop-color="var(--series-1)" stop-opacity=".16"/><stop offset="1" stop-color="var(--series-1)" stop-opacity="0"/></linearGradient></defs>${grid}<path d="${p1} L${right},140 L34,140Z" fill="url(#chart-fill)"/><path d="${p1}" stroke="var(--series-1)" stroke-width="2.4" fill="none"/><path d="${p2}" stroke="var(--series-2)" stroke-width="2" stroke-dasharray="6 3" fill="none"/><text x="34" y="166">90s ago</text><text x="${width/2}" y="166" text-anchor="middle">45s</text><text x="${right}" y="166" text-anchor="end">Now · Mbps</text>`;
}
drawTrafficChart();
addEventListener('resize',()=>{if(!$('#overview').hidden)drawTrafficChart()});
const rows=[
 {name:'ubuntu-desktop.iso',host:'releases.ubuntu.com · Programs',progress:68,size:'4.0 GB of 5.9 GB',speed:'10.4 MB/s',remaining:'About 3 min',state:'active'},
 {name:'Design resources.zip',host:'files.example.com · Archives',progress:42,size:'420 MB of 1.0 GB',speed:'5.4 MB/s',remaining:'About 2 min',state:'active'},
 {name:'Workshop recording.mp4',host:'media.example.com · Videos',progress:24,size:'Paused · 192 MB of 800 MB',speed:'—',remaining:'Paused',state:'active'},
 {name:'Getting started.pdf',host:'docs.example.com · Documents',progress:100,size:'Finished · 8.2 MB',speed:'—',remaining:'Complete',state:'done'}
];
let filter='all';
function paintRows(){
  const visible=rows.map((r,id)=>({...r,id})).filter(r=>(filter==='all'||r.state===filter)&&r.name.toLowerCase().includes($('#search').value.toLowerCase()));
  $('#transfer-rows').innerHTML=visible.map(r=>`<article class="transfer-row"><div class="transfer-file"><span class="file-icon"><svg aria-hidden="true"><use href="#i-file"/></svg></span><div><h3>${r.name}</h3><p>${r.host}</p></div></div><div class="progress-cell"><div class="progress-track"><span style="width:${r.progress}%;${r.state==='done'?'background:var(--success)':''}"></span></div><span class="progress-text">${r.size}</span></div><div class="rate">${r.speed}<small>${r.remaining}</small></div><button class="detail-button" data-detail="${r.id}" aria-label="View sample details for ${r.name}">View</button></article>`).join('');
  $('#empty-search').hidden=visible.length!==0;
  $$('[data-detail]').forEach(b=>b.onclick=()=>{const r=rows[+b.dataset.detail];openDialog(r.name,`<p>Illustrative transfer detail</p><table><tbody><tr><th scope="row">Progress</th><td>${r.progress}%</td></tr><tr><th scope="row">Transferred</th><td>${r.size}</td></tr><tr><th scope="row">Speed</th><td>${r.speed}</td></tr></tbody></table><p>The implemented detail panel will also contain connection contributions, file/part information, recovery actions, and the existing activity log.</p>`)});
}
$$('[data-filter]').forEach(b=>b.onclick=()=>{filter=b.dataset.filter;$$('[data-filter]').forEach(x=>x.classList.toggle('selected',x===b));paintRows()});
$('#search').oninput=paintRows;paintRows();
function openDialog(title,html){$('#dialog-title').textContent=title;$('#dialog-body').innerHTML=html;$('#dialog').showModal()}
$('#add').onclick=()=>openDialog('Add download','<p>Paste a link, magnet link, or several links on separate lines.</p><label for="sample-links">Links</label><textarea id="sample-links" placeholder="https://example.com/file.zip"></textarea><p>This is a visual reference. No download will start. The planned flow checks the link before offering Download now or Add to queue, with advanced options available on demand.</p>');
$('#chart-data').onclick=()=>openDialog('Sample traffic data','<p>Illustrative current readings; the chart is a static design sample.</p><table><thead><tr><th scope="col">Connection</th><th scope="col">Download</th></tr></thead><tbody><tr><th scope="row">Ethernet</th><td>84.2 Mbps</td></tr><tr><th scope="row">Wi-Fi</th><td>42.2 Mbps</td></tr><tr><th scope="row">Total</th><td>126.4 Mbps</td></tr></tbody></table>');
