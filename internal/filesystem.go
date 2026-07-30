package internal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iamangus/vitra/internal/okf"
	"github.com/iamangus/vitra/internal/vector"
)

type FileSystem struct {
	VaultPath    string
	VectorStore  vector.VectorStore
	live         *LiveSync
	index        *VaultIndex
	skillsDirRel string
	chunkSize    int
	chunkOverlap int
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

// SetChunkConfig overrides the default note chunking parameters. Pass 0 to
// keep the hardcoded defaults (1000 chars chunk, 200 chars overlap).
func (fs *FileSystem) SetChunkConfig(chunkSize, chunkOverlap int) {
	fs.chunkSize = chunkSize
	fs.chunkOverlap = chunkOverlap
}

func (fs *FileSystem) SetSkillsDir(rel string) {
	fs.skillsDirRel = rel
}

func (fs *FileSystem) isSkillsPath(relPath string) bool {
	if fs.skillsDirRel == "" {
		return false
	}
	return strings.HasPrefix(relPath+"/", fs.skillsDirRel+"/")
}

// NoteData is the shared representation of a parsed note returned by ReadNote.
type NoteData struct {
	Path        string                 `json:"path"`
	Type        string                 `json:"type"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Resource    string                 `json:"resource,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Timestamp   string                 `json:"timestamp,omitempty"`
	Links       []string               `json:"links,omitempty"`
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
	meta := okf.Extract(frontmatter)
	title := meta.Title
	if title == "" {
		title = filepath.Base(path)
	}
	return &NoteData{
		Path:        path,
		Type:        meta.Type,
		Title:       title,
		Description: meta.Description,
		Resource:    meta.Resource,
		Tags:        meta.Tags,
		Timestamp:   meta.Timestamp,
		Links:       okf.ExtractOKFLinks(body, filepath.Dir(path)),
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
		content = fmt.Sprintf("---\ntype: Note\ntitle: %s\n---\n\n", filepath.Base(path))
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
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && fs.skillsDirRel != "" {
				rel, rdErr := filepath.Rel(fs.VaultPath, filePath)
				if rdErr == nil && rel == fs.skillsDirRel {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
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
func (fs *FileSystem) SemanticSearch(ctx context.Context, query string, limit int, filter vector.Filter) ([]vector.SearchResult, error) {
	if fs.VectorStore == nil {
		return nil, fmt.Errorf("vector store not configured")
	}
	return fs.VectorStore.SemanticSearch(ctx, query, limit, filter)
}

// FindSimilarFiles proxies the vector store's similar-file search.
func (fs *FileSystem) FindSimilarFiles(ctx context.Context, path string, limit int, filter vector.Filter) ([]vector.SearchResult, error) {
	if fs.VectorStore == nil {
		return nil, fmt.Errorf("vector store not configured")
	}
	return fs.VectorStore.FindSimilarFiles(ctx, path, limit, filter)
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
		frontmatter, _ := parseNote(content)
		title := ""
		if t, ok := frontmatter["title"].(string); ok {
			title = t
		}
		meta := okf.Extract(frontmatter)
		chunks := vector.ChunkNote(rel, string(content), fs.chunkSize, fs.chunkOverlap, title)
		for i := range chunks {
			chunks[i].Type = meta.Type
			chunks[i].Tags = meta.Tags
		}
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
// OKF metadata (type, tags) is extracted from the note's frontmatter so that
// type/tag-filtered queries match correctly.
func (fs *FileSystem) autoIndex(path, content string) {
	if fs.VectorStore == nil || fs.isSkillsPath(path) {
		return
	}
	frontmatter, _ := parseNote([]byte(content))
	title := ""
	if t, ok := frontmatter["title"].(string); ok {
		title = t
	}
	meta := okf.Extract(frontmatter)
	chunks := vector.ChunkNote(path, content, fs.chunkSize, fs.chunkOverlap, title)
	for i := range chunks {
		chunks[i].Type = meta.Type
		chunks[i].Tags = meta.Tags
	}
	if err := fs.VectorStore.IndexNote(context.Background(), path, chunks); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to index note %s: %v\n", path, err)
	}
}

// autoDelete removes a note from the vector store, logging failures.
func (fs *FileSystem) autoDelete(path string) {
	if fs.VectorStore == nil || fs.isSkillsPath(path) {
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
		// Suppress OKF reserved filenames from the sidebar tree (spec §3.1).
		if !entry.IsDir() && okf.IsReservedFilename(name) {
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
	idx.SetSkillsDir(fs.skillsDirRel)
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
		// Suppress OKF reserved filenames from the sidebar tree (spec §3.1).
		if !entry.IsDir() && okf.IsReservedFilename(name) {
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

// ConceptView is the OKF concept listing projection returned by ListConcepts.
type ConceptView struct {
	Path        string   `json:"path"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Resource    string   `json:"resource,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Timestamp   string   `json:"timestamp,omitempty"`
}

// ConceptFilter narrows ListConcepts results. Empty/zero fields mean "no
// constraint". Tags use AND semantics: only notes carrying every requested
// tag match.
type ConceptFilter struct {
	Type     string
	Tag      []string
	Resource string
	Since    string // ISO 8601; only notes with timestamp >= Since match
	Limit    int
}

// ListConcepts walks the vault and returns the OKF concept projection for
// each non-reserved .md file. Filters apply in-memory after parsing frontmatter.
// The second return value is the vault-root index.md's declared okf_version.
func (fs *FileSystem) ListConcepts(filter ConceptFilter) ([]ConceptView, string, error) {
	okfVersion := ""
	if rootIdx, err := os.ReadFile(filepath.Join(fs.VaultPath, "index.md")); err == nil {
		if fm, _ := parseNote(rootIdx); fm != nil {
			if v, ok := fm["okf_version"]; ok {
				if s, ok := v.(string); ok {
					okfVersion = s
				}
			}
		}
	}

	limit := filter.Limit
	var views []ConceptView
	err := filepath.Walk(fs.VaultPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && fs.skillsDirRel != "" {
				rel, rdErr := filepath.Rel(fs.VaultPath, filePath)
				if rdErr == nil && rel == fs.skillsDirRel {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		if okf.IsReservedFilename(info.Name()) {
			return nil
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}
		frontmatter, _ := parseNote(content)
		meta := okf.Extract(frontmatter)

		if filter.Type != "" && meta.Type != filter.Type {
			return nil
		}
		if filter.Resource != "" && meta.Resource != filter.Resource {
			return nil
		}
		if len(filter.Tag) > 0 {
			has := make(map[string]bool, len(meta.Tags))
			for _, t := range meta.Tags {
				has[t] = true
			}
			for _, want := range filter.Tag {
				if !has[want] {
					return nil
				}
			}
		}
		if filter.Since != "" && meta.Timestamp != "" && meta.Timestamp < filter.Since {
			return nil
		}

		rel, _ := filepath.Rel(fs.VaultPath, filePath)
		rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		title := meta.Title
		if title == "" {
			title = strings.TrimSuffix(info.Name(), ".md")
		}
		views = append(views, ConceptView{
			Path:        rel,
			Type:        meta.Type,
			Title:       title,
			Description: meta.Description,
			Resource:    meta.Resource,
			Tags:        meta.Tags,
			Timestamp:   meta.Timestamp,
		})
		return nil
	})
	if err != nil {
		return nil, okfVersion, err
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Type != views[j].Type {
			return views[i].Type < views[j].Type
		}
		return views[i].Path < views[j].Path
	})
	if limit > 0 && len(views) > limit {
		views = views[:limit]
	}
	return views, okfVersion, nil
}

// ActivityEntry is one parsed entry from any log.md file in the vault.
type ActivityEntry struct {
	Source     string `json:"source"`
	SourcePath string `json:"source_path"`
	Date       string `json:"date"`
	Kind       string `json:"kind"`
	Text       string `json:"text"`
}

// ListActivity walks the vault for log.md files (any depth), parses each via
// okf.ParseLogEntries, and returns the merged set sorted newest first.
func (fs *FileSystem) ListActivity(scope string, limit int) ([]ActivityEntry, error) {
	root := fs.VaultPath
	if scope != "" {
		root = filepath.Join(fs.VaultPath, scope)
	}
	var entries []ActivityEntry
	err := filepath.Walk(root, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && fs.skillsDirRel != "" {
				rel, rdErr := filepath.Rel(fs.VaultPath, filePath)
				if rdErr == nil && rel == fs.skillsDirRel {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if info.Name() != "log.md" {
			return nil
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(fs.VaultPath, filePath)
		rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		sourcePath := filepath.Dir(rel)
		for _, e := range okf.ParseLogEntries(content) {
			entries = append(entries, ActivityEntry{
				Source:     rel,
				SourcePath: sourcePath,
				Date:       e.Date,
				Kind:       e.Kind,
				Text:       e.Text,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Date > entries[j].Date })
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

// UpdateNoteMetadata parses the note's frontmatter, merges the provided
// key/value pairs (overwriting existing keys), re-emits the frontmatter block
// while preserving the body verbatim. If `timestamp` is not in updates, the
// current UTC time is injected. The note is re-indexed and live notified.
func (fs *FileSystem) UpdateNoteMetadata(path string, updates map[string]interface{}) error {
	fullPath, err := safeVaultPath(fs.VaultPath, path+".md")
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("note not found: %s", path)
		}
		return err
	}
	frontmatter, body := parseNote(content)
	if frontmatter == nil {
		frontmatter = map[string]interface{}{}
	}
	for k, v := range updates {
		frontmatter[k] = v
	}
	if _, ok := frontmatter["timestamp"]; !ok {
		frontmatter["timestamp"] = okf.FormatTimestamp(time.Now())
	}
	newContent := EmitFrontmatter(frontmatter) + string(body)
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return err
	}
	if fs.index != nil {
		fs.index.UpdateFile(fs.VaultPath, path)
	}
	fs.autoIndex(path, newContent)
	fs.NotifyVaultChange([]string{path}, false, true, true, true)
	return nil
}

// EmitFrontmatter renders a frontmatter map back into a `---\n...\n---\n` YAML
// block. Keys are emitted in OKF-canonical order (type, title, description,
// resource, tags, timestamp, okf_version), then any extra keys. Values are
// quoted only when they contain YAML special characters.
func EmitFrontmatter(fm map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("---\n")
	order := []string{"type", "title", "description", "resource", "tags", "timestamp", "okf_version"}
	written := make(map[string]bool)
	for _, k := range order {
		if v, ok := fm[k]; ok {
			writeYamlKV(&b, k, v)
			written[k] = true
		}
	}
	keys := make([]string, 0, len(fm))
	for k := range fm {
		if !written[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		writeYamlKV(&b, k, fm[k])
	}
	b.WriteString("---\n")
	return b.String()
}

func writeYamlKV(b *strings.Builder, k string, v interface{}) {
	switch val := v.(type) {
	case []interface{}:
		b.WriteString(k)
		b.WriteString(": [")
		for i, item := range val {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(yamlQuote(fmt.Sprintf("%v", item)))
		}
		b.WriteString("]\n")
	case []string:
		b.WriteString(k)
		b.WriteString(": [")
		for i, item := range val {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(yamlQuote(item))
		}
		b.WriteString("]\n")
	default:
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(yamlQuote(fmt.Sprintf("%v", val)))
		b.WriteString("\n")
	}
}

func yamlQuote(s string) string {
	if s == "" || strings.ContainsAny(s, ":#[]{}\n\"'") {
		return strconv.Quote(s)
	}
	return s
}

// TransitiveClosure performs a BFS over the vault's OKF link graph starting
// from the given seed concept, returning all reachable concepts up to `depth`
// hops (inclusive). The seed itself is NOT included unless it is reachable
// via a cycle. Edges are derived from each note's outgoing OKF cross-links
// (okf.ExtractOKFLinks). Missing targets are tolerated (spec §5.3).
func (fs *FileSystem) TransitiveClosure(seed string, depth int) ([]ConceptView, error) {
	if depth <= 0 {
		depth = 1
	}
	visited := map[string]bool{seed: true}
	frontier := []string{seed}
	var found []ConceptView
	views, _, err := fs.ListConcepts(ConceptFilter{})
	if err != nil {
		return nil, err
	}
	viewByPath := make(map[string]ConceptView, len(views))
	for _, v := range views {
		viewByPath[v.Path] = v
	}

	for d := 0; d < depth && len(frontier) > 0; d++ {
		var next []string
		for _, p := range frontier {
			fullPath, err := safeVaultPath(fs.VaultPath, p+".md")
			if err != nil {
				continue
			}
			content, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			_, body := parseNote(content)
			links := okf.ExtractOKFLinks(body, filepath.Dir(p))
			for _, link := range links {
				if visited[link] {
					continue
				}
				visited[link] = true
				if v, ok := viewByPath[link]; ok {
					found = append(found, v)
					next = append(next, link)
				}
			}
		}
		frontier = next
	}
	return found, nil
}
