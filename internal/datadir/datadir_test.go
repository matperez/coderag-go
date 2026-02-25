package datadir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot get home dir: %v", err)
	}
	base := filepath.Join(home, dirName, projects)

	tests := []struct {
		name     string
		root     string
		wantPref string
		wantLen  int
	}{
		{
			name:     "same path same hash",
			root:     "/foo/bar",
			wantPref: base,
			wantLen:  len(base) + 1 + hashLen,
		},
		{
			name:     "different path different hash",
			root:     "/baz",
			wantPref: base,
			wantLen:  len(base) + 1 + hashLen,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DataDir(tt.root)
			if err != nil {
				t.Fatalf("DataDir() error = %v", err)
			}
			if !strings.HasPrefix(got, tt.wantPref) {
				t.Errorf("DataDir() = %q, want prefix %q", got, tt.wantPref)
			}
			if len(got) < tt.wantLen {
				t.Errorf("DataDir() = %q, want length >= %d", got, tt.wantLen)
			}
			// Hash part (last segment) must be 16 hex chars
			hashPart := filepath.Base(got)
			if len(hashPart) != hashLen {
				t.Errorf("hash segment = %q, want length %d", hashPart, hashLen)
			}
			for _, c := range hashPart {
				if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
					continue
				}
				t.Errorf("hash segment %q contains non-hex char %c", hashPart, c)
				break
			}
		})
	}
}

func TestDataDir_stableHash(t *testing.T) {
	// Same path must yield same directory
	got1, err := DataDir("/tmp/project")
	if err != nil {
		t.Fatal(err)
	}
	got2, err := DataDir("/tmp/project")
	if err != nil {
		t.Fatal(err)
	}
	if got1 != got2 {
		t.Errorf("same path gave different dirs: %q vs %q", got1, got2)
	}
	// Different paths should yield different hashes (with very high probability)
	got3, err := DataDir("/tmp/other")
	if err != nil {
		t.Fatal(err)
	}
	if got1 == got3 {
		t.Errorf("different paths gave same dir: %q", got1)
	}
}
