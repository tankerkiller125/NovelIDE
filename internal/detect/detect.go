// Package detect implements the rule-based consistency engine.
//
// It answers questions like: "this character died in book 2 chapter 12 —
// is this later passage using them as an active participant?" Detection is
// deterministic and fully local: part-of-speech tagging (via the pure-Go
// prose library) decides whether a mention is an *action* ("Aria walked",
// "Aria was walking") or a harmless reference ("Aria was dead", "Aria's
// sword", "she was carried out"). No AI, no network.
package detect

import (
	"fmt"
	"strings"

	"novelide/internal/match"
	"novelide/internal/model"
	"novelide/internal/nlp"
)

// Severity of a flag.
type Severity string

const (
	SevError   Severity = "error"
	SevWarning Severity = "warning"
	SevInfo    Severity = "info"
)

// Flag is one detected consistency problem, anchored to a text span.
type Flag struct {
	EntryID  string   `json:"entryId"`
	Start    int      `json:"start"` // rune offset
	End      int      `json:"end"`
	Severity Severity `json:"severity"`
	Rule     string   `json:"rule"`
	Message  string   `json:"message"`
}

// deadStates are status values treated as "no longer able to act".
var deadStates = map[string]bool{
	"dead": true, "deceased": true, "destroyed": true, "killed": true,
}

// speechVerbs handle inverted dialogue attribution ("said Aria", "whispered
// Aria") — the one construction where a verb *before* the name means the
// entity is acting. A closed class, so a list is the right tool.
var speechVerbs = map[string]bool{
	"said": true, "says": true, "asked": true, "asks": true, "replied": true,
	"replies": true, "whispered": true, "whispers": true, "shouted": true,
	"shouts": true, "muttered": true, "mutters": true, "murmured": true,
	"murmurs": true, "answered": true, "answers": true, "cried": true,
	"cries": true, "called": true, "calls": true, "snapped": true,
	"snaps": true, "sighed": true, "sighs": true, "growled": true,
	"growls": true, "hissed": true, "hisses": true, "yelled": true,
	"yells": true, "added": true, "adds": true, "began": true, "begins": true,
	"continued": true, "continues": true, "agreed": true, "agrees": true,
	"warned": true, "warns": true, "demanded": true, "demands": true,
}

// copulas: "X was ..." asserts a state, not an action — unless followed by
// a present participle ("was walking").
var copulas = map[string]bool{
	"is": true, "was": true, "were": true, "are": true, "am": true,
	"be": true, "been": true, "being": true, "isn't": true, "wasn't": true,
	"weren't": true, "aren't": true,
}

// perfectAux: "X had walked" reads as recollection/backstory far more often
// than as live action, so perfect constructions stay informational.
var perfectAux = map[string]bool{"had": true, "has": true, "have": true, "hadn't": true}

// semiLinking verbs act like copulas when followed by an adjective or past
// participle ("lay dead", "seemed lost") but are real actions otherwise
// ("lay down", "looked around").
var semiLinking = map[string]bool{
	"seemed": true, "seems": true,
	"appeared": true, "appears": true, "looked": true, "looks": true,
	"felt": true, "feels": true, "remained": true, "remains": true,
}

// posture verbs describe a body's position. A dead or destroyed entity can
// perfectly well lie, sit, or rest somewhere ("the spot where James lay"), so
// these read as a state, not an action — unless a motion particle marks a
// deliberate change of position ("lay down", "sat up"), which a corpse can't do.
var posture = map[string]bool{
	"lay": true, "lie": true, "lies": true, "lying": true, "lain": true, "laid": true,
	"sat": true, "sits": true, "sitting": true,
	"rested": true, "rests": true, "resting": true,
	"sprawled": true, "slumped": true, "slouched": true, "reclined": true,
	"knelt": true, "crouched": true, "hung": true, "hangs": true,
}

// motionParticle marks the deliberate-movement sense of a posture verb.
var motionParticle = map[string]bool{
	"down": true, "up": true, "back": true, "forward": true, "over": true,
}

