# NetMerge Download Manager: IDM-style plan

Goal: an IDM-class download manager that is **off by default**, lives in its **own module** (`dm/`) that cannot
affect the NetMerge product (merging, dispatcher, usage, presets), and goes beyond IDM by using every connection
you have at the same time.

## 1. Rules of separation (hard requirements)

1. `dm/` is a separate Go package. It imports **nothing** from the product (package `main` cannot be imported, so
   this is enforced by the compiler). It talks to the product only through one small `Host` interface.
2. The product talks to `dm/` only through one file (`downloads_host.go`), every call wrapped in a recover.
3. **Off by default.** When off: no goroutines, no files read or written, no routes answering, no browser-extension
   access, no user-interface code loaded. A person who uses another download manager is never affected.
4. The user interface of the manager is a separate file (`ui/downloads.js`) loaded by the page only when the
   manager is on.
5. Panics inside the module are caught inside the module. A bug there can never take the product down.
6. The product's own network code (health checks, speed test, internet-address lookup) keeps its own copy of the
   connection helpers; it does not call into `dm/`.

## 2. IDM features compared with what we have

| IDM feature | Before | Plan |
| --- | --- | --- |
| Multi-part download, dynamic segmentation | pieces + endgame | smaller tail pieces, work stealing, faster ramp |
| Resume, crash recovery, auto retry | yes | keep, add per-download log |
| Connection reuse | per-line keep-alive | keep, tune |
| Slow connection replacement | stall watchdog only | replace connections far below their siblings |
| Speed limiter (global/per download) | yes | keep |
| Queues with schedule (start/stop time, days, max active) | one queue, start time | **several named queues**, each with schedule and limits |
| Scheduler actions (shutdown, sleep, hibernate, exit) | shutdown/sleep/hibernate | keep |
| Categories and folder per type | yes | editable folders per category, per-site folder rules, temp folder |
| If file exists: ask/rename/overwrite | rename | rename / overwrite / skip option |
| Batch download from pattern `file[001-100].jpg` | no | **add** |
| Add many links, import/export list | many links only | **add** export/import (text, JSON) |
| Site grabber (links of a page by file type) | no | **add** (simple grabber) |
| Mirrors (same file, several addresses) | no | **add** |
| Refresh expired link | no | **add** "change link" keeping the progress |
| Site authentication | no | **add** user/password for one download (HTTP Basic) |
| FTP | no | **add** (passive mode, resume, multi-connection) |
| HLS | yes (no separate audio) | **add** separate audio + DASH, joined with ffmpeg when present |
| Video grabber panel in the browser | no | **add** media detection in the extension |
| Browser integration (Chrome/Edge/Firefox) | yes | add file-type list, media panel |
| Download properties, segment map, log | partial | **add** properties, live segment map, event log |
| Search, filter by status/category, sort | no | **add** |
| Drag and drop links | no | **add** |
| Completion notification, open/locate | partly | **add** notification |
| Antivirus scan after download | no | **add** (optional command, off by default) |
| Torrent | no | **add** own BitTorrent client: .torrent and magnet, trackers (HTTP/UDP), DHT, ut_metadata, resume, per-line peers |
| Clipboard monitoring | no | not planned (needs a system hook); drag/paste instead |
| Proxy settings | n/a | not needed: NetMerge is the multi-line network |

## 3. Performance work (inspired by IDM)

Measured with a loopback server (no network limits) before and after:

- reusable buffers instead of per-read allocation (less garbage collection)
- one disk writer per download that coalesces writes, so network reads never wait for the disk
- faster ramp: start with the full per-line connection count when the server allows ranges
- smaller pieces near the end and work stealing, so the last 5% is not one slow connection
- replace connections that run far below their siblings (per-connection throttling by servers)
- first request reused as the first piece (saves one round trip on every download)

## 4. Phases (each: implement, test, fix, then next)

1. Separation + on/off switch + lazy user interface + panic isolation
2. Engine performance (buffers, disk writer, ramp, tail, slow-connection replacement), with benchmark
3. IDM core features (queues, categories, rules, temp folder, exists policy, auth, mirrors, change link,
   properties/log/segment map, search/filter, patterns, grabber, import/export, drag and drop, notifications,
   antivirus hook)
