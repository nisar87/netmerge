# NetMerge's own network engine (being tested)

NetMerge runs a network engine underneath it: today that is **mihomo** (downloaded at first start, see `enginedl.go`).
This folder holds NetMerge's own engine: a small engine that does exactly what NetMerge needs and nothing else. It
is a separate Go module (`netmerge/engine`, standard library only) that shares no code with the app. NetMerge.exe
carries it and runs it as `NetMerge.exe -engine -d <config folder>` (`cmd/nmengine` builds it on its own, for tests).
The app keeps using mihomo until **Settings → Advanced → Network engine** is set to "NetMerge engine (testing)",
and falls back to mihomo by itself if the new engine does not start.

## The contract

`engine_contract_test.go` (in the app's folder) is the definition of "an engine NetMerge can use". It builds
NetMerge's real configuration, starts an engine binary and checks:

| What NetMerge relies on | Checked |
|---|---|
| Started as `engine -d <config folder>`, reads `config.yaml` (JSON) and the block/allow files under `filters/` | yes |
| Local API with the secret (`Authorization: Bearer`): `GET /version` (401 without the secret) | yes |
| `GET /proxies`: lines `LINK1..n`, groups `FAST`, `STABLE`, `AUTO`, `CALL`, `SLOT1..n` with the line each points at (`now`) | yes |
| `PUT /proxies/SLOTn` and `PUT /proxies/CALL` with `{"name": "LINKx"}` → 204; unknown names refused | yes |
| `PUT /configs?force=true` with `{"path": ...}` reloads while running | yes |
| `GET /connections`: id, download/upload, chains (group … line), rule, metadata (process, type, source, destination, host) | yes |
| Rules in NetMerge's order: process, port, IP range, domain, keyword, block/allow lists (`AND`/`NOT`), default | yes (blocking, default route) |
| Each connection sent out on the adapter of the line it was given (`interface-name`) | yes (on loopback) |
| Refused connections logged as `[TCP] src(app) --> host:port match … using REJECT` at the `info` level, so the counter in `blocking.go` sees them | yes, with NetMerge's own log reader |
| Sharing: HTTP/SOCKS proxy port with password (407 without), Shadowsocks (aes-128-gcm) port | yes |
| Capturing all traffic of the PC (virtual adapter, routes, DNS) | own engine: `tun_e2e_linux_test.go` (CI, root) and `tun_e2e_windows_test.go` (CI, a Windows machine with Wintun) |

The CI job `engine-contract` runs this against mihomo 1.19.33 on every change, so the contract always describes
what works today.

## Stages

0. **Contract**: the test above, passing against mihomo. Done.
1. **Proxy part**: config, rules, groups and line choice, binding to adapters, API, log, sharing ports
   (HTTP/SOCKS, Shadowsocks). Passes the contract in CI. Done.
2. **Capturing traffic**: Wintun (Windows) or /dev/net/tun (Linux), routes and DNS, its own small TCP/IP handling
   (the system's TCP does the heavy work: connections are rewritten to a local listener, no user-space TCP stack),
   UDP sessions, the program of each connection, website names from TLS/HTTP/QUIC, fake-address DNS. Done; tested
   in CI on Linux and on a Windows machine, not yet on a real PC with several lines.
3. **The switch**: Settings → Advanced → Network engine, with automatic fallback to mihomo. Done (3.17.0).
   Still to do on the real PC: speed comparisons, a 24-hour run, sleep/wake and unplugging lines.
4. **Removing mihomo**, only when the new engine has been at least as good in daily use.

## Compared with mihomo

`cmd/enginebench` runs mihomo and this engine in turns with the same settings and measures start time, new
connections (HTTP, HTTPS, UDP), downloads, block accuracy, a line that silently stops answering, memory and CPU.
The workflow "Engine comparison" runs it on a Windows machine (internet) and on a private test network on Linux
(`-local`: its own web, HTTPS, UDP and DNS servers, a 350,000-name block list). Results are in each run's
summary; the engine is only changed when it stays ahead.

## Built to grow (not used yet)

- "Where a connection goes out" is one interface, so something besides "this adapter" could be added later.
- The line for a new connection can be chosen inside the engine (instead of NetMerge switching slots from outside).
- Traffic arriving from the local network is handled like the PC's own.

## Rules for this code

- Written from scratch; no code copied from mihomo (GPL-3.0). Only Go's own library, plus Wintun (a prebuilt DLL
  licensed for redistribution; `scripts/get-wintun.py` fetches it and checks its fingerprint).
- Lighter than mihomo: only what the contract needs.
