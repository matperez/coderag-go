# Proposal for go-tree-sitter: clear Tree.cache on Close()

This document summarizes a suggested change to [github.com/smacker/go-tree-sitter](https://github.com/smacker/go-tree-sitter) for a future PR. It builds on the findings in [TREE_SITTER_MEMORY_VERIFICATION.md](TREE_SITTER_MEMORY_VERIFICATION.md).

## Related upstream issue

- **Issue [#181](https://github.com/smacker/go-tree-sitter/issues/181):** *unexpected memory behaviour when relying on runtime.SetFinalizer vs explicit Close*  
  The issue argues that docs should recommend calling `Close()` explicitly, because finalizers may not run in time when most allocations are in C (tree-sitter) and the Go heap does not grow, so GC is not triggered. We follow that recommendation and call `tree.Close()` after every file. The proposal below addresses a further problem that remains even when `Close()` is called.

- **PR [#182](https://github.com/smacker/go-tree-sitter/pull/182):** updates documentation to clarify that users should not rely on SetFinalizer and should call `Close()`.

## Problem

In `bindings.go`, `Tree` has a node cache:

```go
type Tree struct {
	*BaseTree
	p     *Parser
	cache map[C.TSNode]*Node
}
```

- Every access to a node (RootNode, Child, Type(), etc.) goes through `cachedNode(ptr)`, which creates a `*Node` and stores it in `t.cache` if not already present.
- `BaseTree.Close()` only runs `C.ts_tree_delete(t.c)` and sets `t.isClosed = true`. It does **not** clear `Tree.cache`.

So when the user calls `tree.Close()`, the C tree is freed, but the Go map and all `*Node` structs remain in the heap until the next GC. In workloads that parse many trees (e.g. indexing thousands of files), this leads to high `alloc_space` and peak memory attributed to `cachedNode`, even though the application correctly calls `Close()` and holds no references to the tree or nodes after that.

## Proposed change

When `Close()` is invoked on a `Tree`, clear the cache so the Go heap is released immediately instead of waiting for GC:

- If `Tree` overrides or wraps `Close()`, then in that `Close()` (after calling the base implementation): set `t.cache = nil` or `t.cache = make(map[C.TSNode]*Node)` so the map and all `*Node` entries become unreachable.
- If only `BaseTree.Close()` exists, the library could add a `Tree.Close()` that clears `t.cache` and then calls `BaseTree.Close()`.

This keeps the existing API and semantics; it only makes `Close()` release both C and Go memory as soon as it is called.

## Evidence

In our project we index a large codebase: we call `tree.Close()` after extracting the data we need from each file (see [TREE_SITTER_MEMORY_VERIFICATION.md](TREE_SITTER_MEMORY_VERIFICATION.md)). Heap profiles still show a large share of `alloc_space` in `(*Tree).cachedNode` because the cache is not cleared on `Close()`. Clearing the cache in the library would reduce peak Go heap usage without requiring callers to run `runtime.GC()` or change their usage pattern.

## Next steps

- Open an issue or PR in [smacker/go-tree-sitter](https://github.com/smacker/go-tree-sitter) describing the above (optionally linking to this doc or the verification doc).
- Implement the change (clear `Tree.cache` when `Close()` is called) and add a test or note in the PR.
