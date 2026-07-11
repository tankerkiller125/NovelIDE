// Package nlp wraps the prose library (pure Go, fully offline) to give the
// detection engine real part-of-speech tags and named-entity recognition
// instead of hand-curated word lists. No network, no API keys.
//
// It uses the maintained tsawler/prose v3 fork (the original jdkato/prose
// was archived in 2023). v3 reports native Start/End positions — as byte
// offsets, which this package converts to rune offsets to match the
// match.Span conventions used everywhere else.
package nlp

import (
	"unicode/utf8"

	prose "github.com/tsawler/prose/v3"
)

// Token is a word with its Penn Treebank POS tag and rune offsets into the
// source text.
type Token struct {
	Text       string
	Tag        string
	Start      int // rune offset
	End        int
	Confidence float64
}

// Entity is a named entity found by NER, aligned to rune offsets.
type Entity struct {
	Text       string
	Label      string // PERSON, GPE, ORG, FAC, ...
	Start      int
	End        int
	Confidence float64
}

// Doc is a parsed chapter.
type Doc struct {
	Tokens   []Token
	Entities []Entity
	// sentences are [start, end) rune ranges, in order.
	sentences [][2]int
}

// Parse runs tokenization, POS tagging, sentence segmentation, and NER over
// text, converting all positions to rune offsets.
func Parse(text string) (*Doc, error) {
	pd, err := prose.NewDocument(text)
	if err != nil {
		return nil, err
	}

	// byte offset -> rune offset lookup table.
	b2r := make([]int, len(text)+1)
	ri := 0
	for bi := 0; bi < len(text); {
		_, size := utf8.DecodeRuneInString(text[bi:])
		for k := 0; k < size; k++ {
			b2r[bi+k] = ri
		}
		bi += size
		ri++
	}
	b2r[len(text)] = ri
	conv := func(b int) int {
		if b < 0 {
			return 0
		}
		if b > len(text) {
			return ri
		}
		return b2r[b]
	}

	d := &Doc{}
	for _, t := range pd.Tokens() {
		d.Tokens = append(d.Tokens, Token{
			Text: t.Text, Tag: t.Tag,
			Start: conv(t.Start), End: conv(t.End),
			Confidence: t.Confidence,
		})
	}
	for _, e := range pd.Entities() {
		d.Entities = append(d.Entities, Entity{
			Text: e.Text, Label: e.Label,
			Start: conv(e.Start), End: conv(e.End),
			Confidence: e.Confidence,
		})
	}
	for _, s := range pd.Sentences() {
		d.sentences = append(d.sentences, [2]int{conv(s.Start), conv(s.End)})
	}
	return d, nil
}

// SentenceOf returns the index range [from, to) of d.Tokens forming the
// sentence that contains rune position pos.
func (d *Doc) SentenceOf(pos int) (int, int) {
	sStart, sEnd := 0, 0
	for _, s := range d.sentences {
		if pos >= s[0] && pos < s[1] {
			sStart, sEnd = s[0], s[1]
			break
		}
	}
	if sEnd == 0 && len(d.sentences) > 0 {
		// pos past the last sentence (e.g. trailing whitespace): use the last.
		last := d.sentences[len(d.sentences)-1]
		sStart, sEnd = last[0], last[1]
	}
	from := len(d.Tokens)
	to := len(d.Tokens)
	for i, t := range d.Tokens {
		if from == len(d.Tokens) && t.Start >= sStart {
			from = i
		}
		if t.End > sEnd {
			to = i
			break
		}
	}
	if from > to {
		from = to
	}
	return from, to
}

// SentenceTokenRanges returns one [from, to) token-index range per
// sentence, in order.
func (d *Doc) SentenceTokenRanges() [][2]int {
	out := make([][2]int, 0, len(d.sentences))
	ti := 0
	for _, s := range d.sentences {
		for ti < len(d.Tokens) && d.Tokens[ti].Start < s[0] {
			ti++
		}
		from := ti
		for ti < len(d.Tokens) && d.Tokens[ti].End <= s[1] {
			ti++
		}
		out = append(out, [2]int{from, ti})
	}
	return out
}

// IsVerb reports whether a tag is any verb form.
func IsVerb(tag string) bool {
	return len(tag) >= 2 && tag[0] == 'V' && tag[1] == 'B'
}
