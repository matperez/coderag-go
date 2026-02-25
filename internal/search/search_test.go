package search

import (
	"strconv"
	"testing"
)

func TestBuildIndex_and_Search(t *testing.T) {
	docs := []struct{ URI, Content string }{
		{"file://a.go", "func getUserById(id string) { return user }"},
		{"file://b.go", "func getOrderByID(id int) { return order }"},
		{"file://c.go", "authenticate user login"},
	}
	idx := BuildIndex(docs)
	if idx.TotalDocuments != 3 {
		t.Errorf("TotalDocuments = %d", idx.TotalDocuments)
	}
	if len(idx.Documents) != 3 {
		t.Errorf("len(Documents) = %d", len(idx.Documents))
	}

	results := Search("get user", idx, 10)
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'get user'")
	}
	if results[0].URI != "file://a.go" && results[0].URI != "file://b.go" {
		t.Errorf("top result URI = %s", results[0].URI)
	}
	found := false
	for _, r := range results {
		if r.URI == "file://a.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("file://a.go not in results: %v", results)
	}
}

func TestSearch_emptyQuery(t *testing.T) {
	idx := BuildIndex([]struct{ URI, Content string }{{"u", "content"}})
	results := Search("", idx, 5)
	if results != nil {
		t.Errorf("empty query should return nil, got %v", results)
	}
	results = Search("   ", idx, 5)
	if len(results) != 0 {
		t.Errorf("whitespace query: got %v", results)
	}
}

func TestSearch_noMatch(t *testing.T) {
	docs := []struct{ URI, Content string }{
		{"file://a.go", "hello world"},
	}
	idx := BuildIndex(docs)
	results := Search("xyznonexistent", idx, 5)
	if len(results) != 0 {
		t.Errorf("expected no results, got %v", results)
	}
}

func TestSearch_limit(t *testing.T) {
	docs := []struct{ URI, Content string }{
		{"f1", "term a"},
		{"f2", "term a"},
		{"f3", "term a"},
	}
	idx := BuildIndex(docs)
	results := Search("term", idx, 2)
	if len(results) != 2 {
		t.Errorf("limit 2: got %d results", len(results))
	}
}

func TestSearchFromStorage(t *testing.T) {
	idf := map[string]float64{"user": 1.5, "get": 1.2}
	candidates := []StorageCandidate{
		{FilePath: "a.go", TokenCount: 10, Terms: map[string]TermScore{
			"user": {0.3, 0.5, 2}, "get": {0.2, 0.3, 1},
		}},
		{FilePath: "b.go", TokenCount: 5, Terms: map[string]TermScore{
			"user": {0.5, 0.8, 1},
		}},
	}
	// Set content/lines for first candidate so Result has them
	candidates[0].Content = "func GetUser()"
	candidates[0].StartLine, candidates[0].EndLine = 1, 1
	candidates[1].Content = "user id"
	candidates[1].StartLine, candidates[1].EndLine = 2, 2
	avgLen := 7.5
	results := SearchFromStorage("get user", idf, candidates, avgLen, 10)
	if len(results) != 2 {
		t.Fatalf("got %d results", len(results))
	}
	if results[0].URI != "file://a.go" {
		t.Errorf("top result: %s", results[0].URI)
	}
}

// benchmarkDocs returns N document structs with short code-like content.
func benchmarkDocs(N int) []struct{ URI, Content string } {
	docs := make([]struct{ URI, Content string }, N)
	templates := []string{
		"func getUserById(id string) { return user }",
		"func getOrderByID(id int) { return order }",
		"authenticate user login",
		"type Repository interface { Find(id string) (*Entity, error) }",
		"// handler processes request and returns response",
	}
	for i := 0; i < N; i++ {
		docs[i] = struct{ URI, Content string }{
			URI:     "file://doc" + strconv.Itoa(i) + ".go",
			Content: templates[i%len(templates)],
		}
	}
	return docs
}

func BenchmarkBuildIndex(b *testing.B) {
	docs := benchmarkDocs(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildIndex(docs)
	}
}

func BenchmarkSearch(b *testing.B) {
	docs := benchmarkDocs(500)
	idx := BuildIndex(docs)
	query := "get user"
	limit := 10
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Search(query, idx, limit)
	}
}
