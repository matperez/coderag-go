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
// We split on the inverse: anything that is not a word character.
var wordBoundary = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// Tokenize splits text into tokens suitable for indexing and search.
// Word boundaries follow Unicode letters and digits; ASCII identifiers
// are further split by camelCase/snake_case. All tokens are lowercased;
// tokens shorter than 2 characters are dropped.
func Tokenize(text string) []string {
	if text == "" {
		return nil
	}
	segments := wordBoundary.FindAllString(text, -1)
	var out []string
	for _, seg := range segments {
		tokens := splitSegment(seg)
		for _, t := range tokens {
			t = strings.ToLower(t)
			if len(t) >= minTokenLen {
				out = append(out, t)
			}
		}
	}
	return out
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
