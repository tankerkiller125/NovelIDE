package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"novelide/internal/model"
)

// AssetsDir holds images referenced by codex entries.
const AssetsDir = "assets"

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true,
}

// resolveInsideWorkspace joins a workspace-relative path to wsPath and
// returns the absolute path only if it stays inside the workspace. It is the
// single guard for every filesystem access built from an entry's stored
// (and therefore potentially attacker-controlled, via a shared workspace)
// image path.
func resolveInsideWorkspace(wsPath, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("empty path")
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute path not allowed: %q", rel)
	}
	full := filepath.Join(wsPath, clean)
	within, err := filepath.Rel(wsPath, full)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %q", rel)
	}
	return full, nil
}

// SetEntryImage copies an image file into the workspace's assets directory
// (named after the entry) and records the relative path on the entry, then
// saves it. Replaces any previous image for the entry.
func SetEntryImage(wsPath string, e *model.CodexEntry, srcPath string) error {
	ext := strings.ToLower(filepath.Ext(srcPath))
	if !imageExts[ext] {
		return fmt.Errorf("unsupported image type %q", ext)
	}
	if e.ID == "" {
		e.ID = Slugify(e.Name)
	}
	rel := AssetsDir + "/" + e.ID + ext
	dst := filepath.Join(wsPath, AssetsDir, e.ID+ext)
	if err := os.MkdirAll(filepath.Join(wsPath, AssetsDir), 0o755); err != nil {
		return err
	}
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// If the entry previously had an image with a different extension,
	// remove the stale file so it doesn't linger (only if it's safely
	// inside the workspace — never follow an escaping path).
	if e.Image != "" && e.Image != rel {
		if old, err := resolveInsideWorkspace(wsPath, e.Image); err == nil {
			_ = os.Remove(old)
		}
	}
	e.Image = rel
	return SaveEntry(wsPath, e)
}

// ClearEntryImage removes an entry's image (file and reference) and saves.
// A reference that points outside the workspace is dropped without touching
// the filesystem.
func ClearEntryImage(wsPath string, e *model.CodexEntry) error {
	if e.Image != "" {
		if p, err := resolveInsideWorkspace(wsPath, e.Image); err == nil {
			_ = os.Remove(p)
		}
	}
	e.Image = ""
	return SaveEntry(wsPath, e)
}

// ReadImage returns the bytes of a workspace-relative image path, refusing
// any path that escapes the workspace.
func ReadImage(wsPath, rel string) ([]byte, error) {
	full, err := resolveInsideWorkspace(wsPath, rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(full)
}
