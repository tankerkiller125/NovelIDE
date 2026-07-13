package detect

import (
	"testing"

	"novelide/internal/match"
	"novelide/internal/model"
	"novelide/internal/nlp"
)

func extractWorkspace() *model.Workspace {
	return &model.Workspace{
		Manifest: model.Manifest{Books: []string{"book-one"}},
		Books:    []model.Book{{ID: "book-one", Chapters: []string{"01.md"}}},
		Schema:   model.DefaultSchema(),
		Codex: []model.CodexEntry{
			{ID: "aria", Name: "Aria", Type: "character"},
			{ID: "kael", Name: "Kael", Type: "character"},
			{ID: "mira", Name: "Mira", Type: "character",
				Fields: map[string]string{"hair": "copper-red", "gender": "female"}},
			{ID: "ashblade", Name: "the Ashblade", Type: "item"},
			{ID: "cinder-guard", Name: "the Cinder Guard", Type: "faction"},
		},
	}
}

func extract(ws *model.Workspace, text string) ([]Suggestion, []Flag) {
	m := match.New(ws.Codex)
	doc, err := nlp.Parse(text)
	if err != nil {
		panic(err)
	}
	return Extract(ws, "book-one", "01.md", m.Scan(text), doc)
}

func suggestionKeys(ss []Suggestion) map[string]bool {
	out := map[string]bool{}
	for _, s := range ss {
		out[s.Key] = true
	}
	return out
}

func timelineAgeWorkspace(vals []model.TimedValue) *model.Workspace {
	return &model.Workspace{
		Manifest: model.Manifest{Books: []string{"book-one"}},
		Books:    []model.Book{{ID: "book-one", Chapters: []string{"01.md"}}},
		Schema:   model.DefaultSchema(),
		Codex: []model.CodexEntry{
			{ID: "aria", Name: "Aria", Type: "character",
				FieldTimelines: map[string][]model.TimedValue{"age": vals}},
		},
	}
}

func TestTimelinedFieldNotResuggested(t *testing.T) {
	// Age already tracked on a timeline → the manuscript stating it must NOT
	// re-suggest saving it as a static fact.
	ws := timelineAgeWorkspace([]model.TimedValue{{Value: "seven"}})
	ss, flags := extract(ws, "Aria was seven years old.")
	for _, s := range ss {
		if s.FieldKey == "age" {
			t.Errorf("age on a timeline was re-suggested: %+v", s)
		}
	}
	for _, f := range flags {
		if f.Rule == "field-contradiction" {
			t.Errorf("matching timelined age wrongly flagged: %+v", f)
		}
	}
}

func TestTimelinedFieldContradiction(t *testing.T) {
	// A timelined value that disagrees with the prose at this point still flags.
	ws := timelineAgeWorkspace([]model.TimedValue{{Value: "ten"}})
	_, flags := extract(ws, "Aria was seven years old.")
	found := false
	for _, f := range flags {
		if f.Rule == "field-contradiction" {
			found = true
		}
	}
	if !found {
		t.Error("expected a contradiction between the timelined age and the prose")
	}
}

func TestAppearancePossessive(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Aria's hair was copper-red.")
	if !suggestionKeys(ss)["field|aria|hair|copper-red"] {
		t.Errorf("expected hair field suggestion, got %+v", ss)
	}
}

func TestAppearancePronounPossessive(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Aria stepped into the light. Her eyes were green.")
	if !suggestionKeys(ss)["field|aria|eyes|green"] {
		t.Errorf("expected eyes field via pronoun resolution, got %+v", ss)
	}
}

func TestPronounAmbiguityAbstains(t *testing.T) {
	ws := extractWorkspace()
	// Two characters in scope — "her" must not resolve to either.
	ss, _ := extract(ws, "Aria faced Mira across the table. Her eyes were green.")
	k := suggestionKeys(ss)
	if k["field|aria|eyes|green"] || k["field|mira|eyes|green"] {
		t.Errorf("ambiguous pronoun should abstain, got %+v", ss)
	}
}

func TestAppearanceAdjectiveNounOrder(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Aria pushed back. Her silver hair caught the light.")
	if !suggestionKeys(ss)["field|aria|hair|silver"] {
		t.Errorf("expected 'her silver hair' to yield a field, got %+v", ss)
	}
}

