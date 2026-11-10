# Implementation ledger

Base: `2868dbe`. Authorized 9 November 2026: implement the complete reviewed frontend redesign. Backend and existing contracts remain frozen.

## Run manifest

S2: broad frontend integration with full-object settings and live async state. Parent owns shared foundation, shell, remaining app screens, integration and final verification. Three isolated local checkouts under `/tmp` own disjoint Downloads, popup and fixture/test work. Inherited model/effort (AUTO) per the developer's delegation rule; no model overrides or nested delegation. Independent review follows integration. No push, release or backend edits.

## Decisions

- Keep existing proven feature event handlers and contracts while extracting them from inline HTML into explicit frontend controllers. Use native ES modules for reusable state/transport/polling functions and load the controller boundary deliberately. This preserves advanced workflows during the visual/IA migration; there is no second legacy UI entry or parallel polling owner in the final app.
- Continue on the user's requested `design/frontend-redesign-plan` branch. Workers use independent clones; only reviewed allowlisted patches are integrated.
- Real Windows network operations and Narrator cannot be established by Linux browser fixtures. Complete all available implementation/build/test checks and record that acceptance limit accurately.

## Progress

- [x] Shared API/state/polling contracts and focused tests
- [x] Responsive shell, design system, routing, dialogs, icons
- [x] Overview, Connections, Speed test and Presets
- [x] Downloads and advanced flows (isolated worker)
- [x] Apps, Usage, Protection, DNS and Activity
- [x] Sharing, Settings, Setup and extension popup
- [x] Integration, accessible states and regression verification
- [x] Independent review and final evidence, delivered in the implementation commit

## Implemented architecture and deliberate adjustments

- The single production entry is `ui/index.html`. Shared transport, immutable field deltas, serialized settings transactions and scheduling are native ES modules; established feature controllers remain classic scripts loaded in an explicit order. This avoids an unnecessary framework/build-system migration while removing the monolithic inline controller.
- Connections use stable cards with expandable preferences rather than a separate drawer. Overview remains a compact summary; every connection action and advanced role/weight setting stays available on its dedicated page. Cards retain existing input nodes when adapters appear or disappear.
- Chart inspection supports pointer and keyboard input, a current-rate table and pausing visual updates. The separate 24-hour history retains the existing backend samples and units.
- Downloads uses separate Transfers, Queues, Browser and Settings tabs, a modal add flow, grouped settings and details, and 100-row pages. Advanced actions, torrent/stream options, queues, browser pairing, file operations and exact existing payloads are preserved.
- Runtime assets are local. Development tools are pinned; no package installation or Node process is needed by the shipped application.
- Existing decimal KB/MB/GB presentation is preserved. Download bandwidth limit selectors retain the existing 1,048,576-byte-per-unit contract. Live connection rates convert bytes/second to Mbps; historical samples already use Mbps.

## Review resolutions

Independent reviewers examined the parent shell/settings/state controllers and feature controllers, in addition to the isolated implementation tests. Reproductions and regression tests cover pending settings reversals, late state snapshots, adapter discovery during a rename, failed setup saves, restore ordering, popup focus restoration, and advanced feature drafts. Review findings are corrected before final verification. Actual test counts and evidence are recorded separately in `evidence/implementation-verification.md`.
