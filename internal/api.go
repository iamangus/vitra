package internal

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/iamangus/vitra/internal/vector"
)

const maxBodySize = 10 << 20 // 10 MB

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("ERROR encoding JSON response: %v", err)
	}
}

func safeVaultPath(vaultPath, input string) (string, error) {
	clean := filepath.Clean(filepath.Join(vaultPath, input))
	cleanVault := filepath.Clean(vaultPath)
	if !strings.HasPrefix(clean, cleanVault+string(filepath.Separator)) && clean != cleanVault {
		return "", errors.New("path escapes vault")
	}
	return clean, nil
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

	fullPath, err := safeVaultPath(fs.VaultPath, path+".md")
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

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
	html, err := renderMarkdown(body, fs.VaultPath, fs.index)
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

	content, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		log.Printf("ERROR saving note %s: read body: %v", path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	fullPath, err := safeVaultPath(fs.VaultPath, path+".md")
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	_, statErr := os.Stat(fullPath)
	isNewNote := os.IsNotExist(statErr)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("ERROR saving note %s: mkdir %s: %v", path, dir, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		log.Printf("ERROR saving note %s: write file: %v", path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fs.index != nil {
		fs.index.UpdateFile(fs.VaultPath, path)
	}
	fs.autoIndex(path, string(content))
	fs.NotifyVaultChange([]string{path}, isNewNote, true, true, true)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Saved"))
}

func (fs *FileSystem) HandleAPICreateNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodySize)).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	fullPath, err := safeVaultPath(fs.VaultPath, req.Path+".md")
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	if _, err := os.Stat(fullPath); err == nil {
		http.Error(w, "Note already exists", http.StatusConflict)
		return
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	content := fmt.Sprintf("---\ntype: Note\ntitle: %s\n---\n\n", filepath.Base(req.Path))
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fs.index != nil {
		fs.index.UpdateFile(fs.VaultPath, req.Path)
	}
	fs.autoIndex(req.Path, content)
	fs.NotifyVaultChange([]string{req.Path}, true, true, true, true)

	w.WriteHeader(http.StatusCreated)
}

func (fs *FileSystem) HandleAPICreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodySize)).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	fullPath, err := safeVaultPath(fs.VaultPath, req.Path)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

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
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodySize)).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Old == "" || req.New == "" {
		http.Error(w, "Old and new paths required", http.StatusBadRequest)
		return
	}

	oldFull, err := safeVaultPath(fs.VaultPath, req.Old)
	if err != nil {
		http.Error(w, "Invalid old path", http.StatusBadRequest)
		return
	}
	newFull, err := safeVaultPath(fs.VaultPath, req.New)
	if err != nil {
		http.Error(w, "Invalid new path", http.StatusBadRequest)
		return
	}

	if err := os.Rename(oldFull, newFull); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	relOld := strings.TrimSuffix(filepath.ToSlash(req.Old), ".md")
	relNew := strings.TrimSuffix(filepath.ToSlash(req.New), ".md")
	if fs.index != nil {
		fs.index.RenameFile(relOld, relNew)
	}
	if fs.VectorStore != nil {
		fs.autoDelete(relOld)
		info, err := os.Stat(newFull)
		if err == nil && !info.IsDir() && strings.HasSuffix(req.New, ".md") {
			if content, err := os.ReadFile(newFull); err == nil {
				fs.autoIndex(relNew, string(bytes.TrimSpace(content)))
			}
		} else if err == nil && info.IsDir() {
			filepath.WalkDir(newFull, func(p string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
					return nil
				}
				rel, _ := filepath.Rel(fs.VaultPath, p)
				if c, err := os.ReadFile(p); err == nil {
					fs.autoIndex(strings.TrimSuffix(filepath.ToSlash(rel), ".md"), string(bytes.TrimSpace(c)))
				}
				return nil
			})
		}
	}

	fs.NotifyVaultChange([]string{relOld, relNew}, true, true, true, true)

	w.WriteHeader(http.StatusOK)
}

func (fs *FileSystem) HandleAPIDelete(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	fullPath, err := safeVaultPath(fs.VaultPath, path)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	relPath := filepath.ToSlash(path)
	relTrim := strings.TrimSuffix(relPath, ".md")
	// If deleting a folder, remove all contained notes from the vector index first.
	if fs.VectorStore != nil {
		if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
			filepath.WalkDir(fullPath, func(p string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
					return nil
				}
				rel, _ := filepath.Rel(fs.VaultPath, p)
				fs.autoDelete(strings.TrimSuffix(filepath.ToSlash(rel), ".md"))
				return nil
			})
		} else {
			fs.autoDelete(relTrim)
		}
	}
	if err := os.RemoveAll(fullPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fs.index != nil {
		fs.index.RemoveFile(relTrim)
	}

	fs.NotifyVaultChange([]string{path}, true, true, true, true)

	w.WriteHeader(http.StatusOK)
}

