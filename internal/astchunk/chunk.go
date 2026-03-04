package astchunk

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/matperez/coderag-go/internal/chunk"
)

// ChunkNodeTypes maps language (by ext) to AST node types that form chunk boundaries.
var ChunkNodeTypes = map[string][]string{
	"go":         {`function_declaration`, `type_declaration`},
	"js":         {`function_declaration`, `class_declaration`},
	"javascript": {`function_declaration`, `class_declaration`},
	"mjs":        {`function_declaration`, `class_declaration`},
	"cjs":        {`function_declaration`, `class_declaration`},
	"ts":         {`function_declaration`, `class_declaration`, `interface_declaration`, `type_alias_declaration`},
	"tsx":        {`function_declaration`, `class_declaration`, `interface_declaration`, `type_alias_declaration`},
	"typescript": {`function_declaration`, `class_declaration`, `interface_declaration`, `type_alias_declaration`},
	"css":        {`rule_set`, `at_rule`},
	"md":         {`atx_heading`, `setext_heading`, `paragraph`, `block_quote`, `fenced_code_block`, `loose_list`, `tight_list`, `table`},
	"markdown":   {`atx_heading`, `setext_heading`, `paragraph`, `block_quote`, `fenced_code_block`, `loose_list`, `tight_list`, `table`},
	"yaml":       {`block_mapping_pair`, `block_sequence_item`},
	"yml":        {`block_mapping_pair`, `block_sequence_item`},
	"toml":       {`table`, `key_value`},
	"proto":      {`message_definition`, `service_definition`, `enum_definition`},
	"protobuf":   {`message_definition`, `service_definition`, `enum_definition`},
	"json":       {`object`, `array`, `pair`},
	"py":         {`function_definition`, `class_definition`},
	"python":     {`function_definition`, `class_definition`},
	"rb":         {`method`, `class`, `module`},
	"ruby":       {`method`, `class`, `module`},
	"c":          {`function_definition`, `struct_specifier`, `declaration`},
	"h":          {`function_definition`, `struct_specifier`, `declaration`},
	"cpp":        {`function_definition`, `class_specifier`, `struct_specifier`},
	"cc":         {`function_definition`, `class_specifier`, `struct_specifier`},
	"cxx":        {`function_definition`, `class_specifier`, `struct_specifier`},
	"hpp":        {`function_definition`, `class_specifier`, `struct_specifier`},
	"hh":         {`function_definition`, `class_specifier`, `struct_specifier`},
	"hxx":        {`function_definition`, `class_specifier`, `struct_specifier`},
	"cs":         {`method_declaration`, `class_declaration`, `struct_declaration`, `interface_declaration`},
	"csharp":     {`method_declaration`, `class_declaration`, `struct_declaration`, `interface_declaration`},
	"sh":         {`function_definition`, `command`},
	"bash":       {`function_definition`, `command`},
	"html":       {`element`, `script_element`, `style_element`},
	"htm":        {`element`, `script_element`, `style_element`},
	"java":       {`method_declaration`, `class_declaration`, `interface_declaration`},
	"rs":         {`function_item`, `impl_item`, `struct_item`, `enum_item`, `trait_item`},
	"rust":       {`function_item`, `impl_item`, `struct_item`, `enum_item`, `trait_item`},
	"swift":      {`function_declaration`, `class_declaration`, `struct_declaration`, `enum_declaration`},
	"php":        {`function_definition`, `class_declaration`, `method_declaration`},
	"lua":        {`function_definition`, `local_function_statement`},
	"kt":         {`function_declaration`, `class_declaration`, `object_declaration`},
	"kotlin":     {`function_declaration`, `class_declaration`, `object_declaration`},
	"scala":      {`function_definition`, `class_definition`, `object_definition`},
	"sc":         {`function_definition`, `class_definition`, `object_definition`},
	"groovy":     {`method_definition`, `class_declaration`},
	"grvy":       {`method_definition`, `class_declaration`},
	"gy":         {`method_definition`, `class_declaration`},
	"gvy":        {`method_definition`, `class_declaration`},
	"ex":         {`function_definition`, `module_definition`},
	"exs":        {`function_definition`, `module_definition`},
	"elixir":     {`function_definition`, `module_definition`},
	"elm":        {`value_declaration`, `type_alias_declaration`, `type_declaration`},
	"ml":         {`value_definition`, `type_definition`, `module_definition`},
	"mli":        {`value_definition`, `type_definition`, `module_definition`},
	"ocaml":      {`value_definition`, `type_definition`, `module_definition`},
	"hcl":        {`block`, `attribute`},
	"cue":        {`field`, `definition`},
	"dockerfile": {`instruction`},
	"svelte":     {`script_element`, `style_element`, `element`},
	"sql":        {`create_table_statement`, `create_index_statement`, `create_trigger_statement`, `create_view_statement`},
}

