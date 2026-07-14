// Package watch detects changes to a workspace's files made outside NovelIDE
// (another editor, a sync tool, an AI agent…) so the app can refresh instead of
// showing stale data. It polls rather than using OS notifications: no external
// dependency, uniform behaviour across filesystems (including network mounts),
// and a second or two of latency is plenty for this use.
package watch

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Change reports files that differ since the previous poll. Paths are
// workspace-relative with forward slashes.
type Change struct {
	Modified   []string `json:"modified"`   // existing files whose contents changed
	Structural []string `json:"structural"` // files added or removed
}

type meta struct {
	modNano int64
	size    int64
}

// Watcher polls a directory tree on an interval, invoking onChange whenever the
// set of files or their contents change.
type Watcher struct {
	root     string
	interval time.Duration
	onChange func(Change)
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// Start begins watching root, calling onChange (on its own goroutine) for each
// batch of changes. The initial state is taken as the baseline and does not
// fire onChange.
func Start(root string, interval time.Duration, onChange func(Change)) *Watcher {
	w := &Watcher{
		root:     root,
		interval: interval,
		onChange: onChange,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go w.run()
	return w
}

// Stop ends watching and waits for the goroutine to exit.
func (w *Watcher) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.done
}

func (w *Watcher) run() {
	defer close(w.done)
	prev := w.scan()
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-t.C:
			cur := w.scan()
			if ch, changed := diff(prev, cur); changed {
				w.onChange(ch)
			}
			prev = cur
		}
	}
}

// scan fingerprints every relevant file by modification time and size. Our own
// metadata store, VCS data, and in-flight atomic-write temp files are skipped.
func (w *Watcher) scan() map[string]meta {
	out := map[string]meta{}
	_ = filepath.WalkDir(w.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // transient (e.g. a file removed mid-walk) — ignore
		}
		if d.IsDir() {
			if n := d.Name(); n == ".novelide" || n == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".tmp-") {
			return nil // atomic-write temp file
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(w.root, p)
		if err != nil {
			return nil
		}
		out[filepath.ToSlash(rel)] = meta{modNano: info.ModTime().UnixNano(), size: info.Size()}
		return nil
	})
	return out
}

func diff(prev, cur map[string]meta) (Change, bool) {
	var ch Change
	for p, m := range cur {
		if pm, ok := prev[p]; !ok {
			ch.Structural = append(ch.Structural, p) // added
		} else if pm != m {
			ch.Modified = append(ch.Modified, p) // contents changed
		}
	}
	for p := range prev {
		if _, ok := cur[p]; !ok {
			ch.Structural = append(ch.Structural, p) // removed
		}
	}
	sort.Strings(ch.Modified)
	sort.Strings(ch.Structural)
	return ch, len(ch.Modified) > 0 || len(ch.Structural) > 0
}
