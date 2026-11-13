# Handover: NetMerge for Windows

Written 2026-11-11 for the next person or session working on NetMerge, last updated 2026-11-13 (version 3.18.0). Read this first, then
`README.md` and the top of `CHANGELOG.md`. Facts here were true on that date; check the code when in doubt.

## 1. The owner and how to work with him

- The owner (Nisar) wants a very stable product and expects the engineer to decide and act, not to ask at every step:
  "do what you think is right". Ask only before something hard to undo.
- Be honest about what is verified and what is not. Say "tested on your PC", "tested in CI only" or "not tested".
- Plain words, no jargon. Short answers. He is not always at the PC.
- He gave full access to his PC for testing (see section 5) and does not mind test installs there.
- Every change goes through a branch and a pull request; CI must pass; then merge to main; then a release.

## 2. What NetMerge is

A single Windows program (Go, standard library only, MIT licence) that merges several internet connections (cable,
Wi-Fi, USB phone tethering) into one, for the whole PC. Its own network engine (`engine/`, built into the same program) captures the PC's traffic and sends each
**new connection** out on one of the lines. One single connection never gets faster than its line; many connections
(downloads in pieces, browsers, updates) do.

Not part of NetMerge any more:
- The download manager is now a separate program, **Pullwell** (`nisar87/pullwell`), built in another thread.
  NetMerge keeps only a note in Settings and `/api/legacy-downloads` (saves an old download list for Pullwell).
- An **Android** version is being explored in `nisar87/netmerge-android` (another thread). Code is copied, never shared.

## 3. Code map (repository root)

| File | What it does |
| --- | --- |
| `main.go` | Start-up, single instance (port 9196), admin relaunch, tray, crash log |
| `engine.go` | Starts/stops the engine (`NetMerge.exe -engine -d config`), re-attaches to a running one, restarts it after crashes |
| `engine_procs.go`, `procs_*.go` | Closes only NetMerge's own engine processes (`data/engine.pid` + program path); the engine is asked to stop via `POST /quit`. Removes the `core` folder of versions before 3.18 |
| `engine/` | The network engine, a separate Go module (standard library only). See `engine/README.md` |
| `engine_contract_test.go`, `engine_compat_test.go` | What the engine must do for NetMerge, and that every setting NetMerge writes is one the engine reads |
| `usage_offline.go` | Counts data used while NetMerge was closed (64-bit adapter counters, same Windows boot only) |
| `config.go` | Writes the engine's settings file: one proxy per line bound to its adapter, TUN, rules, bypass lists |
| `dispatch.go` | The heart: decides which line new connections use (smooth weighted slots, 8 slots, learned capacities with ~15-minute half-life, "slow" flags, call protection) |
| `ratecap.go` | Rejects impossible byte-counter readings (USB adapter counters restart; 32-bit wrap) and readings across a long gap (sleep) |
| `stats.go` | Samples adapter counters every second; per-line speed, usage, status shown in the window |
| `health.go` | Probes each line every 10 s; dead after 4 failures; captive portal detection |
| `usage.go` | Daily usage per line, data allowances (quota) with "leave" action; correction tool |
| `adapters.go`, `sys_windows.go` | Windows adapters, routes, shell, updater script, shortcuts, power |
| `api.go` | Local web API for the window. **Anything that changes state must be POST with header `X-NetMerge: 1`** (`action()` wrapper); GET is read-only. Fixed in 3.11.24 after a security audit. |
| `update.go` | Self-update from GitHub releases (`appVersion` here must match `VERSION`) |
| `speedtest.go`, `blocking.go`, `dns.go`, `share.go`, `preset.go`, `backup.go`, `history.go` | Speed test, ad blocking, DNS, sharing to other devices, presets, backup, history |
| `eventlog.go`, `report.go`, `diagnostics/` | The log beyond plain lines (problems with their place in the code, counts once a minute, how the last run ended, crash files) and the diagnostics reports sent to Firestore project `netmerge-windows`; `diagnostics/read.sh` reads them (see `diagnostics/README.md`) |
| `ui/` | The window (HTML/CSS/JS served by the program). 3.20.0 look: `styles/tokens.css` (Aurora palette, light and dark), `styles/aurora.css` (glass, light, controls, motion modes, loaded last), `features/quantum-flow.js` (the canvas animation on the Overview; presentation only) |
| `frontend/` | Tests for the window: a fixture server that imitates the API + Playwright browser tests; `scripts/perf.mjs` measures the window's own cost (frames, main-thread work, memory) |

## 4. Build, test, release

