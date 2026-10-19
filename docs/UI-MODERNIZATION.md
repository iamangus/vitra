# Document-first workspace

## Scope and disposition

The approved layout direction is implemented across the application: restrained purple accents, local Inter/JetBrains Mono fonts, a quiet document canvas, labeled navigation, discoverable file actions, and matching light/dark states. No external font requests are needed.

| Area | Delivered |
| --- | --- |
| Foundation and shell | Shared tokens and controls, AA text contrast checks, skip link, integrated responsive header, mobile drawer with focus trapping and inert background |
| Vault navigation | Notes/Search/Graph/Activity labels, persistent find-note entry, visible row action buttons, accessible native HTML application dialogs, keyboard menu navigation, move destination picker, drag/drop retained |
| Home and empty states | Recent notes stored locally, vault overview, actionable first-note and missing-note states |
| Reader | Document title/type/tags/location, readable typography, incoming links, optional heading outline, copyable code blocks, keyboard-scrollable supported HTML tables |
| Editor | Raw Markdown/YAML unchanged, labeled Read/Edit/Split controls, saved/dirty/conflict feedback, retained draft recovery, explicit discard confirmation, no side-by-side panes on narrow workspaces |
| Search | Full-text context snippets, escaped text highlighting, semantic excerpts and section context, no uncalibrated similarity percentage, loading/empty/unavailable states |
| Activity | Date-grouped timeline, source links, empty-state format example and live refresh |
| Graph | Deferred 3D module, labeled fit/labels controls, type filters, keyboard-accessible note search, selection inspector and explicit Open note, list fallback when WebGL is unavailable |
| Safety and compatibility | Existing revision protection, autosave and draft manager retained; actual filenames used for destructive/rename/download operations, including literal `.md` note IDs; existing vault exclusions and MCP tools retained |

The only backend enhancement is an optional additive `snippet` field in full-text search results. It uses bounded Unicode-safe text around a match, omits frontmatter, and is displayed as text, never injected HTML. Existing `path` and `title` fields and search matching/order are unchanged. It is also additive for callers of the shared search result type.

Authentication, authenticated-proxy setup, collaboration, rich-text migration, backend redesign, merging, and deployment remain out of scope.

## Visual evidence

All images use disposable fixtures and local mock embeddings, never a real vault or provider.

- Before: [`main` desktop light](ui/baseline-desktop-light.png), [desktop dark](ui/baseline-desktop-dark.png), [phone light](ui/baseline-phone-light.png), [phone dark](ui/baseline-phone-dark.png), captured from a separate source archive of merged `main` (`b63616f`).
- After: [desktop light](../frontend/tests/e2e/design.spec.js-snapshots/reader-desktop-light-linux.png), [desktop dark](../frontend/tests/e2e/design.spec.js-snapshots/reader-desktop-dark-linux.png), [tablet light](../frontend/tests/e2e/design.spec.js-snapshots/reader-tablet-light-linux.png), [tablet dark](../frontend/tests/e2e/design.spec.js-snapshots/reader-tablet-dark-linux.png), [phone light](../frontend/tests/e2e/design.spec.js-snapshots/reader-phone-light-linux.png), [phone dark](../frontend/tests/e2e/design.spec.js-snapshots/reader-phone-dark-linux.png).
- [Split editor](../frontend/tests/e2e/design.spec.js-snapshots/split-desktop-linux.png), [outline](../frontend/tests/e2e/design.spec.js-snapshots/outline-desktop-linux.png), [home](../frontend/tests/e2e/design.spec.js-snapshots/home-desktop-linux.png), [search](../frontend/tests/e2e/design.spec.js-snapshots/search-desktop-linux.png).
- [Mobile navigation](../frontend/tests/e2e/design.spec.js-snapshots/navigation-phone-dark-linux.png), [rename dialog](../frontend/tests/e2e/design.spec.js-snapshots/rename-phone-dark-linux.png), [conflict](../frontend/tests/e2e/design.spec.js-snapshots/conflict-phone-linux.png), [recovered draft](../frontend/tests/e2e/design.spec.js-snapshots/recovered-phone-linux.png), [long title](../frontend/tests/e2e/design.spec.js-snapshots/long-title-phone-linux.png), [wide content](../frontend/tests/e2e/design.spec.js-snapshots/reader-overflow-phone-light-linux.png).
- [Activity timeline](../frontend/tests/e2e/responsive.spec.js-snapshots/activity-phone-light-linux.png), [semantic search](../frontend/tests/e2e/responsive.spec.js-snapshots/semantic-phone-dark-linux.png), [incoming links](../frontend/tests/e2e/responsive.spec.js-snapshots/backlinks-phone-dark-linux.png), [rendered graph](ui/graph-rendered-dark.png).

