package internal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iamangus/vitra/internal/vector"
)

type FileSystem struct {
	VaultPath   string
	VectorStore vector.VectorStore
	live        *LiveSync
	index       *VaultIndex
}

func NewFileSystem(vaultPath string) *FileSystem {
	return &FileSystem{
		VaultPath: vaultPath,
		live:      NewLiveSync(),
	}
}

// SetVectorStore attaches a vector store so that note mutations are auto-indexed.
func (fs *FileSystem) SetVectorStore(store vector.VectorStore) {
	fs.VectorStore = store
}

// NoteData is the shared representation of a parsed note returned by ReadNote.
type NoteData struct {
	Path        string                 `json:"path"`
	Title       string                 `json:"title"`
	Content     string                 `json:"content"`
	Frontmatter map[string]interface{} `json:"frontmatter"`
	HTML        string                 `json:"html,omitempty"`
	Breadcrumbs []map[string]string    `json:"breadcrumbs,omitempty"`
}

// ReadNote reads, parses and renders a note. Shared between web and MCP layers.
func (fs *FileSystem) ReadNote(path string) (*NoteData, error) {
	fullPath, err := safeVaultPath(fs.VaultPath, path+".md")
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("note not found: %s", path)
		}
		return nil, err
	}
	frontmatter, body := parseNote(content)
	html, err := renderMarkdown(body, fs.VaultPath, fs.index)
	if err != nil {
		return nil, err
	}
	return &NoteData{
		Path:        path,
		Title:       filepath.Base(path),
		Content:     string(content),
		Frontmatter: frontmatter,
		HTML:        html,
		Breadcrumbs: buildBreadcrumbs(path),
	}, nil
}

// WriteNote writes or overwrites a note and auto-indexes it in the vector store.
func (fs *FileSystem) WriteNote(path, content string) error {
	fullPath, err := safeVaultPath(fs.VaultPath, path+".md")
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	_, statErr := os.Stat(fullPath)
	isNewNote := os.IsNotExist(statErr)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return err
	}
	if fs.index != nil {
		fs.index.UpdateFile(fs.VaultPath, path)
	}
	fs.autoIndex(path, content)
	fs.NotifyVaultChange([]string{path}, isNewNote, true, true, true)
	return nil
}

// CreateNote creates a new note if it doesn't already exist.
func (fs *FileSystem) CreateNote(path, content string) error {
	fullPath, err := safeVaultPath(fs.VaultPath, path+".md")
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("note already exists: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	if content == "" {
		content = fmt.Sprintf("---\ntitle: %s\n---\n\n", filepath.Base(path))
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return err
	}
	if fs.index != nil {
		fs.index.UpdateFile(fs.VaultPath, path)
	}
	fs.autoIndex(path, content)
	fs.NotifyVaultChange([]string{path}, true, true, true, true)
	return nil
}

// DeleteNote deletes a note (path.md) or a folder at path, and removes it from the index.
func (fs *FileSystem) DeleteNote(path string) error {
	fullPath, err := safeVaultPath(fs.VaultPath, path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	info, statErr := os.Stat(fullPath)
	if statErr != nil {
		return fmt.Errorf("not found: %s", path)
	}

	if !info.IsDir() {
		// It's a file (may or may not have .md suffix). Delete it directly.
		rel := strings.TrimSuffix(filepath.ToSlash(path), ".md")
		if err := os.Remove(fullPath); err != nil {
			return err
		}
		if fs.index != nil {
			fs.index.RemoveFile(rel)
		}
		fs.autoDelete(rel)
		fs.NotifyVaultChange([]string{rel}, true, true, true, true)
		return nil
	}

	// Folder: remove all contained notes from the index, then delete the tree.
	if fs.index != nil || fs.VectorStore != nil {
		filepath.WalkDir(fullPath, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
				return nil
			}
			rel, _ := filepath.Rel(fs.VaultPath, p)
			rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
			if fs.index != nil {
				fs.index.RemoveFile(rel)
			}
			fs.autoDelete(rel)
			return nil
		})
	}
	if err := os.RemoveAll(fullPath); err != nil {
		return err
	}
	fs.NotifyVaultChange([]string{path}, true, true, true, true)
	return nil
}

