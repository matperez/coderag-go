package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/matperez/coderag-go/internal/astchunk"
	"github.com/matperez/coderag-go/internal/chunk"
	"github.com/matperez/coderag-go/internal/embeddings"
	"github.com/matperez/coderag-go/internal/scan"
	"github.com/matperez/coderag-go/internal/storage"
	"github.com/matperez/coderag-go/internal/tokenizer"
	"github.com/matperez/coderag-go/internal/vectorstore"
	"github.com/matperez/coderag-go/internal/watcher"
)

const defaultMaxChunkSize = 1000

const defaultIndexingBatchSize = 500

// contentSafeForEmbedding returns false for binary or invalid-UTF-8 content so we don't send it to the embedding API.
func contentSafeForEmbedding(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	if strings.Contains(s, "\x00") {
		return false
	}
	printable := 0
	for _, r := range s {
		if unicode.IsPrint(r) || unicode.IsSpace(r) {
			printable++
		}
	}
	runes := utf8.RuneCountInString(s)
	if runes == 0 {
		return true
	}
	return float64(printable)/float64(runes) >= 0.85
}

// Indexer indexes a codebase into storage.
type Indexer struct {
	storage           storage.Storage
	tok               *tokenizer.Tokenizer
	root              string
	maxSize           int64
	maxChunk          int
	indexingBatchSize int
	watch             bool
	skipUnchanged     bool // if true, skip re-indexing files with same hash (and mtime within 1s)
	embedder          embeddings.Provider
	vecStore          vectorstore.Store
	status            IndexStatus
}

// IndexStatus is the current indexing status.
type IndexStatus struct {
	IsIndexing     bool
	Progress       int // 0-100 when indexing
	TotalFiles     int
	ProcessedFiles int
	TotalChunks    int
	IndexedChunks  int
	CurrentFile    string // file being indexed (empty when idle)
}

// Config for the indexer.
type Config struct {
	Storage           storage.Storage
	Root              string
	MaxFileSize       int64
	MaxChunkSize      int
	IndexingBatchSize int                 // files per batch (0 = default 500). Memory bounded by batch size.
	Watch             bool                // if true, after indexing run watcher until context is cancelled
	SkipUnchanged     *bool               // if true or nil, skip re-indexing when hash (and mtime) unchanged; set to false to force full re-index
	Embedder          embeddings.Provider // optional: when set with VecStore, index chunk embeddings
	VecStore          vectorstore.Store   // optional: when set with Embedder, write embeddings here
}

// New creates an indexer.
func New(cfg Config) *Indexer {
	if cfg.MaxChunkSize <= 0 {
		cfg.MaxChunkSize = defaultMaxChunkSize
	}
	batchSize := cfg.IndexingBatchSize
	if batchSize <= 0 {
		batchSize = defaultIndexingBatchSize
	}
	skipUnchanged := cfg.SkipUnchanged == nil || *cfg.SkipUnchanged
	return &Indexer{
		storage:           cfg.Storage,
		tok:               tokenizer.New(),
		root:              cfg.Root,
		maxSize:           cfg.MaxFileSize,
		maxChunk:          cfg.MaxChunkSize,
		indexingBatchSize: batchSize,
		watch:             cfg.Watch,
		skipUnchanged:     skipUnchanged,
		embedder:          cfg.Embedder,
		vecStore:          cfg.VecStore,
	}
}

// GetStatus returns the current index status (progress when indexing, counts from storage when idle).
func (x *Indexer) GetStatus() IndexStatus {
	s := x.status
	if !s.IsIndexing {
		fc, _ := x.storage.FileCount()
		cc, _ := x.storage.ChunkCount()
		s.IndexedChunks = cc
		s.ProcessedFiles = fc
		s.TotalFiles = fc
		s.TotalChunks = cc
	}
	return s
}

