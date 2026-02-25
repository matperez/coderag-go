package vectorstore

import "context"

// MockStore is an in-memory Store for tests (no LanceDB required).
type MockStore struct {
	Rows []Row
}

// Upsert appends rows to the in-memory slice (no dedup).
func (m *MockStore) Upsert(ctx context.Context, rows []Row) error {
	m.Rows = append(m.Rows, rows...)
	return nil
}

// Search returns the first k rows as "results" (no actual vector similarity).
func (m *MockStore) Search(ctx context.Context, query []float32, k int) ([]SearchResult, error) {
	n := k
	if n > len(m.Rows) {
		n = len(m.Rows)
	}
	out := make([]SearchResult, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, SearchResult{ChunkID: m.Rows[i].ID, Metadata: m.Rows[i].Metadata})
	}
	return out, nil
}

// DeleteChunk removes rows with the given chunk ID.
func (m *MockStore) DeleteChunk(ctx context.Context, chunkID int64) error {
	filtered := m.Rows[:0]
	for _, r := range m.Rows {
		if r.ID != chunkID {
			filtered = append(filtered, r)
		}
	}
	m.Rows = filtered
	return nil
}

// Close is a no-op.
func (m *MockStore) Close() error { return nil }
