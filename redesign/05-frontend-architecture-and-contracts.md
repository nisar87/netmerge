# Frontend architecture and frozen contracts

## Hard boundary

The backend is complete and out of scope. Read Go files to understand the interface; do not modify them to simplify the redesign. No API additions, altered JSON shapes, persistence changes, new engine decisions, or backend test rewrites.

Future implementation allowlist: `ui/**`; `extension/src/popup.html`, `popup.css`, and presentation-only changes to `popup.js`; self-contained development tooling/tests in `frontend/**`; design docs. Popup icon markup can be inline to avoid manifest/build changes. Keep `ui/qr.js` unchanged unless a separately justified frontend defect requires review. Freeze all `*.go`, `dm/**`, `go.mod`, extension `core.js`/`background.js`, manifests/build script, release workflows, `VERSION`, `CHANGELOG.md`, existing backend tests and binaries.

## Runtime structure

Use native ES modules, ordinary DOM APIs, semantic HTML, CSS custom properties and locally vendored SVG icons. No Node runtime in production, no remote font/icon imports, no framework migration. The current `//go:embed ui` and file server support assets in subdirectories; keep all shipped module filenames non-hidden. Test artifacts belong outside that embedded directory.

```text
ui/
  index.html                  shell and module entry
  styles/{tokens,base,shell,components,motion}.css
  app/{main,router,theme,api,store,polling,format}.js
  components/{icons,dialog,drawer,toast,field,status,chart,table}.js
  features/overview.js
  features/connections/{index,details,speed-test,presets}.js
  features/downloads/{index,transfers,add,details,queues,browser,settings}.js
  features/{apps,usage,protection,blocking,dns,sharing,activity,settings,setup}.js
  assets/icons.svg           selected licensed Lucide symbols
  assets/NOTICE.txt          asset license attribution
  qr.js                     retained existing encoder
frontend/
  package.json, package-lock.json, playwright.config.js
  tests/fixtures/            sanitized contract scenarios
  tests/contract/, tests/e2e/, tests/visual/
  scripts/serve.mjs          serves existing or redesigned ui with fixture API
  scripts/check-scope.mjs    baseline diff guard
```

This is a target, not scaffolding to generate before it is needed. Keep components small and colocate page behavior; do not build a general-purpose UI framework. Extract current helpers before replacing functionality. A temporary compatibility adapter may expose `go`, `api`, and shared format helpers to legacy download scripts until their migration is complete. Never have old and new polling loops active simultaneously.

## Frontend interfaces

```js
request(path, { body, signal } = {}) // Promise<JSON>; GET if body absent, POST otherwise
subscribe(topic, listener)          // unsubscribe(); listener receives immutable snapshot
publish(topic, snapshot)            // update a confirmed resource
commitSettings(edit)                // Promise<confirmed Settings>; edit(latest) returns complete Settings
startResource({key, interval, load, visible}) // disposer(); one request in flight
navigate(route, {replace=false}={}) // hash route and heading focus
openDialog({title, content, returnFocus}) // {close, element}
mountPage(root, context)            // returns {update, destroy}; no global timers
```

`request` retains `Content-Type: application/json` and `X-NetMerge: 1` on POST, same-origin relative `/api/` paths and normal browser Origin. It parses JSON errors; non-JSON/network/abort errors get actionable fallback text. Never loosen guards or add CORS. The fixture server may use another loopback port; **live backend remains localhost/127.0.0.1:9196**. Serve redesigned assets through the existing build or intercept requests in browser tests, not by editing server guards.

Separate confirmed snapshots, form drafts/dirty fields, UI-only selection, and pending operations. Serialize settings mutations; read fresh `/api/state` immediately before composing each complete settings payload; merge only that form's dirty fields, preserving all other fields, including each link's weight/role/quota and unrelated arrays. For link edits key by `name`, never index or display label. Re-fetch after success or ambiguous failure. A new API snapshot must not overwrite a dirty field. Backend has no revision/ETag contract, so this prevents frontend-created races but cannot promise atomic coordination with an external settings writer; do not introduce a backend change to solve that here.

Download config writes similarly send a full current config with the intended fields changed; do not omit queues/site rules/limits when saving one section. Inverse flags (`noCallProtect`, `noUpdateCheck`, `noNotify`, `noTorrents`, `noUpload`, `extOff`) need explicit mapping tests.

## Polling and lifecycle

Start with existing cadences: state 1 s; Downloads 1 s only enabled and visible; Blocking 3 s; Sharing 2.5 s; Usage 5 s; DNS 1.5 s; Events 5 s; Presets 8 s; Updates 4 s; history no faster than once per 55 s. Logs remain manual. On-demand fetch on tab entry. Offscreen feature resources stop; state can retain the existing hidden-window ~8 s cadence. Use recursive scheduling after completion, request deduplication, bounded GET backoff and a refreshed read on visibility return. Never faster than the existing request budget. Pending mutation results still reconcile even if a component unmounts. Discard superseded GET responses; abort stale reads, not dispatched mutations. Stop old timers on page disposal.

## Endpoint inventory

The body examples below describe the current contract, not a new REST design. Go handlers remain authoritative; fixtures capture exact representative response shapes before each feature is ported.