// nodeBounds holds extracted node data so we can release the tree before building chunks.
type nodeBounds struct {
	StartByte, EndByte int
	StartLine, EndLine int
	Type               string
}

// ChunkByAST splits content into chunks using AST boundaries (functions, types, classes).
// Returns (chunks, true) if AST parsing succeeded, or (nil, false) to use character-based fallback.
// The tree is closed as soon as bounds are collected so the AST cache can be freed.
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
	root := tree.RootNode()
	if root == nil || root.HasError() {
		tree.Close()
		return nil, false
	}
	types, ok := ChunkNodeTypes[ext]
	if !ok {
		tree.Close()
		return nil, false
	}
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}
	contentBytes := []byte(content)
	var bounds []nodeBounds
	collectChunkBounds(root, typeSet, &bounds)
	imports := extractImports(root, contentBytes, ext)
	tree.Close() // release tree and its cache before building chunks
	if len(bounds) == 0 {
		return nil, false
	}
	chunks := buildChunksFromBounds(contentBytes, bounds, imports, maxChunkSize)
	return chunks, true
}

func collectChunkBounds(n *sitter.Node, typeSet map[string]bool, out *[]nodeBounds) {
	if typeSet[n.Type()] {
		*out = append(*out, nodeBounds{
			StartByte: int(n.StartByte()),
			EndByte:   int(n.EndByte()),
			StartLine: int(n.StartPoint().Row) + 1,
			EndLine:   int(n.EndPoint().Row) + 1,
			Type:      n.Type(),
		})
		return
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		child := n.Child(i)
		if child != nil {
			collectChunkBounds(child, typeSet, out)
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

func buildChunksFromBounds(content []byte, bounds []nodeBounds, imports string, maxChunkSize int) []chunk.Chunk {
	var chunks []chunk.Chunk
	for _, b := range bounds {
		part := string(content[b.StartByte:b.EndByte])
		chunkType := nodeTypeToChunkType(b.Type)
		if len(part) > maxChunkSize {
			// Split by character (line boundaries)
			sub := chunk.ChunkByCharacters(part, maxChunkSize)
			for i := range sub {
				sub[i].Type = chunkType
				sub[i].StartLine = b.StartLine
				sub[i].EndLine = b.EndLine
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
				StartLine: b.StartLine,
				EndLine:   b.EndLine,
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
	case "function_declaration", "function_definition", "function_item", "local_function_statement", "method", "method_declaration", "method_definition":
		return "function"
	case "type_declaration", "type_definition", "type_alias_declaration", "value_declaration", "value_definition":
		return "type"
	case "class_declaration", "class", "class_specifier", "class_definition":
		return "class"
	case "struct_declaration", "struct_specifier", "struct_item":
		return "struct"
	case "interface_declaration", "interface_definition":
		return "interface"
	case "enum_item", "enum_declaration":
		return "enum"
	case "impl_item", "trait_item":
		return "impl"
	case "module", "module_definition":
		return "module"
	case "object_declaration", "object_definition":
		return "object"
	case "block_mapping_pair", "block_sequence_item":
		return "block"
	case "key_value":
		return "section"
	case "table":
		return "table"
	case "message_definition", "service_definition", "enum_definition":
		return "definition"
	case "object", "array":
		return "block"
	case "pair":
		return "pair"
	case "rule_set":
		return "rule"
	case "at_rule":
		return "at_rule"
	case "atx_heading", "setext_heading":
		return "heading"
	case "paragraph":
		return "paragraph"
	case "block_quote", "fenced_code_block":
		return "block"
	case "loose_list", "tight_list":
		return "list"
	case "declaration", "command", "element", "script_element", "style_element":
		return "block"
	case "block", "attribute", "field", "definition", "instruction":
		return "block"
	case "create_table_statement", "create_index_statement", "create_trigger_statement", "create_view_statement":
		return "definition"
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
