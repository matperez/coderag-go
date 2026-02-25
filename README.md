# CodeRAG-Go

Гибридный поиск по кодовой базе (ключевой BM25 + опционально векторный) в виде одного Go-бинарника с MCP-сервером для интеграции с Cursor, Claude и др.

## Установка и запуск

**Требования:** Go 1.25+

```bash
# Клонировать и собрать
git clone https://github.com/matperez/coderag-go.git && cd coderag-go
go build -o coderag-mcp ./cmd/coderag-mcp
# или установить в $GOPATH/bin
go install ./cmd/coderag-mcp
```

**Флаги:**

- `--root` — корень проекта для индексации и поиска (по умолчанию `.`)
- `--index-only` — выполнить индексацию и выйти (без запуска MCP)
- `--max-size` — максимальный размер файла в байтах (0 = без ограничения)

**Примеры:**

```bash
# Индексация репозитория
./coderag-mcp --root /path/to/project --index-only

# Запуск MCP-сервера (ожидает JSON-RPC по stdin/stdout)
./coderag-mcp --root /path/to/project
```

Данные индекса хранятся в `~/.coderag-go/projects/<hash>/` (hash от абсолютного пути `--root`).

## Конфигурация MCP

### Cursor

В настройках MCP добавьте сервер, например в `.cursor/mcp.json` или в настройках Cursor:

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

Перед первым использованием выполните индексацию:

```bash
/path/to/coderag-mcp --root /path/to/your/project --index-only
```

### Claude Desktop / другие клиенты

Аналогично: укажите `command` (путь к `coderag-mcp`) и `args: ["--root", "<корень_проекта>"]`. Транспорт — stdio.

## Инструменты MCP

- **codebase_search** — поиск по кодовой базе (BM25): `query`, `limit`, `file_extensions`, `path_filter`, `exclude_paths`, `include_content`. Ответ в виде markdown с путями и опционально сниппетами.
- **codebase_index_status** — статус индекса: `is_indexing`, `progress`, количество файлов и чанков.

## Документация

- [Функциональные требования и принятые решения](docs/REQUIREMENTS_AND_DECISIONS.md) — что делает система и какие технические решения приняты.
- [План реализации](docs/IMPLEMENTATION_PLAN.md) — пошаговый план с тестами и коммитом после каждого шага.

## Сборка и тесты

```bash
make build   # или go build ./...
make test    # go test ./...
make lint    # golangci-lint run
```
