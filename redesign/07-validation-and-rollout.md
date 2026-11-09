# Validation and rollout

## Planning delivery versus implementation

This branch supplies the observed baseline, design specification, tokens, visual reference and task plan. It does not replace the production frontend. Actual checks performed on this branch are recorded in [verification](evidence/verification.md); the matrix below is the acceptance requirement for future implementation.

## Feature and scenario matrix

| Area | Required scenarios | Verification |
|---|---|---|
| Shell | All routes/aliases; Back/Forward/refresh; desktop rail/menu; Auto/light/dark; storage unavailable | Browser + visual + keyboard |
| State | Loading, empty/null lists, slow/failed/out-of-order responses, reconnect, app restart, hidden tab | Deterministic fixture tests |
| Merge/connections | On/off/pending/error, one/no/many links, unhealthy/unknown/portal/standby/quota/same ISP, rename/role/weight/rescan/forget | Fixtures + Windows smoke |
| Speed/presets | Test stages/failures/results/share apply; built-in/custom/restore/schedule; midnight span | Request assertions + Windows smoke |
| Downloads lifecycle | Optional off, add/batch/duplicate/partial result, unknown size, queued/paused/done/error/retry, per-item/bulk, remove/delete/open | Fixtures + existing extension tests + Windows jobs |
| Download formats | HTTP/FTP, magnet/torrent selection, HLS/DASH with/without ffmpeg, unsupported format, checksum success/failure | Fixtures + existing test-plan scenarios |
| Advanced downloads | Every action in contract ledger, queues, folder rules, auth/header/mirror/relink, export/import, pattern expansion, limits, shutdown cancel | Request assertions + controlled local fixtures |
| Apps/usage | Every rule target, missing adapter, no active app, quota unlimited/80%/100%, reset and action | Unit + browser + Windows |
| Protection/DNS | Bypass/stable/VPN, packs/lists/allow, pending update, custom/encrypted/private domains, test/fallback/apply/reset | Browser + Windows |
| Sharing | Off, no engine, no address, public network, QR/manual/copy, rotation, device rename | Synthetic QR visuals + real isolated device |
| Activity/settings | Events/logs, setup, every toggle, restart consequence, backup/restore, engine/update progress, diagnostics, quit | Browser + Windows |
| Popup | Unreachable/off/unpaired/paired, expiry/error, site/type inputs, permissions, media and duplicate, disconnect | Mocked popup + Chrome/Edge/Firefox smoke |

Use `docs/download-manager-test-plan.md` and existing extension tests as regression inputs. Do not replace their coverage with snapshots of empty pages.

## Visual and accessibility matrix

Capture representative Overview, Connections details, populated Transfers, add/torrent dialog, Usage, DNS, Sharing, Settings, setup, popup, and failure states. Review light and dark at 375 × 812, 768 × 1024, 1024 × 768, 1440 × 900, and 1920 × 1080. Include 390 px to compare the baseline. Use deterministic data, time, font environment and reduced motion in screenshot tests.

Required manual checks: keyboard-only primary journeys; logical headings; labels/expanded/checked state; focus restore; Narrator status announcements; 200% and 400% reflow; forced-colors/high-contrast mode; denied clipboard; OS-theme change; reduced motion. Axe finds many semantic issues but does not establish full WCAG conformance. Contrast tests cover text, buttons, badges, chart series and focus against actual adjacent surfaces.

No document overflow at narrow widths; tables alone may scroll with an accessible label. Core controls remain available at 320 CSS px equivalent. Every modal fits available height and allows content scrolling without losing close/submit/focus visibility.

## Performance budgets (targets, not measured results)

- Shell usable within 500 ms after local asset response on the reference Windows test PC; record cold/warm separately.
- User interaction feedback under 100 ms; long network operation remains visibly pending.
- Initial shell JS + CSS target ≤200 KB uncompressed excluding deferred feature modules; total frontend target ≤600 KB including icons/QR. Record actual artifact sizes; deviations need a measured reason, not silent budget expansion.
- One active request per polling resource; no overlapping stale responses; no more frequent polling than the existing frontend.
- 1000-job fixture: render ≤100 rows per page; searching or changing tabs does not block input for >100 ms on the reference machine.
- No repeating animation in a hidden window; no layout shift for metrics/pending buttons; charts redraw only when visible data changes.
- With external requests blocked, all icons, fonts and navigation load. Network operations show their existing backend failures.

## Test command contract for future tooling

```bash
npm --prefix frontend ci
npm --prefix frontend run test:unit
npm --prefix frontend run test:e2e
npm --prefix frontend run check:scope -- --base <implementation-base>
node --test extension/core.test.js
GOCACHE=/tmp/netmerge-go-cache go vet ./...
GOCACHE=/tmp/netmerge-go-cache go test ./...
GOCACHE=/tmp/netmerge-go-cache GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w -H=windowsgui" -o /tmp/NetMerge-redesign.exe .
```

The `frontend/` commands are planned interfaces and do not exist yet on this planning branch. Implementation must define them before using this checklist. Scope checking accepts only the explicit allowlist; additions outside it require a scope decision, not auto-formatting the whole repository. Baseline backend failures, if any, are documented and compared without backend edits.

## Release and rollback

1. Keep frontend implementation on its dedicated branch; preserve the known-good base commit.
2. Inspect the complete diff, run acceptance checks, and test the embedded Windows candidate. Record version, commit, environment, before/after captures and unresolved limitations.
3. Remove temporary preview entry/legacy bridge before release. Planning assets remain in `redesign/`, outside Go's embedded `ui/` directory.
4. Use the existing release process only after release authorization. This plan does not change versions, publish artifacts, enable features, push to main or trigger updater deployment.
5. Roll back by reverting the frontend implementation commits/releasing the prior artifact through the existing process. The redesign introduces no persisted backend schema migration, so no data rollback is required. Keep any new local UI preferences optional and backward-compatible.

## Required evidence for “complete”

Feature mapping signed off; focused tests and full relevant regression checks pass; screenshots reviewed in both themes; keyboard/Narrator checks recorded; real Windows smoke evidence; exact allowlist diff; measured bundle/request behavior. List any unsupported or untested environment explicitly. Do not equate the static design preview, source audit or Linux stub screenshots with a production acceptance pass.
