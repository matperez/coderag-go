// Command coderag-cli is a console client for querying an existing CodeRAG index (no indexing).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/matperez/coderag-go/internal/datadir"
	"github.com/matperez/coderag-go/internal/embeddings"
	"github.com/matperez/coderag-go/internal/metadata"
	"github.com/matperez/coderag-go/internal/search"
	"github.com/matperez/coderag-go/internal/storage"
	"github.com/matperez/coderag-go/internal/tokenizer"
	"github.com/matperez/coderag-go/internal/vectorstore"
)

const requestTimeout = 30 * time.Second

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
		runSearch(*root, *jsonOut, args[1:])
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

func runSearch(root string, jsonOut bool, args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	limit := fs.Int("limit", 10, "max results")
	includeContent := fs.Bool("include-content", false, "include snippet content")
	ext := fs.String("ext", "", "comma-separated extensions e.g. .go,.js")
	pathFilter := fs.String("path-filter", "", "include only paths containing this substring")
	exclude := fs.String("exclude", "", "comma-separated substrings to exclude from paths")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Fprintf(os.Stderr, "query required\n")
		os.Exit(1)
	}
	query := rest[0]

	st, dataDir, err := openStorage(root)
	if err != nil {
		slog.Error("search failed", "error", err)
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	var embedder embeddings.Provider
	var vecStore vectorstore.Store
	embCfg := embeddings.DefaultOpenAIConfig()
	embeddingsEnabled := embCfg.APIKey != "" || os.Getenv("OPENAI_BASE_URL") != ""
	if embeddingsEnabled {
		embedder = embeddings.NewOpenAIProvider(embCfg)
		dimCtx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		dimVec, err := embedder.GenerateEmbedding(dimCtx, "x")
		cancel()
		if err != nil {
			slog.Debug("embeddings disabled", "error", err)
			embedder = nil
		} else {
			dim := len(dimVec)
			vs, err := vectorstore.Open(context.Background(), dataDir, dim)
			if err != nil {
				slog.Debug("vector store disabled", "error", err)
			} else {
				vecStore = vs
				defer func() { _ = vs.Close() }()
			}
		}
	}

	tokens := tokenizer.Tokenize(query)
	if len(tokens) == 0 {
		fmt.Fprintf(os.Stderr, "No search terms.\n")
		os.Exit(1)
	}
	idf, candidates, err := st.SearchCandidates(tokens)
	if err != nil {
		slog.Error("search failed", "error", err)
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if len(candidates) == 0 {
		if !jsonOut {
			fmt.Println("No matches.")
		}
		return
	}

	extSlice := splitComma(*ext)
	excludeSlice := splitComma(*exclude)
	avgLen := 0.0
	sc := make([]search.StorageCandidate, 0, len(candidates))
	for _, c := range candidates {
		if !matchFilters(c.FilePath, extSlice, *pathFilter, excludeSlice) {
			continue
		}
		terms := make(map[string]search.TermScore)
		for k, v := range c.Terms {
			terms[k] = search.TermScore{TF: v.TF, TFIDF: v.TFIDF, RawFreq: v.RawFreq}
		}
		sc = append(sc, search.StorageCandidate{
			ChunkID: c.ChunkID, FilePath: c.FilePath, Content: c.Content, StartLine: c.StartLine, EndLine: c.EndLine,
			TokenCount: c.TokenCount, Terms: terms,
		})
		avgLen += float64(c.TokenCount)
	}
	if len(sc) == 0 {
		if !jsonOut {
			fmt.Println("No matches after filters.")
		}
		return
	}
	avgLen /= float64(len(sc))

	var results []search.Result
	var hybridOpts *search.HybridOpts
	if embedder != nil && vecStore != nil {
		hybridOpts = &search.HybridOpts{VecStore: vecStore, Embedder: embedder, BM25Weight: 0.5}
	}
	ctx := context.Background()
	if hybridOpts != nil {
		var err error
		results, err = search.HybridFromStorage(ctx, query, st, idf, sc, avgLen, *limit, hybridOpts)
		if err != nil {
			slog.Error("search failed", "error", err)
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	} else {
		results = search.SearchFromStorage(query, idf, sc, avgLen, *limit)
	}

	if jsonOut {
		// Task 4: output JSON
	} else {
		writeSearchResultsMarkdown(results, *includeContent, root)
	}
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func matchFilters(path string, ext []string, pathFilter string, exclude []string) bool {
	if pathFilter != "" && !strings.Contains(path, pathFilter) {
		return false
	}
	for _, e := range exclude {
		if strings.Contains(path, e) {
			return false
		}
	}
	if len(ext) == 0 {
		return true
	}
	for _, e := range ext {
		e = strings.TrimSpace(e)
		if e != "" && (strings.HasSuffix(path, e) || (len(e) > 0 && e[0] != '.' && strings.HasSuffix(path, "."+e))) {
			return true
		}
	}
	return false
}

func writeSearchResultsMarkdown(results []search.Result, includeContent bool, root string) {
	for i, r := range results {
		path := strings.TrimPrefix(r.URI, "file://")
		if root != "" && !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		fmt.Printf("### %d. %s", i+1, path)
		if r.StartLine > 0 || r.EndLine > 0 {
			fmt.Printf(" (L%d-L%d)", r.StartLine, r.EndLine)
		}
		fmt.Println()
		fmt.Println()
		if includeContent && r.Content != "" {
			fmt.Println("```")
			fmt.Println(strings.TrimSpace(r.Content))
			fmt.Println("```")
			fmt.Println()
		}
	}
}
