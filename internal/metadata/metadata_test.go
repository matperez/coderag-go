package metadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsure_createsFileWhenMissing(t *testing.T) {
	dir := t.TempDir()
	rootPath := "/tmp/myproject"

	err := Ensure(dir, rootPath)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	path := filepath.Join(dir, metadataFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var got ProjectMetadata
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	want := ProjectMetadata{
		Path:           rootPath,
		Name:           ".",
		CreatedAt:      got.CreatedAt,
		LastAccessedAt: got.LastAccessedAt,
	}
	if got.CreatedAt == "" || got.LastAccessedAt == "" {
		t.Fatalf("timestamps must be set: got %+v", got)
	}
	if want.CreatedAt != want.LastAccessedAt {
		t.Fatalf("on create CreatedAt and LastAccessedAt must match: got %+v", got)
	}
	if got != want {
		t.Errorf("metadata:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestEnsure_updatesOnlyLastAccessedAtWhenFileExists(t *testing.T) {
	dir := t.TempDir()
	rootPath := "/tmp/myproject"

	err := Ensure(dir, rootPath)
	if err != nil {
		t.Fatalf("first Ensure() error = %v", err)
	}
	path := filepath.Join(dir, metadataFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	var first ProjectMetadata
	if err := json.Unmarshal(data, &first); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	// Ensure second call gets a different LastAccessedAt (format has millisecond precision)
	time.Sleep(2 * time.Millisecond)

	err = Ensure(dir, rootPath)
	if err != nil {
		t.Fatalf("second Ensure() error = %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after second Ensure error = %v", err)
	}
	var got ProjectMetadata
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal second error = %v", err)
	}
	want := ProjectMetadata{
		Path:           first.Path,
		Name:           first.Name,
		CreatedAt:      first.CreatedAt,
		LastAccessedAt: got.LastAccessedAt,
	}
	if got.LastAccessedAt == first.LastAccessedAt {
		t.Fatalf("LastAccessedAt must change: %q", got.LastAccessedAt)
	}
	if got != want {
		t.Errorf("metadata after second Ensure:\n  got  %+v\n  want %+v", got, want)
	}
}
