# Screen and workflow specifications

All screens use the shared shell, theme, component states and accessibility rules. This document is the feature-parity inventory. A check mark during future delivery must mean the behavior was verified, not merely that its control is visible. Screens without an HTML preview are fully specified here; the preview is a representative visual reference, not coverage for every interaction.

## 1. Overview

**Question:** Is my internet working as intended? **Primary action:** the global merge control.

Top: title, actual merge status, one priority warning if needed. A compact summary explains contributing versus available connections and the current mode. Four metrics: download, upload, contributing links, uptime. Main chart below: Live (existing 90-s samples) / Last 24 hours (existing history); never imply longer retention. Show scope and timestamp. On the right/below, connection summaries with the same identity colors; recent activity; optional download summary with link to Transfers.

Move advanced weights, network identities, roles and quotas to connection details. Keep a visible mode selector or link next to the summary: Combine all (`fast`), Keep connections steady (`stable`), or selected line (`link:<name>`). This still edits the existing setting.

States: initial loading; merging off (label raw counters “Adapter traffic”); starting; running with 2+ contributing links; only one link; no links; all excluded; provider overlap; engine error; call protection; disconnected API; collecting history. Never label standby or unavailable adapters as contributing simply because `enabled` is true. Show unknown health as unknown. Preserve total/peak and downloaded-byte information in the chart summary/details.

## 2. Connections

**Primary action:** Scan for connections (secondary to global merging). Tabs: Connections, Speed test, Presets.

Use full-width, comparable rows or a compact responsive grid for 2–4 lines. Each row: type icon, user label, status text, current download/upload, role, and Details. A header explains Smart vs Classic balancing. Global mode remains reachable here. Stable sorting by adapter identity prevents rows moving under the pointer.

Details drawer: rename; included/excluded switch; original adapter alias, description, IP, link rate, public IP; current throughput, latency, quality and history; traffic contribution and today's usage; allowance link; role Always/When others are busy/Backup; Classic weight 1–5 only editable in the applicable mode; missing-adapter Forget with confirmation. Preserve unavailable information as “Not available”; link speed is not internet speed. Missing/disabled/dead/captive portal/quota exhausted/slow standby/role standby each have distinct text and next actions only if existing APIs support them.

### Speed test

Idle state explains duration and bandwidth consumption. Start once; while running show current phase, progress and live Mbps. Results compare per-line single/multiple streams, combined capacity, actual through-NetMerge measurement and scaling. Keep server warning text and failed-line errors. Show Apply suggested shares only when suggestions exist, with Smart/Classic context. Do not invent a cancel endpoint, automatic retesting, or historical test results. Leave the page without canceling the backend operation; show its ongoing status on return.

### Presets

Show active preset and its impact before a grid/list of built-in profiles. Retain Default/my settings restoration; custom save name and delete; schedule enabled/from/to/preset; scheduled-active explanation. Save current settings is an explicit action. A schedule change shows saved or failed; it does not claim a new scheduler. Clearly show midnight-crossing windows using existing server semantics. Protect custom input and selected schedule from refreshes.

## 3. Downloads

**Primary action:** Add download. Tabs: Transfers, Queues, Browser, Settings. If manager is off, show “Downloads are off”, existing opt-in consequences and Enable downloads. Do not silently enable on navigation, paste, or drag/drop. Disabling stops jobs according to existing manager semantics; explain that they resume when re-enabled. Merging and manager enablement are independent.

### Transfers

Top summary: active rate, active/waiting counts and any hold reason. Compact toolbar: Add download, search, status filters (All / In progress / Finished / Needs attention), type, queue, sort, overflow menu. Pause all, Resume all, Clear finished remain reachable. A selected-row action bar appears only with selection; show selected and visible counts separately.

Rows show file icon/name, source host, progress with known/unknown total, transferred/total, speed, ETA only when size/speed justify it, and status. Connection contribution is available in expanded row/details. Never show a fabricated percentage for unknown size. States preserve queued, downloading, paused, done, error; add presentation labels for retryAt, verification, scheduled wait, call hold, preset hold, no internet, single-line fallback, assembling streams and torrent metadata only when actual response fields justify them.

Do not reorder live rows every second; use stable ties, update displayed values in place, and defer disruptive sort changes while a row/menu has focus. Filters operate on existing item data. For large lists prefer explicit pagination (100 rows/page) over fragile custom virtualization; preserve selection across pages and provide Clear selection. Ctrl+A selects visible matching items exactly as existing behavior; text inputs retain native Ctrl+A. Delete uses the existing removal confirmation; Escape clears selection or closes the top overlay.

