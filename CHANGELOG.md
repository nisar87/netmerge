# Changelog

## 3.18.0

- **One engine, built in.** NetMerge's own network engine is now the only one. The separate engine that NetMerge used to download (about 60 MB, in the `core` folder) is gone: nothing is downloaded on first start, and an old `core` folder is removed the first time 3.18.0 starts. Before this change the two engines were compared once more side by side on GitHub's machines, with the same settings:

  | | Windows PC (real internet) | Test network (Linux) |
  |---|---|---|
  | Start | 73% faster | 48% faster |
  | Connection / response time | 5 to 8% faster | 83% faster (TLS 72%, UDP 92%) |
  | Downloads | Cloudflare 15% faster; Google about the same (8 parallel: 4% slower, within the noise) | 4 to 25% faster |
  | Memory (peak) | 60% less | 59% less |
  | CPU | 67% less | 23% less |
  | Blocked sites refused correctly | | 40 of 40 (previous: 30) |

  Every setting NetMerge writes is understood by the engine; a new test checks this for 13 combinations of settings.
- **Removed: Settings → Advanced → Network engine and Network engine mode.** There is nothing to choose any more; those modes only applied to the previous engine. Settings → About shows the engine's version.
- **Settings → About.** Who made NetMerge, with links to nisarworks.com and to the source code.

## 3.17.5

- **Diagnostics now show what each line carried.** The diagnostics zip includes the last minutes of per-line speeds, connections and slots (kept even when the debug log is off) and the list of open connections with the line each one uses. A friend's zip about an unused hotspot could not show this before: the per-second record was only written with the debug log on.

## 3.17.4

