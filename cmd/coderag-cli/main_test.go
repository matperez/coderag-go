package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matperez/coderag-go/internal/datadir"
	"github.com/matperez/coderag-go/internal/storage"
)

func TestCLI_statusAndSearch(t *testing.T) {
	// Create a minimal index in a temp "project" dir.
	projectDir := t.TempDir()
	absProject, err := filepath.Abs(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	dataDir, err := datadir.DataDir(absProject)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dataDir, "index.db")
	st, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	f := storage.File{Path: "hello.go", Content: "package main\nfunc Hello() {}", Hash: "h1", Size: 1, Mtime: 1, IndexedAt: 1}
	if err := st.StoreFile(f); err != nil {
		t.Fatal(err)
	}
	ids, err := st.StoreChunks("hello.go", []storage.Chunk{
		{Content: "func Hello() {}", Type: "function", StartLine: 1, EndLine: 2, TokenCount: 3, Magnitude: 0},
	})
	if err != nil || len(ids) != 1 {
		t.Fatalf("StoreChunks: %v", err)
	}
	if err := st.StoreChunkVectors(ids[0], []storage.VectorRow{
		{Term: "hello", TF: 0.5, TFIDF: 1.0, RawFreq: 1},
		{Term: "func", TF: 0.5, TFIDF: 1.0, RawFreq: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.RebuildIDFAndTfidf(); err != nil {
		t.Fatal(err)
	}
	st.Close()

	// Build CLI (from module root).
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "coderag-cli")
	build := exec.Command("go", "build", "-o", bin, "./cmd/coderag-cli")
	build.Dir = findModuleRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}

	// status: human output
	cmd := exec.Command(bin, "--root", projectDir, "status")
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "files: 1") || !strings.Contains(string(out), "chunks: 1") {
		t.Errorf("status output missing files/chunks: %s", out)
	}

	// status --json (stdout only; stderr has logs)
	cmd = exec.Command(bin, "--root", projectDir, "--json", "status")
	cmd.Dir = projectDir
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	var statusObj map[string]any
	if err := json.Unmarshal(out, &statusObj); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, out)
	}
	if statusObj["files"].(float64) != 1 || statusObj["chunks"].(float64) != 1 {
		t.Errorf("status JSON: got %v", statusObj)
	}

	// search (human)
	cmd = exec.Command(bin, "--root", projectDir, "search", "hello", "--limit", "1")
	cmd.Dir = projectDir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("search: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "hello.go") {
		t.Errorf("search output missing path: %s", out)
	}

	// search --json (stdout only)
	cmd = exec.Command(bin, "--root", projectDir, "--json", "search", "hello", "--limit", "1")
	cmd.Dir = projectDir
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("search --json: %v\n%s", err, out)
	}
	var searchObj map[string]any
	if err := json.Unmarshal(out, &searchObj); err != nil {
		t.Fatalf("search JSON: %v\n%s", err, out)
	}
	results, ok := searchObj["results"].([]any)
	if !ok || len(results) < 1 {
		t.Errorf("search JSON: got %v", searchObj)
	}
}

func TestCLI_missingIndex(t *testing.T) {
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "coderag-cli")
	build := exec.Command("go", "build", "-o", bin, "./cmd/coderag-cli")
	build.Dir = findModuleRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, "--root", "/nonexistent/dir/for/cli/test", "status")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("expected exit 1 for missing index, got success\n%s", out)
	}
	if !strings.Contains(string(out), "index not found") {
		t.Errorf("stderr should contain 'index not found': %s", out)
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
