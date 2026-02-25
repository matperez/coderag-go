package embeddings

import "context"

// Provider generates vector embeddings for text.
type Provider interface {
	// GenerateEmbedding returns the embedding vector for a single text.
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
	// GenerateEmbeddings returns embedding vectors for multiple texts in one request (batch).
	GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
}
