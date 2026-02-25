package watcher

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	gitignore "github.com/monochromegane/go-gitignore"
)

// Op is the type of file event.
type Op int

const (
	Add Op = iota
	Change
	Remove
)

// Event is a file system event (path relative to watch root or absolute).
type Event struct {
	Path string
	Op   Op
}

// Options configures the watcher.
type Options struct {
	Root         string        // directory to watch
	Extensions   []string      // if non-nil, only emit for these extensions
	UseGitignore bool          // respect .gitignore
	Debounce     time.Duration // coalesce events (default 100ms)
}

// Watcher watches a directory tree and emits filtered, deduplicated, debounced events.
type Watcher struct {
	opts    Options
	matcher gitignore.IgnoreMatcher
	skipDir map[string]bool
	mu      sync.Mutex
	closed  bool
	evCh    chan Event
	done    chan struct{}
}

// New creates a watcher. Call Start to begin watching.
func New(opts Options) *Watcher {
	if opts.Debounce <= 0 {
		opts.Debounce = 100 * time.Millisecond
	}
	w := &Watcher{
		opts:    opts,
		skipDir: map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true},
		evCh:    make(chan Event, 64),
		done:    make(chan struct{}),
	}
	if opts.UseGitignore {
		ignorePath := filepath.Join(opts.Root, ".gitignore")
		w.matcher, _ = gitignore.NewGitIgnore(ignorePath, opts.Root)
	}
	if w.matcher == nil {
		w.matcher = gitignore.DummyIgnoreMatcher(false)
	}
	return w
}

// Start starts watching. Events are sent to Events(). Call Close to stop.
func (w *Watcher) Start() error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	// Add root and all subdirs (skip excluded)
	addDir := func(dir string) error {
		return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(w.opts.Root, path)
			base := filepath.Base(path)
			if w.skipDir[base] || w.matcher.Match(path, true) {
				if rel != "." {
					return filepath.SkipDir
				}
			}
			return fw.Add(path)
		})
	}
	if err := addDir(w.opts.Root); err != nil {
		_ = fw.Close()
		return err
	}
	pending := make(map[string]Op)
	var debounceTimer *time.Timer
	flush := func() {
		w.mu.Lock()
		for path, op := range pending {
			w.evCh <- Event{Path: path, Op: op}
		}
		for k := range pending {
			delete(pending, k)
		}
		w.mu.Unlock()
	}
	go func() {
		defer func() { _ = fw.Close() }()
		for {
			select {
			case <-w.done:
				flush()
				close(w.evCh)
				return
			case e, ok := <-fw.Events:
				if !ok {
					return
				}
				path := e.Name
				// Ignore dirs for "file" events; we only care about file add/change/remove
				// Chmod is often emitted for dirs too
				pathAbs, _ := filepath.Abs(path)
				rootAbs, _ := filepath.Abs(w.opts.Root)
				rel, err := filepath.Rel(rootAbs, pathAbs)
				if err != nil {
					rel = path
				}
				var op Op
				switch {
				case e.Op&fsnotify.Create == fsnotify.Create:
					op = Add
				case e.Op&fsnotify.Write == fsnotify.Write:
					op = Change
				case e.Op&fsnotify.Remove == fsnotify.Remove:
					op = Remove
				default:
					continue
				}
				// Filter: if it's a directory, add watch and skip emit (we don't emit dir events)
				if op == Add {
					if fi, err := os.Stat(path); err == nil && fi.IsDir() {
						_ = addDir(path)
						continue
					}
				}
				if w.opts.Extensions != nil {
					ext := filepath.Ext(path)
					found := false
					for _, e := range w.opts.Extensions {
						if ext == e {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				}
				if op != Remove && w.matcher.Match(path, false) {
					continue
				}
				w.mu.Lock()
				pending[rel] = op
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(w.opts.Debounce, func() {
					flush()
				})
				w.mu.Unlock()
			case _, ok := <-fw.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return nil
}

// Events returns the channel of events. Closed when Close is called.
func (w *Watcher) Events() <-chan Event {
	return w.evCh
}

// Close stops the watcher and closes the event channel.
func (w *Watcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()
	close(w.done)
	return nil
}
