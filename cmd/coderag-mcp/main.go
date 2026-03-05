// Command coderag-mcp runs the CodeRAG MCP server for codebase search.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/matperez/coderag-go/internal/datadir"
	"github.com/matperez/coderag-go/internal/embeddings"
	"github.com/matperez/coderag-go/internal/indexer"
	"github.com/matperez/coderag-go/internal/metadata"
	"github.com/matperez/coderag-go/internal/search"
	"github.com/matperez/coderag-go/internal/storage"
	"github.com/matperez/coderag-go/internal/tokenizer"
	"github.com/matperez/coderag-go/internal/vectorstore"
)

const requestTimeout = 30 * time.Second

func main() {
	logLevel := flag.String("log-level", "", "log level: debug, info, warn, error (default from CODERAG_LOG or info)")
	root := flag.String("root", ".", "project root to index/search")
	indexOnly := flag.Bool("index-only", false, "run indexing and exit")
	maxSize := flag.Int64("max-size", 0, "max file size in bytes (0 = no limit)")
	pprofAddr := flag.String("pprof", "", "enable pprof HTTP server at address, e.g. :6060 (or set CODERAG_PPROF)")
	flag.Parse()

	level := parseLogLevel(*logLevel)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	addr := *pprofAddr
	if addr == "" {
		addr = os.Getenv("CODERAG_PPROF")
	}
	if addr != "" {
		go func() {
			slog.Info("pprof server listening", "addr", addr)
			if err := http.ListenAndServe(addr, nil); err != nil {
				slog.Error("pprof server failed", "error", err)
			}
		}()
	}

	rootPath, err := filepath.Abs(*root)
	if err != nil {
		slog.Error("root resolution failed", "error", err)
		os.Exit(1)
	}
	dataDir, err := datadir.DataDir(rootPath)
	if err != nil {
		slog.Error("datadir resolution failed", "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		slog.Error("mkdir datadir failed", "error", err)
		os.Exit(1)
	}
	if err := metadata.Ensure(dataDir, rootPath); err != nil {
		slog.Warn("metadata write failed", "error", err)
	}
	slog.Info("starting", "root", rootPath, "data_dir", dataDir)

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
			slog.Warn("embeddings disabled", "error", err)
			embedder = nil
		} else {
			dim := len(dimVec)
			vs, err := vectorstore.Open(context.Background(), dataDir, dim)
			if err != nil {
				slog.Warn("vector store disabled", "error", err)
			} else {
				vecStore = vs
				defer func() { _ = vs.Close() }()
			}
		}
	}

	dbPath := filepath.Join(dataDir, "index.db")
	st, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		slog.Error("storage open failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()

	idx := indexer.New(indexer.Config{
		Storage:           st,
		Root:              rootPath,
		DataDir:           dataDir,
		MaxFileSize:       *maxSize,
		IndexingBatchSize: envInt("INDEXING_BATCH_SIZE"), // 0 = default 50
		Embedder:          embedder,
		VecStore:          vecStore,
	})

	if *indexOnly {
		if err := idx.Index(context.Background()); err != nil {
			slog.Error("index failed", "error", err)
			os.Exit(1)
		}
		s := idx.GetStatus()
		slog.Info("indexed", "files", s.ProcessedFiles, "chunks", s.IndexedChunks)
		return
	}

	var hybridOpts *search.HybridOpts
	if embedder != nil && vecStore != nil {
		hybridOpts = &search.HybridOpts{
			VecStore:   vecStore,
			Embedder:   embedder,
			BM25Weight: 0.5,
		}
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "coderag-go", Version: "0.1.0"}, nil)
	registerCodebaseSearch(server, st, rootPath, hybridOpts)
	registerCodebaseIndexStatus(server, idx)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		slog.Error("MCP server failed", "error", err)
		os.Exit(1)
	}
}

func parseLogLevel(flagVal string) slog.Level {
	s := flagVal
	if s == "" {
		s = os.Getenv("CODERAG_LOG")
	}
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// flexInt unmarshals from JSON number or string so MCP clients that send limit as string are accepted.
type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return err
		}
		*f = flexInt(n)
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*f = flexInt(n)
	return nil
}

type codebaseSearchArgs struct {
	Query          string    `json:"query" jsonschema:"Search query"`
	Limit          *flexInt  `json:"limit,omitempty" jsonschema:"Max number of results (default 10)"`
	FileExtensions *[]string `json:"file_extensions,omitempty" jsonschema:"Filter by extensions e.g. .go,.js"`
	PathFilter     *string   `json:"path_filter,omitempty" jsonschema:"Include only paths matching this substring"`
	ExcludePaths   *[]string `json:"exclude_paths,omitempty" jsonschema:"Exclude paths containing any of these"`
	IncludeContent bool      `json:"include_content,omitempty" jsonschema:"Include snippet content in results"`
}

