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
		Path:                  rootPath,
		Name:                  ".",
		CreatedAt:             got.CreatedAt,
		LastAccessedAt:        got.LastAccessedAt,
		IDFRebuildCompletedAt: "",
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
		Path:                  first.Path,
		Name:                  first.Name,
		CreatedAt:             first.CreatedAt,
		LastAccessedAt:        got.LastAccessedAt,
		IDFRebuildCompletedAt: first.IDFRebuildCompletedAt,
	}
	if got.LastAccessedAt == first.LastAccessedAt {
		t.Fatalf("LastAccessedAt must change: %q", got.LastAccessedAt)
	}
	if got != want {
		t.Errorf("metadata after second Ensure:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestRead_missingFile(t *testing.T) {
	dir := t.TempDir()
	m, err := Read(dir)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if m != nil {
		t.Errorf("Read() = %+v, want nil", m)
	}
}

func TestRead_write_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := "/tmp/proj"
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	m := &ProjectMetadata{
		Path:                  path,
		Name:                  ".",
		CreatedAt:             now,
		LastAccessedAt:        now,
		IDFRebuildCompletedAt: now,
	}
	if err := Write(dir, m); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got == nil {
		t.Fatal("Read() = nil")
	}
	if got.Path != path || got.IDFRebuildCompletedAt != now {
		t.Errorf("Read() = %+v", got)
	}
}

func TestSetIDFRebuildCompleted_noFile(t *testing.T) {
	dir := t.TempDir()
	if err := SetIDFRebuildCompleted(dir); err != nil {
		t.Fatalf("SetIDFRebuildCompleted() error = %v", err)
	}
	// No file existed, so nothing to do - no file created
	m, _ := Read(dir)
	if m != nil {
		t.Errorf("expected no file, got %+v", m)
	}
}

func TestSetIDFRebuildCompleted_and_ClearIDFRebuildCompleted(t *testing.T) {
	dir := t.TempDir()
	if err := Ensure(dir, "/x"); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if err := SetIDFRebuildCompleted(dir); err != nil {
		t.Fatalf("SetIDFRebuildCompleted() error = %v", err)
	}
	m, _ := Read(dir)
	if m == nil || m.IDFRebuildCompletedAt == "" {
		t.Fatalf("after Set: %+v", m)
	}
	if err := ClearIDFRebuildCompleted(dir); err != nil {
		t.Fatalf("ClearIDFRebuildCompleted() error = %v", err)
	}
	m, _ = Read(dir)
	if m == nil || m.IDFRebuildCompletedAt != "" {
		t.Errorf("after Clear: %+v", m)
	}
}

func TestEnsure_preservesIDFRebuildCompletedAt(t *testing.T) {
	dir := t.TempDir()
	if err := Ensure(dir, "/p"); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if err := SetIDFRebuildCompleted(dir); err != nil {
		t.Fatalf("SetIDFRebuildCompleted() error = %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := Ensure(dir, "/p"); err != nil {
		t.Fatalf("second Ensure() error = %v", err)
	}
	m, _ := Read(dir)
	if m == nil || m.IDFRebuildCompletedAt == "" {
		t.Errorf("Ensure must not clear IDFRebuildCompletedAt: got %+v", m)
	}
}
