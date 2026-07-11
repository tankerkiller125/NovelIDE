package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, ws, rel, content string) {
	t.Helper()
	p := filepath.Join(ws, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotLifecycle(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "books/01/manuscript/01-one.md", "# One\n\nHello world.\n")
	write(t, ws, "codex/character/aria.yaml", "id: aria\nname: Aria\n")

	now := time.Date(2026, 7, 11, 15, 30, 0, 0, time.UTC)
	snap, created, err := Create(ws, "first", false, now)
	if err != nil || !created {
		t.Fatalf("create: created=%v err=%v", created, err)
	}
	if snap.Files != 2 {
		t.Errorf("want 2 files captured, got %d", snap.Files)
	}

	// Re-creating with no changes is a no-op (dedup).
	if _, created, _ := Create(ws, "again", false, now.Add(time.Minute)); created {
		t.Error("unchanged workspace should not create a second snapshot")
	}

	// Modify a chapter and add a new file.
	write(t, ws, "books/01/manuscript/01-one.md", "# One\n\nHello, brave world.\n")
	write(t, ws, "books/01/manuscript/02-two.md", "# Two\n")

	changes, err := Changes(ws, snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	byRel := map[string]string{}
	for _, c := range changes {
		byRel[c.Rel] = c.Status
	}
	if byRel["books/01/manuscript/01-one.md"] != "modified" {
		t.Errorf("chapter one should be modified: %+v", changes)
	}
	if byRel["books/01/manuscript/02-two.md"] != "added" {
		t.Errorf("chapter two should be added: %+v", changes)
	}

	// Diff of the modified chapter shows the changed line as del+add.
	diff, err := FileDiff(ws, snap.ID, "books/01/manuscript/01-one.md")
	if err != nil {
		t.Fatal(err)
	}
	var adds, dels int
	for _, l := range diff.Lines {
		switch l.Op {
		case "add":
			adds++
		case "del":
			dels++
		}
	}
	if adds != 1 || dels != 1 {
		t.Errorf("expected one changed line (1 add, 1 del), got %d/%d: %+v", adds, dels, diff.Lines)
	}

	// Restore everything back to the snapshot.
	n, err := Restore(ws, snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 { // one modified restored, one added removed
		t.Errorf("expected 2 files restored, got %d", n)
	}
	got, _ := os.ReadFile(filepath.Join(ws, "books/01/manuscript/01-one.md"))
	if string(got) != "# One\n\nHello world.\n" {
		t.Errorf("chapter not reverted: %q", got)
	}
	if _, err := os.Stat(filepath.Join(ws, "books/01/manuscript/02-two.md")); !os.IsNotExist(err) {
		t.Error("added chapter should have been removed on full restore")
	}
}

func TestDeleteGCsBlobs(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "a.md", "alpha\n")
	s1, _, _ := Create(ws, "s1", false, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	write(t, ws, "a.md", "beta\n")
	s2, _, _ := Create(ws, "s2", false, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	// Two distinct contents → two blobs.
	entries, _ := os.ReadDir(blobDir(ws))
	if len(entries) != 2 {
		t.Fatalf("expected 2 blobs, got %d", len(entries))
	}
	if err := Delete(ws, s1.ID); err != nil {
		t.Fatal(err)
	}
	// s1's unique blob is now unreferenced and collected; s2's remains.
	entries, _ = os.ReadDir(blobDir(ws))
	if len(entries) != 1 {
		t.Errorf("expected 1 blob after GC, got %d", len(entries))
	}
	list, _ := List(ws)
	if len(list) != 1 || list[0].ID != s2.ID {
		t.Errorf("index should contain only s2: %+v", list)
	}
	if _, err := os.Stat(manifestPath(ws, s1.ID)); !os.IsNotExist(err) {
		t.Error("deleted snapshot manifest should be gone")
	}
}

func TestPathJailOnRestore(t *testing.T) {
	ws := t.TempDir()
	if err := RestoreFile(ws, "x", "../escape.md"); err == nil {
		t.Error("restore outside workspace must be rejected")
	}
}
