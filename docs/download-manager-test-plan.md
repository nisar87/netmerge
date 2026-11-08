# NetMerge Download Manager: Test Plan (for Codex)

Version under test: **3.11.1** (source `main`, the `VERSION` file must say 3.11.1). Audience: an engineer or agent who has not seen this code. Record every result in the table format of section 12.

This is revision 2. A first run (3.11.0, Windows, `download-manager-test-results.md`) covered part of the plan; section 0.5 says what is already proven, what was fixed, and what still has to be done. **Do not repeat what is proven, except the retests.** Spend the time on the BLOCKED items.

## 0. What the thing is

The download manager is a separate module (`dm/` package, UI in `ui/downloads*.js/css`, extension in `extension/`). It is **off by default**. When off, it must leave no trace: no goroutines, no files, no routes. When on, it downloads files, streams (HLS/DASH), FTP and torrents, spreading connections over the person's network "lines" (Wi-Fi, Ethernet, mobile).

Known limits (not bugs, do not report): torrents have no incoming connections, no encryption (MSE), no uTP, and no seeding after finish (upload happens only while downloading). Magnet links cannot be previewed before they start.

## 0.5 Status after the first run

Already PASSED on Windows (spot-check only if something looks off): C2 C3 C6 C7, D11 D18 D19 D20 D25 D27 D33 D34 D35 D37, F1 F2, S2 S3 S4 S5 S9 S13 S14, T1 T3 T4 T5, U1 U6 U7 U8 U16, X1 X2 X9. Fuzzing `FuzzBtParse` 5 minutes: clean.

Fixed in 3.11.1, **retest these first**:

| Retest | Steps | Expected |
|---|---|---|
| R1 (was BUG-01) | Add with options, 1 connection, "Add, start later", open Properties | Connections shows 1, not Automatic. Also `POST add` with `"conns":1`: the item has `conns:1` |
| R2 (was BUG-04) | Open "Add with options", press Shift+Tab 3 times, then Tab 40 times, then Enter | Focus never leaves the dialog; the page behind cannot be tabbed or clicked by keyboard; Esc closes the dialog and focus returns to the button that opened it |
| R3 (was BUG-05) | At 390x800 open every dialog (Add with options, Properties, Queues, Link grabber, torrent preview, Connect a browser) | For each: card `scrollWidth` equals `clientWidth` (no hidden overflow) and every control is reachable |
| R4 | Run `go test -run TestHLSRealFFmpegStreams -v ./dm` on the Windows machine with real ffmpeg on PATH | PASS for plain, aes and fmp4. If fmp4 still returns 404, attach the playlist and the folder listing; do not call it a product bug until a real fMP4 stream (S-section) also fails |

Not bugs (plan errors, corrected below): there is no "ask" file-exists policy (D28), and `move` only reorders the queue (D38).

## 1. Rules for the tester

1. Never mark a test PASS without observing the "Expected" result yourself. Write what you saw.
2. A test you could not run is **BLOCKED**, with a reason written for that test alone (not one sentence copied to many rows) and what you tried first. "No fixture" is not a reason any more: the fixtures are in `scripts/dmtest/`. Never guess a PASS.
3. Capture evidence: screenshot, log lines, API response, file hash.
4. Do not change product code while testing. File bugs instead (section 12). Exception: fixture and helper scripts of your own.
5. Run the destructive tests (delete, remove, overwrite) only in the test folder.
6. Use only content you are allowed to download. For torrents use a torrent you create yourself (T-section) or a legal public one (e.g. a Linux distribution ISO).
7. Test on every platform you can reach. State the platform in each result. The author could test only Linux with simulated lines, so **Windows is the highest priority** (section 3).

## 1.5 Rules for an isolated test run

- The app listens on a fixed port, **9196**. You cannot run a second instance beside the user's live NetMerge. Ask the user to close it, start yours with `-root <test folder>`, and tell the user to restart the real one afterwards. Never run restart/hard-kill tests against the live instance or the live data folder.
- The API only answers `Host: 127.0.0.1:9196` or `localhost:9196`. Any `POST` needs the header `X-NetMerge: 1`, and an `Origin` header (if sent) must start with `http://127.0.0.1:` or `http://localhost:`. Without that you get 403 (that is the protection, not a bug).
- `NM_STUB_LOOPBACK` and `NM_STUB_ADAPTERS` exist only in non-Windows builds. On Windows you test with the real adapters.
- Torrents: peers and trackers on 127.0.0.1 are refused in normal builds, by design. Use this PC's LAN address (see `torrent_fixture.py`). Torrents added by a web page (the extension) additionally refuse LAN addresses; add local-seeder torrents through the app's own page.

