# Current application audit

## What was inspected

Source baseline: `39987fa`; repository version 3.11.4. Read `README.md`, all frontend entry points, popup UI, configuration types, and the API handlers that the frontend calls. No backend source was edited.

Built the unchanged Go application into `/tmp/netmerge-ui-observation/netmerge` and ran it with `-root /tmp/netmerge-ui-observation -background`. All generated settings/logs stayed in that disposable root. Automatic merging and update checks were disabled; an inert engine-file placeholder prevented the development app downloading the Windows engine. No real merging or speed test was started. The download manager was enabled only in this disposable instance to inspect its screens.

Observed the actual existing frontend with Chromium, at 1440 × 1000 and 390 × 844. Linux adapters and counters are development stubs from `sys_other.go`, not production measurements. The screenshot script visited all 13 destinations, first-run setup, expanded download settings, and Add with options. It recorded **zero page JavaScript errors**. See [inspection.json](evidence/current/inspection.json).

| Evidence | What it establishes |
|---|---|
| [Overview, light](evidence/current/overview-light.png) | Current hierarchy, live chart, connection cards, sidebar and status repetition |
| [Overview, dark](evidence/current/overview-dark.png) | Existing dark tokens and chart treatment |
| [Narrow overview](evidence/current/overview-narrow.png) | 390 px page fits; navigation scroll area is 1,678 px wide |
| [Downloads](evidence/current/downloads-light.png) | Add controls, filter/toolbar density and empty state |
| [Download settings](evidence/current/downloads-settings-light.png) | Long configuration block beneath task content |
| [Add options](evidence/current/download-options-light.png) | Advanced download input surface |
| [Settings](evidence/current/settings-light.png) | Everyday, maintenance, and advanced settings presentation |
| [Setup](evidence/current/setup-light.png) | First-run checklist and optional choices |

The remaining named screenshots document Apps, Speed test, Usage, Share, Protection, Blocking, DNS, Presets, History, and Logs. Populated download rows, successful sharing, pairing, and backend failure states were inspected in source, not exercised against real services. The popup was inspected in source, not loaded as a real browser extension. Distinguish these evidence levels in later reviews.

## Existing frontend structure

| Source | Responsibilities |
|---|---|
| `ui/index.html` | Main markup, theme tokens, layouts, chart drawing, page routing, state polling, settings, setup, most screens |
| `ui/downloads.js` | Download view, add/probe, row actions, pairing, torrent preview, drag/drop, bulk operations |
| `ui/downloads-more.js` | Options, properties, queues, folder rules, mirrors, link grabber, import/export |
| `ui/downloads.css` | Download layout and visual styles |
| `ui/qr.js` | QR encoder, to retain |
| `extension/src/popup.html`, `.css`, `.js` | Pairing, enablement, exclusions, file types, permissions, detected media |
| `api.go` | Embeds `ui/` recursively; same-origin serving and request guards; read-only integration reference |

No existing Node frontend build or frontend browser test setup was found. The browser extension has an existing Node test suite. The Go app serves port 9196 and enforces that host; a redesign must work within this existing serving model.

## Strengths to carry forward

- Light/dark/system appearance, readable system fonts, stable numeric formatting, meaningful SVG navigation icons.
- Product-specific language that explains merging limits, metered links, call protection, and optional downloads.
- Rich connection states: quality, health, captive portal, standby, allowance, same-provider warnings.
- Working first-run guide, diagnostics, settings backup, update states, and recovery actions.
- Download dialogs already implement focus trapping, inert background, Escape and focus restoration. Do not replace these with less capable overlays.
- Reduced-motion CSS and some hidden-window throttling already exist. The redesign should complete these behaviors, not claim to introduce them from zero.
- Downloads are independently optional. Appearance work must retain this separation.

## Findings and design response

| Priority | Observation/evidence | User impact | Design response |
|---|---|---|---|
| P1 | 13 peer-level destinations, including three protection-related pages and two logs/history pages | The app's structure is harder to learn than the workflows require | Group destinations into task areas with local tabs; preserve old destination aliases |
| P1 | Narrow navigation scroll width 1,678 px at 390 px viewport | Most navigation choices are out of view | Labeled menu button opens a complete navigation sheet |
| P1 | Download settings occupy a long, dense panel below the list | Everyday controls compete with configuration; settings are hard to revisit | Separate Transfers, Queues, Browser, and Settings tabs |
| P1 | Repeated `innerHTML` replacement and manual focus restoration in live views | Source-level risk to typing, selection and open menus during polling | Keyed updates and separate server state/form drafts; explicitly test focus preservation |
| P1 | `/api/settings` accepts a full settings object, not a generic patch | A careless UI rewrite can clear unrelated settings | Serialize writes and compose complete payloads from fresh confirmed settings |
| P2 | Overview puts adapter details, weights, roles and spark charts in every card | Monitoring and configuration compete; two cards leave unused width | Compact connection summary rows; detailed controls in Connections and a detail panel |
| P2 | Engine-off development screenshot still shows combined adapter traffic | “Combined” can be confused with traffic actually merged | Label off-state counters “Adapter traffic”; show merging status separately |
| P2 | Many 12–13 px supporting labels and subdued metadata | Long sessions and high-DPI Windows use need stronger hierarchy | Standard 14 px secondary text, 15–16 px body, measured contrast pairs |
| P2 | Protection, DNS, presets, queues and download options use different setting patterns | Save/apply behavior and discovery vary | Shared setting rows, drafts, pending indicators, consistent detail panels |
| P2 | Some destructive operations use native `confirm()`; setup has less complete focus handling than downloads | Dialogs and keyboard behavior are inconsistent | One accessible confirmation/dialog primitive retaining the precise consequences |
| P2 | Canvas tooltips are pointer-oriented; chart context is mostly visual | Data needs a nonvisual route and pause affordance | Stable summaries, named series, sample table, keyboard exploration, pause visual updates |
| P3 | Mixed bespoke icons, emoji on Share's empty state, simple square popup mark | Product surfaces feel less cohesive | One vendored SVG set and a preserved/redrawn NetMerge mark |

These are design findings, not claims of backend defects. No backend remediation is included.

## Observation limitations

The Linux stubs do not prove Windows tray behavior, administrator prompts, engine startup, network isolation, actual speed, downloads, device discovery, or extension handoff. No successful protected-site download, torrent, HLS/DASH job, real update, or restore was performed. Those behaviors are mapped from existing code and must be covered by later fixtures and Windows smoke checks. The current screenshot set is a baseline, not a comprehensive accessibility certification.
