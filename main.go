package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	vaultPath := os.Getenv("VAULT_PATH")
	if vaultPath == "" {
		vaultPath = "./vault"
	}

	fs := NewFileSystem(vaultPath)

	http.HandleFunc("GET /", handleIndex)
	http.HandleFunc("GET /files", fs.handleFileTree)
	http.HandleFunc("GET /note/{path...}", fs.handleViewNote)
	http.HandleFunc("GET /edit/{path...}", fs.handleEditNote)
	http.HandleFunc("POST /save/{path...}", fs.handleSaveNote)
	http.HandleFunc("POST /create-note", fs.handleCreateNote)
	http.HandleFunc("POST /create-folder", fs.handleCreateFolder)
	http.HandleFunc("POST /rename", fs.handleRename)
	http.HandleFunc("POST /delete", fs.handleDelete)
	http.HandleFunc("GET /search", fs.handleSearch)
	http.HandleFunc("GET /backlinks/{path...}", fs.handleBacklinks)
	http.HandleFunc("POST /preview/{path...}", fs.handlePreview)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Vitra starting on :%s with vault at %s", port, vaultPath)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
