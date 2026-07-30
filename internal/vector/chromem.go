package vector

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/philippgille/chromem-go"
)

// ChromemStore implements VectorStore backed by an embedded chromem-go DB.
//
// It uses an OpenAI-compatible EmbeddingClient for generating embeddings,
// wraps it as a chromem.EmbeddingFunc for the collection's query path, and
// stores pre-computed embeddings alongside note chunks as documents.
type ChromemStore struct {
	db        *chromem.DB
	coll      *chromem.Collection
	embedder  *EmbeddingClient
	collName  string

	// mu guards pathEmbeddings which caches one (averaged) embedding per note
	// path so FindSimilarFiles can retrieve a reference vector without reading
	// the note file from disk (the store has no knowledge of the vault path).
	mu            sync.RWMutex
	pathEmbeddings map[string][]float32
}

// NewChromemStore opens (or creates) a persistent chromem-go DB at persistPath
// (use "" for in-memory) and ensures the notes collection exists. When the
// embedder has no API key configured the store still works for read paths but
// indexing/querying will return an error from the embedding API.
func NewChromemStore(persistPath string, compress bool, embedder *EmbeddingClient) (*ChromemStore, error) {
	if embedder == nil {
		embedder = NewEmbeddingClient()
	}

	var db *chromem.DB
	var err error
	if persistPath == "" {
		db = chromem.NewDB()
	} else {
		if err := os.MkdirAll(persistPath, 0755); err != nil {
			return nil, fmt.Errorf("create chromem dir: %w", err)
		}
		db, err = chromem.NewPersistentDB(persistPath, compress)
		if err != nil {
			return nil, fmt.Errorf("open persistent chromem db: %w", err)
		}
	}

	const collName = "vitra_notes"
	embedFunc := chromem.EmbeddingFunc(func(ctx context.Context, text string) ([]float32, error) {
		return embedder.EmbedText(text)
	})
	coll, err := db.GetOrCreateCollection(collName, nil, embedFunc)
	if err != nil {
		return nil, fmt.Errorf("create chromem collection: %w", err)
	}

	return &ChromemStore{
		db:             db,
		coll:           coll,
		embedder:       embedder,
		collName:       collName,
		pathEmbeddings: make(map[string][]float32),
	}, nil
}

// IndexNote chunks-are-already-provided: we embed them in batch and add as
// documents. Existing chunks for the same path are removed first so re-writes
// don't accumulate stale entries.
func (c *ChromemStore) IndexNote(ctx context.Context, path string, chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	// Remove any previously-stored chunks for this path.
	if err := c.coll.Delete(ctx, map[string]string{"path": path}, nil); err != nil {
		// Non-fatal on first index of a new path.
		fmt.Fprintf(os.Stderr, "chromem: delete before re-index %s: %v\n", path, err)
	}

	texts := make([]string, len(chunks))
	for i, ch := range chunks {
		texts[i] = ch.Text
	}
	embeddings, err := c.embedder.EmbedTexts(texts)
	if err != nil {
		return fmt.Errorf("embed chunks: %w", err)
	}

	ids := make([]string, len(chunks))
	metas := make([]map[string]string, len(chunks))
	contents := make([]string, len(chunks))
	for i, ch := range chunks {
		ids[i] = fmt.Sprintf("%s#%d", path, ch.Index)
		meta := map[string]string{
			"path":    path,
			"title":   getTitleFromPath(path),
			"heading": ch.Heading,
			"index":   fmt.Sprintf("%d", ch.Index),
			"type":    ch.Type,
		}
		for _, tag := range ch.Tags {
			tag = strings.ReplaceAll(strings.ToLower(tag), " ", "_")
			meta["tag_"+tag] = "1"
		}
		metas[i] = meta
		contents[i] = ch.Text
	}

	if err := c.coll.Add(ctx, ids, embeddings, metas, contents); err != nil {
		return fmt.Errorf("add documents: %w", err)
	}

	// Cache an averaged embedding for FindSimilarFiles.
	if avg := averageEmbeddings(embeddings); avg != nil {
		c.mu.Lock()
		c.pathEmbeddings[path] = avg
		c.mu.Unlock()
	}
	return nil
}

