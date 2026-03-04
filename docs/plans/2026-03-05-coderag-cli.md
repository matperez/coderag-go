# coderag-cli Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a separate console binary `coderag-cli` with subcommands `search` and `status` that query an existing index (no indexing); global flags `--root` and `--json` for index path and machine-readable output.

**Architecture:** New `cmd/coderag-cli/main.go` reuses `internal/datadir`, `internal/storage`, `internal/search`, `internal/tokenizer`, `internal/embeddings`, `internal/vectorstore`, `internal/metadata`. No MCP SDK. Subcommands dispatch to shared search/status logic; output is either human-readable (markdown/text) or JSON when `--json` is set.

**Tech Stack:** Go stdlib `flag`, `encoding/json`; existing packages for storage and search.

**Design reference:** `docs/plans/2026-03-05-coderag-cli-design.md`

---

## Task 1: Scaffold coderag-cli and global flags

**Files:**
- Create: `cmd/coderag-cli/main.go`

**Step 1: Implement minimal main**

- Parse global flags: `root` (string, default "."), `json` (bool, default false).
- Parse subcommand: first non-flag arg must be `search` or `status`; otherwise print usage to stderr and exit 1.
- Resolve absolute root with `filepath.Abs(root)`; get data dir with `datadir.DataDir(absoluteRoot)`. If data dir path is missing or `index.db` does not exist, print "index not found for root: <root>" to stderr, exit 1.
- For now: if subcommand is `status`, open storage, call `FileCount()` and `ChunkCount()`, print "files: N" and "chunks: M" to stdout, exit 0. If subcommand is `search`, print "query required" and exit 1 (stub).
- Use `slog` for errors; exit 1 on storage open error.

**Step 2: Run build**

```bash
cd /Users/angolovin/projects/coderag-go
go build -o bin/coderag-cli ./cmd/coderag-cli
```

Expected: build succeeds (no lancedb in CLI for simplicity first, or same CGO as mcp — design says "optional embedder+vectorstore"; for minimal first step use storage only).

**Step 3: Manual smoke test**

```bash
./bin/coderag-cli --root . status
```

Expected: if no index: "index not found" and exit 1. If index exists: "files: N", "chunks: M" and exit 0.

**Step 4: Commit**

```bash
git add cmd/coderag-cli/main.go
git commit -m "feat(cli): add coderag-cli scaffold with --root, status subcommand"
```

---

## Task 2: status: add metadata and --json

**Files:**
- Modify: `cmd/coderag-cli/main.go`

**Step 1: Status metadata**

- In status branch: call `metadata.Read(dataDir)`. If non-nil, include in output: `last_accessed_at: m.LastAccessedAt`, `idf_rebuild_completed_at: m.IDFRebuildCompletedAt` (skip if empty).

**Step 2: --json for status**

- If global `json` is true, output a single JSON object to stdout: `{"files":N,"chunks":M,"last_accessed_at":"...","idf_rebuild_completed_at":"..."}`. Omit optional fields if empty. No extra newline or pretty-print required (compact JSON is fine).

**Step 3: Run and test**

```bash
./bin/coderag-cli --root /path/to/project status
./bin/coderag-cli --root /path/to/project --json status
```

Expected: human lines vs one JSON line.

**Step 4: Commit**

```bash
git add cmd/coderag-cli/main.go
git commit -m "feat(cli): status metadata and --json output"
```

---

## Task 3: search subcommand (human output)

**Files:**
- Modify: `cmd/coderag-cli/main.go`

**Step 1: Parse search args**

- For subcommand `search`: require one positional argument (the query). Parse optional flags: `--limit` (int, default 10), `--include-content` (bool), `--ext` (string, comma-separated extensions), `--path-filter` (string), `--exclude` (string, comma-separated). Use a new `flag.FlagSet` for search so that `coderag-cli search --limit 5 "handler"` works.

**Step 2: Run search pipeline**

- Reuse logic from MCP: tokenize query with `tokenizer.Tokenize(query)`; if no tokens, print "No search terms." to stderr, exit 1. Call `st.SearchCandidates(tokens)`. Build `[]search.StorageCandidate` applying same filter logic as in main.go (matchFilters: file_extensions, path_filter, exclude_paths). Compute avgDocLength from candidates. If embedder and vecstore are available (same env as MCP: OPENAI_API_KEY or OPENAI_BASE_URL), build `search.HybridOpts` and call `search.HybridFromStorage`; else call `search.SearchFromStorage`. Limit results to `--limit`.

**Step 3: Markdown output**

