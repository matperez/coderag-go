package astchunk

import (
	"context"
	"testing"
)

func TestLanguageForExt(t *testing.T) {
	tests := []struct {
		ext string
		ok  bool
	}{
		{".go", true},
		{"go", true},
		{".js", true},
		{"js", true},
		{"javascript", true},
		{".yaml", true},
		{"yml", true},
		{".toml", true},
		{".proto", true},
		{"protobuf", true},
		{".json", true},
		{"json", true},
		{".ts", true},
		{".tsx", true},
		{".css", true},
		{".md", true},
		{"markdown", true},
		{".txt", false},
		{"", false},
	}
	for _, tt := range tests {
		lang := LanguageForExt(tt.ext)
		if (lang != nil) != tt.ok {
			t.Errorf("LanguageForExt(%q) = %v, want ok=%v", tt.ext, lang != nil, tt.ok)
		}
	}
}

func TestParse_Go(t *testing.T) {
	content := []byte(`package main
func Foo() int { return 1 }
`)
	lang := LanguageForExt(".go")
	if lang == nil {
		t.Fatal("LanguageForExt(.go) returned nil")
	}
	tree := Parse(context.Background(), content, lang)
	if tree == nil {
		t.Fatal("Parse returned nil")
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("RootNode is nil")
	}
	if root.Type() != "source_file" {
		t.Errorf("root type = %q, want source_file", root.Type())
	}
	// Go source_file has function_declaration as child
	var hasFunc bool
	for i := 0; i < int(root.ChildCount()); i++ {
		if root.Child(i).Type() == "function_declaration" {
			hasFunc = true
			break
		}
	}
	if !hasFunc {
		t.Error("expected function_declaration node in Go AST")
	}
}

func TestParse_JavaScript(t *testing.T) {
	content := []byte("function foo() { return 1; }")
	lang := LanguageForExt(".js")
	if lang == nil {
		t.Fatal("LanguageForExt(.js) returned nil")
	}
	tree := Parse(context.Background(), content, lang)
	if tree == nil {
		t.Fatal("Parse returned nil")
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("RootNode is nil")
	}
	if root.Type() != "program" {
		t.Errorf("root type = %q, want program", root.Type())
	}
	// program has function_declaration as child
	var hasFunc bool
	for i := 0; i < int(root.ChildCount()); i++ {
		c := root.Child(i)
		if c.Type() == "function_declaration" || (c.Type() == "expression_statement" && c.ChildCount() > 0) {
			hasFunc = true
			break
		}
	}
	// JS might wrap in expression_statement
	for i := 0; i < int(root.NamedChildCount()); i++ {
		if root.NamedChild(i).Type() == "function_declaration" {
			hasFunc = true
			break
		}
	}
	if !hasFunc {
		t.Logf("root children: ")
		for i := 0; i < int(root.ChildCount()); i++ {
			t.Logf("  %s", root.Child(i).Type())
		}
		t.Error("expected function_declaration (or similar) node in JS AST")
	}
}

func TestParse_nilLanguage(t *testing.T) {
	tree := Parse(context.Background(), []byte("package p"), nil)
	if tree != nil {
		t.Error("Parse with nil language should return nil")
	}
}

func TestParse_emptyContent(t *testing.T) {
	lang := LanguageForExt(".go")
	tree := Parse(context.Background(), nil, lang)
	if tree != nil {
		t.Error("Parse with nil content should return nil")
	}
}
