package internal

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTimestampFilteringAndMalformedYAMLSafety(t *testing.T) {
	f := testFS(t)
	if err := f.CreateNote("dated", "---\ntimestamp: 2026-10-19T12:00:00+02:00\n---\nbody"); err != nil {
		t.Fatal(err)
	}
	if err := f.CreateNote("undated", "body"); err != nil {
		t.Fatal(err)
	}
	views, _, err := f.ListConcepts(ConceptFilter{Since: "2026-10-19T10:00:00Z"})
	if err != nil || len(views) != 1 {
		t.Fatal(views, err)
	}
	views, _, err = f.ListConcepts(ConceptFilter{Since: "2026-10-19T10:00:01Z"})
	if err != nil || len(views) != 0 {
		t.Fatal(views, err)
	}
	broken := "---\ntitle: [broken\n---\n\nkeep this body\n"
	if err := f.CreateNote("broken", broken); err != nil {
		t.Fatal(err)
	}
	if err := f.UpdateNoteMetadata("broken", map[string]interface{}{"title": "new"}); err == nil {
		t.Fatal("rewrote malformed YAML")
	}
	data, _ := f.ReadVaultFile("broken.md")
	if string(data) != broken {
		t.Fatal("malformed note changed")
	}
	if err := f.CreateNote("typed", "---\ncustom:\n  1: one\nnumber: .nan\n---\nbody"); err != nil {
		t.Fatal(err)
	}
	typed, err := f.ReadNote("typed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(typed); err != nil {
		t.Fatal("metadata not JSON-safe", err)
	}
	note, err := f.ReadNote("dated")
	if err != nil {
		t.Fatal(err)
	}
	body := "\n replacement \n\n"
	if _, err := f.PatchNote("dated", map[string]interface{}{"title": "patched"}, &body, note.Revision); err != nil {
		t.Fatal(err)
	}
	data, _ = f.ReadVaultFile("dated.md")
	if !strings.HasSuffix(string(data), body) {
		t.Fatal("replacement body changed")
	}
}