// DeleteNote removes all chunks for a path from the store.
func (c *ChromemStore) DeleteNote(ctx context.Context, path string) error {
	if err := c.coll.Delete(ctx, map[string]string{"path": path}, nil); err != nil {
		return fmt.Errorf("delete note %s: %w", path, err)
	}
	c.mu.Lock()
	delete(c.pathEmbeddings, path)
	c.mu.Unlock()
	return nil
}

// SemanticSearch embeds the query, retrieves an expanded candidate set from
// chromem, applies keyword boosting to bridge vocabulary gaps between the
// query and note text, then returns the top N re-ranked results.
func (c *ChromemStore) SemanticSearch(ctx context.Context, query string, limit int, filter Filter) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	emb, err := c.embedder.EmbedText(query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	where := buildWhere(filter)

	// Fetch more candidates than needed so re-ranking has room to work.
	fetchLimit := limit * 4
	if fetchLimit < 20 {
		fetchLimit = 20
	}

	results, err := c.coll.QueryEmbedding(ctx, emb, fetchLimit, where, nil)
	if err != nil {
		return nil, err
	}

	searchResults := c.toSearchResults(results)

	// Keyword boosting: adjust distance scores based on literal keyword overlap.
	// Disabled when HYBRID_BOOST_FACTOR is 0 or unset.
	if bf := hybridBoostFactor(); bf > 0 {
		keywords := extractKeywords(query)
		if len(keywords) > 0 {
			applyKeywordBoost(searchResults, keywords, bf)
			sort.Slice(searchResults, func(i, j int) bool {
				return searchResults[i].Distance < searchResults[j].Distance
			})
		}
	}

	if len(searchResults) > limit {
		searchResults = searchResults[:limit]
	}

	return searchResults, nil
}

// FindSimilarFiles returns notes semantically similar to the given path,
// excluding the note itself.
func (c *ChromemStore) FindSimilarFiles(ctx context.Context, path string, limit int, filter Filter) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	c.mu.RLock()
	emb, ok := c.pathEmbeddings[path]
	c.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("note not indexed: %s", path)
	}
	where := buildWhere(filter)
	results, err := c.coll.QueryEmbedding(ctx, emb, limit+20, where, nil)
	if err != nil {
		return nil, err
	}
	all := c.toSearchResults(results)

	var filtered []SearchResult
	seen := make(map[string]bool)
	for _, r := range all {
		if r.Path == path || seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		filtered = append(filtered, r)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered, nil
}

// CheckDuplicate finds an existing note whose similarity to `content` is at
// least `threshold` (cosine similarity, higher = more similar).
func (c *ChromemStore) CheckDuplicate(ctx context.Context, content string, threshold float32) (*SearchResult, error) {
	if threshold <= 0 {
		threshold = 0.95
	}
	results, err := c.SemanticSearch(ctx, content, 1, Filter{})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	// Distance is stored as 1 - similarity, so similarity = 1 - distance.
	similarity := float32(1.0) - results[0].Distance
	if similarity >= threshold {
		return &results[0], nil
	}
	return nil, nil
}

// ReindexVault drops the collection (and its documents) so the caller can
// re-populate it from scratch.
func (c *ChromemStore) ReindexVault(ctx context.Context, vaultPath string) error {
	_ = vaultPath // collection is dropped & recreated
	if err := c.db.DeleteCollection(c.collName); err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	embedFunc := chromem.EmbeddingFunc(func(ctx context.Context, text string) ([]float32, error) {
		return c.embedder.EmbedText(text)
	})
	coll, err := c.db.GetOrCreateCollection(c.collName, nil, embedFunc)
	if err != nil {
		return fmt.Errorf("recreate collection: %w", err)
	}
	c.coll = coll
	c.mu.Lock()
	c.pathEmbeddings = make(map[string][]float32)
	c.mu.Unlock()
	return nil
}

// Close is a no-op for the embedded store; persistence is flushed on each write.
func (c *ChromemStore) Close() error { return nil }

