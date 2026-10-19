# Vitra

A vault-backed Markdown wiki, MCP endpoint, and embedded semantic index in one Go binary, with a Svelte 5/Vite UI. Features include YAML frontmatter, WikiLinks and Markdown note links, backlinks, graph, full-text/semantic search, activity, and OKF concepts/skills.

## Security boundary

**Authentication is not implemented.** Anyone who can reach Vitra can read, edit, and delete notes and invoke MCP tools. Authentication and mandatory authenticated-proxy setup are explicitly deferred. Host/origin checks and sanitization are defense in depth, not access control: keep the service on a trusted local interface/network and do not expose it publicly.

The native default is `127.0.0.1:8080`. Remote use requires explicit bind/host configuration. Vault access uses rooted filesystem handles, rejects symlinks and hidden paths, and forbids destructive root operations. Only one Vitra process should write a vault/index pair. Linux is the currently verified native/container target; non-Linux builds are not verified. Do not grant untrusted local processes write access to these directories.

Vaults, indexes, credentials, build output, and test artifacts must not be committed or included in Docker contexts. No populated vault ships with the app. Back up notes independently: deletion is permanent, not a trash operation.

## Develop and verify

Requires **Go 1.26.9+** and **Node 24+**. If system Go is older, set `GOTOOLCHAIN=auto`; `GOTOOLCHAIN=local` refuses to build below the version in `go.mod`.

```bash
cd frontend
npm ci
npm run check
npm test
npm run build
cd ..
go build ./...
go vet ./...
go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
go build -o tmp/vitra ./cmd/vitra
./tmp/vitra
```

Build the frontend before compiling Go: `frontend/dist` is embedded. Air rebuilds the Go binary; frontend changes still need a Vite build.

```bash
cd frontend
npx playwright install --with-deps chromium
npm run build
npm run test:e2e
npm audit --audit-level=moderate
```

Browser tests create a disposable vault/index and local mock embedding service. They reserve `127.0.0.1:18089`, refuse to reuse an existing server, and never use the normal vault or configured production provider.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `VAULT_PATH` | `./vault` | Notes directory, created at startup |
| `PORT` | `8080` | Port, 1–65535 |
| `BIND_ADDRESS` | `127.0.0.1` | Interface; container overrides to `0.0.0.0` |
| `ALLOWED_HOSTS` | empty | Additional comma-separated hostnames, without scheme/port; localhost/loopback always accepted |
| `SKILLS_DIR_NAME` | `skills` | Single visible subdirectory name |
| `CHROMEM_PATH` | `<vault>/.chromem` | Private generated vector snapshot directory |
| `EMBEDDING_API_URL` | `https://api.openai.com/v1` | OpenAI-compatible base URL; `/embeddings` is appended |
| `EMBEDDING_API_KEY` | empty | Provider key; explicit custom/local endpoints may omit it |
| `EMBEDDING_MODEL` | `text-embedding-3-small` | Model |
| `CHUNK_SIZE` | `0` → 1000 | Unicode code points per chunk; maximum 4000 |
| `CHUNK_OVERLAP` | `0` → 200 | Overlap; `-1` disables; explicit value must be below chunk size |
| `HYBRID_BOOST_FACTOR` | `0.2` | Ranking boost, 0–1; `0` disables |

Indexing is asynchronous and eventually consistent. Saves do not wait for the embedding provider. The vault is the durable work source: startup reconciles it, watcher events enqueue changes, and a 30-second retry scan covers missed events/outages. Failed replacements retain the previous searchable snapshot. `/api/status` and `/readyz` report semantic degradation; `/healthz` only reports core-process liveness.

`snapshot.json` holds a chromem export, model fingerprint, and persisted reference vectors. Legacy chromem `.gob` indexes are derived data and rebuilt from the vault, not silently rewritten. Changing the embedding model/base URL requires full reindexing. Manual full reindex builds before publication and serializes note mutations for its duration; failure/cancellation retains the old index. External editors are supported but cannot participate in HTTP revision locking.

