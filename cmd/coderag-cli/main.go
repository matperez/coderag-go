// Command coderag-cli is a console client for querying an existing CodeRAG index (no indexing).
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/matperez/coderag-go/internal/datadir"
	"github.com/matperez/coderag-go/internal/metadata"
	"github.com/matperez/coderag-go/internal/storage"
)

func main() {
	root := flag.String("root", ".", "project root (index is under ~/.coderag-go/projects/<hash> by root)")
	jsonOut := flag.Bool("json", false, "output JSON")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}
	subcmd := args[0]
	switch subcmd {
	case "status":
		runStatus(*root, *jsonOut)
	case "search":
		runSearchStub()
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: coderag-cli [--root <path>] [--json] <command> [args]\n")
	fmt.Fprintf(os.Stderr, "  status   show index stats (files, chunks)\n")
	fmt.Fprintf(os.Stderr, "  search   search the index (requires query)\n")
}

func openStorage(root string) (*storage.SQLiteStorage, string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, "", fmt.Errorf("root resolution: %w", err)
	}
	dataDir, err := datadir.DataDir(absRoot)
	if err != nil {
		return nil, "", fmt.Errorf("datadir: %w", err)
	}
	dbPath := filepath.Join(dataDir, "index.db")
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("index not found for root: %s", root)
		}
		return nil, "", fmt.Errorf("index: %w", err)
	}
	st, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, "", fmt.Errorf("storage: %w", err)
	}
	return st, dataDir, nil
}

func runStatus(root string, jsonOut bool) {
	st, dataDir, err := openStorage(root)
	if err != nil {
		slog.Error("status failed", "error", err)
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	files, err := st.FileCount()
	if err != nil {
		slog.Error("file count failed", "error", err)
		os.Exit(1)
	}
	chunks, err := st.ChunkCount()
	if err != nil {
		slog.Error("chunk count failed", "error", err)
		os.Exit(1)
	}

	meta, _ := metadata.Read(dataDir)
	var lastAccessed, idfRebuild string
	if meta != nil {
		lastAccessed = meta.LastAccessedAt
		idfRebuild = meta.IDFRebuildCompletedAt
	}

	if jsonOut {
		fmt.Printf(`{"files":%d,"chunks":%d`, files, chunks)
		if lastAccessed != "" {
			fmt.Printf(`,"last_accessed_at":%q`, lastAccessed)
		}
		if idfRebuild != "" {
			fmt.Printf(`,"idf_rebuild_completed_at":%q`, idfRebuild)
		}
		fmt.Println("}")
	} else {
		fmt.Printf("files: %d\nchunks: %d\n", files, chunks)
		if lastAccessed != "" {
			fmt.Printf("last_accessed_at: %s\n", lastAccessed)
		}
		if idfRebuild != "" {
			fmt.Printf("idf_rebuild_completed_at: %s\n", idfRebuild)
		}
	}
}

func runSearchStub() {
	fmt.Fprintf(os.Stderr, "query required\n")
	os.Exit(1)
}
