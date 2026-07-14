package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherDetectsChanges(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "novelide.yaml", "name: X\n")
	write(t, dir, "books/01/manuscript/01-one.md", "hello")

	changes := make(chan Change, 16)
	w := Start(dir, 20*time.Millisecond, func(c Change) { changes <- c })
	defer w.Stop()
	time.Sleep(60 * time.Millisecond) // let the baseline scan settle

	waitFor := func(pred func(Change) bool, msg string) {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case c := <-changes:
				if pred(c) {
					return
				}
			case <-deadline:
				t.Fatalf("timed out: %s", msg)
			}
		}
	}

	// Modify an existing file → Modified.
	write(t, dir, "books/01/manuscript/01-one.md", "hello world")
	waitFor(func(c Change) bool { return has(c.Modified, "books/01/manuscript/01-one.md") },
		"modification not detected")

	// Add a new file → Structural.
	write(t, dir, "codex/character/aria.yaml", "id: aria\n")
	waitFor(func(c Change) bool { return has(c.Structural, "codex/character/aria.yaml") },
		"new file not detected")

	// Remove a file → Structural.
	os.Remove(filepath.Join(dir, "codex/character/aria.yaml"))
	waitFor(func(c Change) bool { return has(c.Structural, "codex/character/aria.yaml") },
		"removal not detected")
}

func TestWatcherIgnoresInternalDirs(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.md", "x")

	changes := make(chan Change, 16)
	w := Start(dir, 20*time.Millisecond, func(c Change) { changes <- c })
	defer w.Stop()

	// Snapshots, sync state, VCS, and temp files must not trigger a change.
	write(t, dir, ".novelide/snapshots/blob", "data")
	write(t, dir, ".git/HEAD", "ref")
	write(t, dir, ".tmp-1234", "partial")

	select {
	case c := <-changes:
		t.Fatalf("internal/temp files should be ignored, got %+v", c)
	case <-time.After(300 * time.Millisecond):
		// good — nothing fired
	}
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
