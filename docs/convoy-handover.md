# Convoy (working name): handover document for building the standalone download manager

Audience: an engineer or agent who has **not** seen the NetMerge project or its conversations. This document is meant
to be enough to start. Written 2026-11-10 against NetMerge **3.11.19** (repository `github.com/nisar87/netmerge`,
branch `main`, licence MIT). "The owner" is the project owner (GitHub `nisar87`); decisions marked **Owner** are his.

Related documents in the repository: `docs/standalone-download-manager-plan.md` (the short phased plan),
`docs/download-manager-plan.md` (the original feature plan), `docs/download-manager-audit.md` (what was audited and
fixed), `docs/download-manager-test-plan.md` (a full test plan written for an independent tester),
`docs/CODE_SIGNING_POLICY.md`.

---

## 1. What this is, in one page

NetMerge (the existing product) merges several internet connections of one Windows PC (wired, Wi-Fi, USB phone) into
one faster connection for the whole computer. Inside NetMerge there is also an optional download manager (package
`dm/`) that goes further: it sends the pieces of a download out of chosen connections itself.

**Decision (Owner, 2026-11-10):** the download manager becomes a **separate, standalone application** with nothing to
do with NetMerge. It is an ordinary download manager in the style of IDM (Internet Download Manager), Free Download
Manager or aria2: it opens many connections to the server; if NetMerge is running on the PC, NetMerge spreads those
connections over the lines by itself, and the download manager does not know or care. Without NetMerge it is simply a
good download manager on a single connection.

Consequences:

- The new program needs **no** line detection, line health, line choice, mobile-data protection or usage counting.
  Those belong to NetMerge. In the standalone program the manager sees exactly one "line": the computer's normal
  connection.
- NetMerge will lose its Downloads page, settings and extension pairing once the new program is released.
- The browser extension belongs only to the new program.
- Working name: **Convoy**. Not checked for conflicts (names like Haul, Pullr and GrabLine already exist as software).
  A name, trademark, GitHub, domain and browser-store check is a Phase 2 task. **Owner** decides the final name.

Product promise: "A download manager that is fast, never loses a file, and never gets in the way. Use it with NetMerge
and it also uses all your internet connections."

## 2. Working agreements with the owner

These come from the owner and apply to anyone working on this:

1. He wants a **fully stable** result. Do not ask him for decisions you can take yourself; take them, say what you
   decided.
2. **Be honest about what was and was not verified.** Say "tested on a real PC with three lines" or "only unit tests
   ran". Never report a pass you did not observe. A test that could not be run is BLOCKED with the reason.
3. Tell him when a task needs a stronger model or a real PC he must operate.
4. He is on a modest plan (Claude Pro, Sonnet-class model): keep steps small, run the tests after each step.
5. Open source, voluntary donations; he may sell later (then a paid certificate). Keep the licence and wording consistent.
6. The Windows test machine: a Windows Sandbox on his PC (isolated) and, with his permission, the PC itself. A second
   tester (OpenAI Codex, on his Windows PC) uses the test plan. Never start a second Windows Sandbox if one runs.
7. Each user-visible fix needs a line in `CHANGELOG.md` and, where possible, a test that fails without the fix.

## 3. What exists today (inventory)

All paths are in the NetMerge repository. The Go module is `netmerge` (Go 1.24). **The whole project, including `dm/`,
uses only the Go standard library: no third-party modules.** Keep it that way if you can (single `.exe`, builds
anywhere, nothing to fetch).

### 3.1 The `dm/` package (about 15,600 lines including tests)

It is a package of the same Go module, not a separate module (`dm/` has no `go.mod`). It imports nothing from the
product (package `main` cannot be imported, so the compiler enforces the separation). It talks to the product through
one struct, `Host` (`dm/host.go`), and the product talks to it only through `downloads_host.go`.

