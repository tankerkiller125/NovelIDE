package syncserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"novelide/internal/syncproto"
)

// Store is the sync server's persistence: accounts, per-account workspace
// metadata, and content-addressed blobs, all on the filesystem with JSON
// metadata. A single RWMutex serialises metadata access, which is ample for a
// self-hosted server; blobs are content-addressed and write-once, so they need
// no locking.
//
// Layout under the data directory:
//
//	accounts.json                         all accounts
//	accounts/<accountID>/workspaces.json  that account's workspace list
//	accounts/<accountID>/ws/<wsID>/manifest.json
//	accounts/<accountID>/ws/<wsID>/blobs/<sha256>
type Store struct {
	dir string
	mu  sync.RWMutex
}

// Account is a registered user. PasswordHash is never serialised to clients.
// Password accounts have a PasswordHash and an empty Subject; SSO accounts have
// a Subject (issuer|sub) and no password.
type Account struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash,omitempty"`
	Subject      string `json:"subject,omitempty"` // OIDC "issuer|sub" for SSO accounts
	CreatedAt    string `json:"createdAt"`
}

var (
	// ErrExists is returned when registering a taken username.
	ErrExists = errors.New("username already taken")
	// ErrNotFound is returned for a missing account or workspace.
	ErrNotFound = errors.New("not found")
	// ErrInvalid marks malformed identifiers or paths.
	ErrInvalid = errors.New("invalid request")

	wsIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	hashRe = regexp.MustCompile(`^[a-f0-9]{64}$`)
	nameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

// NewStore opens (creating if needed) a store rooted at dir.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "accounts"), 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) accountsPath() string { return filepath.Join(s.dir, "accounts.json") }
func (s *Store) accountDir(id string) string {
	return filepath.Join(s.dir, "accounts", id)
}
func (s *Store) wsDir(accID, wsID string) string {
	return filepath.Join(s.accountDir(accID), "ws", wsID)
}

// --- account metadata ---

func (s *Store) loadAccounts() ([]Account, error) {
	data, err := os.ReadFile(s.accountsPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Account
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) saveAccounts(accts []Account) error {
	return writeJSONAtomic(s.accountsPath(), accts)
}

// CreateAccount registers a new account with a bcrypt-hashed password.
func (s *Store) CreateAccount(username, password string, now time.Time) (Account, error) {
	username = strings.TrimSpace(username)
	if !nameRe.MatchString(username) {
		return Account{}, fmt.Errorf("%w: username must be 1-64 chars of [A-Za-z0-9._-]", ErrInvalid)
	}
	if len(password) < 8 {
		return Account{}, fmt.Errorf("%w: password must be at least 8 characters", ErrInvalid)
	}
	hash, err := hashPassword(password)
	if err != nil {
		return Account{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	accts, err := s.loadAccounts()
	if err != nil {
		return Account{}, err
	}
	for _, a := range accts {
		if strings.EqualFold(a.Username, username) {
			return Account{}, ErrExists
		}
	}
	acc := Account{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: hash,
		CreatedAt:    now.UTC().Format(time.RFC3339),
	}
	if err := os.MkdirAll(s.accountDir(acc.ID), 0o755); err != nil {
		return Account{}, err
	}
	if err := s.saveAccounts(append(accts, acc)); err != nil {
		return Account{}, err
	}
	return acc, nil
}

// UpsertOIDCAccount finds the account for an OIDC subject, creating it on first
// login. username is a display label from the IdP (not used for lookup).
func (s *Store) UpsertOIDCAccount(subject, username string, now time.Time) (Account, error) {
	if subject == "" {
		return Account{}, fmt.Errorf("%w: empty subject", ErrInvalid)
	}
	if username == "" {
		username = "user"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	accts, err := s.loadAccounts()
	if err != nil {
		return Account{}, err
	}
	for i := range accts {
		if accts[i].Subject == subject {
			// Refresh the display name if the IdP's changed.
			if accts[i].Username != username {
				accts[i].Username = username
				_ = s.saveAccounts(accts)
			}
			return accts[i], nil
		}
	}
	acc := Account{
		ID:        uuid.NewString(),
		Username:  username,
		Subject:   subject,
		CreatedAt: now.UTC().Format(time.RFC3339),
	}
	if err := os.MkdirAll(s.accountDir(acc.ID), 0o755); err != nil {
		return Account{}, err
	}
	if err := s.saveAccounts(append(accts, acc)); err != nil {
		return Account{}, err
	}
	return acc, nil
}

// Authenticate returns the account for username if the password matches.
func (s *Store) Authenticate(username, password string) (Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accts, err := s.loadAccounts()
	if err != nil {
		return Account{}, err
	}
	for _, a := range accts {
		if strings.EqualFold(a.Username, username) {
			if checkPassword(a.PasswordHash, password) {
				return a, nil
			}
			return Account{}, ErrNotFound
		}
	}
	return Account{}, ErrNotFound
}

// AccountByID looks up an account (used to validate a token's subject).
func (s *Store) AccountByID(id string) (Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accts, err := s.loadAccounts()
	if err != nil {
		return Account{}, err
	}
	for _, a := range accts {
		if a.ID == id {
			return a, nil
		}
	}
	return Account{}, ErrNotFound
}

// --- workspace metadata ---

func (s *Store) workspacesPath(accID string) string {
	return filepath.Join(s.accountDir(accID), "workspaces.json")
}

func (s *Store) loadWorkspaces(accID string) ([]syncproto.WorkspaceMeta, error) {
	data, err := os.ReadFile(s.workspacesPath(accID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []syncproto.WorkspaceMeta
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListWorkspaces returns an account's workspaces (never nil).
func (s *Store) ListWorkspaces(accID string) ([]syncproto.WorkspaceMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ws, err := s.loadWorkspaces(accID)
	if err != nil {
		return nil, err
	}
	if ws == nil {
		ws = []syncproto.WorkspaceMeta{}
	}
	return ws, nil
}

// Manifest returns a workspace's current file set. A workspace that has never
// been committed reports revision 0 with no files.
func (s *Store) Manifest(accID, wsID string) (syncproto.Manifest, error) {
	if !wsIDRe.MatchString(wsID) {
		return syncproto.Manifest{}, ErrInvalid
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadManifest(accID, wsID)
}

func (s *Store) manifestPath(accID, wsID string) string {
	return filepath.Join(s.wsDir(accID, wsID), "manifest.json")
}

func (s *Store) loadManifest(accID, wsID string) (syncproto.Manifest, error) {
	data, err := os.ReadFile(s.manifestPath(accID, wsID))
	if os.IsNotExist(err) {
		return syncproto.Manifest{Revision: 0, Files: []syncproto.FileEntry{}}, nil
	}
	if err != nil {
		return syncproto.Manifest{}, err
	}
	var m syncproto.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return syncproto.Manifest{}, err
	}
	if m.Files == nil {
		m.Files = []syncproto.FileEntry{}
	}
	return m, nil
}

// --- blobs ---

func (s *Store) blobPath(accID, wsID, hash string) string {
	return filepath.Join(s.wsDir(accID, wsID), "blobs", hash)
}

// HasBlob reports whether a blob is already stored.
func (s *Store) HasBlob(accID, wsID, hash string) bool {
	if !wsIDRe.MatchString(wsID) || !hashRe.MatchString(hash) {
		return false
	}
	_, err := os.Stat(s.blobPath(accID, wsID, hash))
	return err == nil
}

// PutBlob stores blob bytes, verifying the content hash so a client can't
// mislabel data. Storing an already-present blob is a no-op.
func (s *Store) PutBlob(accID, wsID, hash string, data []byte) error {
	if !wsIDRe.MatchString(wsID) || !hashRe.MatchString(hash) {
		return ErrInvalid
	}
	if sum := sha256Hex(data); sum != hash {
		return fmt.Errorf("%w: content hash %s does not match %s", ErrInvalid, sum, hash)
	}
	dir := filepath.Join(s.wsDir(accID, wsID), "blobs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := s.blobPath(accID, wsID, hash)
	if _, err := os.Stat(p); err == nil {
		return nil // already stored
	}
	return writeFileAtomic(p, data)
}

// GetBlob returns a stored blob, or ErrNotFound.
func (s *Store) GetBlob(accID, wsID, hash string) ([]byte, error) {
	if !wsIDRe.MatchString(wsID) || !hashRe.MatchString(hash) {
		return nil, ErrInvalid
	}
	data, err := os.ReadFile(s.blobPath(accID, wsID, hash))
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	return data, err
}

// --- commit ---

// Commit atomically replaces a workspace's manifest, enforcing optimistic
// concurrency and blob presence. See syncproto.CommitResult for the outcome
// shapes (success, conflict, or missing-blobs).
func (s *Store) Commit(accID string, wsID string, req syncproto.CommitRequest, now time.Time) (syncproto.CommitResult, error) {
	if !wsIDRe.MatchString(wsID) {
		return syncproto.CommitResult{}, ErrInvalid
	}
	if req.Name != "" && !nameRe.MatchString(req.Name) {
		return syncproto.CommitResult{}, fmt.Errorf("%w: bad workspace name", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, f := range req.Files {
		if !hashRe.MatchString(f.Hash) {
			return syncproto.CommitResult{}, fmt.Errorf("%w: bad hash %q", ErrInvalid, f.Hash)
		}
		if !validPath(f.Path) {
			return syncproto.CommitResult{}, fmt.Errorf("%w: bad path %q", ErrInvalid, f.Path)
		}
		if seen[f.Path] {
			return syncproto.CommitResult{}, fmt.Errorf("%w: duplicate path %q", ErrInvalid, f.Path)
		}
		seen[f.Path] = true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cur, err := s.loadManifest(accID, wsID)
	if err != nil {
		return syncproto.CommitResult{}, err
	}
	if req.BaseRevision != -1 && req.BaseRevision != cur.Revision {
		m := cur
		return syncproto.CommitResult{Conflict: true, Current: &m}, nil
	}

	// All referenced blobs must already be uploaded.
	var missing []string
	miss := map[string]bool{}
	for _, f := range req.Files {
		if !miss[f.Hash] && !s.HasBlob(accID, wsID, f.Hash) {
			miss[f.Hash] = true
			missing = append(missing, f.Hash)
		}
	}
	if len(missing) > 0 {
		return syncproto.CommitResult{Missing: missing}, nil
	}

	files := req.Files
	if files == nil {
		files = []syncproto.FileEntry{}
	}
	next := cur.Revision + 1
	if err := os.MkdirAll(s.wsDir(accID, wsID), 0o755); err != nil {
		return syncproto.CommitResult{}, err
	}
	if err := writeJSONAtomic(s.manifestPath(accID, wsID), syncproto.Manifest{Revision: next, Files: files}); err != nil {
		return syncproto.CommitResult{}, err
	}
	if err := s.updateWorkspaceMeta(accID, wsID, req.Name, next, now); err != nil {
		return syncproto.CommitResult{}, err
	}
	return syncproto.CommitResult{Revision: next}, nil
}

// updateWorkspaceMeta upserts the workspace entry in workspaces.json. Caller
// holds the write lock.
func (s *Store) updateWorkspaceMeta(accID, wsID, name string, rev int, now time.Time) error {
	list, err := s.loadWorkspaces(accID)
	if err != nil {
		return err
	}
	ts := now.UTC().Format(time.RFC3339)
	for i := range list {
		if list[i].ID == wsID {
			list[i].Revision = rev
			list[i].UpdatedAt = ts
			if name != "" {
				list[i].Name = name
			}
			return writeJSONAtomic(s.workspacesPath(accID), list)
		}
	}
	if name == "" {
		name = wsID
	}
	list = append(list, syncproto.WorkspaceMeta{ID: wsID, Name: name, Revision: rev, UpdatedAt: ts})
	return writeJSONAtomic(s.workspacesPath(accID), list)
}

// validPath accepts a workspace-relative forward-slash path with no traversal.
func validPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, `\`) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean != p || clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	return true
}
