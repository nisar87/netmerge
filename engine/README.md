# NetMerge's network engine

The engine that does NetMerge's work underneath the window: it captures the PC's traffic, sends each connection
out on the line NetMerge chose, applies the app rules and block lists, and serves the sharing ports for phones. It
does exactly what NetMerge needs and nothing else. It is a separate Go module (`netmerge/engine`, standard library
only) that shares no code with the app. NetMerge.exe carries it and runs it as `NetMerge.exe -engine -d <config
folder>` (`cmd/nmengine` builds it on its own, for tests).

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
| Capturing all traffic of the PC (virtual adapter, routes, DNS) | `tun_e2e_linux_test.go` (CI, root) and `tun_e2e_windows_test.go` (CI, a Windows machine with Wintun) |
| Every setting NetMerge can write is a key the engine reads, and the engine accepts it | `engine_compat_test.go` |

CI runs the contract against the engine on its own and as NetMerge runs it (`NetMerge -engine`) on every change.

## How it works

- **Capturing traffic**: Wintun (Windows) or /dev/net/tun (Linux), set up with the system's own network functions.
  The system's TCP does the heavy work: connections are rewritten to a local listener (no user-space TCP stack),
  checksums are updated for the changed words only. UDP is relayed in sessions. Lost DNS answers are asked again.
- **Choosing the line**: the rules in NetMerge's order, then the group NetMerge set; a line that does not connect
  in time (about a second) while another one does is passed over for a few seconds. A website that is down on
  every line blames no line.
- **Names and programs**: website names from TLS, HTTP and QUIC; the program of each connection from the system's
  socket tables, looked up while the connection's first bytes are awaited.
- **Block lists**: kept as 64-bit fingerprints (a 350,000-name list takes about 8 MB and loads in under 0.1 s).

## Benchmark

`cmd/enginebench` measures the engine the way NetMerge uses it: start time, new connections (HTTP, HTTPS, UDP),
downloads, block accuracy, a line that silently stops answering, memory and CPU. The workflow "Engine benchmark"
runs it for every change to the engine, side by side with the engine on main, on a Windows machine (internet)
and on a private test network on Linux (`-local`: its own web, HTTPS, UDP and DNS servers, a 350,000-name block
list).

## Built to grow (not used yet)

- "Where a connection goes out" is one interface, so something besides "this adapter" could be added later.
- The line for a new connection can be chosen inside the engine (instead of NetMerge switching slots from outside).
- Traffic arriving from the local network is handled like the PC's own.

## Rules for this code

- Written from scratch for NetMerge. Only Go's own library, plus Wintun (a prebuilt DLL licensed for
  redistribution; `scripts/get-wintun.py` fetches it and checks its fingerprint).
- Light: only what the contract needs.
