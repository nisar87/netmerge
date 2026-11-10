# NetMerge Frontend Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the calm, accessible NetMerge control center with complete existing functionality and unchanged backend contracts.

**Architecture:** Incrementally replace the embedded vanilla frontend using shared API/state/lifecycle primitives and feature modules. Keep all serving, engine, persistence, download and extension-interception behavior in the existing backend. Port one feature at a time, retaining a temporary legacy bridge until parity is proven.

**Tech Stack:** Semantic HTML, CSS variables, native JavaScript modules, locally vendored Lucide SVGs, existing QR encoder, Node built-in tests and Playwright/axe for frontend verification. Pin development tools at implementation time; do not assume a version from this plan.

**Spec:** Read [navigation](02-design-brief-and-navigation.md), [design system](design-system/MASTER.md), [screens](03-screen-specifications.md), [interactions](04-interaction-motion-accessibility.md), [contracts](05-frontend-architecture-and-contracts.md), and [validation](07-validation-and-rollout.md) before implementation.

## Global constraints

- Change production frontend only: `ui/**`, popup presentation files; tooling under `frontend/**`.
- No Go, `dm/`, API, persistence, engine, extension core/background/manifest, backend-test or release-file changes.
- Preserve feature parity, settings defaults, full-object write contracts and mutation confirmation semantics.
- Light/dark/Auto; 375 px through wide desktop; keyboard, reduced motion and offline asset loading.
- Downloads remains off until explicit opt-in; no unsolicited module/API load while disabled.
- No runtime CDN, webfont, framework, animation library or new production server.
- Use stable identity for live rows, separate confirmed data from drafts, and never automatically retry POST.
- Each task ends with focused verification and a scoped commit; do not publish or merge as part of implementing this plan without the applicable user authorization.

## Review focus

1. Poll arriving while a textarea/select/drawer is edited: values, cursor, selection and focus remain stable (Tasks 1, 4, 6, 7).
2. Full-object writes and inverse booleans: unrelated config survives and visible switches mean the correct thing (Tasks 1, 5, 7, 8).
3. Null/unknown/late/out-of-order data and backend restart: honest state, no duplicate actions or invalid success (Tasks 1, 3, 9).
4. Long filenames, 1000 jobs, many adapters, zoom and small windows: accessible actions and no page overflow (Tasks 2, 3, 4, 10).
5. Destructive download/file operations and sensitive sharing/pairing: exact consequences, deliberate requests, no leaked credentials (Tasks 4, 5, 8).

## Execution dependencies

```text
1 API/state fixture foundation
  → 2 shell/design primitives
    → 3 overview/connections/test/presets
      → 4 transfers/add/details
        → 5 advanced downloads/queues/browser
      → 6 apps/usage
      → 7 protection/DNS/activity
      → 8 sharing/settings/setup/popup
        → 9 final entry integration
          → 10 acceptance and Windows parity
```

Tasks 6–8 can proceed independently after shared interfaces stabilize, with disjoint file ownership. Tasks 4 and 5 share download state and should be sequential. Do not parallelize shared state/polling redesign. No calendar estimate is committed before a first slice is measured; milestones are defined by demonstrable behavior.

## Task 1: Freeze contracts and establish frontend state handling

**Files:** create `frontend/package.json`, lockfile, `playwright.config.js`, `scripts/serve.mjs`, `scripts/check-scope.mjs`, `tests/fixtures/{state,downloads,services}.json`, `tests/contract/{api,settings,format,polling}.test.mjs`; create `ui/app/{api,store,polling,format}.js`.

**Interfaces:** produces `request`, `subscribe`, `publish`, `commitSettings`, `startResource` and tested formatters described in the architecture document. Tests inject fetch and timers rather than accessing the actual engine.

- [ ] Capture sanitized example payloads and error bodies from existing source/development instance; include off, null arrays, missing adapter, partial download success and ambiguous POST failure.
- [ ] Define scripts: `test:unit` = `node --test tests/contract/*.test.mjs`; `test:e2e` = `playwright test`; `check:scope` = baseline diff validation. Fixture server serves `ui/` and exact fake `/api/` shapes on loopback, never proxies production writes.
- [ ] Write failing behavior tests: POST header/method; JSON/non-JSON errors; serialized full-settings saves; link identity merge; preserved unrelated fields; late GET ignored; one in-flight read; canceled read vs outstanding write; 8× bytes-to-Mbps conversion and history identity conversion.
- [ ] Run `npm --prefix frontend run test:unit` and confirm these tests fail for absent implementation, not missing dependencies.
- [ ] Implement the resource and transaction boundaries; no UI decision logic in API helpers.
- [ ] Rerun unit tests; inspect fixture output for credentials; commit only these frontend files.

**Gate:** a test that changes a label concurrently with a separate setting cannot erase role, weight, quota or rule data. A failed/uncertain save reconciles actual state rather than asserting rollback.

## Task 2: Build the shell and design primitives

