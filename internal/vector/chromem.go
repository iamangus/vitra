package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/philippgille/chromem-go"
)

type ChromemStore struct {
	mu             sync.RWMutex
	db             *chromem.DB
	coll           *chromem.Collection
	embedder       *EmbeddingClient
	persistPath    string
	model          string
	pathEmbeddings map[string][]float32
}

type snapshot struct {
	Model      string
	Data       []byte
	Embeddings map[string][]float32
}

func NewChromemStore(persistPath string, compress bool, embedder *EmbeddingClient) (*ChromemStore, error) {
	if embedder == nil {
		embedder = NewEmbeddingClient()
	}
	c := &ChromemStore{db: chromem.NewDB(), embedder: embedder, persistPath: persistPath, model: embedder.Model + "@" + embedder.BaseURL, pathEmbeddings: map[string][]float32{}}
	if persistPath != "" {
		if err := os.MkdirAll(persistPath, 0700); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Join(persistPath, "snapshot.json"))
		if err == nil {
			var state snapshot
			if err := json.Unmarshal(data, &state); err != nil {
				return nil, fmt.Errorf("read vector snapshot: %w", err)
			}
			if err := c.db.ImportFromReader(bytes.NewReader(state.Data), ""); err != nil {
				return nil, err
			}
			c.model = state.Model
			if state.Embeddings != nil {
				c.pathEmbeddings = state.Embeddings
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	var err error
	c.coll, err = c.db.GetOrCreateCollection("vitra_notes", nil, nil)
	return c, err
}

func cloneDB(db *chromem.DB) (*chromem.DB, error) {
	var data bytes.Buffer
	if err := db.ExportToWriter(&data, false, ""); err != nil {
		return nil, err
	}
	copy := chromem.NewDB()
	if err := copy.ImportFromReader(bytes.NewReader(data.Bytes()), ""); err != nil {
		return nil, err
	}
	return copy, nil
}

func (c *ChromemStore) publish(ctx context.Context, db *chromem.DB, embeddings map[string][]float32, model string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	coll, err := db.GetOrCreateCollection("vitra_notes", nil, nil)
	if err != nil {
		return err
	}
	if c.persistPath != "" {
		var data bytes.Buffer
		if err := db.ExportToWriter(&data, false, ""); err != nil {
			return err
		}
		encoded, err := json.Marshal(snapshot{Model: model, Data: data.Bytes(), Embeddings: embeddings})
		if err != nil {
			return err
		}
		file, err := os.CreateTemp(c.persistPath, ".snapshot-*")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		if _, err = file.Write(encoded); err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Rename(file.Name(), filepath.Join(c.persistPath, "snapshot.json")); err != nil {
			return err
		}
		c.db, c.coll, c.pathEmbeddings, c.model = db, coll, embeddings, model
		if runtime.GOOS == "windows" {
			return nil
		}
		dir, err := os.Open(c.persistPath)
		if err != nil {
			return err
		}
		err = dir.Sync()
		dir.Close()
		if err != nil {
			return err
		}
	}
	c.db, c.coll, c.pathEmbeddings, c.model = db, coll, embeddings, model
	return nil
}

func (c *ChromemStore) prepare(ctx context.Context, path string, chunks []Chunk) ([]chromem.Document, []float32, error) {
	texts := make([]string, len(chunks))
	for i, ch := range chunks {
		texts[i] = ch.Text
	}
	embeddings, err := c.embedder.EmbedTextsContext(ctx, texts)
	if err != nil {
		return nil, nil, err
	}
	docs := make([]chromem.Document, len(chunks))
	for i, ch := range chunks {
		title := ch.Title
		if title == "" {
			title = getTitleFromPath(path)
		}
		meta := map[string]string{"path": path, "title": title, "heading": ch.Heading, "index": strconv.Itoa(i), "type": ch.Type}
		for _, tag := range ch.Tags {
			meta["tag_"+strings.ToLower(strings.TrimSpace(tag))] = "1"
		}
		docs[i] = chromem.Document{ID: fmt.Sprintf("%s#%d", path, i), Embedding: embeddings[i], Metadata: meta, Content: ch.Text}
	}
	return docs, averageEmbeddings(embeddings), nil
}

func (c *ChromemStore) IndexNote(ctx context.Context, path string, chunks []Chunk) error {
	docs, avg, err := c.prepare(ctx, path, chunks)
	if err != nil {
		return fmt.Errorf("embed %s: %w", path, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.coll.Count() > 0 && c.model != c.embedder.Model+"@"+c.embedder.BaseURL {
		return fmt.Errorf("embedding model changed; full reindex required")
	}
	db, err := cloneDB(c.db)
	if err != nil {
		return err
	}
	coll := db.GetCollection("vitra_notes", nil)
	if err := coll.Delete(ctx, map[string]string{"path": path}, nil); err != nil {
		return err
	}
	if len(docs) > 0 {
		if err := coll.AddDocuments(ctx, docs, 1); err != nil {
			return err
		}
	}
	cache := c.copyEmbeddings()
	if len(avg) > 0 {
		cache[path] = avg
	} else {
		delete(cache, path)
	}
	return c.publish(ctx, db, cache, c.model)
}

func (c *ChromemStore) copyEmbeddings() map[string][]float32 {
	copy := make(map[string][]float32, len(c.pathEmbeddings))
	for p, emb := range c.pathEmbeddings {
		copy[p] = emb
	}
	return copy
}

func (c *ChromemStore) DeleteNote(ctx context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	db, err := cloneDB(c.db)
	if err != nil {
		return err
	}
	coll := db.GetCollection("vitra_notes", nil)
	cache := c.copyEmbeddings()
	for p := range cache {
		if p == path || strings.HasPrefix(p, path+"/") {
			if err := coll.Delete(ctx, map[string]string{"path": p}, nil); err != nil {
				return err
			}
			delete(cache, p)
		}
	}
	return c.publish(ctx, db, cache, c.model)
}

func (c *ChromemStore) ReplaceNotes(ctx context.Context, notes map[string][]Chunk) error {
	db := chromem.NewDB()
	coll, err := db.GetOrCreateCollection("vitra_notes", nil, nil)
	if err != nil {
		return err
	}
	cache := map[string][]float32{}
	paths := make([]string, 0, len(notes))
	for p := range notes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		docs, avg, err := c.prepare(ctx, p, notes[p])
		if err != nil {
			return err
		}
		if len(docs) > 0 {
			if err := coll.AddDocuments(ctx, docs, 1); err != nil {
				return err
			}
			cache[p] = avg
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.publish(ctx, db, cache, c.embedder.Model+"@"+c.embedder.BaseURL)
}

func (c *ChromemStore) ReindexVault(ctx context.Context, vaultPath string) error {
	return fmt.Errorf("use ReplaceNotes with a validated vault snapshot")
}

func (c *ChromemStore) SemanticSearch(ctx context.Context, query string, limit int, filter Filter) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 100 {
		return nil, fmt.Errorf("limit must not exceed 100")
	}
	c.mu.RLock()
	empty := c.coll.Count() == 0
	c.mu.RUnlock()
	if empty {
		return []SearchResult{}, nil
	}
	emb, err := c.embedder.EmbedTextContext(ctx, query)
	if err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.model != c.embedder.Model+"@"+c.embedder.BaseURL {
		return nil, fmt.Errorf("embedding model changed; full reindex required")
	}
	count := c.coll.Count()
	if count == 0 {
		return []SearchResult{}, nil
	}
	results, err := c.coll.QueryEmbedding(ctx, emb, min(count, max(20, limit*4)), buildWhere(filter), nil)
	if err != nil {
		return nil, err
	}
	out := toSearchResults(results)
	applyKeywordBoost(out, extractKeywords(query), hybridBoostFactor())
	sort.SliceStable(out, func(i, j int) bool { return out[i].Distance < out[j].Distance })
	return uniqueResults(out, limit, ""), nil
}

func uniqueResults(results []SearchResult, limit int, exclude string) []SearchResult {
	out := []SearchResult{}
	seen := map[string]bool{}
	for _, r := range results {
		if r.Path == exclude || seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (c *ChromemStore) FindSimilarFiles(ctx context.Context, path string, limit int, filter Filter) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 100 {
		return nil, fmt.Errorf("limit must not exceed 100")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	emb, ok := c.pathEmbeddings[path]
	if !ok {
		return nil, fmt.Errorf("note not indexed: %s", path)
	}
	results, err := c.coll.QueryEmbedding(ctx, emb, c.coll.Count(), buildWhere(filter), nil)
	if err != nil {
		return nil, err
	}
	return uniqueResults(toSearchResults(results), limit, path), nil
}

func (c *ChromemStore) CheckDuplicate(ctx context.Context, content string, threshold float32) (*SearchResult, error) {
	if threshold <= 0 {
		threshold = .95
	}
	c.mu.RLock()
	empty := c.coll.Count() == 0
	c.mu.RUnlock()
	if empty {
		return nil, nil
	}
	emb, err := c.embedder.EmbedTextContext(ctx, content)
	if err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.coll.Count() == 0 {
		return nil, nil
	}
	results, err := c.coll.QueryEmbedding(ctx, emb, 1, nil, nil)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 || results[0].Similarity < threshold {
		return nil, nil
	}
	out := toSearchResults(results)
	return &out[0], nil
}

func (c *ChromemStore) Close() error { return nil }

func (c *ChromemStore) Paths() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	paths := make([]string, 0, len(c.pathEmbeddings))
	for path := range c.pathEmbeddings {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func toSearchResults(results []chromem.Result) []SearchResult {
	out := make([]SearchResult, 0, len(results))
	for _, r := range results {
		tags := []string{}
		for k, v := range r.Metadata {
			if strings.HasPrefix(k, "tag_") && v == "1" {
				tags = append(tags, strings.TrimPrefix(k, "tag_"))
			}
		}
		sort.Strings(tags)
		out = append(out, SearchResult{Path: r.Metadata["path"], Title: r.Metadata["title"], Heading: r.Metadata["heading"], Chunk: r.Content, Distance: 1 - r.Similarity, Type: r.Metadata["type"], Tags: tags})
	}
	return out
}

func averageEmbeddings(embeddings [][]float32) []float32 {
	if len(embeddings) == 0 {
		return nil
	}
	avg := make([]float32, len(embeddings[0]))
	for _, emb := range embeddings {
		if len(emb) != len(avg) {
			return nil
		}
		for i, v := range emb {
			avg[i] += v / float32(len(embeddings))
		}
	}
	return avg
}

func buildWhere(filter Filter) map[string]string {
	where := map[string]string{}
	if filter.Type != "" {
		where["type"] = filter.Type
	}
	for _, tag := range filter.Tags {
		if tag = strings.ToLower(strings.TrimSpace(tag)); tag != "" {
			where["tag_"+tag] = "1"
		}
	}
	return where
}

func getTitleFromPath(path string) string { return filepath.Base(path) }

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "was": true, "were": true, "in": true, "on": true, "at": true, "to": true, "for": true, "of": true, "with": true, "and": true, "or": true, "but": true, "not": true, "it": true, "this": true, "that": true, "be": true, "how": true, "what": true, "when": true, "where": true, "who": true, "why": true, "do": true, "does": true, "did": true, "can": true, "will": true, "would": true, "could": true, "should": true, "may": true, "might": true, "has": true, "have": true, "had": true, "been": true, "being": true, "our": true, "my": true, "your": true, "their": true, "we": true, "you": true, "us": true, "them": true, "me": true, "him": true, "her": true, "its": true, "by": true, "from": true, "than": true, "into": true, "up": true, "out": true, "i": true, "he": true, "she": true, "they": true, "more": true, "no": true, "just": true, "so": true, "if": true, "as": true, "also": true, "about": true, "over": true, "only": true, "very": true,
}

func extractKeywords(query string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, word := range strings.Fields(strings.ToLower(query)) {
		word = strings.Trim(word, `"'.,;:!?()[]{}`)
		if len(word) > 1 && !seen[word] && !stopWords[word] {
			seen[word] = true
			out = append(out, word)
		}
	}
	return out
}

func applyKeywordBoost(results []SearchResult, keywords []string, factor float64) {
	if len(keywords) == 0 || factor <= 0 {
		return
	}
	for i := range results {
		text := strings.ToLower(results[i].Title + " " + results[i].Chunk)
		matches := 0
		for _, word := range keywords {
			if strings.Contains(text, word) {
				matches++
			}
		}
		sim := 1 - float64(results[i].Distance)
		results[i].Distance = float32(math.Max(0, 1-sim*(1+factor*float64(matches)/float64(len(keywords)))))
	}
}

func hybridBoostFactor() float64 {
	if raw := os.Getenv("HYBRID_BOOST_FACTOR"); raw != "" {
		if n, err := strconv.ParseFloat(raw, 64); err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= 1 {
			return n
		}
	}
	return .2
}
