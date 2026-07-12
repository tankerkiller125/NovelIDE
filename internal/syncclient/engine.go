package syncclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"novelide/internal/syncproto"
)

// Result summarises what a sync did.
type Result struct {
	Revision  int      `json:"revision"`
	Pushed    int      `json:"pushed"`  // files sent to the server
	Pulled    int      `json:"pulled"`  // files written from the server
	Deleted   int      `json:"deleted"` // local files removed
	Conflicts []string `json:"conflicts"`
	RemoteID  string   `json:"remoteId"`
}

type fileInfo struct {
	hash string
	size int64
}

// syncState is persisted per workspace so subsequent syncs can do a proper
// three-way merge. Base is the file set as of the last successful sync.
//
// Server and Account scope that base to the identity it was captured under.
// The base is only meaningful for the same server+account: reusing it after
// switching accounts would make the merge treat local files as "deleted on the
// remote" and delete them. When the identity differs, the base is discarded.
type syncState struct {
	RemoteID     string            `json:"remoteId"`
	Server       string            `json:"server"`
	Account      string            `json:"account"`
	BaseRevision int               `json:"baseRevision"`
	Base         map[string]string `json:"base"` // path -> hash
}

const stateRel = ".novelide/sync.json"

func statePath(ws string) string { return filepath.Join(ws, filepath.FromSlash(stateRel)) }

func loadState(ws string) syncState {
	var st syncState
	data, err := os.ReadFile(statePath(ws))
	if err == nil {
		_ = json.Unmarshal(data, &st)
	}
	if st.Base == nil {
		st.Base = map[string]string{}
	}
	return st
}

func saveState(ws string, st syncState) error {
	p := statePath(ws)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// scan walks a workspace and returns every file (path -> hash/size), skipping
// the local metadata and VCS directories. Paths use forward slashes.
func scan(ws string) (map[string]fileInfo, error) {
	out := map[string]fileInfo{}
	err := filepath.WalkDir(ws, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".novelide" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(ws, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel)] = fileInfo{hash: hex.EncodeToString(sum[:]), size: int64(len(data))}
		return nil
	})
	return out, err
}

