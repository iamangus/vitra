---
name: example_skill
description: A minimal example skill demonstrating the frontmatter convention used by the vitra skills MCP server.
---

# Example Skill

This file lives in the vault's skills directory (configured by `SKILLS_DIR_NAME`,
defaulting to `skills`). The vitra skills MCP server exposes it as a tool
named `example_skill`.

## Convention

- **`name`** (required): a valid tool identifier — lowercase letters, digits,
  and underscores. Must match `^[a-zA-Z_][a-zA-Z0-9_]*$`. Used as the MCP tool
  name.
- **`description`** (recommended): a short summary shown to the LLM in the tool
  listing. Falls back to the first heading or non-empty paragraph if omitted.

## Behavior

When an LLM calls the tool, the skills MCP server reads this entire file and
returns its contents as the tool result. There are no input parameters — the
intent is context injection on demand, mirroring Anthropic's skill behavior.

## Live updates

The skills directory is watched with fsnotify. Adding, editing, or removing a
`*.md` file here automatically registers, updates, or deregisters the
corresponding MCP tool — no restart required.