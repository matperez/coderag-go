package storage

// File represents an indexed file.
type File struct {
	Path      string
	Content   string
	Hash      string
	Size      int64
	Mtime     int64
	Language  string
	IndexedAt int64
}

// Chunk represents a chunk of a file (for storage).
type Chunk struct {
	Content    string
	Type       string
	StartLine  int
	EndLine    int
	Metadata   string  // JSON, optional
	TokenCount int     // for BM25 length normalization
	Magnitude  float64 // pre-computed TF-IDF magnitude
}

// VectorRow is one term's TF-IDF data for a chunk.
type VectorRow struct {
	Term    string
	TF      float64
	TFIDF   float64
	RawFreq int
}

// Storage persists indexed files and chunks.
type Storage interface {
	StoreFile(file File) error
	StoreChunks(filePath string, chunks []Chunk) (chunkIDs []int64, err error)
	StoreChunkVectors(chunkID int64, rows []VectorRow) error
	GetFile(path string) (*File, error)
	ListFiles() ([]string, error)
	FileCount() (int, error)
	ChunkCount() (int, error)
	// DeleteFile removes the file and all its chunks and vectors from the index.
	DeleteFile(path string) error
	// DocFreqs returns the number of distinct chunks containing each term (for IDF).
	DocFreqs(terms []string) (map[string]int, error)
	// Search support: load chunks that contain any of the given terms.
	SearchCandidates(terms []string) (idf map[string]float64, candidates []SearchCandidate, err error)
	// GetChunk returns path, content, and line range for a chunk by ID. Returns nil if not found.
	GetChunk(chunkID int64) (*ChunkInfo, error)
	// ListChunkIDsByFile returns chunk IDs for the given file path (for vector store cleanup).
	ListChunkIDsByFile(path string) ([]int64, error)
	// RebuildIDFAndTfidf recomputes IDF from document_vectors, updates tfidf in document_vectors, and chunk magnitudes.
	RebuildIDFAndTfidf() error
}

// ChunkInfo is the minimal chunk data for search result resolution.
type ChunkInfo struct {
	Path      string
	Content   string
	StartLine int
	EndLine   int
}

// SearchCandidate is a chunk with its term vectors for BM25 scoring.
type SearchCandidate struct {
	ChunkID    int64
	FilePath   string
	Content    string
	StartLine  int
	EndLine    int
	TokenCount int
	Magnitude  float64
	Terms      map[string]VectorRow // term -> tf/tfidf/raw_freq
}
