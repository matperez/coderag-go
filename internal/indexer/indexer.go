package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/matperez/coderag-go/internal/astchunk"
	"github.com/matperez/coderag-go/internal/chunk"
	"github.com/matperez/coderag-go/internal/scan"
	"github.com/matperez/coderag-go/internal/storage"
	"github.com/matperez/coderag-go/internal/tokenizer"
	"github.com/matperez/coderag-go/internal/watcher"
)

const defaultMaxChunkSize = 1000

// Indexer indexes a codebase into storage.
type Indexer struct {
	storage  storage.Storage
	tok      *tokenizer.Tokenizer
	root     string
	maxSize  int64
	maxChunk int
	watch    bool
	status   IndexStatus
}

// IndexStatus is the current indexing status.
type IndexStatus struct {
	IsIndexing     bool
	Progress       int    // 0-100 when indexing
	TotalFiles     int
	ProcessedFiles int
	TotalChunks    int
	IndexedChunks  int
	CurrentFile    string // file being indexed (empty when idle)
}

// Config for the indexer.
type Config struct {
	Storage      storage.Storage
	Root         string
	MaxFileSize  int64
	MaxChunkSize int
	Watch        bool // if true, after indexing run watcher until context is cancelled
}

// New creates an indexer.
func New(cfg Config) *Indexer {
	if cfg.MaxChunkSize <= 0 {
		cfg.MaxChunkSize = defaultMaxChunkSize
	}
	return &Indexer{
		storage:  cfg.Storage,
		tok:      tokenizer.New(),
		root:     cfg.Root,
		maxSize:  cfg.MaxFileSize,
		maxChunk: cfg.MaxChunkSize,
		watch:    cfg.Watch,
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
	})
	if err != nil {
		return err
	}
	x.status.TotalFiles = len(entries)
	slog.Info("indexing started", "total_files", len(entries))
	// Pass 1: read files, store files, chunk and collect term freqs
	total := len(entries)
	type chunkData struct {
		content   string
		chunkType string
		startLine int
		endLine   int
		termFreq  map[string]int
	}
	fileChunks := make(map[string][]chunkData)
	processed := 0
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
		hash := sha256Hex(content)
		err = x.storage.StoreFile(storage.File{
			Path: path, Content: string(content), Hash: hash,
			Size: e.Size, Mtime: e.Mtime, IndexedAt: nowMillis(),
		})
		if err != nil {
			slog.Warn("skip file", "path", path, "error", err)
			continue
		}
		processed++
		x.status.ProcessedFiles = processed
		if x.status.TotalFiles > 0 {
			x.status.Progress = processed * 100 / x.status.TotalFiles
		}
		if processed%10 == 0 || processed == total {
			slog.Info("indexing progress", "processed", processed, "total", total, "progress_pct", x.status.Progress, "current_file", path)
		}
		ext := filepath.Ext(path)
		var chunks []chunk.Chunk
		if astChunks, ok := astchunk.ChunkByAST(ctx, string(content), ext, x.maxChunk); ok {
			chunks = astChunks
		} else {
			chunks = chunk.ChunkByCharacters(string(content), x.maxChunk)
		}
		for _, c := range chunks {
			tokens := x.tok.Tokenize(c.Content)
			freq := make(map[string]int)
			for _, t := range tokens {
				freq[t]++
			}
			fileChunks[path] = append(fileChunks[path], chunkData{
				content: c.Content, chunkType: c.Type,
				startLine: c.StartLine, endLine: c.EndLine, termFreq: freq,
			})
		}
	}
	// Compute global IDF from all chunk term freqs
	var allFreqs []map[string]int
	for _, chunks := range fileChunks {
		for _, cd := range chunks {
			allFreqs = append(allFreqs, cd.termFreq)
		}
	}
	N := float64(len(allFreqs))
	if N < 1 {
		N = 1
	}
	docFreq := make(map[string]int)
	for _, freq := range allFreqs {
		for term := range freq {
			docFreq[term]++
		}
	}
	idf := make(map[string]float64)
	for term, df := range docFreq {
		idf[term] = math.Log((N+1)/float64(df+1)) + 1
	}
	// Pass 2: store chunks with magnitude/tokenCount and vectors
	for path, chunks := range fileChunks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		stChunks := make([]storage.Chunk, len(chunks))
		for i, cd := range chunks {
			totalTf := 0.0
			for _, c := range cd.termFreq {
				totalTf += float64(c)
			}
			tf := make(map[string]float64)
			if totalTf > 0 {
				for t, c := range cd.termFreq {
					tf[t] = float64(c) / totalTf
				}
			}
			tfidf := make(map[string]float64)
			mag := 0.0
			tokenCount := 0
			for t, c := range cd.termFreq {
				tfidf[t] = tf[t] * idf[t]
				mag += tfidf[t] * tfidf[t]
				tokenCount += c
			}
			mag = math.Sqrt(mag)
			stChunks[i] = storage.Chunk{
				Content: cd.content, Type: cd.chunkType,
				StartLine: cd.startLine, EndLine: cd.endLine,
				TokenCount: tokenCount, Magnitude: mag,
			}
		}
		chunkIDs, err := x.storage.StoreChunks(path, stChunks)
		if err != nil {
			continue
		}
		for i, cid := range chunkIDs {
			cd := chunks[i]
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
	}
	x.status.Progress = 100
	fc, _ := x.storage.FileCount()
	cc, _ := x.storage.ChunkCount()
	x.status.IndexedChunks = cc
	x.status.ProcessedFiles = fc
	x.status.CurrentFile = ""
	slog.Info("indexing done", "processed_files", x.status.ProcessedFiles, "indexed_chunks", x.status.IndexedChunks)

	if x.watch {
		w := watcher.New(watcher.Options{
			Root:         x.root,
			UseGitignore: true,
			Debounce:     150 * time.Millisecond,
		})
		if err := w.Start(); err != nil {
			return err
		}
		defer w.Close()
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
	return nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func nowMillis() int64 { return time.Now().UnixMilli() }
