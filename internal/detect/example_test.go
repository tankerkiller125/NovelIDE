package detect_test

import (
	"path/filepath"
	"strings"
	"testing"

	"novelide/internal/detect"
	"novelide/internal/match"
	"novelide/internal/nlp"
	"novelide/internal/workspace"
)

// TestSeededManuscriptMistakes proves the deliberate continuity errors in
// the Saltglass Chronicles example manuscripts are actually caught by the
// detection engine — so the shipped example demonstrates every feature.
func TestSeededManuscriptMistakes(t *testing.T) {
	dir, err := filepath.Abs("../../examples/saltglass-chronicles")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	matcher := match.New(ws.Codex)

	scan := func(book, chapter string) ([]detect.Flag, []detect.Suggestion) {
		text, err := workspace.ReadChapter(dir, book, chapter)
		if err != nil {
			t.Fatalf("read %s/%s: %v", book, chapter, err)
		}
		doc, err := nlp.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		spans := matcher.Scan(text)
		flags := detect.Check(ws, book, chapter, spans, doc)
		sugs := detect.Suggest(ws, book, chapter, spans, doc)
		exSug, exFlags := detect.Extract(ws, book, chapter, spans, doc)
		return append(flags, exFlags...), append(sugs, exSug...)
	}

	hasFlag := func(flags []detect.Flag, rule, entry string) bool {
		for _, f := range flags {
			if f.Rule == rule && f.EntryID == entry {
				return true
			}
		}
		return false
	}
	hasSug := func(sugs []detect.Suggestion, pred func(detect.Suggestion) bool) bool {
		for _, s := range sugs {
			if pred(s) {
				return true
			}
		}
		return false
	}

	// ---- Book 5: "The Empty Chair" ----
	f5, s5 := scan("05-the-drowned-court", "01-the-empty-chair.md")

	// Dorian Vell died in book 3 but acts here — the flagship error.
	if !hasFlag(f5, "dead-entity-agency", "dorian-vell") {
		t.Errorf("book 5: expected dead-entity-agency for dorian-vell, got %+v", f5)
	}
	// Halden Brooke (dead in book 4) is only mentioned — info, never an error.
	if hasFlag(f5, "dead-entity-agency", "halden-brooke") {
		t.Error("book 5: Halden Brooke is only mentioned; should not raise an agency error")
	}
	// "Vespera Locke killed Perrin Marsh" — kill relation + victim death.
	if !hasSug(s5, func(s detect.Suggestion) bool {
		return s.Kind == "relation" && s.Relation == "killed" &&
			s.EntryID == "vespera-locke" && s.TargetID == "perrin-marsh"
	}) {
		t.Errorf("book 5: expected killed(vespera->perrin) suggestion, got %+v", s5)
	}
	if !hasSug(s5, func(s detect.Suggestion) bool {
		return s.Kind == "status" && s.EntryID == "perrin-marsh" && s.State == "dead"
	}) {
		t.Error("book 5: expected a death-status suggestion for perrin-marsh")
	}
	// "Corin Sedgewick" appears twice and has no codex entry.
	if !hasSug(s5, func(s detect.Suggestion) bool {
		return s.Kind == "entity" && strings.Contains(s.Name, "Sedgewick")
	}) {
		t.Errorf("book 5: expected a new-entity suggestion for Corin Sedgewick, got %+v", s5)
	}
	// "Odile Sarkany's hair was fire-red" — appearance field suggestion.
	if !hasSug(s5, func(s detect.Suggestion) bool {
		return s.Kind == "field" && s.EntryID == "odile-sarkany" && s.FieldKey == "hair"
	}) {
		t.Errorf("book 5: expected a hair field suggestion for odile-sarkany, got %+v", s5)
	}

	// ---- Book 7: "After the Spire" ----
	f7, s7 := scan("07-the-last-canto", "01-after-the-spire.md")

	// Eamon Hollis died in book 6 but acts here.
	if !hasFlag(f7, "dead-entity-agency", "eamon-hollis") {
		t.Errorf("book 7: expected dead-entity-agency for eamon-hollis, got %+v", f7)
	}
	// "Her hair was pale gold" contradicts the codex (dark) — a warning.
	if !hasFlag(f7, "field-contradiction", "wren-alcott") {
		t.Errorf("book 7: expected a hair field-contradiction for wren-alcott, got %+v", f7)
	}
	// "Perrin Marsh had married Odile Sarkany" — marriage suggestion.
	if !hasSug(s7, func(s detect.Suggestion) bool {
		return s.Kind == "relation" && s.Relation == "married-to" &&
			(s.EntryID == "perrin-marsh" || s.TargetID == "perrin-marsh")
	}) {
		t.Errorf("book 7: expected a married-to suggestion for perrin/odile, got %+v", s7)
	}
	// "Cade Marsh was Perrin's brother" — kinship suggestion.
	if !hasSug(s7, func(s detect.Suggestion) bool {
		return s.Kind == "relation" && s.Relation == "sibling-of" &&
			s.EntryID == "cade-marsh" && s.TargetID == "perrin-marsh"
	}) {
		t.Errorf("book 7: expected a sibling-of suggestion (cade->perrin), got %+v", s7)
	}
}
