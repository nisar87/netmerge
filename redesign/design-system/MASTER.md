# NetMerge design system

Status: proposed specification based on the user-selected calm control-center direction. Applies to all app views and extension popup. `tokens.css` is the machine-readable color/spacing reference; screen-specific exceptions require a documented reason.

## Visual principles

Clarity before decoration. Structure before cards. Stable data before animation. Make the most useful action obvious. A thin border separates regions; elevation is reserved for menus and overlays. Use one quiet converging-lines brand mark, retaining recognition of the existing NetMerge logo.

## Theme and color

| Role | Light | Dark | Usage |
|---|---|---|---|
| Canvas | `#F5F7FB` | `#0B1120` | App background |
| Surface | `#FFFFFF` | `#111B2E` | Sidebar, panels, tables |
| Raised surface | `#F0F4FA` | `#19263C` | Hover, settings sections |
| Primary text | `#17243B` | `#E6EDF7` | Titles and body |
| Secondary text | `#53647C` | `#A9B8CD` | Descriptions, chart axes |
| Quiet separator | `#DCE3EE` | `#2A3B55` | Decorative dividers only |
| Control boundary | `#71839C` | `#71839C` | Inputs and essential outlines |
| Action | `#1D4ED8` | `#93B4FF` | Links, primary controls, focus |
| On action | `#FFFFFF` | `#0B1120` | Primary button label |
| Action tint | `#EAF0FF` | `#1C2F51` | Active navigation |
| Success | `#166534` | `#86EFAC` | Confirmed healthy/running |
| Warning | `#92400E` | `#FCD34D` | Actionable degraded/standby context |
| Error | `#B91C1C` | `#FCA5A5` | Failures and destructive actions |

Do not use separator tokens as the only visible boundary of an input. Every status has text and an icon; green is not a brand accent. Chart series use stable link identity, not current sort order: blue, teal, violet, amber, rose, plus distinct dashes/markers. Retain the adapter's identity if its display label changes. Never imply the first five colors limit supported adapters.

Persist system/light/dark in the existing `nm-theme` preference key, confirmed in `ui/index.html`; support existing values `auto`, `light`, `dark` through an adapter. Follow OS changes only in Auto. Apply initial theme before first paint. Never persist server-side appearance settings that do not exist.

## Typography and spacing

- UI: `"Segoe UI Variable Text", "Segoe UI", system-ui, sans-serif`. Use the local platform font; no font download is required.
- Numeric values: same family with `font-variant-numeric: tabular-nums`. Code, paths and logs: `"Cascadia Mono", Consolas, ui-monospace, monospace`.
- Page heading 28/36, weight 650; section 18/26, weight 600; primary metric 36/44, weight 600; body 15/24; form controls 15/22; secondary 14/21; small metadata 12/18 (never essential instructions). At narrow widths, inputs use 16 px.
- Spacing steps: 4, 8, 12, 16, 20, 24, 32, 40, 48. Page gutter 32 desktop / 24 compact / 16 narrow. Panel padding 24 or 16. Default grid gap 24.
- Radius: 6 px compact, 10 px control, 14 px panel, 18 px modal. Pills only for statuses/counts, not every container.
- Desktop sidebar 224 px; compact rail 72 px; header min-height 72 px; content max-width 1440 px. Content still grows vertically; the sidebar alone may scroll on short windows.
- Controls 40 px minimum visual height; comfortable/touch targets 44 px. Compact table icon buttons 32 px with at least 8 px separation. The project targets exceed the 24 px WCAG AA minimum; do not label 44 px as an AA requirement.
- Layers: base 0, sticky header 10, popover 30, overlay 50, toast 60. Native modal dialogs use the top layer; render their notices inside the dialog so a toast does not disappear behind it.

## Icon system

Use a pinned, locally vendored Lucide SVG subset. 24-unit viewBox, 1.75–2 stroke, round caps/joins, 20 px navigation, 16–18 px inline, 24 px feature/empty-state. Verify canonical names at implementation and retain the package license. Do not replace the brand with a generic network icon.

| Meaning | Suggested Lucide name |
|---|---|
| Overview / Connections / Downloads | `LayoutDashboard` / `Network` / `Download` |
| Apps / Usage / Protection / Sharing | `AppWindow` / `ChartNoAxesColumn` / `ShieldCheck` / `Share2` |
| Activity / Settings | `Activity` / `Settings2` |
| Ethernet / Wi-Fi / mobile | `Cable` / `Wifi` / `Smartphone` |
| Start / pause / retry / open folder | `Play` / `Pause` / `RotateCcw` / `FolderOpen` |
| Healthy / warning / error / waiting | `CircleCheck` / `TriangleAlert` / `CircleAlert` / `Clock3` |
| Theme / browser / DNS / speed test | `Sun` or `Moon` / `Globe` / `Globe2` / `Gauge` |

Icons next to text are decorative (`aria-hidden="true"`). Icon buttons have a contextual accessible name and a keyboard-accessible tooltip, for example “Pause Ubuntu ISO”, not “Pause”. Do not use emoji as product controls or status symbols.

## Component specifications

| Component | Anatomy and behavior |
|---|---|
| App shell | Navigation, header title, persistent actual merge status, single main region, skip link |
| Status badge | Icon + complete label; subtle background; no pulse; unknown distinct from unhealthy |
| Button | Primary/secondary/quiet/destructive; stable width; pending label; disabled reason discoverable |
| Merge control | Native button/switch semantics; starting/stopping pending; actual backend result drives state |
| Setting row | Visible label, one concise explanation, control; restart consequence adjacent; error and saved status inline |
| Field | Label, optional hint, control, reserved error slot; preserve entered value on failure |
| Metric | Name, tabular value, explicit unit and scope; unknown renders “—”; never zero for missing telemetry |
| Connection row | Icon, label, status, current rate, contribution; Details action; identity metadata inside drawer |
| Transfer row | Checkbox, file/type, progress/status, size, speed, ETA if derivable, contextual actions; 72–88 px normal height |
| Data table | Semantic caption/headers, keyboard-operable sort with `aria-sort`; sortable headers are buttons |
| Drawer | 420 px desktop; label + close + scrollable body; full-width narrow; saves remain visible without obscuring focus |
| Dialog | Native `dialog` preferred; 480/640 px max-width; heading/body/actions; inert background; Escape/cancel; restore focus |
| Banner | Severity icon, clear explanation, relevant recovery action; one top-priority global banner; page-specific errors stay local |
| Toast | Confirms a completed low-risk action, 4–6 s; does not replace inline errors; no telemetry announcements |
| Empty state | Small icon, direct heading, one reason, one useful action; filtered-empty provides Clear filters |
| Chart | Stable axes/units, stable series colors, current summary, data table alternative, pause visual update option |

All components specify default, hover, pressed, keyboard focus, disabled, pending, success and failure where relevant. Avoid reflow between these states. A disabled control must not be the only place its explanation is available.

## Assets and performance

No photographic hero, decorative background image, external webfont, or large illustration is necessary for this utility. SVG icons and CSS are sufficient. Use CSS opacity/transform for overlays, not layout animation on every telemetry tick. Production assets stay under `ui/` so the existing Go embed captures them. Keep test fixtures, scripts, and screenshots outside `ui/`.