4. FTP
5. Streams: HLS audio, DASH, extension media panel
6. Torrent
7. Final audit, documentation, release

## 5. Honest limits

Everything is tested with simulated servers and peers on Linux. Real-world behaviour on Windows, real
trackers/peers and real streaming sites must be confirmed on the PC. Torrent: TCP only (no uTP), no protocol
encryption, so a minority of peers cannot be reached. DRM streams are never supported. Browser store listing
is not possible from here.

## 6. Results of phase 2 (engine speed)

| Test (loopback, simulated server) | Before | After |
| --- | --- | --- |
| 512 MB over 4 unthrottled lines | 549 MB/s | about 630 MB/s |
| 48 MB, server gives 2 MB/s per connection | 6.1 s | 3.2 s (best possible 3.0 s) |
| 96 MB, every 8th connection throttled to 100 KB/s | 41 s | 7 s |

Tried and dropped: smaller pieces and "slow lines do not take the last pieces" made no measurable difference; the
remaining ~0.2 s at the end of a download is a fixed cost, not a throughput loss.


## Phase 6 result (3.9.0): torrents

Implemented in `dm/bt_*.go` (bencode, .torrent/magnet, trackers, DHT, peer wire, ut_metadata, ut_pex, piece picking, storage, resume) and `dm/dl_torrent.go` (list integration). Tested against local seeders, an HTTP and a UDP tracker and a fake DHT node, including bad pieces, dropped peers, resume, partial file selection and magnet metadata. Loopback benchmark: 160 MB over two capped lines (20 + 10 MB/s) with eight 4 MB/s peers finished at 22 MB/s.

Not verified: real swarms, real trackers and the real DHT (no internet in the build sandbox), Windows behaviour, NAT-heavy peers. Not implemented: incoming connections, MSE encryption, uTP, seeding, web seeds, BitTorrent v2.

## Phase 5 result (3.10.0): streams

DASH (`.mpd`: segment templates by number and by timeline, segment lists, single-file tracks), separate sound tracks for HLS and DASH, joined to one `.mp4` with ffmpeg when it is found (PATH or next to NetMerge), otherwise two files and a note. The extension popup lists the videos and streams the page loaded, and has a list of file types that are always sent. Tested with local DASH/HLS sites and a stand-in ffmpeg.

Not verified: real streaming sites, real ffmpeg output, players' handling of the joined files. Not supported: live streams, DRM, DASH with several periods.

## Phase 7 result (3.10.1): final audit

- Full `go test -race ./...` passes; `GOOS=windows go vet ./...` is clean; the extension tests pass.
- The pages were driven in a real browser (Playwright) against the built program: switch off/on, add by pattern, queues, properties, link grabber, torrent magnet and .torrent file, a running download. This found two real bugs that Go tests could not (the segment bar crashed the page while a split download ran; starting a torrent froze the list). Both are fixed and covered by tests.
- Separation re-checked: `dm` imports nothing from the product (compiler-enforced), the product reaches it through `downloads_host.go` only, and with the manager off nothing runs, no routes answer and no page code loads.

## Left out on purpose

- **Temporary folder for unfinished files.** Unfinished files stay next to the final file, so finishing is an instant rename on the same disk. A separate temp folder would add a copy at the end and more places for leftovers.
- **Clipboard watching and hooking other programs' downloads** (IDM does this with a system hook). NetMerge uses paste, drag and drop and the browser extension.
- **Antivirus scanning built in.** There is a "run a program after each download" setting instead.
- **Torrent:** incoming connections, MSE encryption, uTP, seeding, web seeds, BitTorrent v2.
- **Signing and store listings** (program signing, browser stores): not possible from here.

## What is verified and what is not

Verified here (Linux, simulated servers, peers, trackers and a fake DHT node): the download engine, queues, folders, login, mirrors, FTP, HLS/DASH, torrents, resume after restart, panics isolated, off-by-default behaviour, the pages in a browser.

Not verified here: Windows behaviour (only cross-compiled and vetted), real swarms and trackers, the real DHT, real streaming sites, real ffmpeg, real multi-line PCs (Wi-Fi + mobile + LAN), the browser extension inside real Chrome, Edge and Firefox.
