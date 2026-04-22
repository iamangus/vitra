package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	vaultPath := os.Getenv("VAULT_PATH")
	if vaultPath == "" {
		vaultPath = "./vault"
	}

	fs := NewFileSystem(vaultPath)

	// API routes
	http.HandleFunc("GET /api/files", fs.handleAPIFileTree)
	http.HandleFunc("GET /api/note/{path...}", fs.handleAPIViewNote)
	http.HandleFunc("POST /api/note/{path...}", fs.handleAPISaveNote)
	http.HandleFunc("POST /api/notes", fs.handleAPICreateNote)
	http.HandleFunc("POST /api/folders", fs.handleAPICreateFolder)
	http.HandleFunc("PUT /api/rename", fs.handleAPIRename)
	http.HandleFunc("DELETE /api/delete", fs.handleAPIDelete)
	http.HandleFunc("GET /api/search", fs.handleAPISearch)
	http.HandleFunc("GET /api/backlinks/{path...}", fs.handleAPIBacklinks)
	http.HandleFunc("POST /api/preview/{path...}", fs.handleAPIPreview)

	// Serve static files from frontend/dist
	staticDir := "./frontend/dist"
	fsys := http.Dir(staticDir)
	fileServer := http.FileServer(fsys)

	// SPA fallback: serve index.html for non-API, non-file routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Check if the file exists in dist
		path := filepath.Join(staticDir, r.URL.Path)
		_, err := os.Stat(path)
		if os.IsNotExist(err) || r.URL.Path == "/" {
			// Serve index.html for SPA routes
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Vitra starting on :%s with vault at %s", port, vaultPath)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
