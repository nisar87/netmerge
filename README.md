# NetMerge

Combine two or three internet connections (for example a LAN cable and Wi-Fi from two different providers) into one faster connection on **Windows 11**, with no extra hardware, no paid service and no relay server.

NetMerge wraps the open-source [mihomo](https://github.com/MetaCubeX/mihomo) engine in a small app with a friendly window.

## What it does

- **Merges your connections.** Every new connection a program opens is handed to the line with the most spare speed right now, so downloads, uploads and many-connection programs (browsers, torrent clients, game launchers) use all lines at once.
- **Per-app control.** Send a program to one line only, or keep it off the merge.
- **Usage tab.** Download and upload per connection per day, monthly allowance, warnings and a forecast.
- **Share with your phone.** Scan a QR code with a free app (Hiddify, v2rayNG, Streisand, Shadowrocket) and the phone uses the merged connection too. Connected devices are listed and can be named.
- **Download manager (optional, off by default).** Turn it on in Settings; if you use another download manager, leave it off and NetMerge never touches your downloads. It lives in its own module (`dm/`) that cannot affect merging. Paste a link and the file is split into pieces and downloaded over all lines at once (fast lines take more), with pause, resume after a crash or restart, queue, categories and a share-per-line view. Sites that cannot be split fall back to one line.
- **Torrents.** Paste a magnet link or a link to a .torrent file, or choose a .torrent file. Own BitTorrent client: trackers (HTTP/UDP), DHT, peer exchange, resume, choose which files to download. Each peer connection leaves through one of your lines, spread by what the line can carry. No incoming connections, no encryption (MSE), no uTP, and it does not seed after finishing.
- **Video streams.** Paste an HLS (`.m3u8`) or DASH (`.mpd`) link: NetMerge downloads the parts over all lines and joins them into one file. When picture and sound are separate tracks they are joined into one `.mp4` if `ffmpeg` is on the PC (in the PATH or next to NetMerge; NetMerge does not install it), otherwise you get two files. Not supported: live streams, copy-protected (DRM) streams, DASH with several periods.
- **Browser extension.** For Chrome, Edge, Brave and Firefox: big downloads in the browser are handed to NetMerge automatically (with right-click "Download with NetMerge" for any link, including magnet links; the extension popup also lists the videos and streams the current page loaded). See *Browser extension* below.
- **Ad and tracker blocking** with downloadable filter lists.
- **Link roles.** Use a line always, only when the others are busy, or only as a backup (for example a phone hotspot with limited data).
- **Same-provider warning.** If two lines leave through the same public address, NetMerge tells you merging cannot add speed.
- **Diagnostics export.** One click saves the logs, settings (without passwords) and a summary.
- **Quality score, activity log and 24-hour graph.** See how steady each line is and what NetMerge changed.
- **Calls protection.** Meeting apps use one steady line and downloads are steered away from it during a call.
- **Presets and schedule.** Gaming, work, travel and more in one click, with your own presets and a time schedule.
- **Backup and restore** of settings, plus a first-run guide.
- **Health checks.** A line without internet is left out until it works again.
- **Tray icon, start with Windows, speed test, update notifications.**

### An honest limit

One single connection (one download stream) can only use one line. The built-in download manager gets around this for files by fetching different pieces over different lines, when the website allows it. The gain comes from many connections at once, which is what browsers, download managers and most apps do. A single-stream speed test will show one line's speed.

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

## Browser extension

Each release has two extra files: `NetMerge-extension-chrome-edge.zip` and `NetMerge-extension-firefox.zip`.

1. **Chrome, Edge, Brave:** unzip the file into a folder, open `chrome://extensions` (or `edge://extensions`), switch on *Developer mode*, choose *Load unpacked* and pick the folder. **Firefox:** open `about:debugging#/runtime/this-firefox`, choose *Load Temporary Add-on* and pick the zip (Firefox forgets temporary add-ons when it closes; a signed version needs a Mozilla account).
2. In NetMerge open **Downloads → Download settings → Connect a browser**. A one-time code appears (valid two minutes).
3. Click the extension icon in the browser and type the code. Done.

From then on, downloads of 5 MB or more (changeable in NetMerge) are cancelled in the browser and downloaded by NetMerge over all lines. The browser keeps small files, private windows, addresses inside your own network and sites you list in the extension. If NetMerge is not running, the browser downloads as usual. For links that need you to be signed in, click *Allow for all sites* in the extension so your sign-in cookie for that link can go along.

How it stays safe: the extension talks to NetMerge only on this computer (`127.0.0.1`). Web pages cannot use that connection (it only accepts calls from extensions), every call needs a private token that is stored hashed in NetMerge, and each browser can be disconnected from NetMerge at any time. Links that lead into your own network are always refused.

Limits: downloads that need a form to be submitted again (a POST) or a one-time link that the browser already used cannot be repeated by NetMerge; use the browser for those (switch the extension off for that site). Chrome and Edge store listings are not published yet, so the extension is loaded by hand.

To build it yourself: `python3 extension/build.py` (packages appear in `extension/dist/`), tests: `node --test extension/core.test.js`.

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
| `extension/` | The browser extension (Chrome/Edge and Firefox) |
| `scripts/` | `STOP.bat`, `ROLLBACK.bat` (restore the previous version) |
| `data/` | Created at runtime: settings, usage, logs (not in git) |

## License

MIT. mihomo is a separate project under its own license.
