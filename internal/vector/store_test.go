package vector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testEmbedder(t *testing.T) (*EmbeddingClient, *atomic.Bool) {
	t.Helper()
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "offline", 503)
			return
		}
		var request embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad", 400)
			return
		}
		var data []map[string]interface{}
		for i, text := range request.Input {
			data = append(data, map[string]interface{}{"index": i, "embedding": []float32{1, float32(len(text)%17+1) / 20, .5}})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
	}))
	t.Cleanup(server.Close)
	return &EmbeddingClient{BaseURL: server.URL, Model: "test", Client: server.Client()}, &fail
}

func TestSmallEmptyPersistentAndFailedReplacement(t *testing.T) {
	embedder, fail := testEmbedder(t)
	dir := t.TempDir()
	store, err := NewChromemStore(dir, false, embedder)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	results, err := store.SemanticSearch(ctx, "empty", 5, Filter{})
	if err != nil || len(results) != 0 {
		t.Fatal(results, err)
	}
	for _, path := range []string{"one", "two"} {
		if err := store.IndexNote(ctx, path, ChunkNote(path, "# Heading\nContent", 0, 0, "Display "+path)); err != nil {
			t.Fatal(err)
		}
	}
	results, err = store.SemanticSearch(ctx, "Content", 5, Filter{})
	if err != nil || len(results) != 2 {
		t.Fatal(results, err)
	}
	if !strings.HasPrefix(results[0].Title, "Display ") {
		t.Fatal(results)
	}
	before, err := os.ReadFile(filepath.Join(dir, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if err := store.IndexNote(ctx, "one", ChunkNote("one", "replacement", 0, 0, "")); err == nil {
		t.Fatal("expected provider error")
	}
	if err := store.ReplaceNotes(ctx, map[string][]Chunk{"new": ChunkNote("new", "replacement", 0, 0, "")}); err == nil {
		t.Fatal("expected reindex error")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "snapshot.json"))
	if string(before) != string(after) {
		t.Fatal("failed mutation changed snapshot")
	}
	fail.Store(false)
	reopened, err := NewChromemStore(dir, false, embedder)
	if err != nil {
		t.Fatal(err)
	}
	similar, err := reopened.FindSimilarFiles(ctx, "one", 5, Filter{})
	if err != nil || len(similar) != 1 || similar[0].Path != "two" {
		t.Fatal(similar, err)
	}
	if err := reopened.IndexNote(ctx, "one", nil); err != nil {
		t.Fatal(err)
	}
	if len(reopened.Paths()) != 1 {
		t.Fatal(reopened.Paths())
	}
}

func TestConcurrentIndexSearchReindex(t *testing.T) {
	embedder, _ := testEmbedder(t)
	store, err := NewChromemStore("", false, embedder)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for n := 0; n < 4; n++ {
				var err error
				switch i % 3 {
				case 0:
					err = store.IndexNote(ctx, fmt.Sprintf("note-%d", i), ChunkNote("one", "content", 0, 0, ""))
				case 1:
					_, err = store.SemanticSearch(ctx, "content", 5, Filter{})
				case 2:
					err = store.ReplaceNotes(ctx, map[string][]Chunk{"base": ChunkNote("base", "content", 0, 0, "")})
				}
				if err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestInvalidEmbeddingResponses(t *testing.T) {
	for name, body := range map[string]string{
		"negative":  `{"data":[{"index":-1,"embedding":[1,2]}]}`,
		"missing":   `{"data":[]}`,
		"empty":     `{"data":[{"index":0,"embedding":[]}]}`,
		"duplicate": `{"data":[{"index":0,"embedding":[1,2]},{"index":0,"embedding":[1,2]}]}`,
		"zero":      `{"data":[{"index":0,"embedding":[0,0]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			client := &EmbeddingClient{BaseURL: server.URL, Client: server.Client()}
			if _, err := client.EmbedText("text"); err == nil {
				t.Fatal("accepted malformed response")
			}
		})
	}
}

func TestEmbeddingCancellationAndBatching(t *testing.T) {
	client, _ := testEmbedder(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := client.EmbedTextContext(ctx, "text"); err == nil {
		t.Fatal("cancellation ignored")
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation too slow")
	}
	texts := make([]string, 65)
	for i := range texts {
		texts[i] = "text"
	}
	result, err := client.EmbedTextsContext(context.Background(), texts)
	if err != nil || len(result) != 65 {
		t.Fatal(len(result), err)
	}
	if _, err := client.EmbedText(strings.Repeat("x", 24001)); err == nil {
		t.Fatal("oversized input accepted")
	}
}

func TestChunkBoundariesOverlapAndSummary(t *testing.T) {
	if chunks := ChunkNote("summary-only", "", 0, 0, "", "searchable summary"); len(chunks) == 0 || !strings.Contains(chunks[0].Text, "searchable summary") {
		t.Fatal(chunks)
	}
	chunks := ChunkNote("one", "---\ntitle: ignored\n---\n# Heading", 30, 5, "Title", "special summary")
	if len(chunks) != 1 || !strings.Contains(chunks[0].Text, "# Heading") || !strings.Contains(chunks[0].Text, "special summary") {
		t.Fatal(chunks)
	}
	chunks = ChunkNote("one", strings.Repeat("界", 100), 30, 5, "")
	if len(chunks) != 4 {
		t.Fatal(len(chunks))
	}
	for _, chunk := range chunks {
		if strings.ContainsRune(chunk.Text, '�') {
			t.Fatal("invalid UTF-8")
		}
	}
}

func BenchmarkSnapshotReplacement(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request embeddingRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		data := []map[string]interface{}{}
		for i := range request.Input {
			emb := make([]float32, 384)
			for j := range emb {
				emb[j] = .5
			}
			data = append(data, map[string]interface{}{"index": i, "embedding": emb})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
	}))
	defer server.Close()
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprintf("notes_%d", count), func(b *testing.B) {
			store, err := NewChromemStore(b.TempDir(), false, &EmbeddingClient{BaseURL: server.URL, Model: "benchmark", Client: server.Client()})
			if err != nil {
				b.Fatal(err)
			}
			notes := map[string][]Chunk{}
			for i := 0; i < count; i++ {
				p := fmt.Sprintf("note-%d", i)
				notes[p] = ChunkNote(p, "benchmark text", 0, 0, "")
			}
			if err := store.ReplaceNotes(context.Background(), notes); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := store.IndexNote(context.Background(), "note-0", ChunkNote("note-0", "updated text", 0, 0, "")); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
