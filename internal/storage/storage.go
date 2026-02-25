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
	Content   string
	Type      string
	StartLine int
	EndLine   int
	Metadata  string // JSON, optional
}

// Storage persists indexed files and chunks.
type Storage interface {
	StoreFile(file File) error
	StoreChunks(filePath string, chunks []Chunk) error
	GetFile(path string) (*File, error)
	ListFiles() ([]string, error)
	FileCount() (int, error)
	ChunkCount() (int, error)
}
