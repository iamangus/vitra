package internal

import (
	"github.com/iamangus/vitra/internal/links"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type NoteMeta struct {
	Path     string
	Title    string
	Type     string
	RawLinks []string
	Content  string
}

type VaultIndex struct {
	mu           sync.RWMutex
	notes        map[string]*NoteMeta
	titleIndex   map[string]string
	skillsDirRel string
}

func NewVaultIndex() *VaultIndex {
	return &VaultIndex{
		notes:      make(map[string]*NoteMeta),
		titleIndex: make(map[string]string),
	}
}

func (idx *VaultIndex) SetSkillsDir(rel string) {
	idx.skillsDirRel = rel
}

func (idx *VaultIndex) rebuildAliases() {
	idx.titleIndex = map[string]string{}
	add := func(alias, target string) {
		key := strings.ToLower(alias)
		if previous, ok := idx.titleIndex[key]; ok && previous != target {
			idx.titleIndex[key] = ""
		} else {
			idx.titleIndex[key] = target
		}
	}
	paths := make([]string, 0, len(idx.notes))
	for p := range idx.notes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		meta := idx.notes[p]
		add(meta.Title, p)
		add(filepath.Base(p), p)
	}
	for _, p := range paths {
		idx.titleIndex[strings.ToLower(p)] = p
	}
}

func (idx *VaultIndex) Build(vaultPath string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	previousNotes, previousTitles := idx.notes, idx.titleIndex
	idx.notes, idx.titleIndex = make(map[string]*NoteMeta), make(map[string]string)
	err := walkNotes(vaultPath, "", idx.skillsDirRel, true, func(rel string, content []byte) error {
		idx.indexContent(rel, content)
		return nil
	})
	if err != nil {
		idx.notes, idx.titleIndex = previousNotes, previousTitles
	} else {
		idx.rebuildAliases()
	}
	return err
}

func (idx *VaultIndex) indexFile(vaultPath, fullPath, relPath, title string) {
	root, err := os.OpenRoot(vaultPath)
	if err != nil {
		return
	}
	defer root.Close()
	content, err := root.ReadFile(relPath + ".md")
	if err != nil {
		return
	}

	idx.indexContent(relPath, content)
	idx.rebuildAliases()
}

func (idx *VaultIndex) indexContent(relPath string, content []byte) {
	title := filepath.Base(relPath)
	meta := &NoteMeta{
		Path:     relPath,
		Title:    title,
		Type:     "Note",
		Content:  string(content),
		RawLinks: extractWikiLinkTargets(string(content)),
	}
	if fm, _ := parseNote(content); fm != nil {
		if t, ok := fm["title"].(string); ok && t != "" {
			meta.Title = t
		}
		if t, ok := fm["type"]; ok {
			if s, ok := t.(string); ok && s != "" {
				meta.Type = s
			}
		}
	}
	_, body := parseNote(content)
	meta.RawLinks = links.Extract(body, filepath.Dir(relPath), true)

	idx.notes[relPath] = meta
	idx.titleIndex[strings.ToLower(relPath)] = relPath
	idx.titleIndex[strings.ToLower(title)] = relPath
	idx.titleIndex[strings.ToLower(meta.Title)] = relPath
	idx.titleIndex[strings.ToLower(filepath.Base(relPath))] = relPath
}

func (idx *VaultIndex) UpdateFile(vaultPath, relPath string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if excludedPath(relPath+".md", idx.skillsDirRel, true) {
		return
	}

	fullPath := filepath.Join(vaultPath, relPath+".md")
	title := strings.TrimSuffix(filepath.Base(relPath), ".md")
	idx.indexFile(vaultPath, fullPath, relPath, title)
}

func (idx *VaultIndex) RemoveFile(relPath string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for p := range idx.notes {
		if p == relPath || strings.HasPrefix(p, relPath+"/") {
			delete(idx.notes, p)
		}
	}
	idx.rebuildAliases()
}

func (idx *VaultIndex) RenameFile(oldPath, newPath string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	meta := idx.notes[oldPath]
	if meta == nil {
		return
	}

	delete(idx.notes, oldPath)
	delete(idx.titleIndex, strings.ToLower(oldPath))
	delete(idx.titleIndex, strings.ToLower(meta.Title))
	delete(idx.titleIndex, strings.ToLower(filepath.Base(oldPath)))

	idx.notes[newPath] = meta
	meta.Path = newPath
	meta.Title = strings.TrimSuffix(filepath.Base(newPath), ".md")

	idx.titleIndex[strings.ToLower(newPath)] = newPath
	idx.titleIndex[strings.ToLower(meta.Title)] = newPath
	idx.titleIndex[strings.ToLower(filepath.Base(newPath))] = newPath
}

