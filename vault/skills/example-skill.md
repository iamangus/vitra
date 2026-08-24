---
type: Skill
title: example_skill
description: A minimal example skill demonstrating the frontmatter convention used by the vitra skills directory.
tags: [example, demo]
---

# Example Skill

This file lives in the vault's skills directory (configured by `SKILLS_DIR_NAME`,
defaulting to `skills`). Skills are ordinary OKF markdown notes stored in that
directory; they are listed (metadata only) via `GET /api/skills` and are
managed with the same MCP tools used for notes.

## Convention

Skills follow OKF v0.1 frontmatter conventions:

- **`type`** — should be `Skill` (capitalized) to identify the file as a skill
  and to keep it out of the note/OKF catalog, graph, and semantic index.
- **`title`** — a short, human-readable name for the skill.
- **`description`** — a concise summary shown in the `/api/skills` listing so an
  LLM can decide whether to load the skill.
- **`tags`** — optional metadata for discovery.

## Discovery

Call `GET /api/skills` to get a JSON array of metadata for every skill in the
directory. Each entry includes `name`, `title`, `description`, `tags`, `path`,
`size`, and `mtime`. The `path` field is vault-relative (e.g. `skills/example_skill`)
so it can be passed directly to the note tools.

## Managing skills

Skills are read and edited exactly like notes, using the MCP tools. The `path`
argument for a skill is its vault-relative path under `skills/`:

- `read_note(path="skills/example_skill")` — fetch full skill content.
- `write_note(path="skills/example_skill", content=...)` — overwrite the skill.
- `create_note(path="skills/<new_skill>", ...)` — create a new skill.
- `update_note(path="skills/example_skill", frontmatter={...})` — merge
  frontmatter changes.
- `delete_note(path="skills/example_skill")` — remove a skill.

The `skills/` directory is excluded from the sidebar file tree and the OKF
catalog, graph, activity, and semantic index.
