# CodeRAG-Go

Hybrid codebase search (BM25 keyword search + optional vector search) as a single Go binary with an MCP server for integration with Cursor, Claude, and others.

Inspired by [SylphxAI/coderag](https://github.com/SylphxAI/coderag) (lightning-fast semantic code search with AST chunking, hybrid TF-IDF + vector, MCP-ready).

## Installation and usage

**Requirements:** Go 1.25+

The default build includes LanceDB for optional vector search. Embeddings are enabled at runtime when the relevant environment variables are set; without them, only BM25 search is used.

```bash
# Clone and build (with LanceDB for hybrid search)
git clone https://github.com/matperez/coderag-go.git && cd coderag-go
# One-time: download native LanceDB libraries (required for build with embeddings)
make download-lancedb
# Build with embeddings (Makefile sets CGO_LDFLAGS automatically)
make build
# Or build without embeddings (BM25 only, no CGO)
make build-no-embeddings
```

**Embeddings (optional):** To enable hybrid BM25 + vector search, set environment variables for an OpenAI-compatible API. At startup the binary will create an embedding provider and vector store; indexing will then write chunk embeddings to LanceDB and `codebase_search` will use hybrid ranking.

- `OPENAI_API_KEY` — API key (can be any value for local servers, e.g. `ollama`)
- `OPENAI_BASE_URL` — API base URL (default: `https://api.openai.com/v1`)
- `EMBEDDING_MODEL` or `OPENAI_EMBEDDING_MODEL` — model name (default: `text-embedding-3-small`)

Example for local Ollama:

```bash
export OPENAI_BASE_URL="http://localhost:11434/v1"
export OPENAI_API_KEY="ollama"
export EMBEDDING_MODEL="nomic-embed-text-v2-moe"
# Optional: set context size (e.g. 8192); also truncates long inputs so they fit (or set EMBEDDING_MAX_INPUT_CHARS explicitly)
export OLLAMA_NUM_CTX=8192
./coderag-mcp --root /path/to/project
```

If the embedding API is unreachable or the vector store cannot be opened, the app logs a warning and runs in BM25-only mode.

**Flags:**

- `--root` — project root to index and search (default: `.`)
- `--index-only` — run indexing and exit (do not start MCP)
- `--max-size` — max file size in bytes (0 = no limit)
- `--log-level` — log level: `debug`, `info`, `warn`, `error` (default: from `CODERAG_LOG` or `info`)
- `--pprof` — enable pprof HTTP server at address, e.g. `:6060` (or set `CODERAG_PPROF`); use for CPU/memory profiling during indexing

**Logging:** output goes to stderr (text format). The `CODERAG_LOG` environment variable sets the level when the flag is not used (`debug`, `info`, `warn`, `error`). Logged: startup (root, data_dir); indexing progress (every 10 files and on completion); after all files are processed — IDF computation, storing chunks and BM25 vectors, and (when embeddings are enabled) generating and writing embeddings; then `indexing done`; tool calls (codebase_search, codebase_index_status); search errors; skipped files.

**Profiling:** run with `--pprof :6060` (or `CODERAG_PPROF=:6060`). Then open `http://localhost:6060/debug/pprof/` in a browser or run `go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile?seconds=30` to capture a 30s CPU profile, or `go tool pprof http://localhost:6060/debug/pprof/heap` for a heap snapshot. Use this to see where CPU or memory is spent during indexing.

**Examples:**

```bash
# Index a repository
./coderag-mcp --root /path/to/project --index-only

# Run MCP server (expects JSON-RPC on stdin/stdout)
./coderag-mcp --root /path/to/project
```

Index data is stored in `~/.coderag-go/projects/<hash>/` (hash of the absolute `--root` path).

## MCP configuration

### Cursor

Add the server in MCP settings, e.g. in `.cursor/mcp.json` or in Cursor settings:

```json
{
  "mcpServers": {
    "coderag-go": {
      "command": "/path/to/coderag-mcp",
      "args": ["--root", "/path/to/your/project"]
    }
  }
}
```

Run indexing before first use:

```bash
/path/to/coderag-mcp --root /path/to/your/project --index-only
```

### Claude Desktop / other clients

Same idea: set `command` (path to `coderag-mcp`) and `args: ["--root", "<project_root>"]`. Transport is stdio.

## MCP tools

- **codebase_search** — search the codebase (BM25, or BM25 + vector when embeddings are enabled): `query`, `limit`, `file_extensions`, `path_filter`, `exclude_paths`, `include_content`. Response as markdown with paths and optional snippets.
- **codebase_index_status** — index status: `is_indexing`, `progress`, file and chunk counts; when indexing, `current_file` is included.

## Documentation

- [Functional requirements and decisions](docs/REQUIREMENTS_AND_DECISIONS.md) — what the system does and which technical choices were made.
- [Implementation plan](docs/IMPLEMENTATION_PLAN.md) — step-by-step plan with tests and a commit after each step.

## Build and tests

```bash
make build   # go build -tags lancedb -o coderag-mcp ./cmd/coderag-mcp
make test    # go test ./...
make lint    # golangci-lint run
```

**Benchmarks:** run `go test -bench=. -benchmem ./internal/tokenizer/ ./internal/search/ ./internal/indexer/` to measure tokenization, search, and indexing performance; use to catch regressions and justify optimizations.

To build without LanceDB (BM25-only, no vector store), run `go build ./cmd/coderag-mcp`. The resulting binary will log "vector store requires build with -tags lancedb" if embeddings env vars are set.
