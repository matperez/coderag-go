# CodeRAG-Go implementation plan

Step-by-step plan with test runs and a commit after each step.

**Rule:** after each step — `go test ./...`, optionally `golangci-lint run`, then `git add` + `git commit` with a clear message.

---

## Phase 0: Project skeleton

### Step 0.1 — Directory structure and linter
- **Do:** create package layout (e.g. `internal/tokenizer`, `internal/storage`, `internal/chunk`, `internal/search`, `internal/indexer`, `cmd/coderag-mcp`), add `.golangci.yml` (or minimal config), `Makefile` or script with targets `test`, `lint`, `build`.
- **Tests:** `go build ./...`, `golangci-lint run` (linter may be empty until first code).
- **Commit:** `chore: project layout, golangci-lint, make targets`

### Step 0.2 — Data directory and project hash
- **Do:** package `internal/datadir` (or `pkg/datadir`): function `DataDir(root string) string` → `~/.coderag-go/projects/<hash>/`, hash = first 16 characters of SHA-256 of absolute `root` path. Unit test for hash stability and path format.
- **Tests:** `go test ./internal/datadir/...` (or equivalent).
- **Commit:** `feat: data dir resolution ~/.coderag-go/projects/<hash>`

---

## Phase 1: Storage and base index

### Step 1.1 — SQLite schema and migrations
- **Do:** tables `files` (id, path, content, hash, size, mtime, language, indexed_at), `chunks` (id, file_id, content, type, start_line, end_line, metadata, token_count, magnitude), `document_vectors` (id, chunk_id, term, tf, tfidf, raw_freq). Migrations in `internal/storage/migrations` or separate package (up/down SQL). Driver: `modernc.org/sqlite`.
- **Tests:** test applying migrations on a temp DB (after migrate — tables exist, can insert a row).
- **Commit:** `feat: SQLite schema and migrations for files, chunks, vectors`

### Step 1.2 — Storage interface and implementation (files + chunks)
- **Do:** interface `Storage` with methods `StoreFile`, `StoreChunks`, `GetFile`, `ListFiles` (or minimal set from reference). Implementation `SQLiteStorage` in `internal/storage`, open DB using path from datadir. Store file and chunks without vectors.
- **Tests:** unit tests with temp SQLite (StoreFile + StoreChunks, read back).
- **Commit:** `feat: Storage interface and SQLite implementation (files, chunks)`

### Step 1.3 — Character-based chunking
- **Do:** package `internal/chunk`: function `ChunkByCharacters(content string, maxChunkSize int) []Chunk`. Type `Chunk` with Content, StartLine, EndLine, Type (e.g. "text"). No AST, only size-based split (and by lines so as not to cut mid-line — optional).
- **Tests:** table-driven tests (empty string, short text, long text, maxChunkSize boundaries).
- **Commit:** `feat: character-based chunking`

---

## Phase 2: Tokenizer

### Step 2.1 — Tokenizer (Unicode + camelCase)
- **Do:** package `internal/tokenizer`: word boundaries `\p{L}\p{N}_`, split on them; for ASCII segments — `fatih/camelcase`; lowercase; drop tokens shorter than 2 characters. Function `Tokenize(text string) []string`.
- **Tests:** table-driven: identifiers (`getUserById` → get, user, by, id), Russian text (“получить пользователя”), mixed code and comments.
- **Commit:** `feat: code-aware tokenizer (Unicode, camelCase)`

### Step 2.2 — Optional: Russian stemming
- **Do:** option in tokenizer (constructor or flag): for Cyrillic tokens call stemmer (snowballstem/snowball). Can be off by default.
- **Tests:** tests for “пользователя” → “пользователь”, “получить” → “получ” (or whatever the stemmer returns).
- **Commit:** `feat: optional Russian stemming in tokenizer`

---

## Phase 3: Search (BM25 / TF-IDF)

