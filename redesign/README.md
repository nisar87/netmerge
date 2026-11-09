# NetMerge frontend redesign

Design and implementation package · 9 November 2026 · source baseline `39987fa` (3.11.4)

**Direction selected by the user:** calm control center with light and dark themes. Crisp typography, slate surfaces, restrained blue accents, readable connection health, and purposeful motion.

**Branch:** `design/frontend-redesign-plan`. **Implementation:** the redesigned application is in `ui/`, with the browser popup in `extension/src/popup.*`. The backend, extension interception logic and release configuration are unchanged. The original design preview remains a separate reference with illustrative data.

Latest refinement: [focused speed meter, throughput graph and expandable results](evidence/speed-test.md). The earlier [compact, context-colored download actions](evidence/compact-download-actions.md) remain included.

## Start here

1. Open [the visual preview](preview/index.html) in a browser. Switch between Overview, Downloads, and Settings and between light/dark themes. This is a design reference, not the implemented application.
2. Read [the current application audit](01-current-application-audit.md) and [the design brief and navigation](02-design-brief-and-navigation.md).
3. Use [the design system](design-system/MASTER.md), [screen specifications](03-screen-specifications.md), and [interaction specification](04-interaction-motion-accessibility.md) together.
4. Read [the implementation ledger](IMPLEMENTATION.md) for actual architecture decisions and [the implementation verification](evidence/implementation-verification.md) for executed checks and limitations. Run the real frontend with [the development guide](../frontend/README.md). The numbered implementation plan remains the original design proposal; the ledger records how it was executed.

## Package map

| Artifact | Purpose |
|---|---|
| `01-current-application-audit.md` | Evidence, strengths, problems, limitations of observation |
| `02-design-brief-and-navigation.md` | Product goals, chosen approach, navigation and workflow mappings |
| `design-system/MASTER.md` / `tokens.css` | Typography, colors, dimensions, icons, reusable components |
| `03-screen-specifications.md` | Every existing screen, secondary flow, and feature destination |
| `04-interaction-motion-accessibility.md` | Interaction states, animation, keyboard and accessibility behavior |
| `05-frontend-architecture-and-contracts.md` | Existing API boundaries, state handling, file plan |
| `06-implementation-plan.md` | Ordered, independently verifiable frontend tasks |
| `07-validation-and-rollout.md` | Coverage matrix, acceptance gates, packaging, rollback |
| `research/ui-ux-pro-max.md` | Skill findings, accepted/rejected guidance, primary references |
| `evidence/current/` | 18 screenshots and machine-readable inspection results |
| `preview/` | Self-contained, responsive visual design reference |
| `evidence/verification.md` | Checks actually run on this planning delivery |
| `IMPLEMENTATION.md` | Completed implementation ledger and architecture decisions |
| `evidence/implementation-verification.md` | Executed implementation checks, review resolutions and platform limits |
| `evidence/implemented/` | Actual frontend screenshots, 276-case audit, browser suite and embedded-app smoke results |

## Boundaries and decisions

- Preserve all existing features, request bodies, backend defaults, state meanings, and safety confirmations.
- The implementation changes `ui/` and the extension popup presentation only. Go files, `dm/`, extension interception logic, manifests, release files, and backend tests are frozen.
- Keep vanilla JavaScript, semantic HTML, and CSS. Use the current Go embedding and existing same-origin API. No framework migration, runtime CDN, server route, or API work is required.
- Use locally vendored Lucide SVG icons, system fonts, CSS transitions, existing chart data, and the existing QR encoder. Do not add a motion library solely for these transitions.
- Frontend development/test tooling lives in `frontend/`; it is excluded from the embedded runtime.
- The user explicitly authorized full implementation after selecting the design direction.
- Windows engine integration, real network operations, live downloads, QR pairing, and extension interception require a final smoke test on a disposable Windows test installation. Linux screenshot inspection cannot establish their correctness.

## Definition of the intended result

A person can tell whether merging is working, understand the role of each connection, start or recover a download, and find the next relevant action without scanning technical settings. Existing expert workflows remain available through predictable tabs and detail panels. Both themes, narrow windows, keyboard operation, reduced motion, and backend-unavailable states receive the same level of design attention as the normal desktop state.
