package main

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	tmpl, err := template.ParseFiles("templates/layout.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.ExecuteTemplate(w, "layout.html", nil)
}

func (fs *FileSystem) handleViewNote(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		http.Error(w, "Note path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(fs.VaultPath, path+".md")
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			data := map[string]interface{}{
				"Path": path,
			}
			if r.Header.Get("HX-Request") == "true" {
				tmpl, _ := template.ParseFiles("templates/missing_note.html")
				tmpl.Execute(w, data)
				return
			}
			tmpl, _ := template.ParseFiles("templates/layout.html", "templates/missing_note.html")
			tmpl.ExecuteTemplate(w, "layout.html", data)
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

	data := map[string]interface{}{
		"Path":        path,
		"Title":       filepath.Base(path),
		"Content":     string(content),
		"Frontmatter": frontmatter,
		"HTML":        template.HTML(html),
		"Breadcrumbs": buildBreadcrumbs(path),
	}

	if r.Header.Get("HX-Request") == "true" {
		tmpl, _ := template.ParseFiles("templates/note.html")
		tmpl.ExecuteTemplate(w, "content", data)
		return
	}

	tmpl, _ := template.ParseFiles("templates/layout.html", "templates/note.html")
	tmpl.ExecuteTemplate(w, "layout.html", data)
}

func (fs *FileSystem) handleEditNote(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		http.Error(w, "Note path required", http.StatusBadRequest)
		return
	}

	// Redirect /edit/ to /note/ for unified interface
	http.Redirect(w, r, "/note/"+path, http.StatusSeeOther)
}

func (fs *FileSystem) handleSaveNote(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		http.Error(w, "Note path required", http.StatusBadRequest)
		return
	}

	content := r.FormValue("content")
	fullPath := filepath.Join(fs.VaultPath, path+".md")

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Saved"))
}

func (fs *FileSystem) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	path := r.FormValue("path")
	if path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(fs.VaultPath, path+".md")
	if _, err := os.Stat(fullPath); err == nil {
		http.Error(w, "Note already exists", http.StatusConflict)
		return
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	content := fmt.Sprintf("---\ntitle: %s\n---\n\n", filepath.Base(path))
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/note/"+path)
}

func (fs *FileSystem) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	path := r.FormValue("path")
	if path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(fs.VaultPath, path)
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshTree")
}

func (fs *FileSystem) handleRename(w http.ResponseWriter, r *http.Request) {
	oldPath := r.FormValue("old")
	newPath := r.FormValue("new")
	if oldPath == "" || newPath == "" {
		http.Error(w, "Old and new paths required", http.StatusBadRequest)
		return
	}

	oldFull := filepath.Join(fs.VaultPath, oldPath)
	newFull := filepath.Join(fs.VaultPath, newPath)

	if err := os.Rename(oldFull, newFull); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshTree")
}

func (fs *FileSystem) handleDelete(w http.ResponseWriter, r *http.Request) {
	path := r.FormValue("path")
	if path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(fs.VaultPath, path)
	if err := os.RemoveAll(fullPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshTree")
	w.Header().Set("HX-Redirect", "/")
}

func (fs *FileSystem) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))
	if query == "" {
		if r.Header.Get("HX-Request") == "true" {
			tmpl, _ := template.ParseFiles("templates/search.html")
			tmpl.ExecuteTemplate(w, "content", nil)
			return
		}
		tmpl, _ := template.ParseFiles("templates/layout.html", "templates/search.html")
		tmpl.ExecuteTemplate(w, "layout.html", nil)
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
				"Path":  rel,
				"Title": strings.TrimSuffix(info.Name(), ".md"),
			})
		}
		return nil
	})

	data := map[string]interface{}{
		"Query":   query,
		"Results": results,
	}

	if r.Header.Get("HX-Request") == "true" {
		tmpl, _ := template.ParseFiles("templates/search.html")
		tmpl.ExecuteTemplate(w, "content", data)
		return
	}

	tmpl, _ := template.ParseFiles("templates/layout.html", "templates/search.html")
	tmpl.ExecuteTemplate(w, "layout.html", data)
}

func (fs *FileSystem) handleBacklinks(w http.ResponseWriter, r *http.Request) {
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
				"Path":  rel,
				"Title": strings.TrimSuffix(info.Name(), ".md"),
			})
		}
		return nil
	})

	tmpl, _ := template.ParseFiles("templates/backlinks.html")
	tmpl.Execute(w, map[string]interface{}{
		"Path":      path,
		"Backlinks": backlinks,
	})
}

func (fs *FileSystem) handlePreview(w http.ResponseWriter, r *http.Request) {
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
