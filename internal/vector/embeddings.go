package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

// EmbeddingClient handles communication with OpenAI-compatible embedding APIs
type EmbeddingClient struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

// NewEmbeddingClient creates a new embedding client from environment variables
func NewEmbeddingClient() *EmbeddingClient {
	baseURL := os.Getenv("EMBEDDING_API_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	apiKey := os.Getenv("EMBEDDING_API_KEY")
	model := os.Getenv("EMBEDDING_MODEL")
	if model == "" {
		model = "text-embedding-3-small"
	}

	return &EmbeddingClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// embeddingRequest represents the request body for embedding API
type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// embeddingResponse represents the response from embedding API
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// EmbedTexts generates embeddings for a batch of texts
func (c *EmbeddingClient) EmbedTexts(texts []string) ([][]float32, error) {
	return c.EmbedTextsContext(context.Background(), texts)
}

func (c *EmbeddingClient) EmbedTextsContext(ctx context.Context, texts []string) ([][]float32, error) {
	var out [][]float32
	for start := 0; start < len(texts); {
		end, size := start, 0
		for end < len(texts) && end-start < 32 {
			if len(texts[end]) > 24000 {
				return nil, fmt.Errorf("embedding input exceeds 24000 bytes")
			}
			if end > start && size+len(texts[end]) > 96000 {
				break
			}
			size += len(texts[end])
			end++
		}
		batch, err := c.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		if len(out) > 0 && len(batch) > 0 && len(out[0]) != len(batch[0]) {
			return nil, fmt.Errorf("embedding dimensions changed between batches")
		}
		out = append(out, batch...)
		start = end
	}
	return out, nil
}

func (c *EmbeddingClient) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if c.APIKey == "" && strings.TrimRight(c.BaseURL, "/") == "https://api.openai.com/v1" {
		return nil, fmt.Errorf("embedding provider not configured")
	}
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := embeddingRequest{
		Model: c.Model,
		Input: texts,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.BaseURL, "/")+"/embeddings", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	var resp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		req.Body = io.NopCloser(bytes.NewReader(jsonBody))
		resp, err = client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("embedding request: %w", err)
		}
		if (resp.StatusCode != 429 && resp.StatusCode < 500) || attempt == 2 {
			break
		}
		resp.Body.Close()
		timer := time.NewTimer(time.Duration(attempt+1) * 200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp embeddingResponse
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&errResp); err == nil && errResp.Error != nil {
			return nil, fmt.Errorf("embedding API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("embedding API returned status %d", resp.StatusCode)
	}

	var result embeddingResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("embedding API error: %s", result.Error.Message)
	}

	// Sort embeddings by index to maintain order
	embeddings := make([][]float32, len(texts))
	dimension := 0
	for _, data := range result.Data {
		if data.Index < 0 || data.Index >= len(embeddings) || embeddings[data.Index] != nil || len(data.Embedding) == 0 {
			return nil, fmt.Errorf("invalid embedding response index or vector")
		}
		if dimension == 0 {
			dimension = len(data.Embedding)
		}
		if len(data.Embedding) != dimension {
			return nil, fmt.Errorf("inconsistent embedding dimensions")
		}
		var norm float64
		for _, v := range data.Embedding {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return nil, fmt.Errorf("non-finite embedding")
			}
			norm += float64(v) * float64(v)
		}
		if norm == 0 {
			return nil, fmt.Errorf("zero embedding")
		}
		embeddings[data.Index] = data.Embedding
	}
	for _, emb := range embeddings {
		if len(emb) == 0 {
			return nil, fmt.Errorf("missing embedding")
		}
	}

	return embeddings, nil
}

// EmbedText generates embedding for a single text
func (c *EmbeddingClient) EmbedText(text string) ([]float32, error) {
	return c.EmbedTextContext(context.Background(), text)
}

func (c *EmbeddingClient) EmbedTextContext(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.EmbedTextsContext(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}