// RenameNote renames or moves a note/folder and updates the index accordingly.
func (fs *FileSystem) RenameNote(old, new string) error {
	oldFull, err := safeVaultPath(fs.VaultPath, old)
	if err != nil {
		return fmt.Errorf("invalid old path: %w", err)
	}
	newFull, err := safeVaultPath(fs.VaultPath, new)
	if err != nil {
		return fmt.Errorf("invalid new path: %w", err)
	}
	if err := os.Rename(oldFull, newFull); err != nil {
		return err
	}
	relOld := strings.TrimSuffix(filepath.ToSlash(old), ".md")
	relNew := strings.TrimSuffix(filepath.ToSlash(new), ".md")
	if fs.index != nil {
		fs.index.RenameFile(relOld, relNew)
	}
	// Re-index any notes under the new path.
	if fs.VectorStore != nil {
		fs.autoDelete(relOld)
		info, err := os.Stat(newFull)
		if err == nil && !info.IsDir() && strings.HasSuffix(new, ".md") {
			if content, err := os.ReadFile(newFull); err == nil {
				fs.autoIndex(relNew, string(bytes.TrimSpace(content)))
			}
		} else if err == nil && info.IsDir() {
			filepath.WalkDir(newFull, func(p string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
					return nil
				}
				rel, _ := filepath.Rel(fs.VaultPath, p)
				rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
				if content, err := os.ReadFile(p); err == nil {
					fs.autoIndex(rel, string(bytes.TrimSpace(content)))
				}
				return nil
			})
		}
	}
	fs.NotifyVaultChange([]string{relOld, relNew}, true, true, true, true)
	return nil
}

// CreateFolder creates a folder.
func (fs *FileSystem) CreateFolder(path string) error {
	fullPath, err := safeVaultPath(fs.VaultPath, path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return err
	}
	fs.NotifyVaultChange([]string{path}, true, false, false, false)
	return nil
}

// ListNotes returns the file tree for the vault (or a subpath), without
// UI-specific fields like IsActive/IsOpen.
func (fs *FileSystem) ListNotes(subpath string) ([]SimpleFileNode, error) {
	dir := fs.VaultPath
	if subpath != "" {
		dir = filepath.Join(fs.VaultPath, subpath)
	}
	return fs.buildSimpleTree(dir, 0)
}

// SimpleFileNode is a trimmed file tree node shared with MCP consumers.
type SimpleFileNode struct {
	Name     string           `json:"name"`
	Path     string           `json:"path"`
	IsDir    bool             `json:"is_dir"`
	Children []SimpleFileNode `json:"children,omitempty"`
}

// SearchNotes returns substring matches across the vault.
func (fs *FileSystem) SearchNotes(query string) ([]SearchResult, error) {
	query = strings.ToLower(query)
	if query == "" {
		return nil, nil
	}
	if fs.index != nil {
		return fs.index.Search(query), nil
	}
	var results []SearchResult
	err := filepath.Walk(fs.VaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(strings.ToLower(string(content)), query) {
			rel, _ := filepath.Rel(fs.VaultPath, path)
			rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
			results = append(results, SearchResult{
				Path:  rel,
				Title: strings.TrimSuffix(info.Name(), ".md"),
			})
		}
		return nil
	})
	return results, err
}

// NoteBacklinks returns notes that link to the given path.
func (fs *FileSystem) NoteBacklinks(path string) ([]BacklinkResult, error) {
	if fs.index != nil {
		return fs.index.GetBacklinks(path), nil
	}
	var backlinks []BacklinkResult
	targetName := filepath.Base(path)
	err := filepath.Walk(fs.VaultPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(fs.VaultPath, filePath)
		rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		if rel == path {
			return nil
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}
		if strings.Contains(string(content), "[["+targetName+"]]") {
			backlinks = append(backlinks, BacklinkResult{
				Path:  rel,
				Title: strings.TrimSuffix(info.Name(), ".md"),
			})
		}
		return nil
	})
	return backlinks, err
}

// SemanticSearch proxies the vector store's semantic search.
func (fs *FileSystem) SemanticSearch(ctx context.Context, query string, limit int) ([]vector.SearchResult, error) {
	if fs.VectorStore == nil {
		return nil, fmt.Errorf("vector store not configured")
	}
	return fs.VectorStore.SemanticSearch(ctx, query, limit)
}

