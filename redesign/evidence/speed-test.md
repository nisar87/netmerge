# Focused speed test

Follow-up to `1df8774`, implemented 9 November 2026 from the user's request for one prominent meter, a graph and collapsible detail.

The page centers a large download-speed dial with an adaptive, labeled Mbps scale. The dial animates to received readings and respects reduced motion. The phase, overall progress and single start/retest action stay together. The completed reading explicitly identifies its scope: best through NetMerge, all connections together, or the best individually measured connection. Unknown readings use a dash; an actual zero remains zero.

The throughput graph records accepted state readings once, including while another page is open. It uses elapsed observation time, breaks lines between test stages or missing polling intervals, and keeps at most 600 samples for the current test. Redraws and failed polls cannot duplicate data. New runs, replaced results and cleared test state discard the old timeline. Reloading does not invent a historical graph; saved backend results remain available in Test details.

The graph has independent pause/resume, keyboard sample inspection, a peak reading and a table alternative. Pausing the graph does not stop the actual test or the meter. Rapid numerical updates are excluded from live announcements.

Test details is a native keyboard-operable disclosure containing single-stream and eight-stream connection comparisons, public IPs, connection errors, combined capacity, parallel-stream scaling, share recommendations and recorded samples. Polling preserves the open disclosure and keyboard focus. Warnings remain visible outside the collapsed section. The existing `speedtest` and `applyweights` actions still use empty request bodies; pending actions prevent duplicate submissions. A failed start retains the previous result and provides an inline retry explanation.

The user's development screenshot caught an intermediate edit with a missing `observeSpeedTest` function. The finalized feature defines it before the application's initial render. A fresh read-only browser load of the existing port 4173 confirmed zero JavaScript errors, no false connection-loss banner and the completed result displayed.

## Verification

- 140 integrated Playwright tests passed on the final implementation, including 12 speed-test cases covering sampling, navigation, missing/zero values, completed-result scope, keyboard disclosure, exact share payloads, graph pause/resume, start failures, reconnection/reset handling, and accessible light/dark layouts. Browser output is saved below.
- 21 unit/contract tests passed; the optional standalone Downloads browser test was skipped in this command. Downloads remains covered by the integrated browser suite.
- 36 visual/accessibility cases passed: idle, live and expanded details in light/dark at 375, 390, 768, 1024, 1440 and 1920 pixels. Zero document overflow, axe violations or uncaught browser errors.
- Linux and Windows amd64 builds passed with the final assets embedded.
- JavaScript syntax, whitespace and frontend-scope checks passed. The scope guard confirmed all changes since the original baseline remain within the frontend and design package.

Evidence: [light live meter](speed-test/live-light-1440.png), [dark live meter](speed-test/live-dark-1440.png), [desktop details](speed-test/details-light-1440.png), [narrow details](speed-test/details-dark-390.png), [idle state](speed-test/idle-light-1440.png), [36-case audit](speed-test/audit.json), [140 browser tests](speed-test/browser-tests.txt), and [unit output](speed-test/unit-tests.txt). Screenshots and test data are synthetic.

The existing development process remains on port 4173. Browser regressions use port 4187 and the visual audit uses a separate ephemeral server; neither resets the user's development fixture data. Go source, backend tests, API handlers, schemas and release configuration are unchanged. The existing [Windows validation limits](implementation-verification.md#boundaries-and-remaining-platform-acceptance) still apply; cross-compilation and fixture tests do not establish real network speed or Windows engine behavior.
