package workspace

import (
	"testing"

	"novelide/internal/model"
)

// TestTimelinedFieldRoundTrip ensures a field-timeline survives a save/load
// cycle (YAML), and that plain static fields still load unchanged.
func TestTimelinedFieldRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := Create(dir, "Book", model.KindNovel); err != nil {
		t.Fatal(err)
	}

	entry := &model.CodexEntry{
		ID: "aria", Name: "Aria", Type: "character", Scope: ScopeSeries,
		Fields: map[string]string{"hair": "black"}, // static fact still works
		FieldTimelines: map[string][]model.TimedValue{
			"age": {
				{Value: "17"},
				{Value: "18", At: &model.StoryPoint{Book: "01-book", Chapter: "05-c.md"}},
			},
		},
	}
	if err := SaveEntry(dir, entry); err != nil {
		t.Fatal(err)
	}

	ws, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got *model.CodexEntry
	for i := range ws.Codex {
		if ws.Codex[i].ID == "aria" {
			got = &ws.Codex[i]
		}
	}
	if got == nil {
		t.Fatal("entry not reloaded")
	}
	if got.Fields["hair"] != "black" {
		t.Errorf("static field lost: %v", got.Fields)
	}
	tl := got.FieldTimelines["age"]
	if len(tl) != 2 || tl[0].Value != "17" || tl[1].Value != "18" {
		t.Fatalf("age timeline wrong: %+v", tl)
	}
	if tl[0].At != nil {
		t.Errorf("first value should be unanchored (from the start), got %+v", tl[0].At)
	}
	if tl[1].At == nil || tl[1].At.Book != "01-book" || tl[1].At.Chapter != "05-c.md" {
		t.Errorf("second value anchor wrong: %+v", tl[1].At)
	}
}
