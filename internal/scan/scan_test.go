package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.go"), []byte("package sub"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.ign\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip.ign"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := Scan(dir, Options{UseGitignore: true, Extensions: []string{".go"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
	// .ign file should be ignored by gitignore
	entriesAll, _ := Scan(dir, Options{UseGitignore: true})
	foundIgn := false
	for _, e := range entriesAll {
		if filepath.Ext(e.Path) == ".ign" {
			foundIgn = true
			break
		}
	}
	if foundIgn {
		t.Error("skip.ign should be ignored by .gitignore")
	}
}

func TestScan_skipDirs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.go"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(dir, "node_modules", "x"), 0755)
	os.WriteFile(filepath.Join(dir, "node_modules", "x", "y.js"), []byte("y"), 0644)

	entries, err := Scan(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("node_modules should be skipped, got %d entries", len(entries))
	}
}

func TestScan_maxFileSize(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "small.go"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "big.go"), make([]byte, 100), 0644)

	entries, err := Scan(dir, Options{MaxFileSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || filepath.Base(entries[0].Path) != "small.go" {
		t.Errorf("expected only small.go, got %v", entries)
	}
}
