# CodeRAG-Go: functional requirements and decisions

This document describes what the system should do (functional requirements) and which technical decisions were made for the Go implementation.

---

## 1. Functional requirements

### 1.1. Codebase indexing

- **Scanning:** traverse the filesystem from the project root with respect to `.gitignore` and configurable exclusions.
- **File size limit:** files above a given threshold (default 1 MB) are not indexed or are processed in truncated form (implementation choice).
- **Supported formats:** source code (TypeScript, JavaScript, Python, Go, Rust, Java, etc.), configs (YAML, JSON, TOML), markup (Markdown, HTML), other text formats. Indexing of **code and comments**, including comments **in Russian**.
- **Semantic chunking:** split files into chunks at semantic boundaries (functions, classes, methods, headers, etc.), not by a fixed character count. For languages without AST support — fallback to size-based (character-based) chunking.
- **Persistence:** index is stored locally (SQLite) so the next run does not re-index everything. Support for incremental updates based on changed files.
- **Watch mode (optional):** when enabled, the index is updated in real time as files change.

### 1.2. Search

- **Hybrid search:** combination of keyword search (BM25/TF-IDF) and optional vector (semantic) search over embeddings.
- **Keyword search:** query is tokenized the same way as documents; ranking by BM25 (or TF-IDF). Covers both code and comments; queries in Russian and English should match relevant fragments.
- **Vector search (optional):** when an API key is available (e.g. OpenAI), chunk embeddings are indexed; the query is embedded and k-NN search is run. Results are merged with keyword search (fusion) using configurable weights.
- **Filters:** by file extensions, by paths (include/exclude), by chunk type (e.g. functions only).
- **Response format:** file path, code snippet, line numbers, relevant terms, relevance score. Output in a format suitable for LLMs (e.g. markdown with code blocks).

### 1.3. Launch and CLI

- **Binary:** a single executable (name at discretion, e.g. `coderag-go` or `coderag-mcp`).
- **Default mode:** run as MCP server (stdio), waiting for requests from IDE/client.
- **Command-line arguments:**
  - `--root=<path>` — codebase root to index and search (default: current working directory).
  - `--index-only` — run indexing once and exit (without starting MCP). Useful for pre-indexing or CI.
  - As needed: `--max-size=<bytes>`, `--no-auto-index`, etc., following the reference implementation.
- **Example usage (similar to coderag):**
  - MCP: `coderag-mcp --root=.` or `coderag-go --root=/path/to/project`
  - Index only: `coderag-mcp --root=. --index-only`

### 1.4. Data storage

- **Default data directory:** `~/.coderag-go/projects/<hash>`, where `<hash>` is a stable hash of the project root’s absolute path (e.g. first 16 characters of SHA-256). Different projects use separate data.
- In this directory: SQLite index database; with vector search enabled — LanceDB files; optionally project metadata (path, last access).
- Overriding the path (e.g. via environment variable or flag) is optional.

### 1.5. Integration (MCP)

- **MCP server:** one binary acts as an MCP server (stdio transport) for integration with Cursor, Claude Desktop, VS Code, Windsurf, etc.
- **Tools:**
  - **`codebase_search`** — search the codebase. Parameters: query (`query`), result limit (`limit`), filters (`file_extensions`, `path_filter`, `exclude_paths`), include content in response (`include_content`). Response: list of relevant fragments with path, lines, snippet, score.
  - **`codebase_index_status`** — returns indexing status: whether indexing is in progress, progress (e.g. files/chunks processed), number of indexed files/chunks. Lets the IDE show the user the index state.

### 1.6. Non-functional requirements

- **Single binary:** distribution and run as one executable without requiring Python/Node/Rust runtimes.
- **Fast startup:** with an existing index, search is available within hundreds of milliseconds after launch.
- **Low search latency:** response within tens of milliseconds on typical sizes (thousands to tens of thousands of chunks).
- **Offline:** keyword search and use of a stored index work without network; vector search requires access to an embeddings API.
- **Modularity:** where possible, code is organized into packages with clear boundaries (tokenizer, storage, chunking, search, MCP), dependencies via interfaces, so components can be tested and replaced independently.

### 1.7. Code quality and testing

- **Tests:** unit tests for core logic (tokenizer, BM25/TF-IDF, chunking, storage). Table-driven tests and isolated tests with mocks for external dependencies.
- **Linter:** static analysis (e.g. `golangci-lint`) and passing it in CI; formatting via `go fmt` / `goimports`.
- **E2E tests:** end-to-end tests where needed: e.g. “index test codebase → search by query → check format and expected results”, or “start MCP server → call tool → check response”. E2E not required for every feature, but needed for critical flows (search, indexing, MCP).
- **Benchmarks:** Go benchmarks (`go test -bench`) for performance-critical parts: tokenization, index build/update, search. Used to catch regressions and justify optimizations.

---

## 2. Technical decisions

### 2.1. Platform and constraints

- **Language and runtime:** Go. Single process, single binary.
- **Disallowed dependencies:** no bindings to Rust, WASM, Python; no calling out to external processes for tokenization or parsing (everything in-process).

### 2.2. Tokenizer (search over code and comments, including Russian)