func TestAppearanceAdjectiveChains(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws,
		"They began riding with Aria's red long hair flowing behind her, and Kael's short brown hair gently waving in the wind.")
	k := suggestionKeys(ss)
	if !k["field|aria|hair|red long"] {
		t.Errorf("chained adjectives before body part missed for Aria: %+v", ss)
	}
	if !k["field|kael|hair|short brown"] {
		t.Errorf("chained adjectives before body part missed for Kael: %+v", ss)
	}
}

func TestAppearanceCopulaChain(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Aria's hair was long and silver.")
	if !suggestionKeys(ss)["field|aria|hair|long and silver"] {
		t.Errorf("copula descriptor chain missed: %+v", ss)
	}
}

func TestFieldContradictionFlagged(t *testing.T) {
	ws := extractWorkspace()
	_, flags := extract(ws, "Mira's hair was silver.")
	found := false
	for _, f := range flags {
		if f.Rule == "field-contradiction" && f.Severity == SevWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("codex says copper-red; silver should flag, got %+v", flags)
	}
}

func TestCompatibleValueSilent(t *testing.T) {
	ws := extractWorkspace()
	ss, flags := extract(ws, "Mira's hair was copper.")
	for _, f := range flags {
		if f.Rule == "field-contradiction" {
			t.Errorf("'copper' vs 'copper-red' should be compatible: %+v", f)
		}
	}
	if suggestionKeys(ss)["field|mira|hair|copper"] {
		t.Errorf("recorded field should not be re-suggested: %+v", ss)
	}
}

func TestHeightBuildAge(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Aria was tall. Kael was 31 years old.")
	k := suggestionKeys(ss)
	if !k["field|aria|height|tall"] {
		t.Errorf("height missing: %+v", ss)
	}
	if !k["field|kael|age|31"] {
		t.Errorf("age missing: %+v", ss)
	}
}

func TestGenderFromPronouns(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Aria drew her blade. She stepped forward. She did not look back.")
	if !suggestionKeys(ss)["field|aria|gender|female"] {
		t.Errorf("expected gender suggestion from pronoun evidence, got %+v", ss)
	}
}

func TestGenderFromHonorific(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Lord Kael rose to speak.")
	if !suggestionKeys(ss)["field|kael|gender|male"] {
		t.Errorf("honorific should be strong gender evidence, got %+v", ss)
	}
}

func TestRecordedGenderNotResuggested(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Mira raised her hand. She waited. She sighed.")
	if suggestionKeys(ss)["field|mira|gender|female"] {
		t.Errorf("gender already recorded, got %+v", ss)
	}
}

func TestKinshipCopular(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Kael was Aria's brother.")
	if !suggestionKeys(ss)["relation|kael|sibling-of|aria"] {
		t.Errorf("expected sibling relation, got %+v", ss)
	}
}

func TestKinshipDirectionality(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Mira was Aria's mother.")
	if !suggestionKeys(ss)["relation|mira|parent-of|aria"] {
		t.Errorf("mother should point parent-of at the possessor, got %+v", ss)
	}
	ss2, _ := extract(ws, "Kael was Mira's son.")
	if !suggestionKeys(ss2)["relation|mira|parent-of|kael"] {
		t.Errorf("son should reverse the direction, got %+v", ss2)
	}
}

func TestKinshipAppositive(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Aria's brother Kael grinned.")
	if !suggestionKeys(ss)["relation|kael|sibling-of|aria"] {
		t.Errorf("appositive kinship missed, got %+v", ss)
	}
}

func TestOwnershipAndMembership(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Kael wielded the Ashblade. Aria joined the Cinder Guard.")
	k := suggestionKeys(ss)
	if !k["relation|kael|owns|ashblade"] {
		t.Errorf("wielded should suggest owns, got %+v", ss)
	}
	if !k["relation|aria|member-of|cinder-guard"] {
		t.Errorf("joined should suggest member-of, got %+v", ss)
	}
}

func TestAliasAppositive(t *testing.T) {
	ws := extractWorkspace()
	ss, _ := extract(ws, "Aria, the Ember Witch, smiled at last.")
	if !suggestionKeys(ss)["alias|aria|the ember witch"] {
		t.Errorf("expected alias suggestion, got %+v", ss)
	}
}

func TestKnownAliasNotResuggested(t *testing.T) {
	ws := extractWorkspace()
	ws.Codex[0].Aliases = []string{"the Ember Witch"}
	ss, _ := extract(ws, "Aria, the Ember Witch, smiled at last.")
	if suggestionKeys(ss)["alias|aria|the ember witch"] {
		t.Errorf("existing alias re-suggested: %+v", ss)
	}
}
