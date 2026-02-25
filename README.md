# CodeRAG-Go

Hybrid codebase search (BM25 keyword search + optional vector search) as a single Go binary with an MCP server for integration with Cursor, Claude, and others.

## Installation and usage

**Requirements:** Go 1.25+

```bash
# Clone and build
git clone https://github.com/matperez/coderag-go.git && cd coderag-go
go build -o coderag-mcp ./cmd/coderag-mcp
# or install to $GOPATH/bin
go install ./cmd/coderag-mcp
```

**Flags:**

- `--root` — project root to index and search (default: `.`)
- `--index-only` — run indexing and exit (do not start MCP)
- `--max-size` — max file size in bytes (0 = no limit)
- `--log-level` — log level: `debug`, `info`, `warn`, `error` (default: from `CODERAG_LOG` or `info`)

**Logging:** output goes to stderr (text format). The `CODERAG_LOG` environment variable sets the level when the flag is not used (`debug`, `info`, `warn`, `error`). Logged: startup (root, data_dir), indexing progress (every 10 files and on completion), tool calls (codebase_search, codebase_index_status), search errors, and skipped files.

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

- **codebase_search** — search the codebase (BM25): `query`, `limit`, `file_extensions`, `path_filter`, `exclude_paths`, `include_content`. Response as markdown with paths and optional snippets.
- **codebase_index_status** — index status: `is_indexing`, `progress`, file and chunk counts; when indexing, `current_file` is included.

## Documentation

- [Functional requirements and decisions](docs/REQUIREMENTS_AND_DECISIONS.md) — what the system does and which technical choices were made.
- [Implementation plan](docs/IMPLEMENTATION_PLAN.md) — step-by-step plan with tests and a commit after each step.

## Build and tests

```bash
make build   # or go build ./...
make test    # go test ./...
make lint    # golangci-lint run
```
