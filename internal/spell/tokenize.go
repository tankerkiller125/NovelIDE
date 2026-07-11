package spell

import "unicode"

// Word is a checkable token with rune offsets.
type Word struct {
	Text  string `json:"word"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// Words extracts spellcheckable words: letter runs (internal apostrophes
// allowed), skipping anything with digits and single letters. Offsets are
// rune offsets, matching the rest of the pipeline.
func Words(text string) []Word {
	runes := []rune(text)
	var out []Word
	i := 0
	for i < len(runes) {
		if !unicode.IsLetter(runes[i]) {
			i++
			continue
		}
		j := i
		for j < len(runes) && (unicode.IsLetter(runes[j]) ||
			(runes[j] == '\'' || runes[j] == '’') && j+1 < len(runes) && unicode.IsLetter(runes[j+1]) && j > i) {
			j++
		}
		if j-i > 1 {
			out = append(out, Word{Text: string(runes[i:j]), Start: i, End: j})
		}
		i = j + 1
	}
	return out
}
