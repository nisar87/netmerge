# nmengine — NetMerge's own network engine (in development)

NetMerge runs a network engine underneath it: today that is **mihomo** (downloaded at first start, see `enginedl.go`).
This folder will hold NetMerge's own engine, `nmengine.exe`: a small engine that does exactly what NetMerge needs and
nothing else. It is a separate Go module (`netmerge/engine`), so it shares no code with the app and cannot affect it.
The app keeps using mihomo until a switch in **Settings → Advanced** turns the new engine on, and falls back to
mihomo by itself if the new engine fails to start.

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
| Capturing all traffic of the PC (virtual adapter, routes, DNS) | Windows only: checked on the real PC in stage 2 |

The CI job `engine-contract` runs this against mihomo 1.19.33 on every change, so the contract always describes
what works today.

## Stages

0. **Contract** (this): the test above, passing against mihomo.
1. **Proxy-only engine**: config reading, rules, groups and line choice, binding to adapters, API, log, sharing
   ports (HTTP/SOCKS, Shadowsocks). Must pass the contract in CI. Can be tried on the PC beside mihomo by pointing
   one browser or a phone at its port.
2. **Capturing traffic**: Wintun adapter and routes, a user-space TCP/IP stack (gVisor), finding the program of a
   connection, website names from TLS/HTTP/QUIC, UDP, DNS (only for the custom DNS option). Tested on the real PC.
3. **The switch**: Settings → Advanced → Engine, with automatic fallback to mihomo; speed comparisons, a 24-hour run,
   sleep/wake and unplugging lines.
4. **Removing mihomo**, only when the new engine has been at least as good in daily use.

## Built to grow (not used yet)

- IPv6 throughout.
- "Where a connection goes out" is one interface, so something besides "this adapter" could be added later.
- The line for a new connection can be chosen inside the engine (instead of NetMerge switching slots from outside).
- Traffic arriving from the local network is handled like the PC's own.

## Rules for this code

- Written from scratch; no code copied from mihomo (GPL-3.0). Libraries only with permissive licences (Go's own,
  gVisor: Apache-2.0, Wintun: prebuilt DLL licensed for redistribution).
- Lighter than mihomo: only what the contract needs.