### Add download

480–640 px dialog, one visible label “Links”, multiline input, paste button with Ctrl+V fallback, torrent file chooser, Download now and Add to queue. Drag/drop and page-level paste fill this flow, never silently start jobs. Preserve batch links and expansion patterns with limits reported by backend.

Single URL → probe metadata → file/name/size/resume/line information → confirm. Probe errors retain entered values. Multi-URL submissions use existing add behavior and report added/skipped/partial results honestly. Duplicates use existing feedback/force handling, not an automatic retry. Options disclosure retains destination, file name, selected connections (at least one), speed limit, future start, checksum, headers, Basic auth, mirrors, queue, parallel connections, and stream quality. Passwords/headers stay in transient form state and are excluded from preview fixtures, URL fragments, analytics and error dumps.

Torrent file flow: read file → `torrentInfo` preview → file selection and total chosen size → Download now/Add to queue. No selection is invalid. Magnet metadata may be unavailable initially; show that explicitly. Streams: show quality choices from probe, ffmpeg availability and separate-audio fallback; preserve unsupported live/DRM/multi-period errors. No player, transcoding pipeline, or ffmpeg installer is introduced.

### Details and recovery

Drawer/tab groups: Summary, Connections/parts, Files (torrent), Activity, Options. Include checksum state, transfer totals, per-line stats and segment map; torrent peers/seeds/uploads with existing limitations; folder and full filename; recent job log. Preserve open, reveal folder, pause/resume/retry, queue/move priority, speed limit, parallel connections, file selection, relink with headers and mirrors.

Removal is explicit: “Remove from list” versus “Delete file from computer”. Unfinished removal may delete partial data even without `deleteFiles:true`; preserve that warning. Confirm risky executable open using the existing extension classification. Completion sleep/shutdown countdown stays globally visible while active, with Cancel using `cancelPower`.

### Queues

Queue list with name, schedule, paused state, concurrency, count if derivable from items. Detail panel edits name, hours, days and concurrency, with existing pause/start/delete controls. Main queue is clearly distinguished from named queues. Keep `startQueue` behavior and queue IDs unchanged; no new scheduling engine. Reorder jobs using existing move controls, with keyboard/button alternatives to any future drag affordance.

### Browser

Connected-browser list with created/last-seen timestamps and Disconnect. Primary Connect a browser opens instructions, correct release package links returned from the existing repo context, install steps by browser, code, copy and two-minute expiry. Expired codes show Generate new code; requests only on explicit action. Explain manual Firefox temporary installation where applicable using the existing product instructions, with no store availability claim. Global handoff enablement and minimum-size options retain `extConfig` semantics. Never persist pairing codes or tokens in UI storage/screenshots.

### Download settings: all existing controls

| Group | Controls to retain |
|---|---|
| Files & folders | Default directory, category folders, per-category directories, site folder rules, rename/overwrite/skip collisions |
| Speed & connections | Overall limit, per-line limits/exclusions, simultaneous jobs, connections per line, mobile allowance with existing only-line fallback |
| Schedule & calls | Download hours, call mode avoid/pause/off, queues link, after-all nothing/sleep/shutdown and cancellation |
| Torrents & streams | Torrent enablement, upload enablement/limit, ffmpeg status and existing fallback copy |
| Automation | Notifications, after-download command with `%f`, learned-site reset |
| Lists & tools | Export/import, link grabber with file-type filter and selection, pattern examples |

Settings use grouped subheadings and a local jump list, not another large scroll wall. Existing configuration fields remain intact when saving one section.

## 4. Apps

Tabs Active apps / Saved rules. Active table: process name, down/up, connections, contributing lines, effective rule and Change rule. Search only the data already available. Saved rules: create using seen-process suggestions, edit and delete. Targets: default/no rule, fast, stable, specific adapter, bypass. A friendly description accompanies each technical process name where already known; do not invent an installed-app inventory API or executable icons.

No active apps is a useful empty state distinct from API failure. Saved rules remain visible if the process is stopped or its chosen adapter is missing. Preserve existing rule strings and precedence. Explain routing applies to new connections rather than promising existing sessions move instantly.

## 5. Usage

