package internal

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	iofs "io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/iamangus/vitra/internal/vector"
)

const maxBodySize = 10 << 20

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("JSON response: %v", err)
	}
}

func apiError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, ErrInvalidPath):
		status = http.StatusBadRequest
	case errors.Is(err, ErrConflict), errors.Is(err, os.ErrExist):
		status = http.StatusConflict
	case errors.Is(err, os.ErrNotExist):
		status = http.StatusNotFound
	case errors.Is(err, os.ErrPermission):
		status = http.StatusForbidden
	}
	if status == http.StatusInternalServerError {
		log.Printf("API: %v", err)
		http.Error(w, "Internal server error", status)
		return
	}
	http.Error(w, http.StatusText(status), status)
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
}

func decodeBody(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	data, err := readBody(w, r)
	if err != nil {
		apiError(w, err)
		return false
	}
	if err := json.Unmarshal(data, target); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

func boundedInt(raw string, fallback, maximum int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maximum {
		return 0, ErrInvalidPath
	}
	return n, nil
}

func (fs *FileSystem) HandleAPIFileTree(w http.ResponseWriter, r *http.Request) {
	tree, err := fs.buildTree(fs.VaultPath, r.URL.Query().Get("active"), 0)
	if err != nil {
		apiError(w, err)
		return
	}
	if tree == nil {
		tree = []FileNode{}
	}
	writeJSON(w, tree)
}

func (fs *FileSystem) HandleAPIViewNote(w http.ResponseWriter, r *http.Request) {
	note, err := fs.ReadNote(r.PathValue("path"))
	if err != nil {
		apiError(w, err)
		return
	}
	w.Header().Set("ETag", note.Revision)
	writeJSON(w, note)
}

func (fs *FileSystem) requireRevision(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("If-Match") != "" {
		return true
	}
	if _, err := fs.ReadVaultFile(noteName(r.PathValue("path")) + ".md"); err == nil {
		http.Error(w, "If-Match revision required", http.StatusPreconditionRequired)
		return false
	} else if !os.IsNotExist(err) {
		apiError(w, err)
		return false
	}
	return true
}

func (fs *FileSystem) HandleAPISaveNote(w http.ResponseWriter, r *http.Request) {
	data, err := readBody(w, r)
	if err != nil {
		apiError(w, err)
		return
	}
	if !fs.requireRevision(w, r) {
		return
	}
	expected := r.Header.Get("If-Match")
	rev, err := fs.SaveNote(r.PathValue("path"), string(data), expected, expected == "")
	if err != nil {
		apiError(w, err)
		return
	}
	w.Header().Set("ETag", rev)
	writeJSON(w, map[string]string{"revision": rev})
}

func (fs *FileSystem) HandleAPICreateNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Path == "" {
		apiError(w, ErrInvalidPath)
		return
	}
	if err := fs.CreateNote(req.Path, ""); err != nil {
		apiError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (fs *FileSystem) HandleAPICreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := fs.CreateFolder(req.Path); err != nil {
		apiError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (fs *FileSystem) HandleAPIRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Old == "" || req.New == "" {
		apiError(w, ErrInvalidPath)
		return
	}
	if err := fs.RenameNote(req.Old, req.New); err != nil {
		apiError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (fs *FileSystem) HandleAPIDelete(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("path") == "" {
		apiError(w, ErrInvalidPath)
		return
	}
	if err := fs.DeleteNote(r.URL.Query().Get("path")); err != nil {
		apiError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (fs *FileSystem) HandleAPIDownload(w http.ResponseWriter, r *http.Request) {
	root, rel, info, err := fs.resolveEntry(r.URL.Query().Get("path"))
	if err != nil {
		apiError(w, err)
		return
	}
	defer root.Close()
	if !info.IsDir() {
		file, err := root.Open(rel)
		if err != nil {
			apiError(w, err)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}))
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name() + ".zip"}))
	archive := zip.NewWriter(w)
	err = iofs.WalkDir(root.FS(), filepath.ToSlash(rel), func(p string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if excludedPath(p, fs.skillsDirRel, false) {
			if d.IsDir() {
				return iofs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		file, err := root.Open(p)
		if err != nil {
			return err
		}
		defer file.Close()
		name, err := filepath.Rel(rel, p)
		if err != nil {
			return err
		}
		out, err := archive.Create(filepath.ToSlash(name))
		if err != nil {
			return err
		}
		_, err = io.Copy(out, file)
		return err
	})
	if err != nil {
		log.Printf("download %s: %v", rel, err)
		return
	}
	if err := archive.Close(); err != nil {
		log.Printf("download close: %v", err)
	}
}

func (fs *FileSystem) HandleAPISearch(w http.ResponseWriter, r *http.Request) {
	results, err := fs.SearchNotes(strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		apiError(w, err)
		return
	}
	if results == nil {
		results = []SearchResult{}
	}
	writeJSON(w, results)
}

func (fs *FileSystem) HandleAPISemanticSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 8192 {
		apiError(w, ErrInvalidPath)
		return
	}
	if query == "" {
		writeJSON(w, []vector.SearchResult{})
		return
	}
	limit, err := boundedInt(r.URL.Query().Get("limit"), 5, 100)
	if err != nil {
		apiError(w, err)
		return
	}
	results, err := fs.SemanticSearch(r.Context(), query, limit, vector.Filter{Type: r.URL.Query().Get("type"), Tags: r.URL.Query()["tag"]})
	if err != nil {
		http.Error(w, "Semantic search unavailable", http.StatusServiceUnavailable)
		return
	}
	if results == nil {
		results = []vector.SearchResult{}
	}
	writeJSON(w, results)
}

func (fs *FileSystem) HandleAPIBacklinks(w http.ResponseWriter, r *http.Request) {
	results, err := fs.NoteBacklinks(r.PathValue("path"))
	if err != nil {
		apiError(w, err)
		return
	}
	if results == nil {
		results = []BacklinkResult{}
	}
	writeJSON(w, results)
}

func (fs *FileSystem) HandleAPIGraph(w http.ResponseWriter, r *http.Request) {
	nodes, links := []GraphNode{}, []GraphLink{}
	if fs.index != nil {
		nodes, links = fs.index.GetGraph()
	}
	writeJSON(w, map[string]interface{}{"nodes": nodes, "links": links})
}

func (fs *FileSystem) HandleAPIPreview(w http.ResponseWriter, r *http.Request) {
	content, err := readBody(w, r)
	if err != nil {
		apiError(w, err)
		return
	}
	_, body := parseNote(content)
	html, err := renderMarkdown(body, fs.VaultPath, fs.index, filepath.Dir(r.PathValue("path")))
	if err != nil {
		apiError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (fs *FileSystem) HandleAPIConcepts(w http.ResponseWriter, r *http.Request) {
	limit, err := boundedInt(r.URL.Query().Get("limit"), 1000, 10000)
	if err != nil {
		apiError(w, err)
		return
	}
	since := r.URL.Query().Get("since")
	if since != "" {
		if _, err := time.Parse(time.RFC3339, since); err != nil {
			apiError(w, ErrInvalidPath)
			return
		}
	}
	views, version, err := fs.ListConcepts(ConceptFilter{Type: r.URL.Query().Get("type"), Resource: r.URL.Query().Get("resource"), Tag: r.URL.Query()["tag"], Since: since, Limit: limit})
	if err != nil {
		apiError(w, err)
		return
	}
	if views == nil {
		views = []ConceptView{}
	}
	writeJSON(w, map[string]interface{}{"okf_version": version, "count": len(views), "concepts": views})
}

func (fs *FileSystem) HandleAPIActivity(w http.ResponseWriter, r *http.Request) {
	limit, err := boundedInt(r.URL.Query().Get("limit"), 100, 1000)
	if err != nil {
		apiError(w, err)
		return
	}
	entries, err := fs.ListActivity(r.URL.Query().Get("path"), limit)
	if err != nil {
		apiError(w, err)
		return
	}
	if entries == nil {
		entries = []ActivityEntry{}
	}
	writeJSON(w, entries)
}

func (fs *FileSystem) HandleAPIConceptClosure(w http.ResponseWriter, r *http.Request) {
	seed := r.URL.Query().Get("path")
	if seed == "" {
		apiError(w, ErrInvalidPath)
		return
	}
	depth, err := boundedInt(r.URL.Query().Get("depth"), 2, 20)
	if err != nil {
		apiError(w, err)
		return
	}
	views, err := fs.TransitiveClosure(seed, depth)
	if err != nil {
		apiError(w, err)
		return
	}
	if views == nil {
		views = []ConceptView{}
	}
	writeJSON(w, views)
}

func (fs *FileSystem) HandleAPIPatchNote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Frontmatter map[string]interface{} `json:"frontmatter"`
		Body        *string                `json:"body"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if len(body.Frontmatter) == 0 && body.Body == nil {
		apiError(w, ErrInvalidPath)
		return
	}
	if !fs.requireRevision(w, r) {
		return
	}
	rev, err := fs.PatchNote(r.PathValue("path"), body.Frontmatter, body.Body, r.Header.Get("If-Match"))
	if err != nil {
		apiError(w, err)
		return
	}
	w.Header().Set("ETag", rev)
	writeJSON(w, map[string]string{"revision": rev})
}
