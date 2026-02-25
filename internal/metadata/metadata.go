package metadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const metadataFilename = "metadata.json"

// Time format matching .coderag (ISO 8601 with milliseconds, UTC).
const timeFormat = "2006-01-02T15:04:05.000Z"

// ProjectMetadata holds project metadata written next to the index database.
type ProjectMetadata struct {
	Path           string `json:"path"`
	Name           string `json:"name"`
	CreatedAt      string `json:"createdAt"`
	LastAccessedAt string `json:"lastAccessedAt"`
}

func nowUTC() string {
	return time.Now().UTC().Format(timeFormat)
}

// Ensure creates or updates metadata.json in dataDir. If the file is missing,
// it writes a new one with path=rootPath, name=".", and current timestamps.
// If it exists, it reads it, updates only LastAccessedAt, and writes back.
// Errors (e.g. read/parse) on existing file result in overwriting with fresh metadata.
func Ensure(dataDir, rootPath string) error {
	path := filepath.Join(dataDir, metadataFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return write(path, &ProjectMetadata{
				Path:           rootPath,
				Name:           ".",
				CreatedAt:      nowUTC(),
				LastAccessedAt: nowUTC(),
			})
		}
		return err
	}
	var m ProjectMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		return write(path, &ProjectMetadata{
			Path:           rootPath,
			Name:           ".",
			CreatedAt:      nowUTC(),
			LastAccessedAt: nowUTC(),
		})
	}
	m.LastAccessedAt = nowUTC()
	return write(path, &m)
}

func write(path string, m *ProjectMetadata) error {
	data, err := json.MarshalIndent(m, "", "")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
