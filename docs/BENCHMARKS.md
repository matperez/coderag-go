# Benchmark results

Baseline and comparison runs for regression checks after changes.

**How to run:**

```bash
cd /path/to/coderag-go
go test -bench=. -benchmem ./... 2>&1 | tee benchmarks/run-$(date +%Y%m%d-%H%M).txt
```

Save new results under `benchmarks/` or append a "Results (YYYY-MM-DD)" section below.

---

## Environment (baseline)

- **Date:** 2026-02-26
- **Go:** (run `go version`)
- **OS/arch:** darwin/arm64 (Apple M4)
- **Command:** `go test -bench=. -benchmem ./...`

---

## Results (2026-02-26 baseline)

```
goos: darwin
goarch: arm64
pkg: github.com/matperez/coderag-go/internal/indexer
cpu: Apple M4
BenchmarkIndexer_Index-10    	      15	  76876528 ns/op	 1430388 B/op	   28684 allocs/op

goos: darwin
goarch: arm64
pkg: github.com/matperez/coderag-go/internal/search
cpu: Apple M4
BenchmarkBuildIndex-10    	     883	   1227309 ns/op	  935657 B/op	   28618 allocs/op
BenchmarkSearch-10        	   26910	     44498 ns/op	   41880 B/op	     424 allocs/op

goos: darwin
goarch: arm64
pkg: github.com/matperez/coderag-go/internal/tokenizer
cpu: Apple M4
BenchmarkTokenize-10         	 1820487	       633.5 ns/op	     829 B/op	      31 allocs/op
BenchmarkTokenize_code-10    	  163976	      7739 ns/op	    7278 B/op	     236 allocs/op
BenchmarkTokenize_long-10    	    1376	    831467 ns/op	  689953 B/op	   24790 allocs/op
```

### Summary

| Package     | Benchmark               | ns/op   | B/op   | allocs/op |
|------------|--------------------------|---------|--------|-----------|
| indexer    | BenchmarkIndexer_Index   | 76.9M   | 1.43M  | 28684     |
| search     | BenchmarkBuildIndex      | 1.23M   | 936K   | 28618     |
| search     | BenchmarkSearch          | 44.5K   | 42K    | 424       |
| tokenizer  | BenchmarkTokenize        | 633.5   | 829    | 31        |
| tokenizer  | BenchmarkTokenize_code   | 7739    | 7278   | 236       |
| tokenizer  | BenchmarkTokenize_long   | 831.5K  | 690K   | 24790     |

---

## Later runs

Add new sections below when you re-run benchmarks, e.g.:

### Results (YYYY-MM-DD — brief description)

```
<paste go test -bench=. -benchmem output>
```

| Package | Benchmark | ns/op | B/op | allocs/op |
|---------|-----------|-------|------|------------|
| ...     | ...       | ...   | ...  | ...        |
