package storage

import (
	"path/filepath"
	"testing"
)

func TestSQLiteStorage_StoreFile_GetFile_ListFiles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "store.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	f := File{
		Path:      "foo.go",
		Content:   "package main",
		Hash:      "abc123",
		Size:      12,
		Mtime:     1000,
		IndexedAt: 2000,
	}
	if err := s.StoreFile(f); err != nil {
		t.Fatalf("StoreFile: %v", err)
	}

	got, err := s.GetFile("foo.go")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if got == nil || got.Path != f.Path || got.Content != f.Content || got.Hash != f.Hash {
		t.Errorf("GetFile: got %+v", got)
	}

	paths, err := s.ListFiles()
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(paths) != 1 || paths[0] != "foo.go" {
		t.Errorf("ListFiles: got %v", paths)
	}
}

func TestSQLiteStorage_StoreChunks(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "chunks.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	if err := s.StoreFile(File{Path: "a.go", Content: "x", Hash: "h", Size: 1, Mtime: 1, IndexedAt: 1}); err != nil {
		t.Fatal(err)
	}
	chunks := []Chunk{
		{Content: "chunk1", Type: "text", StartLine: 1, EndLine: 2},
		{Content: "chunk2", Type: "text", StartLine: 3, EndLine: 4},
	}
	_, err = s.StoreChunks("a.go", chunks)
	if err != nil {
		t.Fatalf("StoreChunks: %v", err)
	}
	chunks2 := []Chunk{
		{Content: "only one", Type: "text", StartLine: 1, EndLine: 1},
	}
	_, err = s.StoreChunks("a.go", chunks2)
	if err != nil {
		t.Fatalf("StoreChunks replace: %v", err)
	}
	// Verify: 1 file, 1 chunk after replace
	n, err := s.FileCount()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("FileCount: got %d", n)
	}
	cn, err := s.ChunkCount()
	if err != nil {
		t.Fatal(err)
	}
	if cn != 1 {
		t.Errorf("ChunkCount: got %d", cn)
	}
}

func TestSQLiteStorage_StoreChunkVectors(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "vec.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.StoreFile(File{Path: "f.go", Content: "x", Hash: "h", Size: 1, Mtime: 1, IndexedAt: 1}); err != nil {
		t.Fatal(err)
	}
	ids, err := s.StoreChunks("f.go", []Chunk{{Content: "c", Type: "text", StartLine: 1, EndLine: 1}})
	if err != nil || len(ids) != 1 {
		t.Fatalf("StoreChunks: %v", err)
	}
	vecs := []VectorRow{
		{Term: "get", TF: 0.5, TFIDF: 1.2, RawFreq: 2},
		{Term: "user", TF: 0.3, TFIDF: 0.8, RawFreq: 1},
	}
	if err := s.StoreChunkVectors(ids[0], vecs); err != nil {
		t.Fatalf("StoreChunkVectors: %v", err)
	}
	// Replace vectors
	if err := s.StoreChunkVectors(ids[0], []VectorRow{{Term: "only", TF: 1, TFIDF: 1, RawFreq: 1}}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteStorage_SearchCandidates(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "search.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.StoreFile(File{Path: "p.go", Content: "x", Hash: "h", Size: 1, Mtime: 1, IndexedAt: 1}); err != nil {
		t.Fatal(err)
	}
	ids, err := s.StoreChunks("p.go", []Chunk{{Content: "get user", Type: "text", StartLine: 1, EndLine: 1}})
	if err != nil || len(ids) != 1 {
		t.Fatal(err)
	}
	if err := s.StoreChunkVectors(ids[0], []VectorRow{
		{Term: "get", TF: 0.5, TFIDF: 1.0, RawFreq: 1},
		{Term: "user", TF: 0.5, TFIDF: 1.0, RawFreq: 1},
	}); err != nil {
		t.Fatal(err)
	}
	idf, candidates, err := s.SearchCandidates([]string{"get", "user"})
	if err != nil {
		t.Fatal(err)
	}
	if len(idf) != 2 || idf["get"] <= 0 || idf["user"] <= 0 {
		t.Errorf("idf: %v", idf)
	}
	if len(candidates) != 1 || candidates[0].FilePath != "p.go" {
		t.Errorf("candidates: %+v", candidates)
	}
	if len(candidates[0].Terms) != 2 {
		t.Errorf("terms: %v", candidates[0].Terms)
	}
}