// validRel reports whether a path (typically supplied by the remote server) is
// a safe workspace-relative path: not absolute, no traversal, no backslash or
// Windows drive/volume tricks. The client must never trust remote paths — a
// hostile or compromised server could otherwise make it write or delete files
// anywhere on disk.
func validRel(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, `\`) {
		return false
	}
	if filepath.VolumeName(rel) != "" { // e.g. "C:" on Windows
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	return clean == rel && clean != ".." && !strings.HasPrefix(clean, "../")
}

// safeJoin resolves rel under ws, rejecting anything that would escape the
// workspace directory.
func safeJoin(ws, rel string) (string, error) {
	if !validRel(rel) {
		return "", fmt.Errorf("unsafe path %q", rel)
	}
	p := filepath.Join(ws, filepath.FromSlash(rel))
	wsAbs, err := filepath.Abs(ws)
	if err != nil {
		return "", err
	}
	pAbs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if pAbs != wsAbs && !strings.HasPrefix(pAbs, wsAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %q", rel)
	}
	return p, nil
}

// checkRemoteManifest rejects a whole remote file set up front if any path is
// unsafe, so a hostile manifest never causes a partial, dangerous write.
func checkRemoteManifest(files []syncproto.FileEntry) error {
	for _, f := range files {
		if !validRel(f.Path) {
			return fmt.Errorf("remote manifest contains an unsafe path %q; refusing to sync", f.Path)
		}
	}
	return nil
}

func writeFile(ws, rel string, data []byte) error {
	p, err := safeJoin(ws, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func removeFile(ws, rel string) error {
	p, err := safeJoin(ws, rel)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

var (
	idClean   = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	nameClean = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

// DeriveRemoteID turns a workspace folder name into a valid remote id, falling
// back to a random id when nothing usable remains.
func DeriveRemoteID(ws string) string {
	id := idClean.ReplaceAllString(filepath.Base(ws), "-")
	id = strings.Trim(id, "-.")
	if len(id) > 128 {
		id = id[:128]
	}
	if id == "" {
		return uuid.NewString()
	}
	return id
}

func deriveName(ws string) string {
	n := nameClean.ReplaceAllString(filepath.Base(ws), "-")
	n = strings.Trim(n, "-.")
	if len(n) > 64 {
		n = n[:64]
	}
	return n
}

func hashesOf(m map[string]fileInfo) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v.hash
	}
	return out
}

func manifestFiles(m map[string]fileInfo) []syncproto.FileEntry {
	files := make([]syncproto.FileEntry, 0, len(m))
	for p, fi := range m {
		files = append(files, syncproto.FileEntry{Path: p, Hash: fi.hash, Size: fi.size})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

// commitWithBlobs commits a manifest, uploading any blobs the server reports it
// is missing (their bytes are read from the workspace, where every referenced
// path exists) and retrying. It does not resolve revision conflicts — the
// caller handles those.
func commitWithBlobs(ws string, c *Client, remoteID, name string, files []syncproto.FileEntry, baseRev int) (syncproto.CommitResult, error) {
	// path lookup by hash, to fetch bytes for a missing blob.
	byHash := map[string]string{}
	for _, f := range files {
		byHash[f.Hash] = f.Path
	}
	for attempt := 0; attempt < 6; attempt++ {
		res, err := c.Commit(remoteID, syncproto.CommitRequest{BaseRevision: baseRev, Name: name, Files: files})
		if err != nil {
			return res, err
		}
		if len(res.Missing) == 0 {
			return res, nil // success or conflict — caller decides
		}
		for _, h := range res.Missing {
			rel, ok := byHash[h]
			if !ok {
				return res, fmt.Errorf("server wants blob %s but no file references it", h)
			}
			data, err := os.ReadFile(filepath.Join(ws, filepath.FromSlash(rel)))
			if err != nil {
				return res, err
			}
			if err := c.PutBlob(remoteID, h, data); err != nil {
				return res, err
			}
		}
	}
	return syncproto.CommitResult{}, fmt.Errorf("could not upload all blobs")
}

// mergePlan is the outcome of a three-way merge before any disk changes.
type mergePlan struct {
	result    map[string]fileInfo // final file set to commit
	downloads map[string]string   // path -> hash to fetch and write
	deletes   []string            // local paths to remove
	conflicts []string            // paths that diverged on both sides
}

// merge computes a file-level three-way merge of base/local/remote. On a true
// conflict the local file is kept at its path and the remote version is
// brought in as a "(conflict …)" copy, so nothing is ever lost.
func merge(base map[string]string, local, remote map[string]fileInfo, now time.Time) mergePlan {
	plan := mergePlan{
		result:    map[string]fileInfo{},
		downloads: map[string]string{},
		conflicts: []string{}, // never nil, so it marshals as [] not null
	}
	for p, fi := range local { // start from local
		plan.result[p] = fi
	}
	paths := map[string]bool{}
	for p := range base {
		paths[p] = true
	}
	for p := range local {
		paths[p] = true
	}
	for p := range remote {
		paths[p] = true
	}
	for p := range paths {
		b := base[p]
		l, hasL := local[p]
		r, hasR := remote[p]
		lh, rh := l.hash, r.hash
		if !hasL {
			lh = ""
		}
		if !hasR {
			rh = ""
		}
		localChanged := lh != b
		remoteChanged := rh != b
		switch {
		case !remoteChanged:
			// keep local (already in result)
		case !localChanged:
			// take remote
			if rh == "" {
				delete(plan.result, p)
				plan.deletes = append(plan.deletes, p)
			} else {
				plan.result[p] = fileInfo{hash: rh, size: r.size}
				plan.downloads[p] = rh
			}
		case lh == rh:
			// both moved to the same content — nothing to do
		default:
			// conflict: keep local, bring remote in as a copy
			plan.conflicts = append(plan.conflicts, p)
			if rh != "" {
				cp := conflictName(p, now)
				plan.result[cp] = fileInfo{hash: rh, size: r.size}
				plan.downloads[cp] = rh
			}
		}
	}
	sort.Strings(plan.deletes)
	sort.Strings(plan.conflicts)
	return plan
}

func conflictName(path string, now time.Time) string {
	ext := filepath.Ext(path)
	stamp := now.Format("20060102-150405")
	return strings.TrimSuffix(path, ext) + " (conflict " + stamp + ")" + ext
}

// Sync performs a two-way sync of a linked workspace. Disk is only modified
// after the server accepts the merged manifest, so a mid-sync failure leaves
// the local workspace untouched. account is the id of the signed-in account, so
// a base captured under a different identity is never used to delete files.
func Sync(ws string, c *Client, account string) (Result, error) {
	st := loadState(ws)
	if st.RemoteID == "" {
		return Result{}, fmt.Errorf("this workspace isn't linked to a remote yet")
	}
	// If the base was captured under a different server/account, it describes a
	// remote this identity can't see — using it would delete every local file.
	// Discard it and treat this as a fresh link for the current identity.
	if st.Server != c.BaseURL || st.Account != account {
		st.Base = map[string]string{}
		st.BaseRevision = 0
	}
	st.Server, st.Account = c.BaseURL, account
	name := deriveName(ws)
	for attempt := 0; attempt < 4; attempt++ {
		remote, err := c.Manifest(st.RemoteID)
		if err != nil {
			return Result{}, err
		}
		if err := checkRemoteManifest(remote.Files); err != nil {
			return Result{}, err
		}
		local, err := scan(ws)
		if err != nil {
			return Result{}, err
		}
		remoteMap := map[string]fileInfo{}
		for _, f := range remote.Files {
			remoteMap[f.Path] = fileInfo{hash: f.Hash, size: f.Size}
		}
		// A never-committed (revision 0) remote can't have "deleted" anything —
		// if the base disagrees it's stale, so merge from an empty base to push
		// rather than delete.
		base := st.Base
		if remote.Revision == 0 {
			base = map[string]string{}
		}
		plan := merge(base, local, remoteMap, time.Now())

		res, err := commitWithBlobs(ws, c, st.RemoteID, name, manifestFiles(plan.result), remote.Revision)
		if err != nil {
			return Result{}, err
		}
		if res.Conflict {
			continue // remote advanced while we worked; re-merge
		}

		// Server accepted the merge — now make it true on disk.
		pulled := 0
		for p, h := range plan.downloads {
			data, err := c.GetBlob(st.RemoteID, h)
			if err != nil {
				return Result{}, err
			}
			if err := writeFile(ws, p, data); err != nil {
				return Result{}, err
			}
			pulled++
		}
		for _, p := range plan.deletes {
			if err := removeFile(ws, p); err != nil {
				return Result{}, err
			}
		}
		st.Base = hashesOf(plan.result)
		st.BaseRevision = res.Revision
		if err := saveState(ws, st); err != nil {
			return Result{}, err
		}
		pushed := 0
		for p, fi := range plan.result {
			if remoteMap[p].hash != fi.hash {
				pushed++
			}
		}
		return Result{
			Revision:  res.Revision,
			Pushed:    pushed,
			Pulled:    pulled,
			Deleted:   len(plan.deletes),
			Conflicts: plan.conflicts,
			RemoteID:  st.RemoteID,
		}, nil
	}
	return Result{}, fmt.Errorf("sync kept racing another device; please try again")
}

// LinkPush links the workspace to remoteID and force-pushes the local files as
// the authoritative copy (use when this device holds the canonical workspace).
func LinkPush(ws, remoteID string, c *Client, account string) (Result, error) {
	local, err := scan(ws)
	if err != nil {
		return Result{}, err
	}
	res, err := commitWithBlobs(ws, c, remoteID, deriveName(ws), manifestFiles(local), -1)
	if err != nil {
		return Result{}, err
	}
	if res.Conflict {
		return Result{}, fmt.Errorf("unexpected conflict on force-push")
	}
	st := syncState{RemoteID: remoteID, Server: c.BaseURL, Account: account, BaseRevision: res.Revision, Base: hashesOf(local)}
	if err := saveState(ws, st); err != nil {
		return Result{}, err
	}
	return Result{Revision: res.Revision, Pushed: len(local), Conflicts: []string{}, RemoteID: remoteID}, nil
}

// LinkPull links the workspace to remoteID and pulls the remote files down,
// overwriting local files that differ. Local-only files are kept (a later Sync
// will push them). Use when joining an existing remote workspace.
func LinkPull(ws, remoteID string, c *Client, account string) (Result, error) {
	remote, err := c.Manifest(remoteID)
	if err != nil {
		return Result{}, err
	}
	if err := checkRemoteManifest(remote.Files); err != nil {
		return Result{}, err
	}
	local, err := scan(ws)
	if err != nil {
		return Result{}, err
	}
	pulled := 0
	for _, f := range remote.Files {
		if cur, ok := local[f.Path]; ok && cur.hash == f.Hash {
			continue // already identical
		}
		data, err := c.GetBlob(remoteID, f.Hash)
		if err != nil {
			return Result{}, err
		}
		if err := writeFile(ws, f.Path, data); err != nil {
			return Result{}, err
		}
		pulled++
	}
	// Base reflects the remote we just pulled; local-only extras aren't in it,
	// so the next Sync sees them as local additions and pushes them.
	base := map[string]string{}
	for _, f := range remote.Files {
		base[f.Path] = f.Hash
	}
	st := syncState{RemoteID: remoteID, Server: c.BaseURL, Account: account, BaseRevision: remote.Revision, Base: base}
	if err := saveState(ws, st); err != nil {
		return Result{}, err
	}
	return Result{Revision: remote.Revision, Pulled: pulled, Conflicts: []string{}, RemoteID: remoteID}, nil
}

// LinkedRemoteID returns the remote id a workspace is linked to ("" if none).
func LinkedRemoteID(ws string) string { return loadState(ws).RemoteID }

// Link associates a workspace with a remote id for the given identity without
// transferring anything. A subsequent Sync then reconciles the two sides from
// an empty base — pushing cleanly to a fresh remote, or producing conflict
// copies if both sides already hold different content. Use it for the safe
// auto-link on first sync.
func Link(ws, remoteID, server, account string) error {
	st := loadState(ws)
	if st.RemoteID == remoteID && st.Server == server && st.Account == account {
		return nil
	}
	return saveState(ws, syncState{RemoteID: remoteID, Server: server, Account: account, BaseRevision: 0, Base: map[string]string{}})
}
