package astchunk

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	treesittermarkdown "github.com/smacker/go-tree-sitter/markdown/tree-sitter-markdown"
	"github.com/smacker/go-tree-sitter/protobuf"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/yaml"
	treesitterjson "github.com/tree-sitter/tree-sitter-json/bindings/go"
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
	case "ts", "typescript":
		return typescript.GetLanguage()
	case "tsx":
		return tsx.GetLanguage()
	case "css":
		return css.GetLanguage()
	case "md", "markdown":
		return treesittermarkdown.GetLanguage()
	case "yaml", "yml":
		return yaml.GetLanguage()
	case "toml":
		return toml.GetLanguage()
	case "proto", "protobuf":
		return protobuf.GetLanguage()
	case "json":
		return sitter.NewLanguage(treesitterjson.Language())
	case "txt":
		return nil
	default:
		return nil
	}
}

// SupportedExtensions returns file extensions to index (AST chunking when a grammar exists, otherwise character chunking; .txt has no grammar).
func SupportedExtensions() []string {
	return []string{
		".go", ".js", ".mjs", ".cjs", ".javascript",
		".ts", ".tsx", ".typescript", ".css",
		".md", ".markdown", ".txt",
		".yaml", ".yml", ".toml", ".proto", ".json",
	}
}
