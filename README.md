# Vitra

A clean, fast, Obsidian-compatible notes app with a web-based interface.

## Features

- **Obsidian-compatible** — Works with your existing `.md` files and `[[WikiLinks]]`
- **WikiLinks** — Link between notes with `[[Note Name]]`
- **Tags** — Organize with `#tags`
- **Full-text search** — Find anything instantly
- **Live preview** — See rendered markdown while you edit
- **Split view** — Edit and preview side by side with draggable divider
- **Dark mode** — Easy on the eyes, with purple accents
- **Backlinks** — See which notes link to the one you're reading
- **Mobile-friendly** — Responsive design that works on any device

## Tech Stack

- **Backend**: Go 1.23 + `goldmark` for Markdown rendering
- **Frontend**: Svelte 5 + Vite
- **Live reload**: Air (Go) watches for changes

## Development

### Prerequisites

- Go 1.23+
- Node.js (for frontend builds)
- [Air](https://github.com/cosmtrek/air) for live reloading

### Running

```bash
# Start the Go backend with live reload
air

# In another terminal, build the frontend when you make UI changes
cd frontend && npm run build
```

The Go server serves the built frontend from `frontend/dist/` and handles API requests.

### Frontend Build

The frontend is a static Svelte app built with Vite:

```bash
cd frontend
npm install
npm run build
```

This outputs to `frontend/dist/`, which the Go server serves.

### Project Structure

```
.
├── main.go              # Entry point, HTTP server
├── api.go               # HTTP handlers
├── filesystem.go        # File operations
├── markdown.go          # Markdown rendering (goldmark)
├── frontend/            # Svelte frontend
│   ├── src/
│   │   ├── App.svelte
│   │   ├── app.css
│   │   ├── components/
│   │   │   ├── Sidebar.svelte
│   │   │   ├── NoteEditor.svelte
│   │   │   ├── FileTree.svelte
│   │   │   ├── Backlinks.svelte
│   │   │   └── Search.svelte
│   │   ├── stores/
│   │   │   └── theme.js
│   │   └── lib/
│   │       └── api.js
│   └── dist/            # Built frontend (generated)
├── vault/               # Your notes live here
└── .air.toml            # Air config
```

## Configuration

- `VAULT_PATH` — Directory containing your notes (default: `./vault`)
- `PORT` — Server port (default: `8080`)

## License

MIT