// ProcessEvent applies a watcher event (add/change/remove) to the index.
func (x *Indexer) ProcessEvent(ctx context.Context, relPath string, op watcher.Op) error {
	slog.Info("file event", "path", relPath, "op", op)
	if op == watcher.Remove {
		if x.vecStore != nil {
			chunkIDs, _ := x.storage.ListChunkIDsByFile(relPath)
			for _, cid := range chunkIDs {
				_ = x.vecStore.DeleteChunk(ctx, cid)
			}
		}
		err := x.storage.DeleteFile(relPath)
		if err != nil {
			slog.Warn("event failed", "path", relPath, "op", op, "error", err)
		}
		return err
	}
	// Add or Change
	fullPath := filepath.Join(x.root, relPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil
	}
	hash := sha256Hex(content)
	existing, _ := x.storage.GetFile(relPath)
	if existing != nil && existing.Hash == hash {
		return nil
	}
	info, _ := os.Stat(fullPath)
	mtime := int64(0)
	size := int64(len(content))
	if info != nil {
		mtime = info.ModTime().UnixMilli()
		size = info.Size()
	}
	if err := x.storage.StoreFile(storage.File{
		Path: relPath, Content: string(content), Hash: hash,
		Size: size, Mtime: mtime, IndexedAt: nowMillis(),
	}); err != nil {
		slog.Warn("event failed", "path", relPath, "op", op, "error", err)
		return err
	}
	ext := filepath.Ext(relPath)
	var chunks []chunk.Chunk
	if astChunks, ok := astchunk.ChunkByAST(ctx, string(content), ext, x.maxChunk); ok {
		chunks = astChunks
	} else {
		chunks = chunk.ChunkByCharacters(string(content), x.maxChunk)
	}
	type chunkData struct {
		content   string
		chunkType string
		startLine int
		endLine   int
		termFreq  map[string]int
	}
	var chunkDatas []chunkData
	for _, c := range chunks {
		tokens := x.tok.Tokenize(c.Content)
		freq := make(map[string]int)
		for _, t := range tokens {
			freq[t]++
		}
		chunkDatas = append(chunkDatas, chunkData{
			content: c.Content, chunkType: c.Type,
			startLine: c.StartLine, endLine: c.EndLine, termFreq: freq,
		})
	}
	N, _ := x.storage.ChunkCount()
	n := float64(N)
	if n < 1 {
		n = 1
	}
	allTerms := make(map[string]bool)
	for _, cd := range chunkDatas {
		for t := range cd.termFreq {
			allTerms[t] = true
		}
	}
	termList := make([]string, 0, len(allTerms))
	for t := range allTerms {
		termList = append(termList, t)
	}
	df, err := x.storage.DocFreqs(termList)
	if err != nil {
		slog.Warn("event failed", "path", relPath, "op", op, "error", err)
		return err
	}
	idf := make(map[string]float64)
	for _, t := range termList {
		d := df[t]
		idf[t] = math.Log((n+1)/float64(d+1)) + 1
	}
	stChunks := make([]storage.Chunk, len(chunkDatas))
	for i, cd := range chunkDatas {
		totalTf := 0.0
		for _, c := range cd.termFreq {
			totalTf += float64(c)
		}
		tfidf := make(map[string]float64)
		mag := 0.0
		tokenCount := 0
		for t, c := range cd.termFreq {
			tf := float64(c) / totalTf
			if totalTf <= 0 {
				tf = 0
			}
			tfidfVal := tf * idf[t]
			tfidf[t] = tfidfVal
			mag += tfidfVal * tfidfVal
			tokenCount += c
		}
		mag = math.Sqrt(mag)
		stChunks[i] = storage.Chunk{
			Content: cd.content, Type: cd.chunkType,
			StartLine: cd.startLine, EndLine: cd.endLine,
			TokenCount: tokenCount, Magnitude: mag,
		}
	}
	chunkIDs, err := x.storage.StoreChunks(relPath, stChunks)
	if err != nil {
		slog.Warn("event failed", "path", relPath, "op", op, "error", err)
		return err
	}
	for i, cid := range chunkIDs {
		cd := chunkDatas[i]
		totalTf := 0.0
		for _, c := range cd.termFreq {
			totalTf += float64(c)
		}
		var rows []storage.VectorRow
		for term, rawFreq := range cd.termFreq {
			tf := float64(rawFreq) / totalTf
			if totalTf <= 0 {
				tf = 0
			}
			tfidfVal := tf * idf[term]
			rows = append(rows, storage.VectorRow{
				Term: term, TF: tf, TFIDF: tfidfVal, RawFreq: rawFreq,
			})
		}
		_ = x.storage.StoreChunkVectors(cid, rows)
	}
	if x.embedder != nil && x.vecStore != nil {
		contents := make([]string, len(chunkDatas))
		for i, cd := range chunkDatas {
			contents[i] = cd.content
		}
		vecs, err := x.embedder.GenerateEmbeddings(ctx, contents)
		if err != nil {
			slog.Warn("embedding failed", "path", relPath, "error", err)
			return nil
		}
		rows := make([]vectorstore.Row, 0, len(vecs))
		for j, vec := range vecs {
			rows = append(rows, vectorstore.Row{
				ID: chunkIDs[j], Vector: vec, Metadata: "",
			})
		}
		if err := x.vecStore.Upsert(ctx, rows); err != nil {
			slog.Warn("vector store upsert failed", "path", relPath, "error", err)
		}
	}
	return nil
}

