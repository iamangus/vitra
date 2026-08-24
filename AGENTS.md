# Agent Instructions for Vitra

Vitra is a merged project: a vault-backed markdown wiki web app **plus** an MCP
endpoint **plus** an embedded semantic vector store, all in one Go binary. The
vault follows OKF v0.1 conventions (YAML frontmatter with type/title/tags). The
MCP endpoint is served on the same port as the web UI at `/mcp`.

## Development Workflow

This project uses [Air](https://github.com/cosmtrek/air) for live reloading. The
app is run via Air so any Go file changes are reflected without manual restarts.

```bash
air            # from repo root; builds ./tmp/main and runs on :8080
```

The frontend is a **Svelte 5 + Vite** app. Air only watches Go; you must rebuild
the frontend manually when touching UI:

```bash
cd frontend && npm run build
```

Outputs to `frontend/dist/`, embedded into the binary via `frontend/embed.go`.

## Running the merged binary

```bash
go build -o tmp/main ./cmd/vitra && ./tmp/main
```

Env vars (all optional, defaults shown):

| Var              | Default          | Purpose                                          |
|------------------|------------------|--------------------------------------------------|
| `VAULT_PATH`     | `./vault`        | Path to the notes vault                           |
| `PORT`           | `8080`           | Web UI + MCP port                                 |
| `SKILLS_DIR_NAME`| `skills`         | Subdirectory of the vault holding skill markdown  |
| `CHROMEM_PATH`   | `<vault>/.chromem` | chromem-go persistence dir                     |
| `EMBEDDING_API_URL` | (OpenRouter default) | OpenAI-compatible embedding endpoint        |
| `EMBEDDING_API_KEY` | —                | API key for embedding provider                   |
| `EMBEDDING_MODEL`| `text-embedding-3-small` | Embedding model name                     |

## Build & verify

```bash
go build ./...            # backend must compile clean
go vet ./...              # static checks
cd frontend && npm run build   # frontend must build
```

## Project Structure

```
.
├── cmd/vitra/main.go        # Single binary: web + MCP at /mcp
├── internal/
│   ├── api.go                # HTTP handlers (files, notes, search, OKF endpoints)
│   ├── filesystem.go         # FileSystem struct, vault ops, vector auto-index hooks
│   ├── index.go              # In-memory substring index + backlinks + graph
│   ├── markdown.go           # goldmark rendering with WikiLinks + frontmatter parser
│   ├── live.go               # SSE live updates + fsnotify watcher
│   ├── mcp/
│   │   └── server.go         # MCP server (18 tools: vault + OKF)
│   ├── okf/
│   │   └── okf.go            # OKF helpers: Extract, IsReservedFilename, ExtractOKFLinks, ParseLogEntries
│   └── vector/
│       ├── store.go          # VectorStore interface + SearchResult/Chunk/Filter
│       ├── chromem.go        # chromem-go embedded implementation
│       ├── embeddings.go      # OpenAI-compatible EmbeddingClient
│       ├── chunker.go         # Note → chunks with heading breadcrumbs
│       └── dedup.go           # Duplicate detection helper
├── frontend/                 # Svelte 5 + Vite SPA
│   ├── src/
│   │   ├── components/Search.svelte   # Full-text / Semantic toggle
│   │   └── lib/api.js                 # API client (incl. search.semantic)
│   └── dist/                 # Built frontend (embedded via //go:embed)
├── Dockerfile                # Multi-stage: node → go → alpine
├── docker-compose.yml        # Single-service vitra (no chromadb)
└── .air.toml
```

## Key Conventions

- **No Tailwind** — scoped CSS in components + CSS variables in `app.css`.
- **Purple accents** — primary `#7c3aed` (light) / `#a855f7` (dark).
- **Dark mode** — `#0a0a0c` background, not gray-blue.
- **Mobile-first** — sidebar slide-out overlay on mobile (`<=768px`).
- **Icons** — inline SVGs, no icon library.
- **No code comments** unless explicitly requested.

## API Endpoints

### Web (port 8080)
- `GET  /api/files` — file tree (sidebar)
- `GET  /api/note/{path}` — note (content, html, frontmatter, breadcrumbs, links)
- `POST /api/note/{path}` — save note (auto-indexes to vector store)
- `POST /api/notes` — create note
- `POST /api/folders` — create folder
- `PUT  /api/rename` — rename file/folder
- `DELETE /api/delete` — delete file/folder (auto-removes from vector store)
- `GET  /api/search?q=...` — full-text substring search
- `GET  /api/search/semantic?q=...&limit=N&type=...&tag=...` — semantic search via chromem-go
- `GET  /api/backlinks/{path}` — backlinks
- `GET  /api/graph` — graph nodes (with `type` field) + links
- `GET  /api/events` — SSE stream of vault changes
- `POST /api/preview/{path}` — render markdown preview
- `GET  /api/concepts?type=...&tag=...&resource=...&since=...&limit=...` — OKF concept catalog
- `GET  /api/concepts/closure?path=...&depth=N` — transitive closure of link graph
- `GET  /api/activity?path=...&limit=...` — aggregated log.md activity feed
- `PATCH /api/note/{path}` — merge frontmatter updates (preserves body)
- `GET  /api/skills` — skill metadata `[{name, title, description, tags, path, size, mtime}]` (for system-prompt inclusion)

### MCP (at /mcp on web port)
Streamable HTTP at `http://localhost:8080/mcp`. Tools:

**Vault operations** (13): `read_note`, `write_note`, `create_note`,
`delete_note`, `list_notes`, `search_notes`, `rename_note`, `get_backlinks`,
`create_folder`, `search_wiki`, `find_similar_files`, `suggest_links`,
`reindex_vault`.

**OKF-aware (Scope D)** (5): `list_concepts`, `get_linked_concepts`,
`get_transitive_closure`, `update_note`, `get_index`.

## Skills

Skill markdown files live in the vault under `SKILLS_DIR_NAME` (default
`skills`). They are ordinary OKF notes and are **managed with the same MCP note
tools** (`read_note`, `write_note`, `create_note`, `update_note`,
`delete_note`), addressing them by their vault-relative path, e.g.
`read_note(path="skills/my_skill")`.

Discovery is via `GET /api/skills`, which returns metadata-only for each
`*.md` in `skills/`:

```json
[{ "name": "my_skill", "title": "my_skill", "description": "...",
   "tags": ["example"], "path": "skills/my_skill", "size": 123, "mtime": "..." }]
```

Frontmatter (OKF v0.1):

```markdown
---
type: Skill
title: my_skill
description: What this skill does, shown to the LLM as the description.
tags: [example, demo]
---

# Skill body

The skill content — read it with read_note.
```

Skills are excluded from the OKF catalog (concepts, graph, activity, semantic
index, vault index) and from the sidebar file tree. `type` is OKF canonical
(`Skill`, capitalized).