package internal

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileSystem struct {
	VaultPath string
	live      *LiveSync
}

func NewFileSystem(vaultPath string) *FileSystem {
	return &FileSystem{
		VaultPath: vaultPath,
		live:      NewLiveSync(),
	}
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
			IsOpen:   depth < 2, // Auto-expand first two levels
			Depth:    depth,
		}

		if entry.IsDir() {
			children, err := fs.buildTree(fullPath, activePath, depth+1)
			if err == nil {
				node.Children = children
				// Keep folder open if any child is active
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

	// Sort: folders first, then alphabetically
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})

	return nodes, nil
}