// Index scans root, chunks files, and stores them with TF-IDF vectors.
func (x *Indexer) Index(ctx context.Context) error {
	x.status.IsIndexing = true
	x.status.Progress = 0
	defer func() { x.status.IsIndexing = false }()

	entries, err := scan.Scan(x.root, scan.Options{
		MaxFileSize:  x.maxSize,
		UseGitignore: true,
		Extensions:   astchunk.SupportedExtensions(),
	})
	if err != nil {
		return err
	}
	// Remove from index any paths that no longer exist on disk
	currentPaths := make(map[string]struct{})
	for _, e := range entries {
		path, _ := filepath.Rel(x.root, e.Path)
		if path == "" || path == "." {
			path = e.Path
		}
		currentPaths[path] = struct{}{}
	}
	storedPaths, err := x.storage.ListFiles()
	if err != nil {
		return err
	}
	var deleted int
	for _, p := range storedPaths {
		if _, ok := currentPaths[p]; !ok {
			if x.vecStore != nil {
				chunkIDs, _ := x.storage.ListChunkIDsByFile(p)
				for _, cid := range chunkIDs {
					_ = x.vecStore.DeleteChunk(ctx, cid)
				}
			}
			if err := x.storage.DeleteFile(p); err != nil {
				slog.Warn("delete file from index", "path", p, "error", err)
				continue
			}
			deleted++
		}
	}
	if deleted > 0 {
		slog.Info("removed deleted files from index", "count", deleted)
	}
	x.status.TotalFiles = len(entries)
	total := len(entries)
	slog.Info("indexing started", "total_files", total)

	type fileToIndex struct {
		path    string
		content string
		e       scan.FileEntry
	}
	const embedBatch = 20
	var embedIDs []int64
	var embedContents []string
	flushOneEmbedBatch := func() {
		n := embedBatch
		if n > len(embedContents) {
			n = len(embedContents)
		}
		if n == 0 {
			return
		}
		batchIDs := embedIDs[:n]
		batchContents := embedContents[:n]
		vecs, err := x.embedder.GenerateEmbeddings(ctx, batchContents)
		if err != nil {
			slog.Warn("embedding batch failed", "error", err)
			embedIDs = embedIDs[n:]
			embedContents = embedContents[n:]
			return
		}
		rows := make([]vectorstore.Row, 0, len(vecs))
		for j, vec := range vecs {
			rows = append(rows, vectorstore.Row{
				ID: batchIDs[j], Vector: vec, Metadata: "",
			})
		}
		if err := x.vecStore.Upsert(ctx, rows); err != nil {
			slog.Warn("vector store upsert failed", "error", err)
		}
		embedIDs = embedIDs[n:]
		embedContents = embedContents[n:]
	}

	type chunkData struct {
		content   string
		chunkType string
		startLine int
		endLine   int
		termFreq  map[string]int
	}
	processBatch := func(batch []fileToIndex) {
		for _, item := range batch {
			if err := x.storage.StoreFile(storage.File{
				Path: item.path, Content: item.content, Hash: sha256Hex([]byte(item.content)),
				Size: item.e.Size, Mtime: item.e.Mtime, IndexedAt: nowMillis(),
			}); err != nil {
				slog.Warn("skip file", "path", item.path, "error", err)
				continue
			}
			ext := filepath.Ext(item.path)
			var chunks []chunk.Chunk
			if astChunks, ok := astchunk.ChunkByAST(ctx, item.content, ext, x.maxChunk); ok {
				chunks = astChunks
			} else {
				chunks = chunk.ChunkByCharacters(item.content, x.maxChunk)
			}
			var fileChunkDatas []chunkData
			for _, c := range chunks {
				tokens := x.tok.Tokenize(c.Content)
				freq := make(map[string]int)
				for _, t := range tokens {
					freq[t]++
				}
				fileChunkDatas = append(fileChunkDatas, chunkData{
					content: c.Content, chunkType: c.Type,
					startLine: c.StartLine, endLine: c.EndLine, termFreq: freq,
				})
			}
			stChunks := make([]storage.Chunk, len(fileChunkDatas))
			for i, cd := range fileChunkDatas {
				tokenCount := 0
				for _, c := range cd.termFreq {
					tokenCount += c
				}
				stChunks[i] = storage.Chunk{
					Content: cd.content, Type: cd.chunkType,
					StartLine: cd.startLine, EndLine: cd.endLine,
					TokenCount: tokenCount, Magnitude: 0,
				}
			}
			chunkIDs, err := x.storage.StoreChunks(item.path, stChunks)
			if err != nil {
				continue
			}
			for i, cid := range chunkIDs {
				cd := fileChunkDatas[i]
				totalTf := 0.0
				for _, c := range cd.termFreq {
					totalTf += float64(c)
				}
				var rows []storage.VectorRow
				for term, rawFreq := range cd.termFreq {
					tf := float64(rawFreq) / totalTf
					if totalTf <= 0 {
						tf = 0
					}
					rows = append(rows, storage.VectorRow{
						Term: term, TF: tf, TFIDF: 0, RawFreq: rawFreq,
					})
				}
				_ = x.storage.StoreChunkVectors(cid, rows)
			}
			if x.embedder != nil && x.vecStore != nil {
				for i, cid := range chunkIDs {
					if !contentSafeForEmbedding(fileChunkDatas[i].content) {
						continue
					}
					embedIDs = append(embedIDs, cid)
					embedContents = append(embedContents, fileChunkDatas[i].content)
				}
				for len(embedContents) >= embedBatch {
					flushOneEmbedBatch()
				}
			}
		}
	}

	processed := 0
	var batch []fileToIndex
	batchSize := x.indexingBatchSize
	filesIndexedThisRun := false
	batchNum := 0
	for _, e := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		content, err := os.ReadFile(e.Path)
		if err != nil {
			path, _ := filepath.Rel(x.root, e.Path)
			if path == "" || path == "." {
				path = e.Path
			}
			slog.Warn("skip file", "path", path, "error", err)
			continue
		}
		path, _ := filepath.Rel(x.root, e.Path)
		if path == "" || path == "." {
			path = e.Path
		}
		x.status.CurrentFile = path
		processed++
		x.status.ProcessedFiles = processed
		if total > 0 {
			x.status.Progress = processed * 100 / total
		}
		if processed%5000 == 0 || processed == total {
			slog.Info("indexing progress", "processed", processed, "total", total, "progress_pct", x.status.Progress, "current_file", path)
		}
		hash := sha256Hex(content)
		if x.skipUnchanged {
			existing, _ := x.storage.GetFile(path)
			if existing != nil && existing.Hash == hash {
				if existing.Mtime == e.Mtime || abs64(existing.Mtime-e.Mtime) <= 1000 {
					continue
				}
			}
		}
		batch = append(batch, fileToIndex{path: path, content: string(content), e: e})
		if len(batch) >= batchSize {
			filesIndexedThisRun = true
			batchNum++
			slog.Info("processing batch", "batch", batchNum, "files", len(batch))
			processBatch(batch)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		filesIndexedThisRun = true
		batchNum++
		slog.Info("processing batch", "batch", batchNum, "files", len(batch))
		processBatch(batch)
	}

	if !filesIndexedThisRun {
		x.status.Progress = 100
		fc, _ := x.storage.FileCount()
		cc, _ := x.storage.ChunkCount()
		x.status.IndexedChunks = cc
		x.status.ProcessedFiles = fc
		x.status.CurrentFile = ""
		slog.Info("indexing done", "processed_files", x.status.ProcessedFiles, "indexed_chunks", x.status.IndexedChunks)
		if x.watch {
			return x.runWatcher(ctx)
		}
		return nil
	}

	if x.embedder != nil && x.vecStore != nil {
		for len(embedContents) > 0 {
			flushOneEmbedBatch()
		}
		slog.Info("generating and writing embeddings done")
	}
	slog.Info("rebuilding IDF and TF-IDF from storage")
	if err := x.storage.RebuildIDFAndTfidf(); err != nil {
		slog.Error("rebuild IDF and TF-IDF failed", "error", err)
		return err
	}
	x.status.Progress = 100
	fc, _ := x.storage.FileCount()
	cc, _ := x.storage.ChunkCount()
	x.status.IndexedChunks = cc
	x.status.ProcessedFiles = fc
	x.status.CurrentFile = ""
	slog.Info("indexing done", "processed_files", x.status.ProcessedFiles, "indexed_chunks", x.status.IndexedChunks)
	if x.watch {
		return x.runWatcher(ctx)
	}
	return nil
}

func (x *Indexer) runWatcher(ctx context.Context) error {
	w := watcher.New(watcher.Options{
		Root:         x.root,
		UseGitignore: true,
		Extensions:   astchunk.SupportedExtensions(),
		Debounce:     150 * time.Millisecond,
	})
	if err := w.Start(); err != nil {
		return err
	}
	defer func() { _ = w.Close() }()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e, ok := <-w.Events():
			if !ok {
				return nil
			}
			_ = x.ProcessEvent(ctx, e.Path, e.Op)
		}
	}
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func nowMillis() int64 { return time.Now().UnixMilli() }

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
