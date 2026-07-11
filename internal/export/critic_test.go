package export

import (
	"strings"
	"testing"
)

func TestStripCriticMarkup(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Plain prose, nothing to strip.", "Plain prose, nothing to strip."},
		{"A line.{>> fix this later <<}", "A line."},
		{"She {==drew her blade==}{>> too soon? <<} and struck.", "She drew her blade and struck."},
		{"Kept {++inserted++} text.", "Kept inserted text."},
		{"Removed {--deleted--}text.", "Removed text."},
		{"They said {~~hi~>hello~~}.", "They said hello."},
		{"Multi\n{>> a note\nspanning lines <<}\nline.", "Multi\n\nline."},
		{"Two {>>a<<} notes {>>b<<} here.", "Two  notes  here."},
	}
	for _, c := range cases {
		if got := stripCriticMarkup(c.in); got != c.want {
			t.Errorf("strip(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestConvertSceneBreaks(t *testing.T) {
	in := "Scene one.\n\n<!-- scene: Two -->\n\nScene two.\n\n<!-- scene -->\n\nScene three."
	got := convertSceneBreaks(in)
	if strings.Contains(got, "<!-- scene") {
		t.Errorf("scene markers not converted: %q", got)
	}
	if strings.Count(got, "***") != 2 {
		t.Errorf("expected 2 thematic breaks, got %q", got)
	}
}
