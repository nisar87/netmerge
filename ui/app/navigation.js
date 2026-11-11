const ROUTES = {
  overview:{page:'overview',group:'overview'},
  connections:{page:'connections',group:'connections'},
  'connections/speed-test':{page:'test',group:'connections'},
  'connections/presets':{page:'presets',group:'connections'},
  'apps/active':{page:'apps',group:'apps',view:'active'},
  'apps/rules':{page:'apps',group:'apps',view:'rules'},
  usage:{page:'usage',group:'usage',view:'summary'},
  'usage/allowances':{page:'usage',group:'usage',view:'allowances'},
  'protection/routing':{page:'rules',group:'rules'},
  'protection/blocking':{page:'block',group:'rules'},
  'protection/dns':{page:'dns',group:'rules'},
  'sharing/connect':{page:'share',group:'share',view:'connect'},
  'sharing/devices':{page:'share',group:'share',view:'devices'},
  'activity/events':{page:'history',group:'history'},
  'activity/logs':{page:'log',group:'history'},
  'settings/general':{page:'settings',group:'settings',view:'general'},
  'settings/appearance':{page:'settings',group:'settings',view:'appearance'},
  'settings/maintenance':{page:'settings',group:'settings',view:'maintenance'},
  'settings/advanced':{page:'settings',group:'settings',view:'advanced'}
};
const ALIASES={downloads:'settings/general',apps:'apps/active',rules:'protection/routing',block:'protection/blocking',dns:'protection/dns',share:'sharing/connect',history:'activity/events',log:'activity/logs',settings:'settings/general',test:'connections/speed-test',presets:'connections/presets'};
const TABS={
  connections:[['connections','Traffic mode'],['connections/speed-test','Speed test'],['connections/presets','Presets']],
  apps:[['apps/active','Active apps'],['apps/rules','Routing rules']],
  usage:[['usage','Usage summary'],['usage/allowances','Allowances']],
  rules:[['protection/routing','Routing exceptions'],['protection/blocking','Blocking'],['protection/dns','DNS']],
  share:[['sharing/connect','Connect a device'],['sharing/devices','Connected devices']],
  history:[['activity/events','Events'],['activity/logs','Logs']],
  settings:[['settings/general','General'],['settings/appearance','Appearance'],['settings/maintenance','Maintenance'],['settings/advanced','Advanced']]
};
let currentRoute='overview', routeView='';
$$('.nav').forEach(link=>{link.dataset.label=link.querySelector('.nav-label').textContent;link.setAttribute('aria-label',link.dataset.label);});
$('.skip-link').onclick=event=>{event.preventDefault();$('#main').focus();};
function go(destination,fromHash=false){
  let route=destination.replace(/^#\/?/,'');route=ALIASES[route]||route;
  if(route.startsWith('downloads/'))route='settings/general'; // the download manager moved to Pullwell: its note is in Settings
  if(!ROUTES[route])route='overview';
  if(!fromHash&&location.hash!=='#/'+route){location.hash='#/'+route;return;}
  const previous=currentRoute, config=ROUTES[route];
  currentRoute=route;page=config.page;routeView=config.view||'';
  NM.activeResource=({block:'blocking',share:'sharing',dns:'DNS',history:'activity'})[page]||page;
  $$('.nav').forEach(n=>{const selected=n.dataset.p===config.group;n.classList.toggle('sel',selected);if(selected)n.setAttribute('aria-current','page');else n.removeAttribute('aria-current');});
  $$('.page').forEach(section=>section.hidden=section.id!=='pg-'+page);
  const titles={rules:'Protection',share:'Sharing',history:'Activity'};
  $('#pageTitle').textContent=titles[config.group]||PAGES[config.group][0];
  $('#pageSub').textContent=PAGES[page][1];
  // The Windows tray finds our browser app window by its NetMerge title prefix.
  document.title='NetMerge · '+$('#pageTitle').textContent;
  const tabs=TABS[config.group]||[];
  $('#subnav').hidden=!tabs.length;
  setHTML($('#subnav'),tabs.map(([path,label])=>`<a href="#/${path}" ${path===route?'aria-current="page" class="selected"':''}>${label}</a>`).join(''));
  $$('#pg-'+page+' [data-view]').forEach(panel=>panel.hidden=panel.dataset.view!==routeView);
  $('#featureError').hidden=true;
  closeNavigation(false);
  if(previous!==route)$('#pageTitle').focus({preventScroll:true});
  if(page==='log')loadLog();
  if(page==='block')loadBlock();
  if(page==='usage')loadUsage();
  if(page==='share')loadShare();
  if(page==='presets')loadPresets().catch(()=>{});
  if(page==='history')loadHistoryEvents();
  if(page==='dns')loadDNS(false);
  if(S&&local)render();
}
function closeNavigation(restore=true){
  const wasOpen=document.body.classList.contains('nav-open');
  document.body.classList.remove('nav-open');$('#navScrim').hidden=true;$('#menuBtn').setAttribute('aria-expanded','false');
  $('#main').inert=false;
  if(wasOpen&&restore)$('#menuBtn').focus();
}
$('#menuBtn').onclick=()=>{
  const open=!document.body.classList.contains('nav-open');
  if(!open){closeNavigation();return;}
  document.body.classList.add('nav-open');$('#navScrim').hidden=false;$('#menuBtn').setAttribute('aria-expanded','true');$('#main').inert=true;
  $('#nav a[aria-current]')?.focus();
};
$('#navScrim').onclick=()=>closeNavigation();
document.addEventListener('keydown',event=>{
  if(!document.body.classList.contains('nav-open'))return;
  if(event.key==='Escape'){event.preventDefault();closeNavigation();}
  if(event.key==='Tab'){
    const items=[...$('#sidebar').querySelectorAll('a,button')],first=items[0],last=items.at(-1);
    if(event.shiftKey&&document.activeElement===first){event.preventDefault();last.focus();}
    else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first.focus();}
  }
});
matchMedia('(min-width: 720px)').addEventListener('change',event=>{if(event.matches)closeNavigation(false);});

