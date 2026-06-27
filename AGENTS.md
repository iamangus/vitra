# Agent Instructions for Vitra

Vitra is a merged project: a vault-backed markdown wiki web app **plus** two MCP
servers (tools + skills) **plus** an embedded semantic vector store, all in one
Go binary.

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

| Var              | Default        | Purpose                                          |
|------------------|----------------|--------------------------------------------------|
| `VAULT_PATH`     | `./vault`      | Path to the notes vault                           |
| `PORT`           | `8080`         | Web UI port                                       |
| `MCP_TOOLS_PORT` | `3000`         | Tools MCP server port (Streamable HTTP at `/mcp`) |
| `MCP_SKILLS_PORT`| `3001`         | Skills MCP server port (Streamable HTTP at `/mcp`)|
| `SKILLS_DIR_NAME`| `skills`        | Subdirectory of the vault holding skill markdown  |
| `CHROMEM_PATH`   | `<vault>/.chromem` | chromem-go persistence dir                   |
| `EMBEDDING_API_URL` | (OpenRouter default) | OpenAI-compatible embedding endpoint      |
| `EMBEDDING_API_KEY` | —              | API key for embedding provider                   |
| `EMBEDDING_MODEL`| `text-embedding-3-small` | Embedding model name                   |

## Build & verify

```bash
go build ./...            # backend must compile clean
go vet ./...              # static checks
cd frontend && npm run build   # frontend must build
```

## Project Structure

```
.
├── cmd/vitra/main.go        # Single binary: web + tools MCP + skills MCP
├── internal/
│   ├── api.go                # HTTP handlers (files, notes, search, etc.)
│   ├── filesystem.go         # FileSystem struct, vault ops, vector hooks
│   ├── index.go              # In-memory substring index + backlinks + graph
│   ├── markdown.go           # goldmark rendering with WikiLinks
│   ├── live.go               # SSE live updates + fsnotify watcher
│   ├── mcp/
│   │   ├── server.go         # Tools MCP server (13 vault + vector tools)
│   │   └── skills.go         # Skills MCP server (one no-param tool per .md)
│   └── vector/
│       ├── store.go          # VectorStore interface + SearchResult/Chunk
│       ├── chromem.go        # chromem-go embedded implementation
│       ├── embeddings.go      # OpenAI-compatible EmbeddingClient
│       ├── chunker.go         # Note → chunks with heading breadcrumbs
│       └── dedup.go           # Duplicate detection helper
├── frontend/                 # Svelte 5 + Vite SPA
│   ├── src/
│   │   ├── components/Search.svelte   # Full-text / Semantic toggle
│   │   └── lib/api.js                 # API client (incl. search.semantic)
│   └── dist/                 # Built frontend (embedded via //go:embed)
├── skills/                   # Example skill markdown files
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
- `GET  /api/files` — file tree
- `GET  /api/note/{path}` — note (content, html, frontmatter, breadcrumbs)
- `POST /api/note/{path}` — save note (auto-indexes to vector store)
- `POST /api/notes` — create note
- `POST /api/folders` — create folder
- `PUT  /api/rename` — rename file/folder
- `DELETE /api/delete` — delete file/folder (auto-removes from vector store)
- `GET  /api/search?q=...` — full-text search
- `GET  /api/search/semantic?q=...&limit=N` — semantic search via chromem-go
- `GET  /api/backlinks/{path}` — backlinks
- `GET  /api/graph` — graph nodes/links
- `GET  /api/events` — SSE stream of vault changes
- `POST /api/preview/{path}` — render markdown preview

### MCP servers
- Tools MCP — Streamable HTTP at `http://localhost:3000/mcp`. Exposes 13 tools:
  `read_note`, `write_note`, `create_note`, `delete_note`, `list_notes`,
  `search_notes`, `rename_note`, `get_backlinks`, `create_folder`,
  `search_wiki`, `find_similar_files`, `suggest_links`, `reindex_vault`.
- Skills MCP — Streamable HTTP at `http://localhost:3001/mcp`. Exposes one
  no-param tool per `*.md` file in `<VAULT_PATH>/<SKILLS_DIR_NAME>/`. Each tool
  returns the full skill file content when called. Skill files use frontmatter
  `name` + `description`; the directory is watched live (fsnotify).

## Skills convention

Skill markdown files live in the vault under `SKILLS_DIR_NAME` (default
`skills`). Frontmatter:

```markdown
---
name: my_skill
description: What this skill does, shown to the LLM as the tool description.
---

# Skill body

The tool returns this entire file when invoked.
```

`name` must match `^[a-zA-Z_][a-zA-Z0-9_]*$`. Adding/editing/removing a `*.md`
file live-updates the registered tool set.