Atomic snapshots cost linear time/memory in index size per changed note; benchmark large vaults before deployment. Embedding requests are bounded, cancellable, batched, validated, and retried only for limited transient failures. Summaries, including summary-only notes, contribute to search. Skills, hidden/reserved files, and symlinks are excluded from semantic indexing.

## Editing and HTTP contracts

`GET /api/note/{path}` returns content, metadata, sanitized HTML, links, and a quoted hash `revision` also sent as `ETag`. Existing-note HTTP saves/PATCH require `If-Match`: missing revisions return `428`, stale revisions/existing destinations `409`, oversized bodies `413`, and missing notes `404`. The 10 MiB body limit is checked before writing. Creation is exclusive; renames never replace destinations. Metadata/body PATCH is one atomic mutation, preserving untouched YAML fields and exact body bytes.

Unsaved/conflicting editor drafts are retained in **session storage for that browser tab**. Navigation flushes using the captured note identity/revision. Explicit discard requires confirmation; closing with unsaved work warns. Session storage is not a backup or appropriate for shared browser sessions containing private notes.

| Endpoints | Purpose |
| --- | --- |
| `GET /api/files`, `GET /api/skills` | Tree and skill discovery |
| `GET`, `POST`, `PATCH /api/note/{path}` | Read, revision-aware save, metadata/body update |
| `POST /api/notes`, `POST /api/folders` | Create notes/directories |
| `PUT /api/rename`, `DELETE /api/delete?path=...` | Rename/move/delete |
| `GET /api/download?path=...` | Note or directory ZIP |
| `GET /api/search`, `GET /api/search/semantic` | Text/semantic search; semantic limit ≤100, optional type and repeated tag filters |
| `GET /api/backlinks/{path}`, `GET /api/graph` | Link projections |
| `GET /api/concepts`, `GET /api/concepts/closure` | Concepts and closure; RFC3339 since, AND tags, depth ≤20 |
| `GET /api/activity`, `GET /api/events` | Activity/live invalidations; reconnect refreshes projections |
| `POST /api/preview/{path}` | Sanitized preview without frontmatter |
| `GET /api/status`, `GET /readyz`, `GET /healthz` | Degradation status/liveness |

Read/save note IDs omit the file extension: ID `example.md` identifies the literal filename `example.md.md`. Rename/delete/download accept actual entry filenames and resolve extensionless IDs when no exact entry exists; the UI always sends actual filenames for these operations to avoid suffix/directory collisions. Directory names remain exact. Markdown links are source-relative; ambiguous WikiLinks should use full vault-relative paths. Moves do not rewrite references in other files.

## MCP and skills

Streamable HTTP is at `/mcp` on the web port. The 19 tools are `read_note`, `write_note`, `create_note`, `delete_note`, `list_notes`, `search_notes`, `rename_note`, `get_backlinks`, `create_folder`, `search_wiki`, `find_similar_files`, `suggest_links`, `reindex_vault`, `list_concepts`, `get_linked_concepts`, `get_transitive_closure`, `update_note`, `get_index`, and `list_skills`.

Pass the revision from `read_note` to `write_note`/`update_note` for conflict protection. MCP revisions remain optional for compatibility; omitting one intentionally performs an unconditional write. `create_note` requires a summary and supplies OKF frontmatter. Semantic schemas advertise type/tags filters and bounded integer limits. Tags use AND semantics; legacy single-tag arguments remain supported where advertised.

Skills are ordinary frontmatter Markdown under the configured skills directory, managed using note tools and discovered via `list_skills` or `/api/skills`. They are excluded from normal sidebar, concepts, graph, activity, and search projections.

## Containers

`docker compose up --build` publishes on host loopback. The image uses digest-pinned bases, runs as UID/GID `10001:10001`, has a liveness check, and keeps vault/index in separate writable volumes. Bind mounts must be writable by this identity. A read-only root filesystem, no-new-privileges, and dropping all capabilities are supported. Remote hostnames still need explicit allowlisting and do not add authentication.

CI runs static checks, race/regression tests, dependency scans, Chromium workflows, multiarchitecture builds, and a non-root/read-only container persistence smoke test. See [audit dispositions and evidence](docs/AUDIT.md).
