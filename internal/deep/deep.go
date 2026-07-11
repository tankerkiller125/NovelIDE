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

// FindEntities runs deep NER over text. Offsets in the result are rune
// offsets into text.
func (e *Engine) FindEntities(modelsDir, modelName, text string) ([]Entity, error) {
	m, err := e.load(modelsDir, modelName)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	resp, err := m.Classify(ctx, text, tokenclassification.Parameters{
		AggregationStrategy: tokenclassification.AggregationStrategySimple,
	})
	if err != nil {
		return nil, err
	}

	// Offsets from cybertron may be byte-based; convert defensively.
	conv := offsetConverter(text)
	var out []Entity
	for _, t := range resp.Tokens {
		label, ok := labelMap[strings.ToUpper(t.Label)]
		if !ok {
			continue
		}
		out = append(out, Entity{
			Text:  t.Text,
			Label: label,
			Start: conv(t.Start),
			End:   conv(t.End),
			Score: t.Score,
		})
	}
	return out, nil
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
