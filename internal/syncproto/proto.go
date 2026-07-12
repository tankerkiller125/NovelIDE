// Package syncproto defines the wire types shared by the optional NovelIDE
// sync server and its clients. Keeping them in their own package lets a client
// import the protocol without pulling in any server code.
//
// Sync is deliberately simple and file-oriented, matching NovelIDE's
// plain-file storage: a workspace is a set of files, each addressed by the
// SHA-256 of its contents. Clients push the files they have and pull the ones
// they lack; a per-workspace revision counter provides optimistic concurrency
// so two devices can't silently clobber each other.
package syncproto

// RegisterRequest creates a new account.
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest authenticates an existing account.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse is returned by register and login.
type AuthResponse struct {
	Token     string `json:"token"`
	AccountID string `json:"accountId"`
	Username  string `json:"username"`
}

// AuthConfig tells a client which authentication methods a server offers, so
// the UI can show a password form, an SSO button, or both.
type AuthConfig struct {
	PasswordEnabled bool   `json:"passwordEnabled"`
	SSOEnabled      bool   `json:"ssoEnabled"`
	SSOName         string `json:"ssoName,omitempty"` // display label, e.g. "Zitadel"
}

// MeResponse identifies the authenticated account (used after SSO to learn the
// username, since the token itself doesn't carry it).
type MeResponse struct {
	AccountID string `json:"accountId"`
	Username  string `json:"username"`
}

// WorkspaceMeta is the summary of one synced workspace.
type WorkspaceMeta struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Revision  int    `json:"revision"`
	UpdatedAt string `json:"updatedAt"`
}

// WorkspaceList is the response to GET /api/workspaces.
type WorkspaceList struct {
	Workspaces []WorkspaceMeta `json:"workspaces"`
}

// FileEntry is one file in a workspace manifest: its path and the SHA-256 of
// its contents (lowercase hex).
type FileEntry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// Manifest is the full file set of a workspace at a given revision.
type Manifest struct {
	Revision int         `json:"revision"`
	Files    []FileEntry `json:"files"`
}

// CommitRequest replaces a workspace's file set. BaseRevision is the revision
// the client last synced from; the server rejects the commit if it no longer
// matches (another device pushed in the meantime). Use -1 to create a new
// workspace or to force-overwrite regardless of revision.
type CommitRequest struct {
	BaseRevision int         `json:"baseRevision"`
	Name         string      `json:"name"`
	Files        []FileEntry `json:"files"`
}

// CommitResult is the outcome of a commit. On success Revision is the new
// revision. On a stale BaseRevision, Conflict is true and Current holds the
// server's manifest to merge against. When referenced blobs aren't uploaded
// yet, Missing lists the hashes the client must PUT before retrying.
type CommitResult struct {
	Revision int       `json:"revision,omitempty"`
	Conflict bool      `json:"conflict,omitempty"`
	Missing  []string  `json:"missing,omitempty"`
	Current  *Manifest `json:"current,omitempty"`
}

// ErrorResponse is the body of a non-2xx response.
type ErrorResponse struct {
	Error string `json:"error"`
}
