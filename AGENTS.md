# Agent Instructions for WebTerm

## Development Workflow

This project uses [Air](https://github.com/cosmtrek/air) for live reloading. The app is run via Air so that any file changes are automatically reflected without manual restarts.

### Frontend Build

The frontend is a **Svelte 5 + Vite** app. Air only watches/reloads the Go backend. **You must manually rebuild the frontend** when making UI changes:

```bash
cd frontend && npm run build
```

This outputs to `frontend/dist/`, which the Go server serves statically. Air will then restart the Go server to pick up the new build.

## Testing After Changes

After making changes, use the browser tool to verify the app by navigating to:

```
https://app.srvd.site/
```

No additional build or test commands are required because Air handles live reloading.

## Project Structure

```
.
├── main.go              # Entry point, HTTP server, static file serving
├── api.go               # HTTP handlers (files, notes, search, backlinks, etc.)
├── filesystem.go        # File tree operations
├── markdown.go          # Markdown rendering with goldmark + WikiLinks
├── frontend/            # Svelte 5 frontend
│   ├── src/
│   │   ├── App.svelte           # Root layout (sidebar + main content)
│   │   ├── app.css              # Global styles, CSS variables, themes
│   │   ├── main.js              # Entry point
│   │   ├── components/
│   │   │   ├── Sidebar.svelte   # File tree, new note/folder, theme toggle
│   │   │   ├── NoteEditor.svelte # Note view/edit/split with unified header
│   │   │   ├── FileTree.svelte  # Recursive file tree
│   │   │   ├── Backlinks.svelte # Backlinks section
│   │   │   └── Search.svelte    # Full-text search page
│   │   ├── stores/
│   │   │   └── theme.js         # Light/dark/system theme store
│   │   └── lib/
│   │       └── api.js           # API client
│   └── dist/            # Built frontend (generated, do not commit)
├── vault/               # Default notes directory
└── .air.toml            # Air config
```

## Key Conventions

- **No Tailwind** — All styles use scoped CSS in components + CSS variables in `app.css`
- **CSS Variables** — Themes are controlled via `:root` and `html.dark` CSS custom properties
- **Purple accents** — Primary color is purple (`#7c3aed` light, `#a855f7` dark)
- **Dark mode** — Very dark (`#0a0a0c` background), not gray-blue
- **Mobile-first** — Sidebar becomes a slide-out overlay on mobile (`<=768px`)
- **Icons** — Inline SVGs, no icon library

## API Endpoints

- `GET /api/files` — File tree
- `GET /api/note/{path}` — Get note (returns `{title, content, html}`)
- `POST /api/note/{path}` — Save note
- `POST /api/notes` — Create note
- `POST /api/folders` — Create folder
- `PUT /api/rename` — Rename file/folder
- `DELETE /api/delete` — Delete file/folder
- `GET /api/search?q=...` — Full-text search
- `GET /api/backlinks/{path}` — Get backlinks
- `POST /api/preview/{path}` — Render markdown preview
