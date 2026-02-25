.PHONY: build test lint clean download-lancedb

# Сборка с эмбеддингами (LanceDB). Требует: make download-lancedb и CGO.
# Платформа определяется автоматически (darwin_arm64, darwin_amd64, linux_amd64, ...).
LANCEDB_LIB_DIR ?= $(shell pwd)/lib
LANCEDB_PLATFORM ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')_$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
LANCEDB_LIB := $(LANCEDB_LIB_DIR)/$(LANCEDB_PLATFORM)/liblancedb_go.a
CGO_LANCEDB := CGO_LDFLAGS="$(LANCEDB_LIB) -framework Security -framework CoreFoundation"

build:
	@test -f $(LANCEDB_LIB) || (echo "LanceDB native lib not found. Run: make download-lancedb"; exit 1)
	$(CGO_LANCEDB) go build -tags lancedb -o coderag-mcp ./cmd/coderag-mcp

# Скачать нативные библиотеки LanceDB (один раз перед make build).
download-lancedb:
	@curl -sSL https://raw.githubusercontent.com/lancedb/lancedb-go/main/scripts/download-artifacts.sh | bash

# Сборка без эмбеддингов (только BM25).
build-no-embeddings:
	go build -o coderag-mcp ./cmd/coderag-mcp

test:
	go test ./...

lint:
	golangci-lint run

clean:
	go clean -cache
	rm -f coderag-mcp
