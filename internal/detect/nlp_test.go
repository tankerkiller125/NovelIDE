package detect

import (
	"testing"

	"novelide/internal/match"
	"novelide/internal/model"
	"novelide/internal/nlp"
)

// These tests cover what POS tagging enables beyond the old curated verb
// lists: the open verb class, adverb skipping, passive voice, perfect
// tense, possessives, and noun/verb disambiguation.

func nlpWorkspace() *model.Workspace {
	return &model.Workspace{
		Manifest: model.Manifest{Books: []string{"book-one", "book-two"}},
		Books: []model.Book{
			{ID: "book-one", Title: "One", Chapters: []string{"01.md", "02.md"}},
			{ID: "book-two", Title: "Two", Chapters: []string{"01.md"}},
		},
		Schema: model.DefaultSchema(),
		Codex: []model.CodexEntry{
			{
				ID: "aria", Name: "Aria", Type: "character",
				Status: []model.StatusChange{
					{State: "alive"},
					{State: "dead", At: model.StoryPoint{Book: "book-one", Chapter: "02.md"}},
				},
			},
			{ID: "kael", Name: "Kael", Type: "character"},
		},
	}
}

func checkText(ws *model.Workspace, text string) []Flag {
	m := match.New(ws.Codex)
	doc, err := nlp.Parse(text)
	if err != nil {
		panic(err)
	}
	return Check(ws, "book-two", "01.md", m.Scan(text), doc)
}

func suggestText(ws *model.Workspace, text string) []Suggestion {
	m := match.New(ws.Codex)
	doc, err := nlp.Parse(text)
	if err != nil {
		panic(err)
	}
	return Suggest(ws, "book-one", "01.md", m.Scan(text), doc)
}

func errorCount(flags []Flag) int {
	n := 0
	for _, f := range flags {
		if f.Severity == SevError {
			n++
		}
	}
	return n
}

func TestOpenVerbClassAgency(t *testing.T) {
	ws := nlpWorkspace()
	// None of these verbs were in the old curated list.
	for _, text := range []string{
		"Aria sauntered across the courtyard.",
		"Aria conjured a wall of flame.",
		"Aria slowly walked into the hall.", // adverb between name and verb
		"Aria was walking toward him.",      // progressive
	} {
		if errorCount(checkText(ws, text)) != 1 {
			t.Errorf("%q: expected agency error, got %+v", text, checkText(ws, text))
		}
	}
}

func TestNonAgencyStaysInfo(t *testing.T) {
	ws := nlpWorkspace()
	for _, text := range []string{
		"Aria was dead.",                      // copula + adjective
		"Aria's sword hung on the wall.",      // possessive
		"Aria was carried out of the hall.",   // passive — acted upon
		"Aria had walked these halls once.",   // perfect tense — recollection
		"Aria would have wanted this.",        // modal — counterfactual
		"He missed Aria.",                     // object position
		"Aria lay lifeless among the ashes.",  // semi-linking + adjective
		"Aria was a legend in the ash-farms.", // copula + noun phrase
	} {
		flags := checkText(ws, text)
		if errorCount(flags) != 0 {
			t.Errorf("%q: expected no agency error, got %+v", text, flags)
		}
		if len(flags) == 0 {
			t.Errorf("%q: expected an info mention", text)
		}
	}
}

func TestSemiLinkingActionStillFlags(t *testing.T) {
	ws := nlpWorkspace()
	if errorCount(checkText(ws, "Aria lay down beside the fire.")) != 1 {
		t.Error("'lay down' is an action and should flag")
	}
}

func TestNounLoveNotSuggested(t *testing.T) {
	ws := nlpWorkspace()
	text := "Kael's love for Aria never wavered."
	ss := suggestText(ws, text)
	for _, s := range ss {
		if s.Relation == "loves" {
			t.Errorf("noun 'love' should not suggest a relationship: %+v", s)
		}
	}
}

func TestNewEntitySuggestedFromNER(t *testing.T) {
	ws := nlpWorkspace()
	text := "John Carter rode north at dawn. By nightfall John Carter had reached the river."
	ss := suggestText(ws, text)
	found := false
	for _, s := range ss {
		if s.Kind == "entity" && s.Name == "John Carter" {
			found = true
		}
	}
	if !found {
		t.Errorf("repeated unknown name should suggest a codex entry, got %+v", ss)
	}
}

func TestKnownAndSingleNamesNotSuggested(t *testing.T) {
	ws := nlpWorkspace()
	// Kael is in the codex; "Mirella" appears only once.
	text := "Kael spoke with Kael again. Mirella watched from the door."
	for _, s := range suggestText(ws, text) {
		if s.Kind == "entity" {
			t.Errorf("unexpected entity suggestion: %+v", s)
		}
	}
}
