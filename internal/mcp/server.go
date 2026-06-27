package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/iamangus/vitra/internal"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// StartToolsServer starts the "tools" MCP server, exposing the vault file
// operations plus the semantic vector tools. It blocks until the server stops.
func StartToolsServer(fs *internal.FileSystem, port string) error {
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
		mcp.WithDescription("Create a new note (fails if it already exists)"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Vault-relative path to the note (without .md extension)")),
		mcp.WithString("content", mcp.Description("Optional initial content (defaults to frontmatter with title)")),
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
		mcp.WithDescription("Semantic search across the vault using vector similarity"),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
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

	return server.NewStreamableHTTPServer(s).Start(":" + port)
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
		content := req.GetString("content", "")
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
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
		results, err := fs.SemanticSearch(ctx, query, limit)
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
		results, err := fs.FindSimilarFiles(ctx, path, limit)
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
		results, err := fs.FindSimilarFiles(ctx, path, limit)
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