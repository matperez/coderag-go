package vectorstore

import (
	"context"
	"testing"
)

func TestMockStore_Upsert_Search(t *testing.T) {
	ctx := context.Background()
	s := &MockStore{}
	defer s.Close()

	rows := []Row{
		{ID: 1, Vector: []float32{1, 0}, Metadata: "a.go"},
		{ID: 2, Vector: []float32{0, 1}, Metadata: "b.go"},
	}
	if err := s.Upsert(ctx, rows); err != nil {
		t.Fatal(err)
	}

	results, err := s.Search(ctx, []float32{0.5, 0.5}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].ChunkID != 1 || results[0].Metadata != "a.go" {
		t.Errorf("first result = %+v", results[0])
	}
	if results[1].ChunkID != 2 || results[1].Metadata != "b.go" {
		t.Errorf("second result = %+v", results[1])
	}
}

func TestMockStore_DeleteChunk(t *testing.T) {
	ctx := context.Background()
	s := &MockStore{Rows: []Row{
		{ID: 10, Vector: nil, Metadata: "x"},
	}}
	defer s.Close()

	if err := s.DeleteChunk(ctx, 10); err != nil {
		t.Fatal(err)
	}
	results, _ := s.Search(ctx, nil, 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results after delete, got %d", len(results))
	}
}
