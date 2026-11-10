# Interaction, motion and accessibility

## Common state model

| State | Presentation | Actions |
|---|---|---|
| First load | Static skeleton matching final dimensions; meaningful loading text | Navigation remains available; mutation controls wait for required state |
| Empty | Specific reason and helpful next step | Add, enable, connect or clear filters, according to context |
| Refreshing | Retain previous content and mark refresh status quietly | Preserve focus, selection, scroll and drafts |
| Saving | Disable only the submitted operation; stable “Saving…” label | Cancel only before request dispatch; do not claim cancellation after dispatch |
| Saved | Confirm actual response and refreshed authoritative state | Brief confirmation; clear relevant dirty fields |
| Invalid input | Error beside field, associated with `aria-describedby` | Retain input; focus first invalid field after submit |
| Request failed | Local persistent error with server text safely rendered | Retry explicitly; never replay side-effecting POST automatically |
| Stale | “Last updated …” on retained data; missing values are not zero | Offer retry; disable actions requiring current server state |
| Lost connection | Persistent global connection banner after 3 state failures | Preserve navigation/drafts; reconnect GET only |

Track elapsed time as well as failure counts: label state stale if no successful state response for 5 seconds while visible. A slow request cannot keep a fake Live badge forever. Show busy operations after 150 ms to avoid flicker, but button press feedback is immediate. After reconnect, refresh all currently relevant sources. If app version changes, offer/perform the existing reload behavior; preserve non-sensitive drafts in memory until the user decides, and do not automatically serialize passwords/headers to storage. For pending edits, explain that an update requires reload and allow copying non-sensitive input first.

The live-state model is presentation-only. “Pause chart” freezes visual chart sampling, never merging, downloads, state polling or shutdown warnings. A paused chart labels its time explicitly. Reduced motion keeps current numeric telemetry visible without interpolating it.

## Motion specification

| Interaction | Motion | Reduced-motion equivalent |
|---|---|---|
| Hover/press/focus | 120 ms color/background; no position shift | Immediate state |
| Route content | 180 ms opacity, at most 4 px translation once | Immediate render and heading focus |
| Drawer | 240 ms opacity/16 px translate, ease-out | Immediate show/hide |
| Modal | 180 ms opacity and scale .985→1; exit 120 ms | Immediate show/hide |
| Switch | 120 ms thumb transform only after accepted local action; pending visible | Immediate change once confirmed |
| Known transfer progress | At most 180 ms linear transform of fill | Immediate progress value |
| Newly added transfer | One 180 ms background highlight | Static inserted row |
| Toast | 180 ms opacity/8 px translate | Static notice |
| Indeterminate loading | Small spinner only on active operation | Static icon + “Working…” text |
| Live chart | Redraw at incoming data cadence; no path morphing or animated digits | Discrete redraw; pause still available |

Use `prefers-reduced-motion` for CSS **and** JavaScript animation; cancel active animation when it changes. No background loops, decorative glows, shaking errors, skeleton shimmer, confetti, scroll hijacking, or bouncing status dots. Interrupted transitions settle in the newest state. Do not animate table row position during active use. CPU work and animation stop for hidden pages; explicit operation feedback continues to update when visible again.

## Keyboard and semantics

- Skip link → main; one h1 per route; semantic landmarks, labels and tables. Nav links expose `aria-current="page"`.
- Buttons perform actions; links navigate. Toggles use native checkboxes or switch semantics with accessible name and checked state. Tabs follow tablist/tabpanel keyboard conventions; plain links need no simulated tab semantics.
- Modal: initial focus to useful field or heading, background inert, Tab/Shift+Tab contained, Escape dismisses when safe, named close button, focus restored to opener or nearest surviving action. Nested dialogs avoided. Unsaved multi-field edits use an explicit discard confirmation.
- Keep all focused content visible when sticky headers/footers exist; use scroll padding and real-space layout. The project targets full visibility, stronger than the WCAG AA minimum of not being entirely obscured.
- All pointer-only affordances get keyboard and single-pointer alternatives. Add/paste/file chooser remain alongside drag/drop. Reordering uses Up/Down buttons with contextual names.
- Do not override Ctrl+L, browser Back, OS shortcuts, screen-reader shortcuts, or input selection. Optional `/` search shortcut works only outside editable fields and has a visible search control.
- A single polite live region announces completed actions and meaningful status changes. Errors may use alert semantics once. Do not announce per-second speeds, each progress tick, or recurring banners.

## Charts, color and data

Target WCAG 2.2 AA: normal text 4.5:1, large text 3:1, meaningful control/graphic boundaries 3:1; color never carries meaning alone. Validate real foreground/background combinations in both themes, including tinted states and focus. Disabled controls are exempt in the standard, but remain readable by project choice.

Charts include a title, time window, unit, direct legend, current textual summary and “View data” table. Series differ by line style/marker as well as color. Pointer hover and keyboard sample selection expose equivalent values. Keep missing intervals as gaps; no interpolated uptime or zero-filled unknown samples. Show sample time and “No reading” where appropriate. History is already Mbps, whereas live counters are bytes/s; test the different conversions.

Render numeric percentage only from known totals. Network rates use Mbps (`bytesPerSecond × 8 / 1e6`); the existing shared size/rate helper uses decimal KB/MB/GB scaling. Download bandwidth selectors retain their existing 1048576-based values with MB/s labels. Preserve this existing distinction for parity; a future SI/IEC label change must update every label, selector and conversion test together. All API limits remain bytes/s. Percentages and speed limits must not silently change units during redesign. (The helper scaling was verified and corrected during implementation.)

## Responsive and content stress

| Width/context | Behavior |
|---|---|
| 1440–1920 | Full sidebar, generous page gutter, charts + secondary context in two columns |
| 1200–1439 | Full sidebar, adapt panel columns to content |
| 768–1199 | Icon rail or expanded overlay; primary content uses available width |
| 375–767 | Header menu; single column; cards replace wide transfer summaries; drawer fills width |
| 320 CSS px equivalent/200–400% zoom | Reflow navigation and forms; allow a labeled table scroll region, no page overflow |

Test 100+ character filenames, Windows paths with spaces/non-ASCII, very long hostnames, four-plus adapters, 0 and huge counters, 1000 transfers, slow responses, null lists, denied clipboard, empty results and simultaneous operations. Use `min-width:0` and `overflow-wrap:anywhere` for technical tokens. Full filename/path can be expanded or copied via keyboard. Never use hover-only truncation disclosure.

## Microcopy examples

- “Merging is off. Your connections are still available separately.”
- “Waiting for a connection. This download will continue when one is available.”
- “Using your backup connection. Your other connections are unavailable.”
- “Changes weren’t saved. Your previous settings are still shown.” Only use this after re-fetch confirms no change; a failed POST may have partially applied.
- “Couldn’t confirm this change. Refreshing the current settings…” for ambiguous request outcomes.
- “Remove this unfinished download? Its partial file will be deleted.”
- “This change briefly restarts merging.” beside DNS/engine changes.

Keep statuses calm, factual and actionable. No “100% faster”, “secure connection” derived only from DNS encryption, or claims that every single stream combines lines.
