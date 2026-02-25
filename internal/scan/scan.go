package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/monochromegane/go-gitignore"
)

// FileEntry is a file path with metadata for indexing.
type FileEntry struct {
	Path  string
	Size  int64
	Mtime int64
}

// Options configures the scanner.
type Options struct {
	MaxFileSize  int64    // skip files larger than this (0 = no limit)
	Extensions   []string // if non-nil, only include these extensions (e.g. ".go", ".ts")
	UseGitignore bool     // if true, load and respect .gitignore from root
}

// Scan walks root and returns file entries, respecting gitignore and options.
func Scan(root string, opts Options) ([]FileEntry, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var matcher gitignore.IgnoreMatcher
	if opts.UseGitignore {
		ignorePath := filepath.Join(root, ".gitignore")
		if f, err := os.Open(ignorePath); err == nil {
			_ = f.Close()
			matcher, _ = gitignore.NewGitIgnore(ignorePath, root)
		}
	}
	if matcher == nil {
		matcher = gitignore.DummyIgnoreMatcher(false)
	}
	// Always ignore .git and common build dirs
	skipDir := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
	}
	var entries []FileEntry
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		base := filepath.Base(path)
		if d.IsDir() {
			if skipDir[base] || matcher.Match(path, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if matcher.Match(path, false) {
			return nil
		}
		if opts.Extensions != nil {
			ext := strings.ToLower(filepath.Ext(path))
			found := false
			for _, e := range opts.Extensions {
				if ext == e {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		size := info.Size()
		if opts.MaxFileSize > 0 && size > opts.MaxFileSize {
			return nil
		}
		entries = append(entries, FileEntry{
			Path:  path,
			Size:  size,
			Mtime: info.ModTime().UnixMilli(),
		})
		return nil
	})
	return entries, err
}
