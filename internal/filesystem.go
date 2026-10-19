package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iamangus/vitra/internal/links"
	"github.com/iamangus/vitra/internal/okf"
	"github.com/iamangus/vitra/internal/vector"
	"gopkg.in/yaml.v3"
)

type FileSystem struct {
	VaultPath     string
	VectorStore   vector.VectorStore
	live          *LiveSync
	index         *VaultIndex
	skillsDirRel  string
	chunkSize     int
	chunkOverlap  int
	mutationMu    sync.Mutex
	statusMu      sync.Mutex
	indexError    string
	indexRunMu    sync.Mutex
	indexQueue    chan struct{}
	indexCancel   context.CancelFunc
	indexDone     chan struct{}
	indexedHashes map[string]string
}

func NewFileSystem(vaultPath string) *FileSystem {
	if abs, err := filepath.Abs(vaultPath); err == nil {
		vaultPath = abs
	}
	return &FileSystem{
		VaultPath:    vaultPath,
		live:         NewLiveSync(),
		skillsDirRel: "skills",
	}
}

// SetVectorStore attaches a vector store so that note mutations are auto-indexed.
func (fs *FileSystem) SetVectorStore(store vector.VectorStore) {
	fs.VectorStore = store
	if store != nil {
		fs.startIndexer()
	}
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
	Revision    string                 `json:"revision"`
}

// ReadNote reads, parses and renders a note. Shared between web and MCP layers.
func (fs *FileSystem) ReadNote(path string) (*NoteData, error) {
	path = noteName(path)
	content, err := fs.ReadVaultFile(path + ".md")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("note not found: %w", err)
		}
		return nil, err
	}
	frontmatter, body := parseNote(content)
	html, err := renderMarkdown(body, fs.VaultPath, fs.index, filepath.Dir(path))
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
		Revision:    revision(content),
		Type:        meta.Type,
		Title:       title,
		Description: meta.Description,
		Resource:    meta.Resource,
		Tags:        meta.Tags,
		Timestamp:   meta.Timestamp,
		Links:       fs.noteTargets(path, body),
		Content:     string(content),
		Frontmatter: frontmatter,
		HTML:        html,
		Breadcrumbs: buildBreadcrumbs(path),
	}, nil
}

// WriteNote writes or overwrites a note and auto-indexes it in the vector store.
func (fs *FileSystem) WriteNote(path, content string) error {
	_, err := fs.SaveNote(path, content, "", false)
	return err
}

func (fs *FileSystem) SaveNote(path, content, expected string, exclusive bool) (string, error) {
	fs.mutationMu.Lock()
	defer fs.mutationMu.Unlock()
	return fs.saveNoteLocked(path, content, expected, exclusive)
}

func (fs *FileSystem) saveNoteLocked(path, content, expected string, exclusive bool) (string, error) {
	path = noteName(path)
	if path == "" || len(content) > maxBodySize {
		return "", ErrInvalidPath
	}
	root, rel, err := fs.rootPath(path+".md", true)
	if err != nil {
		return "", err
	}
	defer root.Close()
	previous, readErr := root.ReadFile(rel)
	if readErr != nil && !os.IsNotExist(readErr) {
		return "", readErr
	}
	if expected != "" && (readErr != nil || revision(previous) != expected) {
		return "", ErrConflict
	}
	if err := atomicNote(root, rel, []byte(content), exclusive); err != nil {
		return "", err
	}
	if fs.index != nil {
		fs.index.UpdateFile(fs.VaultPath, path)
	}
	fs.autoIndex(path, content)
	fs.NotifyVaultChange([]string{path}, os.IsNotExist(readErr), true, true, true)
	return revision([]byte(content)), nil
}

// CreateNote creates a new note if it doesn't already exist.
func (fs *FileSystem) CreateNote(path, content string) error {
	if content == "" {
		content = EmitFrontmatter(map[string]interface{}{"type": "Note", "title": filepath.Base(noteName(path))}) + "\n"
	}
	_, err := fs.SaveNote(path, content, "", true)
	return err
}

// DeleteNote deletes a note (path.md) or a folder at path, and removes it from the index.
func (fs *FileSystem) DeleteNote(path string) error {
	fs.mutationMu.Lock()
	defer fs.mutationMu.Unlock()
	root, rel, _, err := fs.resolveEntry(path)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.RemoveAll(rel); err != nil {
		return err
	}
	if fs.index != nil {
		fs.index.RemoveFile(noteName(rel))
	}
	fs.autoDelete(noteName(rel))
	fs.NotifyVaultChange([]string{path}, true, true, true, true)
	return nil
}

