package workspace

import (
	"path/filepath"
	"sort"
)

// DismissedFile stores the keys of codex-gap suggestions the author has
// dismissed. It lives at the workspace root (not under .novelide/) so it is
// carried along by sync — a dismissal on one device stays dismissed on all.
const DismissedFile = "codex-dismissed.yaml"

type dismissedDoc struct {
	Dismissed []string `yaml:"dismissed"`
}

// LoadDismissed returns the dismissed suggestion keys for a workspace (empty if
// none). A missing or unreadable file is treated as "none dismissed".
func LoadDismissed(wsPath string) []string {
	var doc dismissedDoc
	if err := readYAML(filepath.Join(wsPath, DismissedFile), &doc); err != nil {
		return []string{}
	}
	if doc.Dismissed == nil {
		return []string{}
	}
	return doc.Dismissed
}

// AddDismissed records a suggestion key as dismissed (idempotent) and returns
// the full, sorted set.
func AddDismissed(wsPath, key string) ([]string, error) {
	set := map[string]bool{}
	for _, k := range LoadDismissed(wsPath) {
		set[k] = true
	}
	set[key] = true
	return saveDismissed(wsPath, set)
}

// RemoveDismissed un-dismisses a key, so the suggestion can surface again.
func RemoveDismissed(wsPath, key string) ([]string, error) {
	set := map[string]bool{}
	for _, k := range LoadDismissed(wsPath) {
		set[k] = true
	}
	delete(set, key)
	return saveDismissed(wsPath, set)
}

func saveDismissed(wsPath string, set map[string]bool) ([]string, error) {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if err := writeYAML(filepath.Join(wsPath, DismissedFile), dismissedDoc{Dismissed: keys}); err != nil {
		return nil, err
	}
	return keys, nil
}
