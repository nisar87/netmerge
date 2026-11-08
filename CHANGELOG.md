# Changelog

## 3.11.3

- "Connect a browser" now explains everything in place: download buttons for the Chrome/Edge/Brave and Firefox extension files (always the latest release), the steps to add the extension by hand, a button that copies `chrome://extensions`, and then the pairing code. Before, it only said to look in the README.

## 3.11.2

Fixes from the first outside test on Windows (the 3.11.1 build was never published; its changes are included here):
- "Add with options": the connection count you chose was ignored (it was saved as Automatic). Fixed, with a test.
- Dialogs: the Tab key could leave a dialog and press buttons on the page behind it. The page behind is now locked while a dialog is open, Tab stays inside, Esc closes, and focus returns to where you were.
- Dialogs on a phone-width screen no longer clip the password box or the connection list.
- When NetMerge restarts itself as administrator, a folder with spaces in its path (for example `-root "C:\Users\Jo Smith\x"`) was cut at the first space, so it used the wrong folder and could not install the network engine ("cannot find the path"). The path now stays in one piece, and the engine installer reports exactly which folder it could not create.
- Test fixes (no change for users): tests that fake network adapters or use a Unix shell stand-in for ffmpeg are skipped on Windows; the fake-ffmpeg HLS test now runs ffmpeg in the output folder; the speed test for 5000 items uses the best of ten runs.

## 3.11.0

A full audit of the download manager (see `docs/download-manager-audit.md`), and everything on its list is done.

Fixes:
- A torrent never reuses an existing folder when "replace" is the file-exists rule.
- Files you did not choose in a torrent are never created (no empty files).
- Resuming a torrent notices files that were changed or deleted in the meantime and re-checks them.
- The torrent file list is no longer sent with every refresh, only when you open Properties.
- Long file paths work on Windows. Torrent files get the "downloaded from the internet" mark.
- Trackers are told when a torrent is paused or removed.
- Audio-only streams are saved as .m4a; deleting a finished stream also deletes its separate sound file.
- The browser extension does not list media from private windows.

New:
- Torrent settings: a switch for torrents, a switch for sharing while downloading, an upload speed limit. Uploaded amount and ratio on rows and in Properties (peers, trackers, piece size).
- A dialog for .torrent files: choose files and folder before they start.
- Quality choice for HLS and DASH streams in the add dialog.
- The link grabber finds magnet and .torrent links.
- Select several downloads: pause, resume, remove, move to a queue. Ctrl+A, Delete, Esc. Paste a link anywhere on the page to fill the add box. A speed graph. Settings show whether ffmpeg was found.

## 3.10.1

Download manager fixes found while testing the pages in a browser:
- The segment bar (progress of the parts of a file) crashed the Downloads page while a split download was running; the server sent the list in a form the page could not read. Fixed.
- Starting a torrent could freeze the download list (it asked for the connections while holding its own lock). Fixed, with a test.
- Torrent Properties no longer show mirrors and "change link", which do not apply.
- More tests: hostile peers (garbage messages), fuzzing of the torrent/magnet readers, no leftover background work after a torrent ends.

## 3.10.0

Download manager, streams:
- **DASH** (`.mpd`) videos: segment templates (number and timeline), segment lists and single-file tracks; best picture and sound are chosen.
- **Separate sound track** (HLS and DASH): picture and sound are downloaded one after the other over all lines and joined into one `.mp4` when `ffmpeg` is found (PATH, or next to NetMerge). Without ffmpeg both files are kept and the list says so.
- **Browser extension:** the popup lists the videos, sounds and streams the current page loaded, with a Download button. It sees only sites you allowed with "Allow for all sites", keeps nothing on disk, and ignores stream parts, tiny files and addresses in your own network.
- **Extension:** a list of file types that are always sent whatever their size (for example pdf).
- Torrents: peers and trackers on this PC or in the local network are never contacted for torrents added from a web page; link-local and multicast addresses are never contacted.
- Not supported: live streams, DRM, DASH with several periods.

## 3.9.0

