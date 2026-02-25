package embeddings

import "context"

// MockProvider is a Provider that returns fixed-dimension zero vectors (for tests).
type MockProvider struct {
	Dimension int
}

// GenerateEmbedding returns a slice of zero floats of length Dimension.
func (m *MockProvider) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	dim := m.Dimension
	if dim <= 0 {
		dim = 1536
	}
	return make([]float32, dim), nil
}

// GenerateEmbeddings returns one zero vector per text.
func (m *MockProvider) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	dim := m.Dimension
	if dim <= 0 {
		dim = 1536
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, dim)
	}
	return out, nil
}