func (fs *FileSystem) HandleAPIDownload(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	fullPath, err := safeVaultPath(fs.VaultPath, path+".md")
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if info.IsDir() {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.zip", info.Name()))

		zipWriter := zip.NewWriter(w)
		defer zipWriter.Close()

		filepath.Walk(fullPath, func(filePath string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			relPath, err := filepath.Rel(fullPath, filePath)
			if err != nil {
				return nil
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return nil
			}
			header.Name = relPath

			writer, err := zipWriter.CreateHeader(header)
			if err != nil {
				return nil
			}

			file, err := os.Open(filePath)
			if err != nil {
				return nil
			}
			defer file.Close()

			_, err = io.Copy(writer, file)
			return nil
		})
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", info.Name()))
		http.ServeFile(w, r, fullPath)
	}
}

func (fs *FileSystem) HandleAPISearch(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, []map[string]string{})
		return
	}

	if fs.index != nil {
		results := fs.index.Search(query)
		output := make([]map[string]string, len(results))
		for i, r := range results {
			output[i] = map[string]string{
				"path":  r.Path,
				"title": r.Title,
			}
		}
		writeJSON(w, output)
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

func (fs *FileSystem) HandleAPISemanticSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, []any{})
		return
	}
	limit := 5
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	filter := vector.Filter{
		Type: strings.TrimSpace(r.URL.Query().Get("type")),
	}
	if tags := r.URL.Query()["tag"]; len(tags) > 0 {
		filter.Tags = tags
	}
	results, err := fs.SemanticSearch(r.Context(), query, limit, filter)
	if err != nil {
		// Distinguish "not configured" from real errors so the UI can fall back.
		if strings.Contains(err.Error(), "not configured") {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = []vector.SearchResult{}
	}
	writeJSON(w, results)
}

func (fs *FileSystem) HandleAPIBacklinks(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	if fs.index != nil {
		results := fs.index.GetBacklinks(path)
		output := make([]map[string]string, len(results))
		for i, r := range results {
			output[i] = map[string]string{
				"path":  r.Path,
				"title": r.Title,
			}
		}
		writeJSON(w, output)
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
	if fs.index != nil {
		nodes, links := fs.index.GetGraph()
		writeJSON(w, map[string]interface{}{
			"nodes": nodes,
			"links": links,
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"nodes": []GraphNode{},
		"links": []GraphLink{},
	})
}

func (fs *FileSystem) HandleAPIPreview(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("path")
	content, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	html, err := renderMarkdown(content, fs.VaultPath, fs.index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// HandleAPIConcepts returns the OKF concept catalog for the vault, optionally
// filtered by type, tag(s), resource, or updated-since. The response envelope
// includes the vault-root index.md's declared okf_version when present.
func (fs *FileSystem) HandleAPIConcepts(w http.ResponseWriter, r *http.Request) {
	filter := ConceptFilter{
		Type:     strings.TrimSpace(r.URL.Query().Get("type")),
		Resource: strings.TrimSpace(r.URL.Query().Get("resource")),
		Since:    strings.TrimSpace(r.URL.Query().Get("since")),
	}
	if tags := r.URL.Query()["tag"]; len(tags) > 0 {
		filter.Tag = tags
	}
	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	filter.Limit = limit

	views, okfVersion, err := fs.ListConcepts(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if views == nil {
		views = []ConceptView{}
	}
	writeJSON(w, map[string]interface{}{
		"okf_version": okfVersion,
		"count":       len(views),
		"concepts":    views,
	})
}

// HandleAPIActivity aggregates log.md entries across the vault into a single
// date-sorted feed (newest first). Optional `?path=` scopes the walk to a
// subdirectory; `?limit=` caps the result count.
func (fs *FileSystem) HandleAPIActivity(w http.ResponseWriter, r *http.Request) {
	scope := strings.TrimSpace(r.URL.Query().Get("path"))
	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	entries, err := fs.ListActivity(scope, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []ActivityEntry{}
	}
	writeJSON(w, entries)
}

// HandleAPIConceptClosure walks the link graph from a seed concept up to
// `depth` hops, returning the transitive closure of reachable concepts.
func (fs *FileSystem) HandleAPIConceptClosure(w http.ResponseWriter, r *http.Request) {
	seed := strings.TrimSpace(r.URL.Query().Get("path"))
	if seed == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	depth := 2
	if d := r.URL.Query().Get("depth"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 {
			depth = n
		}
	}
	closure, err := fs.TransitiveClosure(seed, depth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if closure == nil {
		closure = []ConceptView{}
	}
	writeJSON(w, closure)
}

// HandleAPIPatchNote merges frontmatter updates into an existing note,
// preserving the body and auto-touching `timestamp` (unless supplied). Body
// may also be supplied, in which case it replaces the current body.
func (fs *FileSystem) HandleAPIPatchNote(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	var body struct {
		Frontmatter map[string]interface{} `json:"frontmatter"`
		Body        *string                `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodySize)).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.Frontmatter) == 0 && body.Body == nil {
		http.Error(w, "no updates provided", http.StatusBadRequest)
		return
	}
	if body.Body != nil {
		fullPath, err := safeVaultPath(fs.VaultPath, path+".md")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, err := os.Stat(fullPath); err != nil {
			http.Error(w, "note not found", http.StatusNotFound)
			return
		}
		note, err := fs.ReadNote(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newContent := EmitFrontmatter(note.Frontmatter) + *body.Body
		if err := fs.WriteNote(path, newContent); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if len(body.Frontmatter) > 0 {
		if err := fs.UpdateNoteMetadata(path, body.Frontmatter); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]string{"status": "ok", "path": path})
}
