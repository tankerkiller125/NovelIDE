// Package history is NovelIDE's built-in revision history: point-in-time
// snapshots of a workspace's text (manuscript, codex, plans, schema), stored
// locally so authors can diff against and roll back to earlier drafts without
// needing git.
//
// On-disk layout, inside the workspace:
//
//	.novelide/snapshots/
//	  index.json          ordered list of snapshots, newest first
//	  blobs/<sha256>       deduplicated file contents (many snapshots share one)
//	  snap-<id>.json       one snapshot's manifest: relPath -> blob hash
//
// Content addressing means an unchanged file costs nothing across snapshots —
// only its hash is recorded — so keeping a long history stays cheap.
package history

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Snapshot is one captured revision (metadata only; file contents live in the
// blob store, referenced by the per-snapshot manifest).
type Snapshot struct {
	ID    string `json:"id"`    // sortable, unique: "20260711-153004-000"
	Time  string `json:"time"`  // RFC3339
	Label string `json:"label"` // author-supplied or auto description
	Auto  bool   `json:"auto"`  // created automatically (daily safety net)
	Files int    `json:"files"` // number of files captured
	Size  int    `json:"size"`  // total bytes of captured content
}

// FileChange describes how one file differs between a snapshot and the current
// workspace, from the snapshot's point of view.
type FileChange struct {
	Rel    string `json:"rel"`
	Status string `json:"status"` // "modified" | "added" | "removed"
}

// DiffLine is one line of a unified diff between the snapshot ("old") and the
// current file ("new").
type DiffLine struct {
	Op   string `json:"op"` // "eq" | "add" | "del"
	Text string `json:"text"`
}

// DiffResult is the line diff for a single file.
type DiffResult struct {
	Rel   string     `json:"rel"`
	Lines []DiffLine `json:"lines"`
}

type manifest struct {
	Files map[string]string `json:"files"` // relPath -> blob hash
}

type indexFile struct {
	Snapshots []Snapshot `json:"snapshots"` // newest first
}

const dirName = ".novelide"

func snapDir(ws string) string   { return filepath.Join(ws, dirName, "snapshots") }
func blobDir(ws string) string   { return filepath.Join(snapDir(ws), "blobs") }
func indexPath(ws string) string { return filepath.Join(snapDir(ws), "index.json") }
func manifestPath(ws, id string) string {
	return filepath.Join(snapDir(ws), "snap-"+id+".json")
}

// captureExts are the text file types a snapshot records. Binary assets
// (images) are deliberately excluded — they're large and rarely edited.
var captureExts = map[string]bool{".md": true, ".yaml": true, ".yml": true, ".txt": true}

// safeRel rejects a manifest path that would escape the workspace.
func safeRel(ws, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute path not allowed: %q", rel)
	}
	full := filepath.Join(ws, clean)
	within, err := filepath.Rel(ws, full)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %q", rel)
	}
	return full, nil
}

