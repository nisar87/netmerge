# Changelog

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
