package main

import (
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type FileSystem struct {
	VaultPath string
}

func NewFileSystem(vaultPath string) *FileSystem {
	return &FileSystem{VaultPath: vaultPath}
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

	return nodes, nil
}

func (fs *FileSystem) handleFileTree(w http.ResponseWriter, r *http.Request) {
	activePath := r.URL.Query().Get("active")
	tree, err := fs.buildTree(fs.VaultPath, activePath, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl, _ := template.ParseFiles("templates/file_tree.html")
	tmpl.Execute(w, tree)
}
