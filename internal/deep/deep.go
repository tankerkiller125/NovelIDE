// Package deep is the optional transformer tier: Cybertron running
// Hugging Face models locally (pure Go, CPU inference, no API calls).
// It is heavyweight — models are hundreds of MB and a scan takes seconds —
// so it never runs on keystrokes; the user triggers it explicitly and can
// turn it off entirely in Settings.
package deep

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nlpodyssey/cybertron/pkg/tasks"
	"github.com/nlpodyssey/cybertron/pkg/tasks/tokenclassification"

	"novelide/internal/model"
)

// Entity is one deep-NER hit, rune-offset aligned.
type Entity struct {
	Text  string
	Label string // normalized: PERSON, ORG, GPE, MISC
	Start int
	End   int
	Score float64
}

// Engine lazily loads and caches one token-classification model.
type Engine struct {
	mu        sync.Mutex
	modelsDir string
	modelName string
	model     tokenclassification.Interface
}

func NewEngine() *Engine { return &Engine{} }

// load (re)loads the model if the configuration changed. The first call for
// a model downloads it from the Hugging Face Hub into modelsDir.
func (e *Engine) load(modelsDir, modelName string) (tokenclassification.Interface, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.model != nil && e.modelsDir == modelsDir && e.modelName == modelName {
		return e.model, nil
	}
	m, err := tasks.Load[tokenclassification.Interface](&tasks.Config{
		ModelsDir: modelsDir,
		ModelName: modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("loading deep model %s: %w", modelName, err)
	}
	e.model = m
	e.modelsDir = modelsDir
	e.modelName = modelName
	return m, nil
}

// labelMap normalizes CoNLL/OntoNotes label schemes to what the rest of the
// engine expects.
var labelMap = map[string]string{
	"PER": "PERSON", "PERSON": "PERSON", "B-PER": "PERSON", "I-PER": "PERSON",
	"LOC": "GPE", "GPE": "GPE", "B-LOC": "GPE", "I-LOC": "GPE",
	"ORG": "ORG", "B-ORG": "ORG", "I-ORG": "ORG",
	"FAC": "GPE", "MISC": "MISC", "B-MISC": "MISC", "I-MISC": "MISC",
}

// maxChunkRunes bounds each window fed to the model. Transformer NER models
// (BERT-family) accept at most 512 word-piece tokens; a chapter is far longer.
// ~1500 runes of English prose tokenizes to roughly 300 tokens, leaving
// comfortable headroom under the limit while keeping the number of inference
// passes low.
const maxChunkRunes = 1500

// FindEntities runs deep NER over text, windowing it so no single input
// exceeds the model's token limit. Offsets in the result are rune offsets into
// the original text.
func (e *Engine) FindEntities(modelsDir, modelName, text string) ([]Entity, error) {
	m, err := e.load(modelsDir, modelName)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var out []Entity
	for _, w := range chunkText(text, maxChunkRunes) {
		if err := e.classifyInto(ctx, m, w.text, w.runeStart, &out, 0); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// window is a slice of the text plus its rune offset in the original.
type window struct {
	text      string
	runeStart int
}

// classifyInto classifies one window and appends its entities (with absolute
// rune offsets) to out. If the model still rejects the input as too long — a
// pathological tokenization the char budget didn't foresee — it splits the
// window near its midpoint and recurses, so a scan never fails on length.
func (e *Engine) classifyInto(ctx context.Context, m tokenclassification.Interface, text string, runeStart int, out *[]Entity, depth int) error {
	resp, err := m.Classify(ctx, text, tokenclassification.Parameters{
		AggregationStrategy: tokenclassification.AggregationStrategySimple,
	})
	if err != nil {
		runes := []rune(text)
		if depth < 8 && len(runes) > 1 && isTooLong(err) {
			mid := splitPoint(runes)
			if err := e.classifyInto(ctx, m, string(runes[:mid]), runeStart, out, depth+1); err != nil {
				return err
			}
			return e.classifyInto(ctx, m, string(runes[mid:]), runeStart+mid, out, depth+1)
		}
		return err
	}

	// Cybertron's aggregated token Text is unreliable — it space-joins
	// word-piece slices taken at shifted offsets, so "Hagrid" comes back as
	// "t Hagr". We ignore it and rebuild the name from the source text around
	// the detected span (see recoverName). Offsets may be byte-based; convert.
	conv := offsetConverter(text)
	wr := []rune(text)
	for _, t := range resp.Tokens {
		label, ok := labelMap[strings.ToUpper(t.Label)]
		if !ok {
			continue
		}
		name, s, e := recoverName(wr, conv(t.Start), conv(t.End))
		if name == "" {
			continue
		}
		*out = append(*out, Entity{
			Text:  name,
			Label: label,
			Start: runeStart + s,
			End:   runeStart + e,
			Score: t.Score,
		})
	}
	return nil
}

// recoverName rebuilds a clean entity name from the source runes for an
// approximate span. The model's offsets can be off by a character or two, so
// we snap the span out to whole-word boundaries and then keep the run bounded
// by proper-noun (capitalized) words — dropping stray leading/trailing bits
// like "at", "and", or a comma that the shifted offsets pulled in. Returns the
// name and its corrected [start,end) rune offsets, or "" if nothing looks like
// a name.
func recoverName(runes []rune, start, end int) (string, int, int) {
	n := len(runes)
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}
	if start >= end {
		return "", 0, 0
	}
	// Snap outward so a mid-word offset captures the whole word.
	for start > 0 && isWordRune(runes[start-1]) {
		start--
	}
	for end < n && isWordRune(runes[end]) {
		end++
	}
	// Split into words with their offsets.
	type wtok struct{ start, end int }
	var words []wtok
	i := start
	for i < end {
		for i < end && !isWordRune(runes[i]) {
			i++
		}
		if i >= end {
			break
		}
		ws := i
		for i < end && isWordRune(runes[i]) {
			i++
		}
		words = append(words, wtok{ws, i})
	}
	// Trim leading/trailing words that aren't capitalized proper nouns.
	lo, hi := 0, len(words)-1
	for lo <= hi && !startsUpper(runes, words[lo].start) {
		lo++
	}
	for hi >= lo && !startsUpper(runes, words[hi].start) {
		hi--
	}
	if lo > hi {
		return "", 0, 0
	}
	s, e := words[lo].start, words[hi].end
	return strings.TrimSpace(string(runes[s:e])), s, e
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '’' || r == '-'
}

func startsUpper(runes []rune, i int) bool {
	return i >= 0 && i < len(runes) && unicode.IsUpper(runes[i])
}

func isTooLong(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "long") || strings.Contains(s, "512") ||
		strings.Contains(s, "sequence")
}

// splitPoint picks a split index near the middle of runes, preferring a space
// so a name isn't cut in half.
func splitPoint(runes []rune) int {
	mid := len(runes) / 2
	for off := 0; off < len(runes)/4; off++ {
		if mid-off > 0 && runes[mid-off] == ' ' {
			return mid - off
		}
		if mid+off < len(runes) && runes[mid+off] == ' ' {
			return mid + off
		}
	}
	return mid
}

// chunkText splits text into windows of at most maxRunes runes, breaking at
// sentence/line boundaries so entity names aren't split across windows. Each
// window carries its rune offset in the original text.
func chunkText(text string, maxRunes int) []window {
	if utf8.RuneCountInString(text) <= maxRunes {
		return []window{{text: text, runeStart: 0}}
	}
	var windows []window
	var cur []rune
	curStart, pos := 0, 0
	flush := func() {
		if len(cur) > 0 {
			windows = append(windows, window{text: string(cur), runeStart: curStart})
			cur = nil
		}
	}
	for _, piece := range splitPieces(text) {
		pr := []rune(piece)
		if len(pr) > maxRunes { // a single over-long sentence: hard-split
			flush()
			for off := 0; off < len(pr); off += maxRunes {
				end := off + maxRunes
				if end > len(pr) {
					end = len(pr)
				}
				windows = append(windows, window{text: string(pr[off:end]), runeStart: pos + off})
			}
			pos += len(pr)
			curStart = pos
			continue
		}
		if len(cur) > 0 && len(cur)+len(pr) > maxRunes {
			flush()
		}
		if len(cur) == 0 {
			curStart = pos
		}
		cur = append(cur, pr...)
		pos += len(pr)
	}
	flush()
	return windows
}

// splitPieces breaks text after each sentence terminator or newline, keeping
// the delimiter. Concatenating the pieces reproduces the original text.
func splitPieces(text string) []string {
	runes := []rune(text)
	var pieces []string
	start := 0
	for i, r := range runes {
		if r == '\n' || r == '.' || r == '!' || r == '?' {
			pieces = append(pieces, string(runes[start:i+1]))
			start = i + 1
		}
	}
	if start < len(runes) {
		pieces = append(pieces, string(runes[start:]))
	}
	return pieces
}

// offsetConverter returns a byte→rune converter; for pure-ASCII text it is
// the identity, which also makes it safe if offsets were already runes.
func offsetConverter(text string) func(int) int {
	if len(text) == utf8.RuneCountInString(text) {
		return func(b int) int { return b }
	}
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
	return func(b int) int {
		if b < 0 {
			return 0
		}
		if b > len(text) {
			return ri
		}
		return b2r[b]
	}
}

// SuggestEntities filters raw deep-NER hits against the codex and shapes
// them for the suggestion pipeline: unknown names with decent confidence,
// one suggestion per name. Exposed separately from FindEntities so it can
// be unit-tested without a 400 MB model.
func SuggestEntities(ws *model.Workspace, ents []Entity) []Entity {
	known := map[string]bool{}
	var knownNames []string
	for i := range ws.Codex {
		for _, n := range ws.Codex[i].Names() {
			known[strings.ToLower(n)] = true
			knownNames = append(knownNames, strings.ToLower(n))
		}
	}
	overlaps := func(name string) bool {
		l := strings.ToLower(name)
		if known[l] {
			return true
		}
		for _, kn := range knownNames {
			if strings.Contains(kn, l) || strings.Contains(l, kn) {
				return true
			}
		}
		return false
	}
	seen := map[string]bool{}
	var out []Entity
	for _, e := range ents {
		name := strings.TrimSpace(e.Text)
		if e.Label == "MISC" || e.Score < 0.75 || len([]rune(name)) < 3 {
			continue
		}
		if overlaps(name) || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		e.Text = name
		out = append(out, e)
	}
	return out
}
