package tokenizer

import (
	"reflect"
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
			name: "mixed code and comment",
			text: "// получить пользователя\nfunc getUserById(id string)",
			slice: true,
			want: []string{"получить", "пользователя", "func", "get", "user", "by", "id"},
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