| Endpoint | Read/write and body | Frontend owner |
|---|---|---|
| `/api/state` | GET `{snap,settings,test,adapters,block,conflict,downloads,downloadsOn}` | Shared state, overview, connections, apps |
| `/api/power` | POST `{on:boolean}` | Global merge control/setup |
| `/api/settings` | POST **complete** allowed settings; response `{ok}` | Settings transaction layer |
| `/api/linkforget` | POST `{name}` | Missing connection details |
| `/api/rescan` | POST `{}` | Connections |
| `/api/speedtest` | POST `{}`; progress in `state.test` | Speed test |
| `/api/applyweights` | POST `{}` | Suggested shares |
| `/api/block` | GET or POST `{enabled}`, `{packs:{id:boolean}}`, `{blocklist:string}` or `{allowlist:string}` | Blocking |
| `/api/block/allow` | POST `{domain}` | Blocked domain action |
| `/api/block/update` | POST `{}`; asynchronous refresh | Filter packs |
| `/api/fixconflict` | POST `{}` after existing confirmation | Conflict banner |
| `/api/usage` | GET `{links,days,start,end,reset,action}` | Usage; quota edits use settings |
| `/api/share` | GET/POST `{enabled}` / `{newPass:true}` / `{name:{id,name}}` | Sharing; response contains sensitive credentials |
| `/api/presets` | GET/POST actions `apply(id)`, `restore`, `save(name)`, `delete(id)`, `schedule(schedule)` | Presets |
| `/api/dns` | GET/POST `test`, `reset`, `set` with `{provider,secure,custom,domains}` | DNS |
| `/api/events` | GET `{events}` | Timeline |
| `/api/history` | GET `{points}`; `t` unix seconds; `d/u` maps already Mbps | History chart |
| `/api/log?f=app|engine|rates` | GET `{text}` (last 300 lines) | Logs |
| `/api/update` | GET/POST `{action:'check'|'install'|'engine'}` | Update and engine-download states |
| `/api/backup` | GET file download | Backup |
| `/api/restore` | POST parsed backup JSON | Restore |
| `/api/diagnostics` | GET ZIP download | Diagnostics |
| `/api/quit` | POST `{}` | Explicit quit confirmation |
| `/api/ping` | GET `{pid}` | Existing liveness support |
| `/api/downloads/switch` | GET/POST `{on}` → `{enabled}` | Manager opt-in only |
| `/api/downloads` | GET snapshot; when off `{enabled:false}`, POST then 409 | Downloads module |

### Download action contract ledger

Every row uses POST `/api/downloads`. Preserve request field case, seconds/bytes units and response-specific handling.

| Action(s) | Important fields and response |
|---|---|
| `probe` | `url,header,height`; metadata/stream options; not a queued job |
| `add` | `url` or `urls`, `name,dir,lines,start,startAt,hash,force,limit,queue,mirrors,conns,height,user,pass,header,torrent,select`; added/skipped/duplicate may be present |
| `torrentInfo` / `files` / `select` | base64 `torrent` / `id` / `id,select:boolean[]` |
| `pause,resume,retry,open,folder` | `id`; preserve risky-open confirmation |
| `remove` | `id,deleteFiles`; unfinished partial data warning applies even if false |
| `clear,pauseAll,resumeAll` | No item ID; preserve manager semantics |
| `move` / `limit` | `id,move` / `id,limitBytes` |
| `setQueue,setConns,setMirrors,relink` | `id,queue` / `id,conns` / `id,urls` / `id,url,header` |
| `config` | Complete `config` object |
| `startQueue` / `cancelPower` / `forgetHosts` | `queue` / no extra / no extra |
| `grab` | `url,types`; links and metadata for selection |
| `export` / `import` | response `{file}` / `text`; retain format and partial-result semantics |
| `extCode` | response `code,expires,repo`; one-use expiry |
| `extRevoke` / `extConfig` | `id` / `extMinMB,extOff` |

Bulk actions remain a sequence of supported operations, with per-item failure counts. Do not invent an atomic batch endpoint. Read response fields rather than assuming every action returns a full snapshot; refetch as necessary.

## Domain invariants

- UI never computes which line the engine should select; display backend health/standby/dispatch/call decisions.
- Live rates are bytes/s; history values are Mbps; timestamps generally use Unix seconds, while speed-test finished time is a serialized Go time. Normalize at the adapter boundary and test explicitly.
- `healthy:null` is unknown. `enabled` plus `present` does not prove contributing; account for dead/standby/quota/slow and actual state.
- No speculative success after an accepted long-running request. Observe returned state for engine, downloads, tests, updates and filter refresh.
- No destructive automatic retry, automatic enabling, silent overwrite, fake undo for deletions, or unsupported scheduler controls.
- Treat names, URLs, domains, logs and release notes as text. Do not concatenate untrusted HTML. Validate navigation schemes; preserve existing safe external-link behavior.
- Keep QR/link generation and extension message/storage/permission semantics. No browser token or sharing password in localStorage, debug fixtures or logs.

## Migration approach

Develop behind a frontend-only preview entry (`ui/redesign.html`) if needed; embed works without a Go edit. The entry is temporary and removed before release. Build one shared state/API layer, then shell and overview, then migrate each feature behind it. A hash route may temporarily open an old view through a compatibility wrapper, visibly tracked in the parity checklist. Remove legacy polling only when its replacement is active; remove legacy files only after every feature has passed. Final entry remains `ui/index.html`, preserving the launch URL and packaging.
