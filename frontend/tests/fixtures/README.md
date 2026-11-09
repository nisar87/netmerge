# Deterministic frontend fixtures

Run `node frontend/scripts/serve.mjs` from any directory. It serves this checkout's actual `ui/` at `/` and `extension/` at `/extension/`. `NETMERGE_FIXTURE_PORT` (or `PORT`) overrides 4173; `NETMERGE_FIXTURE_HOST` accepts only loopback addresses. No Go process, external requests, disk settings writes, downloaded files, process launches, or OS actions are involved.

The fixtures mirror `api.go`, `settings.go`, `stats.go`, feature handlers, and `dm/dl_api.go` at 2868dbe. All addresses, domains, device names, and credentials are synthetic. Explicit `FAKE-TEST-ONLY` passwords and pairing tokens are never usable credentials. Dates are fixed at 2026-11-09T12:00:00Z. The default download manager is **off**.

Test control routes exist only in this development server. They require `X-NetMerge-Test: 1`, reject foreign Origins, and are never shipped inside `ui/`:

- `GET /__test/health`: readiness, no special header required.
- `POST /__test/reset` with `{scenario:"default"}`: replace all state and clear capture. Scenarios: `default`, `off`, `offline` (all API calls return 503), `null` (nullable Go slices/maps), `one-connection`, `empty`, `populated1000` (manager enabled).
- `POST /__test/scenario` with `{scenario:"offline"}`: enter a scenario without clearing capture. `default` recovers connectivity without discarding edits; other data scenarios replace state. Reset for a pristine baseline.
- `GET /__test/requests`: `{requests:[{sequence,method,path,query,body,headers,status}]}`; includes asset reads, excludes control routes. Selected contract headers only; no cookies or bearer credentials.
- `POST /__test/requests` with `{}`: clear capture only.
- `POST /__test/state` with `{state:{...},downloads:{...},block:{...},...}`: deep patch objects; arrays replace, `null` is literal. This models backend updates while a form is dirty.
- `POST /__test/fault` with `{path:"/api/settings",method:"POST",status:500,error:"Fixture save failed",times:1}`: next matching calls fail. `delayMs` delays responses; `body` overrides the response (including `null`). Omit `times` for persistent faults; `times:0` clears that path. Delayed GETs snapshot at request time, allowing stale-response tests. Mutations with delays execute before their delayed acknowledgement.

API POSTs require `X-NetMerge: 1`, JSON, and a same-origin Origin when present. Unsupported actions/paths fail rather than silently succeeding. Actions that normally trigger external work produce deterministic simulated results. Speed/DNS/update progress advances on reads, never wall-clock timers. Native downloads return sanitized JSON and a valid, deterministic diagnostics ZIP.

Verification:

```sh
node --test frontend/tests/fixtures/server.test.mjs
npm --prefix frontend run test:e2e
```

The parent owns Playwright/axe dependencies and package scripts. `frontend/playwright.config.js` starts a fresh server and uses one worker. `NETMERGE_FIXTURE_PORT=4183` selects another port for a concurrent suite. `NETMERGE_BROWSER_EXECUTABLE` can select an installed Chromium. Browser tests block external network requests, fix the browser date (without stopping timers), reset between cases, and attach captured requests on failure. They describe the new hash routes and intentionally fail against the old UI until integration. No snapshots of layout or CSS-class assertions are used.

Implementation sequence: capture faithful baseline objects; prove the server's state transitions and guards with Node HTTP tests; implement the server; add browser navigation, optional-loading, accessibility and exact mutation-contract tests; run available checks; commit only the assigned paths.
