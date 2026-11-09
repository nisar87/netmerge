# Design brief and navigation

## Product and audience

NetMerge is a local Windows utility for people combining multiple internet connections, including people who need dependable calls, metered backup links, and fast downloads. The interface must support a quick health check and occasional configuration, while giving experienced users efficient access to routing, queues, DNS and diagnostics.

Lead with four questions: **Is merging on? Which connections are contributing? Is anything waiting or failing? What can I do next?**

## Chosen direction

The user selected a calm control center with light and dark themes. Use slate neutrals, restrained blue for actions, labeled semantic status, native-feeling typography, and a precise grid. A small converging-lines motif in the brand and overview expresses what NetMerge does. Avoid decorative glass, neon, large marketing headers, animated counters, or constant pulsing charts.

Three visual approaches were considered: the selected balanced control center; a dark, dense technical console (strong for experts but less approachable); and a spacious bright minimal UI (clear but inefficient for transfers and connection comparisons). Both themes are first-class in the selected approach.

Implementation approach: reorganize and modularize the existing vanilla frontend behind unchanged contracts. A cosmetic skin would leave information architecture and interaction problems intact. A React/framework rewrite would add migration and packaging work without being necessary for the desired design. Keep the current hosting, language, and QR encoder.

## Navigation model

Persistent desktop sidebar: 224 px. Two labeled groups, plus utility links. Every item has a consistent 20 px SVG icon and visible text. Downloads stays discoverable while off; opening it shows an explicit enablement explanation and makes **no download API call or module load** until the person opts in.

| Group | Destination | Local navigation |
|---|---|---|
| Monitor | Overview | Live / Last 24 hours chart range |
| Monitor | Connections | Connections · Speed test · Presets |
| Monitor | Downloads | Transfers · Queues · Browser · Settings |
| Configure | Apps | Active apps · Saved rules |
| Configure | Usage | Usage · Allowances |
| Configure | Protection | Routing · Blocking · DNS |
| Configure | Sharing | Connect a device · Connected devices |
| Utilities | Activity | Events · Logs |
| Utilities | Settings | General · Appearance · Updates & backup · Advanced |

At 768–1199 px use a 72 px icon rail with accessible names and focus/hover tooltips; an Expand navigation control remains available. Below 768 px use a labeled menu button and modal navigation sheet with the full list. No horizontal navigation strip. Local tabs wrap when practical or become a labeled native select if four labels cannot fit. Preserve selected tab and filter state within the session.

Use hash navigation (`#/connections/speed-test`) because the existing server need not support application history fallback. Browser Back and Forward must work; refresh returns to the same destination. Route changes update the title and focus the page heading. Query-like fragments encode useful view filters but never URLs with credentials, tokens, passwords or pairing codes.

## Existing-to-new mapping (feature preservation gate)

| Existing `data-p` | Proposed route | Required retained surface |
|---|---|---|
| `overview` | `#/overview` | Aggregate traffic, connections, mode, 24-hour history, recent events, download summary |
| connection controls inside overview | `#/connections` and connection drawer | Enable, rename, weights, role, health, allowance, forget missing link, rescan |
| `downloads` | `#/downloads/transfers` | Entire transfer lifecycle; subflows enumerated in screen specification |
| `presets` | `#/connections/presets` | Built-in/custom profiles, restore own settings, schedule |
| `apps` | `#/apps/active` | Active process traffic, rules, suggestions, bypass and pinning |
| `test` | `#/connections/speed-test` | Per-line, combined, through-engine, scaling and recommended shares |
| `usage` | `#/usage` | Monthly/daily usage, forecasts, quotas, reset day, exhaustion action |
| `share` | `#/sharing/connect` | Setup, QR, platform/address selection, manual credentials, rotation, device naming |
| `rules` | `#/protection/routing` | Address bypass, stable domains, VPN connection |
| `block` | `#/protection/blocking` | Packs, counts, top domains, allow action, custom block/allow lists |
| `dns` | `#/protection/dns` | Provider/custom DNS, encryption, private domains, tests, reset, fallback |
| `history` | `#/activity/events` | Everything/problems/changes filters and refresh |
| `log` | `#/activity/logs` | App/engine/speed logs, refresh and diagnostics shortcut |
| `settings` | `#/settings/general` | Every existing setting, updates, backup/restore, guide, quit |

Any existing internal `go('test')`-style entry points are mapped to aliases during migration, then removed only when all callers are migrated. There is no known existing URL deep-link contract; new hashes are a frontend enhancement.

## Primary journeys

1. **First launch:** recognize app → see engine and connection readiness → choose existing launch options → start merging or skip → overview confirms actual state. Downloads remains off by default.
2. **Daily check:** overview → inspect contributing connections and any warnings → open affected connection → take the existing corrective action → see confirmed result.
3. **Add a download:** Transfers → Add download → paste links or select torrent → inspect available metadata → choose defaults or expand options → Download now / Add to queue → monitor row → open folder or recover failure.
4. **Recover a download:** Needs attention → details → readable error plus existing retry/relink/mirror actions → submitting → confirmed job state. Preserve partial progress semantics.
5. **Protect a call:** observe existing call-protection status → inspect the chosen connection and downloads hold explanation → change existing preferences deliberately. Do not add unsupported call controls.
6. **Manage limited data:** Usage → Allowances → edit per-link amount and existing reset/action → review forecast and warning states → connections identify any quota-excluded link.
7. **Connect a browser/device:** dedicated guided surface → existing one-use pairing or QR → confirmed connection → reachable disconnect/password rotation. Browser pairing and phone sharing remain distinct flows.

## Layout sketches

```text
Desktop overview
┌──────────────┬─────────────────────────────────────────────────────┐
│ NetMerge     │ Overview                         [Merging on   ●]   │
│              ├─────────────────────────────────────────────────────┤
│ Monitor      │ Two connections working together     [Manage]      │
│  Overview    │ Download     Upload       Connections     Uptime    │
│  Connections │ 126.4 Mbps   18.2 Mbps    2 contributing   2h 14m    │
│  Downloads   │                                                     │
│ Configure    │ Traffic                        [Live | Last 24 h]   │
│  Apps        │ ───────── named connection series ───────────────   │
│  Usage       │ [Pause chart] [View data]                           │
│  Protection  │                                                     │
│  Sharing     │ Connections                       Recent activity   │
│              │ Ethernet  Healthy  84.2 Mbps        Call protected   │
│ Activity     │ Wi-Fi     Healthy  42.2 Mbps        Wi-Fi recovered  │
│ Settings     │ Optional downloads summary                          │
└──────────────┴─────────────────────────────────────────────────────┘

Narrow window
┌──────────────────────────────┐
│ [Menu] NetMerge   [Merging on]│
│ Overview                     │
│ Download      Upload         │
│ Traffic chart + data access  │
│ Connections (stacked rows)   │
│ Current downloads            │
│ Recent activity             │
└──────────────────────────────┘
```

## Success criteria

- Default overview places merge state, current traffic, connections, and the most important warning within the first 900 px of a 1440 px-wide window.
- Add download and its recovery details each take at most two navigation actions from Transfers; all advanced controls remain reachable.
- Every old capability maps to a tested new surface. No unsupported backend feature appears as a functioning control.
- No document-level overflow at 375 px; a dense table may scroll within a labeled region. At 200% zoom, menus and sticky controls never cover the focused element.
- Both themes pass the defined contrast pairs and keyboard checks; animation is purposeful and removable.
- Production app can load and navigate with external internet unavailable. Existing backend operations still report their own connectivity requirements.
