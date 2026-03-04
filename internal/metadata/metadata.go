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
	Path                  string `json:"path"`
	Name                  string `json:"name"`
	CreatedAt             string `json:"createdAt"`
	LastAccessedAt        string `json:"lastAccessedAt"`
	IDFRebuildCompletedAt string `json:"idfRebuildCompletedAt"` // non-empty = last RebuildIDFAndTfidf() completed successfully
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
			now := nowUTC()
			return write(path, &ProjectMetadata{
				Path:           rootPath,
				Name:           ".",
				CreatedAt:      now,
				LastAccessedAt: now,
			})
		}
		return err
	}
	var m ProjectMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		now := nowUTC()
		return write(path, &ProjectMetadata{
			Path:           rootPath,
			Name:           ".",
			CreatedAt:      now,
			LastAccessedAt: now,
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

// Read reads metadata.json from dataDir. Returns (nil, nil) if the file does not exist.
func Read(dataDir string) (*ProjectMetadata, error) {
	path := filepath.Join(dataDir, metadataFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m ProjectMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Write writes metadata to dataDir/metadata.json.
func Write(dataDir string, m *ProjectMetadata) error {
	path := filepath.Join(dataDir, metadataFilename)
	return write(path, m)
}

// SetIDFRebuildCompleted sets IDFRebuildCompletedAt to the current time in metadata. No-op if file does not exist.
func SetIDFRebuildCompleted(dataDir string) error {
	m, err := Read(dataDir)
	if err != nil || m == nil {
		return err
	}
	m.IDFRebuildCompletedAt = nowUTC()
	return Write(dataDir, m)
}

// ClearIDFRebuildCompleted clears IDFRebuildCompletedAt in metadata. No-op if file does not exist.
func ClearIDFRebuildCompleted(dataDir string) error {
	m, err := Read(dataDir)
	if err != nil || m == nil {
		return err
	}
	m.IDFRebuildCompletedAt = ""
	return Write(dataDir, m)
}
