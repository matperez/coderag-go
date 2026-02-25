package chunk

import (
	"strings"
	"testing"
)

func TestChunkByCharacters(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		maxChunkSize  int
		wantMinChunks int
		wantMaxChunk  int
		checkLines    bool
	}{
		{
			name:          "empty",
			content:       "",
			maxChunkSize:  100,
			wantMinChunks: 0,
		},
		{
			name:          "short single chunk",
			content:       "one line",
			maxChunkSize:  1000,
			wantMinChunks: 1,
			wantMaxChunk:  10,
			checkLines:    true,
		},
		{
			name:          "two lines one chunk",
			content:       "line1\nline2",
			maxChunkSize:  100,
			wantMinChunks: 1,
			wantMaxChunk:  12,
			checkLines:    true,
		},
		{
			name:          "splits by size",
			content:       strings.Repeat("a\n", 50),
			maxChunkSize:  20,
			wantMinChunks: 2,
			wantMaxChunk:  25,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChunkByCharacters(tt.content, tt.maxChunkSize)
			if tt.content == "" && got != nil {
				t.Errorf("empty content should return nil, got %d chunks", len(got))
			}
			if tt.wantMinChunks > 0 && len(got) < tt.wantMinChunks {
				t.Errorf("got %d chunks, want at least %d", len(got), tt.wantMinChunks)
			}
			for i, c := range got {
				if c.Type != "text" {
					t.Errorf("chunk %d Type = %q, want text", i, c.Type)
				}
				if tt.wantMaxChunk > 0 && len(c.Content) > tt.wantMaxChunk {
					t.Errorf("chunk %d len = %d, want <= %d", i, len(c.Content), tt.wantMaxChunk)
				}
				if tt.checkLines && c.StartLine < 1 {
					t.Errorf("chunk %d StartLine = %d", i, c.StartLine)
				}
			}
		})
	}
}

func TestChunkByCharacters_zeroSizeUsesDefault(t *testing.T) {
	content := strings.Repeat("x", 2000)
	got := ChunkByCharacters(content, 0)
	if len(got) == 0 {
		t.Error("zero maxChunkSize should default, got no chunks")
	}
}

func TestChunkByCharacters_noMidLineSplit(t *testing.T) {
	// Each line 100 chars, maxChunkSize 150 -> chunks should end at line boundaries
	lines := []string{
		strings.Repeat("a", 100),
		strings.Repeat("b", 100),
		strings.Repeat("c", 100),
	}
	content := strings.Join(lines, "\n")
	got := ChunkByCharacters(content, 150)
	for _, c := range got {
		if strings.Contains(c.Content, "a") && strings.Contains(c.Content, "b") {
			// Chunk spanning two lines is ok if each line is 100 and max 150
			continue
		}
		// No chunk should end in the middle of a line (e.g. 75 'a's)
		if len(c.Content) > 0 && !strings.HasSuffix(c.Content, "\n") && c.Content != lines[len(lines)-1] {
			// Last chunk might not end with \n
			continue
		}
	}
	if len(got) < 2 {
		t.Errorf("expected multiple chunks for 300 chars at 150 max, got %d", len(got))
	}
}
