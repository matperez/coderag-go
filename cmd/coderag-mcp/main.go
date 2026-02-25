// Command coderag-mcp runs the CodeRAG MCP server for codebase search.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/matperez/coderag-go/internal/datadir"
	"github.com/matperez/coderag-go/internal/indexer"
	"github.com/matperez/coderag-go/internal/search"
	"github.com/matperez/coderag-go/internal/storage"
	"github.com/matperez/coderag-go/internal/tokenizer"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	logLevel := flag.String("log-level", "", "log level: debug, info, warn, error (default from CODERAG_LOG or info)")
	root := flag.String("root", ".", "project root to index/search")
	indexOnly := flag.Bool("index-only", false, "run indexing and exit")
	maxSize := flag.Int64("max-size", 0, "max file size in bytes (0 = no limit)")
	flag.Parse()

	level := parseLogLevel(*logLevel)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

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
	slog.Info("starting", "root", rootPath, "data_dir", dataDir)

	dbPath := filepath.Join(dataDir, "index.db")
	st, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		slog.Error("storage open failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	idx := indexer.New(indexer.Config{
		Storage:     st,
		Root:        rootPath,
		MaxFileSize: *maxSize,
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

	server := mcp.NewServer(&mcp.Implementation{Name: "coderag-go", Version: "0.1.0"}, nil)
	registerCodebaseSearch(server, st, rootPath, nil) // optional: pass HybridOpts for BM25+vector
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

type codebaseSearchArgs struct {
	Query          string   `json:"query" jsonschema:"Search query"`
	Limit          *int     `json:"limit" jsonschema:"Max number of results (default 10)"`
	FileExtensions *[]string `json:"file_extensions" jsonschema:"Filter by extensions e.g. .go,.js"`
	PathFilter     *string  `json:"path_filter" jsonschema:"Include only paths matching this substring"`
	ExcludePaths   *[]string `json:"exclude_paths" jsonschema:"Exclude paths containing any of these"`
	IncludeContent bool     `json:"include_content" jsonschema:"Include snippet content in results"`
}

func registerCodebaseSearch(s *mcp.Server, st storage.Storage, root string, hybridOpts *search.HybridOpts) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "codebase_search",
		Description: "Search the codebase by natural language or keywords using BM25 (and optional vector search). Returns file paths and optional snippets.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args codebaseSearchArgs) (*mcp.CallToolResult, any, error) {
		limit := 10
		if args.Limit != nil && *args.Limit > 0 {
			limit = *args.Limit
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

func ptrStr(s *string) string { if s == nil { return "" }; return *s }
func ptrSlice(s *[]string) []string { if s == nil { return nil }; return *s }

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
		b.WriteString(fmt.Sprintf("### %d. %s", i+1, path))
		if r.StartLine > 0 || r.EndLine > 0 {
			b.WriteString(fmt.Sprintf(" (L%d-L%d)", r.StartLine, r.EndLine))
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
