package workspace

import (
	"path/filepath"
	"testing"
)

// TestSaltglassExample validates the shipped Saltglass Chronicles codex:
// every entry parses, every relation points at a real entry, and every
// story-time anchor names a real book. This keeps the example honest as it
// grows.
func TestSaltglassExample(t *testing.T) {
	dir, err := filepath.Abs("../../examples/saltglass-chronicles")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Books) != 7 {
		t.Fatalf("want 7 books, got %d", len(ws.Books))
	}
	if len(ws.Schema.Types) != 9 {
		t.Fatalf("want 9 schema types, got %d", len(ws.Schema.Types))
	}
	if len(ws.Codex) < 200 {
		t.Fatalf("expected a comprehensive codex (>=200 entries), got %d", len(ws.Codex))
	}

	books := map[string]bool{}
	for _, b := range ws.Books {
		books[b.ID] = true
	}
	ids := map[string]int{}
	for _, e := range ws.Codex {
		ids[e.ID]++
	}
	for id, n := range ids {
		if n > 1 {
			t.Errorf("duplicate codex id %q (%d files)", id, n)
		}
	}
	relDefs := map[string]bool{}
	for _, r := range ws.Schema.Relations {
		relDefs[r.ID] = true
	}
	typeDefs := map[string]bool{}
	for _, td := range ws.Schema.Types {
		typeDefs[td.ID] = true
	}

	for _, e := range ws.Codex {
		if e.Name == "" {
			t.Errorf("%s: missing name", e.ID)
		}
		if !typeDefs[e.Type] {
			t.Errorf("%s: type %q not in schema", e.ID, e.Type)
		}
		for _, sc := range e.Status {
			if sc.At.Book != "" && !books[sc.At.Book] {
				t.Errorf("%s: status anchor references unknown book %q", e.ID, sc.At.Book)
			}
		}
		for _, r := range e.Relations {
			if ids[r.To] == 0 {
				t.Errorf("%s: relation %q -> unknown entry %q", e.ID, r.Type, r.To)
			}
			if !relDefs[r.Type] {
				t.Errorf("%s: relation type %q not in schema", e.ID, r.Type)
			}
			if r.From != nil && r.From.Book != "" && !books[r.From.Book] {
				t.Errorf("%s: relation 'from' references unknown book %q", e.ID, r.From.Book)
			}
			if r.Until != nil && r.Until.Book != "" && !books[r.Until.Book] {
				t.Errorf("%s: relation 'until' references unknown book %q", e.ID, r.Until.Book)
			}
		}
	}

	// Spot-check the death timeline — deliberately spread across the series
	// so the per-book consistency logic has distinct anchors to exercise.
	deaths := map[string]string{
		"dorian-vell":   "03-the-sablefen-pact",
		"halden-brooke": "04-the-trine-tournament",
		"marisol-quill": "05-the-drowned-court",
		"eamon-hollis":  "06-the-reliquary",
		"cassian-dorn":  "07-the-last-canto",
		"wick-marsh":    "07-the-last-canto",
		"pib":           "07-the-last-canto",
	}
	for _, e := range ws.Codex {
		want, ok := deaths[e.ID]
		if !ok {
			continue
		}
		found := false
		for _, sc := range e.Status {
			if sc.State == "dead" && sc.At.Book == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: expected death anchored to %s", e.ID, want)
		}
		delete(deaths, e.ID)
	}
	for id := range deaths {
		t.Errorf("expected codex entry %q with a death anchor", id)
	}
}