// toSearchResults converts chromem results into VectorStore results, encoding
// similarity as Distance = 1 - similarity to preserve the "lower is better"
// convention used by the rest of the vector package.
func (c *ChromemStore) toSearchResults(results []chromem.Result) []SearchResult {
	out := make([]SearchResult, 0, len(results))
	for _, r := range results {
		path := r.Metadata["path"]
		title := r.Metadata["title"]
		heading := r.Metadata["heading"]
		typ := r.Metadata["type"]
		var tags []string
		for k, v := range r.Metadata {
			if strings.HasPrefix(k, "tag_") && v == "1" {
				tags = append(tags, strings.TrimPrefix(k, "tag_"))
			}
		}
		sort.Strings(tags)
		out = append(out, SearchResult{
			Path:     path,
			Title:    title,
			Heading:  heading,
			Chunk:    r.Content,
			Distance: 1.0 - r.Similarity,
			Type:     typ,
			Tags:     tags,
		})
	}
	return out
}

func averageEmbeddings(embeddings [][]float32) []float32 {
	if len(embeddings) == 0 {
		return nil
	}
	dim := len(embeddings[0])
	avg := make([]float32, dim)
	for _, emb := range embeddings {
		for i, v := range emb {
			avg[i] += v
		}
	}
	for i := range avg {
		avg[i] /= float32(len(embeddings))
	}
	return avg
}

// buildWhere translates an OKF Filter into a chromem `where` map. chromem
// matches all keys exactly (AND across keys). For tag filtering we emit one
// key per required tag (tag_<slug> = "1"); chromem's AND across keys gives the
// intersection (AND) semantics specified in the plan.
func buildWhere(f Filter) map[string]string {
	if f.Type == "" && len(f.Tags) == 0 {
		return nil
	}
	w := map[string]string{}
	if f.Type != "" {
		w["type"] = f.Type
	}
	for _, tag := range f.Tags {
		tag = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(tag)), " ", "_")
		if tag == "" {
			continue
		}
		w["tag_"+tag] = "1"
	}
	return w
}

func getTitleFromPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "was": true, "were": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true, "with": true,
	"and": true, "or": true, "but": true, "not": true, "it": true, "this": true, "that": true,
	"be": true, "how": true, "what": true, "when": true, "where": true, "who": true, "why": true,
	"do": true, "does": true, "did": true, "can": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true, "has": true, "have": true,
	"had": true, "been": true, "being": true, "our": true, "my": true, "your": true,
	"their": true, "we": true, "you": true, "us": true, "them": true, "me": true,
	"him": true, "her": true, "its": true, "by": true, "from": true, "than": true,
	"into": true, "up": true, "out": true, "i": true, "he": true, "she": true,
	"they": true, "more": true, "no": true, "just": true, "so": true, "if": true,
	"as": true, "also": true, "about": true, "over": true, "only": true, "very": true,
}

func extractKeywords(query string) []string {
	seen := make(map[string]bool)
	var keywords []string
	for _, word := range strings.Fields(strings.ToLower(query)) {
		word = strings.Trim(word, `"'.,;:!?()[]{}`)
		if len(word) < 2 || stopWords[word] {
			continue
		}
		if seen[word] {
			continue
		}
		seen[word] = true
		keywords = append(keywords, word)
	}
	return keywords
}

func applyKeywordBoost(results []SearchResult, keywords []string, boostFactor float64) {
	for i := range results {
		chunkLower := strings.ToLower(results[i].Chunk)
		titleLower := strings.ToLower(results[i].Title)
		var matches float64
		for _, kw := range keywords {
			if strings.Contains(chunkLower, kw) || strings.Contains(titleLower, kw) {
				matches++
			}
		}
		if len(keywords) > 0 && matches > 0 {
			keywordScore := matches / float64(len(keywords))
			similarity := 1.0 - float64(results[i].Distance)
			boostedSim := similarity * (1.0 + boostFactor*keywordScore)
			results[i].Distance = float32(math.Max(0.0, 1.0-boostedSim))
		}
	}
}

func hybridBoostFactor() float64 {
	if v := os.Getenv("HYBRID_BOOST_FACTOR"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0.2
}