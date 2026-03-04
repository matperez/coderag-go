package astchunk

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/cue"
	"github.com/smacker/go-tree-sitter/dockerfile"
	"github.com/smacker/go-tree-sitter/elixir"
	"github.com/smacker/go-tree-sitter/elm"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/groovy"
	"github.com/smacker/go-tree-sitter/hcl"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/lua"
	treesittermarkdown "github.com/smacker/go-tree-sitter/markdown/tree-sitter-markdown"
	"github.com/smacker/go-tree-sitter/ocaml"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/protobuf"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/scala"
	"github.com/smacker/go-tree-sitter/sql"
	"github.com/smacker/go-tree-sitter/svelte"
	"github.com/smacker/go-tree-sitter/swift"
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
	case "py", "python":
		return python.GetLanguage()
	case "rb", "ruby":
		return ruby.GetLanguage()
	case "c", "h":
		return c.GetLanguage()
	case "cpp", "cc", "cxx", "hpp", "hh", "hxx":
		return cpp.GetLanguage()
	case "cs", "csharp":
		return csharp.GetLanguage()
	case "sh", "bash":
		return bash.GetLanguage()
	case "html", "htm":
		return html.GetLanguage()
	case "java":
		return java.GetLanguage()
	case "rs", "rust":
		return rust.GetLanguage()
	case "swift":
		return swift.GetLanguage()
	case "php":
		return php.GetLanguage()
	case "lua":
		return lua.GetLanguage()
	case "kt", "kotlin":
		return kotlin.GetLanguage()
	case "scala", "sc":
		return scala.GetLanguage()
	case "groovy", "grvy", "gy", "gvy":
		return groovy.GetLanguage()
	case "ex", "exs", "elixir":
		return elixir.GetLanguage()
	case "elm":
		return elm.GetLanguage()
	case "ml", "mli", "ocaml":
		return ocaml.GetLanguage()
	case "hcl":
		return hcl.GetLanguage()
	case "cue":
		return cue.GetLanguage()
	case "dockerfile":
		return dockerfile.GetLanguage()
	case "svelte":
		return svelte.GetLanguage()
	case "sql":
		return sql.GetLanguage()
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
		".py", ".python", ".rb", ".ruby",
		".c", ".h", ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx",
		".cs", ".csharp", ".sh", ".bash",
		".html", ".htm", ".java", ".rs", ".rust", ".swift",
		".php", ".lua", ".kt", ".kotlin", ".scala", ".sc",
		".groovy", ".grvy", ".gy", ".gvy", ".ex", ".exs", ".elixir",
		".elm", ".ml", ".mli", ".ocaml", ".hcl", ".cue",
		"dockerfile", ".svelte", ".sql",
	}
}