// FindSimilarFiles proxies the vector store's similar-file search.
func (fs *FileSystem) FindSimilarFiles(ctx context.Context, path string, limit int) ([]vector.SearchResult, error) {
	if fs.VectorStore == nil {
		return nil, fmt.Errorf("vector store not configured")
	}
	return fs.VectorStore.FindSimilarFiles(ctx, path, limit)
}

// ReindexVault rebuilds the entire vector index from the on-disk vault.
func (fs *FileSystem) ReindexVault(ctx context.Context) (int, error) {
	if fs.VectorStore == nil {
		return 0, fmt.Errorf("vector store not configured")
	}
	if err := fs.VectorStore.ReindexVault(ctx, fs.VaultPath); err != nil {
		return 0, err
	}
	var indexed int
	err := filepath.Walk(fs.VaultPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(fs.VaultPath, filePath)
		rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}
		chunks := vector.ChunkNote(rel, string(content), 0, 0)
		if err := fs.VectorStore.IndexNote(ctx, rel, chunks); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to index %s: %v\n", rel, err)
		} else {
			indexed++
		}
		return nil
	})
	return indexed, err
}

// autoIndex indexes a note's content into the vector store, logging failures.
func (fs *FileSystem) autoIndex(path, content string) {
	if fs.VectorStore == nil {
		return
	}
	chunks := vector.ChunkNote(path, content, 0, 0)
	if err := fs.VectorStore.IndexNote(context.Background(), path, chunks); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to index note %s: %v\n", path, err)
	}
}

// autoDelete removes a note from the vector store, logging failures.
func (fs *FileSystem) autoDelete(path string) {
	if fs.VectorStore == nil {
		return
	}
	if err := fs.VectorStore.DeleteNote(context.Background(), path); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to delete note %s from index: %v\n", path, err)
	}
}

func (fs *FileSystem) buildSimpleTree(dir string, depth int) ([]SimpleFileNode, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var nodes []SimpleFileNode
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		fullPath := filepath.Join(dir, name)
		relPath, _ := filepath.Rel(fs.VaultPath, fullPath)
		relPath = filepath.ToSlash(relPath)
		node := SimpleFileNode{
			Name:  strings.TrimSuffix(name, ".md"),
			Path:  strings.TrimSuffix(relPath, ".md"),
			IsDir: entry.IsDir(),
		}
		if entry.IsDir() {
			children, err := fs.buildSimpleTree(fullPath, depth+1)
			if err == nil {
				node.Children = children
			}
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})
	return nodes, nil
}

func (fs *FileSystem) BuildIndex() error {
	idx := NewVaultIndex()
	if err := idx.Build(fs.VaultPath); err != nil {
		return err
	}
	fs.index = idx
	if fs.live != nil {
		fs.live.SetIndex(idx)
	}
	return nil
}

func (fs *FileSystem) StartWatcher() error {
	if fs.live == nil {
		return nil
	}
	return fs.live.Start(fs.VaultPath)
}

func (fs *FileSystem) CloseWatcher() error {
	if fs.live == nil {
		return nil
	}
	return fs.live.Close()
}

func (fs *FileSystem) NotifyVaultChange(paths []string, tree bool, graph bool, search bool, notes bool) {
	if fs.live == nil {
		return
	}
	fs.live.Notify(paths, tree, graph, search, notes)
}

type FileNode struct {
	Name     string
	Path     string
	IsDir    bool
	IsActive bool
	IsOpen   bool
	Depth    int
	Children []FileNode
}

func (fs *FileSystem) buildTree(dir string, activePath string, depth int) ([]FileNode, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var nodes []FileNode
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		fullPath := filepath.Join(dir, name)
		relPath, _ := filepath.Rel(fs.VaultPath, fullPath)
		relPath = filepath.ToSlash(relPath)

		node := FileNode{
			Name:     strings.TrimSuffix(name, ".md"),
			Path:     strings.TrimSuffix(relPath, ".md"),
			IsDir:    entry.IsDir(),
			IsActive: strings.TrimSuffix(relPath, ".md") == activePath,
			IsOpen:   depth < 2,
			Depth:    depth,
		}

		if entry.IsDir() {
			children, err := fs.buildTree(fullPath, activePath, depth+1)
			if err == nil {
				node.Children = children
				for _, c := range children {
					if c.IsActive || c.IsOpen {
						node.IsOpen = true
						break
					}
				}
			}
		}

		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})

	return nodes, nil
}