## 2. Setup

### 2.1 Build and start
```
cd netmerge
go build -o nm.exe .                  # Linux/mac: go build -o nm .
go vet ./... && GOOS=windows go vet ./...
go test -count=1 ./...                # on Windows a few tests are skipped by design (fake adapter numbers, /bin/sh ffmpeg stand-in)
go test -race -count=1 ./...          # needs a C compiler (install MinGW-w64/MSYS2 gcc) or use Linux; about 5 minutes; all "ok"
node --test extension/core.test.js    # pass 8, fail 0
mkdir C:\nmtest
nm.exe -root C:\nmtest -background    # UI: http://127.0.0.1:9196/ ; enable the manager in Settings
```

### 2.2 Ready-made fixtures (in the repo, `scripts/dmtest/`)
Use these instead of writing your own. Python 3.8+, standard library only.

1. `python scripts/dmtest/fixtures.py --out fx --ffmpeg`
   Serves port 8765 with deterministic content (byte i = i mod 251). Routes: `/file/5MB`, `/norange/..`, `/nolength/..`, `/flaky/5MB?drop=N`, `/status503/..`, `/status403/..`, `/redirect/..`, `/changing/..` (new ETag and content from the 2nd request), `/slow/..?bps=200000`, `/auth/..` (user/pass), `/disp?name=<tricky name>`, `/page` (links for the grabber), `/sha256/<name>` (expected hash), `/reset`. `--ffmpeg` also generates HLS (3 variants, AES, fMP4, live, separate audio) and DASH (video+audio, audio only) under `/static/hls/...` and `/static/dash/...`. Read the docstring at the top of the file.
2. `python scripts/dmtest/torrent_fixture.py --host <this PC's LAN IP> --out fx` makes a 3-file torrent (`fx/demo.torrent`, `fx/demo-magnet.txt`, `fx/demo-expected.json` with hashes), runs an HTTP tracker (6969) and a seeder (6881) and prints every tracker call (so you can see `started` / `stopped` / `completed`). Options: `--private`, `--corrupt-piece N`, `--throttle BPS`, `--upload-test` (the seeder also asks NetMerge for pieces NetMerge already has, over the connection NetMerge opened, and prints `CORRECT`, `WRONG` or `choked`). Run two copies on different ports/hosts for T13 (one corrupt, one good).
3. `python scripts/dmtest/smoke.py C:\nmtest\dl` runs the ordinary-download cases (file, flaky, 503, 403, redirect, no-range, slow) against the running app and checks every SHA-256. It was run on Linux: all passed.
Still to provide yourself: an FTP server (you already did), a mobile-hotspot line, a quota-limited disk or small VHD, a second machine for a real swarm, Chrome/Firefox.
Also for torrents on the real internet (legal content only, e.g. an Ubuntu ISO torrent): this tests real trackers, DHT, and real peers, which no local fixture can.

### 2.3 API helper
`GET /api/downloads` returns `{enabled, items, config, speed, ffmpeg, ...}`. `POST /api/downloads` takes `{"action": ...}` with the header from 1.5. Actions: `probe add torrentInfo files select pause resume retry remove clear pauseAll resumeAll open move limit setMirrors setQueue setConns export import grab startQueue forgetHosts config extCode extRevoke extConfig relink cancelPower`. `add` accepts `url`/`urls`, `torrent` (base64 .torrent), `select` (bool per file), `height`, `conns`, `queue`, `dir`, `start`, `force`, user/password fields. `probe` takes `url` and optional `height`. Use curl or Python for API tests and Playwright (or the in-app browser) for UI tests.

## 3. Platform priority list (Windows; none of these has been done yet)

