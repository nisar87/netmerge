import {execFileSync} from 'node:child_process';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
const root=resolve(dirname(fileURLToPath(import.meta.url)),'../..');
const git=(...args)=>execFileSync('git',args,{cwd:root,encoding:'utf8'}).trim().split('\n').filter(Boolean);
const base=process.argv[2]||git('merge-base','HEAD','origin/main')[0];
const changed=[...new Set([...git('diff','--name-only',base),...git('ls-files','--others','--exclude-standard')])].filter(path=>!path.startsWith('.agents/'));
// The user additionally authorized persistent Windows EXE branding on 9 Nov 2026.
// Keep that packaging exception explicit; backend source remains outside scope.
const desktopBranding=new Set(['README.md','.github/workflows/ci.yml','.github/workflows/release.yml',
  'assets/windows/netmerge.ico','assets/windows/README.md','scripts/generate-windows-resources.sh',
  'scripts/check-windows-icon.py','netmerge_windows_amd64.syso','netmerge_windows_arm64.syso','netmerge_windows_386.syso']);
const allowed=path=>/^(ui\/|frontend\/|redesign\/)/.test(path)||/^extension\/src\/popup\.(html|css|js)$/.test(path)||desktopBranding.has(path);
const forbidden=changed.filter(path=>!allowed(path));
if(forbidden.length){process.stderr.write('Unexpected changes outside the frontend scope:\n'+forbidden.join('\n')+'\n');process.exitCode=1;}
else process.stdout.write(`${changed.length} changed files are inside the frontend/design scope or the explicitly authorized Windows icon packaging files. Backend source and extension core are unchanged.\n`);