func (idx *VaultIndex) GetGraph() (nodes []GraphNode, links []GraphLink) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	nodes = make([]GraphNode, 0, len(idx.notes))
	links = make([]GraphLink, 0)
	for _, meta := range idx.notes {
		nodes = append(nodes, GraphNode{ID: meta.Path, Title: meta.Title, Type: meta.Type})
	}

	seenLinks := make(map[string]bool)
	for _, meta := range idx.notes {
		for _, rawTarget := range meta.RawLinks {
			targetPath := resolveTitle(idx.titleIndex, rawTarget)
			if targetPath == "" || targetPath == meta.Path {
				continue
			}
			if _, ok := idx.notes[targetPath]; !ok {
				continue
			}
			key := meta.Path + "\x00" + targetPath
			if seenLinks[key] {
				continue
			}
			seenLinks[key] = true
			links = append(links, GraphLink{Source: meta.Path, Target: targetPath})
		}
	}

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(links, func(i, j int) bool {
		if links[i].Source != links[j].Source {
			return links[i].Source < links[j].Source
		}
		return links[i].Target < links[j].Target
	})
	return nodes, links
}

func (idx *VaultIndex) Search(query string) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	query = strings.ToLower(query)
	var results []SearchResult
	for _, meta := range idx.notes {
		if strings.Contains(strings.ToLower(meta.Content), query) {
			results = append(results, SearchResult{Path: meta.Path, Title: meta.Title, Snippet: searchSnippet(meta.Content, query)})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Path < results[j].Path })
	return results
}

func (idx *VaultIndex) GetBacklinks(targetPath string) []BacklinkResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	targetNameLower := strings.ToLower(filepath.Base(targetPath))

	var results []BacklinkResult
	for _, meta := range idx.notes {
		if meta.Path == targetPath {
			continue
		}
		for _, rawTarget := range meta.RawLinks {
			resolved := resolveTitle(idx.titleIndex, rawTarget)
			if resolved == targetPath {
				results = append(results, BacklinkResult{Path: meta.Path, Title: meta.Title})
				break
			}
			if resolved == "" && (strings.EqualFold(rawTarget, targetPath) || strings.EqualFold(rawTarget, targetNameLower)) {
				results = append(results, BacklinkResult{Path: meta.Path, Title: meta.Title})
				break
			}
		}
	}
	return results
}

func (idx *VaultIndex) FindPath(title string) string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	title = strings.TrimSuffix(title, ".md")
	titleLower := strings.ToLower(title)

	if p, ok := idx.titleIndex[titleLower]; ok {
		return p
	}
	if strings.Contains(title, "/") {
		return ""
	}

	base := strings.ToLower(filepath.Base(title))
	if p, ok := idx.titleIndex[base]; ok {
		return p
	}

	return ""
}

func extractWikiLinkTargets(content string) []string {
	matches := WikiLinkRegex.FindAllStringSubmatch(content, -1)
	var targets []string
	seen := make(map[string]bool)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		target := strings.TrimSpace(m[1])
		if !seen[target] {
			seen[target] = true
			targets = append(targets, target)
		}
	}
	return targets
}

func resolveTitle(titleIndex map[string]string, title string) string {
	title = strings.TrimSuffix(title, ".md")
	titleLower := strings.ToLower(title)

	if p, ok := titleIndex[titleLower]; ok {
		return p
	}
	if strings.Contains(title, "/") {
		return ""
	}

	base := strings.ToLower(filepath.Base(title))
	if p, ok := titleIndex[base]; ok {
		return p
	}

	return ""
}

type GraphNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type GraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type SearchResult struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Snippet string `json:"snippet,omitempty"`
}

func searchSnippet(content, query string) string {
	_, body := parseNote([]byte(content))
	text := strings.Join(strings.Fields(string(body)), " ")
	runes := []rune(text)
	start := 0
	if index := strings.Index(strings.ToLower(text), query); index >= 0 {
		start = max(0, len([]rune(strings.ToLower(text)[:index]))-70)
	}
	end := min(len(runes), start+240)
	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet += "…"
	}
	return snippet
}

type BacklinkResult struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}
