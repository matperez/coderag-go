package search

import (
	"math"
	"time"

	"github.com/matperez/coderag-go/internal/tokenizer"
)

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// DocumentVector holds term weights and magnitude for one document (chunk).
type DocumentVector struct {
	URI       string
	Terms     map[string]float64 // term -> TF-IDF (for display) or used in scoring
	RawTerms  map[string]int     // term -> raw frequency (for BM25)
	Magnitude float64
	TokenCount int
}

// SearchIndex is the in-memory index for BM25 search.
type SearchIndex struct {
	Documents      []DocumentVector
	IDF            map[string]float64
	TotalDocuments int
	AvgDocLength   float64
	Metadata       struct {
		GeneratedAt string
		Version     string
	}
}

// Result is a single search result.
type Result struct {
	URI         string
	Score       float64
	MatchedTerms []string
}

// BuildIndex builds a search index from documents (uri, content). Uses tokenizer for term extraction.
func BuildIndex(documents []struct{ URI, Content string }) *SearchIndex {
	tok := tokenizer.New()
	docTerms := make([]map[string]int, len(documents))
	for i, doc := range documents {
		tokens := tok.Tokenize(doc.Content)
		freq := make(map[string]int)
		for _, t := range tokens {
			freq[t]++
		}
		docTerms[i] = freq
	}
	N := float64(len(documents))
	docFreq := make(map[string]int) // number of documents containing each term
	for _, freq := range docTerms {
		for term := range freq {
			docFreq[term]++
		}
	}
	idf := make(map[string]float64)
	for term, df := range docFreq {
		idf[term] = math.Log((N+1)/float64(df+1)) + 1
	}
	avgLen := 0.0
	vecs := make([]DocumentVector, len(documents))
	for i, doc := range documents {
		freq := docTerms[i]
		tf := make(map[string]float64)
		var total float64
		for _, c := range freq {
			total += float64(c)
		}
		for term, c := range freq {
			if total > 0 {
				tf[term] = float64(c) / total
			}
		}
		tfidf := make(map[string]float64)
		for term, tfScore := range tf {
			tfidf[term] = tfScore * idf[term]
		}
		mag := 0.0
		for _, v := range tfidf {
			mag += v * v
		}
		mag = math.Sqrt(mag)
		tokenCount := 0
		for _, c := range freq {
			tokenCount += c
		}
		avgLen += float64(tokenCount)
		vecs[i] = DocumentVector{
			URI:        doc.URI,
			Terms:      tfidf,
			RawTerms:   freq,
			Magnitude:  mag,
			TokenCount: tokenCount,
		}
	}
	if N > 0 {
		avgLen /= N
	}
	if avgLen < 1 {
		avgLen = 1
	}
	return &SearchIndex{
		Documents:      vecs,
		IDF:            idf,
		TotalDocuments: len(documents),
		AvgDocLength:   avgLen,
		Metadata: struct {
			GeneratedAt string
			Version     string
		}{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Version:     "1.0.0",
		},
	}
}

// Search runs BM25 search over the index. Returns results sorted by score descending.
func Search(query string, index *SearchIndex, limit int) []Result {
	if index == nil || limit <= 0 {
		return nil
	}
	tokens := tokenizer.Tokenize(query)
	if len(tokens) == 0 {
		return nil
	}
	// Dedupe query terms
	seen := make(map[string]bool)
	var queryTerms []string
	for _, t := range tokens {
		if !seen[t] {
			seen[t] = true
			queryTerms = append(queryTerms, t)
		}
	}
	avgLen := index.AvgDocLength
	if avgLen < 1 {
		avgLen = 1
	}
	type scored struct {
		uri    string
		score  float64
		terms  []string
	}
	var results []scored
	for _, doc := range index.Documents {
		var matched []string
		for _, q := range queryTerms {
			if doc.RawTerms[q] > 0 {
				matched = append(matched, q)
			}
		}
		if len(matched) == 0 {
			continue
		}
		docLen := float64(doc.TokenCount)
		if docLen < 1 {
			docLen = 1
		}
		score := 0.0
		for _, term := range matched {
			tf := float64(doc.RawTerms[term])
			idf := index.IDF[term]
			num := tf * (bm25K1 + 1)
			denom := tf + bm25K1*(1-bm25B+bm25B*docLen/avgLen)
			score += idf * (num / denom)
		}
		results = append(results, scored{doc.URI, score, matched})
	}
	// Sort by score descending (simple bubble or sort.Slice)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	if limit > len(results) {
		limit = len(results)
	}
	out := make([]Result, limit)
	for i := 0; i < limit; i++ {
		out[i] = Result{
			URI:          results[i].uri,
			Score:        results[i].score,
			MatchedTerms: results[i].terms,
		}
	}
	return out
}

// TermScore holds TF/TFIDF/RawFreq for one term in a chunk.
type TermScore struct {
	TF      float64
	TFIDF   float64
	RawFreq int
}

// StorageCandidate is one chunk's data for low-memory BM25 (from storage.SearchCandidates).
type StorageCandidate struct {
	FilePath   string
	TokenCount int
	Terms      map[string]TermScore
}

// SearchFromStorage runs BM25 search using storage-backed candidates (low-memory).
func SearchFromStorage(
	query string,
	idf map[string]float64,
	candidates []StorageCandidate,
	avgDocLength float64,
	limit int,
) []Result {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	tokens := tokenizer.Tokenize(query)
	seen := make(map[string]bool)
	var queryTerms []string
	for _, t := range tokens {
		if !seen[t] {
			seen[t] = true
			queryTerms = append(queryTerms, t)
		}
	}
	if len(queryTerms) == 0 {
		return nil
	}
	if avgDocLength < 1 {
		avgDocLength = 1
	}
	type scored struct {
		uri   string
		score float64
		terms []string
	}
	var results []scored
	for _, c := range candidates {
		var matched []string
		for _, q := range queryTerms {
			if _, ok := c.Terms[q]; ok {
				matched = append(matched, q)
			}
		}
		if len(matched) == 0 {
			continue
		}
		docLen := float64(c.TokenCount)
		if docLen < 1 {
			docLen = 1
		}
		score := 0.0
		for _, term := range matched {
			r := c.Terms[term]
			tf := float64(r.RawFreq)
			idfVal := idf[term]
			num := tf * (bm25K1 + 1)
			denom := tf + bm25K1*(1-bm25B+bm25B*docLen/avgDocLength)
			score += idfVal * (num / denom)
		}
		results = append(results, scored{"file://" + c.FilePath, score, matched})
	}
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	if limit > len(results) {
		limit = len(results)
	}
	out := make([]Result, limit)
	for i := 0; i < limit; i++ {
		out[i] = Result{URI: results[i].uri, Score: results[i].score, MatchedTerms: results[i].terms}
	}
	return out
}
