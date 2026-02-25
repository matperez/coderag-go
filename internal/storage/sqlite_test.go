package storage

import (
	"math"
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

func TestSQLiteStorage_DeleteFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "del.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.StoreFile(File{Path: "a.go", Content: "a", Hash: "1", Size: 1, Mtime: 1, IndexedAt: 1}); err != nil {
		t.Fatal(err)
	}
	ids, _ := s.StoreChunks("a.go", []Chunk{{Content: "x", Type: "text", StartLine: 1, EndLine: 1}})
	_ = s.StoreChunkVectors(ids[0], []VectorRow{{Term: "x", TF: 1, TFIDF: 1, RawFreq: 1}})
	if err := s.DeleteFile("a.go"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetFile("a.go"); got != nil {
		t.Error("GetFile should return nil after DeleteFile")
	}
	if n, _ := s.FileCount(); n != 0 {
		t.Errorf("FileCount = %d", n)
	}
	if n, _ := s.ChunkCount(); n != 0 {
		t.Errorf("ChunkCount = %d", n)
	}
}

func TestSQLiteStorage_DocFreqs(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "df.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.StoreFile(File{Path: "a.go", Content: "a", Hash: "1", Size: 1, Mtime: 1, IndexedAt: 1}); err != nil {
		t.Fatal(err)
	}
	ids, _ := s.StoreChunks("a.go", []Chunk{
		{Content: "get user", Type: "text", StartLine: 1, EndLine: 1},
		{Content: "user id", Type: "text", StartLine: 2, EndLine: 2},
	})
	_ = s.StoreChunkVectors(ids[0], []VectorRow{{Term: "get", TF: 0.5, TFIDF: 1, RawFreq: 1}, {Term: "user", TF: 0.5, TFIDF: 1, RawFreq: 1}})
	_ = s.StoreChunkVectors(ids[1], []VectorRow{{Term: "user", TF: 0.5, TFIDF: 1, RawFreq: 1}, {Term: "id", TF: 0.5, TFIDF: 1, RawFreq: 1}})
	df, err := s.DocFreqs([]string{"get", "user", "id", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if df["get"] != 1 || df["user"] != 2 || df["id"] != 1 || df["missing"] != 0 {
		t.Errorf("DocFreqs: %v", df)
	}
}

func TestSQLiteStorage_GetChunk(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "getchunk.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.StoreFile(File{Path: "f.go", Content: "x", Hash: "h", Size: 1, Mtime: 1, IndexedAt: 1}); err != nil {
		t.Fatal(err)
	}
	ids, err := s.StoreChunks("f.go", []Chunk{{Content: "chunk content", Type: "func", StartLine: 10, EndLine: 20}})
	if err != nil || len(ids) != 1 {
		t.Fatal(err)
	}
	ci, err := s.GetChunk(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if ci == nil || ci.Path != "f.go" || ci.Content != "chunk content" || ci.StartLine != 10 || ci.EndLine != 20 {
		t.Errorf("GetChunk: %+v", ci)
	}
	if got, _ := s.GetChunk(99999); got != nil {
		t.Error("GetChunk(99999) should return nil")
	}
}

func TestSQLiteStorage_ListChunkIDsByFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "listchunk.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.StoreFile(File{Path: "a.go", Content: "a", Hash: "1", Size: 1, Mtime: 1, IndexedAt: 1}); err != nil {
		t.Fatal(err)
	}
	ids, err := s.StoreChunks("a.go", []Chunk{
		{Content: "c1", Type: "text", StartLine: 1, EndLine: 1},
		{Content: "c2", Type: "text", StartLine: 2, EndLine: 2},
	})
	if err != nil || len(ids) != 2 {
		t.Fatal(err)
	}
	list, err := s.ListChunkIDsByFile("a.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("ListChunkIDsByFile: got %v", list)
	}
	if list2, _ := s.ListChunkIDsByFile("missing.go"); len(list2) != 0 {
		t.Errorf("ListChunkIDsByFile(missing): got %v", list2)
	}
}

