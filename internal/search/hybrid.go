package search

import (
	"context"

	"github.com/matperez/coderag-go/internal/embeddings"
	"github.com/matperez/coderag-go/internal/storage"
	"github.com/matperez/coderag-go/internal/vectorstore"
)

// HybridOpts configures hybrid search. Nil VecStore or Embedder means BM25-only.
type HybridOpts struct {
	VecStore   vectorstore.Store
	Embedder   embeddings.Provider
	BM25Weight float64 // 0..1, vector weight = 1 - BM25Weight
}

// HybridFromStorage runs BM25 and optionally vector search, then fuses results by score.
// If opts is nil or VecStore/Embedder is nil, returns BM25-only via SearchFromStorage.
func HybridFromStorage(
	ctx context.Context,
	query string,
	st storage.Storage,
	idf map[string]float64,
	candidates []StorageCandidate,
	avgDocLength float64,
	limit int,
	opts *HybridOpts,
) ([]Result, error) {
	if opts == nil || opts.VecStore == nil || opts.Embedder == nil {
		return SearchFromStorage(query, idf, candidates, avgDocLength, limit), nil
	}
	vecWeight := 1.0 - opts.BM25Weight
	if vecWeight < 0 {
		vecWeight = 0.5
	}
	bm25Weight := opts.BM25Weight
	if bm25Weight < 0 {
		bm25Weight = 0.5
	}

	// BM25 top 2*limit
	if limit <= 0 {
		return nil, nil
	}
	bm25Results := SearchFromStorage(query, idf, candidates, avgDocLength, limit*2)

	queryVec, err := opts.Embedder.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, err
	}
	vecResults, err := opts.VecStore.Search(ctx, queryVec, limit*2)
	if err != nil {
		return nil, err
	}

	// chunkID -> chunk info (from candidates or storage)
	infoByID := make(map[int64]struct {
		path      string
		content   string
		startLine int
		endLine   int
	})
	for _, c := range candidates {
		infoByID[c.ChunkID] = struct {
			path      string
			content   string
			startLine int
			endLine   int
		}{c.FilePath, c.Content, c.StartLine, c.EndLine}
	}

	// BM25 score by chunk ID
	bm25ByID := make(map[int64]float64)
	var maxBM25 float64
	for _, r := range bm25Results {
		bm25ByID[r.ChunkID] = r.Score
		if r.Score > maxBM25 {
			maxBM25 = r.Score
		}
	}

	// Vector score by chunk ID (reciprocal rank: 1, 1/2, 1/3, ...)
	vecByID := make(map[int64]float64)
	for i, v := range vecResults {
		rank := float64(i + 1)
		vecByID[v.ChunkID] = 1.0 / rank
		if infoByID[v.ChunkID].path == "" && v.ChunkID != 0 {
			ci, _ := st.GetChunk(v.ChunkID)
			if ci != nil {
				infoByID[v.ChunkID] = struct {
					path      string
					content   string
					startLine int
					endLine   int
				}{ci.Path, ci.Content, ci.StartLine, ci.EndLine}
			}
		}
	}
	maxVec := 1.0
	if len(vecResults) > 0 {
		maxVec = 1.0 // best rank is 1
	}

	// All chunk IDs
	seen := make(map[int64]bool)
	var allIDs []int64
	for id := range bm25ByID {
		if !seen[id] {
			seen[id] = true
			allIDs = append(allIDs, id)
		}
	}
	for id := range vecByID {
		if !seen[id] {
			seen[id] = true
			allIDs = append(allIDs, id)
		}
	}

	// Combined score: normalize BM25 and vector to [0,1], then weighted sum
	type scoredID struct {
		id     int64
		score  float64
		terms  []string
		bm25Score float64
	}
	var scored []scoredID
	for _, id := range allIDs {
		normB := 0.0
		if maxBM25 > 0 {
			normB = bm25ByID[id] / maxBM25
		}
		normV := 0.0
		if maxVec > 0 {
			normV = vecByID[id] / maxVec
		}
		combined := bm25Weight*normB + vecWeight*normV
		var terms []string
		for _, r := range bm25Results {
			if r.ChunkID == id {
				terms = r.MatchedTerms
				break
			}
		}
		scored = append(scored, scoredID{id, combined, terms, bm25ByID[id]})
	}
	// Sort by combined score desc
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	if limit > len(scored) {
		limit = len(scored)
	}
	out := make([]Result, 0, limit)
	for i := 0; i < limit; i++ {
		s := scored[i]
		info := infoByID[s.id]
		if info.path == "" {
			ci, _ := st.GetChunk(s.id)
			if ci != nil {
				info.path = ci.Path
				info.content = ci.Content
				info.startLine = ci.StartLine
				info.endLine = ci.EndLine
			}
		}
		out = append(out, Result{
			URI:          "file://" + info.path,
			Score:        s.score,
			MatchedTerms: s.terms,
			Content:      info.content,
			StartLine:    info.startLine,
			EndLine:      info.endLine,
			ChunkID:      s.id,
		})
	}
	return out, nil
}
