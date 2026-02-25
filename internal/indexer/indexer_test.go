package indexer

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matperez/coderag-go/internal/datadir"
	"github.com/matperez/coderag-go/internal/embeddings"
	"github.com/matperez/coderag-go/internal/search"
	"github.com/matperez/coderag-go/internal/storage"
	"github.com/matperez/coderag-go/internal/vectorstore"
	"github.com/matperez/coderag-go/internal/watcher"
)

// countingStorage wraps a Storage and counts StoreFile, StoreChunks, DeleteFile calls.
type countingStorage struct {
	storage.Storage
	storeFileCalls   int
	storeChunksCalls int
	deleteFileCalls  []string
	mu                sync.Mutex
}

func (c *countingStorage) StoreFile(f storage.File) error {
	c.mu.Lock()
	c.storeFileCalls++
	c.mu.Unlock()
	return c.Storage.StoreFile(f)
}

func (c *countingStorage) StoreChunks(filePath string, chunks []storage.Chunk) ([]int64, error) {
	c.mu.Lock()
	c.storeChunksCalls++
	c.mu.Unlock()
	return c.Storage.StoreChunks(filePath, chunks)
}

func (c *countingStorage) DeleteFile(path string) error {
	c.mu.Lock()
	c.deleteFileCalls = append(c.deleteFileCalls, path)
	c.mu.Unlock()
	return c.Storage.DeleteFile(path)
}

func (c *countingStorage) reset() {
	c.mu.Lock()
	c.storeFileCalls = 0
	c.storeChunksCalls = 0
	c.deleteFileCalls = nil
	c.mu.Unlock()
}

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