// Section panels reuse their established controls and event identities.
const settingsCards=$$('#pg-settings > .card');
settingsCards[0].dataset.view='general';settingsCards[1].dataset.view='maintenance';
const appearance=document.createElement('section');appearance.className='card appearance-card';appearance.dataset.view='appearance';
appearance.innerHTML='<h2>Make yourself at home</h2><p class="hint2">Choose a comfortable workspace. System follows your device’s appearance.</p><div class="theme-previews" aria-hidden="true"><div class="theme-preview light"><i></i><div><b></b><span></span><span></span></div><small>Light</small></div><div class="theme-preview dark"><i></i><div><b></b><span></span><span></span></div><small>Dark</small></div></div>';
appearance.append($('#theme').closest('.opt'));$('#pg-settings').append(appearance);$('#theme button[data-v="auto"]').textContent='System';
const advanced=$('#pg-settings details.adv');advanced.dataset.view='advanced';advanced.open=true;advanced.querySelector('summary').textContent='Advanced settings';
$('#pg-settings > .hint2').remove();
const appRules=$('#pg-apps > .card:not(.page-hero)');appRules.dataset.view='rules';
const activeApps=document.createElement('section');activeApps.className='card';activeApps.dataset.view='active';
activeApps.append($('#pg-apps > .sec'),$('#appsTbl'));$('#pg-apps').append(activeApps);
const usage=$('#pg-usage');const quota=$('#usQuota').closest('.card');quota.dataset.view='allowances';
const usageSummary=document.createElement('section');usageSummary.dataset.view='summary';
[...usage.children].filter(el=>el!==quota).forEach(el=>usageSummary.append(el));usage.append(usageSummary);
$('#blkMine').setAttribute('aria-label','My block list');$('#blkAllow').setAttribute('aria-label','My allow list');
// Data tables keep the page responsive without reducing readable column widths.
['rulesTbl','appsTbl'].forEach(id=>{const table=$('#'+id),wrap=document.createElement('div');wrap.className='table-scroll';table.before(wrap);wrap.append(table);});
