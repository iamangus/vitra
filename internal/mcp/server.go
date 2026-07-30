package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iamangus/vitra/internal"
	"github.com/iamangus/vitra/internal/vector"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// NewToolsServer creates the tools MCP server, exposing the vault file
// operations plus the semantic vector tools. Returns the streamable HTTP
// handler to be mounted on the web mux (e.g. at /mcp).
func NewToolsServer(fs *internal.FileSystem, skillsDirName string) *server.StreamableHTTPServer {
	s := server.NewMCPServer(
		"vitra-tools",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	s.AddTool(mcp.NewTool("read_note",
		mcp.WithDescription("Read a note from the vault"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to the note (without .md extension)")),
	), handleReadNote(fs))

	s.AddTool(mcp.NewTool("write_note",
		mcp.WithDescription("Write or overwrite a note in the vault"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to the note (without .md extension)")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Full markdown content of the note")),
	), handleWriteNote(fs))

	s.AddTool(mcp.NewTool("create_note",
		mcp.WithDescription("Create a new note (fails if it already exists). "+
			"Notes follow OKF conventions: every note has frontmatter with a `type` field (defaults to \"Note\"). "+
			"Provide a concise `summary` of what the note contains — it is embedded for vector search. "+
			"Set `tags` as a comma-separated list to enable semantic search filtering."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to the note (without .md extension)")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("A one-line summary of what this note contains (improves semantic search)")),
		mcp.WithString("content", mcp.Description("Optional initial content (defaults to frontmatter with title)")),
		mcp.WithObject("okf", mcp.Description("Optional OKF frontmatter fields (title, type, tags, description)")),
	), handleCreateNote(fs))

	s.AddTool(mcp.NewTool("delete_note",
		mcp.WithDescription("Delete a note or folder from the vault"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to delete")),
	), handleDeleteNote(fs))

	s.AddTool(mcp.NewTool("list_notes",
		mcp.WithDescription("List the vault file tree"),
		mcp.WithString("path", mcp.Description("Optional subpath to list (defaults to vault root)")),
	), handleListNotes(fs))

	s.AddTool(mcp.NewTool("search_notes",
		mcp.WithDescription("Search notes by substring content"),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query string")),
	), handleSearchNotes(fs))

	s.AddTool(mcp.NewTool("rename_note",
		mcp.WithDescription("Rename or move a note or folder"),
		mcp.WithString("old", mcp.Required(), mcp.Description("Current vault-relative path")),
		mcp.WithString("new", mcp.Required(), mcp.Description("New vault-relative path")),
	), handleRenameNote(fs))

	s.AddTool(mcp.NewTool("get_backlinks",
		mcp.WithDescription("Find notes that link to a given note"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to the target note")),
	), handleGetBacklinks(fs))

	s.AddTool(mcp.NewTool("create_folder",
		mcp.WithDescription("Create a new folder in the vault"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to the new folder")),
	), handleCreateFolder(fs))

	s.AddTool(mcp.NewTool("search_wiki",
		mcp.WithDescription("Semantic search across the vault using vector similarity. Best results come from including specific technical terms, project names, or concepts in the query (e.g., 'Go embed Svelte SPA' not 'how do I build a website'). Prefer multiple targeted queries with different phrasings over a single conversational one."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query — use specific technical terms and concepts")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of results (default: 5)")),
	), handleSearchWiki(fs))

	s.AddTool(mcp.NewTool("find_similar_files",
		mcp.WithDescription("Find files semantically similar to a given file"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to the reference note")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of results (default: 5)")),
	), handleFindSimilar(fs))

	s.AddTool(mcp.NewTool("suggest_links",
		mcp.WithDescription("Suggest wiki-links for a note based on semantic similarity"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to the note")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of suggestions (default: 5)")),
	), handleSuggestLinks(fs))

	s.AddTool(mcp.NewTool("reindex_vault",
		mcp.WithDescription("Rebuild the entire vector index for the vault"),
	), handleReindexVault(fs))

	// OKF-aware tools (Scope D).

	s.AddTool(mcp.NewTool("list_concepts",
		mcp.WithDescription("List OKF concepts in the vault, optionally filtered by type/tags/resource/since"),
		mcp.WithString("type", mcp.Description("Filter by OKF `type` frontmatter field")),
		mcp.WithString("tag", mcp.Description("Filter by tag (single tag; repeat for AND semantics)")),
		mcp.WithString("resource", mcp.Description("Filter by canonical resource URI")),
		mcp.WithString("since", mcp.Description("ISO 8601 datetime; only notes with timestamp >= since")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of results (default: unlimited)")),
	), handleListConcepts(fs))

	s.AddTool(mcp.NewTool("get_linked_concepts",
		mcp.WithDescription("Return the concepts that the given concept links to (outgoing OKF cross-links dereferenced into full concept objects)"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to the seed concept")),
	), handleGetLinkedConcepts(fs))

	s.AddTool(mcp.NewTool("get_transitive_closure",
		mcp.WithDescription("Walk the OKF link graph from a seed concept up to `depth` hops, returning the transitive closure of reachable concepts"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to the seed concept")),
		mcp.WithNumber("depth", mcp.Description("Maximum hops (default: 2)")),
	), handleGetTransitiveClosure(fs))

	s.AddTool(mcp.NewTool("update_note",
		mcp.WithDescription("Merge frontmatter updates into an existing note, preserving the body and auto-touching `timestamp` unless supplied"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to the note (without .md extension)")),
		mcp.WithObject("frontmatter", mcp.Description("Key/value map of frontmatter fields to merge (overwrites existing keys)")),
	), handleUpdateNote(fs))

	s.AddTool(mcp.NewTool("get_index",
		mcp.WithDescription("Read a directory's index.md; synthesize one from the folder's concepts when absent (spec §6 permits)"),
		mcp.WithString("path", mcp.Description("Vault-relative directory path (defaults to vault root)")),
	), handleGetIndex(fs))

	s.AddTool(mcp.NewTool("list_skills",
		mcp.WithDescription("List available skills from the skills directory. Each skill is an OKF markdown file with frontmatter (title, description, type, tags). Use read_note to read the full skill content."),
	), handleListSkills(fs, skillsDirName))

	return server.NewStreamableHTTPServer(s)
}

func handleReadNote(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		note, err := fs.ReadNote(path)
		if err != nil {
			return nil, err
		}
		data, err := json.MarshalIndent(note, "", "  ")
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleWriteNote(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		content := req.GetString("content", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		if err := fs.WriteNote(path, content); err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(fmt.Sprintf("Note written: %s", path)), nil
	}
}

func handleCreateNote(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		summary := req.GetString("summary", "")
		if summary == "" {
			return nil, fmt.Errorf("summary is required")
		}
		content := req.GetString("content", "")

		var okfFields map[string]interface{}
		if raw, ok := req.GetArguments()["okf"]; ok && raw != nil {
			switch v := raw.(type) {
			case map[string]interface{}:
				okfFields = v
			case string:
				if v != "" {
					if err := json.Unmarshal([]byte(v), &okfFields); err != nil {
						return nil, fmt.Errorf("okf must be a JSON object, got: %s", v)
					}
				}
			}
		}

		content = internal.EnsureOKFFrontmatter(content, summary, path, okfFields)
		if err := fs.CreateNote(path, content); err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(fmt.Sprintf("Note created: %s", path)), nil
	}
}

func handleDeleteNote(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		if err := fs.DeleteNote(path); err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(fmt.Sprintf("Deleted: %s", path)), nil
	}
}

func handleListNotes(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		tree, err := fs.ListNotes(path)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(tree, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleSearchNotes(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		if query == "" {
			return nil, fmt.Errorf("query is required")
		}
		results, err := fs.SearchNotes(query)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(results, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleRenameNote(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		old := req.GetString("old", "")
		newPath := req.GetString("new", "")
		if old == "" || newPath == "" {
			return nil, fmt.Errorf("old and new paths are required")
		}
		if err := fs.RenameNote(old, newPath); err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(fmt.Sprintf("Renamed: %s -> %s", old, newPath)), nil
	}
}

func handleGetBacklinks(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		results, err := fs.NoteBacklinks(path)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(results, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleCreateFolder(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		if err := fs.CreateFolder(path); err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(fmt.Sprintf("Folder created: %s", path)), nil
	}
}

func handleSearchWiki(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		if query == "" {
			return nil, fmt.Errorf("query is required")
		}
		limit := int(req.GetFloat("limit", 5))
		filter := vector.Filter{
			Type: req.GetString("type", ""),
		}
		if tag := req.GetString("tag", ""); tag != "" {
			filter.Tags = []string{tag}
		}
		results, err := fs.SemanticSearch(ctx, query, limit, filter)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(results, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleFindSimilar(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		limit := int(req.GetFloat("limit", 5))
		filter := vector.Filter{
			Type: req.GetString("type", ""),
		}
		results, err := fs.FindSimilarFiles(ctx, path, limit, filter)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(results, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleSuggestLinks(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		limit := int(req.GetFloat("limit", 5))
		results, err := fs.FindSimilarFiles(ctx, path, limit, vector.Filter{})
		if err != nil {
			return nil, err
		}
		suggestions := make([]map[string]string, len(results))
		for i, r := range results {
			suggestions[i] = map[string]string{
				"path":    r.Path,
				"title":   r.Title,
				"heading": r.Heading,
				"link":    fmt.Sprintf("[[%s]]", r.Title),
			}
		}
		data, _ := json.MarshalIndent(suggestions, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleReindexVault(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		indexed, err := fs.ReindexVault(ctx)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(fmt.Sprintf("Reindexed %d notes", indexed)), nil
	}
}

// handleListConcepts lists OKF concept catalog with optional filters.
func handleListConcepts(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filter := internal.ConceptFilter{
			Type:     req.GetString("type", ""),
			Resource: req.GetString("resource", ""),
			Since:    req.GetString("since", ""),
		}
		if tag := req.GetString("tag", ""); tag != "" {
			filter.Tag = []string{tag}
		}
		filter.Limit = int(req.GetFloat("limit", 0))
		views, okfVersion, err := fs.ListConcepts(filter)
		if err != nil {
			return nil, err
		}
		envelope := map[string]interface{}{
			"okf_version": okfVersion,
			"count":       len(views),
			"concepts":    views,
		}
		data, _ := json.MarshalIndent(envelope, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

// handleGetLinkedConcepts dereferences a concept's outgoing OKF links into
// fully-parsed concept objects.
func handleGetLinkedConcepts(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		note, err := fs.ReadNote(path)
		if err != nil {
			return nil, err
		}
		var out []internal.ConceptView
		for _, link := range note.Links {
			linked, err := fs.ReadNote(link)
			if err != nil {
				continue
			}
			out = append(out, internal.ConceptView{
				Path:        linked.Path,
				Type:        linked.Type,
				Title:       linked.Title,
				Description: linked.Description,
				Resource:    linked.Resource,
				Tags:        linked.Tags,
				Timestamp:   linked.Timestamp,
			})
		}
		if out == nil {
			out = []internal.ConceptView{}
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

// handleGetTransitiveClosure wraps fs.TransitiveClosure for MCP.
func handleGetTransitiveClosure(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		depth := int(req.GetFloat("depth", 2))
		closure, err := fs.TransitiveClosure(path, depth)
		if err != nil {
			return nil, err
		}
		if closure == nil {
			closure = []internal.ConceptView{}
		}
		data, _ := json.MarshalIndent(closure, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

// handleUpdateNote merges frontmatter updates into an existing note.
func handleUpdateNote(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		raw, ok := req.GetArguments()["frontmatter"]
		if !ok {
			return nil, fmt.Errorf("frontmatter is required")
		}
		updates, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("frontmatter must be an object")
		}
		if err := fs.UpdateNoteMetadata(path, updates); err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(fmt.Sprintf("Updated: %s", path)), nil
	}
}

// handleGetIndex returns a directory's index.md content, synthesizing one
// from the folder's concepts when index.md is absent (spec §6 permits).
func handleGetIndex(fs *internal.FileSystem) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		scope := req.GetString("path", "")
		views, _, err := fs.ListConcepts(internal.ConceptFilter{})
		if err != nil {
			return nil, err
		}
		prefix := scope
		if prefix != "" {
			prefix = strings.TrimSuffix(prefix, "/") + "/"
		}
		var inDir []internal.ConceptView
		for _, v := range views {
			if scope == "" && !strings.Contains(v.Path, "/") {
				inDir = append(inDir, v)
			} else if scope != "" && strings.HasPrefix(v.Path, prefix) && !strings.Contains(strings.TrimPrefix(v.Path, prefix), "/") {
				inDir = append(inDir, v)
			}
		}
		indexRel := "index.md"
		if scope != "" {
			indexRel = strings.TrimSuffix(scope, "/") + "/index.md"
		}
		fullPath := filepath.Join(fs.VaultPath, indexRel)
		if content, err := os.ReadFile(fullPath); err == nil {
			return mcp.NewToolResultText(string(content)), nil
		}
		var sb strings.Builder
		base := scope
		if base == "" {
			base = "Vault"
		} else {
			base = filepath.Base(scope)
		}
		sb.WriteString("# ")
		sb.WriteString(base)
		sb.WriteString(" Index\n\n")
		for _, v := range inDir {
			fmt.Fprintf(&sb, "* [%s](%s.md)", v.Title, v.Path)
			if v.Description != "" {
				fmt.Fprintf(&sb, " - %s", v.Description)
			}
			sb.WriteString("\n")
		}
		return mcp.NewToolResultText(sb.String()), nil
	}
}

func handleListSkills(fs *internal.FileSystem, skillsDirName string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		skillsDir := filepath.Join(fs.VaultPath, skillsDirName)
		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			return mcp.NewToolResultText("[]"), nil
		}
		type SkillEntry struct {
			Name        string   `json:"name"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Path        string   `json:"path"`
			Type        string   `json:"type"`
			Tags        []string `json:"tags,omitempty"`
		}
		var skills []SkillEntry
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".md") {
				continue
			}
			fullPath := filepath.Join(skillsDir, name)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			fm, body := internal.ParseNote(content)
			toolName := ""
			if fm != nil {
				if t, ok := fm["title"]; ok {
					if s, ok := t.(string); ok {
						toolName = s
					}
				}
				if toolName == "" {
					if n, ok := fm["name"]; ok {
						if s, ok := n.(string); ok {
							toolName = s
						}
					}
				}
			}
			if toolName == "" {
				toolName = sanitizeToolName(strings.TrimSuffix(name, ".md"))
			}
			if !isValidToolName(toolName) {
				continue
			}
			description := ""
			if fm != nil {
				if d, ok := fm["description"]; ok {
					if s, ok := d.(string); ok {
						description = s
					}
				}
			}
			if description == "" {
				description = firstNonEmptyParagraph(string(body))
			}
			relPath := skillsDirName + "/" + strings.TrimSuffix(name, ".md")
			okfType := ""
			okfTags := []string{}
			if fm != nil {
				if t, ok := fm["type"]; ok {
					if s, ok := t.(string); ok {
						okfType = s
					}
				}
				if tags, ok := fm["tags"]; ok {
					if tagStr, ok := tags.(string); ok && len(tagStr) > 0 {
						tagStr = strings.Trim(tagStr, "[]")
						for _, t := range strings.Split(tagStr, ",") {
							if t = strings.TrimSpace(t); t != "" {
								okfTags = append(okfTags, t)
							}
						}
					}
				}
			}
			skills = append(skills, SkillEntry{
				Name:        toolName,
				Title:       toolName,
				Description: description,
				Path:        relPath,
				Type:        okfType,
				Tags:        okfTags,
			})
		}
		if skills == nil {
			skills = []SkillEntry{}
		}
		data, _ := json.MarshalIndent(skills, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func isValidToolName(name string) bool {
	if len(name) == 0 {
		return false
	}
	c := name[0]
	if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_') {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

func sanitizeToolName(name string) string {
	var b strings.Builder
	name = strings.ToLower(name)
	for i, r := range name {
		if r >= 'a' && r <= 'z' || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	result := b.String()
	if result == "" || (result[0] >= '0' && result[0] <= '9') {
		return "skill"
	}
	return result
}

func firstNonEmptyParagraph(body string) string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}