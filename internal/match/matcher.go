// Package match scans manuscript text for mentions of codex entities.
package match

import (
	"sort"
	"unicode"
	"unicode/utf8"

	"novelide/internal/model"
)

// Span is one entity mention found in text. Offsets are byte offsets into
// the scanned string (CodeMirror-friendly after UTF-16 conversion client-side
// is avoided by the frontend scanning plain JS strings; we report both byte
// and rune offsets so the frontend can use rune offsets directly).
type Span struct {
	EntryID string `json:"entryId"`
	Start   int    `json:"start"` // rune offset
	End     int    `json:"end"`   // rune offset (exclusive)
	Text    string `json:"text"`
}

type candidate struct {
	name    string
	runes   []rune
	entryID string
}

// Matcher finds entity name/alias mentions with word boundaries,
// longest-match-wins, non-overlapping, case-sensitive.
type Matcher struct {
	// byFirst groups candidates by their first rune, longest first.
	byFirst map[rune][]candidate
}

// New builds a Matcher from codex entries.
func New(entries []model.CodexEntry) *Matcher {
	m := &Matcher{byFirst: map[rune][]candidate{}}
	for i := range entries {
		e := &entries[i]
		for _, name := range e.Names() {
			r := []rune(name)
			if len(r) == 0 {
				continue
			}
			m.byFirst[r[0]] = append(m.byFirst[r[0]], candidate{name: name, runes: r, entryID: e.ID})
		}
	}
	for k := range m.byFirst {
		sort.SliceStable(m.byFirst[k], func(i, j int) bool {
			return len(m.byFirst[k][i].runes) > len(m.byFirst[k][j].runes)
		})
	}
	return m
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// Scan returns all entity mentions in text, in order of appearance.
func (m *Matcher) Scan(text string) []Span {
	runes := []rune(text)
	var spans []Span
	for i := 0; i < len(runes); i++ {
		// Word boundary on the left.
		if i > 0 && isWordRune(runes[i-1]) && isWordRune(runes[i]) {
			continue
		}
		cands, ok := m.byFirst[runes[i]]
		if !ok {
			continue
		}
		for _, c := range cands {
			end := i + len(c.runes)
			if end > len(runes) {
				continue
			}
			if string(runes[i:end]) != c.name {
				continue
			}
			// Word boundary on the right.
			if end < len(runes) && isWordRune(runes[end-1]) && isWordRune(runes[end]) {
				continue
			}
			spans = append(spans, Span{EntryID: c.entryID, Start: i, End: end, Text: c.name})
			i = end - 1 // non-overlapping; loop increment moves past
			break
		}
	}
	return spans
}

// RuneLen is a helper for callers dealing with byte offsets.
func RuneLen(s string) int { return utf8.RuneCountInString(s) }
