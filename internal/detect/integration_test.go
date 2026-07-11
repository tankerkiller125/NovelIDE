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

// TestDemoWorkspace exercises the full pipeline against the example project
// shipped in the repo: load from disk, scan a chapter, check flags.
func TestDemoWorkspace(t *testing.T) {
	dir, err := filepath.Abs("../../examples/demo-series")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Books) != 2 || len(ws.Codex) != 6 {
		t.Fatalf("demo workspace shape wrong: %d books, %d codex entries", len(ws.Books), len(ws.Codex))
	}
	if len(ws.Books[0].Plan) != 2 {
		t.Fatalf("book one plan not loaded: %+v", ws.Books[0].Plan)
	}
	if len(ws.Schema.Types) == 0 || len(ws.Schema.Relations) == 0 {
		t.Fatal("schema not loaded from codex-schema.yaml")
	}
	var kael *struct {
		relations int
	}
	for _, e := range ws.Codex {
		if e.ID == "kael-dryn" {
			kael = &struct{ relations int }{len(e.Relations)}
		}
	}
	if kael == nil || kael.relations != 3 {
		t.Fatalf("kael-dryn relations not loaded: %+v", kael)
	}

	text, err := workspace.ReadChapter(dir, "02-the-ash-court", "01-what-remains.md")
	if err != nil {
		t.Fatal(err)
	}
	m := match.New(ws.Codex)
	spans := m.Scan(text)
	if len(spans) == 0 {
		t.Fatal("no entity mentions found in demo chapter")
	}
	doc, err := nlp.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	flags := detect.Check(ws, "02-the-ash-court", "01-what-remains.md", spans, doc)

	var agency, mention int
	for _, f := range flags {
		switch f.Rule {
		case "dead-entity-agency":
			agency++
			if !strings.Contains(f.Message, "Aria") {
				t.Errorf("unexpected agency flag: %+v", f)
			}
		case "dead-entity-mention":
			mention++
		}
	}
	if agency == 0 {
		t.Error("'Aria walked into the room' should raise a dead-entity-agency error")
	}
	if mention == 0 {
		t.Error("'Kael missed Aria' should raise info-level mentions")
	}

	// The death chapter itself must stay clean.
	deathText, err := workspace.ReadChapter(dir, "01-the-ember-crown", "02-the-battle-of-cinders.md")
	if err != nil {
		t.Fatal(err)
	}
	deathDoc, err := nlp.Parse(deathText)
	if err != nil {
		t.Fatal(err)
	}
	deathFlags := detect.Check(ws, "01-the-ember-crown", "02-the-battle-of-cinders.md", m.Scan(deathText), deathDoc)
	for _, f := range deathFlags {
		if f.Severity == detect.SevError {
			t.Errorf("death chapter should not error: %+v", f)
		}
	}
}
