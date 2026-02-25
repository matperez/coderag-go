package search

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/matperez/coderag-go/internal/embeddings"
	"github.com/matperez/coderag-go/internal/storage"
	"github.com/matperez/coderag-go/internal/vectorstore"
)

func TestHybridFromStorage_BM25Only(t *testing.T) {
	ctx := context.Background()
	// Nil opts -> BM25 only
	idf := map[string]float64{"foo": 1.5, "bar": 1.2}
	candidates := []StorageCandidate{
		{ChunkID: 1, FilePath: "a.go", Content: "foo bar", StartLine: 1, EndLine: 2, TokenCount: 2, Terms: map[string]TermScore{"foo": {RawFreq: 1}, "bar": {RawFreq: 1}}},
	}
	results, err := HybridFromStorage(ctx, "foo bar", nil, idf, candidates, 2.0, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].ChunkID != 1 || results[0].Content != "foo bar" {
		t.Errorf("result = %+v", results[0])
	}
}

func TestHybridFromStorage_WithMockVector(t *testing.T) {
	ctx := context.Background()
	// Use mock store and mock embedder so we get both BM25 and vector contribution
	st := setupHybridStorage(t)
	defer st.Close()

	idf := map[string]float64{"handle": 1.2, "request": 1.1}
	candidates := []StorageCandidate{
		{ChunkID: 10, FilePath: "api.go", Content: "func HandleRequest", StartLine: 5, EndLine: 10, TokenCount: 4, Terms: map[string]TermScore{"handle": {RawFreq: 1}, "request": {RawFreq: 1}}},
		{ChunkID: 20, FilePath: "other.go", Content: "something else", StartLine: 1, EndLine: 2, TokenCount: 2, Terms: map[string]TermScore{}},
	}
	vecStore := &vectorstore.MockStore{
		Rows: []vectorstore.Row{
			{ID: 10, Vector: []float32{1, 0}, Metadata: "api.go"},
			{ID: 20, Vector: []float32{0, 1}, Metadata: "other.go"},
		},
	}
	defer vecStore.Close()
	emb := &embeddings.MockProvider{Dimension: 2}

	results, err := HybridFromStorage(ctx, "handle request", st, idf, candidates, 3.0, 5, &HybridOpts{
		VecStore: vecStore, Embedder: emb, BM25Weight: 0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Should get at least chunk 10 (BM25 + vector) and possibly 20 (vector only)
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	// First result should be chunk 10 (matches both keyword and is first in vector)
	if results[0].ChunkID != 10 {
		t.Errorf("first result ChunkID = %d, want 10", results[0].ChunkID)
	}
	// Results should contain both keyword match and semantic (vector) data
	found10 := false
	for _, r := range results {
		if r.ChunkID == 10 {
			found10 = true
			if r.Content != "func HandleRequest" {
				t.Errorf("chunk 10 content = %q", r.Content)
			}
			break
		}
	}
	if !found10 {
		t.Error("expected chunk 10 in results")
	}
}

func setupHybridStorage(t *testing.T) *storage.SQLiteStorage {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "db.sqlite")
	st, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return st
}
