package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcher_createChangeRemove(t *testing.T) {
	dir := t.TempDir()
	w := New(Options{
		Root:         dir,
		UseGitignore: false,
		Debounce:     20 * time.Millisecond,
	})
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Create file
	fpath := filepath.Join(dir, "a.go")
	if err := os.WriteFile(fpath, []byte("package p\n"), 0644); err != nil {
		t.Fatal(err)
	}
	expectEvent(t, w, "a.go", Add, 2*time.Second)

	// Change file
	if err := os.WriteFile(fpath, []byte("package p\nfunc F() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	expectEvent(t, w, "a.go", Change, 2*time.Second)

	// Remove file
	if err := os.Remove(fpath); err != nil {
		t.Fatal(err)
	}
	expectEvent(t, w, "a.go", Remove, 2*time.Second)
}

func expectEvent(t *testing.T, w *Watcher, relPath string, op Op, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case e, ok := <-w.Events():
			if !ok {
				t.Fatal("events channel closed")
			}
			if e.Path == relPath && e.Op == op {
				return
			}
			t.Logf("got event %s %v (waiting for %s %v)", e.Path, e.Op, relPath, op)
		case <-deadline:
			t.Fatalf("timeout waiting for event %s %v", relPath, op)
		}
	}
}
