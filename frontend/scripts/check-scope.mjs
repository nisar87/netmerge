import {execFileSync} from 'node:child_process';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
const root=resolve(dirname(fileURLToPath(import.meta.url)),'../..');
const base=process.argv[2]||'39987fa48c25bf2eb29a700a44abf916174105bb';
const git=(...args)=>execFileSync('git',args,{cwd:root,encoding:'utf8'}).trim().split('\n').filter(Boolean);
const changed=[...new Set([...git('diff','--name-only',base),...git('ls-files','--others','--exclude-standard')])].filter(path=>!path.startsWith('.agents/'));
const allowed=path=>/^(ui\/|frontend\/|redesign\/)/.test(path)||/^extension\/src\/popup\.(html|css|js)$/.test(path);
const forbidden=changed.filter(path=>!allowed(path));
if(forbidden.length){process.stderr.write('Unexpected changes outside the frontend scope:\n'+forbidden.join('\n')+'\n');process.exitCode=1;}
else process.stdout.write(`${changed.length} changed files are inside the frontend/design scope. Backend, extension core and release configuration are unchanged.\n`);
