# Frontend development and verification

The shipped UI is in `ui/`, embedded by the existing Go build. No Node runtime, bundler, CDN, remote font or package download is needed to run NetMerge. `frontend/` contains development-only dependencies and deterministic fixtures; it is not embedded.

```sh
npm ci --prefix frontend
npm --prefix frontend run dev
```

Open the loopback address printed by the fixture server. All data and credentials are synthetic. Fixtures simulate network and operating-system outcomes without running those actions. See `tests/fixtures/README.md` for scenario and failure controls.

```sh
npm --prefix frontend run test:unit
node --test frontend/tests/fixtures/server.test.mjs
npm --prefix frontend run test:e2e
npm --prefix frontend run check:scope
node frontend/scripts/audit.mjs
```

Install Chromium with `cd frontend && npx playwright install chromium`, or set `NETMERGE_BROWSER_EXECUTABLE` to an installed Chromium for Playwright tests/audits.

The audit checks all routes in light and dark themes at 375, 390, 768, 1024, 1440 and 1920 pixels, records accessibility violations, document overflow, browser errors and unexpected external requests, and captures representative screenshots. Its output defaults to `/tmp/netmerge-frontend-audit`; set `NETMERGE_AUDIT_OUTPUT` to choose another directory. Audit data is synthetic and safe to share.

For the speed meter, observed throughput timeline and expandable results, run `node frontend/scripts/audit-speed-test.mjs`. It checks idle, live and expanded-result states in both themes at the same six widths. Screenshots and accessibility results default to `/tmp/netmerge-speedtest-review/audit`. The graph samples in this audit are synthetic API readings; the shipped UI only plots readings received from the existing state endpoint.

Runtime boundaries:

- `ui/app/boot.mjs` owns the API client, settings write queue, polling scheduler, icon helper and confirmation service.
- `ui/app/controller.js`, `navigation.js`, `state.js` and `start.js` own shared presentation state and the application shell.
- `ui/features/` preserves the existing feature endpoints and handlers in separate controllers.
- `ui/styles/` owns tokens, common controls and responsive application layout.
- `ui/assets/icons.svg` is a vendored Lucide subset; `NOTICE.txt` contains its license. Package versions are pinned in the development lockfile.

Keep backend files, `/api` handlers, persisted schemas and release/build configuration outside frontend changes. Whole-settings writes must use the serialized delta merge; do not POST a stale full settings snapshot. Mutations are never automatically retried. New periodic reads must use the shared scheduler and an active-page/visibility predicate.

Windows WebView2, Narrator and real adapter/device actions require a Windows smoke test. Linux browser fixtures and cross-compilation do not establish those results.

The download manager that used to be part of NetMerge is now a separate program, Pullwell
(https://github.com/nisar87/pullwell). NetMerge keeps only a note in Settings and `/api/legacy-downloads`, which saves
an earlier version's download list in the format Pullwell imports.