func codebaseSearchInputSchema() *jsonschema.Schema {
	s, err := jsonschema.For[codebaseSearchArgs](nil)
	if err != nil {
		panic("codebase_search input schema: " + err.Error())
	}
	if s.Properties != nil {
		if limitProp := s.Properties["limit"]; limitProp != nil {
			// Allow integer or string so clients (e.g. Cursor) that send limit as string pass validation.
			limitProp.Types = []string{"integer", "string"}
			limitProp.Type = ""
		}
	}
	return s
}

func registerCodebaseSearch(s *mcp.Server, st storage.Storage, root string, hybridOpts *search.HybridOpts) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "codebase_search",
		Description: "Search the codebase by keywords and phrases using BM25 (and optional vector search). Best for: exact terms, method/type/API names, proto and contract names; path_filter and file_extensions help narrow to a service or language. Returns file paths and optional snippets. Prefer when you need matches to specific identifiers or when filtering by path/extension.",
		InputSchema: codebaseSearchInputSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args codebaseSearchArgs) (*mcp.CallToolResult, any, error) {
		limit := 10
		if args.Limit != nil && *args.Limit > 0 {
			limit = int(*args.Limit)
		}
		slog.Info("tool call", "tool", "codebase_search", "query", truncate(args.Query, 80), "limit", limit)
		tokens := tokenizer.Tokenize(args.Query)
		if len(tokens) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "No search terms."}},
			}, nil, nil
		}
		idf, candidates, err := st.SearchCandidates(tokens)
		if err != nil {
			slog.Error("search failed", "err", err)
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil, nil
		}
		if len(candidates) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "No matches."}},
			}, nil, nil
		}
		avgLen := 0.0
		sc := make([]search.StorageCandidate, 0, len(candidates))
		for _, c := range candidates {
			if !matchFilters(c.FilePath, ptrSlice(args.FileExtensions), ptrStr(args.PathFilter), ptrSlice(args.ExcludePaths)) {
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
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "No matches after filters."}},
			}, nil, nil
		}
		avgLen /= float64(len(sc))
		var results []search.Result
		if hybridOpts != nil {
			var err error
			results, err = search.HybridFromStorage(ctx, args.Query, st, idf, sc, avgLen, limit, hybridOpts)
			if err != nil {
				slog.Error("search failed", "err", err)
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil, nil
			}
		} else {
			results = search.SearchFromStorage(args.Query, idf, sc, avgLen, limit)
		}
		slog.Debug("search done", "results", len(results))
		md := formatSearchResultsMarkdown(results, args.IncludeContent, root)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: md}},
		}, nil, nil
	})
}

func registerCodebaseIndexStatus(s *mcp.Server, idx *indexer.Indexer) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "codebase_index_status",
		Description: "Return current indexing status: is_indexing, progress (0-100), file and chunk counts.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		slog.Debug("tool call", "tool", "codebase_index_status")
		st := idx.GetStatus()
		text := fmt.Sprintf("is_indexing: %v\nprogress: %d\nfiles: %d\nchunks: %d\n",
			st.IsIndexing, st.Progress, st.ProcessedFiles, st.IndexedChunks)
		if st.CurrentFile != "" {
			text += fmt.Sprintf("current_file: %s\n", st.CurrentFile)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// envInt returns the integer value of the environment variable key, or 0 if unset or invalid.
func envInt(key string) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return 0
	}
	return v
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func ptrSlice(s *[]string) []string {
	if s == nil {
		return nil
	}
	return *s
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

func formatSearchResultsMarkdown(results []search.Result, includeContent bool, root string) string {
	var b strings.Builder
	for i, r := range results {
		path := strings.TrimPrefix(r.URI, "file://")
		if root != "" && !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		fmt.Fprintf(&b, "### %d. %s", i+1, path)
		if r.StartLine > 0 || r.EndLine > 0 {
			fmt.Fprintf(&b, " (L%d-L%d)", r.StartLine, r.EndLine)
		}
		b.WriteString("\n\n")
		if includeContent && r.Content != "" {
			b.WriteString("```\n")
			b.WriteString(strings.TrimSpace(r.Content))
			b.WriteString("\n```\n\n")
		}
	}
	return b.String()
}