**Files:** create `ui/redesign.html`, `ui/app/{main,router,theme}.js`, `ui/styles/{tokens,base,shell,components,motion}.css`, `ui/components/{icons,dialog,drawer,toast,field,status,table}.js`, `ui/assets/{icons.svg,NOTICE.txt}`; create `frontend/tests/e2e/shell.spec.js`.

**Interfaces:** consumes state/API foundation; produces `navigate`, `mountPage` contract, dialog/drawer primitives, `nm-theme` compatibility and route aliases.

- [ ] Write browser checks for hash refresh/back, old destination aliases, skip link, menu keyboard operation, modal return focus, theme persistence and restricted localStorage fallback.
- [ ] Implement selected tokens, licensed icon subset, navigation groups, title/merge-status header, rail/sheet responsive behavior and component states.
- [ ] Preserve a temporary adapter for legacy pages; assert only one polling owner is active.
- [ ] Run `npm --prefix frontend run test:e2e -- tests/e2e/shell.spec.js`; inspect screenshots at 375/768/1024/1440 in both themes and reduced motion.
- [ ] Verify the full focus path at 200% zoom; commit the independently reviewable shell.

**Gate:** no runtime external requests; no backend file diff; every existing destination remains reachable, even if temporarily legacy.

## Task 3: Migrate Overview and Connections

**Files:** create `ui/features/overview.js`, `ui/features/connections/{index,details,speed-test,presets}.js`, `ui/components/chart.js`; create `frontend/tests/e2e/connections.spec.js`, `tests/contract/connections.test.mjs`.

**Interfaces:** consumes state/test/preset/history resources and `commitSettings`; produces mounting functions and shared connection labels/colors keyed by adapter name.

- [ ] Write scenario checks for engine off, one/no/four-plus links, same provider, quota exclusion, standby, missing/unknown health, call protection and mixed live/history units.
- [ ] Implement overview hierarchy, chart/time-range/data-table and pause-visual-update affordance.
- [ ] Port connection edits, rescan/forget, mode, Classic weights, role and details; confirm consequences without adding network decisions.
- [ ] Port speed-test phases/results and apply-weights; port preset apply/restore/save/delete/schedule.
- [ ] Run unit suite and `test:e2e -- tests/e2e/connections.spec.js`; inspect stable focus while rates change and navigation during a test; commit.

**Gate:** charts reflect actual units and state; no enabled-but-standby link falsely counted as contributing; no invented cancellation or scheduling API.

## Task 4: Migrate transfers, add flow and details

**Files:** create `ui/features/downloads/{index,transfers,add,details}.js`; create `frontend/tests/e2e/downloads.spec.js`, `tests/contract/downloads.test.mjs`.

**Interfaces:** consumes optional download resource and request helper; `mountDownloads(root,context)` initializes only after enablement; exposes same item IDs and existing actions.

- [ ] Write fail-first scenarios: manager off produces no module/API requests; explicit enable; known/unknown-size progress; batch partial/duplicate; probe failure retains inputs; per-item actions; errors and retryAt; focus/selection survive polls; unfinished removal vs disk deletion.
- [ ] Build compact list/filter/search/sort/selection and pagination; preserve Ctrl+A/Delete/Escape and clipboard denial fallback.
- [ ] Build add/probe/confirm, batch, file chooser, torrent selection and stream options; retain all supported payload fields and defaults.
- [ ] Build details with parts/line stats, files, log, checksum and actionable recovery. Preserve executable-open confirmation and partial-file warning.
- [ ] Test 1000 fixture jobs, long paths, null metadata and rapid double-click; assert a single mutation and exact bodies. Run unit and focused browser suite, then commit.

**Gate:** every supported add kind and lifecycle action remains functional; visual updates do not steal focus; no fake transfer data in production.

## Task 5: Complete advanced downloads and browser management

**Files:** create `ui/features/downloads/{queues,browser,settings}.js`; extend `add.js`, `details.js`; create `frontend/tests/e2e/download-tools.spec.js` and `tests/contract/download-config.test.mjs`.

**Interfaces:** consumes confirmed download config; produces serialized full-config edits and existing download-action requests.

- [ ] Write tests for preserving queues/site rules on settings save; inverse upload/notification flags; per-line limits; midnight/day schedules; partial bulk failure; pairing expiry; revoke; countdown cancellation.
- [ ] Implement queue manager, folders, mirrors/relink, connection counts, link grabber, import/export and existing duplicate/overwrite behavior.
- [ ] Implement every settings group in the screen ledger, including after-command, ffmpeg status, mobile policy and learned-site reset.
- [ ] Implement guided browser pairing, global threshold/enablement and browser revocation; ensure codes are not persisted.
- [ ] Run focused unit/browser checks; inspect each secondary dialog at narrow width and in dark theme; commit.

**Gate:** tick every action in the download contract ledger and every settings row; preserve backend defaults and side effects.

## Task 6: Migrate Apps and Usage

**Files:** create `ui/features/{apps,usage}.js`; create `frontend/tests/e2e/apps-usage.spec.js`, `tests/contract/usage.test.mjs`.

**Interfaces:** consumes state/apps and usage resource; mutates only intended fields via `commitSettings`.

