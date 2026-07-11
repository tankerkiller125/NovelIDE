package workspace

import (
	"testing"

	"novelide/internal/model"
)

func TestCreateLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ws, err := Create(dir, "The Ember Cycle", model.KindSeries)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Manifest.Name != "The Ember Cycle" || len(ws.Books) != 1 {
		t.Fatalf("unexpected workspace: %+v", ws)
	}
	if len(ws.Books[0].Chapters) != 1 {
		t.Fatalf("starter chapter missing: %+v", ws.Books[0])
	}

	entry := &model.CodexEntry{
		Name: "Aria Voss", Type: "character",
		Aliases: []string{"Aria"}, Summary: "Fire mage",
		Status: []model.StatusChange{{State: "alive"}},
		Scope:  ScopeSeries,
	}
	if err := SaveEntry(dir, entry); err != nil {
		t.Fatal(err)
	}
	if entry.ID != "aria-voss" {
		t.Errorf("slug id, got %q", entry.ID)
	}

	bookID := ws.Books[0].ID
	local := &model.CodexEntry{Name: "The Vault", Type: "location", Scope: bookID}
	if err := SaveEntry(dir, local); err != nil {
		t.Fatal(err)
	}

	ws2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws2.Codex) != 2 {
		t.Fatalf("want 2 codex entries, got %+v", ws2.Codex)
	}
	var scopes = map[string]string{}
	for _, e := range ws2.Codex {
		scopes[e.ID] = e.Scope
	}
	if scopes["aria-voss"] != ScopeSeries || scopes["the-vault"] != bookID {
		t.Errorf("scopes wrong: %v", scopes)
	}

	name, err := CreateChapter(dir, bookID, "The Fall")
	if err != nil {
		t.Fatal(err)
	}
	if name != "02-the-fall.md" {
		t.Errorf("chapter name %q", name)
	}
	if err := WriteChapter(dir, bookID, name, "Aria fell.\n"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadChapter(dir, bookID, name)
	if err != nil || got != "Aria fell.\n" {
		t.Errorf("read back %q, err %v", got, err)
	}

	if _, err := ReadChapter(dir, "../escape", "x.md"); err == nil {
		t.Error("path traversal not rejected")
	}

	id2, err := CreateBook(dir, "The Ash Court")
	if err != nil {
		t.Fatal(err)
	}
	ws3, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws3.Books) != 2 || ws3.Books[1].ID != id2 {
		t.Errorf("second book missing: %+v", ws3.Books)
	}
}
