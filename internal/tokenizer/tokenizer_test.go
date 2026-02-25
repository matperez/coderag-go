package tokenizer

import (
	"reflect"
	"strings"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		want  []string
		slice bool // if true, only check that want is a subset of got in order
	}{
		{
			name: "empty",
			text: "",
			want: nil,
		},
		{
			name: "camelCase identifier",
			text: "getUserById",
			want: []string{"get", "user", "by", "id"},
		},
		{
			name: "snake_case",
			text: "get_user_by_id",
			want: []string{"get", "user", "by", "id"},
		},
		{
			name: "Russian text",
			text: "получить пользователя",
			want: []string{"получить", "пользователя"},
		},
		{
			name:  "mixed code and comment",
			text:  "// получить пользователя\nfunc getUserById(id string)",
			slice: true,
			want:  []string{"получить", "пользователя", "func", "get", "user", "by", "id"},
		},
		{
			name: "single short dropped",
			text: "a b c",
			want: nil, // all length 1
		},
		{
			name: "short and long",
			text: "ab cde",
			want: []string{"ab", "cde"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.text)
			if tt.slice {
				if len(got) < len(tt.want) {
					t.Errorf("Tokenize() got %v, want at least %v", got, tt.want)
				}
				for i, w := range tt.want {
					if i >= len(got) || got[i] != w {
						t.Errorf("Tokenize() at %d: got %v, want prefix containing %v", i, got, tt.want)
						break
					}
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Tokenize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenize_lowercase(t *testing.T) {
	got := Tokenize("GetUser")
	want := []string{"get", "user"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize() = %v, want %v", got, want)
	}
}

func TestTokenize_unicodeWordBoundaries(t *testing.T) {
	// Cyrillic and Latin in one string
	got := Tokenize("user пользователь")
	want := []string{"user", "пользователь"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize() = %v, want %v", got, want)
	}
}

func TestTokenizer_WithRussianStemmer(t *testing.T) {
	z := NewWithStemmer(RussianStemmer)
	got := z.Tokenize("пользователя получить")
	// Snowball Russian stems: пользователя -> пользовател, получить -> получ
	if len(got) < 2 {
		t.Errorf("Tokenize with Russian stemmer = %v, want at least 2 tokens", got)
	}
	// Both forms should normalize to same stem so "пользователя" and "пользователь" match
	if got[0] != "пользовател" && got[0] != "пользователь" {
		t.Errorf("first token = %q, want stem of пользователя", got[0])
	}
	if got[1] != "получ" {
		t.Errorf("second token = %q, want получ", got[1])
	}
}

func BenchmarkTokenize(b *testing.B) {
	tok := New()
	text := "getUserById"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tok.Tokenize(text)
	}
}

func BenchmarkTokenize_code(b *testing.B) {
	tok := New()
	text := `// getUserById returns user by id
func getUserById(id string) (*User, error) {
	if id == "" {
		return nil, errors.New("id required")
	}
	return userRepository.FindByID(ctx, id)
}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tok.Tokenize(text)
	}
}

func BenchmarkTokenize_long(b *testing.B) {
	tok := New()
	line := "func getUserById(id string) { return userRepo.Find(id) }\n"
	const targetLen = 16 * 1024
	var bld strings.Builder
	for bld.Len() < targetLen {
		bld.WriteString(line)
	}
	text := bld.String()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tok.Tokenize(text)
	}
}
