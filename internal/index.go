package internal

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type NoteMeta struct {
	Path     string
	Title    string
	RawLinks []string
	Content  string
}

type VaultIndex struct {
	mu         sync.RWMutex
	notes      map[string]*NoteMeta
	titleIndex map[string]string
}

func NewVaultIndex() *VaultIndex {
	return &VaultIndex{
		notes:      make(map[string]*NoteMeta),
		titleIndex: make(map[string]string),
	}
}

func (idx *VaultIndex) Build(vaultPath string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.notes = make(map[string]*NoteMeta)
	idx.titleIndex = make(map[string]string)

	return filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(vaultPath, path)
		rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		idx.indexFile(vaultPath, path, rel, strings.TrimSuffix(info.Name(), ".md"))
		return nil
	})
}

func (idx *VaultIndex) indexFile(vaultPath, fullPath, relPath, title string) {
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return
	}

	meta := &NoteMeta{
		Path:     relPath,
		Title:    title,
		Content:  string(content),
		RawLinks: extractWikiLinkTargets(string(content)),
	}

	idx.notes[relPath] = meta
	idx.titleIndex[strings.ToLower(relPath)] = relPath
	idx.titleIndex[strings.ToLower(title)] = relPath
	idx.titleIndex[strings.ToLower(filepath.Base(relPath))] = relPath
}

func (idx *VaultIndex) UpdateFile(vaultPath, relPath string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	fullPath := filepath.Join(vaultPath, relPath+".md")
	title := strings.TrimSuffix(filepath.Base(relPath), ".md")
	idx.indexFile(vaultPath, fullPath, relPath, title)
}

func (idx *VaultIndex) RemoveFile(relPath string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	meta := idx.notes[relPath]
	if meta == nil {
		return
	}
	delete(idx.notes, relPath)
	delete(idx.titleIndex, strings.ToLower(relPath))
	delete(idx.titleIndex, strings.ToLower(meta.Title))
	delete(idx.titleIndex, strings.ToLower(filepath.Base(relPath)))
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
	for _, meta := range idx.notes {
		nodes = append(nodes, GraphNode{ID: meta.Path, Title: meta.Title})
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

	return nodes, links
}

func (idx *VaultIndex) Search(query string) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	query = strings.ToLower(query)
	var results []SearchResult
	for _, meta := range idx.notes {
		if strings.Contains(strings.ToLower(meta.Content), query) {
			results = append(results, SearchResult{Path: meta.Path, Title: meta.Title})
		}
	}
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

	base := strings.ToLower(filepath.Base(title))
	if p, ok := titleIndex[base]; ok {
		return p
	}

	return ""
}

type GraphNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type GraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type SearchResult struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

type BacklinkResult struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}
