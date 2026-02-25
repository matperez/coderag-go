package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// SQLiteStorage implements Storage using SQLite.
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage opens the database at dbPath, runs migrations, and returns the storage.
// The parent directory of dbPath must exist.
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := RunMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &SQLiteStorage{db: db}, nil
}

// Close closes the database connection.
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// StoreFile inserts or replaces a file by path.
func (s *SQLiteStorage) StoreFile(file File) error {
	_, err := s.db.Exec(`
		INSERT INTO files (path, content, hash, size, mtime, language, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			content = excluded.content,
			hash = excluded.hash,
			size = excluded.size,
			mtime = excluded.mtime,
			language = excluded.language,
			indexed_at = excluded.indexed_at
	`,
		file.Path, file.Content, file.Hash, file.Size, file.Mtime, file.Language, file.IndexedAt,
	)
	return err
}

// StoreChunks inserts chunks for the file at filePath (file must already be stored).
func (s *SQLiteStorage) StoreChunks(filePath string, chunks []Chunk) error {
	var fileID int64
	err := s.db.QueryRow("SELECT id FROM files WHERE path = ?", filePath).Scan(&fileID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("file not found: %s", filePath)
	}
	if err != nil {
		return err
	}
	// Delete existing chunks for this file so we replace atomically
	_, err = s.db.Exec("DELETE FROM chunks WHERE file_id = ?", fileID)
	if err != nil {
		return err
	}
	for _, c := range chunks {
		_, err = s.db.Exec(`
			INSERT INTO chunks (file_id, content, type, start_line, end_line, metadata)
			VALUES (?, ?, ?, ?, ?, ?)
		`, fileID, c.Content, c.Type, c.StartLine, c.EndLine, nullString(c.Metadata))
		if err != nil {
			return err
		}
	}
	return nil
}

// GetFile returns the file by path, or nil if not found.
func (s *SQLiteStorage) GetFile(path string) (*File, error) {
	var f File
	var lang sql.NullString
	err := s.db.QueryRow(`
		SELECT path, content, hash, size, mtime, language, indexed_at
		FROM files WHERE path = ?
	`, path).Scan(&f.Path, &f.Content, &f.Hash, &f.Size, &f.Mtime, &lang, &f.IndexedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lang.Valid {
		f.Language = lang.String
	}
	return &f, nil
}

// ListFiles returns all file paths in the index.
func (s *SQLiteStorage) ListFiles() ([]string, error) {
	rows, err := s.db.Query("SELECT path FROM files ORDER BY path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// FileCount returns the number of indexed files.
func (s *SQLiteStorage) FileCount() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&n)
	return n, err
}

// ChunkCount returns the total number of chunks.
func (s *SQLiteStorage) ChunkCount() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&n)
	return n, err
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
