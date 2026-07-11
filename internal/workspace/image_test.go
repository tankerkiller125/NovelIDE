package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"novelide/internal/model"
)

func TestEntryImageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := Create(dir, "Art", model.KindNovel); err != nil {
		t.Fatal(err)
	}
	// a tiny fake PNG source
	src := filepath.Join(t.TempDir(), "portrait.png")
	if err := os.WriteFile(src, []byte("\x89PNG\r\n\x1a\nfake"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := &model.CodexEntry{Name: "Aria Voss", Type: "character", Scope: ScopeSeries}
	if err := SetEntryImage(dir, e, src); err != nil {
		t.Fatal(err)
	}
	if e.Image != "assets/aria-voss.png" {
		t.Fatalf("image path = %q", e.Image)
	}
	// persisted on the entry and copied into the workspace
	ws, _ := Load(dir)
	var found bool
	for _, ce := range ws.Codex {
		if ce.ID == "aria-voss" {
			found = true
			if ce.Image != "assets/aria-voss.png" {
				t.Errorf("saved image = %q", ce.Image)
			}
		}
	}
	if !found {
		t.Fatal("entry not saved")
	}
	b, err := ReadImage(dir, "assets/aria-voss.png")
	if err != nil || len(b) == 0 {
		t.Errorf("read image: %v (%d bytes)", err, len(b))
	}

	// path traversal is refused
	if _, err := ReadImage(dir, "../../../etc/passwd"); err == nil {
		t.Error("path traversal not rejected")
	}
	if _, err := ReadImage(dir, "/etc/passwd"); err == nil {
		t.Error("absolute path not rejected")
	}

	// unsupported type
	bad := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(bad, []byte("x"), 0o644)
	if err := SetEntryImage(dir, e, bad); err == nil {
		t.Error("non-image extension should be rejected")
	}

	// clear removes the file and the reference
	if err := ClearEntryImage(dir, e); err != nil {
		t.Fatal(err)
	}
	if e.Image != "" {
		t.Errorf("image not cleared: %q", e.Image)
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "aria-voss.png")); !os.IsNotExist(err) {
		t.Error("image file not removed on clear")
	}
}

// A malicious/hand-edited image path must never let ClearEntryImage (or the
// stale-file removal in SetEntryImage) delete a file outside the workspace.
func TestEntryImagePathTraversalOnDelete(t *testing.T) {
	dir := t.TempDir()
	if _, err := Create(dir, "Art", model.KindNovel); err != nil {
		t.Fatal(err)
	}
	// A sentinel file living OUTSIDE the workspace that must survive.
	outside := t.TempDir()
	victim := filepath.Join(outside, "important.txt")
	if err := os.WriteFile(victim, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Craft an entry whose Image escapes the workspace to the victim file.
	rel := "../../" + filepath.Base(outside) + "/important.txt"
	e := &model.CodexEntry{ID: "evil", Name: "Evil", Type: "character", Scope: ScopeSeries, Image: rel}

	if err := ClearEntryImage(dir, e); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("path traversal deleted a file outside the workspace: %v", err)
	}
	if e.Image != "" {
		t.Errorf("reference should still be cleared: %q", e.Image)
	}
}
