# Changelog

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
