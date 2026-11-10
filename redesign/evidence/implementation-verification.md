# Frontend implementation verification

Executed 9 November 2026 on `design/frontend-redesign-plan`. Runtime baseline: `39987fa48c25bf2eb29a700a44abf916174105bb` (3.11.4); design-plan commit: `2868dbe1d90cfae9da93d8ca64828c0fd658fa0c`.

The implemented application uses the selected calm control-center direction: coordinated light/dark/system themes, local Lucide icons, responsive navigation, restrained motion, accessible dialogs, and separate destinations for everyday and advanced workflows. The production entry remains `ui/index.html`, embedded by the existing Go build. Development dependencies and fixtures are isolated in `frontend/`.

## Executed checks

| Check | Result |
|---|---|
| Frontend contract/unit tests | 21 passed; optional browser test skipped by default and executed separately below |
| Isolated Downloads browser regression | 1 passed |
| Fixture-server tests | 13 passed |
| Complete Playwright suite | **118 passed**, no retries |
| Route/theme/viewport audit | **276 passed**: 23 routes × 2 themes × 6 widths |
| Actual Go-embedded application smoke | **20 routes passed**, using an isolated temporary Linux profile |
| Existing extension core tests | 8 passed; test and interception source unchanged |
| Existing backend tests | `go test ./...` passed for `netmerge` and `netmerge/dm`; initial uncached run and final cached rerun |
| Go static analysis | `go vet ./...` passed |
| Linux and Windows amd64 builds | Both passed with the final embedded UI |
| JavaScript parsing | 48 production/development scripts and modules passed `node --check` |
| Scope and whitespace | Frontend/design allowlist passed; `git diff --check` passed |

The Playwright total includes 35 shell/navigation cases, 28 feature/contract workflows, 20 dialog/theme cases, 15 popup cases, 14 feature-state regressions, and 6 shared-state/restore/setup regressions. Download coverage includes opt-in loading, 1,000-row pagination, filtering and selection, probe/add flows, transfer controls, advanced options, queues, configuration merging, browser pairing, and retained drafts.

The route audit covers widths **375, 390, 768, 1024, 1440, and 1920 pixels**. Every case had zero document overflow and zero axe findings for the selected WCAG 2 A/AA and 2.1 AA rules. Across the audit there were zero uncaught browser errors and zero external requests. The 20 dialog cases also had zero selected axe findings and verified focus within native modal dialogs at 390 pixels. These are automated checks, not a claim of complete manual accessibility certification.

The actual Go application served the final embedded assets successfully. Its 20-route read-only smoke had zero browser errors, overflow, or selected axe findings. The temporary application was stopped after verification.

## Review findings resolved

- Serialize complete-settings writes and merge only explicit edits into fresh server state. Preserve rapid reversals, later edits, newly discovered adapters, and unrelated fields; reject stale in-flight state responses.
- Keep failed settings saves visible and retryable. Order restore after earlier writes, prevent concurrent edits during restore, and keep first-run setup open after a failed save.
- Preserve DNS provider-dependent controls, custom addresses, pending edits and unsubmitted domains through testing, saving and polling.
- Preserve sharing rename identity, text, selection and errors when addresses change; retain newer edits during a pending rename.
- Preserve newer block/allow list edits and preset names/schedules; serialize explicit schedule changes and ignore obsolete reads.
- Prevent late log responses from replacing the selected source. Keep update release notes open and keyboard focus stable during progress updates.
- Restore popup focus after failed actions without stealing focus moved elsewhere. Handle permission/storage delays and failures visibly.
- Distinguish zero contributors, a single contributor, and adapter traffic while merging is off. Keep byte/Mbps conversions and existing bandwidth-limit units intact.

Independent reviewers inspected shared state, shell and feature controllers. Reproductions were turned into focused regressions before the final integrated suite ran.

## Reproduce

Environment used: Node `v24.19.0`, Go `go1.26.5 linux/amd64`, Playwright `1.65.0`, axe Playwright integration `4.14.0`, and its installed Chromium. Runtime assets require no Node process or external downloads. The browser override below was necessary for this environment's preinstalled Chromium location; a normal Playwright installation can omit it.

```sh
npm ci --prefix frontend
npm --prefix frontend run test:unit
npm --prefix frontend run test:fixtures
node extension/core.test.js

export NETMERGE_BROWSER_EXECUTABLE=/tmp/netmerge-ui-review/browsers/chromium-1254/chrome-linux64/chrome
npm --prefix frontend run test:e2e
npm --prefix frontend run audit

PLAYWRIGHT_MODULE="$PWD/frontend/node_modules/playwright/index.mjs" \
PLAYWRIGHT_BROWSERS_PATH=/tmp/netmerge-ui-review/browsers \
node frontend/tests/contract/downloads-browser.test.mjs

npm --prefix frontend run check:scope
git diff --check
GOCACHE=/tmp/netmerge-go-cache go test ./...
GOCACHE=/tmp/netmerge-go-cache go vet ./...
GOCACHE=/tmp/netmerge-go-cache go build -o /tmp/netmerge-ui-observation/netmerge-final .
GOOS=windows GOARCH=amd64 GOCACHE=/tmp/netmerge-go-cache \
go build -o /tmp/netmerge-redesign-windows.exe .
```

The deterministic fixture API serves the real frontend and models existing request/response contracts. Synthetic credentials, addresses, files and OS outcomes are documented in [the fixture guide](../../frontend/tests/fixtures/README.md). It does not execute real network, download, device or power actions. Popup tests execute the actual HTML/CSS/JavaScript with browser extension APIs mocked.

## Evidence

- [Complete browser test output](implemented/browser-tests.txt)
- [Audit output](implemented/route-audit.txt) and [all 276 machine-readable cases](implemented/audit.json)
- [Actual embedded-application smoke results](implemented/embedded-app-audit.json)
- [Light Overview](implemented/light-1440-overview.png) and [Dark Overview](implemented/dark-1440-overview.png)
- [Light Connections](implemented/light-1440-connections.png) and [Dark Connections](implemented/dark-1440-connections.png)
- [Light Downloads](implemented/light-1440-downloads-transfers.png) and [Dark Downloads](implemented/dark-1440-downloads-transfers.png)
- [Light download settings](implemented/light-1440-downloads-settings.png) and [Dark download settings](implemented/dark-1440-downloads-settings.png)
- [Light Appearance](implemented/light-1440-settings-appearance.png) and [Dark Appearance](implemented/dark-1440-settings-appearance.png)
- [Narrow light Overview](implemented/light-390-overview.png) and [Narrow dark Overview](implemented/dark-390-overview.png)
- [Narrow light Downloads](implemented/light-390-downloads-transfers.png) and [Narrow dark Downloads](implemented/dark-390-downloads-transfers.png)

All 14 committed implementation screenshots use synthetic fixture data. They show the actual production frontend, distinct from the earlier static design preview.

## Boundaries and remaining platform acceptance

All changed files are under `ui/`, `frontend/`, `redesign/`, or the three `extension/src/popup.*` files. Go source, backend tests, `dm/`, Go dependencies, QR encoder, extension interception/background code, manifests, and build/release configuration are unchanged from the runtime baseline. The source-scope script also checks untracked files, excluding the pre-existing user-provided `.agents/` directory.

Native Windows WebView2, Narrator, actual adapter merging/failover, real downloads and file operations, phone QR pairing, installed-extension interception, and OS shutdown/update actions were not executed in this Linux environment. The Windows cross-build proves compilation and embedding, not those runtime outcomes. They remain the Windows acceptance checklist in [validation and rollout](../07-validation-and-rollout.md). No release, push, or merge was performed.