Download manager: **torrents** (own BitTorrent client, part of the separate download module, off unless the manager is on):
- Add a **magnet link**, a link to a **.torrent file**, or choose / drop a .torrent file. The description of a magnet link is fetched from peers (BEP 9) and kept, so it resumes without peers.
- Peers are found through HTTP and UDP trackers, the DHT and peer exchange. Every connection leaves through one of your lines; peers are spread over the lines by what each line can carry, so the lines add up like for any other download.
- Rarest-first piece picking, every piece checked against its hash, bad pieces fetched again, peers that send bad data are dropped and banned, end-game for the last pieces, resume after a restart (also checks files that are already in the folder).
- Choose which files of a torrent to download (Properties). Torrents get their own category folder.
- The browser extension's right-click menu also takes magnet links.
- Not included yet: incoming connections, encrypted connections (MSE), uTP, and seeding after the download is complete (uploads happen only while downloading, never on a mobile line).

## 3.8.0

Download manager, IDM-style features:
- **Queues**: several named queues, each with its own hours, days of the week, number of downloads at once, and a pause switch. A running download stops when its queue closes and continues when it opens.
- **Folders**: a folder for each file type, and a folder for each website. What to do when a file with that name exists: keep both, replace, or skip.
- **Add with options**: user name and password for protected links (also `ftp://user:pass@host/…` and `http://user:pass@host/…`; the password is never kept in the link or shown), mirrors (other addresses of the same file, tried when the first fails), connections per line, queue, checksum.
- **Change link**: a link that expired can be replaced and the progress is kept.
- **Properties** of each download: details, log of what happened, parts-of-the-file bar, queue, mirrors.
- **Link patterns**: `photo[001-120].jpg`, `[a-f]`, `[1-100:5]` add many links at once.
- **Find links on a web page** and choose which files to download.
- **Save / load the list** (and load a plain text file of links).
- Search, sort and filter by queue; optional program to run after each download (for example a virus scanner, off by default); notifications can be switched off.
- **FTP** (`ftp://`): own client, passive mode, resume, several connections over all lines. Encrypted FTP (`ftps://`) is not supported yet.

## 3.7.0

Download manager speed (measured on a loopback test server before → after):
- Reading from the network and writing to the disk now overlap, with reusable buffers and no extra copy: 549 → about 630 MB/s on four unthrottled lines.
- Big files open 8 connections per line at once instead of climbing one by one (a server that gives each connection 2 MB/s: 6.1 s → 3.2 s for 48 MB).
- A connection that is far slower than what the line normally gets (servers often throttle single connections) is replaced and continues where it stopped (96 MB with every 8th connection throttled: 41 s → 7 s).

## 3.6.0

- The download manager is now a separate module (`dm/`) and is **off by default**. Turn it on in Settings → Download manager. While it is off it starts nothing, reads and writes no files, shows no Downloads page and does not answer the browser extension (which then says the manager is off), so it cannot clash with another download manager.
- It cannot affect the product: it imports nothing from NetMerge, talks to it through one adapter file, and its crashes are caught inside the module.
- The Downloads page now loads only when the manager is on.
- Upgrading: if you used the old built-in downloads, turn the manager on once; your list and settings are kept.

## 3.5.0
Audit for any mix of connections (two Wi-Fi, two Wi-Fi + LAN, three LANs, a LAN swapped for a second Wi-Fi):
- Fixed: usage could jump by up to 4 GB when the adapter behind a connection name changed (a USB adapter plugged in again, a cable moved to another adapter).
- Fixed: downloads kept using an old address after a Wi-Fi reconnected or got a new address, and failed until they were restarted. The address is now read each time a connection is opened.
- Fixed: with the built-in DNS on, downloads, the speed test, health checks and the internet-address lookup could be sent to made-up addresses and fail. Names are now asked again out of the connection itself when that happens.
- A line set to "only when the others are busy" or "backup" is now respected by downloads too (and still listed in the downloads settings).
- The learned speed of a connection is remembered per adapter, so a different adapter that ends up with the same name starts fresh.
- New: "Forget it" on a connection that is no longer plugged in (for example the LAN after moving to a second Wi-Fi). It also fixes the rules and settings that pointed at it.

## 3.4.1
- Fixed: the Downloads page could show up empty (only the title) when no browser had been connected yet.