- [ ] Write tests for each app-rule target, stopped process, missing pinned adapter, duplicate rules, unlimited quota, reset days 1/28, warning/exclusion and unit/period formatting.
- [ ] Implement active/saved app views, existing suggestions, editing/deletion and default-rule explanation.
- [ ] Implement usage summary, actual available history/table, allowances, forecast caveat and exhaustion action.
- [ ] Verify text/select edits survive polls; inspect large counters, long names and empty periods; run suites and commit.

**Gate:** totals/periods match fixtures; settings edits preserve other app/connection data; no invented historical backend data.

## Task 7: Migrate Protection, DNS and Activity

**Files:** create `ui/features/{protection,blocking,dns,activity}.js`; create `frontend/tests/e2e/protection-activity.spec.js`.

**Interfaces:** consumes block/DNS/events/log resources and settings transaction layer; local tabs map old rules/block/dns/history/log routes.

- [ ] Write cases for routing chips, whitelist precedence, asynchronous pack update, dirty block text, DNS draft/test/fallback/restart, missing test results and log injection strings.
- [ ] Port routing and blocking with exact body shapes and inline failures.
- [ ] Port DNS selection, custom/private domains, test recommendation and explicit apply/reset.
- [ ] Port events filters and logs/source/refresh; preserve reading position and diagnostic export.
- [ ] Run focused suite, keyboard and chart/table checks; commit.

**Gate:** no “protected” badge while required merging is off; tests never change DNS without explicit application; logs render as text.

## Task 8: Migrate Sharing, Settings, Setup and popup presentation

**Files:** create `ui/features/{sharing,settings,setup}.js`; modify only `extension/src/popup.{html,css,js}` for presentation; create `frontend/tests/e2e/settings-sharing.spec.js`, `popup.spec.js`.

**Interfaces:** sharing uses existing QR functions and payloads; popup retains current runtime messages, storage keys, permissions request and DOM hooks.

- [ ] Write tests for sharing off/not-running/no address, device rename, password rotation, QR address selection, inverse preferences, restore confirmation, setup skip/start, engine download and update progress.
- [ ] Build sharing guide and manual details; preserve protocols and mask credentials without breaking copy/QR.
- [ ] Build settings groups and first-run guide. Show pending and partial/ambiguous errors correctly; no restart success claims before state confirms.
- [ ] Restyle popup and correct navigation instructions; exercise unreachable/off/pair/connected/media/permission/duplicate states with mocked extension APIs.
- [ ] Run focused browser suite and `node --test extension/core.test.js`; inspect QR readability and keyboard focus. Commit frontend files only.

**Gate:** no background/manifest/core diff; real extension integration remains a Windows validation item; no credential-containing screenshots.

## Task 9: Integrate final entry and remove temporary compatibility

**Files:** modify `ui/index.html`, `ui/app/main.js`; remove temporary `ui/redesign.html` and obsolete inline/legacy frontend logic only after parity; update relevant frontend suites.

**Interfaces:** final launch stays `/`; all modules use one shared resource manager.

- [ ] Review feature matrix against screenshots, source and completed tasks; resolve every unported item.
- [ ] Write integration cases for reconnect, changed app version with dirty draft, manager toggled while viewing Downloads, pending requests during route switch and restore followed by refresh.
- [ ] Switch the final entry; remove old timers, globals, duplicated styles and unused frontend assets. Retain `qr.js`.
- [ ] Run all frontend suites, extension core tests and diff allowlist check; build the app with the unchanged embed configuration.
- [ ] Inspect production-served assets with external networking blocked; commit migration cleanup.

**Gate:** no hidden legacy dependency or extra polling loop; all shipped assets embedded; no test data in `ui/`.

## Task 10: Acceptance, Windows parity and review evidence

**Files:** frontend test/fixture refinements as needed; append evidence under `redesign/evidence/`; no backend remediation.

- [ ] Run the validation matrix and capture before/after views with synthetic data in both themes.
- [ ] Run existing Go vet/test suite and Windows cross-build as non-regression checks, keeping generated files under `/tmp`. A pre-existing failure is reported with baseline evidence, not “fixed” in backend scope.
- [ ] Exercise real Windows engine on/off, tray close vs quit, startup, speed test, sharing, updater UI, download lifecycles and browser pairing on a disposable installation with test data.
- [ ] Inspect accessibility manually with keyboard and Windows Narrator; run axe, contrast, reduced-motion and responsive checks.
- [ ] Verify only allowlisted paths changed from the implementation base; record limitations and release decision. Commit evidence separately if useful.

**Gate:** all required checks pass or an explicit, documented product decision defers the affected feature. No claim of completed Windows validation based on Linux stubs.

## Milestones for review

1. Shell + overview + connections: approve overall visual hierarchy and primary status model.
2. Downloads + advanced tools: approve dense data presentation and recovery flows.
3. Remaining surfaces + popup: approve feature parity and cross-surface consistency.
4. Integrated Windows candidate: approve measured accessibility, performance, regression evidence and deployment readiness.

For this planning delivery, the next step is review of the concrete design package. The task checkboxes above intentionally remain unchecked; application implementation has not started.
