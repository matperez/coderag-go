package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// OpenAIConfig configures the OpenAI-compatible HTTP client.
type OpenAIConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	BatchSize int
}

const defaultOpenAIBase = "https://api.openai.com/v1"

// DefaultOpenAIConfig returns config from environment: OPENAI_API_KEY, OPENAI_BASE_URL (optional),
// EMBEDDING_MODEL or OPENAI_EMBEDDING_MODEL (default text-embedding-3-small).
func DefaultOpenAIConfig() OpenAIConfig {
	base := os.Getenv("OPENAI_BASE_URL")
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
	return OpenAIConfig{
		APIKey:    os.Getenv("OPENAI_API_KEY"),
		BaseURL:   base,
		Model:     model,
		BatchSize: 20,
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
		cfg:   cfg,
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
	var all [][]float32
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]
		body := map[string]interface{}{
			"input": batch,
			"model": p.cfg.Model,
		}
		if len(batch) == 1 {
			body["input"] = batch[0]
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
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("embeddings API: %s", resp.Status)
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
