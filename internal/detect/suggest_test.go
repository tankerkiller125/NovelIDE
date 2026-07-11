package detect

import (
	"testing"

	"novelide/internal/match"
	"novelide/internal/model"
	"novelide/internal/nlp"
)

func suggestWorkspace() *model.Workspace {
	return &model.Workspace{
		Manifest: model.Manifest{Books: []string{"book-one"}},
		Books:    []model.Book{{ID: "book-one", Chapters: []string{"01.md"}}},
		Schema:   model.DefaultSchema(),
		Codex: []model.CodexEntry{
			{ID: "aria", Name: "Aria", Type: "character"},
			{ID: "kael", Name: "Kael", Type: "character"},
			{ID: "mira", Name: "Mira", Type: "character",
				Status: []model.StatusChange{{State: "dead"}}},
			{ID: "torv", Name: "Torv", Type: "character",
				Relations: []model.Relation{{Type: "killed", To: "aria"}}},
		},
	}
}

func suggest(ws *model.Workspace, text string) []Suggestion {
	m := match.New(ws.Codex)
	doc, err := nlp.Parse(text)
	if err != nil {
		panic(err)
	}
	return Suggest(ws, "book-one", "01.md", m.Scan(text), doc)
}

func keys(ss []Suggestion) map[string]bool {
	out := map[string]bool{}
	for _, s := range ss {
		out[s.Key] = true
	}
	return out
}

func TestDeathSuggested(t *testing.T) {
	ws := suggestWorkspace()
	for _, text := range []string{
		"Aria died at dawn.",
		"By morning, Aria was dead.",
		"Aria had died before anyone reached her.",
	} {
		ss := suggest(ws, text)
		if !keys(ss)["status|aria|dead"] {
			t.Errorf("%q: expected death suggestion, got %+v", text, ss)
		}
	}
}

func TestNegatedAndRecordedDeathsNotSuggested(t *testing.T) {
	ws := suggestWorkspace()
	for _, text := range []string{
		"Aria nearly died.",
		"Aria was not dead.",
		"Mira was dead.", // codex already records Mira's death
	} {
		if ss := suggest(ws, text); len(ss) != 0 {
			t.Errorf("%q: expected no suggestions, got %+v", text, ss)
		}
	}
}

func TestKillSuggestsRelationAndDeath(t *testing.T) {
	ws := suggestWorkspace()
	ss := suggest(ws, "Kael killed Aria in the throne room.")
	k := keys(ss)
	if !k["relation|kael|killed|aria"] || !k["status|aria|dead"] {
		t.Errorf("expected kill relation + death status, got %+v", ss)
	}
}

func TestPassiveKillReversesDirection(t *testing.T) {
	ws := suggestWorkspace()
	ss := suggest(ws, "Aria was killed by Kael.")
	k := keys(ss)
	if !k["relation|kael|killed|aria"] {
		t.Errorf("passive voice should attribute the kill to Kael, got %+v", ss)
	}
	if !k["status|aria|dead"] {
		t.Errorf("victim death missing, got %+v", ss)
	}
}

func TestExistingKillRelationNotResuggested(t *testing.T) {
	ws := suggestWorkspace()
	ss := suggest(ws, "Torv killed Aria.")
	if keys(ss)["relation|torv|killed|aria"] {
		t.Errorf("already-recorded relation resuggested: %+v", ss)
	}
}

func TestMarriageAndLove(t *testing.T) {
	ws := suggestWorkspace()
	k := keys(suggest(ws, "Kael married Aria that spring. Torv loved Mira."))
	if !k["relation|kael|married-to|aria"] {
		t.Error("marriage not suggested")
	}
	if !k["relation|torv|loves|mira"] {
		t.Error("love not suggested")
	}
}

func TestSentenceBoundaryLimitsPairs(t *testing.T) {
	ws := suggestWorkspace()
	// Kill verb in a different sentence from the second entity.
	if ss := suggest(ws, "Kael killed the beast. Aria watched."); len(ss) != 0 {
		t.Errorf("cross-sentence pair should not match, got %+v", ss)
	}
}