## 3.4.0
- Downloads: **video streams** (HLS, ".m3u8" links). The many small parts of a stream are fetched over all your connections at once, decrypted when the stream uses plain AES-128, and joined into one video file (.ts, or .mp4 for fragmented streams). The best quality is chosen automatically, finished parts are kept when you pause or restart, and the "add" window shows the stream's length and quality.
- Streams NetMerge cannot take are refused with a clear message: live streams, copy-protected (DRM) streams, streams with sound in a separate part, and DASH (".mpd").

## 3.3.0
- New: **browser extension** for Chrome, Edge, Brave and Firefox. Big downloads are handed to NetMerge automatically (5 MB and more, changeable), right-click any link for "Download with NetMerge", and your browser keeps everything NetMerge should not take (small files, private windows, local addresses, sites you choose). If NetMerge is not running the browser just downloads as before.
- Connect a browser in **Downloads → Download settings** with a one-time code. Tokens are stored hashed, only extensions (not web pages) can use the connection, wrong codes are limited, and each browser can be disconnected. The extension files come with every release.

## 3.2.0
- Downloads hardening: a crash or power cut at any moment no longer risks a damaged file (the next start finishes it correctly), damaged resume files are ignored and the download starts over, and a full disk is reported before the download starts.
- Downloads give up with a clear message when every line is down for a while instead of waiting forever, huge files use larger pieces, link secrets (tokens in the address) are hidden from error messages, and saved cookies are removed once a download finishes.
- The list holds up to 5,000 downloads and stays fast. Idle connections are closed when a download ends.

## 3.1.0
- Downloads: queue order (up, down, first), speed limits (all downloads, per line, per download, changeable while running), choose which lines downloads may use, start at a set time, only download at certain hours, a checksum check (SHA-256, SHA-1, MD5) with a clear result, and a warning when a link is already in the list.
- Calls: while a meeting runs, downloads keep off the call's line (or pause, your choice). Presets now steer downloads: Gaming pauses them, Work slows them, Max speed leaves them alone.
- "When everything is finished": sleep or shut down the PC after the last download (a shutdown gives one minute to cancel). Download settings are part of backup and restore.
- Friendlier error messages for downloads, with the technical text on hover.

## 3.0.0
- New: **Downloads**. Paste a link and NetMerge splits the file into pieces and downloads them over all your connections at once. Fast lines take more pieces, a line that fails is simply skipped, and the file resumes where it stopped after a pause, a crash or a restart.
- Safe by default: websites that cannot be split (or that only accept one address per file) are downloaded over one line automatically, the mobile line is not used unless you allow it, links that lead into your own network are refused, and a file is only finished when its size is right.
- Downloads page: add one or many links, check what a link is before adding it, progress with the share of each connection, pause, resume, retry, open, show in folder, categories and folders, search and filters, drag a link onto the window, tray notification when a file finishes. Overview shows a "Downloading now" card.

## 2.7.0
- New DNS page: choose Windows default (still the default), Cloudflare, Google, Quad9, OpenDNS, AdGuard or your own servers, optionally encrypted (DNS-over-HTTPS). Company/VPN domains can be kept on the Windows DNS.
- "Find the best DNS" test: sends real lookups through every connection to each provider, times them, flags servers that redirect unknown names, and recommends one. Nothing changes until you press Use this.
- Safety: if the chosen servers stop answering, or the engine will not start with them, NetMerge returns to the Windows DNS by itself and says so. Old "built-in DNS" setting keeps working as "NetMerge built-in".
- Backup/restore include the DNS choice.

## 2.6.0
- Overview cards in plain language: "Share of the work", "When to use this line" (Always / Only when the others are busy / Only if the others fail) with a one-line explanation of the current choice, "internet address" instead of "public IP".
- New History page: connection changes (plugged, unplugged, back), outages, presets, merging started and stopped, grouped by day with Problems and Changes filters. Saved on disk (last 500 events) and kept across restarts. "See all" on the Overview opens it.

