package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/matperez/coderag-go/internal/chunk"
	"github.com/matperez/coderag-go/internal/scan"
	"github.com/matperez/coderag-go/internal/storage"
	"github.com/matperez/coderag-go/internal/tokenizer"
)

const defaultMaxChunkSize = 1000

// Indexer indexes a codebase into storage.
type Indexer struct {
	storage  storage.Storage
	tok      *tokenizer.Tokenizer
	root     string
	maxSize  int64
	maxChunk int
}

// Config for the indexer.
type Config struct {
	Storage    storage.Storage
	Root       string
	MaxFileSize int64
	MaxChunkSize int
}

// New creates an indexer.
func New(cfg Config) *Indexer {
	if cfg.MaxChunkSize <= 0 {
		cfg.MaxChunkSize = defaultMaxChunkSize
	}
	return &Indexer{
		storage:  cfg.Storage,
		tok:     tokenizer.New(),
		root:    cfg.Root,
		maxSize: cfg.MaxFileSize,
		maxChunk: cfg.MaxChunkSize,
	}
}

// Index scans root, chunks files, and stores them with TF-IDF vectors.
func (x *Indexer) Index(ctx context.Context) error {
	entries, err := scan.Scan(x.root, scan.Options{
		MaxFileSize:  x.maxSize,
		UseGitignore: true,
	})
	if err != nil {
		return err
	}
	// Pass 1: read files, store files, chunk and collect term freqs
	type chunkData struct {
		content   string
		chunkType string
		startLine int
		endLine   int
		termFreq  map[string]int
	}
	fileChunks := make(map[string][]chunkData)
	for _, e := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		content, err := os.ReadFile(e.Path)
		if err != nil {
			continue
		}
		path, _ := filepath.Rel(x.root, e.Path)
		if path == "" || path == "." {
			path = e.Path
		}
		hash := sha256Hex(content)
		err = x.storage.StoreFile(storage.File{
			Path: path, Content: string(content), Hash: hash,
			Size: e.Size, Mtime: e.Mtime, IndexedAt: nowMillis(),
		})
		if err != nil {
			continue
		}
		chunks := chunk.ChunkByCharacters(string(content), x.maxChunk)
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
	return nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func nowMillis() int64 { return time.Now().UnixMilli() }
