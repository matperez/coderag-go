# План реализации CodeRAG-Go

Пошаговый план с проверкой тестов и коммитом после каждого шага.

**Правило:** после каждого шага — `go test ./...`, при необходимости `golangci-lint run`, затем `git add` + `git commit` с осмысленным сообщением.

---

## Фаза 0: Каркас проекта

### Шаг 0.1 — Структура каталогов и линтер
- **Сделать:** создать структуру пакетов (например `internal/tokenizer`, `internal/storage`, `internal/chunk`, `internal/search`, `internal/indexer`, `cmd/coderag-mcp`), добавить `.golangci.yml` (или минимальный конфиг), `Makefile` или скрипт с целями `test`, `lint`, `build`.
- **Тесты:** `go build ./...`, `golangci-lint run` (линтер может быть пустым до первого кода).
- **Коммит:** `chore: project layout, golangci-lint, make targets`

### Шаг 0.2 — Каталог данных и хеш проекта
- **Сделать:** пакет `internal/datadir` (или `pkg/datadir`): функция `DataDir(root string) string` → `~/.coderag-go/projects/<hash>/`, хеш = первые 16 символов SHA-256 от абсолютного пути `root`. Юнит-тест на стабильность хеша и путь.
- **Тесты:** `go test ./internal/datadir/...` (или аналог).
- **Коммит:** `feat: data dir resolution ~/.coderag-go/projects/<hash>`

---

## Фаза 1: Хранилище и базовый индекс

### Шаг 1.1 — Схема SQLite и миграции
- **Сделать:** таблицы `files` (id, path, content, hash, size, mtime, language, indexed_at), `chunks` (id, file_id, content, type, start_line, end_line, metadata, token_count, magnitude), `document_vectors` (id, chunk_id, term, tf, tfidf, raw_freq). Миграции в `internal/storage/migrations` или отдельном пакете (up/down SQL). Драйвер: `modernc.org/sqlite`.
- **Тесты:** тест применения миграций на временной БД (после migrate — таблицы существуют, можно вставить строку).
- **Коммит:** `feat: SQLite schema and migrations for files, chunks, vectors`

### Шаг 1.2 — Интерфейс хранилища и реализация (файлы + чанки)
- **Сделать:** интерфейс `Storage` с методами `StoreFile`, `StoreChunks`, `GetFile`, `ListFiles` (или минимальный набор из референса). Реализация `SQLiteStorage` в `internal/storage`, открытие БД по пути из datadir. Сохранение файла и чанков без векторов.
- **Тесты:** юнит-тесты с временной SQLite (StoreFile + StoreChunks, чтение обратно).
- **Коммит:** `feat: Storage interface and SQLite implementation (files, chunks)`

### Шаг 1.3 — Character-based chunking
- **Сделать:** пакет `internal/chunk`: функция `ChunkByCharacters(content string, maxChunkSize int) []Chunk`. Тип `Chunk` с полями Content, StartLine, EndLine, Type (например "text"). Без AST, только разбиение по размеру (и по строкам, чтобы не резать посередине строки — по желанию).
- **Тесты:** table-driven тесты (пустая строка, короткий текст, длинный текст, границы maxChunkSize).
- **Коммит:** `feat: character-based chunking`

---

## Фаза 2: Токенизатор

### Шаг 2.1 — Токенизатор (Unicode + camelCase)
- **Сделать:** пакет `internal/tokenizer`: границы слов `\p{L}\p{N}_`, разбиение по ним; для ASCII-сегментов — `fatih/camelcase`; lowercase; отбрасывание токенов короче 2 символов. Функция `Tokenize(text string) []string`.
- **Тесты:** table-driven: идентификаторы (`getUserById` → get, user, by, id), русский текст («получить пользователя»), смешанный код и комментарии.
- **Коммит:** `feat: code-aware tokenizer (Unicode, camelCase)`

### Шаг 2.2 — Опционально: русский стемминг
- **Сделать:** опция в токенизаторе (конструктор или флаг): для токенов из кириллицы вызывать стеммер (snowballstem/snowball). По умолчанию можно отключить.
- **Тесты:** тесты на «пользователя» → «пользователь», «получить» → «получ» (или как даёт стеммер).
- **Коммит:** `feat: optional Russian stemming in tokenizer`

---

## Фаза 3: Поиск (BM25 / TF-IDF)

### Шаг 3.1 — BM25/TF-IDF по чанкам в памяти
- **Сделать:** пакет `internal/search`: построение индекса (термы по чанкам, IDF), функция `Search(query string, index Index, limit int) []Result`. Использовать `internal/tokenizer`. Структуры: DocumentVector (uri, terms, magnitude), SearchIndex (documents, idf). BM25 с параметрами k1, b.
- **Тесты:** несколько документов (чанков), запрос, проверка порядка и наличия ожидаемых документов; граничные случаи (пустой запрос, нет совпадений).
- **Коммит:** `feat: BM25 search over in-memory index`

