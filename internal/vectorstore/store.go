package vectorstore

import (
	"context"
)

const tableName = "chunks"

// Row is a single vector row (id, vector, metadata).
type Row struct {
	ID       int64
	Vector   []float32
	Metadata string
}

// SearchResult is one k-NN result.
type SearchResult struct {
	ChunkID  int64
	Metadata string
}

// Store persists vectors and supports k-NN search.
type Store interface {
	// Upsert inserts or replaces vectors for the given rows.
	Upsert(ctx context.Context, rows []Row) error
	// Search returns the k nearest chunk IDs (and metadata) for the query vector.
	Search(ctx context.Context, query []float32, k int) ([]SearchResult, error)
	// DeleteChunk removes all vectors for the given chunk ID.
	DeleteChunk(ctx context.Context, chunkID int64) error
	Close() error
}
