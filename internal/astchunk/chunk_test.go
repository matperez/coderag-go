package astchunk

import (
	"context"
	"strings"
	"testing"
)

func TestChunkByAST_Go(t *testing.T) {
	content := `package main

import "fmt"

func Foo() int { return 1 }

func Bar() string { return "x" }
`
	chunks, ok := ChunkByAST(context.Background(), content, ".go", 1000)
	if !ok {
		t.Fatal("ChunkByAST should succeed for valid Go")
	}
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks (Foo, Bar), got %d", len(chunks))
	}
	var hasFoo, hasBar bool
	for _, c := range chunks {
		if c.Type != "function" && c.Type != "type" {
			t.Errorf("chunk type %q", c.Type)
		}
		if strings.Contains(c.Content, "Foo") {
			hasFoo = true
		}
		if strings.Contains(c.Content, "Bar") {
			hasBar = true
		}
	}
	if !hasFoo || !hasBar {
		t.Errorf("expected chunks to contain Foo and Bar")
	}
}

func TestChunkByAST_JavaScript(t *testing.T) {
	content := `function foo() { return 1; }
class Bar { method() {} }
`
	chunks, ok := ChunkByAST(context.Background(), content, ".js", 1000)
	if !ok {
		t.Fatal("ChunkByAST should succeed for valid JS")
	}
	if len(chunks) < 1 {
		t.Errorf("expected at least 1 chunk, got %d", len(chunks))
	}
	// JS may merge or split; ensure we have function/class content
	var hasFoo, hasBar bool
	for _, c := range chunks {
		if strings.Contains(c.Content, "foo") {
			hasFoo = true
		}
		if strings.Contains(c.Content, "Bar") {
			hasBar = true
		}
	}
	if !hasFoo || !hasBar {
		t.Errorf("expected chunks to contain foo and Bar declarations")
	}
}

func TestChunkByAST_Markdown(t *testing.T) {
	content := "# Hello\n\nSome paragraph.\n\n- list item"
	chunks, ok := ChunkByAST(context.Background(), content, ".md", 1000)
	if !ok {
		t.Fatal("ChunkByAST should succeed for .md with grammar")
	}
	if len(chunks) == 0 {
		t.Error("expected at least one chunk for markdown")
	}
}

func TestChunkByAST_txtFallback(t *testing.T) {
	chunks, ok := ChunkByAST(context.Background(), "plain text", ".txt", 1000)
	if ok || chunks != nil {
		t.Error(".txt has no grammar, should fallback")
	}
}

func TestChunkByAST_brokenCodeFallback(t *testing.T) {
	content := "package main\nfunc (  broken"
	chunks, ok := ChunkByAST(context.Background(), content, ".go", 1000)
	if ok {
		t.Error("ChunkByAST should not succeed for broken Go")
	}
	if chunks != nil {
		t.Error("chunks should be nil on parse error")
	}
}

func TestChunkByAST_unknownExt(t *testing.T) {
	chunks, ok := ChunkByAST(context.Background(), "x", ".xyz", 1000)
	if ok || chunks != nil {
		t.Error("unknown extension should fallback")
	}
}

func TestChunkByAST_Python(t *testing.T) {
	content := `def foo():
    return 1

class Bar:
    pass
`
	chunks, ok := ChunkByAST(context.Background(), content, ".py", 1000)
	if !ok {
		t.Fatal("ChunkByAST should succeed for valid Python")
	}
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks (foo, Bar), got %d", len(chunks))
	}
	var hasFoo, hasBar bool
	for _, c := range chunks {
		if strings.Contains(c.Content, "def foo") || strings.Contains(c.Content, "foo") {
			hasFoo = true
		}
		if strings.Contains(c.Content, "class Bar") || strings.Contains(c.Content, "Bar") {
			hasBar = true
		}
	}
	if !hasFoo || !hasBar {
		t.Errorf("expected chunks to contain foo and Bar declarations")
	}
}

func TestChunkByAST_Ruby(t *testing.T) {
	content := `def foo
  1
end

class Bar
end
`
	chunks, ok := ChunkByAST(context.Background(), content, ".rb", 1000)
	if !ok {
		t.Fatal("ChunkByAST should succeed for valid Ruby")
	}
	if len(chunks) < 1 {
		t.Errorf("expected at least 1 chunk, got %d", len(chunks))
	}
	var hasFoo, hasBar bool
	for _, c := range chunks {
		if strings.Contains(c.Content, "foo") {
			hasFoo = true
		}
		if strings.Contains(c.Content, "Bar") {
			hasBar = true
		}
	}
	if !hasFoo || !hasBar {
		t.Errorf("expected chunks to contain foo and Bar declarations")
	}
}
