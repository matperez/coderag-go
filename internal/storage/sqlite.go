package storage

import (
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"

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
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set synchronous: %w", err)
	}
	if _, err := db.Exec("PRAGMA cache_size=-64000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set cache_size: %w", err)
	}
	if _, err := db.Exec("PRAGMA temp_store=MEMORY"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set temp_store: %w", err)
	}
	slog.Info("database open", "db", dbPath)
	if err := RunMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	slog.Info("migrations applied", "db", dbPath)
	return &SQLiteStorage{db: db}, nil
}

// Close closes the database connection.
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// RunInTransaction runs fn in a single transaction. On error the transaction is rolled back.
func (s *SQLiteStorage) RunInTransaction(fn func(Storage) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	txSt := &txStorage{tx: tx}
	if err := fn(txSt); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// txStorage implements Storage using a *sql.Tx for use inside RunInTransaction.
type txStorage struct {
	tx *sql.Tx
}

func (s *txStorage) StoreFile(file File) error {
	_, err := s.tx.Exec(`
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

func (s *txStorage) StoreChunks(filePath string, chunks []Chunk) ([]int64, error) {
	var fileID int64
	err := s.tx.QueryRow("SELECT id FROM files WHERE path = ?", filePath).Scan(&fileID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}
	if err != nil {
		return nil, err
	}
	_, err = s.tx.Exec("DELETE FROM chunks WHERE file_id = ?", fileID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(chunks))
	for _, c := range chunks {
		res, err := s.tx.Exec(`
			INSERT INTO chunks (file_id, content, type, start_line, end_line, metadata, token_count, magnitude)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, fileID, c.Content, c.Type, c.StartLine, c.EndLine, nullString(c.Metadata), c.TokenCount, c.Magnitude)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *txStorage) StoreChunkVectors(chunkID int64, rows []VectorRow) error {
	_, err := s.tx.Exec("DELETE FROM document_vectors WHERE chunk_id = ?", chunkID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		_, err = s.tx.Exec(`
			INSERT INTO document_vectors (chunk_id, term, tf, tfidf, raw_freq) VALUES (?, ?, ?, ?, ?)
		`, chunkID, r.Term, r.TF, r.TFIDF, r.RawFreq)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *txStorage) GetFile(path string) (*File, error) {
	var f File
	var lang sql.NullString
	err := s.tx.QueryRow(`
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

func (s *txStorage) ListFiles() ([]string, error) {
	rows, err := s.tx.Query("SELECT path FROM files ORDER BY path")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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

func (s *txStorage) FileCount() (int, error) {
	var n int
	err := s.tx.QueryRow("SELECT COUNT(*) FROM files").Scan(&n)
	return n, err
}

func (s *txStorage) ChunkCount() (int, error) {
	var n int
	err := s.tx.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&n)
	return n, err
}

func (s *txStorage) DeleteFile(path string) error {
	var fileID int64
	err := s.tx.QueryRow("SELECT id FROM files WHERE path = ?", path).Scan(&fileID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.tx.Exec("DELETE FROM document_vectors WHERE chunk_id IN (SELECT id FROM chunks WHERE file_id = ?)", fileID)
	if err != nil {
		return err
	}
	_, err = s.tx.Exec("DELETE FROM chunks WHERE file_id = ?", fileID)
	if err != nil {
		return err
	}
	_, err = s.tx.Exec("DELETE FROM files WHERE id = ?", fileID)
	return err
}

func (s *txStorage) DocFreqs(terms []string) (map[string]int, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(terms))
	args := make([]interface{}, len(terms))
	for i, t := range terms {
		placeholders[i] = "?"
		args[i] = t
	}
	rows, err := s.tx.Query(
		"SELECT term, COUNT(DISTINCT chunk_id) FROM document_vectors WHERE term IN ("+
			strings.Join(placeholders, ",")+") GROUP BY term",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	df := make(map[string]int)
	for _, t := range terms {
		df[t] = 0
	}
	for rows.Next() {
		var term string
		var count int
		if err := rows.Scan(&term, &count); err != nil {
			return nil, err
		}
		df[term] = count
	}
	return df, rows.Err()
}

func (s *txStorage) SearchCandidates(terms []string) (map[string]float64, []SearchCandidate, error) {
	if len(terms) == 0 {
		return nil, nil, nil
	}
	var N int
	if err := s.tx.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&N); err != nil {
		return nil, nil, err
	}
	n := float64(N)
	if n < 1 {
		n = 1
	}
	placeholders := make([]string, len(terms))
	args := make([]interface{}, len(terms))
	for i, t := range terms {
		placeholders[i] = "?"
		args[i] = t
	}
	rows, err := s.tx.Query(
		"SELECT term, COUNT(DISTINCT chunk_id) FROM document_vectors WHERE term IN ("+
			strings.Join(placeholders, ",")+") GROUP BY term",
		args...,
	)
	if err != nil {
		return nil, nil, err
	}
	df := make(map[string]int)
	for rows.Next() {
		var term string
		var count int
		if err := rows.Scan(&term, &count); err != nil {
			_ = rows.Close()
			return nil, nil, err
		}
		df[term] = count
	}
	_ = rows.Close()
	idf := make(map[string]float64)
	for _, term := range terms {
		d := df[term]
		idf[term] = math.Log((n+1)/float64(d+1)) + 1
	}
	qArgs := make([]interface{}, len(terms))
	for i, t := range terms {
		qArgs[i] = t
	}
	rows2, err := s.tx.Query(
		"SELECT DISTINCT chunk_id FROM document_vectors WHERE term IN ("+
			strings.Join(placeholders, ",")+")",
		qArgs...,
	)
	if err != nil {
		return nil, nil, err
	}
	var chunkIDs []int64
	for rows2.Next() {
		var id int64
		if err := rows2.Scan(&id); err != nil {
			_ = rows2.Close()
			return nil, nil, err
		}
		chunkIDs = append(chunkIDs, id)
	}
	_ = rows2.Close()
	if len(chunkIDs) == 0 {
		return idf, nil, nil
	}
	candidates := make([]SearchCandidate, 0, len(chunkIDs))
	for _, cid := range chunkIDs {
		var path, content string
		var startLine, endLine, tokenCount int
		var magnitude float64
		err := s.tx.QueryRow(`
			SELECT f.path, c.content, c.start_line, c.end_line, c.token_count, c.magnitude FROM chunks c
			JOIN files f ON f.id = c.file_id WHERE c.id = ?
		`, cid).Scan(&path, &content, &startLine, &endLine, &tokenCount, &magnitude)
		if err != nil {
			return nil, nil, err
		}
		vecRows, err := s.tx.Query(
			"SELECT term, tf, tfidf, raw_freq FROM document_vectors WHERE chunk_id = ? AND term IN ("+
				strings.Join(placeholders, ",")+")",
			append([]interface{}{cid}, qArgs...)...,
		)
		if err != nil {
			return nil, nil, err
		}
		termsMap := make(map[string]VectorRow)
		for vecRows.Next() {
			var r VectorRow
			if err := vecRows.Scan(&r.Term, &r.TF, &r.TFIDF, &r.RawFreq); err != nil {
				_ = vecRows.Close()
				return nil, nil, err
			}
			termsMap[r.Term] = r
		}
		_ = vecRows.Close()
		candidates = append(candidates, SearchCandidate{
			ChunkID: cid, FilePath: path, Content: content, StartLine: startLine, EndLine: endLine,
			TokenCount: tokenCount, Magnitude: magnitude, Terms: termsMap,
		})
	}
	return idf, candidates, nil
}

func (s *txStorage) GetChunk(chunkID int64) (*ChunkInfo, error) {
	var c ChunkInfo
	err := s.tx.QueryRow(`
		SELECT f.path, c.content, c.start_line, c.end_line FROM chunks c
		JOIN files f ON f.id = c.file_id WHERE c.id = ?
	`, chunkID).Scan(&c.Path, &c.Content, &c.StartLine, &c.EndLine)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *txStorage) ListChunkIDsByFile(path string) ([]int64, error) {
	rows, err := s.tx.Query(
		"SELECT c.id FROM chunks c JOIN files f ON f.id = c.file_id WHERE f.path = ?",
		path,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *txStorage) RebuildIDFAndTfidf() error {
	N, err := s.ChunkCount()
	if err != nil {
		return err
	}
	n := float64(N)
	if n < 1 {
		n = 1
	}
	rows, err := s.tx.Query("SELECT term, COUNT(DISTINCT chunk_id) FROM document_vectors GROUP BY term")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	type termDF struct {
		term string
		df   int
	}
	var terms []termDF
	for rows.Next() {
		var t termDF
		if err := rows.Scan(&t.term, &t.df); err != nil {
			return err
		}
		terms = append(terms, t)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, t := range terms {
		idf := math.Log((n+1)/float64(t.df+1)) + 1
		_, err := s.tx.Exec("UPDATE document_vectors SET tfidf = tf * ? WHERE term = ?", idf, t.term)
		if err != nil {
			return err
		}
	}
	_, err = s.tx.Exec(`
		UPDATE chunks SET magnitude = (
			SELECT COALESCE(SQRT(SUM(tfidf * tfidf)), 0) FROM document_vectors WHERE chunk_id = chunks.id
		)
	`)
	return err
}

func (s *txStorage) RunInTransaction(fn func(Storage) error) error {
	return fmt.Errorf("nested transaction not supported")
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
// Returns the inserted chunk IDs in order.
func (s *SQLiteStorage) StoreChunks(filePath string, chunks []Chunk) ([]int64, error) {
	var fileID int64
	err := s.db.QueryRow("SELECT id FROM files WHERE path = ?", filePath).Scan(&fileID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec("DELETE FROM chunks WHERE file_id = ?", fileID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(chunks))
	for _, c := range chunks {
		tokenCount := c.TokenCount
		magnitude := c.Magnitude
		res, err := s.db.Exec(`
			INSERT INTO chunks (file_id, content, type, start_line, end_line, metadata, token_count, magnitude)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, fileID, c.Content, c.Type, c.StartLine, c.EndLine, nullString(c.Metadata), tokenCount, magnitude)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}
	return ids, nil
}

// StoreChunkVectors inserts TF-IDF vector rows for a chunk. Replaces any existing vectors for that chunk.
func (s *SQLiteStorage) StoreChunkVectors(chunkID int64, rows []VectorRow) error {
	_, err := s.db.Exec("DELETE FROM document_vectors WHERE chunk_id = ?", chunkID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		_, err = s.db.Exec(`
			INSERT INTO document_vectors (chunk_id, term, tf, tfidf, raw_freq) VALUES (?, ?, ?, ?, ?)
		`, chunkID, r.Term, r.TF, r.TFIDF, r.RawFreq)
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
	defer func() { _ = rows.Close() }()
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

// DeleteFile removes the file and all its chunks and vectors.
func (s *SQLiteStorage) DeleteFile(path string) error {
	var fileID int64
	err := s.db.QueryRow("SELECT id FROM files WHERE path = ?", path).Scan(&fileID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM document_vectors WHERE chunk_id IN (SELECT id FROM chunks WHERE file_id = ?)", fileID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM chunks WHERE file_id = ?", fileID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM files WHERE id = ?", fileID)
	return err
}

// DocFreqs returns the number of distinct chunks containing each term.
func (s *SQLiteStorage) DocFreqs(terms []string) (map[string]int, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(terms))
	args := make([]interface{}, len(terms))
	for i, t := range terms {
		placeholders[i] = "?"
		args[i] = t
	}
	rows, err := s.db.Query(
		"SELECT term, COUNT(DISTINCT chunk_id) FROM document_vectors WHERE term IN ("+
			strings.Join(placeholders, ",")+") GROUP BY term",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	df := make(map[string]int)
	for _, t := range terms {
		df[t] = 0
	}
	for rows.Next() {
		var term string
		var count int
		if err := rows.Scan(&term, &count); err != nil {
			return nil, err
		}
		df[term] = count
	}
	return df, rows.Err()
}

// RebuildIDFAndTfidf recomputes IDF from document_vectors, updates tfidf per row, and chunk magnitudes.
func (s *SQLiteStorage) RebuildIDFAndTfidf() error {
	N, err := s.ChunkCount()
	if err != nil {
		return err
	}
	n := float64(N)
	if n < 1 {
		n = 1
	}
	rows, err := s.db.Query("SELECT term, COUNT(DISTINCT chunk_id) FROM document_vectors GROUP BY term")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	type termDF struct {
		term string
		df   int
	}
	var terms []termDF
	for rows.Next() {
		var t termDF
		if err := rows.Scan(&t.term, &t.df); err != nil {
			return err
		}
		terms = append(terms, t)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, t := range terms {
		idf := math.Log((n+1)/float64(t.df+1)) + 1
		_, err := s.db.Exec("UPDATE document_vectors SET tfidf = tf * ? WHERE term = ?", idf, t.term)
		if err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`
		UPDATE chunks SET magnitude = (
			SELECT COALESCE(SQRT(SUM(tfidf * tfidf)), 0) FROM document_vectors WHERE chunk_id = chunks.id
		)
	`)
	return err
}

// SearchCandidates returns IDF for the given terms and chunks that contain any of them.
func (s *SQLiteStorage) SearchCandidates(terms []string) (map[string]float64, []SearchCandidate, error) {
	if len(terms) == 0 {
		return nil, nil, nil
	}
	var N int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&N); err != nil {
		return nil, nil, err
	}
	n := float64(N)
	if n < 1 {
		n = 1
	}
	// Document frequency per term
	placeholders := make([]string, len(terms))
	args := make([]interface{}, len(terms))
	for i, t := range terms {
		placeholders[i] = "?"
		args[i] = t
	}
	rows, err := s.db.Query(
		"SELECT term, COUNT(DISTINCT chunk_id) FROM document_vectors WHERE term IN ("+
			strings.Join(placeholders, ",")+") GROUP BY term",
		args...,
	)
	if err != nil {
		return nil, nil, err
	}
	df := make(map[string]int)
	for rows.Next() {
		var term string
		var count int
		if err := rows.Scan(&term, &count); err != nil {
			_ = rows.Close()
			return nil, nil, err
		}
		df[term] = count
	}
	_ = rows.Close()
	idf := make(map[string]float64)
	for _, term := range terms {
		d := df[term]
		idf[term] = math.Log((n+1)/float64(d+1)) + 1
	}
	// Chunk IDs that have any of the terms
	qArgs := make([]interface{}, len(terms))
	for i, t := range terms {
		qArgs[i] = t
	}
	rows2, err := s.db.Query(
		"SELECT DISTINCT chunk_id FROM document_vectors WHERE term IN ("+
			strings.Join(placeholders, ",")+")",
		qArgs...,
	)
	if err != nil {
		return nil, nil, err
	}
	var chunkIDs []int64
	for rows2.Next() {
		var id int64
		if err := rows2.Scan(&id); err != nil {
			_ = rows2.Close()
			return nil, nil, err
		}
		chunkIDs = append(chunkIDs, id)
	}
	_ = rows2.Close()
	if len(chunkIDs) == 0 {
		return idf, nil, nil
	}
	// Load chunk + file path + content + lines + token_count, magnitude for each chunk ID
	candidates := make([]SearchCandidate, 0, len(chunkIDs))
	for _, cid := range chunkIDs {
		var path, content string
		var startLine, endLine, tokenCount int
		var magnitude float64
		err := s.db.QueryRow(`
			SELECT f.path, c.content, c.start_line, c.end_line, c.token_count, c.magnitude FROM chunks c
			JOIN files f ON f.id = c.file_id WHERE c.id = ?
		`, cid).Scan(&path, &content, &startLine, &endLine, &tokenCount, &magnitude)
		if err != nil {
			return nil, nil, err
		}
		vecRows, err := s.db.Query(
			"SELECT term, tf, tfidf, raw_freq FROM document_vectors WHERE chunk_id = ? AND term IN ("+
				strings.Join(placeholders, ",")+")",
			append([]interface{}{cid}, qArgs...)...,
		)
		if err != nil {
			return nil, nil, err
		}
		termsMap := make(map[string]VectorRow)
		for vecRows.Next() {
			var r VectorRow
			if err := vecRows.Scan(&r.Term, &r.TF, &r.TFIDF, &r.RawFreq); err != nil {
				_ = vecRows.Close()
				return nil, nil, err
			}
			termsMap[r.Term] = r
		}
		_ = vecRows.Close()
		candidates = append(candidates, SearchCandidate{
			ChunkID: cid, FilePath: path, Content: content, StartLine: startLine, EndLine: endLine,
			TokenCount: tokenCount, Magnitude: magnitude, Terms: termsMap,
		})
	}
	return idf, candidates, nil
}

// GetChunk returns chunk path, content, and line range by chunk ID.
func (s *SQLiteStorage) GetChunk(chunkID int64) (*ChunkInfo, error) {
	var c ChunkInfo
	err := s.db.QueryRow(`
		SELECT f.path, c.content, c.start_line, c.end_line FROM chunks c
		JOIN files f ON f.id = c.file_id WHERE c.id = ?
	`, chunkID).Scan(&c.Path, &c.Content, &c.StartLine, &c.EndLine)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListChunkIDsByFile returns all chunk IDs for the given file path.
func (s *SQLiteStorage) ListChunkIDsByFile(path string) ([]int64, error) {
	rows, err := s.db.Query(
		"SELECT c.id FROM chunks c JOIN files f ON f.id = c.file_id WHERE f.path = ?",
		path,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