- Format results like MCP: for each result, print "### N. path (Lstart-Lend)\n\n" and optionally a code block with result.Content. Path: use result.URI (file://...) or join with root for absolute path. Write to stdout.

**Step 4: Embedder/vectorstore init (optional)**

- In main, after opening storage: if embeddings env is set, create embedder and vectorstore the same way as in cmd/coderag-mcp (embCfg, NewOpenAIProvider, vectorstore.Open). Pass to search when calling HybridFromStorage. If not set, pass nil and use BM25-only.

**Step 5: Build and test**

```bash
go build -o bin/coderag-cli ./cmd/coderag-cli
./bin/coderag-cli --root /path/to/indexed/project search "http handler" --limit 3 --include-content
```

Expected: markdown list of up to 3 results with paths and snippets.

**Step 6: Commit**

```bash
git add cmd/coderag-cli/main.go
git commit -m "feat(cli): search subcommand with filters and optional hybrid"
```

---

## Task 4: search --json output

**Files:**
- Modify: `cmd/coderag-cli/main.go`

**Step 1: JSON struct and output**

- Define a struct for one result: `URI`, `Path` (trimmed file://), `Score`, `StartLine`, `EndLine`, `Content` (omitempty), `MatchedTerms`. When global `--json` is true, encode `{"results": [...]}` to stdout (compact JSON). Omit `Content` when `--include-content` is false.

**Step 2: Test**

```bash
./bin/coderag-cli --root /path/to/project --json search "handler" --limit 2
```

Expected: single line JSON with results array.

**Step 3: Commit**

```bash
git add cmd/coderag-cli/main.go
git commit -m "feat(cli): --json output for search"
```

---

## Task 5: Makefile and build parity with coderag-mcp

**Files:**
- Modify: `Makefile`

**Step 1: Add CLI targets**

- Add a target `build-cli` that builds `bin/coderag-cli` with the same CGO/LanceDB setup as `build` (so hybrid search works): `$(CGO_LANCEDB) go build -tags lancedb -o bin/coderag-cli ./cmd/coderag-cli`. Add `build-no-embeddings-cli` that builds without lancedb: `go build -o bin/coderag-cli ./cmd/coderag-cli`. Optionally extend `build` to build both binaries (e.g. build coderag-mcp and coderag-cli in one rule). Update `install` to copy both binaries if both exist, or add `install-cli`.

**Step 2: Commit**

```bash
git add Makefile
git commit -m "build: add coderag-cli targets to Makefile"
```

---

## Task 6: Tests for CLI

**Files:**
- Create: `cmd/coderag-cli/main_test.go` (or `cli_test.go`)

**Step 1: Test status and search with test index**

- Use a temp dir; create a minimal index (copy test DB or use storage to StoreFile + StoreChunks + StoreChunkVectors for one file with a few terms). Run CLI via `exec.Command`: `coderag-cli --root <tmp> status` and `coderag-cli --root <tmp> --json status`; parse stdout/stderr and assert files/chunks in output or JSON. Run `coderag-cli --root <tmp> search "term" --limit 1` and assert at least one result or "No matches" and exit 0; with `--json` assert JSON has "results" key. Use `go test -v ./cmd/coderag-cli/...` (build the binary first or build in test with `go build -o <tmp>/coderag-cli .` and set PATH or use full path in exec).

**Step 2: Test missing index**

- Run `coderag-cli --root /nonexistent/dir status`; expect exit code 1 and stderr containing "index not found" or similar.

**Step 3: Commit**

```bash
git add cmd/coderag-cli/main_test.go
git commit -m "test(cli): add tests for status and search"
```

---

## Task 7: README and docs

**Files:**
- Modify: `README.md`

**Step 1: Document coderag-cli**

- Add short section "Console CLI (coderag-cli)": describe that it queries an existing index (no indexing). Usage: `coderag-cli --root <project> search "<query>" [--limit N] [--include-content] [--json]` and `coderag-cli --root <project> status [--json]`. Mention that index must exist (build it with `coderag-mcp --root <project> --index-only`). Optionally list `--ext`, `--path-filter`, `--exclude`. Add build: `make build-cli` or `make build` if it builds both.

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add coderag-cli usage to README"
```

---

## Execution

Plan complete and saved to `docs/plans/2026-03-05-coderag-cli.md`.

**Two execution options:**

1. **Subagent-driven (this session)** — implement task-by-task with review between tasks.
2. **Parallel session (separate)** — open a new session and use the executing-plans skill for batch execution with checkpoints.

Which approach do you prefer?
