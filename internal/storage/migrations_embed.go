package storage

// Migration SQL inlined so we don't require embed for minimal setup.
// 001_schema.up.sql content:
const migration001Up = `
CREATE TABLE IF NOT EXISTS files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL UNIQUE,
    content TEXT NOT NULL,
    hash TEXT NOT NULL,
    size INTEGER NOT NULL,
    mtime INTEGER NOT NULL,
    language TEXT,
    indexed_at INTEGER NOT NULL,
    magnitude REAL DEFAULT 0,
    token_count INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS files_path_idx ON files(path);
CREATE INDEX IF NOT EXISTS files_hash_idx ON files(hash);

CREATE TABLE IF NOT EXISTS chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    type TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    metadata TEXT,
    token_count INTEGER DEFAULT 0,
    magnitude REAL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS chunks_file_id_idx ON chunks(file_id);
CREATE INDEX IF NOT EXISTS chunks_type_idx ON chunks(type);

CREATE TABLE IF NOT EXISTS document_vectors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chunk_id INTEGER NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    term TEXT NOT NULL,
    tf REAL NOT NULL,
    tfidf REAL NOT NULL,
    raw_freq INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS vectors_chunk_id_idx ON document_vectors(chunk_id);
CREATE INDEX IF NOT EXISTS vectors_term_idx ON document_vectors(term);
CREATE INDEX IF NOT EXISTS vectors_tfidf_idx ON document_vectors(tfidf);
CREATE INDEX IF NOT EXISTS vectors_term_chunk_idx ON document_vectors(term, chunk_id);
`
