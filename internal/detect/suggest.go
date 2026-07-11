package detect

import (
	"fmt"
	"strings"

	"novelide/internal/match"
	"novelide/internal/model"
	"novelide/internal/nlp"
)

// Suggestion is a fact the engine noticed in the manuscript that the codex
// doesn't record yet — the frontend offers to save it. It is the inverse of
// Check: Check flags text contradicting the codex; Suggest flags codex gaps
// implied by the text.
type Suggestion struct {
	Kind     string `json:"kind"`               // "status" | "relation" | "entity" | "field" | "alias"
	EntryID  string `json:"entryId,omitempty"`  // status: the entity; relation: the subject (e.g. killer)
	TargetID string `json:"targetId,omitempty"` // relation object (e.g. victim)
	State    string `json:"state,omitempty"`    // for Kind == "status"
	Relation string `json:"relation,omitempty"` // relation type id, for Kind == "relation"
	// Name: for Kind == "entity" the unknown proper name; for "alias" the
	// new alias text.
	Name       string `json:"name,omitempty"`
	FieldKey   string `json:"fieldKey,omitempty"`   // for Kind == "field": e.g. "hair", "gender"
	FieldValue string `json:"fieldValue,omitempty"` // the observed value
	Start      int    `json:"start"`                // rune offsets of the triggering mention
	End        int    `json:"end"`
	Message    string `json:"message"`
	// Key is position-independent so a dismissal survives edits and rescans.
	Key string `json:"key"`
}

var killLemmas = map[string]bool{
	"kill": true, "murder": true, "slay": true, "behead": true,
	"execute": true, "assassinate": true,
}

var dieLemmas = map[string]bool{"die": true, "perish": true}

// deadAdjectives complete copula phrasings: "was dead", "lay lifeless".
var deadAdjectives = map[string]bool{"dead": true, "lifeless": true}

var negations = map[string]bool{
	"almost": true, "nearly": true, "never": true, "not": true, "n't": true,
	"hadn't": true, "wasn't": true, "isn't": true, "couldn't": true,
	"barely": true, "hardly": true,
}

