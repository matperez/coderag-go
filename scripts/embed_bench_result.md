# OpenRouter embeddings benchmark (synthetic chunks)

**Date:** 2026-03-05  
**Endpoint:** `POST https://openrouter.ai/api/v1/embeddings`  
**Model:** `openai/text-embedding-3-small`  
**Payload:** synthetic placeholder text, 500 chars per chunk (no real code).

## Latency

| Request        | Run 1   | Run 2   | Run 3   | ~avg   |
|----------------|---------|---------|---------|--------|
| 1 chunk        | 1.06 s  | 0.54 s  | 0.63 s  | ~0.74 s |
| 20 chunks (batch) | 1.20 s | 1.12 s  | 1.10 s  | ~1.14 s |

- Single chunk: ~0.5–1.1 s (cold first request slower).
- Batch of 20: ~1.1 s per request → **~57 ms per chunk** when batched.

## Extrapolation (travel repo)

- **Chunks in index:** 361 903  
- **Batch size in app:** 20  
- **Number of requests:** 361 903 / 20 ≈ **18 096**  
- **At ~1.1 s/request:** 18 096 × 1.1 s ≈ **5.5 hours** (embedding API only, no I/O/LanceDB).

Response: `data[].embedding` dimension **1536** (text-embedding-3-small).
