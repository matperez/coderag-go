package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/matperez/coderag-go/internal/datadir"
	"github.com/matperez/coderag-go/internal/storage"
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
