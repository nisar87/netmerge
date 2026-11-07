# NetMerge

Combine two or three internet connections (for example a LAN cable and Wi-Fi from two different providers) into one faster connection on **Windows 11**, with no extra hardware, no paid service and no relay server.

NetMerge wraps the open-source [mihomo](https://github.com/MetaCubeX/mihomo) engine in a small app with a friendly window.

## What it does

- **Merges your connections.** Every new connection a program opens is handed to the line with the most spare speed right now, so downloads, uploads and many-connection programs (browsers, torrent clients, game launchers) use all lines at once.
- **Per-app control.** Send a program to one line only, or keep it off the merge.
- **Usage tab.** Download and upload per connection per day, monthly allowance, warnings and a forecast.
- **Share with your phone.** Scan a QR code with a free app (Hiddify, v2rayNG, Streisand, Shadowrocket) and the phone uses the merged connection too. Connected devices are listed and can be named.
- **Ad and tracker blocking** with downloadable filter lists.
- **Health checks.** A line without internet is left out until it works again.
- **Tray icon, start with Windows, speed test, update notifications.**

### An honest limit

One single connection (one download stream) can only use one line. The gain comes from many connections at once, which is what browsers, download managers and most apps do. A single-stream speed test will show one line's speed.

## Install

1. Download `NetMerge.exe` from the [latest release](../../releases/latest).
2. Put it in its own folder (for example `C:\NetMerge`) and run it. Click **Yes** on the administrator prompt (needed to merge connections).
3. On the first start NetMerge downloads the network engine by itself (about 20 MB, checked against its published SHA-256).
4. Turn on **Merging**.

Windows SmartScreen may warn about an unknown publisher the first time, because the file is not code-signed. Choose *More info → Run anyway*, or build it yourself (below).

### Requirements

- Windows 11 (64-bit), administrator rights
- Two or more active connections with a default gateway
- Other firewall or VPN drivers can conflict with the virtual adapter. NetMerge tells you if it finds one (for example AdGuard's network driver).

## Updates

NetMerge looks for a new release on GitHub a few times a day. When there is one, a banner appears with **Update now**. It downloads `NetMerge.exe`, checks it against the SHA-256 published with the release, swaps it in, keeps the old one as `NetMerge-previous.exe` and restarts. **Settings** has a switch to turn automatic checks off.

## Build from source

```
go vet ./... && go test ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w -H=windowsgui" -o NetMerge.exe .
```

The window (HTML, CSS, JavaScript) is embedded from `ui/`. You can run the app on Linux or macOS for interface work; it then uses fake data.

### Publish a release

1. Change the number in `VERSION` (and `appVersion` in `update.go`, which CI checks matches).
2. Add a `## x.y.z` section to `CHANGELOG.md` (it becomes the release notes and the "What's new" in the app).
3. Push to `main`. The *Release* workflow builds `NetMerge.exe` and `NetMerge.exe.sha256` and publishes release `vx.y.z`. Everyone's app finds it within hours (or at once with *Settings → Check for updates*).

## Safety notes

- The proxy-sharing port is opened to the local network only, needs a password, and refuses addresses outside private ranges.
- Updates are verified by SHA-256 published in the same release, over HTTPS from github.com only. This protects against corrupted downloads. Protect your GitHub account with two-factor authentication, because whoever can publish a release can publish an update.

## Files

| Path | Purpose |
| --- | --- |
| `*.go` | The app (engine control, dispatcher, usage, sharing, updates, tray) |
| `ui/` | The window |
| `scripts/` | `STOP.bat`, `ROLLBACK.bat` (restore the previous version) |
| `data/` | Created at runtime: settings, usage, logs (not in git) |

## License

MIT. mihomo is a separate project under its own license.
