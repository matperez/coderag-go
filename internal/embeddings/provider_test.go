package embeddings

import (
	"context"
	"os"
	"testing"
)

func TestTruncateToRunes(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"abc", 3, "abc"},
		{"abc", 5, "abc"},
		{"abc", 0, "abc"},
		{"abc", -1, "abc"},
		{"hello", 2, "he"},
		{"привет", 3, "при"},
		{"a\x80b", 2, "a\x80"}, // invalid UTF-8 rune still counts as one rune
	}
	for _, tt := range tests {
		got := truncateToRunes(tt.s, tt.n)
		if got != tt.want {
			t.Errorf("truncateToRunes(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestMockProvider_GenerateEmbedding(t *testing.T) {
	ctx := context.Background()
	p := &MockProvider{Dimension: 4}
	vec, err := p.GenerateEmbedding(ctx, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 4 {
		t.Errorf("len(vec) = %d, want 4", len(vec))
	}
}

func TestMockProvider_GenerateEmbeddings(t *testing.T) {
	ctx := context.Background()
	p := &MockProvider{Dimension: 8}
	vecs, err := p.GenerateEmbeddings(ctx, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 {
		t.Fatalf("len(vecs) = %d", len(vecs))
	}
	if len(vecs[0]) != 8 || len(vecs[1]) != 8 {
		t.Errorf("dimension: %d %d", len(vecs[0]), len(vecs[1]))
	}
}

func TestDefaultOpenAIConfig(t *testing.T) {
	cfg := DefaultOpenAIConfig()
	if cfg.Model == "" {
		t.Error("default model should be set")
	}
	if cfg.BaseURL == "" {
		t.Error("default base URL should be set")
	}
}

func TestOpenAIProvider_GenerateEmbedding_noKey(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{APIKey: "", BaseURL: "https://api.openai.com/v1", Model: "x"})
	_, err := p.GenerateEmbedding(context.Background(), "hi")
	if err == nil {
		t.Error("expected error when API key is empty")
	}
}

func TestOpenAIProvider_Integration(t *testing.T) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}
	cfg := DefaultOpenAIConfig()
	cfg.APIKey = key
	p := NewOpenAIProvider(cfg)
	vec, err := p.GenerateEmbedding(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) == 0 {
		t.Error("expected non-empty vector")
	}
}
