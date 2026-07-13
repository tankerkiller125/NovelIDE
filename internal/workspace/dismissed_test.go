package workspace

import (
	"testing"

	"novelide/internal/model"
)

func TestDismissedPersistence(t *testing.T) {
	dir := t.TempDir()
	if _, err := Create(dir, "Book", model.KindNovel); err != nil {
		t.Fatal(err)
	}

	if got := LoadDismissed(dir); len(got) != 0 {
		t.Fatalf("fresh workspace should have no dismissals, got %v", got)
	}

	// Adding is idempotent and returned sorted.
	if _, err := AddDismissed(dir, "status|aria|dead"); err != nil {
		t.Fatal(err)
	}
	AddDismissed(dir, "relation|kael|killed|aria")
	got, err := AddDismissed(dir, "status|aria|dead") // dup
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 unique keys, got %v", got)
	}

	// It survives a reload and rides along on the loaded workspace.
	ws, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Dismissed) != 2 {
		t.Errorf("workspace.Dismissed should carry 2 keys, got %v", ws.Dismissed)
	}

	// Removing un-dismisses.
	got, _ = RemoveDismissed(dir, "status|aria|dead")
	if len(got) != 1 || got[0] != "relation|kael|killed|aria" {
		t.Errorf("remove wrong: %v", got)
	}
}
