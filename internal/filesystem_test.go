package internal

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func testFS(t *testing.T) *FileSystem {
	t.Helper()
	f := NewFileSystem(t.TempDir())
	f.SetSkillsDir("skills")
	if err := f.BuildIndex(); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestContainmentAndRootGuards(t *testing.T) {
	f := testFS(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.VaultPath, "link")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../secret", "link/secret", "/etc/passwd", ".chromem/state", "a/../../x", "a\\b"} {
		if _, err := f.ReadNote(p); err == nil {
			t.Errorf("read accepted %q", p)
		}
		if err := f.WriteNote(p, "overwrite"); err == nil {
			t.Errorf("write accepted %q", p)
		}
	}
	for _, p := range []string{"", ".", "a/..", "./"} {
		if err := f.DeleteNote(p); err == nil {
			t.Errorf("delete root accepted %q", p)
		}
		if err := f.RenameNote(p, "moved"); err == nil {
			t.Errorf("rename root accepted %q", p)
		}
	}
	if _, err := f.ListNotes("../"); err == nil {
		t.Fatal("list escaped")
	}
	if _, err := f.ListActivity("../", 10); err == nil {
		t.Fatal("activity escaped")
	}
	data, _ := os.ReadFile(filepath.Join(outside, "secret.md"))
	if string(data) != "secret" {
		t.Fatal("outside data changed")
	}
}

func TestCreateConflictAndRevision(t *testing.T) {
	f := testFS(t)
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.CreateNote("one", "original") }()
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("%d successful creates", success)
	}
	note, err := f.ReadNote("one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.SaveNote("one", "updated", note.Revision, false); err != nil {
		t.Fatal(err)
	}
	if _, err = f.SaveNote("one", "stale", note.Revision, false); !errors.Is(err, ErrConflict) {
		t.Fatal("stale save accepted", err)
	}
	if err := f.CreateNote("other", "keep"); err != nil {
		t.Fatal(err)
	}
	if err := f.RenameNote("one", "other"); !errors.Is(err, ErrConflict) {
		t.Fatal("rename overwrote", err)
	}
}

func TestFolderOperationsAndExclusions(t *testing.T) {
	f := testFS(t)
	for _, p := range []string{"folder/a", "folder/deep/b", "skills/private", "index", "log"} {
		if err := f.CreateNote(p, "hello"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.RenameNote("folder", "moved"); err != nil {
		t.Fatal(err)
	}
	results, _ := f.SearchNotes("hello")
	if len(results) != 2 {
		t.Fatalf("unexpected results: %#v", results)
	}
	for _, r := range results {
		if !strings.HasPrefix(r.Path, "moved/") {
			t.Fatal(r)
		}
	}
	if err := f.DeleteNote("moved"); err != nil {
		t.Fatal(err)
	}
	results, _ = f.SearchNotes("hello")
	if len(results) != 0 {
		t.Fatal(results)
	}
	if err := f.CreateNote("a", "content"); err != nil {
		t.Fatal(err)
	}
	if err := f.RenameNote("a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := f.DeleteNote("b"); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	f := testFS(t)
	body := "\r\n\r\n# Heading\r\n\r\n  body  \r\n\r\n"
	content := "---\r\ntype: Note\r\ntags: [foo, bar]\r\ndescription: |\r\n  first line\r\n  second line\r\ncustom:\r\n  enabled: true\r\n  count: 3\r\n---\r\n" + body
	if err := f.CreateNote("one", content); err != nil {
		t.Fatal(err)
	}
	if err := f.UpdateNoteMetadata("one", map[string]interface{}{"title": "A: \"quoted\" title"}); err != nil {
		t.Fatal(err)
	}
	data, err := f.ReadVaultFile("one.md")
	if err != nil {
		t.Fatal(err)
	}
	fm, rest := parseNote(data)
	if string(rest) != body {
		t.Fatalf("body changed: %q", rest)
	}
	if fm["description"] != "first line\nsecond line\n" {
		t.Fatal(fm)
	}
	if _, ok := fm["custom"].(map[string]interface{}); !ok {
		t.Fatal(fm)
	}
	note, err := f.ReadNote("one")
	if err != nil {
		t.Fatal(err)
	}
	if len(note.Tags) != 2 || note.Tags[0] != "foo" || note.Tags[1] != "bar" {
		t.Fatal(note.Tags)
	}
	if note.Timestamp == "" {
		t.Fatal("missing timestamp")
	}
}

func TestOversizedSavePreservesFile(t *testing.T) {
	f := testFS(t)
	if err := f.CreateNote("one", "original"); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/note/one", strings.NewReader(strings.Repeat("x", maxBodySize+1)))
	r.SetPathValue("path", "one")
	w := httptest.NewRecorder()
	f.HandleAPISaveNote(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatal(w.Code, w.Body.String())
	}
	data, _ := f.ReadVaultFile("one.md")
	if string(data) != "original" {
		t.Fatal("file changed")
	}
}

func TestDownloadFolderAndGraphArrays(t *testing.T) {
	f := testFS(t)
	if err := f.CreateNote("folder/one", "hi"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	f.HandleAPIDownload(w, httptest.NewRequest("GET", "/api/download?path=folder", nil))
	archive, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 1 || archive.File[0].Name != "one.md" {
		t.Fatal(archive.File)
	}
	w = httptest.NewRecorder()
	f.HandleAPIGraph(w, httptest.NewRequest("GET", "/api/graph", nil))
	if strings.Contains(w.Body.String(), `"links":null`) {
		t.Fatal(w.Body.String())
	}
}

func TestSanitizedMarkdown(t *testing.T) {
	f := testFS(t)
	html, err := renderMarkdown([]byte(`<img src=x onerror="alert(1)"><script>alert(1)</script> [[missing|<img src=x onerror=alert(1)>]] [bad](javascript:alert(1))`), f.VaultPath, f.index)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script", `href="javascript:`} {
		if strings.Contains(html, bad) {
			t.Fatal(html)
		}
	}
	if regexp.MustCompile(`<[^>]+\sonerror\s*=`).MatchString(html) {
		t.Fatal(html)
	}
	if !strings.Contains(html, "/note/missing") {
		t.Fatal(html)
	}
}