func TestIndexer_Index_e2e_astChunking(t *testing.T) {
	dir := t.TempDir()
	// Go file with distinct function name for AST chunking
	goCode := `package main
import "fmt"
func HandleRequest() { fmt.Println("ok") }
func Other() {}
`
	jsCode := `function fetchData() { return 1; }
class Helper { run() {} }
`
	if err := os.WriteFile(filepath.Join(dir, "api.go"), []byte(goCode), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib.js"), []byte(jsCode), 0644); err != nil {
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
	// Search by function name (tokenizer: HandleRequest -> handle, request)
	idf, candidates, err := st.SearchCandidates([]string{"handle", "request"})
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
	if len(sc) > 0 {
		avgLen /= float64(len(sc))
	}
	results := search.SearchFromStorage("handle request", idf, sc, avgLen, 10)
	if len(results) == 0 {
		t.Fatal("expected search result for 'handle request'")
	}
	var found bool
	for _, c := range candidates {
		if c.FilePath == "api.go" && strings.Contains(c.Content, "HandleRequest") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a chunk from api.go containing HandleRequest (AST chunking)")
	}
}

func TestIndexer_ProcessEvent_incrementalUpdate(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "x.go")
	if err := os.WriteFile(fpath, []byte("package p\nfunc Old() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dataDir, err := datadir.DataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := storage.NewSQLiteStorage(filepath.Join(dataDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	idx := New(Config{Storage: st, Root: dir})
	if err := idx.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Change file content
	if err := os.WriteFile(fpath, []byte("package p\nfunc NewContent() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := idx.ProcessEvent(context.Background(), "x.go", watcher.Change); err != nil {
		t.Fatal(err)
	}
	// Search for new content (tokenizer: "NewContent" -> "new", "content")
	idf, candidates, err := st.SearchCandidates([]string{"new", "content"})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected chunk with new/content after ProcessEvent(Change)")
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
	if len(sc) > 0 {
		avgLen /= float64(len(sc))
	}
	results := search.SearchFromStorage("new content", idf, sc, avgLen, 10)
	if len(results) == 0 {
		t.Error("search for 'new content' should return result after incremental update")
	}
	if len(results) > 0 && !strings.Contains(results[0].URI, "x.go") {
		t.Errorf("expected x.go in result: %s", results[0].URI)
	}
}

func TestIndexer_WatchMode(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "w.go")
	if err := os.WriteFile(fpath, []byte("package p\nfunc A() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dataDir, _ := datadir.DataDir(dir)
	st, _ := storage.NewSQLiteStorage(filepath.Join(dataDir, "index.db"))
	defer st.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	idx := New(Config{Storage: st, Root: dir, Watch: true})
	done := make(chan error, 1)
	go func() {
		done <- idx.Index(ctx)
	}()
	// Wait for initial index to complete (poll status)
	for i := 0; i < 50; i++ {
		s := idx.GetStatus()
		if !s.IsIndexing && s.IndexedChunks > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Change file
	if err := os.WriteFile(fpath, []byte("package p\nfunc Watched() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	cancel()
	err := <-done
	if err != nil && err != context.Canceled {
		t.Logf("Index returned: %v", err)
	}
	// Search should see "watched" (tokenized)
	idf, candidates, _ := st.SearchCandidates([]string{"watched"})
	if len(candidates) > 0 {
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
		avgLen /= float64(len(sc))
		results := search.SearchFromStorage("watched", idf, sc, avgLen, 5)
		if len(results) > 0 {
			t.Logf("watch mode: search found %d results", len(results))
		}
	}
}

func TestIndexer_Index_skipUnchanged(t *testing.T) {
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
	realSt, err := storage.NewSQLiteStorage(filepath.Join(dataDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer realSt.Close()
	cs := &countingStorage{Storage: realSt}
	idx := New(Config{Storage: cs, Root: dir})
	if err := idx.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cs.storeFileCalls != 2 || cs.storeChunksCalls != 2 {
		t.Errorf("first run: StoreFile=%d StoreChunks=%d, want 2, 2", cs.storeFileCalls, cs.storeChunksCalls)
	}
	cs.reset()
	if err := idx.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cs.storeFileCalls != 0 || cs.storeChunksCalls != 0 {
		t.Errorf("second run (unchanged): StoreFile=%d StoreChunks=%d, want 0, 0", cs.storeFileCalls, cs.storeChunksCalls)
	}
	fc, _ := realSt.FileCount()
	cc, _ := realSt.ChunkCount()
	if fc != 2 || cc < 2 {
		t.Errorf("after second run: FileCount=%d ChunkCount=%d", fc, cc)
	}
}

func TestIndexer_Index_deletedFilesRemoved(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.go")
	bPath := filepath.Join(dir, "b.go")
	if err := os.WriteFile(aPath, []byte("package main\nfunc Foo() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("package main\nfunc Bar() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dataDir, err := datadir.DataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	realSt, err := storage.NewSQLiteStorage(filepath.Join(dataDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer realSt.Close()
	cs := &countingStorage{Storage: realSt}
	idx := New(Config{Storage: cs, Root: dir})
	if err := idx.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(bPath); err != nil {
		t.Fatal(err)
	}
	cs.reset()
	if err := idx.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	var deletedB bool
	for _, p := range cs.deleteFileCalls {
		if p == "b.go" {
			deletedB = true
			break
		}
	}
	if !deletedB {
		t.Errorf("DeleteFile not called for b.go; deleteFileCalls=%v", cs.deleteFileCalls)
	}
	fc, _ := realSt.FileCount()
	if fc != 1 {
		t.Errorf("FileCount=%d, want 1 after removing b.go", fc)
	}
}

func TestIndexer_Index_skipUnchanged_disabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package p\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dataDir, _ := datadir.DataDir(dir)
	realSt, _ := storage.NewSQLiteStorage(filepath.Join(dataDir, "index.db"))
	defer realSt.Close()
	cs := &countingStorage{Storage: realSt}
	skipFalse := false
	idx := New(Config{Storage: cs, Root: dir, SkipUnchanged: &skipFalse})
	if err := idx.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	cs.reset()
	if err := idx.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cs.storeFileCalls != 1 || cs.storeChunksCalls != 1 {
		t.Errorf("with SkipUnchanged=false second run: StoreFile=%d StoreChunks=%d, want 1, 1", cs.storeFileCalls, cs.storeChunksCalls)
	}
}

// recordingVecStore records chunk IDs passed to DeleteChunk for tests.
type recordingVecStore struct {
	vectorstore.MockStore
	deletedIDs []int64
	mu         sync.Mutex
}

func (r *recordingVecStore) DeleteChunk(ctx context.Context, chunkID int64) error {
	r.mu.Lock()
	r.deletedIDs = append(r.deletedIDs, chunkID)
	r.mu.Unlock()
	return r.MockStore.DeleteChunk(ctx, chunkID)
}

func TestIndexer_Index_embeddingsUpsert(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc Foo() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dataDir, err := datadir.DataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	realSt, err := storage.NewSQLiteStorage(filepath.Join(dataDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer realSt.Close()
	embedder := &embeddings.MockProvider{Dimension: 8}
	vs := &vectorstore.MockStore{}
	idx := New(Config{Storage: realSt, Root: dir, Embedder: embedder, VecStore: vs})
	if err := idx.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(vs.Rows) < 1 {
		t.Errorf("expected at least 1 row in vector store, got %d", len(vs.Rows))
	}
	for i, row := range vs.Rows {
		if row.ID <= 0 {
			t.Errorf("row %d: ID=%d, want positive", i, row.ID)
		}
		if len(row.Vector) != 8 {
			t.Errorf("row %d: vector len=%d, want 8", i, len(row.Vector))
		}
	}
}

func TestIndexer_Index_deletedFilesRemoved_vecStore(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.go")
	bPath := filepath.Join(dir, "b.go")
	if err := os.WriteFile(aPath, []byte("package main\nfunc Foo() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("package main\nfunc Bar() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dataDir, err := datadir.DataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	realSt, err := storage.NewSQLiteStorage(filepath.Join(dataDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer realSt.Close()
	vs := &recordingVecStore{}
	embedder := &embeddings.MockProvider{Dimension: 8}
	idx := New(Config{Storage: realSt, Root: dir, Embedder: embedder, VecStore: vs})
	if err := idx.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	bChunkIDs, err := realSt.ListChunkIDsByFile("b.go")
	if err != nil || len(bChunkIDs) == 0 {
		t.Fatalf("expected chunk IDs for b.go: err=%v len=%d", err, len(bChunkIDs))
	}
	if err := os.Remove(bPath); err != nil {
		t.Fatal(err)
	}
	vs.deletedIDs = nil
	if err := idx.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, wantID := range bChunkIDs {
		var found bool
		for _, got := range vs.deletedIDs {
			if got == wantID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DeleteChunk not called for b.go chunk ID %d; deletedIDs=%v", wantID, vs.deletedIDs)
		}
	}
}

const benchmarkFileCount = 200

func BenchmarkIndexer_Index(b *testing.B) {
	baseDir := b.TempDir()
	rootDir := filepath.Join(baseDir, "src")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		b.Fatal(err)
	}
	content := []byte("package p\nfunc F() {}\n")
	for i := 0; i < benchmarkFileCount; i++ {
		p := filepath.Join(rootDir, "f"+strconv.Itoa(i)+".go")
		if err := os.WriteFile(p, content, 0644); err != nil {
			b.Fatal(err)
		}
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dbPath := filepath.Join(baseDir, "db", strconv.Itoa(i), "index.db")
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			b.Fatal(err)
		}
		st, err := storage.NewSQLiteStorage(dbPath)
		if err != nil {
			b.Fatal(err)
		}
		idx := New(Config{Storage: st, Root: rootDir})
		if err := idx.Index(ctx); err != nil {
			st.Close()
			b.Fatal(err)
		}
		st.Close()
	}
}