- Local: `go vet ./... && go test ./...` (also `GOOS=windows go vet ./...`). The window: `cd frontend && npm run
  test:unit && npm run test:fixtures` (Playwright e2e runs in CI).
- CI (`.github/workflows/ci.yml`) runs on pull requests: Go tests + Windows build, the window's Playwright suite
  (99 tests), the engine contract, the own engine's tests incl. a Linux capture test (root), and a **windows-latest**
  job: process/counter tests and the own engine capturing real traffic through Wintun (`engine/tun_e2e_windows_test.go`).
- `scripts/get-wintun.py` fetches wintun-0.14.1.zip (zip SHA-256 pinned), puts the DLL in `engine/wintun/amd64/`;
  the release passes the DLL's SHA-256 (`e5da8447…dafce`) to the engine with `-X netmerge/engine.wintunSHA256=`.
  This sandbox cannot reach wintun.net or proxy.golang.org; CI can.
- Release: change `VERSION` **and** `appVersion` in `update.go`, add a `CHANGELOG.md` entry at the top, push to main.
  `release.yml` builds `NetMerge.exe` + `.sha256` and publishes the release (6-10 minutes). A push without a
  `VERSION` change does not release.
- Commit messages and PR descriptions follow the attribution lines the session gives you.

## 5. Testing on the owner's real PC

- The PC: Windows 11, lines = **Ethernet** (Intel, about 100 Mbps cable; the ISP slows it after about 100 MB),
  **Wi-Fi** (TP-Link USB stick, 130 Mbps link), **Ethernet 6** (Samsung phone, USB tethering, mobile data,
  plugged in when testing). Gateway 192.168.1.1 for cable and Wi-Fi (different public IPs).
- A **test copy** of NetMerge runs from `E:\NetMerge\test-3.11.14\` (self-updates to each release). Its window is
  `http://127.0.0.1:9196`. After a release, update it with `POST /api/update {"action":"check"}` then
  `{"action":"install"}` (header `X-NetMerge: 1`), then check the version.
- Tools that worked: the built-in browser pane on the PC (tab on `127.0.0.1:9196`) for the API and the window;
  `E:\NetMerge` is a connected folder (read/write files). The Windows shell can only be clicked, not typed into.
  For commands, a small helper (`E:\NetMerge\tools\phone-runner.cmd`, double-click to start) runs `*.ps1` files
  dropped into `E:\NetMerge\tools\jobs` and writes the output to `jobs\done` (made for the Android tests; it can run
  any PowerShell test).
- Good measurements: many connections to neutral servers (e.g. `dl.google.com` Chrome .deb, ranges allowed).
  Not good: fast.com (few connections, ISP/Netflix limits), Cloudflare `__down` links and cloudflaremirrors.com
  (ignore ranges, one connection).

## 6. Verified on the real PC (so far)

Merging across 3 real lines; a line lost and rejoined (cable pull, USB unplug); self-update; "Show in folder";
counter restart after replugging a USB adapter (fixed 3.11.17-3.11.19); false usage across sleep (fixed 3.11.24);
"too slow" label after replug (fixed 3.11.23); GET requests from other websites cannot change anything (3.11.24);
the download manager removal (3.13.0); the window refreshes itself after an update (3.13.1, tested in CI; the
first real update after it will show it on the PC).

## 7. Open items

1. **IPv6 is not merged** (`"ipv6": false` in the engine config). If a line gives the PC IPv6, that traffic bypasses
   NetMerge. Not the case on the owner's PC; matters for other users. (The engine can capture IPv6; the app does not
   turn it on yet.)
2. **Since 3.18.0 the built-in engine is the only engine.** It ran on the owner's PC from 3.17.1 (both lines, a day
   of use) and is measured on every change by the "Engine benchmark" workflow. Not yet seen on the PC: sleep/wake
   and unplugging a line with this engine, a 24-hour run.
3. Not yet done: code signing (SignPath application pending; "Windows protected your PC" warning), a 24-hour run,
   running as a standard user, a trademark check of the name "NetMerge" (a similar `net-merge` project exists).

4. **Diagnostics (3.19.0).** Reports arrive in Firestore `netmerge-windows`. A daily scheduled task reads them with
   `diagnostics/read.sh`, fixes what they show and releases at most once a day.

## 8. Lessons

- Anything that counts bytes or learns speeds must reject impossible readings (counters restart and wrap).
- Test on the real PC before calling something fixed; the logs (`/api/log`, `/api/log?f=engine`) explain most problems.
- The merging logic (`dispatch.go`) took long to get stable: change it only with a test that shows the problem first.
