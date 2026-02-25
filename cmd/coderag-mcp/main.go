// Command coderag-mcp runs the CodeRAG MCP server for codebase search.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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

	server := mcp.NewServer(&mcp.Implementation{Name: "coderag-go", Version: "0.1.0"}, nil)
	registerCodebaseSearch(server, st, rootPath)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("MCP server: %v", err)
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

func registerCodebaseSearch(s *mcp.Server, st storage.Storage, root string) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "codebase_search",
		Description: "Search the codebase by natural language or keywords using BM25. Returns file paths and optional snippets.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args codebaseSearchArgs) (*mcp.CallToolResult, any, error) {
		limit := 10
		if args.Limit != nil && *args.Limit > 0 {
			limit = *args.Limit
		}
		tokens := tokenizer.Tokenize(args.Query)
		if len(tokens) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "No search terms."}},
			}, nil, nil
		}
		idf, candidates, err := st.SearchCandidates(tokens)
		if err != nil {
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
				FilePath: c.FilePath, Content: c.Content, StartLine: c.StartLine, EndLine: c.EndLine,
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
		results := search.SearchFromStorage(args.Query, idf, sc, avgLen, limit)
		md := formatSearchResultsMarkdown(results, args.IncludeContent, root)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: md}},
		}, nil, nil
	})
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