// Timeline resolves (book, chapter) pairs to a global ordinal so status
// changes and the chapter being scanned can be compared.
type Timeline struct {
	order map[string]int // "book/chapter" -> position; "book/" -> position of book start
}

// NewTimeline builds a Timeline from a workspace's book/chapter ordering.
// Book starts get their own ordinal so an event anchored to "book N" (no
// chapter) sorts after every chapter of book N-1 and before every chapter
// of book N — even when books have no chapters yet.
func NewTimeline(ws *model.Workspace) *Timeline {
	t := &Timeline{order: map[string]int{}}
	pos := 0
	for _, b := range ws.Books {
		pos++
		t.order[b.ID+"/"] = pos
		for _, ch := range b.Chapters {
			pos++
			t.order[b.ID+"/"+ch] = pos
		}
	}
	return t
}

// position returns the global ordinal for a story point. Points that can't
// be resolved (unknown book/chapter) return -1. An empty point means "before
// everything" (0).
func (t *Timeline) position(p model.StoryPoint) int {
	if p.Book == "" {
		return 0
	}
	if p.Chapter == "" {
		if v, ok := t.order[p.Book+"/"]; ok {
			return v
		}
		return -1
	}
	if v, ok := t.order[p.Book+"/"+p.Chapter]; ok {
		return v
	}
	return -1
}

// StateAt returns the entity's effective state when the given chapter takes
// place, considering only status changes anchored strictly before it.
func (t *Timeline) StateAt(e *model.CodexEntry, bookID, chapter string) string {
	here := t.position(model.StoryPoint{Book: bookID, Chapter: chapter})
	if here < 0 {
		return ""
	}
	state := ""
	best := -1
	for _, sc := range e.Status {
		p := t.position(sc.At)
		if p < 0 || p > here {
			continue
		}
		// A death anchored to this very chapter shouldn't flag usage
		// within the chapter (the death scene itself), except when the
		// anchor is strictly earlier.
		if p == here && sc.At.Chapter != "" {
			continue
		}
		if p >= best {
			best = p
			state = sc.State
		}
	}
	return state
}

// valueAt resolves a timelined field to the value in effect at the given
// chapter: the most recent value anchored at or before it (a nil anchor means
// "from the start"). When the position is unknown it falls back to the latest
// value. Returns "" if no value applies yet.
func (t *Timeline) valueAt(vals []model.TimedValue, bookID, chapter string) string {
	here := t.position(model.StoryPoint{Book: bookID, Chapter: chapter})
	best := -1
	val := ""
	for _, tv := range vals {
		p := 0
		if tv.At != nil {
			p = t.position(*tv.At)
		}
		if p < 0 || (here >= 0 && p > here) {
			continue
		}
		if p >= best {
			best = p
			val = tv.Value
		}
	}
	return val
}

// mentionRole classifies how a mention participates in its sentence.
type mentionRole int

const (
	roleReference mentionRole = iota // possessive, memory, description of a state
	roleAgent                        // the entity performs an action
	rolePatient                      // the entity is acted upon ("was carried out")
)