### Шаг 3.2 — Сохранение и загрузка векторов в SQLite
- **Сделать:** в хранилище: методы для записи/чтения векторов (термы по chunk_id: term, tf, tfidf, raw_freq). При индексации — сохранение векторов после токенизации. Предвычисление magnitude и сохранение в chunks.
- **Тесты:** StoreFile + StoreChunks + запись векторов; чтение векторов для заданного chunk_id или всех.
- **Коммит:** `feat: persist TF-IDF vectors in SQLite`

### Шаг 3.3 — Поиск по SQLite (low-memory)
- **Сделать:** функция поиска, работающая по данным из SQLite: по запросу получить термы, по термам выбрать chunk_id из document_vectors, вычислить скоринги, вернуть топ-N. Интерфейс, совместимый с текущим Search (или одна функция с флагом in-memory vs DB).
- **Тесты:** запись индекса в SQLite, поиск через SQLite-режим, сравнение с ожидаемым порядком.
- **Коммит:** `feat: search via SQLite (low-memory mode)`

---

## Фаза 4: Индексатор (скан + чанки + хранилище)

### Шаг 4.1 — Сканирование файлов и .gitignore
- **Сделать:** пакет `internal/scan` или внутри `internal/indexer`: обход директории от root, учёт .gitignore (библиотека `ignore` или своя минимальная), фильтр по расширениям, maxFileSize. Выдача списка путей и при необходимости метаданных (mtime, size).
- **Тесты:** тестовая директория в testdata с несколькими файлами и .gitignore; проверка, что исключённые файлы не попадают в список.
- **Коммит:** `feat: file scanner with gitignore support`

### Шаг 4.2 — Индексатор: сканирование → чанки → хранилище
- **Сделать:** `internal/indexer`: запуск сканера, чтение файлов, character-based chunking, токенизация чанков, сохранение в Storage (файлы + чанки + векторы). Без watch. Методы `Index(ctx, root)` и при необходимости `GetStatus()`.
- **Тесты:** юнит с моком Storage; e2e с временной директорией и реальной SQLite — индекс небольшой testdata, проверка что файлы и чанки записаны.
- **Коммит:** `feat: indexer (scan, chunk, tokenize, store)`

### Шаг 4.3 — Статус индексации
- **Сделать:** метод `IndexStatus` или аналог: идёт ли индексация, прогресс (processed/total files или chunks), общее число проиндексированных файлов/чанков. Хранилище должно уметь отдавать counts.
- **Тесты:** во время индексации статус отражает прогресс; после — итоговые числа.
- **Коммит:** `feat: index status (progress, counts)`

---

## Фаза 5: AST chunking (опционально после MVP)

### Шаг 5.1 — Интеграция tree-sitter и маппинг языков
- **Сделать:** зависимость smacker/go-tree-sitter, маппинг расширения файла → язык (конфиг или функция). Для выбранного языка получать grammar и парсить в AST.
- **Тесты:** парсинг короткого фрагмента кода (Go/JS) — дерево не nil, есть ожидаемые узлы.
- **Коммит:** `feat: tree-sitter integration and language mapping`

### Шаг 5.2 — Извлечение чанков по границам AST
- **Сделать:** обход AST, выделение узлов по типам (function, class, method и т.д.), формирование []Chunk с контекстом (импорты). Слияние мелких, разбиение крупных по maxChunkSize. Fallback на character-based при ошибке или неизвестном языке.
- **Тесты:** тесты на типичных файлах (Go, JS/TS) — количество и типы чанков; fallback для .md или битого кода.
- **Коммит:** `feat: AST-based chunking with fallback`

### Шаг 5.3 — Подключение AST chunking к индексатору
- **Сделать:** в индексаторе для поддерживаемых расширений вызывать AST chunking вместо character-based. Конфигурируемый maxChunkSize.
- **Тесты:** e2e: индекс проекта с .go/.ts файлами, поиск по имени функции — в результатах нужный чанк.
- **Коммит:** `feat: use AST chunking in indexer for supported languages`

---

## Фаза 6: Watcher и инкрементальные обновления

### Шаг 6.1 — fsnotify и очередь изменений
- **Сделать:** подписка на события fsnotify по корню проекта (рекурсивно), фильтрация по .gitignore и расширениям. Очередь событий add/change/remove с дедупликацией и debounce.
- **Тесты:** тест с временной директорией: создание/изменение/удаление файла — в очереди ожидаемое событие.
- **Коммит:** `feat: file watcher with event queue`

