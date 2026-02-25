package tokenizer

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/fatih/camelcase"
)

// minTokenLen is the minimum rune length for a token to be kept.
const minTokenLen = 2

// wordBoundary matches sequences of letters (Unicode), digits, or underscore.
var wordBoundary = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// Stemmer is an optional stemmer for normalizing tokens (e.g. Russian: пользователя -> пользователь).
// If nil, no stemming is applied.
type Stemmer interface {
	Stem(word string) string
}

// StemFunc adapts a function to Stemmer.
type StemFunc func(word string) string

func (f StemFunc) Stem(word string) string { return f(word) }

// Tokenizer holds options for tokenization.
type Tokenizer struct {
	Stemmer Stemmer // optional, e.g. for Russian
}

// New returns a Tokenizer with default options (no stemming).
func New() *Tokenizer {
	return &Tokenizer{}
}

// NewWithStemmer returns a Tokenizer that applies stemmer to tokens (typically Cyrillic).
func NewWithStemmer(s Stemmer) *Tokenizer {
	return &Tokenizer{Stemmer: s}
}

// Tokenize splits text into tokens. Word boundaries follow Unicode letters and digits;
// ASCII identifiers are split by camelCase/snake_case. Tokens are lowercased;
// tokens shorter than 2 characters are dropped. If Stemmer is set, it is applied
// to each token (typically for Cyrillic tokens).
func (z *Tokenizer) Tokenize(text string) []string {
	if text == "" {
		return nil
	}
	segments := wordBoundary.FindAllString(text, -1)
	var out []string
	for _, seg := range segments {
		tokens := splitSegment(seg)
		for _, t := range tokens {
			t = strings.ToLower(t)
			if z.Stemmer != nil && isCyrillic(t) {
				t = z.Stemmer.Stem(t)
			}
			if len(t) >= minTokenLen {
				out = append(out, t)
			}
		}
	}
	return out
}

func isCyrillic(s string) bool {
	for _, r := range s {
		if r >= 0x0400 && r <= 0x04FF {
			return true
		}
	}
	return false
}

// Tokenize is the default tokenizer (no stemming). For stemmer support use Tokenizer.Tokenize.
func Tokenize(text string) []string {
	return New().Tokenize(text)
}

// splitSegment splits a segment (continuous word chars). If the segment
// is ASCII-only and looks like an identifier, apply camelCase split.
func splitSegment(seg string) []string {
	if seg == "" {
		return nil
	}
	if !isASCIIIdentifier(seg) {
		return []string{seg}
	}
	parts := camelcase.Split(seg)
	return parts
}

func isASCIIIdentifier(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' {
			return false
		}
	}
	return true
}
