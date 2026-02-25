package astchunk

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
)

// LanguageForExt returns the tree-sitter Language for the given file extension.
// Extension may be with or without leading dot (e.g. ".go" or "go").
// Returns nil if the extension is not supported.
func LanguageForExt(ext string) *sitter.Language {
	ext = strings.TrimPrefix(strings.ToLower(ext), ".")
	switch ext {
	case "go":
		return golang.GetLanguage()
	case "js", "javascript", "mjs", "cjs":
		return javascript.GetLanguage()
	default:
		return nil
	}
}

// SupportedExtensions returns a list of file extensions that have a tree-sitter grammar.
func SupportedExtensions() []string {
	return []string{".go", ".js", ".mjs", ".cjs", ".javascript"}
}
