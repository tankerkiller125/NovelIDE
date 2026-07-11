package main

import (
	"strings"
	"testing"
)

func TestMentionSnippet(t *testing.T) {
	// A mention deep inside a long chapter is clipped on both sides.
	long := strings.Repeat("word ", 60) + "Aria drew her blade " + strings.Repeat("word ", 60)
	at := strings.Index(long, "Aria")
	// convert byte index to rune index (ASCII here, so equal)
	got := mentionSnippet(long, at)
	if !strings.Contains(got, "Aria") {
		t.Fatalf("snippet lost the mention: %q", got)
	}
	if !strings.HasPrefix(got, "… ") || !strings.HasSuffix(got, " …") {
		t.Errorf("expected ellipses on both sides, got %q", got)
	}

	// A mention near the start has no leading ellipsis.
	short := "Aria drew her blade and struck."
	got = mentionSnippet(short, 0)
	if strings.HasPrefix(got, "… ") {
		t.Errorf("no leading ellipsis expected for start-of-text: %q", got)
	}
	if got != "Aria drew her blade and struck." {
		t.Errorf("short snippet mangled: %q", got)
	}

	if mentionSnippet("anything", -1) != "" {
		t.Error("negative offset should yield empty snippet")
	}
}
