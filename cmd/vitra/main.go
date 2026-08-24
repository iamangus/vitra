package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	iofs "io/fs"

	"github.com/iamangus/vitra/frontend"
	"github.com/iamangus/vitra/internal"
	"github.com/iamangus/vitra/internal/mcp"
	"github.com/iamangus/vitra/internal/vector"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Flush() {
	if flusher, ok := lrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lrw, r)
		if lrw.statusCode >= 500 {
			log.Printf("%s %s %d", r.Method, r.URL.Path, lrw.statusCode)
		}
	})
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func main() {
	vaultPath := env("VAULT_PATH", "./vault")
	port := env("PORT", "8080")
	skillsDirName := env("SKILLS_DIR_NAME", "skills")
	chromemPath := env("CHROMEM_PATH", filepath.Join(vaultPath, ".chromem"))
	chunkSize := intEnv("CHUNK_SIZE", 0)
	chunkOverlap := intEnv("CHUNK_OVERLAP", 0)

	fs := internal.NewFileSystem(vaultPath)
	fs.SetSkillsDir(skillsDirName)
	fs.SetChunkConfig(chunkSize, chunkOverlap)
	if err := fs.BuildIndex(); err != nil {
		log.Fatalf("failed to build vault index: %v", err)
	}
	if err := fs.StartWatcher(); err != nil {
		log.Fatalf("failed to start vault watcher: %v", err)
	}
	defer fs.CloseWatcher()

	// Vector store: embedded chromem-go. Construction succeeds even without
	// an embedding API key; embedding calls fail at use-time so the web UI and
	// tools MCP can start regardless and surface "not configured" errors.
	store, err := vector.NewChromemStore(chromemPath, false, nil)
	if err != nil {
		log.Fatalf("failed to open vector store at %s: %v", chromemPath, err)
	}
	defer store.Close()
	fs.SetVectorStore(store)
	log.Printf("vector store initialized at %s", chromemPath)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/files", fs.HandleAPIFileTree)
	mux.HandleFunc("GET /api/events", fs.HandleAPIEvents)
	mux.HandleFunc("GET /api/note/{path...}", fs.HandleAPIViewNote)
	mux.HandleFunc("POST /api/note/{path...}", fs.HandleAPISaveNote)
	mux.HandleFunc("POST /api/notes", fs.HandleAPICreateNote)
	mux.HandleFunc("POST /api/folders", fs.HandleAPICreateFolder)
	mux.HandleFunc("PUT /api/rename", fs.HandleAPIRename)
	mux.HandleFunc("DELETE /api/delete", fs.HandleAPIDelete)
	mux.HandleFunc("GET /api/download", fs.HandleAPIDownload)
	mux.HandleFunc("GET /api/search", fs.HandleAPISearch)
	mux.HandleFunc("GET /api/search/semantic", fs.HandleAPISemanticSearch)
	mux.HandleFunc("GET /api/backlinks/{path...}", fs.HandleAPIBacklinks)
	mux.HandleFunc("GET /api/graph", fs.HandleAPIGraph)
	mux.HandleFunc("POST /api/preview/{path...}", fs.HandleAPIPreview)
	mux.HandleFunc("GET /api/concepts", fs.HandleAPIConcepts)
	mux.HandleFunc("GET /api/concepts/closure", fs.HandleAPIConceptClosure)
	mux.HandleFunc("GET /api/activity", fs.HandleAPIActivity)
	mux.HandleFunc("PATCH /api/note/{path...}", fs.HandleAPIPatchNote)
	mux.HandleFunc("GET /api/skills", fs.HandleAPISkills)

	mcpHandler := mcp.NewToolsServer(fs)
	mux.Handle("/mcp", mcpHandler)

	distFS, err := iofs.Sub(frontend.Dist, "dist")
	if err != nil {
		log.Fatalf("failed to load embedded frontend assets: %v", err)
	}
	fileServer := http.FileServer(http.FS(distFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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

	errCh := make(chan error, 1)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	webServer := &http.Server{Addr: ":" + port, Handler: loggingMiddleware(mux)}
	go func() {
		log.Printf("vitra web server listening on :%s (vault: %s, MCP at /mcp)", port, vaultPath)
		if err := webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("web server: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Printf("shutting down...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()
		_ = mcpHandler.Shutdown(shutCtx)
		_ = webServer.Shutdown(shutCtx)
	case err := <-errCh:
		log.Printf("fatal: %v", err)
	}
}