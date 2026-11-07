# Changelog

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
