package tokenizer

import "github.com/blevesearch/snowball/russian"

// RussianStemmer stems Russian (Cyrillic) words using Snowball algorithm.
var RussianStemmer Stemmer = StemFunc(func(word string) string {
	return russian.Stem(word, true)
})