Visual regression snapshots hide only the moving WebGL canvas, not the selected-note inspector or controls. The unmodified rendered graph is separately attached to the Playwright report and captured above. The graph is not pixel-deterministic; its generated node positions are not a visual regression contract.

## Verification

Local environment: Node 24.22.0, Go 1.26.9, Playwright Chromium with software WebGL. The OpenChamber browser panel is display-only in this client (`drivable: false`); Playwright supplied the actual interaction, accessibility, screenshot, and browser evidence.

- `npm run check`: no Svelte errors or warnings.
- `npm test`: 9 passing tests, including light/dark normal-text contrast tokens at 4.5:1 or higher on their tested surfaces.
- `npm run build`: passing production build, locally bundled fonts.
- `npm run test:e2e`: 27 browser workflows, including the original 11 updated for application dialogs plus visual/accessibility/responsive coverage.
- axe WCAG A/AA checks on representative reader, home, search, activity, graph, and modal states; no detected violations in tested states. This is not a claim of complete WCAG certification or assistive-device testing.
- Keyboard and emulated touch checks: modal Escape/focus restoration, visible file actions, move picker, mobile drawer, outline navigation, raw-source editing, split fallback, explicit graph opening.
- Data safety: note-bound saves during navigation, stale response cancellation, revision conflict, recovered draft persistence, cancel/discard safeguards, move flush, literal extension collision protection, hostile HTML and Origin regression checks.
- `go build ./...`, `go vet ./...`, `go test -race ./...`, and `govulncheck@v1.9.0 ./...`: passing locally; no reachable vulnerabilities found.
- CI runs the production build, checks, unit/browser/race/vulnerability tests, multiarchitecture container build, and existing non-root/read-only persistence smoke test. Browser reports and screenshots are uploaded as `browser-evidence` for 14 days. Final CI status is recorded in the PR.

## Bundle comparison

Fresh builds of merged `main` and this branch, same Node/Vite environment (decimal kB):

| Asset | Merged main | Modernized |
| --- | ---: | ---: |
| Main JS | 84.28 kB / 29.16 gzip | 106.34 kB / 37.98 gzip |
| Main CSS | 21.76 kB / 4.19 gzip | 30.16 kB / 6.43 gzip |
| Deferred graph JS | 1,476.91 kB / 396.68 gzip | 1,480.34 kB / 398.05 gzip |
| Local font assets | none | 48.25 kB Inter + 40.40 kB JetBrains Mono |

Main JS increases by about 22 kB (26%). The graph stays deferred; browser coverage asserts that opening a note/search does not fetch its JS/CSS. Fonts are requested only when used. The pre-existing large graph-chunk warning remains; there is no claim of faster load time or device-wide performance improvement.

## Remaining limitations

- Chromium desktop and emulated tablet/phone were verified, not physical iOS/Android devices, Safari/Firefox, or a screen reader. Software WebGL evidence does not guarantee every GPU/driver combination.
- The existing Markdown renderer does not enable GFM pipe tables. Existing supported HTML tables are styled and keyboard-scrollable. Expanding Markdown syntax was deliberately not added as a second backend feature.
- Local browser screenshots are Linux baselines; snapshot differences on another browser/platform should be inspected rather than blindly regenerated. Update intentionally with `npm run test:e2e -- --update-snapshots` after reviewing rendered evidence.
- Files remain raw Markdown/YAML. Conflict resolution retains/copies/discards a draft; there is no merge editor or collaborative editing.
- Local session draft storage may be unavailable or cleared by the browser. Existing storage-warning behavior is preserved; it is not a server-side draft backup.
- Graph labels/positions evolve during simulation. The accessible note list remains the keyboard path; the canvas is a spatial supplement.
- No Docker/Podman is installed locally; container verification is delegated to the existing GitHub CI job, without publishing a PR image.
