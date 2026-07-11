package workspace

import (
	"testing"

	"novelide/internal/model"
)

func TestSchemaAndRelationsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ws, err := Create(dir, "Dune-ish", model.KindSeries)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Schema.Types) == 0 {
		t.Fatal("new workspace should carry the default schema")
	}

	// Add custom types and a custom relation, as a Dune-scale project would.
	schema := ws.Schema
	schema.Types = append(schema.Types,
		model.TypeDef{ID: "house", Label: "Great House", Icon: "🏛"},
		model.TypeDef{ID: "planet", Label: "Planet", Icon: "🪐"},
	)
	schema.Relations = append(schema.Relations,
		model.RelationDef{ID: "heir-of", Label: "heir of", InverseLabel: "names as heir"},
		model.RelationDef{Label: "bonded to", Symmetric: true}, // id auto-slugged
	)
	if err := SaveSchema(dir, schema); err != nil {
		t.Fatal(err)
	}

	house := &model.CodexEntry{Name: "House Atreides", Type: "house", Scope: ScopeSeries}
	if err := SaveEntry(dir, house); err != nil {
		t.Fatal(err)
	}
	paul := &model.CodexEntry{
		Name: "Paul", Type: "character", Scope: ScopeSeries,
		Relations: []model.Relation{
			{Type: "member-of", To: "house-atreides"},
			{Type: "heir-of", To: "house-atreides", From: &model.StoryPoint{Book: ws.Books[0].ID}},
		},
	}
	if err := SaveEntry(dir, paul); err != nil {
		t.Fatal(err)
	}

	ws2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	ids := ws2.Schema.TypeIDs()
	if len(ids) != 8 { // 6 defaults (incl. arc) + 2 custom
		t.Fatalf("custom types not persisted: %v", ids)
	}
	var bonded bool
	for _, r := range ws2.Schema.Relations {
		if r.ID == "bonded-to" && r.Symmetric {
			bonded = true
		}
	}
	if !bonded {
		t.Error("relation id was not auto-slugged from label")
	}
	for _, e := range ws2.Codex {
		if e.ID == "paul" {
			if len(e.Relations) != 2 || e.Relations[1].From == nil || e.Relations[1].From.Book != ws.Books[0].ID {
				t.Errorf("relations round-trip failed: %+v", e.Relations)
			}
			return
		}
	}
	t.Error("paul not found after reload")
}