## 2.5.2
- Settings reorganised: "Everyday" and "Updates and backup" first, rarely needed options folded into "Advanced". Every option says what it does, what happens if you change it, and what is recommended.
- Protection page explains that it is only for special cases and marks each box "Only if needed".
- The separate "Normal" preset is gone: the "Default · my settings" card covers it.

## 2.5.1
- Presets page starts with a "Default · my settings" card: your own settings from before any preset, one click to return to them.

## 2.5.0
- Presets page: Normal, Work from home, Gaming, Max speed, Travel / limited data and Safe browsing, each one click. A preset adds to your settings (your own app rules always win) and "Back to my settings" undoes it.
- Save your current settings as your own preset (up to 12).
- Schedule: switch to a preset between two times (also over midnight) and back afterwards.

## 2.4.1
- Every 10 seconds NetMerge checks that the engine's slots match the split it set, and sets them again (with a line in the log) if they differ.
- Learned line speeds start fresh: values inflated by earlier false spikes are discarded once.

## 2.4.0
- Calls protection now keeps downloads off the call's line: meeting apps use a dedicated CALL line (the steadiest one, moved only when clearly needed) and new connections of other apps steer away from it while a call is running.
- Fixed: the engine could start while a crash-prone driver (AdGuard) was loaded and was stopped a moment later.
- Speed readings taken right after a stall of the sampler are no longer shown (periodic false spikes); stalls are written to the log.
- A line that is far slower or poorer than the others shows a one-click "Use it only when busy" hint.
- Usage page: "Last 7 days" card (total, daily average, busiest day, change on the week before).

## 2.3.3
- Setup guide names the real number of connections found (it said "Two connections found" while listing three).

## 2.3.2
- Speeds on the connection cards are measured between the real counter readings, so they no longer jump to 200+ Mbps and back to zero when the engine answers slowly.
- The window keeps updating every second even when the engine's connection list is slow; the status check no longer waits for the service check.

## 2.3.1
- A new or slow connection (a weak mobile signal) no longer takes a big share of new connections: unknown lines start at a quarter of the best line, laggy or poor lines get less, and a line far slower than the best one stands by instead of slowing pages down.

## 2.3.0
- Sleep and resume: after the PC wakes up, NetMerge re-checks every connection and refreshes addresses and the engine.
- Quality score for each connection (loss, jitter, delay from the last 30 checks).
- Recent activity on the Overview page: what NetMerge changed and why.
- Captive portal detection: a hotel or café Wi-Fi that needs a sign-in page is flagged instead of counted as working.
- Protect calls and meetings: Teams, Zoom, Discord, Skype, Slack, WhatsApp, Webex and Meet stay on one steady line (your own app rules win; switch off in Settings). It does not stop a busy download from filling the line.
- Last 24 hours: speed graph per connection, saved across restarts.
- Back up and restore settings (no passwords).
- First-run setup guide (can be reopened from Settings).

## 2.2.2
- After an update or restart the open window reloads itself, so it never shows an old page or a stale "Lost contact" message.
- The updater waits a little longer before starting the new version.

## 2.2.1
- Connection cards: the details are laid out in a fixed grid and the "Share of new connections" and "Use this line" rows line up across cards, whatever the data length.

## 2.2.0
- Link roles: set a line to "Always", "Only when the others are busy" or "Backup: only if the others fail" on its card on the Overview page.
- Warning when two lines show the same public address (same provider or router), because merging them cannot add speed.
- Diagnostics: one click in Settings saves a zip with logs, settings (without passwords) and a summary.
- Overview status line shows standby lines and provider warnings.
- Removed a personal default bypass address from new installs.

## 2.1.1
- First release delivered through the in-app updater (no changes to merging).

## 2.1.0
- Smart connection dispatcher that spreads each burst of new connections over all your lines.
- Usage tab: daily download and upload for every connection, monthly allowance, usage forecast.
- Share tab: share the merged connection with a phone by scanning a QR code; connected devices with names.
- Overview status line, tray icon, Windows "Public network" warning for sharing.
- Update notifications: NetMerge checks GitHub for new versions and installs them on request.
- First start downloads the network engine by itself.
- Fixes: ARP parsing on non-English Windows, dead speed-test servers removed, hidden window polls less, log size cap.
