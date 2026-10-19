# Security and reliability audit disposition

Scope: the 36 findings from the project-wide audit, with authentication and mandatory authenticated-proxy setup explicitly deferred by the owner. Changes are isolated to the task worktree/branch. No live services are deployed or mutated. Tests use temporary vaults, indexes, and mock embedding services. The authorized repository vault cleanup includes the six tracked notes and moved untracked cluster notes/index data; Git history is not rewritten.

## Finding-by-finding disposition

“Implemented” describes the code change, not a claim of complete security or universal platform coverage. Verification below records observed evidence; pending checks remain explicitly pending.

| # | Finding | Disposition and evidence surface |
| --- | --- | --- |
| 1 | Unauthenticated network access | Authentication deferred. Loopback default, explicit host allowlist, browser origin checks, request bounds, CSP and exposure warning implemented. `TestSecurityMiddleware`; unauthenticated access remains a documented risk. |
| 2 | Stored XSS | Implemented: HTML allowlist sanitizer, escaped WikiLink attributes/labels, escaped graph tooltips, CSP. `TestSanitizedMarkdown`, graph unit test, hostile-HTML browser workflow. |
| 3 | Incomplete containment | Implemented: rooted reads/mutations/scans and path validation; scoped activity/list/index paths protected; symlinks excluded. `TestContainmentAndRootGuards`, MCP protocol containment test. |
| 4 | Root mutation | Implemented: root-equivalent destructive paths rejected. `TestContainmentAndRootGuards`, MCP root-delete rejection. |
| 5 | Wrong-note autosave | Implemented: per-note drafts, captured identity/revision, serial saves, navigation flush, session recovery. Draft unit tests and immediate-navigation browser workflow. |
| 6 | Unsafe/conflicting mutations | Implemented: atomic synced replacement, exclusive creation, no-replace rename, HTTP revision preconditions, optional MCP revisions. Concurrent creation/revision tests; rename/delete and dirty-move browser workflows. |
| 7 | Truncated oversized saves | Implemented: body limit rejects before mutation. `TestOversizedSavePreservesFile`; shared bounded decoding. |
| 8 | YAML/body corruption | Implemented: YAML nodes preserve unknown/nested fields, scalar types and exact body bytes; malformed metadata updates fail without rewriting. `TestMetadataRoundTrip`, `TestTimestampFilteringAndMalformedYAMLSafety`. |
| 9 | Index loss after provider failure | Implemented: prepare a candidate, persist atomically, then publish; old searchable snapshot retained on embedding failure. `TestSmallEmptyPersistentAndFailedReplacement`. |
| 10 | Destructive/racy full reindex | Implemented: independent candidate collection and atomic snapshot publication, serialized filesystem mutations, cancellation/error propagation. Concurrent index/search/reindex race test; failed-reindex snapshot preservation test. |
| 11 | Skills/hidden content embedded | Implemented: shared rooted scan and exclusions in startup, reconciliation and full reindex. Watcher/exclusion integration test; only mock services receive fixture text. |
| 12 | Missing-note creation UI | Implemented: structured HTTP status errors and 404 handling. API unit test, missing-note browser workflow. |
| 13 | Extensionless file operations | Implemented: server resolves note IDs versus exact directories. Folder operations test; UI rename/delete/dirty-move workflows. |
| 14 | Folder downloads | Implemented: resolve entry type before adding note extension; rooted ZIP traversal and safe attachment headers. ZIP regression and browser workflow. |
| 15 | Stale subtree indexes | Implemented: subtree removal, rooted rebuilds/reconciliation, directory-level invalidations. Folder mutation and external directory-move integration tests. |
| 16 | Missing external/startup vectors | Implemented: asynchronous startup reconciliation, filesystem-triggered work and periodic retry scans. `TestWatcherReconcilesExternalDirectoriesAndExcludedContent`. |
| 17 | Similarity lost after restart | Implemented: reference vectors persisted with the collection snapshot. Reopen-and-similarity regression. |
| 18 | Small/empty search failures | Implemented: empty short-circuit, candidate count bounded by collection size, unique note results. Small/empty index regression. |
| 19 | Linkless graph crash | Implemented: stable backend arrays and defensive preprocessing. ZIP/graph API test, graph unit test, repeated graph browser workflow with `links: null`. |
| 20 | Activity live signal absent | Implemented: activity invalidation field and directory/log invalidations. Live event test and activity-change browser workflow. |
| 21 | Divergent link interpretation | Implemented: Markdown-aware extraction excluding code, canonical relative/absolute paths, ordinary note links rewritten, missing WikiLink targets retained, shared graph/backlink/closure extraction. Link/parser/projection tests. |
| 22 | Inconsistent projections | Implemented: common hidden/reserved/skills/symlink policy; Markdown-only sidebar with exact directory names. Folder/exclusion and watcher integration tests. |
| 23 | Stale asynchronous UI state | Implemented: abort/generation guards for note loads and previews, generations for search/graph, revisioned draft queues. Stale-load browser workflow and serial-save unit test. |
| 24 | Missing summary/title metadata | Implemented: canonical title stored, summary injected into chunks, summary-only notes searchable. Chunk metadata regression and persisted result-title assertion. |
| 25 | MCP schema mismatch | Implemented: advertised filters/AND tags, legacy single tag where advertised, bounded integer limits, revision parameters and clarified paths. Real streamable-HTTP client initialization/list/call tests assert 19 tools and filter schemas. |
| 26 | Timestamp/PATCH inconsistencies | Implemented: one atomic PATCH, explicit/default timestamps, typed YAML timestamp projection and RFC3339 comparisons; missing timestamps do not match `since`. Metadata/filter tests. |
| 27 | Embedding robustness | Implemented: context propagation, bounded batches/input/response sizes, transient retries, complete/indexed/finite/nonzero/dimension validation. Malformed-response, cancellation, batching, provider-failure tests. |
| 28 | Shutdown/timeouts | Implemented: read-header/read/idle limits, cancellable request base context, ten-second shutdown grace with error handling, bounded SSE writes and watcher/indexer cancellation. Disposable server termination observed; request-context cancellation tests. |
| 29 | Mobile navigation lockout | Implemented: always-reachable mobile toggle, stable breakpoint behavior, keyboard-adjustable resize controls and focused dialogs. Mobile browser workflow and 360 px screenshot inspection; full accessibility certification is not claimed. |
| 30 | Graph resource/performance issues | Implemented: lazy route, linear preprocessing, resize observer, teardown/GPU disposal, bounded canvas labels and separate library/Svelte containers. Large-graph unit observation and repeated mount/theme browser workflow. |
| 31 | Vault/index tracking | Implemented: tracked vault files removed; local worktree vault emptied; Git/Docker exclusions added. Verify with `git ls-files vault` after staging and ignore-rule checks. No history rewrite or deletion outside authorized vault/test data. |
| 32 | Vulnerable dependencies | Implemented: updated npm lockfile, Go x/sys/x/net, minimum Go 1.26.9. npm audit and Go 1.26.9 govulncheck report no vulnerabilities. The host's original Go 1.26.7 scan failed on reachable standard-library advisories and is not an accepted verification toolchain. |
| 33 | Missing release gates | Implemented: Go race/regression tests, vet/module verification, JS/Svelte checks, frontend unit tests, Chromium workflows, dependency scans and CI gates. Commands below. JS checking remains incremental/non-strict, not a TypeScript migration. |
| 34 | Container safeguards | Implemented: digest-pinned verified registry bases, non-root UID, filtered build context, separate persistence, read-only/capability-dropped smoke test in CI. **Local runtime verification unavailable:** no Docker/Podman executable. Remote verification must be established by the PR check result. |
| 35 | Lost live updates | Implemented: full invalidation after dropped events/reconnect, isolated frontend listener failures, SSE write/flush failure handling and deadlines. Slow-subscriber and frontend reconnect/listener tests. |
| 36 | Stale docs/operations/UX | Implemented: current README/agent facts, skills/status endpoints, validated config, real chunk overlap, watcher degradation reporting, responsive controls, theme preference updates and no Svelte accessibility warnings. Frontend checks, parser/chunker tests, browser workflows and documented boundaries. |

