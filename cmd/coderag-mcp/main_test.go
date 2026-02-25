package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matperez/coderag-go/internal/datadir"
	"github.com/matperez/coderag-go/internal/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestIndexOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc Foo() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "coderag-mcp")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, "-index-only", "-root", dir)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	dataDir, err := datadir.DataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dataDir, "index.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("index.db not created: %v", err)
	}
	st, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n, err := st.FileCount()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("FileCount = %d, want 1", n)
	}
}

func TestMCPCodebaseSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\nfunc HelloWorld() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "coderag-mcp-e2e")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	// Index first
	indexCmd := exec.Command(bin, "-index-only", "-root", dir)
	indexCmd.Dir = dir
	if out, err := indexCmd.CombinedOutput(); err != nil {
		t.Fatalf("index: %v\n%s", err, out)
	}
	// Start MCP server and call codebase_search
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	transport := &mcp.CommandTransport{Command: exec.Command(bin, "-root", dir)}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "codebase_search",
		Arguments: map[string]any{
			"query": "HelloWorld", "limit": 5, "include_content": true,
			"file_extensions": []any{}, "path_filter": "", "exclude_paths": []any{},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatal("empty response")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type %T", res.Content[0])
	}
	if !strings.Contains(tc.Text, "hello.go") {
		t.Errorf("response should contain hello.go: %s", tc.Text)
	}
	if !strings.Contains(tc.Text, "HelloWorld") {
		t.Errorf("response should contain HelloWorld: %s", tc.Text)
	}
}

func TestMCPCodebaseIndexStatus(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.go"), []byte("package p\n"), 0644)
	bin := filepath.Join(t.TempDir(), "coderag-mcp-status")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	indexCmd := exec.Command(bin, "-index-only", "-root", dir)
	indexCmd.Dir = dir
	if out, err := indexCmd.CombinedOutput(); err != nil {
		t.Fatalf("index: %v\n%s", err, out)
	}
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	transport := &mcp.CommandTransport{Command: exec.Command(bin, "-root", dir)}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "codebase_index_status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %v", res.Content)
	}
	tc := res.Content[0].(*mcp.TextContent)
	if !strings.Contains(tc.Text, "is_indexing: false") {
		t.Errorf("expected is_indexing: false in %s", tc.Text)
	}
	if !strings.Contains(tc.Text, "files: 1") {
		t.Errorf("expected files: 1 in %s", tc.Text)
	}
}
