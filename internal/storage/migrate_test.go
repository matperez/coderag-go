package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Tables must exist: insert and read back
	_, err = db.Exec(
		"INSERT INTO files (path, content, hash, size, mtime, indexed_at) VALUES (?, ?, ?, ?, ?, ?)",
		"foo.go", "package main", "abc", 12, 1000, 2000,
	)
	if err != nil {
		t.Fatalf("insert file: %v", err)
	}
	var path string
	err = db.QueryRow("SELECT path FROM files WHERE id = 1").Scan(&path)
	if err != nil {
		t.Fatalf("select file: %v", err)
	}
	if path != "foo.go" {
		t.Errorf("got path %q", path)
	}

	_, err = db.Exec(
		"INSERT INTO chunks (file_id, content, type, start_line, end_line) VALUES (?, ?, ?, ?, ?)",
		1, "chunk content", "text", 1, 2,
	)
	if err != nil {
		t.Fatalf("insert chunk: %v", err)
	}

	_, err = db.Exec(
		"INSERT INTO document_vectors (chunk_id, term, tf, tfidf, raw_freq) VALUES (?, ?, ?, ?, ?)",
		1, "term", 0.5, 1.2, 2,
	)
	if err != nil {
		t.Fatalf("insert vector: %v", err)
	}
}

func TestRunMigrations_idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test2.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}
}

func TestRunMigrations_createsDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "db.sqlite")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
}
