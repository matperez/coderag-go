package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// OpenAIConfig configures the OpenAI-compatible HTTP client.
type OpenAIConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	BatchSize int
	// NumCtx when > 0 is sent as options.num_ctx (e.g. for Ollama). Set via OLLAMA_NUM_CTX or EMBEDDING_NUM_CTX.
	NumCtx int
	// MaxInputChars truncates each input to this many runes when > 0. If 0 and NumCtx > 0, a safe value is derived for batch.
	MaxInputChars int
}

const defaultOpenAIBase = "https://api.openai.com/v1"

// DefaultOpenAIConfig returns config from environment: OPENAI_API_KEY, OPENAI_BASE_URL (optional),
// EMBEDDING_MODEL or OPENAI_EMBEDDING_MODEL (default text-embedding-3-small).
// BaseURL must be the API root so that BaseURL+"/embeddings" is the embeddings endpoint (e.g. https://api.openai.com/v1).
// Optional: OLLAMA_NUM_CTX or EMBEDDING_NUM_CTX set context size (e.g. 8192), sent as options.num_ctx when supported; inputs are truncated to fit. EMBEDDING_MAX_INPUT_CHARS overrides the per-input truncation length.
func DefaultOpenAIConfig() OpenAIConfig {
	base := strings.TrimSuffix(os.Getenv("OPENAI_BASE_URL"), "/")
	if base == "" {
		base = defaultOpenAIBase
	}
	model := os.Getenv("EMBEDDING_MODEL")
	if model == "" {
		model = os.Getenv("OPENAI_EMBEDDING_MODEL")
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	numCtx := 0
	if n := os.Getenv("OLLAMA_NUM_CTX"); n != "" {
		if v, err := strconv.Atoi(n); err == nil && v > 0 {
			numCtx = v
		}
	}
	if numCtx == 0 {
		if n := os.Getenv("EMBEDDING_NUM_CTX"); n != "" {
			if v, err := strconv.Atoi(n); err == nil && v > 0 {
				numCtx = v
			}
		}
	}
	maxInputChars := 0
	if n := os.Getenv("EMBEDDING_MAX_INPUT_CHARS"); n != "" {
		if v, err := strconv.Atoi(n); err == nil && v > 0 {
			maxInputChars = v
		}
	}
	return OpenAIConfig{
		APIKey:        os.Getenv("OPENAI_API_KEY"),
		BaseURL:       base,
		Model:         model,
		BatchSize:     20,
		NumCtx:        numCtx,
		MaxInputChars: maxInputChars,
	}
}

// OpenAIProvider implements Provider via OpenAI-compatible HTTP API.
type OpenAIProvider struct {
	client *http.Client
	cfg    OpenAIConfig
}

// NewOpenAIProvider creates an OpenAI-compatible embedding provider.
func NewOpenAIProvider(cfg OpenAIConfig) *OpenAIProvider {
	return &OpenAIProvider{
		client: &http.Client{},
		cfg:    cfg,
	}
}

type openAIResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// GenerateEmbedding returns the embedding for a single text.
func (p *OpenAIProvider) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	vecs, err := p.GenerateEmbeddings(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return vecs[0], nil
}

// truncateToRunes truncates s to at most maxRunes runes.
func truncateToRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	var n int
	for i := range s {
		if n == maxRunes {
			return s[:i]
		}
		n++
	}
	return s
}

// GenerateEmbeddings returns embeddings for multiple texts, batching requests if needed.
// Empty APIKey is allowed when BaseURL is not the default (e.g. local Ollama).
func (p *OpenAIProvider) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if p.cfg.APIKey == "" && p.cfg.BaseURL == defaultOpenAIBase {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}
	batchSize := p.cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 20
	}
	maxChars := p.cfg.MaxInputChars
	if maxChars <= 0 && p.cfg.NumCtx > 0 {
		// Conservative: ~2 chars per token so code and non-ASCII stay under context (per-input or whole batch).
		maxChars = (p.cfg.NumCtx * 2) / batchSize
		if maxChars < 1 {
			maxChars = 1
		}
	}
	var all [][]float32
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]
		if maxChars > 0 {
			truncated := make([]string, len(batch))
			for j, t := range batch {
				truncated[j] = truncateToRunes(t, maxChars)
			}
			batch = truncated
		}
		body := map[string]interface{}{
			"input": batch,
			"model": p.cfg.Model,
		}
		if len(batch) == 1 {
			body["input"] = batch[0]
		}
		if p.cfg.NumCtx > 0 {
			body["options"] = map[string]int{"num_ctx": p.cfg.NumCtx}
		}
		enc, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.BaseURL+"/embeddings", bytes.NewReader(enc))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			url := p.cfg.BaseURL + "/embeddings"
			respBody, _ := io.ReadAll(resp.Body)
			detail := strings.TrimSpace(string(respBody))
			if dir, err := os.Getwd(); err == nil {
				_ = os.WriteFile(dir+"/embed_failed_request.json", enc, 0644)
				_ = os.WriteFile(dir+"/embed_failed_response.json", respBody, 0644)
			}
			if len(detail) > 200 {
				detail = detail[:200] + "..."
			}
			if resp.StatusCode == http.StatusNotFound {
				return nil, fmt.Errorf("embeddings API: %s (check OPENAI_BASE_URL: must point to API root so that %s/embeddings exists)", resp.Status, p.cfg.BaseURL)
			}
			if detail != "" {
				return nil, fmt.Errorf("embeddings API: %s (%s): %s (request/response dumped to embed_failed_request.json, embed_failed_response.json)", resp.Status, url, detail)
			}
			return nil, fmt.Errorf("embeddings API: %s (%s) (request/response dumped to embed_failed_*.json)", resp.Status, url)
		}
		var data openAIResp
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return nil, err
		}
		for _, d := range data.Data {
			all = append(all, d.Embedding)
		}
	}
	return all, nil
}
