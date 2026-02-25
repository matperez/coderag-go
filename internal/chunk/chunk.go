package chunk

// Chunk represents a fragment of content with line bounds.
type Chunk struct {
	Content   string
	Type      string
	StartLine int
	EndLine   int
}

// ChunkByCharacters splits content into chunks of at most maxChunkSize characters.
// It splits on line boundaries so that no chunk cuts in the middle of a line.
// Type is set to "text" for all chunks. Line numbers are 1-based.
func ChunkByCharacters(content string, maxChunkSize int) []Chunk {
	if maxChunkSize <= 0 {
		maxChunkSize = 1000
	}
	if content == "" {
		return nil
	}
	lines := splitLines(content)
	if len(lines) == 0 {
		return nil
	}
	var chunks []Chunk
	var buf string
	startLine := 1
	for i, line := range lines {
		lineNum := i + 1
		next := buf
		if next != "" {
			next += "\n"
		}
		next += line
		if len(next) > maxChunkSize && buf != "" {
			chunks = append(chunks, Chunk{
				Content:   buf,
				Type:      "text",
				StartLine: startLine,
				EndLine:   lineNum - 1,
			})
			buf = line
			startLine = lineNum
			if len(buf) > maxChunkSize {
				chunks = append(chunks, Chunk{
					Content:   buf,
					Type:      "text",
					StartLine: startLine,
					EndLine:   lineNum,
				})
				buf = ""
				startLine = lineNum + 1
			}
			continue
		}
		buf = next
	}
	if buf != "" {
		chunks = append(chunks, Chunk{
			Content:   buf,
			Type:      "text",
			StartLine: startLine,
			EndLine:   len(lines),
		})
	}
	return chunks
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	return lines
}