- **A struggling line no longer draws new connections.** In a speed test on the real PC the cable suddenly carried only 7 of its usual 64 Mbit/s while its response time rose from 90 ms to 430 ms; because NetMerge remembered how fast it can be, it kept giving it 4 to 6 of every 8 new connections. Now a line that carries far less than its usual speed while its response time has risen to several times its usual one counts as full: new connections go to the other lines until it recovers (shown as "congested" in the diagnostics). A line at full speed, a line with room to spare and a line that is merely a little slower are not affected (checked against the readings from the PC's log). This is decided by NetMerge itself, so it works the same with either engine. Connections that are already open stay where they are; a connection cannot move to another line.

## 3.17.3

- **Wi-Fi took no part in a speed test (NetMerge engine).** Found in the PC's logs: 11 seconds before the test, nvidia's update server did not answer on Wi-Fi (it does not answer on either line, every hour). The engine then passed Wi-Fi over for 15 seconds, so all 8 of the speed test's connections went to the cable and Wi-Fi carried nothing. A line is now passed over only when another line made the same connection while it failed or hung, and then for 10 seconds; when every line fails the same connection (a server that is down), no line is blamed.

## 3.17.2

Fixes from five hours of the NetMerge engine on the real PC (3.17.1, both lines, 2,650 connections, 289 blocks counted):

- **No IPv6 through the NetMerge engine when IPv6 is off** (NetMerge's setting, as with the standard engine). The virtual adapter offered an IPv6 path, so Windows tried IPv6 first (Teams did) through lines that have none, and those attempts waited until they failed.
- **Merging shows as on only once traffic really flows through the NetMerge engine.** Its first start on the PC took 3 seconds to get capture ready (the firewall permission is added once; later starts skip it), while NetMerge already said it had started.
- **Adapters are listed with Windows' own function instead of PowerShell**: about a millisecond instead of seconds, and no more "adapter scan failed" (seen 7 times in the PC's log). Checked against PowerShell's list on a Windows machine in CI.

What the PC's log showed about the NetMerge engine itself: every connection was named with its program, websites with their names; connections that failed: 7 in 5 hours (the standard engine, the 1.5 hours before on the same PC: 25), of which 5 were nvidia's server not answering on either engine; 7 connections were rescued by moving to the other line within about a second.

## 3.17.1

- **The NetMerge engine now really starts.** In 3.17.0 it refused NetMerge's own settings file (the file starts with a comment line, which the standard engine accepted and the new engine did not), so NetMerge went back to the standard engine after a few seconds. Found in the logs of the first test on a real PC; the engine test now writes the settings file exactly as NetMerge does.
- **Compared with the standard engine, side by side, every time the engine changes.** A new check runs both engines in turns with the same settings, on a Windows machine (internet) and on a private test network (no internet noise, a 350,000-name block list like the real one). In the latest runs the NetMerge engine was ahead of the standard engine or level with it on every measurement (on the internet, HTTPS, UDP and the Cloudflare download came out level, within the variation between runs); the latest results:

  | | Standard engine | NetMerge engine |
  |---|---|---|
  | Start until traffic flows through it (Windows, with block lists) | 1.2 s | 0.39 s |
  | New connection, small page (Windows) | 55.2 ms | 53.3 ms |
  | One download (Windows, Google) | 445 Mbit/s | 741 Mbit/s |
  | 8 downloads at once (Windows, Google) | 1,229 Mbit/s | 1,696 Mbit/s |
  | Engine memory, peak (Windows) | 150 MB | 59 MB |
  | Engine CPU time for the same work (Windows) | 5.0 s | 0.7 s |
  | New connection (test network) | 5.8 ms | 0.7 ms |
  | 64 downloads at once (test network) | 3.9 Gbit/s | 5.2 Gbit/s |
  | Blocked websites refused correctly, of 40 (test network) | 30 | 40 |

  The standard engine let plain-web ads on port 8080 through; the NetMerge engine refuses them by name, and writes the website's name (not its address) in the log, so "Blocked today" shows names. Internet results vary from run to run; the full tables are in the "Engine comparison" checks on GitHub.
- **What was changed to get there:** the adapter is set up with Windows' own network functions instead of a netsh program per setting (capture ready in a fraction of a second); the adapter is created while the block lists load; block lists load twice as fast and take 8 MB instead of tens of MB; checksums are updated only for the changed parts of each packet; the program of a connection is looked up while the connection's first bytes are awaited, from the matching table only; adapters are found by name directly (2 ms less per new connection on Windows); lost DNS answers are asked again.
- **Switching engines while NetMerge is closed** now starts the chosen engine instead of keeping the other one running.
- Error messages about a failed engine start show only that engine's own lines.

Still not tested on your PC: please switch to the NetMerge engine in Settings → Advanced and use it as usual.

## 3.17.0

- **NetMerge's own network engine, to try.** Settings → Advanced → Network engine now offers "NetMerge engine (testing)" next to the standard engine. It is written for NetMerge only, is part of NetMerge.exe (nothing extra to download) and does what NetMerge uses: merging over your lines, app and website rules, blocking, call protection, sharing with phones (proxy and Shadowsocks), and capturing the PC's traffic. New connections that would wait on a line that does not answer are moved to another line within about a second. If it cannot start, NetMerge uses the standard engine by itself and says so; Settings → Advanced → Engine shows which one runs. The standard engine stays the default.
  - Tested: every check NetMerge has for an engine passes in CI with the new engine, the capture test runs on Linux and on a Windows machine (Wintun) in CI. **Not yet tested on a real PC with several lines**: please try it there and switch back if anything misbehaves.
- **The window no longer freezes while merging starts.** Starting the engine used to hold the window for up to 15 seconds.
- **NetMerge only closes its own engine.** It used to close every program of the standard engine's name on the PC, including other programs' copies.
- **Speeds keep updating while the engine is checked.** The engine check every 5 seconds could hold up the speed reading for several seconds (seen 40 times in a real log as "sampler was held up").
- **Line choices are set again right after settings change.** Saving a setting reloads the engine, which forgot which line each slot used until the next check ("slot check" lines in the log).
- **Data used while NetMerge is closed is counted.** On the next start NetMerge adds what each adapter carried in between (to that day), unless Windows restarted meanwhile. Adapters' 64-bit counters are used for this, so more than 4 GB is counted right.
- **Update check works when GitHub's API refuses** (it allows 60 checks an hour per internet address, which mobile networks share): NetMerge then reads the release page instead.

## 3.16.3

- **Blocked ads from shared phones are now counted.** Ads were already blocked for a phone using Sharing, but "blocked today", the total and the "Blocked today" list missed them: NetMerge looked for a program name in the engine's log line, and a phone's line has none. On a real PC's log the old count caught 97 of 3,183 blocked connections (the other 3,086 came from a phone); now all are counted. A phone's entries show as "Shared device (its address)". Its websites may appear as an address instead of a name, because that is what the engine reports for them.

## 3.16.2

- **Allowed websites now follow your rules.** With blocking on, a website on an allow list used to skip every rule after it: an app sent to one line (or to skip NetMerge) was merged anyway for that site, a bank or login site you allowed could be spread over several lines, and an allowed call address left the steady call line. Now an allowed website is simply not blocked, and goes on to your app rules, call protection, "keep on one line" list and the built-in bank rule like any other site. This also covers the sites NetMerge always allows (GitHub, jsdelivr, gstatic and Microsoft's connection check). Checked with the standard engine version NetMerge used then: it accepts the new rule, refuses blocked sites and lets allowed ones continue; the "blocked today" counts read the engine's new log line.
- **One list of blocked websites.** "Most blocked today" and "Blocked today" showed the same websites. They are now one list, most blocked first, with a bar next to each count and the Allow button.

## 3.16.1

Every text in the window was checked against what NetMerge really does, and corrected where it did not match. Nothing else changed.

- **Overview:** "Peak speed" and "Downloaded" now say they count since NetMerge opened (they did not reset when merging started). The status line describes the mode you use: spare speed (Smart), turns by your shares (Classic), or one line per website (steady sites).
- **Traffic mode:** "steady sites" says it uses the shares you set; one-line mode says the other connections (all of them) wait as backup; Classic balancing is described as taking turns by your shares.
- **Roles and calls:** "Only when the others are busy" and the call protection that keeps downloads off the call's line work only with Smart balancing in "fastest" mode; the texts now say so. A connection that is not plugged in no longer says "or no internet".
- **Settings:** Connection balancing points to the shares on each connection card (they moved to Overview); Compatibility engine mode warns that it is much slower; NetMerge checks for new connections about every 10 seconds and for updates every 6 hours; detailed logging and backup (no passwords, no phone names) are described as they work.
- **Protection:** local network addresses and websites with "bank" in their name already have built-in rules; the VPN list names more clients and says Automatic picks the first wired line; blocking also covers phones you share with. The stronger block list now points to the "Blocked today" list by its real name.
- **Also:** the speed test takes about two minutes; suggested shares say when they are used; encrypted DNS falls back within a few minutes; app rules point to the traffic mode, not "the Overview setting".

## 3.16.0

- **A real speedometer on Overview.** The combined speed is now a gauge with a needle, tick marks and a scale. The arc fills and the needle moves with the real download speed, and the colour changes with it: grey when idle, amber for light use, yellow when busy, green near top speed. The scale follows this session's top speed, so it fits your connections. The thin inner ring still shows each line's share.
- **A small gauge on every connection card**, coloured the same way, next to that line's speed.
- Only the window changed: merging and everything else work exactly as before.

## 3.15.0

The whole window now has the look of the new Overview. Only the window changed: merging, usage counting, settings and everything else work exactly as before.

- **Each connection in one place.** Your connections now have one card each, on Overview: live speed, a 90-second graph, share of traffic and response time on top, the on/off switch in the corner, and its name, addresses, share of the work and "when to use" under **Settings** on the card. The Connections page no longer repeats them.
- **Connections → Traffic mode.** Choose how your lines work together with three pictures (fastest, steady sites, one line only) instead of a drop-down list, with a count of connections found, in the merge, waiting and unplugged.
- **Usage.** A ring for each connection (its share of the data, or of its allowance), a period total with a colour split, the last 7 days as small bars, and a 31-day bar chart above the daily table.
- **Apps.** A summary of apps online, download speed and rules; each app has a coloured initial and a speed bar.
- **Protection.** Blocking opens with a shield and the day's count; the most blocked websites are shown as bars. Routing exceptions and DNS have the same clear header.
- **Activity.** Events are a timeline with counts for today, problems and in total; logs read like a terminal.
- **Presets and Settings.** Each preset has its own icon; Settings sections have clearer headings.

## 3.14.0

A redesigned Overview. Only the window changed: merging, usage counting, settings and everything else work exactly as before.

- **One picture of the merge.** Each working connection flows into one ring that shows the combined speed. Each line's slice of the ring and the thickness of its ribbon show its share of the traffic right now, and the ribbons move while data flows. Upload, peak speed, data downloaded and running time sit next to it.
- **Live speed by connection, on Overview.** Every connection has its own card with its current download and upload speed, a 90-second graph, its share of the traffic and its response time. A line that is unplugged, off or standing by is shown dimmed with its status.
- **Each figure in one place.** Live figures (speed, graph, response time, share of traffic) are now only on Overview, data used today is on Usage, and the Connections page keeps what you set there: on/off, name, addresses, share of the work and preferences. The short connection list and the numbers in the chart legend, which repeated the same speeds, are gone.
- **Clearer last 24 hours.** The graph has a speed scale, grid lines, shaded areas and each connection's peak and average. Recent activity is now a short timeline next to it.

## 3.13.1

- After an update the window now refreshes by itself. Before, it showed "NetMerge was updated. Reload when you are ready" and kept running the old version until you clicked Reload. If you are in the middle of something (a setting being saved, an open dialog, typing in a field, or unsaved text on another page), it waits and refreshes once you are done.

## 3.13.0

- The download manager is no longer part of NetMerge. It is now a program of its own, **Pullwell** (https://github.com/nisar87/pullwell). Like any download manager it opens many connections, and while NetMerge is on those connections are spread over all your lines.
- Nothing is deleted: files you downloaded, the old list and its settings stay where they are. If you used the old download manager, **Settings → Download manager → Save old list** saves the list in the format Pullwell imports.
- The Downloads page, its settings and the browser extension are gone. Uninstall the NetMerge extension from your browser and use Pullwell's own. Old links to the Downloads page open Settings.
- Presets no longer pause or slow downloads. Merging, usage counting and all other features are unchanged.

## 3.12.0

- A redesigned desktop control center with light, dark and system themes, clearer connection status, responsive navigation, local icons, accessible dialogs and restrained motion. Connection, download, app, usage, protection, sharing, activity and settings controls have dedicated pages, with advanced options still available.
- Speed tests now have one large live speed meter, a throughput graph and expandable details for individual connections, parallel streams and recommendations. The mobile-data warning remains visible.
- Downloads have compact, clearly labelled action icons, bulk controls and paged transfer lists. New transfers appear in the selected sort order, explicit queue moves update the displayed order, and a failed first refresh can be retried immediately.
- Download and browser preferences keep the final choice when settings are changed quickly while a save is pending. App-routing edits stay attached to the correct process when the rule list changes in another window.
- Re-adding a stopped download preserves the existing copy and accurately reports its restart. Usage corrections retain edits during refreshes, and missing usage readings are handled without a page error.
- The browser extension popup matches the new interface while preserving pairing, capture settings and interception behavior.
- Windows executables now include the NetMerge icon at eight sizes, alongside release product and version details. The window title remains compatible with the existing tray controls.
- The stable 3.11.25 merging engine, download backend, update behavior and saved settings formats are unchanged; this release brings the redesigned frontend and desktop branding to the stable app.

## 3.11.25

- No changes to the program: this release checks on a real PC that the new update step of 3.11.24 installs a newer version correctly.

## 3.11.24

Found in a full review of NetMerge:

- Security: any website open in a browser could make NetMerge act through a plain link or image pointing at 127.0.0.1:9196: turn merging off, quit NetMerge, start a speed test (using mobile data), change the line shares or block settings. Everything that changes something now accepts only the page's own request (POST with NetMerge's header), which other websites cannot send.
- Usage: a byte counter read across a long gap (the PC waking from sleep) is no longer counted. A USB adapter's counter often restarts during sleep, and over hours such a restart could pass as believable traffic and add up to about 4 GB of false usage.
- The network engine's download is now checked against a fingerprint fixed inside NetMerge, not only against the one listed next to the file.
- Updating: if keeping a copy of the running version failed, the update could put back an older version instead. The new version is now put in place with a few retries, and the previous one is restored only if the program file is missing.
- The desktop shortcut was not created when the folder name contains an apostrophe (C:\Users\O'Brien).
- A second launch could hang if another program held NetMerge's port without answering.
- The speed test page now says that it uses real data on every connection (about 100-300 MB on a mobile line).
- The crash log is started afresh once it passes 1 MB.

## 3.11.23

- A line that was just plugged back in (a USB phone, a Wi-Fi stick) could show "Too slow to help: standing by" for a few seconds. The label was shown whenever a line had no share of the connections, and a returning line has none until the next split is settled. It now appears only when NetMerge has really judged the line far slower than the best one.

## 3.11.22

- Downloads: a name you type in the add window is now kept. Before, the server's name always replaced it (found on a real PC). Names suggested by the browser still give way to the server's, as before.

## 3.11.21

- Downloads, the end of a file: when every piece was handed out, a connection that finished early could only race a piece already running on another line, so the last pieces crawled along on single slow connections (seen on a real PC: the last 4 MB of a 148 MB file took 3 seconds while the Wi-Fi's slow connections held them). Now an idle connection takes the second half of the biggest piece still running, again and again, until the halves get small. Interrupted downloads still resume where they stopped.
- Downloads: fixed a data race found by the race detector: the download list could read a download's size and pieces while that download was still starting up.
- Downloads: pasting a link again whose download failed or was paused now restarts that download, instead of quietly doing nothing. The add window says so ("already in your list but stopped"). "Add, start later" still leaves it alone.

## 3.11.20

- Downloads: some servers refuse a download unless the request names the page it came from (a `Referer`), which browsers always send. Seen with Cloudflare's speed-test link, which failed with "the website refused the download" although it works in a browser. When a server refuses the first request and no referer was given, NetMerge now asks once more with the site's own address as the referer (IDM does the same). A referer passed by the browser extension is kept unchanged.

## 3.11.19

- A connection is now called "too slow to help: standing by" only when it is under a tenth of the best connection in both directions. Before, download and upload were added together, so a phone that downloads well but had never been seen uploading could flip between normal and "too slow" with no connections (seen on a real PC: 18 Mbps down, 0.6 up, next to a wired line of 93 down and 99 up).

## 3.11.18

- Usage: the same counter restart that confused the line speeds (3.11.17) also added about 4.3 GB to that day's usage in each direction, which could wrongly use up a monthly allowance. New on the Usage page: "Correct a wrong number" (choose the connection and the day, type the real figure). The rejection of impossible counter readings now has its own test using the exact numbers seen on a real PC.

## 3.11.17

- Fixed a merge problem that appeared after a connection was unplugged and plugged in again (a USB phone, a Wi-Fi stick): when the adapter's byte counter restarted, NetMerge read it as billions of bytes in one second, learned that as the line's speed and saved it. Every other line then looked "too slow to help: standing by" and all connections went to the phone, even after restarting. Impossible counter readings are now thrown away (for speed and for usage), no line can be remembered as faster than its adapter can carry, and the saved speed memory is reset (it is relearned within seconds).

## 3.11.16

- "Show in folder": the Explorer window was not being found because its title can carry " - File Explorer" after the folder name. It is now matched correctly, and the log lists the Explorer windows it saw when it cannot find one.

## 3.11.15

- "Show in folder" again: on a real PC the folder still opened behind NetMerge. The window is now also lifted to the top of the stack and the system is asked to switch to it, and the log records whether the Explorer window was found.

## 3.11.14

- "Show in folder": the first fix was not enough, because Windows does not let a background program take the focus. NetMerge now joins the input of the window that has the focus for a moment, so the folder comes to the front.

## 3.11.13

- Downloads: "Show in folder" now brings the Explorer window to the front. Before, the window opened behind the NetMerge window and only the taskbar button flashed.

## 3.11.12

- "Start with Windows" can no longer be switched on from a copy of NetMerge that was started with its own data folder (`-root`, used for tests and development). The start-up task is one per computer, and such a copy would have replaced the real installation's task with one that starts the wrong folder.
- Overview: the "Only one connection is active" message was shown twice (a banner and the status line). Only the status line is left.

## 3.11.11

- NetMerge.exe now carries Windows file details (product name, version, description, licence), so the file's Properties window and some security tools show what it is. This is a step toward signing the program.
- Housekeeping: a stray Linux build file that had been committed to the repository by mistake was removed, and a draft code signing policy was added under `docs/`.

## 3.11.10

From the Windows retest of 3.11.9:
- "Show in folder" now asks Windows itself to open the folder with the file selected. In the sandbox the old way (starting `explorer.exe /select`) opened Home instead when NetMerge ran as administrator. If Windows cannot do it, the old way is the fallback. Not covered by an automatic test (it needs a Windows desktop), so it needs a check on Windows.

## 3.11.9

Found by running the download manager on Windows with merging on:
- A download from this computer itself (a `127.0.0.1` address, for example a local test server or a dev site) failed with "the requested address is not valid in its context" while merging was on, because the connection was tied to a network line's address. A target on this computer now ignores the line, as it should.

## 3.11.8

Fixes from running the download-manager test plan against 3.11.7:
- Taking a file off a torrent that is running (or paused) now deletes that file and any folder it leaves empty. Before, a half-filled file of the full size stayed behind and looked finished.
- A private torrent no longer offers peer exchange to the peers it talks to (it already ignored the peers they sent).
- A link from the browser that leads into your own network is refused before the browser's download is cancelled. This includes an ordinary-looking name that resolves to an inside address: before, the browser download was cancelled first and NetMerge refused afterwards, so the file was lost.
- The file preview of a `.torrent` is refused too when torrents are switched off in the download settings.
- Torrents: a log line now records when a peer is banned for sending pieces that fail the hash check.
- New tests: the upload limit holds, a peer that sends three bad pieces is banned, deselecting drops the file, a private torrent offers no peer exchange, local-network links are refused up front, a failed browser download is restarted, and a randomised stop-damage-resume stress test for torrents.

## 3.11.7

- Testing in a virtual machine: the new start option `-allow-virtual` (same as the variable `NM_ALLOW_VIRTUAL=1`) now survives the automatic restart as administrator. Windows does not pass environment variables to a program started as administrator, so the variable alone was lost and the virtual adapter was ignored again ("No internet connection found"). Without the option nothing changes.

## 3.11.6

- Fixed: a torrent could finish as "done" with wrong file content after it was paused or closed while a file was edited or deleted. Resuming trusted the saved list of finished pieces after hashing only a sample of them, so damage the sample missed was never noticed (found by the independent Windows test, reproduced with a test, then fixed). NetMerge now saves each file's size and modification time with that list; on resume every file that differs, including one that was deleted and made again, has its finished pieces hashed again and the damaged ones are fetched again. Older saved lists are checked once in full.

## 3.11.5

- For testing inside a virtual machine such as Windows Sandbox: starting NetMerge with the environment variable `NM_ALLOW_VIRTUAL=1` makes it treat virtual adapters (for example Hyper-V) as real connections. Without it nothing changes: virtual adapters are still ignored, so merging and downloads work only over real lines.

## 3.11.4

Fixes from an independent review of the download manager's code:
- A link pattern with huge numbers (for example `file[9223372036854775807-9223372036854775807].bin`) made the pattern expansion count past the biggest number, wrap around and never stop, using more and more memory. Numbers that are too large are now refused and counting is bounded. Reproduced with a test, then fixed.
- Torrents whose files would land in the same place are refused before anything is written: the same path twice, a file that is also a folder, names that differ only in capital letters, or names that become equal once forbidden letters are replaced.
- Importing a plain list of links now accepts `ftp://` and magnet lines too (not only http and https).
- Browser extension: when a link is already in the list, the browser download is no longer just cancelled. A paused or failed copy is restarted; a finished copy (you asked for the file again) becomes a new download.
- Exported lists keep links, names, folders, queues, mirrors and hashes; they do not keep file selection, connection counts, quality or speed limits.

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
