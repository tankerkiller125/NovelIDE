package workspace

import (
	"regexp"
	"strings"
)

// TextMatch is one occurrence of a query within a chapter's text.
type TextMatch struct {
	Line   int    `json:"line"`   // 1-based line number
	Col    int    `json:"col"`    // 1-based rune column within the line
	Before string `json:"before"` // line text before the match (trimmed to a window)
	Match  string `json:"match"`  // the matched text
	After  string `json:"after"`  // line text after the match (trimmed to a window)
}

// queryRegexp compiles a plain search query into a regexp honouring the
// case-sensitivity and whole-word options. The query is treated as literal
// text (not a pattern) — QuoteMeta escapes any regex metacharacters.
func queryRegexp(query string, caseSensitive, wholeWord bool) (*regexp.Regexp, error) {
	pat := regexp.QuoteMeta(query)
	if wholeWord {
		pat = `\b` + pat + `\b`
	}
	if !caseSensitive {
		pat = `(?i)` + pat
	}
	return regexp.Compile(pat)
}

const matchContext = 48 // runes of line context kept on each side of a match

// SearchText finds every match of query in text, line by line, with a window
// of surrounding context for display.
func SearchText(text, query string, caseSensitive, wholeWord bool) ([]TextMatch, error) {
	if query == "" {
		return nil, nil
	}
	re, err := queryRegexp(query, caseSensitive, wholeWord)
	if err != nil {
		return nil, err
	}
	var out []TextMatch
	for i, line := range strings.Split(text, "\n") {
		locs := re.FindAllStringIndex(line, -1)
		if locs == nil {
			continue
		}
		runesBefore := runeIndex(line)
		for _, loc := range locs {
			before := clipLeft([]rune(line[:loc[0]]), matchContext)
			after := clipRight([]rune(line[loc[1]:]), matchContext)
			out = append(out, TextMatch{
				Line:   i + 1,
				Col:    runesBefore[loc[0]] + 1,
				Before: before,
				Match:  line[loc[0]:loc[1]],
				After:  after,
			})
		}
	}
	return out, nil
}

// ReplaceAllText replaces every match of query with replacement, returning the
// new text and the number of replacements made. The replacement is inserted
// literally (no regex expansion).
func ReplaceAllText(text, query, replacement string, caseSensitive, wholeWord bool) (string, int, error) {
	if query == "" {
		return text, 0, nil
	}
	re, err := queryRegexp(query, caseSensitive, wholeWord)
	if err != nil {
		return text, 0, err
	}
	count := 0
	out := re.ReplaceAllStringFunc(text, func(string) string {
		count++
		return replacement
	})
	return out, count, nil
}

// runeIndex maps each rune-start byte offset in s to the number of runes
// preceding it, so a byte match position (always on a rune boundary) can be
// reported as a rune column.
func runeIndex(s string) []int {
	idx := make([]int, len(s)+1)
	runes := 0
	for i := range s { // i ranges over rune-start byte offsets
		idx[i] = runes
		runes++
	}
	idx[len(s)] = runes
	return idx
}

func clipLeft(r []rune, n int) string {
	if len(r) <= n {
		return string(r)
	}
	return "…" + string(r[len(r)-n:])
}

func clipRight(r []rune, n int) string {
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
