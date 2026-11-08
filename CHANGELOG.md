# Changelog

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