| File | What it does |
| --- | --- |
| `host.go` | `Host` interface, `Setup`, `Routes`, `Enabled/SetEnabled` (the on/off switch), `Shutdown`, the `/api/downloads/switch` route |
| `dl_manager.go` | The manager: items, queues, scheduling, saving `downloads.json`, config (`dlConfig`), pause/resume/remove, limits, power actions |
| `dl_engine.go` | The HTTP engine: probing, ranges, pieces, work stealing, end-game, stall watchdog, retries; defines `dlLine` and `dlEnv` |
| `dl_api.go` | The JSON API (`POST /api/downloads`, actions listed in section 3.3) |
| `dl_ext.go` | Browser extension server side: pairing, `/ext/*` routes, refusal of local-network targets |
| `dl_features.go` | Batch patterns `file[001-100].jpg`, link grabber, import/export, mirrors, relink, site rules |
| `dl_ftp.go` | FTP (passive, resume, several connections); no ftps |
| `dl_hls.go`, `dl_dash.go` | HLS and DASH streams, quality choice, separate audio joined with ffmpeg when present |
| `dl_torrent.go`, `bt_*.go` | Own BitTorrent client: .torrent and magnet, HTTP/UDP trackers, DHT, ut_metadata, resume; no incoming connections, no MSE, no uTP, no seeding after finish |
| `dl_util.go`, `atomic.go` | Helpers, atomic file writes |
| `dl_longpath_windows.go`, `dl_longpath_other.go` | Windows long path handling |
| `*_test.go` | Unit and integration tests (the full suite takes about 390 s) |

### 3.2 The `Host` interface (what the manager needs from its host)

Every field is optional. In the standalone program these must be implemented natively:

| Field | Purpose | Standalone notes |
| --- | --- | --- |
| `Root` | folder; the manager keeps its files in `Root/data` | choose the data folder (see 4.4) |
| `Version`, `Repo` | for the extension and update links | new repository name |
| `Lines` | the connections that exist | **return one line**: `Line{Name: "Computer", Kind: "ethernet", Dial: net.Dialer.DialContext}`; no binding to adapter addresses |
| `LineRate`, `CallActive`, `PresetDownloads` | NetMerge-specific | leave nil |
| `Guard` | protects the local API | keep a guard: only `Host: 127.0.0.1:<port>` / `localhost:<port>`, header `X-NetMerge: 1` on every POST (rename), `Origin` must be local |
| `Log`, `Note` | diagnostics, activity page | implement |
| `Notify` | desktop notification | see 4.3 (real toasts) |
| `Power`, `PowerCancel` | sleep/hibernate/shutdown/exit after downloads; shutdown has a one-minute warning that can be cancelled | implement with Windows APIs |
| `OpenPath` | open a file / show it in the file manager | Windows: `SHOpenFolderAndSelectItems` plus raising the window (see section 7, item 4) |
| `FreeSpace` | free bytes in a folder | implement |
| `MarkFromInternet` | the "downloaded from the internet" mark (Zone.Identifier) so SmartScreen checks the file | implement |
| `Run` | run a program with no window (used for the after-download command, e.g. a virus scanner) | implement |

The line type is small:

```go
type dlLine struct {
    Name  string
    Kind  string
    Rate  float64                                                    // bytes/s the line is known to carry; only used to order lines
    Dial  func(ctx context.Context, network, addr string) (net.Conn, error)
    Alive func() bool                                                // nil = always usable
}
```