| # | Test | Expected |
|---|------|----------|
| W1 | Start with the real lines, enable manager, download a 200 MB file | Speed is the sum of the lines; the Downloads page shows per-line use; file hash matches |
| W2 | Download a file to a path longer than 260 chars (torrent with deep folder names) | Succeeds (the `\\?\` prefix); no error about the path |
| W3 | Finish any download, right-click the file, Properties | The "This file came from another computer" Unblock box is shown (Mark of the Web) for ordinary files and torrent files |
| W4 | Antivirus/SmartScreen: open a downloaded .exe | The usual SmartScreen warning appears (proves the mark is set) |
| W5 | "Show in folder", "Open", "Delete file" on Windows | Explorer opens with the file selected; default app opens; file goes away |
| W6 | After-download actions: shutdown/sleep (use "cancel power" within the grace period) | Countdown appears; cancel works; no real shutdown during test |
| W7 | Mobile-hotspot line connected | Torrent upload never leaves through it (see T14) |
| W8 | Run on a clean user profile, no admin | Works without admin rights |

## 4. Core module behaviour (switch, isolation)

| # | Steps | Expected |
|---|-------|----------|
| C1 | Fresh root, never enabled: `GET /api/downloads` | `{"enabled":false}` |
| C2 | While off: `POST /api/downloads` add | HTTP 409 |
| C3 | While off: request any `/ext/*` | 404 |
| C4 | While off: look at process (goroutine count via `/debug` if available, or thread count) and the data folder | No download workers, no `downloads*.json` state, no `.nmdl` files; `downloads-switch.json` is absent or says off |
| C5 | While off: open the app UI | The Downloads page is not loaded (network tab shows no `downloads.js/.css`); Downloads nav item is not active |
| C6 | Settings: turn the switch on | Page loads module; `GET` returns `enabled:true`; nav item appears |
| C7 | Turn it off while a download runs | Downloads stop cleanly; no crash; data stays; turn on again, items still listed and resumable |
| C8 | Restart the app with the switch on | Manager comes back on; unfinished items are paused/resumable, finished remain |
| C9 | Kill the app hard (kill -9) in the middle of a download, restart | Item resumes from saved parts; final file hash correct |
| C10 | Provoke a panic path if you can (malformed request body, giant JSON) | Server stays up; error returned; other downloads unaffected |

## 5. Ordinary file downloads (HTTP/HTTPS)

For every case verify: final file size and **SHA-256 equals the source**; state ends as "Finished"; temp files (`.nmdl`, `.nmpart`) are gone afterwards.

| # | Case | Expected |
|---|------|----------|
| D1 | 1 KB file | Finishes instantly, correct name and hash |
| D2 | 5 MB file with range support, default settings | Split into parts, uses both/all lines (segment bar shows several parts), hash OK |
| D3 | 200 MB file | Speed near the sum of the lines (loopback: just fast); memory stays small (check RSS, must not grow to file size) |
| D4 | Server with no Range support | Single stream, completes, no corruption; UI does not claim "resumable" |
| D5 | Chunked response with no Content-Length | Completes; progress shows unknown total; name from header/URL |
| D6 | Pause then resume mid-download | Resumes at the same byte, not from 0 (check server logs for Range starting at saved offset); hash OK |
| D7 | Pause, restart the app, resume | Same as D6 |
| D8 | Server drops the connection mid-way | Auto retry with back-off; completes; hash OK |
| D9 | 503, 503, 200 | Retries; completes |
| D10 | 403 on first request, 200 afterwards with a fresh request | Handled according to retry rules; if it fails the error text is human readable and **Retry** works |
| D11 | 404 | Fails fast with "not found" style message; no retry storm |
| D12 | Redirect chain (3 hops) | Follows; final name correct |
| D13 | Content-Disposition with unicode / quotes / `../` in filename | Name is sanitised; file is created only inside the download folder (path traversal is impossible) |
| D14 | Reserved Windows names (`CON`, `NUL.txt`), trailing dot/space, 300-char name | Safe name chosen; no failure |
| D15 | File changes on the server between pause and resume (new ETag) | Detected; the download restarts cleanly or asks; never produces a mixed-up file (hash must match one of the two versions) |
| D16 | Basic-auth URL `http://user:pass@host/file` and separate user/password fields | Downloads; the password is not shown in the list or logs in clear text |
| D17 | Custom headers / referrer / cookies (from the extension) | Sent to the server (check server log) |
| D18 | Add duplicate URL | Warns about duplicate; "force" adds anyway |
| D19 | Add 5 URLs at once (one per line) | 5 items created |
| D20 | URL pattern `pic[01-03].bin`, `[a-f]`, `[1-100:5]` | Expands to the right number of links (3, 6, 20) |
| D21 | Disk full (small tmpfs/quota) | Item fails with a clear "no space" message, partial files cleaned up per setting, app remains responsive |
| D22 | Download folder missing or read-only | Clear error; no crash |
| D23 | Mirrors: give 2 mirrors, kill the first server mid-download | Switches to the other mirror; hash OK |
| D24 | Per-download connections (`setConns`) 1, 4, 16 | Server sees that many concurrent connections per line at most |
| D25 | Speed limit per download and global limit (`limit`, "limitAll") | Measured speed stays within about 10% of the limit; removing the limit restores speed |
| D26 | Scheduler / queue: create queue "Night" with a start window, add item | Item waits until the window; "start queue now" runs it; outside the window nothing starts |
| D27 | Max simultaneous downloads = 2, add 5 | Never more than 2 active; others "Waiting"; each starts as one finishes |
| D28 | File-exists rule: rename / overwrite / skip (settings `onExists`; there is no "ask") with an existing file | rename gives `name (1).ext`; overwrite replaces; skip does nothing and says why; an unknown value falls back to rename |
| D29 | Folder rules by file type (categories) | Images go to Pictures folder etc. per config |
| D30 | After-download command (`afterCmd`) | Runs once per finished file with the path; a failing command does not mark the download failed |
| D31 | Virus-scan hook if configured | Runs; result shown |
| D32 | Retry button on a failed download | Restarts, keeps already downloaded parts when valid |
| D33 | Remove from list (finished) | Item gone, file stays |
| D34 | Remove (unfinished) | Item and partial files gone |
| D35 | "Delete file" on a finished item | File removed from disk and item removed |
| D36 | Clear finished | Only finished items disappear |
| D37 | Export then import the list | Same items return, no duplicates |
| D38 | Reorder with `move` (`up`/`down`/`top`/`bottom` in the queue) | Order changes in the list and persists after restart. (`move` does not move files to another folder; the manager has no such feature.) |
| D39 | Properties dialog | Shows URL, size, folder, dates, headers; edits (queue, conns, mirrors, link) save and apply |
| D40 | Change the link of a broken download (`relink`) | Continues with new link if size matches |

## 6. FTP

| # | Case | Expected |
|---|------|----------|
| F1 | Anonymous FTP file | Downloads, hash OK |
| F2 | FTP with login | Works; wrong password gives a clear error, no retry loop |
| F3 | FTP resume (REST) after pause | Continues from offset |
| F4 | FTP server drops connection | Retries, completes |
| F5 | FTP passive port blocked | Clear error |

## 7. Streams (HLS / DASH)

Use `probe` first, then `add`. ffmpeg present unless stated.

| # | Case | Expected |
|---|------|----------|
| S1 | Probe an HLS master URL | Reports stream kind, duration, a list of heights `[1080,720,360]`, whether the sound is separate, whether ffmpeg is available |
| S2 | Add with no quality | Picks the best (1080p) |
| S3 | Add with `height:720` | Picks 720p |
| S4 | Add with `height:100` (lower than any) | Picks the smallest (360p) |
| S5 | UI: add dialog shows a "Picture quality" dropdown only when there are 2+ heights; changing it re-probes and the label updates | As described; selected height is sent on Add |
| S6 | HLS with separate audio + ffmpeg | One playable file with picture and sound (verify with `ffprobe`: 1 video + 1 audio stream) |
| S7 | Same, **no ffmpeg** | Two files saved (video and sound) with a clear note; settings page shows "ffmpeg not installed" |
| S8 | AES-128 encrypted HLS | Decrypts; output plays |
| S9 | Live HLS (no ENDLIST) | Refuses or records with a clear message; never hangs forever silently |
| S10 | Segment missing (404) in the middle | Retries then fails with a clear message, or skips per setting; no corrupt-but-"finished" file |
| S11 | Pause/resume a stream | Continues from the next missing segment, not from 0 |
| S12 | DASH video+audio with ffmpeg | One file with both |
| S13 | DASH audio-only | Output ends `.m4a` (not `.mp4`) |
| S14 | Delete a finished stream with "delete file" | Main file **and** the separate sound file (`.audio.aac`/`.m4a`) are removed |
| S15 | Stream through the extension's media sniffer | Media list in the popup shows the stream; clicking sends it to the manager |

## 8. Torrents

How: start `torrent_fixture.py --host <LAN IP> --out fx` (add `--upload-test` for T14/T15, `--private` for T20, `--corrupt-piece 5` plus a second good seeder for T13, `--throttle 200000` when you need time to pause/kill). Add `fx/demo.torrent` through the app's file chooser, or paste the line from `fx/demo-magnet.txt`. Compare file hashes with `fx/demo-expected.json`. Read the tracker lines in the fixture's console for T21. This was verified on Linux: download complete with correct hashes, tracker `started`/`completed`, and NetMerge uploaded a correct piece to the fixture while downloading.

The seeder must be on this PC's LAN address or another machine (loopback peers are refused in normal builds). State which you used.

| # | Case | Expected |
|---|------|----------|
| T1 | Add a `.torrent` through the UI file picker | A **preview dialog** opens: name, size, file count, per-file checkboxes, "All files", running total, folder box, duplicate warning, buttons Cancel / Add start later / Download now |
| T2 | Untick one file, "Download now" | Only chosen files download; the unchosen file is **never created** (not even as empty file); content hash of chosen files OK |
| T3 | Untick everything | Refused: "Choose at least one file" |
| T4 | Add the same torrent again | Preview says "already in your list"; adding anyway needs force |
| T5 | Junk file instead of a `.torrent` | Clear error, nothing added |
| T6 | Magnet link pasted in the Add box | Starts; fetches metadata from peers; then shows file list in Properties; downloads |
| T7 | Magnet with trackers only vs DHT only | Both find peers (DHT test needs a reachable DHT; mark BLOCKED if not) |
| T8 | Completed torrent folder | Files in `Folder/Name/...` with correct tree; hashes match; "Finished" |
| T9 | **Existing folder with same name**, rule "overwrite" | Torrent goes to a new renamed folder; the old folder is untouched |
| T10 | Pause / resume mid-way | Resume skips verified pieces (peers see no repeated requests for done pieces) |
| T11 | Quit app mid-way, restart, resume | Resumes from `.nmbt` state |
| T12 | While paused, edit a downloaded file (change a byte) and delete another, then resume | Changed/missing pieces are noticed and re-downloaded; final hashes OK |
| T13 | Corrupt piece from a malicious seeder | Piece rejected by SHA-1 check; peer banned after 3 bad pieces; torrent still completes from a good seeder |
| T14 | Upload: connect a test leecher while downloading | With sharing on, the leecher gets pieces we already have; **never** pieces of unchosen files; **never** through a mobile line |
| T15 | Settings: switch "Share while downloading" off | No unchoke/upload at all (leecher gets nothing) |
| T16 | Settings: upload limit e.g. 100 KB/s | Measured upload within about 10% of limit |
| T17 | Settings: torrents off | Magnet and `.torrent` adds are refused with "Torrents are switched off in the download settings."; ordinary links still work; the Upload controls hide |
| T18 | Rows and Properties for a torrent | Row shows peers/seeds and `↑ uploaded`; Properties shows Uploaded, ratio, peers connected/seeding, piece size, tracker list (max 6 + "and N more"), private flag, file list with checkboxes |
| T19 | Properties: change file selection of a running torrent | Torrent restarts with the new selection; unselected files' data dropped |
| T20 | Private torrent | Only its own tracker used: no DHT, no PEX |
| T21 | Pause or remove | Tracker receives `event=stopped` (check tracker log) |
| T22 | Remove torrent with "delete files" | Removes only that torrent's files and empty dirs; **never** a user's other files in the same folder (put an unrelated file in the folder to prove it) |
| T23 | Tracker down / all peers gone | UI shows "no peers" status, no CPU spin, no crash |
| T24 | Run a torrent for 10 minutes | Goroutine count and memory flat after it ends (no leaks) |
| T25 | Torrent files get the Windows "from the internet" mark | See W3 |
| T26 | A link grabber on the test page with magnet + `.torrent` links | Both listed under type "Torrents"; the magnet's name comes from `dn` |

## 9. Web UI

Run at 1200x900 and 390x800 (phone). No JS errors in console, no horizontal scroll at phone width.

| # | Case | Expected |
|---|------|----------|
| U1 | Nav, empty state | Friendly empty state with add box and tips |
| U2 | Add box, Add / start later / options dialog | Each behaves as labelled |
| U3 | Filters (All / In progress / Finished / Needs attention), type filter, search, sort | List changes accordingly; counts correct |
| U4 | Row actions: pause, resume, open, show in folder, remove, delete file, retry, properties | Each works; failure shows a toast with the reason |
| U5 | Bulk: tick rows, bar shows "N selected" | Pause / Resume / Remove / Move to queue apply to all selected; Remove asks first and says what will be lost; "Select all shown" and "Clear selection" work |
| U6 | Ctrl+A on the page | Selects all shown rows (not when typing in a field) |
| U7 | Delete key with a selection | Opens the remove question; Esc clears the selection |
| U8 | Paste a link (Ctrl+V) while focus is not in a field | Add box is filled and a "Link pasted" toast appears; **does not auto-start** |
| U9 | Speed graph | Appears next to the total speed once running for a few seconds; shows peak |
| U10 | Settings page, Downloads block | Switch, folder, limits, torrents block, ffmpeg line, file-exists rule, after-download command all save and survive reload |
| U11 | Queues dialog | Create, edit, delete queues; start now |
| U12 | Link grabber dialog | Finds links on a page, filters by type, adds selected |
| U13 | Keyboard only | Tab order logical; dialogs closable with Esc; focus trapped in dialogs |
| U14 | Dark/light mode | Readable |
| U15 | Language/long names/RTL names | No layout break |
| U16 | 2000 items in the list | Page stays responsive (scroll, filter under 1 s) |

## 10. Browser extension

Install the unpacked extension (Chrome and Firefox). Pair via the code from the app (`extCode`).

| # | Case | Expected |
|---|------|----------|
| E1 | Pairing with the code | Extension shows connected; revoking (`extRevoke`) disconnects it |
| E2 | Click a download link in a page | Extension offers/takes it over per setting; manager receives URL, referrer, cookies, headers |
| E3 | Right-click link, "Download with NetMerge" | Added |
| E4 | Right-click a magnet link | Added as torrent |
| E5 | Media sniffer: open a page with an HLS/MP4 stream | Popup list shows it; click sends it |
| E6 | **Private/incognito window** | Media is **not** recorded or listed; downloads from incognito are not taken over unless the person allows |
| E7 | Manager switched off | Extension does nothing and shows "off"; the browser's own download works normally |
| E8 | Manager app not running | Extension falls back to the browser download; no lost download |
| E9 | Exclusion list (file types, sites, min size) | Honoured |
| E10 | Permissions in the manifest | Only what is documented (webRequest etc.); no `<all_urls>` scripts injected in pages beyond what is documented |

## 11. Security and robustness

| # | Case | Expected |
|---|------|----------|
| X1 | API call from another origin / wrong Host header / DNS-rebinding style (`Host: evil.com`) | Rejected |
| X2 | `/ext/*` without a valid token | Rejected (401/403) |
| X3 | Path traversal in filename, in `move` target, in torrent file paths (`../../x`, absolute paths, `C:\`) | Never writes outside the download folder |
| X4 | Torrent with duplicate file paths / path collisions / 10 000 files / huge piece count | Refused or handled; no crash, bounded memory |
| X5 | Hostile peer: garbage, giant length prefixes, wrong infohash, slow-loris | Disconnected; no crash; goroutines return to baseline |
| X6 | Hostile tracker response (huge, malformed bencode) | Ignored safely |
| X7 | SSRF: add `http://127.0.0.1:9196/...`, `file:///etc/passwd`, `ftp://` to private ranges | `file:` refused; loopback/private targets handled per documented policy (note what happens) |
| X8 | Giant JSON body / 100 000 URLs in one add | Rejected with limit message |
| X9 | Credentials in URLs | Not logged; not exported in clear in the exported list unless asked |
| X10 | `go test -race -count=1 ./...` and the fuzz target `FuzzBtParse` for 5 minutes | No failures, no crashes |
| X11 | Run 24 hours with 20 queued downloads and a flaky server | No growth in memory/handles; app responsive |

## 12. Reporting

Fill one row per test; keep it in `docs/download-manager-test-results.md`.

```
| ID | Platform | Result (PASS/FAIL/BLOCKED) | What I did and saw | Evidence (file/log/screenshot) |
```
For each FAIL write a bug: ID, steps to reproduce, expected, actual, platform, version (`VERSION` file), logs, and severity: **Critical** (data loss, crash, security), **High** (feature does not work), **Medium** (works with a workaround or wrong message), **Low** (cosmetic).

End with a summary: counts of PASS/FAIL/BLOCKED per section, the list of all FAIL/BLOCKED with the reason, and a clear statement of what platforms and conditions were **not** covered.

## 13. Exit criteria

Release is acceptable when: all Critical and High tests pass on Windows; sections 4 (switch/isolation) and 11 (security) have no FAIL; every remaining FAIL has a written bug; every BLOCKED has a stated reason and a plan.
