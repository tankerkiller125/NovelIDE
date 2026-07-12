package syncclient

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"novelide/internal/syncproto"
	"novelide/internal/syncserver"
)

func testServer(t *testing.T) string {
	t.Helper()
	srv, err := syncserver.New(syncserver.Config{
		DataDir:           t.TempDir(),
		Secret:            []byte("test"),
		AllowRegistration: true,
		TokenTTL:          time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts.URL
}

func writeWS(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readWS(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTwoDeviceSync(t *testing.T) {
	url := testServer(t)
	c := New(url, "")
	auth, err := c.Register("alice", "hunter2pw")
	if err != nil {
		t.Fatal(err)
	}
	acct := auth.AccountID
	// Both "devices" use the same account/token.
	devA, devB := t.TempDir(), t.TempDir()
	ca, cb := New(url, c.Token), New(url, c.Token)

	// Device A creates the workspace and pushes it.
	writeWS(t, devA, "books/01/manuscript/01-one.md", "v1")
	if _, err := LinkPush(devA, "story", ca, acct); err != nil {
		t.Fatal(err)
	}

	// Device B joins by pulling.
	if _, err := LinkPull(devB, "story", cb, acct); err != nil {
		t.Fatal(err)
	}
	if got := readWS(t, devB, "books/01/manuscript/01-one.md"); got != "v1" {
		t.Fatalf("B should have v1, got %q", got)
	}

	// B edits and syncs; A syncs and should receive B's change.
	writeWS(t, devB, "books/01/manuscript/01-one.md", "v2-from-b")
	if _, err := Sync(devB, cb, acct); err != nil {
		t.Fatal(err)
	}
	res, err := Sync(devA, ca, acct)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pulled != 1 {
		t.Errorf("A should have pulled 1 file, got %d", res.Pulled)
	}
	if got := readWS(t, devA, "books/01/manuscript/01-one.md"); got != "v2-from-b" {
		t.Fatalf("A should have v2-from-b, got %q", got)
	}
}

func TestConflictCreatesCopy(t *testing.T) {
	url := testServer(t)
	c := New(url, "")
	auth, _ := c.Register("alice", "hunter2pw")
	acct := auth.AccountID
	devA, devB := t.TempDir(), t.TempDir()
	ca, cb := New(url, c.Token), New(url, c.Token)

	writeWS(t, devA, "ch.md", "base")
	LinkPush(devA, "story", ca, acct)
	LinkPull(devB, "story", cb, acct)

	// Both edit the same file differently, from the same base.
	writeWS(t, devA, "ch.md", "A-edit")
	writeWS(t, devB, "ch.md", "B-edit")

	// B pushes first.
	if _, err := Sync(devB, cb, acct); err != nil {
		t.Fatal(err)
	}
	// A syncs into the divergence → conflict.
	res, err := Sync(devA, ca, acct)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0] != "ch.md" {
		t.Fatalf("expected a conflict on ch.md, got %+v", res.Conflicts)
	}
	// A keeps its own edit at the original path.
	if got := readWS(t, devA, "ch.md"); got != "A-edit" {
		t.Errorf("A should keep A-edit, got %q", got)
	}
	// The remote version lands as a conflict copy — nothing is lost.
	copies, _ := filepath.Glob(filepath.Join(devA, "ch (conflict*.md"))
	if len(copies) != 1 {
		t.Fatalf("expected one conflict copy, found %v", copies)
	}
	if b, _ := os.ReadFile(copies[0]); string(b) != "B-edit" {
		t.Errorf("conflict copy should hold B-edit, got %q", b)
	}
}

func TestRejectsUnsafeRemotePaths(t *testing.T) {
	bad := []string{
		"../escape.md",
		"../../etc/passwd",
		"/etc/passwd",
		"a/../../b.md",
		`..\windows.md`,
		"",
	}
	for _, p := range bad {
		if validRel(p) {
			t.Errorf("validRel(%q) = true, want false", p)
		}
		if _, err := safeJoin(t.TempDir(), p); err == nil {
			t.Errorf("safeJoin allowed unsafe path %q", p)
		}
	}
	for _, p := range []string{"a.md", "books/01/manuscript/01-one.md", "assets/map.png"} {
		if !validRel(p) {
			t.Errorf("validRel(%q) = false, want true", p)
		}
	}

	// A hostile manifest is refused wholesale before any file is touched.
	err := checkRemoteManifest([]syncproto.FileEntry{
		{Path: "ok.md", Hash: "x"},
		{Path: "../../../home/victim/.ssh/authorized_keys", Hash: "y"},
	})
	if err == nil {
		t.Fatal("checkRemoteManifest accepted a traversal path")
	}

	// writeFile/removeFile refuse to escape the workspace even if reached
	// directly, and never create the out-of-tree file.
	ws := t.TempDir()
	if err := writeFile(ws, "../pwned.md", []byte("x")); err == nil {
		t.Error("writeFile wrote outside the workspace")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(ws), "pwned.md")); !os.IsNotExist(err) {
		t.Error("an out-of-workspace file was created")
	}
}

func TestSyncRequiresLink(t *testing.T) {
	url := testServer(t)
	c := New(url, "")
	c.Register("alice", "hunter2pw")
	ws := t.TempDir()
	writeWS(t, ws, "a.md", "x")
	if _, err := Sync(ws, New(url, c.Token), "acct"); err == nil {
		t.Error("Sync on an unlinked workspace should error")
	}
}

// TestSecondUserDoesNotDeleteFiles reproduces the data-loss bug: syncing a
// folder as one account, then syncing the same folder as a *different* account
// (whose remote is empty) must NOT interpret the stale base as "remote deleted
// everything" and wipe the local workspace.
func TestSecondUserDoesNotDeleteFiles(t *testing.T) {
	url := testServer(t)
	reg := New(url, "")
	a, _ := reg.Register("alice", "hunter2pw")
	b, _ := reg.Register("bob", "correcthorse")

	ws := t.TempDir()
	writeWS(t, ws, "novelide.yaml", "name: Saltglass\n")
	writeWS(t, ws, "books/01/manuscript/01-one.md", "chapter one")

	// Alice links + syncs the folder (pushes; base now records all files).
	ca := New(url, a.Token)
	if err := Link(ws, "saltglass", url, a.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ws, ca, a.AccountID); err != nil {
		t.Fatal(err)
	}

	// Now Bob (a different account, empty remote) syncs the SAME folder.
	cb := New(url, b.Token)
	res, err := Sync(ws, cb, b.AccountID)
	if err != nil {
		t.Fatalf("bob sync errored: %v", err)
	}
	if res.Deleted != 0 {
		t.Errorf("bob's sync deleted %d files — must delete none", res.Deleted)
	}
	// The workspace is intact...
	if got := readWS(t, ws, "novelide.yaml"); got != "name: Saltglass\n" {
		t.Fatalf("novelide.yaml was damaged/removed: %q", got)
	}
	readWS(t, ws, "books/01/manuscript/01-one.md") // fatal if missing
	// ...and Bob now has his own remote copy.
	if res.Pushed == 0 {
		t.Error("bob's sync should have pushed the workspace to his account")
	}
	m, _ := cb.Manifest("saltglass")
	if len(m.Files) != 2 {
		t.Errorf("bob's remote should hold 2 files, got %d", len(m.Files))
	}
}
