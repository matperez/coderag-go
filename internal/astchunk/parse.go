package astchunk

import (
	"context"

	sitter "github.com/smacker/go-tree-sitter"
)

// Parse parses content with the given tree-sitter language and returns the AST.
// If language is nil or parsing fails, returns nil.
func Parse(ctx context.Context, content []byte, lang *sitter.Language) *sitter.Tree {
	if lang == nil || len(content) == 0 {
		return nil
	}
	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil
	}
	return tree
}
