package syncserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"novelide/internal/syncproto"
)

func newTestServer(t *testing.T, allowReg bool) *httptest.Server {
	t.Helper()
	srv, err := New(Config{
		DataDir:           t.TempDir(),
		Secret:            []byte("test-secret"),
		AllowRegistration: allowReg,
		TokenTTL:          time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

type client struct {
	t     *testing.T
	base  string
	token string
}

func (c *client) do(method, path, token string, body any) (*http.Response, []byte) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		c.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, data
}

func register(t *testing.T, base, user, pw string) *client {
	t.Helper()
	c := &client{t: t, base: base}
	resp, data := c.do("POST", "/api/register", "", syncproto.RegisterRequest{Username: user, Password: pw})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register %s: status %d: %s", user, resp.StatusCode, data)
	}
	var auth syncproto.AuthResponse
	if err := json.Unmarshal(data, &auth); err != nil {
		t.Fatal(err)
	}
	c.token = auth.Token
	return c
}

func TestSyncPushPull(t *testing.T) {
	ts := newTestServer(t, true)
	alice := register(t, ts.URL, "alice", "hunter2pw")

	content := []byte("# Chapter One\n\nThe ash fields stretched east.\n")
	hash := sha256Hex(content)
	files := []syncproto.FileEntry{{Path: "books/01/manuscript/01-one.md", Hash: hash, Size: int64(len(content))}}

	// Commit before uploading the blob → 422 with the missing hash.
	resp, data := alice.do("POST", "/api/workspaces/saltglass/commit", alice.token,
		syncproto.CommitRequest{BaseRevision: -1, Name: "saltglass", Files: files})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 missing-blobs, got %d: %s", resp.StatusCode, data)
	}
	var res syncproto.CommitResult
	json.Unmarshal(data, &res)
	if len(res.Missing) != 1 || res.Missing[0] != hash {
		t.Fatalf("expected missing hash %s, got %+v", hash, res.Missing)
	}

	// Upload the blob, then commit succeeds at revision 1.
	up, _ := http.NewRequest("PUT", ts.URL+"/api/workspaces/saltglass/blobs/"+hash, bytes.NewReader(content))
	up.Header.Set("Authorization", "Bearer "+alice.token)
	if r, err := http.DefaultClient.Do(up); err != nil || r.StatusCode != http.StatusCreated {
		t.Fatalf("put blob: err=%v status=%v", err, r.Status)
	}
	resp, data = alice.do("POST", "/api/workspaces/saltglass/commit", alice.token,
		syncproto.CommitRequest{BaseRevision: -1, Name: "saltglass", Files: files})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("commit: status %d: %s", resp.StatusCode, data)
	}
	json.Unmarshal(data, &res)
	if res.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", res.Revision)
	}

	// The workspace now appears in the list.
	resp, data = alice.do("GET", "/api/workspaces", alice.token, nil)
	var list syncproto.WorkspaceList
	json.Unmarshal(data, &list)
	if len(list.Workspaces) != 1 || list.Workspaces[0].ID != "saltglass" || list.Workspaces[0].Revision != 1 {
		t.Fatalf("workspace list wrong: %+v", list.Workspaces)
	}

	// Pull the manifest and the blob back (as a second device would).
	resp, data = alice.do("GET", "/api/workspaces/saltglass/manifest", alice.token, nil)
	var man syncproto.Manifest
	json.Unmarshal(data, &man)
	if man.Revision != 1 || len(man.Files) != 1 || man.Files[0].Hash != hash {
		t.Fatalf("manifest wrong: %+v", man)
	}
	resp, blob := alice.do("GET", "/api/workspaces/saltglass/blobs/"+hash, alice.token, nil)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(blob, content) {
		t.Fatalf("blob pull mismatch: status %d", resp.StatusCode)
	}
}

func TestSyncConflict(t *testing.T) {
	ts := newTestServer(t, true)
	alice := register(t, ts.URL, "alice", "hunter2pw")

	commit := func(base int, files []syncproto.FileEntry) (*http.Response, syncproto.CommitResult) {
		resp, data := alice.do("POST", "/api/workspaces/w/commit", alice.token,
			syncproto.CommitRequest{BaseRevision: base, Name: "w", Files: files})
		var r syncproto.CommitResult
		json.Unmarshal(data, &r)
		return resp, r
	}
	put := func(b []byte) {
		h := sha256Hex(b)
		req, _ := http.NewRequest("PUT", ts.URL+"/api/workspaces/w/blobs/"+h, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+alice.token)
		http.DefaultClient.Do(req)
	}

	a := []byte("v1")
	put(a)
	if resp, _ := commit(-1, []syncproto.FileEntry{{Path: "a.md", Hash: sha256Hex(a), Size: 2}}); resp.StatusCode != 200 {
		t.Fatalf("first commit failed: %d", resp.StatusCode)
	}

	// Committing again against the now-stale base revision 0 conflicts.
	b := []byte("v2")
	put(b)
	resp, res := commit(0, []syncproto.FileEntry{{Path: "a.md", Hash: sha256Hex(b), Size: 2}})
	if resp.StatusCode != http.StatusConflict || !res.Conflict {
		t.Fatalf("expected 409 conflict, got %d %+v", resp.StatusCode, res)
	}
	if res.Current == nil || res.Current.Revision != 1 {
		t.Fatalf("conflict should carry current manifest at rev 1: %+v", res.Current)
	}

	// Committing against the correct base revision succeeds.
	resp, res = commit(1, []syncproto.FileEntry{{Path: "a.md", Hash: sha256Hex(b), Size: 2}})
	if resp.StatusCode != 200 || res.Revision != 2 {
		t.Fatalf("expected rev 2, got %d %+v", resp.StatusCode, res)
	}
}