Summary by connection and combined period: downloaded, uploaded, used, allowance, remaining/forecast when derivable. Daily stacked bars use existing returned days, with an equivalent table and explicit units; default to the actual 31-day data window. Do not offer unbacked yearly history or arbitrary server filtering.

Allowances tab: per-link optional allowance, reset day 1–28 and exhaustion action Warn/Leave out. Zero/empty means unlimited. Explain 80% and 100% warnings; backend determines quota exhaustion. Display current allowance period separately from the daily-history window. Treat forecast as an estimate, not a server certainty. Save section with inline errors; confirm when a setting can exclude the active line.

## 6. Protection

### Routing

Three clear sections: bypass addresses, sites requiring a steady connection, VPN connection. Preserve chips/add/remove and existing validation formats. Full values are available without hover. Long hostnames wrap. Link call-protection preferences to Settings rather than duplicating their source of truth.

### Blocking

Master switch and effective state: enabled but merging off is not active protection. Summary counts, filter packs with update status, top blocked domains, recent/top-20 table with Allow, editable custom block/allow lists with explicit Save. Keep whitelist precedence and original field shapes. List update is asynchronous; show last known state, never success just because the request was accepted.

### DNS

Actual active provider, encryption and fallback appear before configuration. Provider selector, custom primary/secondary addresses, company-domain exceptions; draft Apply and reset to Windows. Explain merge restart beside Apply. Test providers shows progress, per-line medians, failure rates, hijack/redirect warnings, recommendation and Use this (explicit apply). Preserve Windows/builtin/provider/custom values. Tests never auto-apply. A network fallback message stays visible until actual state resolves.

## 7. Sharing

Distinct enabled and usable status; merging off, public-network warning and missing local address remain explicit. Guided panel: Android/iPhone choice, existing app instructions, address selection, QR, Copy link and connection step. Use existing QR algorithm and protocol generation. QR has fixed white backing, adequate quiet zone and no decorative recoloring. Provide manual setup with server/port/user/password and existing protocol tabs; mask password until reveal and allow copy deliberately.

Connected devices table: existing ID/name, address, activity and traffic. Rename preserves stable device identity. No invented kick-device API. Password rotation confirmation explains that devices must reconnect; backend generates the password. Disabled sharing gets a concise empty state with Enable sharing. Never capture real QR secrets in visual regression fixtures.

## 8. Activity

Events tab: timeline, Everything/Problems/Changes filters, local search if desired, timestamp, affected connection and event text. Keep provided order and finite server history; no infinite-history promise. New events do not shift a focused row. Logs tab: NetMerge/Engine/Speed selector, selectable monospaced text, manual refresh and optional visual auto-follow that pauses when user scrolls away. Preserve empty-log explanation and detailed-logging dependency. Diagnostics export stays available with the existing note about website names in logs.

## 9. Settings and first run

General: start with Windows, merge automatically, call protection, download-manager opt-in; explain close-window versus Quit. Appearance: Auto/Light/Dark with labeled preview swatches. Updates & backup: current version, check, automatic-check preference, release notes, install/progress/restart/errors; backup download, restore upload and a confirmation that settings will change; reopen setup. Advanced: Smart/Classic, engine mode Normal/System/Compatibility (restart consequence), rescan, detailed logs, diagnostics, engine version.

Do not expose unused backend settings just because they exist. No account, billing, cloud sync, telemetry opt-in, custom update feed, network driver installation, or updater rewrite.

First run becomes a focused dialog with connection readiness, engine download/progress/error/retry, launch options and Start merging/Skip. Show one-connection/no-connection guidance. “Done” does not claim merging started; wait for actual engine state. Preserve `setupDone` behavior. Modal keyboard handling must match the download dialogs. Installation/admin prompts and tray remain owned by the existing app.

## 10. Browser extension popup

Use shared color/type/icon roles in a compact 340–360 px popup, respecting small browser windows. Keep all current DOM hooks and runtime-message/storage contracts. States: app unreachable, manager off, unpaired, pairing pending/invalid/expired, connected idle/active, globally disabled, permission-needed, detected media, duplicate handoff and send failure.

Primary action matches the state: Retry / Connect / media Download. Include visible pairing label; inline error; connected status; local handoff switch; minimum-size explanation; collapsed advanced site/file-type inputs; existing permissions explanation and request; Disconnect. Fix the instructional path to match the new Browser tab when implemented. `background.js`, `core.js`, manifest permissions, and interception behavior remain unchanged.
