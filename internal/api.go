package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (fs *FileSystem) HandleAPIFileTree(w http.ResponseWriter, r *http.Request) {
	activePath := r.URL.Query().Get("active")
	tree, err := fs.buildTree(fs.VaultPath, activePath, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, tree)
}

func (fs *FileSystem) HandleAPIViewNote(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		http.Error(w, "Note path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(fs.VaultPath, path+".md")
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Note not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	frontmatter, body := parseNote(content)
	html, err := renderMarkdown(body, fs.VaultPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"path":        path,
		"title":       filepath.Base(path),
		"content":     string(content),
		"frontmatter": frontmatter,
		"html":        html,
		"breadcrumbs": buildBreadcrumbs(path),
	})
}

func (fs *FileSystem) HandleAPISaveNote(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		http.Error(w, "Note path required", http.StatusBadRequest)
		return
	}

	content, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	fullPath := filepath.Join(fs.VaultPath, path+".md")
	_, statErr := os.Stat(fullPath)
	isNewNote := os.IsNotExist(statErr)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fs.NotifyVaultChange([]string{path}, isNewNote, true, true, true)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Saved"))
}

func (fs *FileSystem) HandleAPICreateNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(fs.VaultPath, req.Path+".md")
	if _, err := os.Stat(fullPath); err == nil {
		http.Error(w, "Note already exists", http.StatusConflict)
		return
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	content := fmt.Sprintf("---\ntitle: %s\n---\n\n", filepath.Base(req.Path))
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fs.NotifyVaultChange([]string{req.Path}, true, true, true, true)

	w.WriteHeader(http.StatusCreated)
}

func (fs *FileSystem) HandleAPICreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(fs.VaultPath, req.Path)
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fs.NotifyVaultChange([]string{req.Path}, true, false, false, false)

	w.WriteHeader(http.StatusCreated)
}

func (fs *FileSystem) HandleAPIRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Old == "" || req.New == "" {
		http.Error(w, "Old and new paths required", http.StatusBadRequest)
		return
	}

	oldFull := filepath.Join(fs.VaultPath, req.Old)
	newFull := filepath.Join(fs.VaultPath, req.New)

	if err := os.Rename(oldFull, newFull); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fs.NotifyVaultChange([]string{req.Old, req.New}, true, true, true, true)

	w.WriteHeader(http.StatusOK)
}

func (fs *FileSystem) HandleAPIDelete(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(fs.VaultPath, path)
	if err := os.RemoveAll(fullPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fs.NotifyVaultChange([]string{path}, true, true, true, true)

	w.WriteHeader(http.StatusOK)
}

func (fs *FileSystem) HandleAPISearch(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, []map[string]string{})
		return
	}

	var results []map[string]string
	filepath.Walk(fs.VaultPath, func(path string, info os.FileInfo, err error) error {
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
			results = append(results, map[string]string{
				"path":  rel,
				"title": strings.TrimSuffix(info.Name(), ".md"),
			})
		}
		return nil
	})

	writeJSON(w, results)
}

func (fs *FileSystem) HandleAPIBacklinks(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	targetName := filepath.Base(path)
	var backlinks []map[string]string

	filepath.Walk(fs.VaultPath, func(filePath string, info os.FileInfo, err error) error {
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

		pattern := "[[" + targetName + "]]"
		if strings.Contains(string(content), pattern) {
			backlinks = append(backlinks, map[string]string{
				"path":  rel,
				"title": strings.TrimSuffix(info.Name(), ".md"),
			})
		}
		return nil
	})

	writeJSON(w, backlinks)
}

func (fs *FileSystem) HandleAPIGraph(w http.ResponseWriter, r *http.Request) {
	type GraphNode struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	type GraphLink struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}

	nodes := make(map[string]GraphNode)
	links := make([]GraphLink, 0)

	wikiLinkRegex := regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)

	// First pass: collect all nodes
	filepath.Walk(fs.VaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		rel, _ := filepath.Rel(fs.VaultPath, path)
		rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		title := strings.TrimSuffix(info.Name(), ".md")

		nodes[rel] = GraphNode{ID: rel, Title: title}
		return nil
	})

	// Second pass: collect links
	filepath.Walk(fs.VaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		rel, _ := filepath.Rel(fs.VaultPath, path)
		rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		matches := wikiLinkRegex.FindAllStringSubmatch(string(content), -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			targetName := strings.TrimSpace(m[1])
			targetPath := findNotePath(targetName, fs.VaultPath)
			if targetPath != "" && targetPath != rel {
				// Only add link if target node exists in our graph
				if _, ok := nodes[targetPath]; ok {
					links = append(links, GraphLink{Source: rel, Target: targetPath})
				}
			}
		}

		return nil
	})

	nodeList := make([]GraphNode, 0, len(nodes))
	for _, n := range nodes {
		nodeList = append(nodeList, n)
	}

	writeJSON(w, map[string]interface{}{
		"nodes": nodeList,
		"links": links,
	})
}

func (fs *FileSystem) HandleAPIPreview(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("path") // path is part of route but not needed for preview
	content, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	html, err := renderMarkdown(content, fs.VaultPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}
