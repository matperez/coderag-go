# Benchmark runs

Optional: save raw output here for comparison.

```bash
mkdir -p benchmarks
go test -bench=. -benchmem ./... 2>&1 | tee benchmarks/run-$(date +%Y%m%d-%H%M).txt
```

Summary and baseline are in [docs/BENCHMARKS.md](../docs/BENCHMARKS.md).
