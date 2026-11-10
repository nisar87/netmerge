# Plan: the download manager as its own application

Status: proposal, written 2026-11-10 after testing on a real PC with three internet connections
(wired 100 Mbps, USB Wi-Fi stick, USB phone). Nothing in this plan has been built yet.

## 1. The idea

NetMerge stays what it is: the layer that gives the whole computer one merged connection.
The download manager becomes a separate program that knows nothing about NetMerge, exactly like IDM
today. It opens many connections; when NetMerge is running, NetMerge spreads those connections over the
lines. Without NetMerge it is a normal download manager.

Why this is cleaner than today's design: the current download manager sends its pieces out of chosen lines
itself, so it carries its own line detection, line choice, mobile-data protection and usage counting. All of
that already exists in NetMerge. A plain download manager needs none of it.

## 2. Evidence from the test (2026-11-10, same PC, same hour)

| Method | Connections | Result |
| --- | --- | --- |
| NetMerge speed test (through the merge) | 16 | 94 Mbps (best step) |
| Plain parallel downloads through the merge (browser as a stand-in for IDM) | 8 | 78 Mbps average, about 135 Mbps while all lines were busy |
| same | 16 | 86 Mbps average, about 130 Mbps while all lines were busy |
| Current download manager, direct to the lines (147 MB file) | 8 per line | about 80 Mbps average |

Reading: the plain method is as good as the direct one. The average is well below the peak because of the
**tail**: connections that landed on the slow lines finish last while the fast line sits idle. A
download manager that hands out small pieces dynamically (the current one does) and races the last pieces
loses far less to the tail. This is the first improvement to build, whatever else is decided.

Other findings that shape the plan:

- fast.com is limited by the ISPs / Netflix servers per connection (23 Mbps without NetMerge, 28 with).
  It is a poor way to judge merging; many connections to neutral servers are the right test.
- A counter restart on an unplugged USB adapter once corrupted line speeds and usage. Fixed in 3.11.17-3.11.19.
  Lesson: anything that counts bytes or learns speeds must reject impossible readings.
- One test link (`speed.cloudflare.com/__down`) was refused by the current download manager ("the website refused
  the download") although it works in a browser. Sites that expect browser-like requests are the likeliest
  daily-use failure and must be understood before anything is split.

## 3. Phases

Each phase ends with a gate: do not start the next one until the gate is met.

### Phase 0 - make it dependable where it is (inside NetMerge)

- Done in 3.11.20: the Cloudflare link was refused because that server needs a `Referer`; the engine now retries
  with the site's own address as the referer (IDM does the same). HEAD, `Range` and the user agent were not the cause.
- End-game: the engine already races the last pieces; verify with the benchmark (a tail was seen only with plain
  fixed-size connections in a browser) and tune only if one shows.
- Pieces: smaller pieces near the end, connection count that grows while speed grows (start 8, up to 32).
- Daily use for two weeks by the owner, with a short log of every problem.
- A repeatable benchmark script (same file, merged vs single line, 8/16/32 connections) kept in `scripts/`.

Gate: two weeks without data loss or corrupt file; the refused-link case understood; benchmark numbers recorded.

### Phase 1 - a "normal connection" mode

- Today the download manager asks the host for a list of lines. Add a host that offers exactly one line:
  "the computer's normal connection" (no binding to an adapter address).
- Everything line-specific (choose lines, mobile protection, per-line limits) stays hidden in this mode.
- Tests: all existing tests run in this mode against the fixtures; plus a test through NetMerge on a real PC.

Gate: normal mode passes the full dm test suite; real-PC test through NetMerge equals today's speed.

### Phase 2 - separate the program

- New repository or module and name for the application: own `main`, own settings folder, own updater,
  own installer (and portable zip), own icon and window.
- Move the download code, the web interface and the extension into it. No shared code with NetMerge is
  needed in this design, apart from small utilities that are copied.
- NetMerge: remove the Downloads page, its settings and the extension pairing; keep a short note and a link.
  Provide a one-time import of an existing download list.
- Release both with their own change logs.

Gate: both programs build and pass their tests; NetMerge works with the downloads code removed; the new
program works with NetMerge off and on.

### Phase 3 - the browser side (this is most of the product)

- Extension for Chrome and Edge first, Firefox after: catching downloads, video detection, "download all
  links", context menu, per-site ignore list, private windows ignored.
- Pairing with the program stays; add a clear "not connected" state and automatic reconnect.
- Store listings (Chrome Web Store, Edge Add-ons, Firefox Add-ons) with a plain privacy statement.

Gate: a person who has never seen it can install the program and the extension and catch a download in
under five minutes.

### Phase 4 - trust and quality

- Code signing for the new program (a new SignPath application, or the same one if it allows it).
- A 24-hour soak test, very large files (tens of GB), full-disk and read-only-folder behaviour, hundreds of
  items in the list, antivirus-locked files, long paths.
- Benchmark against IDM and aria2 on the same PC, published with the method.

Gate: no open data-loss bug; numbers published.

### Phase 5 - what makes people choose it

- Import from IDM (its list export) and from browsers' download history.
- A command line and a small local API for scripts.
- Optional use of yt-dlp for video sites (installed on request, not bundled).
- Checksums: use a `.sha256` / `Digest` header when the site offers one and tell the person the result.
- Optional "scan with my antivirus when finished" hook.

## 4. Product ideas, ranked by value for daily use

1. Remove the tail (end-game pieces). Largest speed gain for the least work.
2. Browser behaviour that matches a real browser (cookies, referer, user agent, redirects). Fewest "refused" errors.
3. One-click capture that never gets in the way (extension polish, clear on/off state).
4. Crash-safe resume with a clear message when a server changes the file.
5. Automatic connection count per file and per site (some servers ban many connections).
6. Queue, schedule, speed limit and "when finished" actions - already there; keep them simple.
7. Honest progress: show per-file speed, time left, and which problem is blocking a stuck download in plain words.
8. IDM import: removes the biggest reason not to switch.

## 5. Risks

- Maintaining two programs alone. Mitigation: no shared code to keep in step; each has its own tests; shared
  lessons go in the change logs.
- A stronger model helps with Phase 2 (a large refactor). If the model in use struggles, do it in small
  steps with the tests running after each.
- The browser stores can reject or delay an extension. Mitigation: keep a manual-install route.
- The merge still decides the speed. If NetMerge's dispatcher has a problem, the download manager looks slow
  although it is fine. Keep the benchmark in both projects so the cause is clear.

## 6. Decision points for the owner

- Name of the new program.
- Whether to keep the download manager inside NetMerge during Phase 0-1 (recommended: yes, it is where it is tested).
- Licence and donation wording for the new program (suggested: same as NetMerge, voluntary donations).
