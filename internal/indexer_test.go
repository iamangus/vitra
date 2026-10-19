package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/iamangus/vitra/internal/vector"
)

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

func TestWatcherReconcilesExternalDirectoriesAndExcludedContent(t *testing.T) {
	f := testFS(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		data := []map[string]interface{}{}
		for i, text := range body.Input {
			if strings.Contains(text, "PRIVATE") {
				t.Error("excluded content sent")
			}
			data = append(data, map[string]interface{}{"index": i, "embedding": []float32{1, .2, .5}})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
	}))
	defer server.Close()
	store, err := vector.NewChromemStore(t.TempDir(), false, &vector.EmbeddingClient{BaseURL: server.URL, Model: "mock", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	f.SetVectorStore(store)
	if err := f.StartWatcher(); err != nil {
		t.Fatal(err)
	}
	defer f.CloseWatcher()
	if err := f.CreateNote("skills/private", "PRIVATE"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(f.VaultPath, "folder/deep"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.VaultPath, "folder/deep/one.md"), []byte("external content"), 0600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return reflect.DeepEqual(store.Paths(), []string{"folder/deep/one"}) })
	if err := os.Rename(filepath.Join(f.VaultPath, "folder"), filepath.Join(f.VaultPath, "moved")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return reflect.DeepEqual(store.Paths(), []string{"moved/deep/one"}) })
	if err := os.RemoveAll(filepath.Join(f.VaultPath, "moved")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return len(store.Paths()) == 0 })
	if count, err := f.ReindexVault(context.Background()); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}
