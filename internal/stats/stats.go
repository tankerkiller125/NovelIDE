// Package stats tracks writing progress — words written per day per
// workspace — by watching the total manuscript word count over time and
// attributing each change to the current day. Persisted to the user's
// config dir so daily totals and streaks survive restarts.
package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// data is one workspace's tracked history.
type data struct {
	Goal        int            `json:"goal"`        // daily word goal (0 = none)
	LastTotal   int            `json:"lastTotal"`   // manuscript word count at last check
	Initialized bool           `json:"initialized"` // false until the first baseline is set
	Days        map[string]int `json:"days"`        // date (YYYY-MM-DD) -> words written that day
}

type file struct {
	Workspaces map[string]*data `json:"workspaces"`
}

// Stats is the snapshot handed to the frontend.
type Stats struct {
	Today      string `json:"today"`      // YYYY-MM-DD
	TodayWords int    `json:"todayWords"` // net words written today
	Goal       int    `json:"goal"`       // daily goal (0 = none)
	Streak     int    `json:"streak"`     // consecutive days meeting the goal (or with any writing if no goal)
	Total      int    `json:"total"`      // current total manuscript word count
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "novelide", "stats.json"), nil
}

func load() *file {
	f := &file{Workspaces: map[string]*data{}}
	p, err := configPath()
	if err != nil {
		return f
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return f
	}
	_ = json.Unmarshal(b, f)
	if f.Workspaces == nil {
		f.Workspaces = map[string]*data{}
	}
	return f
}

func save(f *file) {
	p, err := configPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	if b, err := json.MarshalIndent(f, "", "  "); err == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
}

func (d *data) ensure() {
	if d.Days == nil {
		d.Days = map[string]int{}
	}
}

func today() string { return time.Now().Format("2006-01-02") }

// snapshot builds the returned Stats, computing the streak.
func (d *data) snapshot(total int) Stats {
	d.ensure()
	t := today()
	s := Stats{Today: t, TodayWords: d.Days[t], Goal: d.Goal, Total: total}
	// Streak: walk back day by day while the goal is met (or, with no goal,
	// while there was any writing).
	day := time.Now()
	for {
		key := day.Format("2006-01-02")
		w := d.Days[key]
		met := (d.Goal > 0 && w >= d.Goal) || (d.Goal == 0 && w > 0)
		if !met {
			// Today not yet meeting the goal doesn't break a prior streak.
			if key == t {
				day = day.AddDate(0, 0, -1)
				continue
			}
			break
		}
		s.Streak++
		day = day.AddDate(0, 0, -1)
	}
	return s
}

// Record attributes the change in total manuscript words since the last
// check to today, then returns the snapshot. The first call for a workspace
// only baselines (it does not credit pre-existing words to today).
func Record(wsPath string, currentTotal int) Stats {
	f := load()
	d := f.Workspaces[wsPath]
	if d == nil {
		d = &data{}
		f.Workspaces[wsPath] = d
	}
	d.ensure()
	if !d.Initialized {
		d.Initialized = true
		d.LastTotal = currentTotal
		save(f)
		return d.snapshot(currentTotal)
	}
	delta := currentTotal - d.LastTotal
	if delta != 0 {
		t := today()
		d.Days[t] += delta
		if d.Days[t] < 0 {
			d.Days[t] = 0
		}
		d.LastTotal = currentTotal
		save(f)
	}
	return d.snapshot(currentTotal)
}

// SetGoal updates a workspace's daily word goal and returns the snapshot.
func SetGoal(wsPath string, goal, currentTotal int) Stats {
	if goal < 0 {
		goal = 0
	}
	f := load()
	d := f.Workspaces[wsPath]
	if d == nil {
		d = &data{Initialized: true, LastTotal: currentTotal}
		f.Workspaces[wsPath] = d
	}
	d.Goal = goal
	save(f)
	return d.snapshot(currentTotal)
}