// Suggest scans chapter text for facts the codex doesn't know yet: deaths,
// kills, marriages, love — and repeated proper names (via NER) that have no
// codex entry at all. Heuristics only fire within a single sentence and
// never when the fact is already recorded. doc is the parsed chapter,
// shared across detection passes.
func Suggest(ws *model.Workspace, bookID, chapter string, spans []match.Span, doc *nlp.Doc) []Suggestion {
	if doc == nil {
		return nil
	}
	byID := map[string]*model.CodexEntry{}
	for i := range ws.Codex {
		byID[ws.Codex[i].ID] = &ws.Codex[i]
	}
	relDefs := map[string]bool{}
	for _, r := range ws.Schema.Relations {
		relDefs[r.ID] = true
	}
	hasDeadStatus := func(e *model.CodexEntry) bool {
		for _, sc := range e.Status {
			if deadStates[sc.State] {
				return true
			}
		}
		return false
	}
	hasRelation := func(e *model.CodexEntry, relType, to string, symmetric bool) bool {
		for _, r := range e.Relations {
			if r.Type == relType && r.To == to {
				return true
			}
		}
		if symmetric {
			if other, ok := byID[to]; ok {
				for _, r := range other.Relations {
					if r.Type == relType && r.To == e.ID {
						return true
					}
				}
			}
		}
		return false
	}

	var out []Suggestion
	seen := map[string]bool{}
	add := func(s Suggestion) {
		if !seen[s.Key] {
			seen[s.Key] = true
			out = append(out, s)
		}
	}
	suggestDeath := func(victim *model.CodexEntry, sp match.Span) {
		if hasDeadStatus(victim) {
			return
		}
		add(Suggestion{
			Kind: "status", EntryID: victim.ID, State: "dead",
			Start: sp.Start, End: sp.End,
			Message: fmt.Sprintf("%s appears to die here, but the Codex doesn't record it. Add a \"dead\" status anchored to this chapter?", victim.Name),
			Key:     "status|" + victim.ID + "|dead",
		})
	}

	// Group entity mentions by sentence.
	type sentSpan struct {
		from, to int // token index range of the sentence
		mentions []match.Span
	}
	sents := map[int]*sentSpan{}
	for _, sp := range spans {
		from, to := doc.SentenceOf(sp.Start)
		if s, ok := sents[from]; ok {
			s.mentions = append(s.mentions, sp)
		} else {
			sents[from] = &sentSpan{from: from, to: to, mentions: []match.Span{sp}}
		}
	}

	for _, sent := range sents {
		toks := doc.Tokens[sent.from:sent.to]
		negated := false
		for _, t := range toks {
			if negations[strings.ToLower(t.Text)] {
				negated = true
				break
			}
		}
		if negated {
			continue
		}

		// Single-entity death phrasing: "X died", "X was dead",
		// "X was killed" (passive with no named killer in the sentence).
		for _, m := range sent.mentions {
			e, ok := byID[m.EntryID]
			if !ok {
				continue
			}
			for i, t := range toks {
				if t.Start < m.End || t.Start-m.End > 24 {
					continue
				}
				word := strings.ToLower(t.Text)
				if dieLemmas[lemmaOf(word)] && (nlp.IsVerb(t.Tag) || t.Tag == "VBN") {
					suggestDeath(e, m)
					break
				}
				if copulas[word] || semiLinking[word] || perfectAux[word] {
					// find the complement: "was dead", "was killed",
					// "had died", "had been slain".
					for j := i + 1; j < len(toks) && j <= i+3; j++ {
						cw := strings.ToLower(toks[j].Text)
						lem := lemmaOf(cw)
						if deadAdjectives[cw] || dieLemmas[lem] ||
							(killLemmas[lem] && toks[j].Tag == "VBN") {
							// For "was killed by X" the pair loop below
							// also records the killer relation.
							suggestDeath(e, m)
							break
						}
						if !copulas[cw] { // allow chained aux: "had been"
							break
						}
					}
				}
				break // only the first meaningful token after the mention
			}
		}

		// Two-entity patterns within one sentence.
		ms := sent.mentions
		for i := 0; i < len(ms); i++ {
			for j := i + 1; j < len(ms); j++ {
				a, aok := byID[ms[i].EntryID]
				b, bok := byID[ms[j].EntryID]
				if !aok || !bok || a.ID == b.ID {
					continue
				}
				var between []nlp.Token
				for _, t := range toks {
					if t.Start >= ms[i].End && t.End <= ms[j].Start {
						between = append(between, t)
					}
				}
				if len(between) == 0 || len(between) > 6 {
					continue
				}
				killIdx, byIdx := -1, -1
				hasMarry, hasLove := false, false
				for k, t := range between {
					lem := lemmaOf(t.Text)
					switch {
					case killLemmas[lem] && (nlp.IsVerb(t.Tag) || t.Tag == "VBN"):
						killIdx = k
					case strings.ToLower(t.Text) == "by":
						byIdx = k
					case lem == "marry" || lem == "wed":
						hasMarry = true
					case lem == "love" && nlp.IsVerb(t.Tag):
						hasLove = true
					}
				}
				switch {
				case killIdx >= 0:
					killer, victim, vspan := a, b, ms[j]
					if byIdx == killIdx+1 { // "B was killed by A"
						killer, victim, vspan = b, a, ms[i]
					}
					if relDefs["killed"] && !hasRelation(killer, "killed", victim.ID, false) {
						add(Suggestion{
							Kind: "relation", EntryID: killer.ID, TargetID: victim.ID, Relation: "killed",
							Start: ms[i].Start, End: ms[j].End,
							Message: fmt.Sprintf("%s appears to kill %s here. Record a \"killed\" relationship (from this chapter)?", killer.Name, victim.Name),
							Key:     "relation|" + killer.ID + "|killed|" + victim.ID,
						})
					}
					suggestDeath(victim, vspan)
				case hasMarry:
					if relDefs["married-to"] && !hasRelation(a, "married-to", b.ID, true) {
						add(Suggestion{
							Kind: "relation", EntryID: a.ID, TargetID: b.ID, Relation: "married-to",
							Start: ms[i].Start, End: ms[j].End,
							Message: fmt.Sprintf("%s and %s appear to marry here. Record a \"married to\" relationship?", a.Name, b.Name),
							Key:     "relation|" + a.ID + "|married-to|" + b.ID,
						})
					}
				case hasLove:
					if relDefs["loves"] && !hasRelation(a, "loves", b.ID, false) {
						add(Suggestion{
							Kind: "relation", EntryID: a.ID, TargetID: b.ID, Relation: "loves",
							Start: ms[i].Start, End: ms[j].End,
							Message: fmt.Sprintf("%s appears to love %s. Record a \"loves\" relationship?", a.Name, b.Name),
							Key:     "relation|" + a.ID + "|loves|" + b.ID,
						})
					}
				}
			}
		}
	}

	out = append(out, suggestNewEntities(ws, doc)...)
	return out
}

// suggestNewEntities uses NER to find proper names that appear repeatedly
// but have no codex entry — a character or place the author introduced and
// forgot to catalogue.
func suggestNewEntities(ws *model.Workspace, doc *nlp.Doc) []Suggestion {
	known := map[string]bool{}
	var knownNames []string
	for i := range ws.Codex {
		for _, n := range ws.Codex[i].Names() {
			known[n] = true
			knownNames = append(knownNames, n)
		}
	}
	overlapsKnown := func(name string) bool {
		if known[name] {
			return true
		}
		for _, kn := range knownNames {
			if strings.Contains(kn, name) || strings.Contains(name, kn) {
				return true
			}
		}
		return false
	}

	counts := map[string][]nlp.Entity{}
	for _, e := range doc.Entities {
		if e.Label != "PERSON" && e.Label != "GPE" {
			continue
		}
		name := strings.TrimSpace(e.Text)
		if len([]rune(name)) < 3 || overlapsKnown(name) {
			continue
		}
		counts[name] = append(counts[name], e)
	}

	var out []Suggestion
	for name, occs := range counts {
		if len(occs) < 2 {
			continue // one-off capitalized words are too noisy
		}
		out = append(out, Suggestion{
			Kind: "entity", Name: name,
			Start: occs[0].Start, End: occs[0].End,
			Message: fmt.Sprintf("%q appears %d times in this chapter but has no Codex entry. Create one?", name, len(occs)),
			Key:     "entity|" + name,
		})
	}
	return out
}
