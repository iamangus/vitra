package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFileTreeIncludesSkills(t *testing.T) {
	for _, skillsDir := range []string{"skills", "custom-skills"} {
		t.Run(skillsDir, func(t *testing.T) {
			vault := t.TempDir()
			if err := os.MkdirAll(filepath.Join(vault, skillsDir), 0755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"example.md", ".hidden.md", "index.md", "log.md", "attachment.txt"} {
				if err := os.WriteFile(filepath.Join(vault, skillsDir, name), []byte("---\ntype: Skill\n---\n\nExample skill"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			fs := NewFileSystem(vault)
			fs.SetSkillsDir(skillsDir)
			response := httptest.NewRecorder()
			fs.HandleAPIFileTree(response, httptest.NewRequest(http.MethodGet, "/api/files?active="+skillsDir+"/example", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			var tree []FileNode
			if err := json.Unmarshal(response.Body.Bytes(), &tree); err != nil {
				t.Fatal(err)
			}
			if len(tree) != 1 || tree[0].Path != skillsDir || !tree[0].IsDir {
				t.Fatalf("expected visible skills folder, got %+v", tree)
			}
			children := tree[0].Children
			if len(children) != 1 || children[0].Path != skillsDir+"/example" || !children[0].IsActive {
				t.Fatalf("expected only visible active skill note, got %+v", children)
			}
			if err := fs.BuildIndex(); err != nil {
				t.Fatal(err)
			}
			if len(fs.index.notes) != 0 {
				t.Fatalf("skills should remain excluded from the vault index, got %+v", fs.index.notes)
			}
			concepts, _, err := fs.ListConcepts(ConceptFilter{})
			if err != nil || len(concepts) != 0 {
				t.Fatalf("skills should remain excluded from concepts, got %+v, %v", concepts, err)
			}
		})
	}
}
