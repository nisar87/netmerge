# UI/UX Pro Max research and decisions

Read skill: `.agents/skills/ui-ux-pro-max/SKILL.md`. Searches run locally on 9 November 2026. The skill is part of the pre-existing untracked `.agents/` directory; it is not included in this branch's commit.

## Queries and evaluated results

| Query | Verified result | Decision |
|---|---|---|
| `network monitoring desktop utility dashboard professional calm --design-system -p NetMerge --density 7` | Operations landing pattern, glassmorphism, slate/green, Fira typography | Product match partly relevant; landing/trial CTA, glass effects and remote font import unsuitable for embedded utility. Did not persist as authoritative design system. |
| `desktop utility control panel minimal --domain style -n 2` | `minimalism-and-swiss-style`; functional grid, restrained effects, light/dark | Accept clarity, grid, hierarchy and lightweight effects; use modest radii for Windows utility context. |
| `keyboard focus modal --domain ux -n 3` | Focus states; focus-not-obscured minimum/enhanced | Accept; distinguish AA and AAA correctly; project chooses full focus visibility. |
| `real time trend comparison --domain chart -n 2` | Time-series line and real-time streaming recommendations | Accept time-axis clarity, named series, sample table and pause. Keep existing canvas/SVG rather than automatically adding chart libraries/WebGL. |
| `motion reduced animation feedback --domain ux -n 2` | Reduced motion and excessive motion | Accept; discrete updates and small purposeful transitions. |
| `responsive semantic keyboard --stack html-tailwind -n 2` | Semantic color tokens and focus-visible | Nearest available HTML stack, not the project's actual Tailwind dependency. Apply framework-neutral advice in plain CSS; do not adopt Tailwind or version claims. |

The curated `design-system/MASTER.md` is the product-specific design authority. No unverified generated landing-page instructions or font imports were copied into production files.

## Primary references checked

URLs are recorded as source identifiers for later verification, not runtime dependencies:

- W3C WCAG 2.2 recommendation: `https://www.w3.org/TR/WCAG22/` — accessibility target and success criteria.
- W3C Target Size Minimum: `https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum` — 24 CSS px AA requirement with exceptions; project chooses larger targets.
- W3C changes in WCAG 2.2: `https://www.w3.org/WAI/standards-guidelines/wcag/new-in-22/` — focus visibility and target-size distinctions.
- Lucide official accessibility guide source: `https://github.com/lucide-icons/lucide/blob/main/docs/how-to/accessibility.md` — decorative icons and accessible names.
- Playwright official visual comparisons: `https://playwright.dev/docs/test-snapshots` — future deterministic browser screenshots.

These references inform proposed requirements. They do not certify the existing app or preview. License files and actual icon identifiers must be verified when vendoring the implementation's pinned icon subset.

## Explicit product choices

The user selected the calm control-center light/dark direction. System fonts, blue action color, nine grouped destinations, vanilla modules, proposed tokens and preview composition are design recommendations made in this package. They are not represented as separately approved user decisions.