func TestAccountIsolation(t *testing.T) {
	ts := newTestServer(t, true)
	alice := register(t, ts.URL, "alice", "hunter2pw")
	bob := register(t, ts.URL, "bob", "correcthorse")

	content := []byte("secret prose")
	hash := sha256Hex(content)
	req, _ := http.NewRequest("PUT", ts.URL+"/api/workspaces/shared/blobs/"+hash, bytes.NewReader(content))
	req.Header.Set("Authorization", "Bearer "+alice.token)
	http.DefaultClient.Do(req)
	alice.do("POST", "/api/workspaces/shared/commit", alice.token,
		syncproto.CommitRequest{BaseRevision: -1, Name: "shared",
			Files: []syncproto.FileEntry{{Path: "a.md", Hash: hash, Size: int64(len(content))}}})

	// Bob shares the workspace *id* string but sees his own empty namespace.
	resp, data := bob.do("GET", "/api/workspaces/shared/manifest", bob.token, nil)
	var man syncproto.Manifest
	json.Unmarshal(data, &man)
	if resp.StatusCode != 200 || man.Revision != 0 || len(man.Files) != 0 {
		t.Fatalf("bob should see an empty manifest, got %d %+v", resp.StatusCode, man)
	}
	// Bob cannot read Alice's blob (it isn't in his namespace).
	resp, _ = bob.do("GET", "/api/workspaces/shared/blobs/"+hash, bob.token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bob should not see alice's blob, got %d", resp.StatusCode)
	}
	// Bob's own list is empty.
	_, data = bob.do("GET", "/api/workspaces", bob.token, nil)
	var list syncproto.WorkspaceList
	json.Unmarshal(data, &list)
	if len(list.Workspaces) != 0 {
		t.Fatalf("bob's workspace list should be empty, got %+v", list.Workspaces)
	}
}

func TestAuthAndRegistrationGates(t *testing.T) {
	ts := newTestServer(t, true)
	register(t, ts.URL, "alice", "hunter2pw")

	c := &client{t: t, base: ts.URL}
	// Duplicate username.
	if resp, _ := c.do("POST", "/api/register", "", syncproto.RegisterRequest{Username: "alice", Password: "another8x"}); resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate register should be 409, got %d", resp.StatusCode)
	}
	// Short password rejected.
	if resp, _ := c.do("POST", "/api/register", "", syncproto.RegisterRequest{Username: "shorty", Password: "x"}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("short password should be 400, got %d", resp.StatusCode)
	}
	// Wrong password.
	if resp, _ := c.do("POST", "/api/login", "", syncproto.LoginRequest{Username: "alice", Password: "nope"}); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad login should be 401, got %d", resp.StatusCode)
	}
	// No token.
	if resp, _ := c.do("GET", "/api/workspaces", "", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("missing token should be 401, got %d", resp.StatusCode)
	}
	// Tampered token.
	if resp, _ := c.do("GET", "/api/workspaces", "garbage.token", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad token should be 401, got %d", resp.StatusCode)
	}

	// Registration disabled on a fresh server.
	closed := newTestServer(t, false)
	cc := &client{t: t, base: closed.URL}
	if resp, _ := cc.do("POST", "/api/register", "", syncproto.RegisterRequest{Username: "eve", Password: "password1"}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("registration should be disabled (403), got %d", resp.StatusCode)
	}
}

func TestBlobHashVerification(t *testing.T) {
	ts := newTestServer(t, true)
	alice := register(t, ts.URL, "alice", "hunter2pw")
	// PUT a blob under a hash that doesn't match its contents → rejected.
	wrong := sha256Hex([]byte("something else"))
	req, _ := http.NewRequest("PUT", ts.URL+"/api/workspaces/w/blobs/"+wrong, bytes.NewReader([]byte("actual")))
	req.Header.Set("Authorization", "Bearer "+alice.token)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("mismatched blob hash should be 400, got %d", resp.StatusCode)
	}
}