// RenameNote renames or moves a note/folder and updates the index accordingly.
func (fs *FileSystem) RenameNote(old, new string) error {
	fs.mutationMu.Lock()
	defer fs.mutationMu.Unlock()
	root, oldRel, info, err := fs.resolveEntry(old)
	if err != nil {
		return err
	}
	defer root.Close()
	if !info.IsDir() {
		new = noteName(new) + ".md"
	}
	newRoot, newRel, err := fs.rootPath(new, true)
	if err != nil {
		return err
	}
	defer newRoot.Close()
	if _, err := root.Stat(newRel); err == nil {
		return ErrConflict
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := root.MkdirAll(filepath.Dir(newRel), 0755); err != nil {
		return err
	}
	if err := renameNoReplace(root, oldRel, newRel); err != nil {
		return err
	}
	relOld, relNew := noteName(oldRel), noteName(newRel)
	if fs.index != nil {
		fs.index.RemoveFile(relOld)
		if err := fs.index.Build(fs.VaultPath); err != nil {
			return err
		}
	}
	fs.autoDelete(relOld)
	fs.indexSubtree(relNew)
	fs.NotifyVaultChange([]string{relOld, relNew}, true, true, true, true)
	return nil
}

// CreateFolder creates a folder.
func (fs *FileSystem) CreateFolder(path string) error {
	fs.mutationMu.Lock()
	defer fs.mutationMu.Unlock()
	root, rel, err := fs.rootPath(path, true)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	defer root.Close()
	if err := root.MkdirAll(rel, 0755); err != nil {
		return err
	}
	fs.NotifyVaultChange([]string{path}, true, false, false, false)
	return nil
}

// ListNotes returns the file tree for the vault (or a subpath), without
// UI-specific fields like IsActive/IsOpen.
func (fs *FileSystem) ListNotes(subpath string) ([]SimpleFileNode, error) {
	dir, err := safeVaultPath(fs.VaultPath, subpath)
	if err != nil {
		return nil, err
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
	idx := NewVaultIndex()
	idx.SetSkillsDir(fs.skillsDirRel)
	if err := idx.Build(fs.VaultPath); err != nil {
		return nil, err
	}
	return idx.Search(query), nil
}

// NoteBacklinks returns notes that link to the given path.
func (fs *FileSystem) NoteBacklinks(path string) ([]BacklinkResult, error) {
	if fs.index != nil {
		return fs.index.GetBacklinks(path), nil
	}
	idx := NewVaultIndex()
	idx.SetSkillsDir(fs.skillsDirRel)
	if err := idx.Build(fs.VaultPath); err != nil {
		return nil, err
	}
	return idx.GetBacklinks(path), nil
}

// SemanticSearch proxies the vector store's semantic search.
func (fs *FileSystem) SemanticSearch(ctx context.Context, query string, limit int, filter vector.Filter) ([]vector.SearchResult, error) {
	if fs.VectorStore == nil {
		return nil, fmt.Errorf("vector store not configured")
	}
	results, err := fs.VectorStore.SemanticSearch(ctx, query, limit, filter)
	if err != nil {
		return nil, err
	}
	return fs.visibleSearchResults(results), nil
}

// FindSimilarFiles proxies the vector store's similar-file search.
func (fs *FileSystem) FindSimilarFiles(ctx context.Context, path string, limit int, filter vector.Filter) ([]vector.SearchResult, error) {
	if fs.VectorStore == nil {
		return nil, fmt.Errorf("vector store not configured")
	}
	results, err := fs.VectorStore.FindSimilarFiles(ctx, path, limit, filter)
	if err != nil {
		return nil, err
	}
	return fs.visibleSearchResults(results), nil
}

func (fs *FileSystem) visibleSearchResults(results []vector.SearchResult) []vector.SearchResult {
	out := make([]vector.SearchResult, 0, len(results))
	for _, result := range results {
		if excludedPath(result.Path+".md", fs.skillsDirRel, true) {
			continue
		}
		root, rel, err := fs.rootPath(result.Path+".md", false)
		if err != nil {
			continue
		}
		info, err := root.Stat(rel)
		root.Close()
		if err == nil && info.Mode().IsRegular() {
			out = append(out, result)
		}
	}
	return out
}

// ReindexVault rebuilds the entire vector index from the on-disk vault.
func (fs *FileSystem) ReindexVault(ctx context.Context) (int, error) {
	if fs.VectorStore == nil {
		return 0, fmt.Errorf("vector store not configured")
	}
	fs.indexRunMu.Lock()
	defer fs.indexRunMu.Unlock()
	fs.mutationMu.Lock()
	defer fs.mutationMu.Unlock()
	notes := map[string][]vector.Chunk{}
	hashes := map[string]string{}
	err := walkNotes(fs.VaultPath, "", fs.skillsDirRel, true, func(path string, data []byte) error {
		notes[path] = fs.noteChunks(path, data)
		hashes[path] = revision(data)
		return ctx.Err()
	})
	if err != nil {
		return 0, err
	}
	replacer, ok := fs.VectorStore.(interface {
		ReplaceNotes(context.Context, map[string][]vector.Chunk) error
	})
	if !ok {
		return 0, fmt.Errorf("store does not support atomic reindex")
	}
	if err := replacer.ReplaceNotes(ctx, notes); err != nil {
		fs.recordIndexError(err)
		return 0, err
	}
	fs.indexedHashes = hashes
	fs.statusMu.Lock()
	fs.indexError = ""
	fs.statusMu.Unlock()
	return len(notes), nil
}

// autoIndex indexes a note's content into the vector store, logging failures.
// OKF metadata (type, tags) is extracted from the note's frontmatter so that
// type/tag-filtered queries match correctly.
func (fs *FileSystem) autoIndex(path, content string) {
	fs.queueIndex()
}

// autoDelete removes a note from the vector store, logging failures.
func (fs *FileSystem) autoDelete(path string) {
	fs.queueIndex()
}

func (fs *FileSystem) buildSimpleTree(dir string, depth int) ([]SimpleFileNode, error) {
	entries, err := fs.readDirectory(dir)
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
		if excludedPath(relPath, fs.skillsDirRel, true) || entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !strings.HasSuffix(name, ".md")) {
			continue
		}
		node := SimpleFileNode{
			Name:  strings.TrimSuffix(name, ".md"),
			Path:  strings.TrimSuffix(relPath, ".md"),
			IsDir: entry.IsDir(),
		}
		if entry.IsDir() {
			node.Path = relPath
			node.Name = name
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
	if err := os.MkdirAll(fs.VaultPath, 0755); err != nil {
		return err
	}
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
	if fs.indexCancel != nil {
		fs.indexCancel()
		<-fs.indexDone
	}
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
	entries, err := fs.readDirectory(dir)
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

		if excludedPath(relPath, fs.skillsDirRel, true) || entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !strings.HasSuffix(name, ".md")) {
			continue
		}
		node := FileNode{
			Name:     strings.TrimSuffix(name, ".md"),
			Path:     strings.TrimSuffix(relPath, ".md"),
			IsDir:    entry.IsDir(),
			IsActive: strings.TrimSuffix(relPath, ".md") == activePath,
			IsOpen:   depth < 2,
			Depth:    depth,
		}
		if entry.IsDir() {
			node.Path = relPath
			node.Name = name
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
	if rootIdx, err := fs.ReadVaultFile("index.md"); err == nil {
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
	err := walkNotes(fs.VaultPath, "", fs.skillsDirRel, true, func(rel string, content []byte) error {
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
		if filter.Since != "" {
			since, err := time.Parse(time.RFC3339, filter.Since)
			if err != nil {
				return err
			}
			timestamp, err := time.Parse(time.RFC3339, meta.Timestamp)
			if err != nil || timestamp.Before(since) {
				return nil
			}
		}
		title := meta.Title
		if title == "" {
			title = filepath.Base(rel)
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
	var entries []ActivityEntry
	err := walkNotes(fs.VaultPath, scope, fs.skillsDirRel, false, func(rel string, content []byte) error {
		if filepath.Base(rel) != "log" {
			return nil
		}
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
	_, err := fs.PatchNote(path, updates, nil, "")
	return err
}

func (fs *FileSystem) PatchNote(path string, updates map[string]interface{}, replacement *string, expected string) (string, error) {
	fs.mutationMu.Lock()
	defer fs.mutationMu.Unlock()
	content, err := fs.ReadVaultFile(noteName(path) + ".md")
	if err != nil {
		return "", err
	}
	fm, body, err := splitFrontmatter(content)
	if err != nil {
		return "", err
	}
	node := yaml.Node{Kind: yaml.MappingNode}
	if fm != nil {
		var validated map[string]interface{}
		if err := yaml.Unmarshal(fm, &validated); err != nil {
			return "", err
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(fm, &doc); err != nil {
			return "", err
		}
		if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
			return "", fmt.Errorf("frontmatter must be a mapping")
		}
		node = *doc.Content[0]
	}
	merged := make(map[string]interface{}, len(updates)+1)
	for k, v := range updates {
		merged[k] = v
	}
	if _, ok := merged["timestamp"]; !ok {
		merged["timestamp"] = okf.FormatTimestamp(time.Now())
	} else {
		timestamp, ok := merged["timestamp"].(string)
		if !ok {
			return "", ErrInvalidPath
		}
		if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
			return "", ErrInvalidPath
		}
	}
	for key, value := range merged {
		var val yaml.Node
		if err := val.Encode(value); err != nil {
			return "", err
		}
		found := false
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				node.Content[i+1] = &val
				found = true
				break
			}
		}
		if !found {
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &val)
		}
	}
	encoded, err := yaml.Marshal(&node)
	if err != nil {
		return "", err
	}
	if replacement != nil {
		body = []byte(*replacement)
	}
	return fs.saveNoteLocked(path, "---\n"+string(encoded)+"---\n"+string(body), expected, false)
}

// EmitFrontmatter renders a frontmatter map back into a `---\n...\n---\n` YAML
// block. Keys are emitted in OKF-canonical order (type, title, description,
// resource, tags, timestamp, okf_version), then any extra keys. Values are
// quoted only when they contain YAML special characters.
func EmitFrontmatter(fm map[string]interface{}) string {
	data, err := yaml.Marshal(fm)
	if err != nil {
		return ""
	}
	return "---\n" + string(data) + "---\n"
}

// EnsureOKFFrontmatter guarantees OKF-compliant frontmatter for new notes.
//
// When content already has frontmatter:
//   - If type is present → keep it (directive: don't overwrite caller-provided type)
//   - If type is absent → inject type: Note
//   - summary is always injected/updated
//
// When content has no frontmatter:
//   - Synthesize one from okf fields (or defaults: type="Note", title from path)
//
// When content is empty:
//   - Synthesize from okf fields via EmitFrontmatter (same as before, but with summary)
//
// Returns the content string with compliant frontmatter prepended when needed.
func EnsureOKFFrontmatter(content, summary, path string, okfFields map[string]interface{}) string {
	titleFromPath := filepath.Base(path)

	if content == "" {
		if okfFields == nil {
			okfFields = make(map[string]interface{})
		}
		if _, ok := okfFields["type"]; !ok {
			okfFields["type"] = okf.DefaultType
		}
		if _, ok := okfFields["title"]; !ok {
			okfFields["title"] = titleFromPath
		}
		okfFields["summary"] = summary
		return EmitFrontmatter(okfFields) + "\n"
	}

	frontmatter, bodyBytes := parseNote([]byte(content))
	body := string(bodyBytes)
	hasType := false
	if frontmatter != nil {
		if v, ok := frontmatter["type"]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				hasType = true
			}
		}
	}

	if frontmatter == nil {
		frontmatter = make(map[string]interface{})
	}
	if !hasType {
		frontmatter["type"] = okf.DefaultType
	}
	frontmatter["summary"] = summary
	for k, v := range okfFields {
		if _, exists := frontmatter[k]; !exists {
			frontmatter[k] = v
		}
	}

	return EmitFrontmatter(frontmatter) + "\n" + body
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
			targets := fs.noteTargets(p, body)
			for _, link := range targets {
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

func (fs *FileSystem) noteTargets(path string, body []byte) []string {
	targets := links.Extract(body, filepath.Dir(path), true)
	out := make([]string, 0, len(targets))
	seen := map[string]bool{}
	for _, target := range targets {
		if fs.index != nil {
			if resolved := fs.index.FindPath(target); resolved != "" {
				target = resolved
			}
		}
		if !seen[target] && !excludedPath(target+".md", fs.skillsDirRel, true) {
			seen[target] = true
			out = append(out, target)
		}
	}
	return out
}