## Local evidence

Observed on Linux, using Go 1.26.9 and Node 24.22.0 where stated:

- `go build ./...`, `go vet ./...`, `go mod verify`: passed.
- `go test -race -count=3 ./...`: passed, including real filesystem watcher and streamable-HTTP MCP tests.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...`: no vulnerabilities found under Go 1.26.9. Original host toolchain remains Go 1.26.7 with `GOTOOLCHAIN=local`; explicitly select the required toolchain.
- Darwin/arm64 and Windows/amd64 cross-builds passed; runtime behavior on those platforms is not verified. Windows file moves use exclusive hard-link/unlink semantics; Linux/macOS use native no-replace rename.
- Frontend `check`: zero errors/warnings; `test`: all seven draft/API/graph/live tests pass; `build`: passing; `npm audit`: zero known vulnerabilities. All ten Chromium workflows pass, including deterministic conflict handling, dirty-note drag/move, and activity updates.
- Earlier complete Chromium run: all eight original workflows passed. A later parallel verification exposed a race in the conflict **test setup**: a clean editor legitimately refreshed to the newer revision before editing. The fixture now intercepts the old-revision save before making the competing update; it does not suppress real application events.
- Mobile screenshot inspection used a disposable fixture server at 360×740, with measured document width 360 (no horizontal overflow). The OpenChamber browser client was display-only; Chromium automation supplied the interaction evidence.
- Synthetic snapshot benchmark (384-dimensional fixture embeddings; three iterations; local CPU/filesystem): 100 notes ≈36.9 ms / 5.27 MB allocated per replacement; 1,000 notes ≈146.3 ms / 54.9 MB allocated. These are observations, not service-level guarantees.
- Synthetic graph preprocessing: 10,000 nodes / 50,000 edges ≈62.3 ms in one local Node 24 test run. Browser rendering/GPU performance at this scale was not benchmarked.
- Main JS bundle ≈84.1 kB uncompressed after lazy loading versus ≈1.55 MB in the audit build; the separately loaded 3D route remains ≈1.48 MB and triggers Vite's large-chunk advisory.

## Remaining boundaries and verification

- Authentication is deferred, not “fixed.” Do not expose this service to untrusted clients.
- No Docker daemon/Podman runtime is available locally. Container build/startup/persistence must be verified by PR CI before that acceptance criterion is complete.
- No real provider credentials, cluster resources, production vaults, or live deployments are used. Provider/model relevance is covered by mock protocol tests, not a relevance-quality evaluation of a real embedding model.
- Derived index snapshots are single-writer and linear-cost. Large-vault production load, power-failure injection, arbitrary concurrent external editor writes, and multi-process locking are not certified by these tests.
- Unsaved draft recovery is per browser tab/session storage; it is not durable cross-device storage. Confirm before discarding, and keep independent vault backups.
- Delivery commits, pull request URL, and final CI results are recorded in Git/PR metadata and the PR verification section rather than self-referential commit hashes in this document.
