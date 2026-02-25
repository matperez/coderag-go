package astchunk

import (
	"context"
	"strings"

	"github.com/matperez/coderag-go/internal/chunk"
	sitter "github.com/smacker/go-tree-sitter"
)

// ChunkNodeTypes maps language (by ext) to AST node types that form chunk boundaries.
var ChunkNodeTypes = map[string][]string{
	"go": {`function_declaration`, `type_declaration`},
	"js": {`function_declaration`, `class_declaration`},
	"javascript": {`function_declaration`, `class_declaration`},
	"mjs": {`function_declaration`, `class_declaration`},
	"cjs": {`function_declaration`, `class_declaration`},
}

// ChunkByAST splits content into chunks using AST boundaries (functions, types, classes).
// Returns (chunks, true) if AST parsing succeeded, or (nil, false) to use character-based fallback.
// maxChunkSize is used to split oversized nodes and to merge small consecutive chunks.
func ChunkByAST(ctx context.Context, content string, ext string, maxChunkSize int) ([]chunk.Chunk, bool) {
	if maxChunkSize <= 0 {
		maxChunkSize = 1000
	}
	ext = strings.TrimPrefix(strings.ToLower(ext), ".")
	lang := LanguageForExt(ext)
	if lang == nil {
		return nil, false
	}
	tree := Parse(ctx, []byte(content), lang)
	if tree == nil {
		return nil, false
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return nil, false
	}
	types, ok := ChunkNodeTypes[ext]
	if !ok {
		return nil, false
	}
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}
	var nodes []*sitter.Node
	collectChunkNodes(root, typeSet, &nodes)
	if len(nodes) == 0 {
		return nil, false
	}
	imports := extractImports(root, []byte(content), ext)
	chunks := buildChunksFromNodes([]byte(content), nodes, imports, maxChunkSize)
	return chunks, true
}

func collectChunkNodes(n *sitter.Node, typeSet map[string]bool, out *[]*sitter.Node) {
	if typeSet[n.Type()] {
		*out = append(*out, n)
		return
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		child := n.Child(i)
		if child != nil {
			collectChunkNodes(child, typeSet, out)
		}
	}
}

func extractImports(root *sitter.Node, content []byte, ext string) string {
	if ext == "go" {
		var b strings.Builder
		for i := 0; i < int(root.ChildCount()); i++ {
			c := root.Child(i)
			if c != nil && c.Type() == "import_declaration" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.Write(content[c.StartByte():c.EndByte()])
			}
		}
		return b.String()
	}
	return ""
}

func buildChunksFromNodes(content []byte, nodes []*sitter.Node, imports string, maxChunkSize int) []chunk.Chunk {
	var chunks []chunk.Chunk
	for _, n := range nodes {
		part := string(content[n.StartByte():n.EndByte()])
		startLine := int(n.StartPoint().Row) + 1
		endLine := int(n.EndPoint().Row) + 1
		chunkType := nodeTypeToChunkType(n.Type())
		if len(part) > maxChunkSize {
			// Split by character (line boundaries)
			sub := chunk.ChunkByCharacters(part, maxChunkSize)
			for i := range sub {
				sub[i].Type = chunkType
				sub[i].StartLine = startLine
				sub[i].EndLine = endLine
				if i > 0 {
					sub[i].StartLine = sub[i-1].EndLine + 1
				}
				if i < len(sub)-1 {
					sub[i].EndLine = sub[i].StartLine + strings.Count(sub[i].Content, "\n")
				}
			}
			chunks = append(chunks, sub...)
		} else {
			chunks = append(chunks, chunk.Chunk{
				Content:   part,
				Type:      chunkType,
				StartLine: startLine,
				EndLine:   endLine,
			})
		}
	}
	// Prepend imports to first chunk if present
	if imports != "" && len(chunks) > 0 {
		chunks[0].Content = imports + "\n\n" + chunks[0].Content
	}
	// Merge small consecutive chunks
	return mergeSmallChunks(chunks, maxChunkSize)
}

func nodeTypeToChunkType(t string) string {
	switch t {
	case "function_declaration":
		return "function"
	case "type_declaration":
		return "type"
	case "class_declaration":
		return "class"
	default:
		return t
	}
}

func mergeSmallChunks(chunks []chunk.Chunk, maxChunkSize int) []chunk.Chunk {
	if len(chunks) <= 1 {
		return chunks
	}
	var out []chunk.Chunk
	cur := chunks[0]
	for i := 1; i < len(chunks); i++ {
		next := chunks[i]
		if len(cur.Content)+1+len(next.Content) <= maxChunkSize && cur.EndLine+1 >= next.StartLine {
			cur.Content = cur.Content + "\n" + next.Content
			cur.EndLine = next.EndLine
			continue
		}
		out = append(out, cur)
		cur = next
	}
	out = append(out, cur)
	return out
}
