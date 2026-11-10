# Compact download actions

Follow-up to `3fa9e55`, implemented 9 November 2026 from the user's screenshot feedback.

Common transfer, selection, queue, browser-disconnect, pagination and ordering controls now use compact Lucide icons. Pause is amber, resume/start is green, retry/open/edit is blue, removal/disconnection is red, and secondary controls are neutral. Finished downloads use an open-file symbol. Status text, selection counts, queue names, primary actions and destructive confirmations keep their labels.

The selection bar groups related controls, uses a quieter surface, and wraps into two readable rows on narrow screens. The queue selector starts at “Move to queue…” and resets after moving a selection. Buttons expose accessible names, hover/focus hints, visible focus styling and 44-pixel touch targets on narrow layouts. Escape dismisses a hint before the existing selection shortcut acts. Hints follow confirmed live state changes without moving focus.

The shared icon helper normalizes numbered names such as `Trash2` and `Settings2`, and the local sprite contains every new symbol. Backend handlers, API payloads, selection scope and confirmation behavior are unchanged.

Validation:

- **128 Playwright tests passed**, including 10 new compact-control cases. Existing transfer/queue/browser actions and all download dialogs remain covered.
- The new cases cover keyboard/hover hints, Escape, focus through live updates, page selection, queue payloads, removal cancellation and local SVG resolution. Selected-state axe checks passed in light/dark at 375, 768 and 1440 pixels with zero overflow or selected WCAG findings.
- **21 unit/contract tests and the optional standalone Downloads browser regression passed.**
- Linux and Windows amd64 builds, JavaScript parsing, whitespace and frontend-scope checks passed.
- Inspected actual desktop and narrow screenshots. The existing development server remains on port 4173; tests used separate servers and did not reset the user's development data.

Evidence: [full browser output](compact-download-actions/browser-tests.txt), [dark desktop](compact-download-actions/dark-1440-selected.png), [light desktop](compact-download-actions/light-1440-selected.png), [narrow layout](compact-download-actions/dark-390-selected.png), and [keyboard hint](compact-download-actions/dark-1440-tooltip.png). All screenshots contain synthetic fixture data.

Run the browser checks with `npm --prefix frontend run test:e2e`; use `NETMERGE_FIXTURE_PORT=4187` while the development server is running. The existing [platform validation limits](implementation-verification.md#boundaries-and-remaining-platform-acceptance) still apply.