Things in `dm/` that exist **only because of lines** and can be removed or hidden in the standalone program:
per-line limits (`LimitLines`), `ExcludeLines`, `AllowMobile`, `MaxPerLine` (becomes "connections per download"),
`CallMode` (keep the call's line free), per-line byte counts in the item view, the "lines" list in the API snapshot,
`PresetDownloads`.

### 3.3 The API (served by the host program on 127.0.0.1)

`POST /api/downloads` with JSON `{"action": ..., ...}` and header `X-NetMerge: 1`; `GET /api/downloads` returns
`{"items": [...], "config": {...}, "active", "speed", "lines", "knownSites", "ffmpeg", "hold", "powerAt", "ext"}`.
`POST /api/downloads/switch {"on": true|false}` turns the manager on or off (it is **off by default** in NetMerge; in
the standalone program it is simply on).

Actions: `add` (fields `url`/`urls`, `torrent`, `name`, `dir`, `conns`, `header`, `force`, queue, start-later ...),
`probe` (what is behind a link; used by the "add" dialog), `torrentInfo`, `files`, `select`, `pause`, `resume`,
`retry`, `remove` (`deleteFiles`), `clear`, `pauseAll`, `resumeAll`, `open`, `folder`, `move` (queue order), `limit`
(speed limit for one download), `relink`, `setMirrors`, `setQueue`, `setConns`, `export`, `import`, `grab` (link
grabber), `startQueue`, `forgetHosts`, `config`, `cancelPower`, `extCode`, `extRevoke`, `extConfig`.

Item states: `queued`, `downloading`, `paused`, `done`, `error`. An item has id, url, header, name, dir, path, category,
size, done, state, err, detail, log (event list), bytes per line, limit, retries, hash, and more (`dl_manager.go`
`dlItem`).

Config (`dlConfig`): download folder, categories on/off and folders per category, max active downloads, max per line,
global speed limit, quiet hours window, after-all action (sleep/shutdown), named queues with their own schedule, file-exists
policy (`rename` | `overwrite` | `skip`), per-site folder rules, notification switch, torrent switches and upload limit,
after-download command.

Data files, in `Root/data`: `downloads.json` (the list), `downloads-switch.json`, `extension.json` (paired
browsers), plus per-download `.nmpart` (data, preallocated) and `.nmdl` (state: URL, ETag, last-modified, piece
size, bytes per piece) next to the target file. Resume trusts the `.nmdl` file only if the server's ETag/size still
match.

### 3.4 The browser extension (`extension/`)

Manifest V3. Chrome/Edge/Brave package and a Firefox package, built by `python3 extension/build.py` into
`extension/dist/`. Files: `src/background.js` (service worker), `src/core.js` (decision logic `shouldCatch`, unit
tests in `core.test.js`), `src/popup.*`.

Behaviour: on `downloads.onCreated` the extension pauses the browser download, sends the link to the program
(`POST /ext/add` with the browser's cookies, referer, user agent), then cancels and erases the browser's own download;
if the program is unreachable, the browser download continues (fallback is tested). Default threshold: only files of
5 MB and larger; private windows are ignored; never takes over local/private hosts; ignore list per site; file-type
list; media sniffing (webRequest) for video/audio streams needs the optional "all sites" permission.

Pairing: the program shows a code like `ABCDE-FGHIJ` (`extCode`), valid a short time; the extension sends it to
`/ext/pair` and receives a token; all other `/ext/*` calls need the token. Routes: `/ext/ping`, `/ext/pair`,
`/ext/status`, `/ext/unpair`, `/ext/add`. Links from web pages are refused when they lead into the local network
(`dlRefuseLocalTarget`, which also resolves names).

Fixed port today: **9196** on 127.0.0.1 (NetMerge's own port). The standalone program needs its own port and a
single-instance guard.

### 3.5 The web interface (`ui/downloads.js`, `ui/downloads-more.js`, `ui/downloads.css`, about 650 lines)

Plain JavaScript, no framework, no build step, loaded by NetMerge's main page (`ui/index.html`) only when the manager
is on. It uses helpers defined by the main page: `$`, `api`, `esc`, `bytes`, `toast`, `setHTML`, `go`, and the page's
CSS variables. **The standalone program needs its own page shell that provides these helpers** (copy the small
helpers from `ui/index.html`; the NetMerge shell itself is about 1,600 lines, of which the downloads part is a small share).

### 3.6 Tests and test tools

- `go test ./dm` (about 390 s; the whole repository takes about the same). `go vet ./...`. A race run
  (`go test -race ./...`) was green on 3.11.9.
- `extension/core.test.js` (Node).
- `scripts/dmtest/fixtures.py`: a local test server on port 8765: ranges, flaky and slow servers, 403/503, redirects,
  no-range servers, HLS/DASH fixtures, `--ffmpeg`, `--out`.
- `scripts/dmtest/torrent_fixture.py`: a local seeder (uses the PC's LAN address; peers on 127.0.0.1 are refused by
  design).
- `scripts/dmtest/smoke.py`: quick end-to-end check of ordinary downloads through a running program.
- `docs/download-manager-test-plan.md`: about 280 lines, written for Codex, with the test table format.

## 4. Target design for the standalone program

### 4.1 Architecture

```
 Browser extension  --HTTP-->  Convoy (one Windows .exe, Go)
                                 |- download engine (dm/, copied over, lines reduced to one)
                                 |- local API + embedded web interface (127.0.0.1:<port>)
                                 |- native window (WebView2) + tray icon + toast notifications
                                 |- updater (GitHub Releases) + single-instance guard
```

If NetMerge runs on the same PC, it merges the connections underneath; Convoy dials normally.

### 4.2 What to keep, change, drop

Keep: the whole download engine, pieces and work stealing, resume format, queues and schedule, categories, batch
patterns, link grabber, import/export, mirrors, relink, FTP, HLS/DASH, torrent client, the extension, the web interface.

Change:
- `Host.Lines` returns the single normal line; remove the line UI (per-line share, per-line limits, mobile switch).
- "Connections per line" becomes "connections per download" (automatic by default, up to 32).
- Own port, own data folder, own `X-` header name, own extension name/icon, own update repository.
- Replace NetMerge's tray balloons with real toasts (4.3).
- A window host that does not depend on an installed Edge/Chrome (4.2.1).
- Remove "off by default": the manager is the program.

Drop: presets, call protection, line roles, usage counting, everything in `downloads_host.go` that is NetMerge-specific.

#### 4.2.1 Interface technology (decision taken, Owner delegated it)

Keep the **web interface served by the Go program**, shown in a native window.

- Why: it already exists and is tested; HTML/CSS/JS is the quickest to extend for both people and agents; no
  runtime or toolchain to fetch; the whole program stays one file.
- Window host: today NetMerge opens Edge or Chrome in "app" mode with its own profile (`--app=<url>`), which depends on
  that browser. For Convoy use Windows' built-in **WebView2** as the host (a thin native wrapper written in Go with
  syscalls, or a small vetted library if the "no dependencies" rule must be relaxed; if so, pin the version and vendor
  it). Keep Edge/Chrome app mode as a fallback, and the default browser as the last fallback.
  **Known hard part (the hardest engineering item of Phase 2):** WebView2's entry point lives in
  `WebView2Loader.dll`, which Windows does not ship. A wrapper without dependencies must either embed that DLL in the
  exe (extract it to the data folder at first start) or locate the WebView2 runtime itself (registry key of the
  runtime, then load its `EmbeddedBrowserWebView.dll`). The WebView2 runtime itself is present on current Windows 10/11
  and with Edge; if it is missing, fall back to Edge/Chrome app mode and say so. This is Windows COM work through
  `syscall`: give it to a stronger model, or accept one small pinned library, and test it on a real PC.
- Window behaviour to keep: single window, closing the window keeps the program in the tray, "Open" from the tray
  brings the existing window to the front.
- Rejected: Electron (about 100 MB, heavy), Tauri (needs Rust and Node), native toolkits (every UI change slower,
  everything already built would be thrown away).

### 4.3 Notifications

Requirements: finished, failed (with the reason in plain words), "needs attention" (login required, disk full),
"all done" with the chosen power action. Toasts should have buttons: Open, Show in folder.

- Phase 3: real Windows toast notifications through the system API (an unpackaged program needs an
  AppUserModelID registered through a Start-menu shortcut, so **toasts are for the installed version only**; the
  portable zip has no shortcut and uses tray balloons). **Do not use PowerShell to show toasts**: hidden PowerShell
  with bypass flags is exactly what makes antivirus products suspicious.
- Fallback: tray balloon (what NetMerge does today: `trayBalloon` in `sys_tray_windows.go`).
- A switch in settings (`noNotify` exists), and no notification storm when many items finish together (group them).

### 4.4 Files, folders, instances

- Installer (per-user, no administrator needed) and a portable zip. Data folder: `%LOCALAPPDATA%\Convoy\data`
  (installed) or `data` beside the exe (portable, as NetMerge does today).
- One instance per user: a second start brings the first window to the front. Own port: a **fixed default port plus a short
  list of fallback ports**. The extension must reach the program *before* it can pair, so it cannot be told a random
  port; it tries the default, then each fallback in order, and remembers the one that answered. The program takes the
  first free port of that list and shows it on the pairing screen. (NetMerge itself uses the fixed port 9196.)
- Updater: GitHub Releases (like NetMerge: download, verify the `.sha256`, replace and restart). A tested model
  exists in NetMerge `update.go`; it worked on a real PC on 2026-11-10 (3.11.14 to 3.11.19 by itself).
- "Start with Windows": a per-user Run key or a scheduled task; never create more than one; refuse from a copy that
  was started with a custom data folder (the lesson of NetMerge 3.11.12).

### 4.5 Reliability rules (non-negotiable)

1. A file is never reported finished unless its size matches and, where a checksum is known, the checksum matches.
2. Writes of state files are atomic (`atomic.go`); a crash at any moment leaves a resumable download.
3. Resume checks ETag/size/last-modified; a changed file restarts with a clear message, never a mixed file.
4. Never overwrite or delete a user file without the file-exists policy saying so; torrents always get a fresh name.
5. Impossible numbers are thrown away at the source (the NetMerge counter bug: an adapter byte counter that restarted
   was read as billions of bytes and corrupted speeds and usage). Apply the same thinking to progress and speed.
6. Every failure shows a plain-language reason and a next step, not a status code.
7. Panics inside the engine are caught inside it (`safeHandler`); a bug in one download must not stop the others.

### 4.6 Security rules

- The local API answers only local `Host` headers, requires the custom header on POST, and checks `Origin`.
- The extension needs a one-time pairing code; tokens can be revoked.
- Links from web pages never lead into the local network (names are resolved and checked).
- The after-download command runs without a shell and only with the file path as an argument.
- No telemetry. The program contacts the sites you download from and GitHub for updates, nothing else; say so in the
  README and the signing policy.

## 5. Phases (each ends with a gate)

The phase numbers match `docs/standalone-download-manager-plan.md`, which has the rationale.

**Phase 0: dependable where it is (inside NetMerge, owner: the NetMerge thread).** Refused-link investigation,
end-game and tail removal, two weeks of daily use, benchmark script. *Gate:* two weeks without data loss or corrupt
file; refused-link case understood; benchmark recorded.

**Phase 1: "normal connection" mode (in the NetMerge repository).** A host that offers one line (plain dialer).
Hide line-specific UI in this mode. Run the whole dm test suite in this mode. *Gate:* suite passes; a real-PC test
through NetMerge equals today's speed.

**Phase 2: separate the program (this is where the new thread starts).** Timing: the Phase 0 gate needs two weeks of
daily use, counted from 2026-11-10, so the build starts around 2026-11-24 unless the owner decides otherwise. The name
check below is the one Phase 2 task that can run before then.
1. Create the new repository (name after the name check) with licence, README, CI copied from NetMerge
   (`.github/workflows/release.yml`: a change of the `VERSION` file runs vet, tests, build, publish; the version in the
   code must equal `VERSION`; a `.sha256` is published next to the exe).
2. Copy `dm/`, `ui/downloads*`, `extension/`, `scripts/dmtest/`, the relevant docs and tests; write `main.go` and a
   host (`Host` with one line, native Windows implementations of the optional functions in 3.2).
3. Window host (4.2.1), tray, single instance, updater, installer and portable zip.
4. In NetMerge (its own thread): remove the downloads code, page and extension pairing; add a one-time import of
   `downloads.json` into the new program (optional). The import must also understand the old per-download file names
   (`.nmpart` data and `.nmdl` state): **keep reading and writing the same names** in the new program, or unfinished
   downloads would restart from zero (rule 4.5.3). Nothing about the file formats needs to change for the move.
*Gate:* both programs build and pass their tests; the new program works with NetMerge off and on.

**Phase 3: the browser side.** Polish, store listings (Chrome Web Store, Edge Add-ons, Firefox Add-ons), a "not
connected" state with automatic reconnect, real toasts (4.3). *Gate:* a new person installs the program and the
extension and catches a download in under five minutes.

**Phase 4: trust and quality.** Code signing, 24-hour soak, very large files, full disk, read-only folders, hundreds of
items, antivirus-locked files, long paths; benchmark against IDM and aria2 on one PC, published with the method.
*Gate:* no open data-loss bug.

**Phase 5: what makes people choose it.** IDM list import, a command line and local API for scripts, optional yt-dlp
use (installed on request, not bundled), checksum use (`.sha256`, `Digest` header), optional antivirus hook.

## 6. Product backlog, ranked by value for daily use

1. **Check the tail with a real benchmark.** In a test on a three-line PC, 16 *plain* connections (a browser, fixed-size
   connections) averaged 86 Mbps while the lines were busy at about 130 Mbps together: the slow line's last connections
   finished late while the fast line sat idle. The NetMerge engine already has end-game handling and work stealing, and
   its own 147 MB downloads showed no visible tail (13 MB/s until the end), so this item is "verify with the benchmark and
   fix only if a tail shows", not a known defect. Smaller pieces near the end and racing the last pieces are the tools.
2. **Look like a real browser: DONE for the known case (NetMerge 3.11.20).** `https://speed.cloudflare.com/__down?...`
   answered 403 to a request without a `Referer` and 200 with one (HEAD, `Range` and the user agent were not the
   cause; the server ignores `Range`). The engine now retries once with the site's own address as the referer when a
   server answers 403 and no referer was given (`dlProbeReferer` in `dm/dl_engine.go`, test
   `TestDLRefusedWithoutRefererRetriesWithSiteReferer`); a referer from the browser or the person is never replaced.
   Still open: cookies/login walls, and servers that check more than the referer.
3. **Capture that never gets in the way:** clear on/off state, easy per-site ignore, no double downloads.
4. **Honest progress and reasons:** speed, time left, and the plain-language reason when a download is stuck.
5. **Automatic connection count** per file and per site (some servers ban many connections; some throttle each
   connection so more connections help).
6. **IDM import** (its list export) and browser download history import.
7. Keep queue, schedule, speed limit and after-all actions simple.
8. Checksums when a site offers them, shown to the person.

## 7. Lessons learned in NetMerge (read before you repeat them)

1. **Impossible counter readings.** Windows 32-bit interface counters restart when a USB adapter is replugged; the
   difference read as about 2^32 bytes in one second. It inflated learned line speeds (a phone at "7 Gbit/s"), left
   other lines "too slow to help" and added 4.3 GB per direction to usage. Fixed in 3.11.17-3.11.18 (`ratecap.go`:
   reject readings above max(3 x link rate, 1 Gbit/s)). A saved file of learned values must be validated on load.
2. **Slow-line rule.** Adding download and upload capacity together misjudged a phone whose upload had never been
   seen; compare each direction (3.11.19).
3. **Loopback.** A socket bound to a line's address cannot connect to 127.x; loopback targets bypass binding (3.11.9).
4. **Show in folder / bringing a window to the front.** Windows refuses focus changes from a background program.
   `SHOpenFolderAndSelectItems` opens the window behind. What finally worked (confirmed on a real PC): find the
   Explorer window (class `CabinetWClass`; its title can be "Folder - File Explorer", so match the prefix), then
   `AttachThreadInput` to the foreground window's thread, tap Alt, `SetForegroundWindow`, and lift it with
   `SetWindowPos` topmost-then-not-topmost (`sys_windows.go`, `raiseExplorer`). Log the outcome; it made the bug
   diagnosable.
5. **Start-up task.** One scheduled task per computer; a test copy started with a custom data folder must not replace it.
6. **Self-update works** and is the fastest way to deploy to a real PC being tested. Releases take about 6-10 min
   (CI runs the full tests).
7. **Windows Defender / SmartScreen.** Hidden PowerShell with bypass flags, self-elevation, firewall rules and
   downloading an engine all raise suspicion. The standalone program needs none of these; keep it that way. Code
   signing: the owner applied to the SignPath Foundation (free for open source, review pending on 2026-11-10); the
   fallback is a paid certificate. Reputation with SmartScreen builds over time.
8. **Test the unpleasant physical cases on a real PC**: unplugging a cable mid-download found the counter bug.
   A simulated line failure did not.

## 8. How to test (minimum before any release)

1. `go vet ./... && go test ./...` (about 7 min). `go test -race ./...` before big changes to the engine.
2. `node extension/core.test.js` after any extension change.
3. `scripts/dmtest/fixtures.py` + `smoke.py` against the built program.
4. On a Windows PC: install the built program, pair the extension in Edge and Chrome, catch an ordinary download, a
   video, and a download while the program is closed (the browser must continue it); unplug and replug the
   network cable mid-download; pause/resume; close the program during a download and start it again.
5. Verify files by hash against the server or a second copy. For `.deb`/`.zip` files the archive's internal checks
   (`dpkg-deb`, `unzip -t`, `xz -t`) are a strong integrity test when no reference hash exists.
6. A benchmark that always reports the same four numbers: 1 line vs merged, 8/16/32 connections, with a note of the
   time of day (the owner's ISPs vary by 30-50% between runs).

Known limits that are **not bugs** (do not report): torrents have no incoming connections, no MSE encryption, no
uTP and no seeding after finish; no `ftps://`; no live or DRM streams; no multi-period DASH.

## 9. Open decisions

| Decision | Status |
| --- | --- |
| Final name | **Owner.** Working name: **Pullwell** (checked 2026-11-10: no software product of that name; Firefox Add-ons 0 results; Edge Add-ons only an unrelated fuzzy match; pullwell.app and pullwell.dev unregistered; pullwell.com parked; GitHub user "pullwell" is an empty test account, which does not block a `nisar87/pullwell` repository). Still to check: Chrome Web Store, `winget search pullwell`, pullwell.io, and a real trademark search before anything is public. Rejected: **Spillway** (several active GitHub projects, one a single-binary Go tool; spillway.app is a live product), **NDM** (the established name of Neat Download Manager, and it would tie the program to NetMerge), **Convoy** (`frain-dev/convoy`, `convoypanel/downloader`, trademark owner "Aegis Convoy Inc."), **Tributary** (a Windows media manager in winget), **Pullstream** (popular npm package), **Rivlet** (Product Hunt page), **Millrace**, **Headrace**, **Fetchwell** (existing software). "Convoy" remains the placeholder inside this document and the repository files until the name is confirmed. |
| Window host: own WebView2 wrapper or a vetted library | Decide at the start of Phase 2; default own wrapper, no dependency |
| Licence and donation wording | Suggested: MIT like NetMerge, voluntary donations |
| Code signing route | SignPath Foundation application pending; paid certificate as fallback |
| Whether the standalone program also offers a "line aware" mode | Default: no (NetMerge's job) |
| Chrome Web Store / Edge Add-ons developer accounts | **Owner** (needs his identity and a small fee for Chrome) |

## 10. Glossary

- **Line:** one internet connection (a network adapter). **Merge:** NetMerge combining lines for the whole PC.
- **Piece:** a byte range of a download handed to one connection. **Work stealing / end-game:** idle connections take over
  or duplicate the remaining pieces.
- **Tail:** the slow finish when the last pieces sit on a slow connection.
- **Dispatcher:** NetMerge's logic that assigns new connections to lines (whole connections, not pieces).
- **Pairing:** linking the browser extension to the program with a one-time code.