### Step 3.1 — BM25/TF-IDF over in-memory chunks
- **Do:** package `internal/search`: build index (terms per chunk, IDF), function `Search(query string, index Index, limit int) []Result`. Use `internal/tokenizer`. Structures: DocumentVector (uri, terms, magnitude), SearchIndex (documents, idf). BM25 with k1, b parameters.
- **Tests:** multiple documents (chunks), query, check order and presence of expected docs; edge cases (empty query, no matches).
- **Commit:** `feat: BM25 search over in-memory index`

### Step 3.2 — Persist and load vectors in SQLite
- **Do:** in storage: methods to write/read vectors (terms per chunk_id: term, tf, tfidf, raw_freq). During indexing — save vectors after tokenization. Precompute magnitude and store in chunks.
- **Tests:** StoreFile + StoreChunks + write vectors; read vectors for given chunk_id or all.
- **Commit:** `feat: persist TF-IDF vectors in SQLite`

### Step 3.3 — Search via SQLite (low-memory)
- **Do:** search function that works from SQLite: get terms from query, select chunk_id from document_vectors by terms, compute scores, return top-N. Interface compatible with current Search (or single function with in-memory vs DB flag).
- **Tests:** write index to SQLite, search via SQLite mode, compare with expected order.
- **Commit:** `feat: search via SQLite (low-memory mode)`

---

## Phase 4: Indexer (scan + chunks + storage)

### Step 4.1 — File scanning and .gitignore
- **Do:** package `internal/scan` or inside `internal/indexer`: walk directory from root, respect .gitignore (library `ignore` or minimal custom), filter by extensions, maxFileSize. Output list of paths and optionally metadata (mtime, size).
- **Tests:** test directory in testdata with several files and .gitignore; verify excluded files are not in the list.
- **Commit:** `feat: file scanner with gitignore support`

### Step 4.2 — Indexer: scan → chunks → storage
- **Do:** `internal/indexer`: run scanner, read files, character-based chunking, tokenize chunks, save to Storage (files + chunks + vectors). No watch. Methods `Index(ctx, root)` and if needed `GetStatus()`.
- **Tests:** unit with mock Storage; e2e with temp dir and real SQLite — index small testdata, verify files and chunks are written.
- **Commit:** `feat: indexer (scan, chunk, tokenize, store)`

### Step 4.3 — Index status
- **Do:** method `IndexStatus` or similar: whether indexing is running, progress (processed/total files or chunks), total indexed files/chunks. Storage must expose counts.
- **Tests:** during indexing status reflects progress; after — final counts.
- **Commit:** `feat: index status (progress, counts)`

---

## Phase 5: AST chunking (optional after MVP)

### Step 5.1 — tree-sitter integration and language mapping
- **Do:** dependency smacker/go-tree-sitter, map file extension → language (config or function). For chosen language get grammar and parse to AST.
- **Tests:** parse short code fragment (Go/JS) — tree not nil, expected nodes present.
- **Commit:** `feat: tree-sitter integration and language mapping`

### Step 5.2 — Extract chunks at AST boundaries
- **Do:** walk AST, extract nodes by type (function, class, method, etc.), build []Chunk with context (imports). Merge small, split large by maxChunkSize. Fallback to character-based on error or unknown language.
- **Tests:** tests on typical files (Go, JS/TS) — chunk count and types; fallback for .md or invalid code.
- **Commit:** `feat: AST-based chunking with fallback`

### Step 5.3 — Wire AST chunking into indexer
- **Do:** in indexer, for supported extensions call AST chunking instead of character-based. Configurable maxChunkSize.
- **Tests:** e2e: index project with .go/.ts files, search by function name — expected chunk in results.
- **Commit:** `feat: use AST chunking in indexer for supported languages`

---

## Phase 6: Watcher and incremental updates

### Step 6.1 — fsnotify and change queue
- **Do:** subscribe to fsnotify on project root (recursive), filter by .gitignore and extensions. Queue events add/change/remove with deduplication and debounce.
- **Tests:** test with temp dir: create/change/delete file — expected event in queue.
- **Commit:** `feat: file watcher with event queue`

