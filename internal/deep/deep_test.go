package deep

import (
	"testing"

	"novelide/internal/model"
)

func TestSuggestEntitiesFiltering(t *testing.T) {
	ws := &model.Workspace{
		Codex: []model.CodexEntry{
			{ID: "aria", Name: "Aria Voss", Aliases: []string{"Aria"}},
		},
	}
	ents := []Entity{
		{Text: "Torin Vale", Label: "PERSON", Score: 0.95},   // keep
		{Text: "Aria Voss", Label: "PERSON", Score: 0.99},    // known
		{Text: "Voss", Label: "PERSON", Score: 0.9},          // overlaps known
		{Text: "Torin Vale", Label: "PERSON", Score: 0.91},   // duplicate
		{Text: "Xu", Label: "PERSON", Score: 0.95},           // too short
		{Text: "Maybe Someone", Label: "PERSON", Score: 0.5}, // low confidence
		{Text: "Emberfall", Label: "GPE", Score: 0.9},        // keep (location)
		{Text: "Old Tongue", Label: "MISC", Score: 0.9},      // MISC dropped
	}
	got := SuggestEntities(ws, ents)
	if len(got) != 2 || got[0].Text != "Torin Vale" || got[1].Text != "Emberfall" {
		t.Errorf("unexpected filtering result: %+v", got)
	}
}

func TestOffsetConverterUnicode(t *testing.T) {
	text := "«Über» Aria"
	conv := offsetConverter(text)
	// "Aria" starts at byte 10 («=2 bytes, Ü=2 bytes, »=2 bytes) = rune 7
	if conv(10) != 7 {
		t.Errorf("byte 10 should map to rune 7, got %d", conv(10))
	}
	ascii := offsetConverter("plain text")
	if ascii(5) != 5 {
		t.Errorf("ascii should be identity")
	}
}
