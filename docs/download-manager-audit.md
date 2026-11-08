# Download manager audit (after 3.10.1)

Scope: `dm/` (engine, manager, FTP, HLS/DASH, torrent), `downloads_host.go`, `ui/downloads*.js`, `extension/`.
Status marks: [ ] open, [x] done and tested, [-] decided not to do (reason given).

## A. Known issues (things that are wrong or risky today)

- [x] **K1 Torrent folder may be an existing folder.** With "replace" as the file-exists policy a torrent can be given the name of a folder that already exists; removing the torrent from the list would then delete that folder. Torrents must always get a fresh name.
- [x] **K2 Partial selection leaves empty files.** Files that were not chosen are still created (as empty or zero-filled files) in the torrent folder.
- [x] **K3 Resume trusts the saved list blindly.** If a file was changed or deleted while NetMerge was closed, the torrent believes pieces that are gone. Sample-check saved pieces at start and re-check everything when one is wrong.
- [x] **K4 Big lists sent every second.** The torrent file list is part of every refresh; a torrent with thousands of files makes the page and the saved list heavy.
- [x] **K5 Windows long paths.** Torrent file paths longer than 260 characters fail on Windows.
- [x] **K6 No "downloaded from the internet" mark on torrent files** (Windows SmartScreen), unlike other downloads.
- [x] **K11 Trackers are not told when a torrent is paused or removed** (stale peer entries on the tracker).
- [x] **K13 Audio-only DASH is saved as `.mp4`.** Should be `.m4a`.
- [x] **K15 Deleting a finished stream with "delete file" leaves the separate sound file behind** (when ffmpeg was not available).
- [x] **K18 Extension media list in private windows.** The page list must not record media of private windows.

## B. Missing features

- [x] **M1 Torrent upload settings.** Switch for uploading while downloading, an upload speed limit, and uploaded amount / ratio shown.
- [x] **M2 Switch for torrents.** A person who wants links but no BitTorrent can turn it off (magnet and .torrent are refused with a clear message).
- [x] **M3 Choose the files before a .torrent starts.** A dialog with the file list, sizes and folder, when a .torrent file is chosen.
- [x] **M4 Quality choice for HLS and DASH.** Pick the picture quality (and see the sizes) instead of always the best.
- [x] **M5 Link grabber finds magnet and .torrent links.**
- [x] **M6 Bulk actions.** Select several downloads: pause, resume, remove, move to a queue.
- [x] **M7 Speed graph** of the total download speed on the Downloads page.
- [x] **M8 Paste to add.** Pressing Ctrl+V on the Downloads page with a link on the clipboard fills the add box.
- [-] **M9 Seeding after a torrent finishes.** Without incoming connections seeding reaches almost nobody; it would give a false sense of "sharing". Not done until incoming connections exist.
- [-] **M10 Encrypted FTP (ftps://).** Cannot be verified here against real servers (many require TLS session reuse); a half-working version is worse than a clear message.
- [-] **M11 DASH with several periods, live streams, DRM.** Joining periods correctly needs a real remuxer; live and DRM are out of scope.
- [-] **M12 Incoming connections, MSE encryption, uTP for torrents.** Large, security-sensitive, and impossible to verify here.

## C. Interface improvements

- [x] **I1 Torrent details in Properties:** peers, seeds, uploaded, ratio, pieces, tracker count; progress shows uploaded and ratio on the row.
- [x] **I2 Narrow windows.** The Downloads page and the dialogs must work at phone-like widths (no sideways scrolling).
- [x] **I3 Settings page:** a Torrents block (switch, upload switch, upload limit) and a line saying whether ffmpeg was found (for joining picture and sound).
- [x] **I4 Keyboard:** Delete removes the selected downloads (with a question), Ctrl+A selects all in the list.

## D. Order of work

K1, K2, K3, K11, K13, K15, K4, K5, K6, K18 → M2, M1 → M5 → M4 → M3 → M6, M7, M8 → I1, I2, I3, I4 → full verification (Go race tests, Windows vet, extension tests, browser run at desktop and phone width).