### Step 6.2 — Incremental index updates
- **Do:** on event — re-read file (or remove from index), re-chunk, recompute vectors, update/delete in Storage. Use hash/mtime for “unchanged” skip.
- **Tests:** e2e: index → change file → update → search returns current content.
- **Commit:** `feat: incremental index updates on file change`

### Step 6.3 — Watch mode in indexer
- **Do:** flag `Watch bool` in indexer options; when true, after first index run start watcher and process events in background. Clean shutdown (context cancel / Stop).
- **Tests:** run with watch, change file, verify update; stop without panic.
- **Commit:** `feat: indexer watch mode`

---

## Phase 7: Vector search and embeddings (optional)

### Step 7.1 — Embeddings client (OpenAI-compatible)
- **Do:** package `internal/embeddings`: interface `EmbeddingProvider` (GenerateEmbedding, GenerateEmbeddings batch), implementation via HTTP to OpenAI API. Config from env.
- **Tests:** mock provider; optional integration test with real API (skip without key).
- **Commit:** `feat: embedding provider (OpenAI-compatible)`

### Step 7.2 — LanceDB and vector storage
- **Do:** create table in LanceDB at path from datadir, write vectors (id, vector, metadata). k-NN search by query (query embedding).
- **Tests:** write several vectors, search — expected id returned.
- **Commit:** `feat: LanceDB vector storage`

### Step 7.3 — Hybrid search
- **Do:** combine BM25 and vector results (fusion by weights). Option in indexer/search: when EmbeddingProvider is set, index embeddings and call hybrid in search.
- **Tests:** mock embeddings, verify results include both keyword and semantic matches.
- **Commit:** `feat: hybrid search (BM25 + vector)`

---

## Phase 8: CLI and MCP server

### Step 8.1 — CLI: flags and initialization
- **Do:** `cmd/coderag-mcp`: parse `--root`, `--index-only`, `--max-size`, etc. Create DataDir, Storage, Indexer. With `--index-only` — run indexing and exit.
- **Tests:** run with `--index-only` on testdata, verify index is created (e.g. DB file exists or status call).
- **Commit:** `feat: CLI (--root, --index-only)`

### Step 8.2 — MCP server and codebase_search tool
- **Do:** use modelcontextprotocol/go-sdk, stdio transport. Register tool `codebase_search` (query, limit, file_extensions, path_filter, exclude_paths, include_content). Call search on index, format response (markdown with paths and snippets).
- **Tests:** e2e: start server, send JSON-RPC request to call tool, parse response and check fields/results (e.g. subprocess test).
- **Commit:** `feat: MCP server and codebase_search tool`

### Step 8.3 — codebase_index_status tool
- **Do:** register tool `codebase_index_status`, return indexing status (is_indexing, progress, total_files, indexed_files, total_chunks, indexed_chunks).
- **Tests:** call tool before and after indexing — different status.
- **Commit:** `feat: MCP tool codebase_index_status`

### Step 8.4 — Documentation and wrap-up
- **Do:** update README: how to install, how to run, example MCP config for Cursor/Claude. Document env vars. CHANGELOG or version if needed.
- **Tests:** `go build ./cmd/...`, full test run.
- **Commit:** `docs: README and MCP setup`

---

## Benchmarks (add as packages appear)

- After Step 2.1: `BenchmarkTokenize`.
- After Step 3.1: search benchmark (N documents, query).
- After Step 4.2: indexing benchmark (e.g. 1000 files in testdata).

These can be added in separate commits after the corresponding steps, e.g.: `perf: add benchmark for tokenizer`.

---

## Checklist before each commit

1. `go build ./...`
2. `go test ./...`
3. `golangci-lint run` (when config is in use)
4. `git add -A` (deliberately), `git commit -m "..."`
