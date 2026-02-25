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
	Type      string
	StartLine int
	EndLine   int
	Metadata  string // JSON, optional
	TokenCount int   // for BM25 length normalization
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
}