func TestSQLiteStorage_RebuildIDFAndTfidf(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rebuild.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.StoreFile(File{Path: "a.go", Content: "a", Hash: "1", Size: 1, Mtime: 1, IndexedAt: 1}); err != nil {
		t.Fatal(err)
	}
	chunks := []Chunk{
		{Content: "get user", Type: "text", StartLine: 1, EndLine: 1, TokenCount: 2, Magnitude: 0},
		{Content: "user id", Type: "text", StartLine: 2, EndLine: 2, TokenCount: 2, Magnitude: 0},
	}
	ids, err := s.StoreChunks("a.go", chunks)
	if err != nil || len(ids) != 2 {
		t.Fatalf("StoreChunks: %v", err)
	}
	_ = s.StoreChunkVectors(ids[0], []VectorRow{
		{Term: "get", TF: 0.5, TFIDF: 0, RawFreq: 1},
		{Term: "user", TF: 0.5, TFIDF: 0, RawFreq: 1},
	})
	_ = s.StoreChunkVectors(ids[1], []VectorRow{
		{Term: "user", TF: 0.5, TFIDF: 0, RawFreq: 1},
		{Term: "id", TF: 0.5, TFIDF: 0, RawFreq: 1},
	})
	if err := s.RebuildIDFAndTfidf(); err != nil {
		t.Fatalf("RebuildIDFAndTfidf: %v", err)
	}
	N := 2
	n := float64(N)
	if n < 1 {
		n = 1
	}
	idfGet := math.Log((n+1)/float64(1+1)) + 1
	idfUser := math.Log((n+1)/float64(2+1)) + 1
	idfID := math.Log((n+1)/float64(1+1)) + 1
	wantIDF := map[string]float64{"get": idfGet, "user": idfUser, "id": idfID}
	tfidfGet := 0.5 * idfGet
	tfidfUser := 0.5 * idfUser
	tfidfID := 0.5 * idfID
	mag0 := math.Sqrt(tfidfGet*tfidfGet + tfidfUser*tfidfUser)
	mag1 := math.Sqrt(tfidfUser*tfidfUser + tfidfID*tfidfID)
	wantByChunkID := map[int64]SearchCandidate{
		ids[0]: {
			ChunkID: ids[0], FilePath: "a.go", Content: "get user", StartLine: 1, EndLine: 1,
			TokenCount: 2, Magnitude: mag0,
			Terms: map[string]VectorRow{
				"get":  {Term: "get", TF: 0.5, TFIDF: tfidfGet, RawFreq: 1},
				"user": {Term: "user", TF: 0.5, TFIDF: tfidfUser, RawFreq: 1},
			},
		},
		ids[1]: {
			ChunkID: ids[1], FilePath: "a.go", Content: "user id", StartLine: 2, EndLine: 2,
			TokenCount: 2, Magnitude: mag1,
			Terms: map[string]VectorRow{
				"user": {Term: "user", TF: 0.5, TFIDF: tfidfUser, RawFreq: 1},
				"id":   {Term: "id", TF: 0.5, TFIDF: tfidfID, RawFreq: 1},
			},
		},
	}
	gotIDF, gotCandidates, err := s.SearchCandidates([]string{"get", "user", "id"})
	if err != nil {
		t.Fatalf("SearchCandidates: %v", err)
	}
	if len(gotIDF) != len(wantIDF) {
		t.Fatalf("idf: got %v want %v", gotIDF, wantIDF)
	}
	for term, w := range wantIDF {
		if g, ok := gotIDF[term]; !ok || math.Abs(g-w) > 1e-9 {
			t.Errorf("idf[%q]: got %v want %v", term, gotIDF[term], w)
		}
	}
	if len(gotCandidates) != len(wantByChunkID) {
		t.Fatalf("candidates: got %d want %d", len(gotCandidates), len(wantByChunkID))
	}
	for _, g := range gotCandidates {
		w, ok := wantByChunkID[g.ChunkID]
		if !ok {
			t.Errorf("unexpected chunk id %d", g.ChunkID)
			continue
		}
		if w.FilePath != g.FilePath || w.Content != g.Content ||
			w.StartLine != g.StartLine || w.EndLine != g.EndLine || w.TokenCount != g.TokenCount ||
			math.Abs(w.Magnitude-g.Magnitude) > 1e-9 {
			t.Errorf("candidate ChunkID=%d: got %+v want %+v", g.ChunkID, g, w)
		}
		for term, wr := range w.Terms {
			gr, ok := g.Terms[term]
			if !ok || math.Abs(gr.TFIDF-wr.TFIDF) > 1e-9 || math.Abs(gr.TF-wr.TF) > 1e-9 || gr.RawFreq != wr.RawFreq {
				t.Errorf("candidate ChunkID=%d Terms[%q]: got %+v want %+v", g.ChunkID, term, gr, wr)
			}
		}
	}
}