### Шаг 6.2 — Инкрементальное обновление индекса
- **Сделать:** при событии — перечитать файл (или удалить из индекса), переразбить на чанки, пересчитать векторы, обновить/удалить в Storage. Учёт hash/mtime для «без изменений».
- **Тесты:** e2e: индекс → изменить файл → обновление → поиск возвращает актуальный контент.
- **Коммит:** `feat: incremental index updates on file change`

### Шаг 6.3 — Режим watch в индексаторе
- **Сделать:** флаг `Watch bool` в опциях индексатора; при true после первой индексации запускать watcher и обрабатывать события в фоне. Корректная остановка (context cancel / Stop).
- **Тесты:** запуск с watch, изменение файла, проверка обновления; остановка без паники.
- **Коммит:** `feat: indexer watch mode`

---

## Фаза 7: Векторный поиск и эмбеддинги (опционально)

### Шаг 7.1 — Клиент эмбеддингов (OpenAI-совместимый)
- **Сделать:** пакет `internal/embeddings`: интерфейс `EmbeddingProvider` (GenerateEmbedding, GenerateEmbeddings batch), реализация через HTTP к OpenAI API. Конфиг из env.
- **Тесты:** мок-провайдер; опционально интеграционный тест с реальным API (skip без ключа).
- **Коммит:** `feat: embedding provider (OpenAI-compatible)`

### Шаг 7.2 — LanceDB и запись векторов
- **Сделать:** создание таблицы в LanceDB по пути из datadir, запись векторов (id, vector, metadata). Поиск k-NN по запросу (эмбеддинг запроса).
- **Тесты:** запись нескольких векторов, поиск — возвращается ожидаемый id.
- **Коммит:** `feat: LanceDB vector storage`

### Шаг 7.3 — Гибридный поиск
- **Сделать:** объединение результатов BM25 и векторного поиска (fusion по весам). Опция в индексаторе/поиске: при наличии EmbeddingProvider индексировать эмбеддинги и в поиске вызывать гибрид.
- **Тесты:** мок эмбеддингов, проверка что результаты содержат и ключевые, и семантические совпадения.
- **Коммит:** `feat: hybrid search (BM25 + vector)`

---

## Фаза 8: CLI и MCP-сервер

### Шаг 8.1 — CLI: флаги и инициализация
- **Сделать:** `cmd/coderag-mcp`: разбор `--root`, `--index-only`, `--max-size` и т.д. Создание DataDir, Storage, Indexer. При `--index-only` — запуск индексации и выход.
- **Тесты:** запуск с `--index-only` на testdata, проверка что индекс создаётся (например, проверка наличия файла БД или вызов status).
- **Коммит:** `feat: CLI (--root, --index-only)`

### Шаг 8.2 — MCP-сервер и инструмент codebase_search
- **Сделать:** подключение modelcontextprotocol/go-sdk, stdio-транспорт. Регистрация инструмента `codebase_search` (query, limit, file_extensions, path_filter, exclude_paths, include_content). Вызов поиска по индексу, форматирование ответа (markdown с путями и сниппетами).
- **Тесты:** e2e: запуск сервера, отправка JSON-RPC запроса на вызов инструмента, парсинг ответа и проверка наличия полей/результатов (можно в тесте с subprocess).
- **Коммит:** `feat: MCP server and codebase_search tool`

### Шаг 8.3 — Инструмент codebase_index_status
- **Сделать:** регистрация инструмента `codebase_index_status`, возврат статуса индексации (is_indexing, progress, total_files, indexed_files, total_chunks, indexed_chunks).
- **Тесты:** вызов инструмента до и после индексации — различие в статусе.
- **Коммит:** `feat: MCP tool codebase_index_status`

### Шаг 8.4 — Документация и финализация
- **Сделать:** обновить README: как установить, как запустить, пример конфига MCP для Cursor/Claude. Указать переменные окружения. При необходимости CHANGELOG или версия.
- **Тесты:** `go build ./cmd/...`, полный прогон тестов.
- **Коммит:** `docs: README and MCP setup`

---

## Бенчмарки (добавлять по мере появления пакетов)

- После Шага 2.1: бенчмарк `BenchmarkTokenize`.
- После Шага 3.1: бенчмарк поиска по индексу (N документов, запрос).
- После Шага 4.2: бенчмарк индексации (например, 1000 файлов в testdata).

Их можно вводить в отдельных коммитах после соответствующих шагов, например: `perf: add benchmark for tokenizer`.

---

## Чеклист перед каждым коммитом

1. `go build ./...`
2. `go test ./...`
3. `golangci-lint run` (когда конфиг подключён)
4. `git add -A` (осознанно), `git commit -m "..."`