// classify decides the mention's grammatical role using POS tags.
func classify(doc *nlp.Doc, sp match.Span) (mentionRole, string) {
	from, to := doc.SentenceOf(sp.Start)
	toks := doc.Tokens[from:to]

	// Index of the first token at/after the mention, and last before it.
	after := -1
	before := -1
	for i, t := range toks {
		if t.End <= sp.Start {
			before = i
		}
		if after == -1 && t.Start >= sp.End {
			after = i
		}
	}

	// Inverted dialogue attribution: `said Aria`.
	if before >= 0 && speechVerbs[strings.ToLower(toks[before].Text)] {
		return roleAgent, toks[before].Text
	}

	if after == -1 {
		return roleReference, ""
	}

	// Walk forward over the mention's complements, skipping adverbs,
	// commas, and the word "too" so "Aria, too, slowly walked" resolves.
	i := after
	skipped := 0
	for i < len(toks) && skipped < 3 {
		t := toks[i]
		if t.Tag == "RB" || t.Tag == "RBR" || t.Tag == "," || strings.ToLower(t.Text) == "too" {
			i++
			skipped++
			continue
		}
		break
	}
	if i >= len(toks) {
		return roleReference, ""
	}
	t := toks[i]
	word := strings.ToLower(t.Text)
	next := func() *nlp.Token {
		for j := i + 1; j < len(toks); j++ {
			if toks[j].Tag == "RB" || toks[j].Tag == "," {
				continue
			}
			return &toks[j]
		}
		return nil
	}

	switch {
	case t.Tag == "POS": // "Aria's sword" — possessive, pure reference
		return roleReference, ""
	case t.Tag == "MD": // "Aria would have wanted..." — modal, usually counterfactual
		return roleReference, ""
	case perfectAux[word]: // "Aria had walked these halls" — recollection
		return roleReference, ""
	case copulas[word]:
		if n := next(); n != nil && n.Tag == "VBG" { // "was walking"
			return roleAgent, t.Text + " " + n.Text
		}
		if n := next(); n != nil && n.Tag == "VBN" { // "was carried" — passive
			return rolePatient, t.Text + " " + n.Text
		}
		return roleReference, "" // "was dead", "was a legend", "was there"
	case posture[word]:
		// "James lay down" / "sat up" — a deliberate change of position is an
		// action; a corpse can't do it. Look a token or two ahead (past any
		// adverb) for the motion particle.
		for j := i + 1; j < len(toks) && j <= i+2; j++ {
			if motionParticle[strings.ToLower(toks[j].Text)] {
				return roleAgent, t.Text + " " + toks[j].Text
			}
			if toks[j].Tag == "RB" || toks[j].Tag == "," {
				continue
			}
			break
		}
		return roleReference, "" // "where James lay", "James lay dead / still"
	case semiLinking[word]:
		// Tag check plus a small lexicon: the tagger occasionally calls
		// "lifeless" a noun.
		if n := next(); n != nil &&
			(n.Tag == "JJ" || n.Tag == "VBN" || deadAdjectives[strings.ToLower(n.Text)]) {
			return roleReference, "" // "seemed lost", "looked dead"
		}
		return roleAgent, t.Text // "looked around"
	case nlp.IsVerb(t.Tag): // open class: any tagged verb is an action
		return roleAgent, t.Text
	}
	return roleReference, ""
}

// Check runs all rules against one chapter's text and pre-computed entity
// spans, returning flags for the frontend to render. doc is the parsed
// chapter (shared across detection passes); a nil doc disables the
// grammar-based rules.
func Check(ws *model.Workspace, bookID, chapter string, spans []match.Span, doc *nlp.Doc) []Flag {
	if doc == nil {
		return nil
	}
	tl := NewTimeline(ws)
	byID := map[string]*model.CodexEntry{}
	for i := range ws.Codex {
		byID[ws.Codex[i].ID] = &ws.Codex[i]
	}

	var flags []Flag
	for _, sp := range spans {
		e, ok := byID[sp.EntryID]
		if !ok {
			continue
		}
		state := tl.StateAt(e, bookID, chapter)
		if !deadStates[strings.ToLower(state)] {
			continue
		}
		label := e.Type
		if label == "" {
			label = "entity"
		}
		role, verb := classify(doc, sp)
		switch role {
		case roleAgent:
			flags = append(flags, Flag{
				EntryID: sp.EntryID, Start: sp.Start, End: sp.End,
				Severity: SevError, Rule: "dead-entity-agency",
				Message: fmt.Sprintf("%s is %s at this point in the story, but appears to act here (%q … %s)", e.Name, state, sp.Text, verb),
			})
		default:
			flags = append(flags, Flag{
				EntryID: sp.EntryID, Start: sp.Start, End: sp.End,
				Severity: SevInfo, Rule: "dead-entity-mention",
				Message: fmt.Sprintf("%s (%s) is %s at this point in the story — mention is fine, active use is not", e.Name, label, state),
			})
		}
	}
	return flags
}
