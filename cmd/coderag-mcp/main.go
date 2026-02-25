// Command coderag-mcp runs the CodeRAG MCP server for codebase search.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/matperez/coderag-go/internal/datadir"
	"github.com/matperez/coderag-go/internal/indexer"
	"github.com/matperez/coderag-go/internal/storage"
)

func main() {
	root := flag.String("root", ".", "project root to index/search")
	indexOnly := flag.Bool("index-only", false, "run indexing and exit")
	maxSize := flag.Int64("max-size", 0, "max file size in bytes (0 = no limit)")
	flag.Parse()

	rootPath, err := filepath.Abs(*root)
	if err != nil {
		log.Fatalf("root: %v", err)
	}
	dataDir, err := datadir.DataDir(rootPath)
	if err != nil {
		log.Fatalf("datadir: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("mkdir datadir: %v", err)
	}
	dbPath := filepath.Join(dataDir, "index.db")
	st, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer st.Close()

	idx := indexer.New(indexer.Config{
		Storage:     st,
		Root:        rootPath,
		MaxFileSize: *maxSize,
	})

	if *indexOnly {
		if err := idx.Index(context.Background()); err != nil {
			log.Fatalf("index: %v", err)
		}
		s := idx.GetStatus()
		log.Printf("indexed %d files, %d chunks", s.ProcessedFiles, s.IndexedChunks)
		return
	}

	// TODO: start MCP server (phase 8.2)
	log.Fatal("MCP server not implemented yet; use --index-only to index")
}