// collect walks the workspace and returns the current text files as a map of
// forward-slash relative path -> contents.
func collect(ws string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.WalkDir(ws, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Never descend into our own store, VCS, or binary asset dirs.
			if d.Name() == dirName || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !captureExts[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		rel, err := filepath.Rel(ws, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func readIndex(ws string) (indexFile, error) {
	var idx indexFile
	data, err := os.ReadFile(indexPath(ws))
	if os.IsNotExist(err) {
		return idx, nil
	}
	if err != nil {
		return idx, err
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return idx, err
	}
	return idx, nil
}

func writeIndex(ws string, idx indexFile) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath(ws), data, 0o644)
}

func readManifest(ws, id string) (manifest, error) {
	var m manifest
	data, err := os.ReadFile(manifestPath(ws, id))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	return m, nil
}

// manifestsEqual reports whether two file->hash maps are identical.
func manifestsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// List returns all snapshots, newest first.
func List(ws string) ([]Snapshot, error) {
	idx, err := readIndex(ws)
	if err != nil {
		return nil, err
	}
	if idx.Snapshots == nil {
		return []Snapshot{}, nil
	}
	return idx.Snapshots, nil
}

// Create captures the workspace's current text into a new snapshot. If nothing
// has changed since the most recent snapshot, no snapshot is written and
// created is false — so auto-snapshots and repeat clicks don't pile up
// duplicates.
func Create(ws, label string, auto bool, now time.Time) (snap Snapshot, created bool, err error) {
	files, err := collect(ws)
	if err != nil {
		return snap, false, err
	}
	// Build the manifest and stage blobs.
	man := manifest{Files: make(map[string]string, len(files))}
	total := 0
	for rel, data := range files {
		man.Files[rel] = hashOf(data)
		total += len(data)
	}

	idx, err := readIndex(ws)
	if err != nil {
		return snap, false, err
	}
	// Skip if identical to the latest snapshot.
	if len(idx.Snapshots) > 0 {
		if prev, err := readManifest(ws, idx.Snapshots[0].ID); err == nil &&
			manifestsEqual(prev.Files, man.Files) {
			return idx.Snapshots[0], false, nil
		}
	}

	if err := os.MkdirAll(blobDir(ws), 0o755); err != nil {
		return snap, false, err
	}
	for rel, data := range files {
		bp := filepath.Join(blobDir(ws), man.Files[rel])
		if _, err := os.Stat(bp); err == nil {
			continue // blob already stored (shared with another snapshot)
		}
		if err := os.WriteFile(bp, data, 0o644); err != nil {
			return snap, false, err
		}
	}

	id := fmt.Sprintf("%s-%03d", now.Format("20060102-150405"), now.Nanosecond()/1e6)
	if label == "" {
		if auto {
			label = "Auto-saved " + now.Format("Jan 2, 2006")
		} else {
			label = "Snapshot " + now.Format("Jan 2, 2006 15:04")
		}
	}
	snap = Snapshot{
		ID:    id,
		Time:  now.Format(time.RFC3339),
		Label: label,
		Auto:  auto,
		Files: len(files),
		Size:  total,
	}
	manData, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return snap, false, err
	}
	if err := os.WriteFile(manifestPath(ws, id), manData, 0o644); err != nil {
		return snap, false, err
	}
	idx.Snapshots = append([]Snapshot{snap}, idx.Snapshots...)
	if err := writeIndex(ws, idx); err != nil {
		return snap, false, err
	}
	return snap, true, nil
}

// Changes compares a snapshot to the current workspace, returning per-file
// differences from the snapshot's point of view (modified/added/removed),
// sorted by path.
func Changes(ws, id string) ([]FileChange, error) {
	man, err := readManifest(ws, id)
	if err != nil {
		return nil, err
	}
	cur, err := collect(ws)
	if err != nil {
		return nil, err
	}
	curHash := make(map[string]string, len(cur))
	for rel, data := range cur {
		curHash[rel] = hashOf(data)
	}
	var out []FileChange
	for rel, h := range man.Files {
		if ch, ok := curHash[rel]; !ok {
			out = append(out, FileChange{Rel: rel, Status: "removed"})
		} else if ch != h {
			out = append(out, FileChange{Rel: rel, Status: "modified"})
		}
	}
	for rel := range curHash {
		if _, ok := man.Files[rel]; !ok {
			out = append(out, FileChange{Rel: rel, Status: "added"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

// blobContent returns the bytes stored for a snapshot's file (empty if the
// file wasn't in that snapshot).
func blobContent(ws, id, rel string) ([]byte, bool, error) {
	man, err := readManifest(ws, id)
	if err != nil {
		return nil, false, err
	}
	h, ok := man.Files[rel]
	if !ok {
		return nil, false, nil
	}
	data, err := os.ReadFile(filepath.Join(blobDir(ws), h))
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// FileDiff computes a line diff between the snapshot's version of rel ("old")
// and the current file ("new").
func FileDiff(ws, id, rel string) (DiffResult, error) {
	res := DiffResult{Rel: rel}
	oldData, _, err := blobContent(ws, id, rel)
	if err != nil {
		return res, err
	}
	full, err := safeRel(ws, rel)
	if err != nil {
		return res, err
	}
	newData, err := os.ReadFile(full)
	if err != nil && !os.IsNotExist(err) {
		return res, err
	}
	res.Lines = diffLines(splitLines(string(oldData)), splitLines(string(newData)))
	return res, nil
}

// RestoreFile overwrites the current file with the snapshot's version. A file
// that was absent in the snapshot is deleted (restoring that revision's state).
func RestoreFile(ws, id, rel string) error {
	full, err := safeRel(ws, rel)
	if err != nil {
		return err
	}
	data, ok, err := blobContent(ws, id, rel)
	if err != nil {
		return err
	}
	if !ok {
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

// Restore rolls the whole workspace back to a snapshot: every captured file is
// restored, and text files created since the snapshot are removed. Returns the
// number of files written. Binary assets are left untouched.
func Restore(ws, id string) (int, error) {
	changes, err := Changes(ws, id)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range changes {
		switch c.Status {
		case "modified", "removed":
			if err := RestoreFile(ws, id, c.Rel); err != nil {
				return n, err
			}
			n++
		case "added":
			// Created after the snapshot → delete to match that revision.
			full, err := safeRel(ws, c.Rel)
			if err != nil {
				return n, err
			}
			if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// Delete removes a snapshot and garbage-collects blobs no other snapshot
// references.
func Delete(ws, id string) error {
	idx, err := readIndex(ws)
	if err != nil {
		return err
	}
	kept := idx.Snapshots[:0]
	found := false
	for _, s := range idx.Snapshots {
		if s.ID == id {
			found = true
			continue
		}
		kept = append(kept, s)
	}
	if !found {
		return fmt.Errorf("snapshot %q not found", id)
	}
	idx.Snapshots = kept
	if err := writeIndex(ws, idx); err != nil {
		return err
	}
	// GC: collect hashes still referenced by remaining snapshots.
	live := map[string]bool{}
	for _, s := range idx.Snapshots {
		if m, err := readManifest(ws, s.ID); err == nil {
			for _, h := range m.Files {
				live[h] = true
			}
		}
	}
	if entries, err := os.ReadDir(blobDir(ws)); err == nil {
		for _, e := range entries {
			if !live[e.Name()] {
				_ = os.Remove(filepath.Join(blobDir(ws), e.Name()))
			}
		}
	}
	return os.Remove(manifestPath(ws, id))
}

// LatestIsFromDay reports whether the newest snapshot was taken on the given
// day — used to throttle automatic daily snapshots.
func LatestIsFromDay(ws string, day time.Time) bool {
	idx, err := readIndex(ws)
	if err != nil || len(idx.Snapshots) == 0 {
		return false
	}
	t, err := time.Parse(time.RFC3339, idx.Snapshots[0].Time)
	if err != nil {
		return false
	}
	ty, tm, td := t.Date()
	dy, dm, dd := day.Date()
	return ty == dy && tm == dm && td == dd
}
