//go:build lancedb

package vectorstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLanceStore_Open_Upsert_Search(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dim := 4

	s, err := Open(ctx, dir, dim)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Insert rows
	rows := []Row{
		{ID: 1, Vector: []float32{1, 0, 0, 0}, Metadata: "file.go"},
		{ID: 2, Vector: []float32{0, 1, 0, 0}, Metadata: "other.go"},
		{ID: 3, Vector: []float32{0, 0, 1, 0}, Metadata: "pkg/a.go"},
	}
	if err := s.Upsert(ctx, rows); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Search for vector closest to [1,0,0,0] -> expect chunk 1 first
	results, err := s.Search(ctx, []float32{1, 0, 0, 0}, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 1 {
		t.Fatal("expected at least one result")
	}
	if results[0].ChunkID != 1 {
		t.Errorf("first result ChunkID = %d, want 1", results[0].ChunkID)
	}
	if results[0].Metadata != "file.go" {
		t.Errorf("first result Metadata = %q, want file.go", results[0].Metadata)
	}
}

func TestLanceStore_DeleteChunk(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dim := 4

	s, err := Open(ctx, dir, dim)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.Upsert(ctx, []Row{
		{ID: 10, Vector: []float32{1, 1, 1, 1}, Metadata: "x"},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	results, err := s.Search(ctx, []float32{1, 1, 1, 1}, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].ChunkID != 10 {
		t.Fatalf("expected one result with id 10, got %v", results)
	}

	if err := s.DeleteChunk(ctx, 10); err != nil {
		t.Fatalf("DeleteChunk: %v", err)
	}

	results2, err := s.Search(ctx, []float32{1, 1, 1, 1}, 5)
	if err != nil {
		t.Fatalf("Search after delete: %v", err)
	}
	if len(results2) != 0 {
		t.Errorf("after DeleteChunk expected 0 results, got %d", len(results2))
	}
}

func TestLanceStore_Open_createsDir(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "sub", "vec")
	_, err := Open(ctx, dir, 8)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("Open should create directory: %v", err)
	}
}