- **Choice:** extended code-oriented tokenizer in pure Go (no StarCoder2, no external models).
- **Rules:**
  - **Word boundaries:** word = sequence of letters in any script (Unicode `\p{L}`) + digits (`\p{N}`) + underscore. Split on spaces and punctuation. This gives correct tokenization for both code (identifiers) and Russian comments (“получить пользователя” → separate terms).
  - **camelCase / snake_case:** splitting is applied **only to ASCII segments** that look like identifiers (e.g. `getUserById` → `get`, `User`, `By`, `Id`). Tokens with Cyrillic are not split further.
  - **Normalization:** lowercase for all tokens.
  - **Optional:** for Cyrillic tokens — Russian stemming (e.g. [snowballstem/snowball](https://pkg.go.dev/github.com/snowballstem/snowball)) so “пользователь” and “пользователя” match in search.
- **Dependencies:** [fatih/camelcase](https://github.com/fatih/camelcase) for identifier splitting; optionally snowball stemmer (pure Go). No external processes or bindings.

### 2.3. AST chunking (semantic code splitting)

- **Parsers:** tree-sitter via Go bindings. All-in-one option: [smacker/go-tree-sitter](https://github.com/smacker/go-tree-sitter) with bundled grammars (javascript, typescript, python, golang, java, rust, html, markdown, toml, yaml, etc.).
- **Chunking layer:** implemented on top of tree-sitter in this project: map file extension → language, walk AST, extract nodes at semantic boundaries (function, class, method, header, etc.), add context (imports, types) to chunks, merge small and split large chunks by `maxChunkSize`. Logic and boundary list are based on the reference implementation (coderag-matperez) and language docs.
- **Fallback:** for unsupported extensions or parse errors — size-based (character-based) chunking.
- **Note:** smacker/go-tree-sitter uses CGO. If a CGO-free binary is required, parsing could be moved to a separate executable (subprocess) or alternatives considered.

### 2.4. Index storage

- **Database:** SQLite. Driver: [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go) to avoid CGO, or [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) if CGO is acceptable.
- **Schema:** tables for files (path, content/hash, mtime, language), chunks (file_id, content, type, start_line, end_line, metadata), TF-IDF/BM25 vectors (terms per chunk). Migrations — manual or via [golang-migrate](https://github.com/golang-migrate/migrate) / [goose](https://github.com/pressly/goose).
- **DB location:** default `~/.coderag-go/projects/<hash>/`, where `<hash>` is a stable hash of the project root path (see 1.4).

### 2.5. Vector storage (optional)

- **Library:** [lancedb/lancedb-go](https://pkg.go.dev/github.com/lancedb/lancedb-go/pkg/lancedb) — official Go SDK. Store embeddings and run nearest-neighbor search.
- **Usage:** only when an embedding provider is configured (e.g. OpenAI API key). DB path in the project data directory (e.g. `vectors.lance`).

### 2.6. Embeddings (optional)

- **Provider:** HTTP client to OpenAI Embeddings API or an OpenAI-compatible endpoint (base URL + API key). Configuration via environment variables (e.g. `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `EMBEDDING_MODEL`).
- **Implementation:** minimal wrapper in Go (or existing OpenAI Go SDK), request batching, rate limits. No Python/Node dependencies.

### 2.7. File watcher

- **Library:** [fsnotify](https://github.com/fsnotify/fsnotify). Cross-platform, stable. On file change/create/delete — enqueue and incrementally update the index (no full rescan).

### 2.8. MCP server

- **SDK:** [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk). MCP server implementation with stdio transport.
- **Tools:** `codebase_search` (search with query, limit, file_extensions, path_filter, exclude_paths, include_content), `codebase_index_status` (indexing status: progress, file/chunk counts). Response format aligned with the reference (markdown with code blocks for search).

### 2.9. Search (BM25 / TF-IDF)

- **Algorithm:** BM25 (or classic TF-IDF) over chunks. Formulas and data structures carried over from the reference; tokenization by the chosen tokenizer (see 2.2).
- **Low-memory mode:** with persistent storage, search can run against SQLite (queries to vector tables) without loading the full index into memory, to support large codebases.

### 2.10. Code quality and tooling

- **Linter:** [golangci-lint](https://golangci-lint.run/); config in repo, run in CI.
- **Tests:** `go test`, table-driven unit tests, mocks for interfaces (storage, embeddings, etc.).
- **E2E:** dedicated tests or test packages for “index → search” and if needed “MCP → call tool”; use test fixtures (small codebase in `testdata`).
- **Benchmarks:** `go test -bench` for tokenization, TF-IDF/BM25, indexing, and search; consider results when refactoring.

---

## 3. Recommended implementation order

1. **Storage and base index:** SQLite schema, migrations, save/load files and chunks (character-based chunking first).
2. **Tokenizer:** extended code-oriented tokenizer (Unicode + camelCase + optional stemming).
3. **Search:** BM25/TF-IDF, integration with tokenizer and storage; validation on typical queries (including Russian).
4. **AST chunking:** go-tree-sitter integration, language mapping, AST walk and chunk boundaries; fallback to character-based.
5. **Watcher and incremental updates:** fsnotify, change queue, re-index only affected files.
6. **Vector search and embeddings:** LanceDB, HTTP to API, merge with keyword search.
7. **MCP server:** wiring around indexer and search, tools `codebase_search` and `codebase_index_status`, CLI config (`--root`, `--index-only`).

---

## 4. References

- Reference implementation: **coderag-matperez** (TypeScript/Bun).
- Go migration research: **coderag-matperez/docs/RESEARCH_GO_REWRITE.md**.
