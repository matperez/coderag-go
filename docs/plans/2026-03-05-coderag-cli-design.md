# coderag-cli design

**Date:** 2026-03-05  
**Status:** approved

## Goal

Separate console binary **coderag-cli** to query an existing index (no indexing). Same index as MCP: `--root` → data dir. Commands: **search** and **status**. Optional **--json** for machine-readable output.

## Binary and global flags

- **Binary:** `coderag-cli` (new entrypoint `cmd/coderag-cli/main.go`).
- **--root** (global): project root path; default `.`. Same semantics as MCP: `datadir.DataDir(absoluteRoot)` → index under `~/.coderag-go/projects/<hash>/`.
- **--json** (global): output JSON instead of human-readable text for the chosen subcommand.

## Subcommand: search

- **Usage:** `coderag-cli [--root <path>] [--json] search <query> [options]`
- **Positional:** `query` — search string (required).
- **Options:** `--limit N` (default 10), `--include-content`, `--ext .go,.js`, `--path-filter <substring>`, `--exclude <substring>[, ...]` (or repeated `--exclude`). Same semantics as MCP codebase_search.
- **Logic:** Resolve data dir from `--root`, open SQLite storage (and optional embedder+vectorstore from env for hybrid). Tokenize query, `SearchCandidates`, apply filters, BM25 (or hybrid), format results.
- **Output (default):** Markdown to stdout (same as MCP): numbered list, path, Lstart–Lend, optional code block with content.
- **Output (--json):** Single JSON object, e.g. `{"results":[{"uri":"file://...", "path":"...", "score":0.42, "start_line":1, "end_line":10, "content":"...", "matched_terms":["x","y"]}]}`. Omit `content` when `--include-content` is false.
- **Exit:** 0 on success, 1 on error (missing index, search failure).

## Subcommand: status

- **Usage:** `coderag-cli [--root <path>] [--json] status`
- **Logic:** Resolve data dir, open storage, `FileCount()`, `ChunkCount()`; optionally read `metadata.Read(dataDir)` for last accessed / IDF rebuild time.
- **Output (default):** Human-readable lines, e.g. `files: 34477`, `chunks: 361903`, optional `last_accessed: ...`, `idf_rebuild_completed_at: ...`.
- **Output (--json):** Single JSON object, e.g. `{"files":34477,"chunks":361903,"last_accessed_at":"...","idf_rebuild_completed_at":"..."}`. Omit metadata fields if missing.
- **Exit:** 0 on success, 1 if index missing or storage error.

## Error handling

- Data dir missing or no `index.db`: clear message to stderr, exit 1.
- Storage open error: log and exit 1.
- Search (e.g. no terms, DB error): message to stderr, exit 1. With `--json`, can add `"error":"..."` in JSON and still exit 1.

## Build and layout

- **Makefile:** Add target to build `bin/coderag-cli` (same pattern as `coderag-mcp`: optional lancedb, same CGO if needed). Optionally `make build` builds both binaries.
- **Code reuse:** Use existing `internal/datadir`, `internal/storage`, `internal/search`, `internal/tokenizer`, `internal/embeddings`, `internal/vectorstore`, `internal/metadata`. No MCP SDK in CLI. Duplicate only flag parsing and output formatting (markdown vs JSON).

## Testing

- At least one integration-style test: create temp index (or use test storage), run `search` and `status` (with and without `--json`), assert exit code and that output contains expected numbers or result count.
