# Shared embedding cache (idea, to revisit)

**Status:** idea only, not implemented.

## Goal

Speed up building the semantic (vector) index when several people index the same or similar codebase, or when re-indexing after a clone. Instead of recomputing embeddings for every chunk via the API each time, use a **centralized cache** that stores embeddings by content and returns them on cache hit.

## How it would work

1. **Before** calling the embedding API for a chunk, the indexer sends the chunk’s **content hash** (e.g. SHA-256 of the text) to the shared service.
2. If the service already has an embedding for that hash (same model), it returns the vector **without** calling the embedding API.
3. If not, the indexer computes the embedding (or the service does), stores it in the cache keyed by hash, and uses it.

So: first run or new content → API call and cache write; repeated content (same repo on another machine, or unchanged files) → cache hit, no API call.

## Why content hash, not mtime

`mtime` is **not** preserved when you clone a repo or copy it to another machine: Git creates files at checkout time, so every clone gets a new mtime. A key like `(path, mtime)` would almost never match across machines. **Content hash** is stable: same file content ⇒ same hash ⇒ same cache key everywhere.

## Benefits

- **Faster indexing** for the team: many chunks (e.g. unchanged or shared files) get cache hits.
- **Lower API usage** and cost when using a paid embedding API (e.g. OpenRouter).
- Rough scale: with ~360k chunks and ~1.1 s per batch of 20, full recompute is ~5.5 h; high cache hit rate could cut that to a small fraction.

## What’s left to do later

- Design of the service: API (REST/gRPC), auth, storage (DB or blob store), TTL/invalidation.
- Integration in the indexer: optional “embedding cache” client, fallback to direct API on miss or when cache is disabled.
- Choice of hash (e.g. SHA-256 of normalized chunk text) and model id in the key so different models don’t share entries.
- Consider privacy: cache only has vectors and hashes, not raw code, if we never store content in the cache (only hash → vector).

## References

- Embedding benchmark (OpenRouter): `scripts/embed_bench_result.md`
- Indexer embedding flow: `internal/indexer/indexer.go` (e.g. `embedBatch`, `GenerateEmbeddings`), `internal/embeddings/`
