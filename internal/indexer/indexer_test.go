package indexer

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/matperez/coderag-go/internal/datadir"
	"github.com/matperez/coderag-go/internal/search"
	"github.com/matperez/coderag-go/internal/storage"
)

func TestIndexer_Index_e2e(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc Foo() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nfunc Bar() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	dataDir, err := datadir.DataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dataDir, "index.db")
	st, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	idx := New(Config{Storage: st, Root: dir})
	if err := idx.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	n, _ := st.FileCount()
	if n != 2 {
		t.Errorf("FileCount = %d", n)
	}
	cn, _ := st.ChunkCount()
	if cn < 2 {
		t.Errorf("ChunkCount = %d", cn)
	}
	// Search via storage
	idf, candidates, err := st.SearchCandidates([]string{"foo", "bar"})
	if err != nil {
		t.Fatal(err)
	}
	var sc []search.StorageCandidate
	avgLen := 0.0
	for _, c := range candidates {
		terms := make(map[string]search.TermScore)
		for k, v := range c.Terms {
			terms[k] = search.TermScore{TF: v.TF, TFIDF: v.TFIDF, RawFreq: v.RawFreq}
		}
		sc = append(sc, search.StorageCandidate{
			FilePath: c.FilePath, Content: c.Content, StartLine: c.StartLine, EndLine: c.EndLine,
			TokenCount: c.TokenCount, Terms: terms,
		})
		avgLen += float64(c.TokenCount)
	}
	if len(candidates) > 0 {
		avgLen /= float64(len(candidates))
	}
	results := search.SearchFromStorage("foo", idf, sc, avgLen, 10)
	if len(results) == 0 {
		t.Error("expected search result for 'foo'")
	}
}

func TestIndexer_GetStatus_afterIndex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.go"), []byte("package p\n"), 0644)
	dataDir, _ := datadir.DataDir(dir)
	st, _ := storage.NewSQLiteStorage(filepath.Join(dataDir, "index.db"))
	defer st.Close()
	idx := New(Config{Storage: st, Root: dir})
	_ = idx.Index(context.Background())
	s := idx.GetStatus()
	if s.IsIndexing {
		t.Error("expected not indexing after Index()")
	}
	if s.ProcessedFiles != 1 || s.IndexedChunks < 1 {
		t.Errorf("GetStatus after index: ProcessedFiles=%d IndexedChunks=%d", s.ProcessedFiles, s.IndexedChunks)
	}
}

func TestIndexer_GetStatus_duringIndex(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dir, "f"+strconv.Itoa(i)+".go"), []byte("package p\n"), 0644)
	}
	dataDir, _ := datadir.DataDir(dir)
	st, _ := storage.NewSQLiteStorage(filepath.Join(dataDir, "index.db"))
	defer st.Close()
	idx := New(Config{Storage: st, Root: dir})
	done := make(chan struct{})
	go func() {
		_ = idx.Index(context.Background())
		close(done)
	}()
	var sawProgress bool
	for i := 0; i < 50; i++ {
		s := idx.GetStatus()
		if s.IsIndexing && s.Progress > 0 {
			sawProgress = true
			break
		}
		select {
		case <-done:
			break
		default:
			// small yield
		}
	}
	<-done
	s := idx.GetStatus()
	if s.IsIndexing {
		t.Error("still indexing after done")
	}
	if s.ProcessedFiles != 5 {
		t.Errorf("ProcessedFiles = %d", s.ProcessedFiles)
	}
	if !sawProgress {
		t.Log("note: did not observe progress during indexing (timing)")
	}
}
