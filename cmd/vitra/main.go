package main

import (
	iofs "io/fs"
	"log"
	"net/http"
	"os"

	"strings"
	frontend "vitra/frontend"
	"vitra/internal"
)

func main() {
	vaultPath := os.Getenv("VAULT_PATH")
	if vaultPath == "" {
		vaultPath = "./vault"
	}

	fs := internal.NewFileSystem(vaultPath)
	if err := fs.BuildIndex(); err != nil {
		log.Fatalf("failed to build vault index: %v", err)
	}
	if err := fs.StartWatcher(); err != nil {
		log.Fatalf("failed to start vault watcher: %v", err)
	}
	defer fs.CloseWatcher()

	http.HandleFunc("GET /api/files", fs.HandleAPIFileTree)
	http.HandleFunc("GET /api/events", fs.HandleAPIEvents)
	http.HandleFunc("GET /api/note/{path...}", fs.HandleAPIViewNote)
	http.HandleFunc("POST /api/note/{path...}", fs.HandleAPISaveNote)
	http.HandleFunc("POST /api/notes", fs.HandleAPICreateNote)
	http.HandleFunc("POST /api/folders", fs.HandleAPICreateFolder)
	http.HandleFunc("PUT /api/rename", fs.HandleAPIRename)
	http.HandleFunc("DELETE /api/delete", fs.HandleAPIDelete)
	http.HandleFunc("GET /api/search", fs.HandleAPISearch)
	http.HandleFunc("GET /api/backlinks/{path...}", fs.HandleAPIBacklinks)
	http.HandleFunc("GET /api/graph", fs.HandleAPIGraph)
	http.HandleFunc("POST /api/preview/{path...}", fs.HandleAPIPreview)

	distFS, err := iofs.Sub(frontend.Dist, "dist")
	if err != nil {
		log.Fatalf("failed to load embedded frontend assets: %v", err)
	}
	fileServer := http.FileServer(http.FS(distFS))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		assetPath := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/"), "./")
		if assetPath == "" {
			assetPath = "index.html"
		}

		info, err := iofs.Stat(distFS, assetPath)
		if err != nil || info.IsDir() || r.URL.Path == "/" {
			http.ServeFileFS(w, r, distFS, "index.html")
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
