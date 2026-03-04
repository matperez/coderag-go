// Command coderag-cli is a console client for querying an existing CodeRAG index (no indexing).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

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
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "coderag-cli",
	Short: "Console client for querying an existing CodeRAG index",
	Long:  "Query an existing index (no indexing). Index must be built with coderag-mcp --root <path> --index-only.",
}

func init() {
	rootCmd.PersistentFlags().String("root", ".", "project root (index is under ~/.coderag-go/projects/<hash> by root)")
	rootCmd.PersistentFlags().Bool("json", false, "output JSON")
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().IntP("limit", "n", 10, "max results")
	searchCmd.Flags().Bool("include-content", false, "include snippet content in output")
	searchCmd.Flags().String("ext", "", "comma-separated extensions e.g. .go,.js")
	searchCmd.Flags().String("path-filter", "", "include only paths containing this substring")
	searchCmd.Flags().String("exclude", "", "comma-separated substrings to exclude from paths")
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show index stats (files, chunks)",
	RunE:  runStatusCmd,
}

func runStatusCmd(cmd *cobra.Command, _ []string) error {
	root, _ := cmd.Root().PersistentFlags().GetString("root")
	jsonOut, _ := cmd.Root().PersistentFlags().GetBool("json")
	runStatus(root, jsonOut)
	return nil
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search the index",
	Long:  "Search by keywords or natural language. Query is required.",
	Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE:  runSearchCmd,
}

func runSearchCmd(cmd *cobra.Command, args []string) error {
	root, _ := cmd.Root().PersistentFlags().GetString("root")
	jsonOut, _ := cmd.Root().PersistentFlags().GetBool("json")
	limit, _ := cmd.Flags().GetInt("limit")
	includeContent, _ := cmd.Flags().GetBool("include-content")
	ext, _ := cmd.Flags().GetString("ext")
	pathFilter, _ := cmd.Flags().GetString("path-filter")
	exclude, _ := cmd.Flags().GetString("exclude")
	query := args[0]
	runSearch(root, jsonOut, query, limit, includeContent, ext, pathFilter, exclude)
	return nil
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
	defer func() { _ = st.Close() }()

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

func runSearch(root string, jsonOut bool, query string, limit int, includeContent bool, ext, pathFilter, exclude string) {
	st, dataDir, err := openStorage(root)
	if err != nil {
		slog.Error("search failed", "error", err)
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()

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

	extSlice := splitComma(ext)
	excludeSlice := splitComma(exclude)
	avgLen := 0.0
	sc := make([]search.StorageCandidate, 0, len(candidates))
	for _, c := range candidates {
		if !matchFilters(c.FilePath, extSlice, pathFilter, excludeSlice) {
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
		results, err = search.HybridFromStorage(ctx, query, st, idf, sc, avgLen, limit, hybridOpts)
		if err != nil {
			slog.Error("search failed", "error", err)
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	} else {
		results = search.SearchFromStorage(query, idf, sc, avgLen, limit)
	}

	if jsonOut {
		writeSearchResultsJSON(results, includeContent, root)
	} else {
		writeSearchResultsMarkdown(results, includeContent, root)
	}
}

type searchResultJSON struct {
	URI          string   `json:"uri"`
	Path         string   `json:"path"`
	Score        float64  `json:"score"`
	StartLine    int      `json:"start_line"`
	EndLine      int      `json:"end_line"`
	Content      string   `json:"content,omitempty"`
	MatchedTerms []string `json:"matched_terms"`
}

func writeSearchResultsJSON(results []search.Result, includeContent bool, root string) {
	out := make([]searchResultJSON, 0, len(results))
	for _, r := range results {
		path := strings.TrimPrefix(r.URI, "file://")
		if root != "" && !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		item := searchResultJSON{
			URI:          r.URI,
			Path:         path,
			Score:        r.Score,
			StartLine:    r.StartLine,
			EndLine:      r.EndLine,
			MatchedTerms: r.MatchedTerms,
		}
		if includeContent {
			item.Content = r.Content
		}
		out = append(out, item)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(map[string]any{"results": out}); err != nil {
		slog.Error("json encode failed", "error", err)
		os.Exit(1)
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
