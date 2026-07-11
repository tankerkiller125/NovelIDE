package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"novelide/internal/model"
)

func TestPlanRoundTripAndReorder(t *testing.T) {
	dir := t.TempDir()
	ws, err := Create(dir, "Planned", model.KindNovel)
	if err != nil {
		t.Fatal(err)
	}
	bookID := ws.Books[0].ID
	if _, err := CreateChapter(dir, bookID, "The Fall"); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateChapter(dir, bookID, "After"); err != nil {
		t.Fatal(err)
	}
	// Chapters: 01-chapter-one.md, 02-the-fall.md, 03-after.md

	plan := []model.ChapterPlan{
		{File: "01-chapter-one.md", Synopsis: "Aria rides out.", Status: "drafted", POV: "aria"},
		{File: "02-the-fall.md", Synopsis: "Aria falls.", Status: "outlined", Arcs: []string{"doom-arc"}},
	}
	if err := SavePlan(dir, bookID, plan); err != nil {
		t.Fatal(err)
	}

	entry := &model.CodexEntry{
		Name: "Aria", Type: "character", Scope: ScopeSeries,
		Status: []model.StatusChange{
			{State: "alive"},
			{State: "dead", At: model.StoryPoint{Book: bookID, Chapter: "02-the-fall.md"}},
		},
		Relations: []model.Relation{
			{Type: "owns", To: "sword", Until: &model.StoryPoint{Book: bookID, Chapter: "02-the-fall.md"}},
		},
	}
	if err := SaveEntry(dir, entry); err != nil {
		t.Fatal(err)
	}

	// Move "the fall" later: 02 <-> 03.
	if err := ReorderChapter(dir, bookID, "02-the-fall.md", 1); err != nil {
		t.Fatal(err)
	}

	ws2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"01-chapter-one.md", "02-after.md", "03-the-fall.md"}
	got := ws2.Books[0].Chapters
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("chapter order after reorder: %v, want %v", got, want)
	}

	// Codex anchors must follow the renamed file.
	for _, e := range ws2.Codex {
		if e.ID != "aria" {
			continue
		}
		if e.Status[1].At.Chapter != "03-the-fall.md" {
			t.Errorf("status anchor not rewritten: %+v", e.Status[1].At)
		}
		if e.Relations[0].Until.Chapter != "03-the-fall.md" {
			t.Errorf("relation anchor not rewritten: %+v", e.Relations[0].Until)
		}
	}

	// Plan entries must follow too, and keep their content.
	p := ws2.Books[0].Plan
	byFile := map[string]model.ChapterPlan{}
	for _, cp := range p {
		byFile[cp.File] = cp
	}
	if byFile["03-the-fall.md"].Synopsis != "Aria falls." {
		t.Errorf("plan did not follow rename: %+v", p)
	}
	if byFile["01-chapter-one.md"].POV != "aria" {
		t.Errorf("untouched plan entry damaged: %+v", p)
	}

	// Stale plan entries (missing files) are pruned on load.
	if err := os.Remove(filepath.Join(dir, BooksDir, bookID, ManuscriptDir, "01-chapter-one.md")); err != nil {
		t.Fatal(err)
	}
	ws3, _ := Load(dir)
	for _, cp := range ws3.Books[0].Plan {
		if cp.File == "01-chapter-one.md" {
			t.Error("stale plan entry survived load")
		}
	}
}

func TestWordCount(t *testing.T) {
	if n := WordCount("# Chapter One\n\nAria rode out — twelve words? No: *eight*.\n"); n != 9 {
		t.Errorf("word count = %d, want 9 (em-dash and # don't count)", n)
	}
}

func TestSeriesPlanAndMoveBook(t *testing.T) {
	dir := t.TempDir()
	ws, err := Create(dir, "Cycle", model.KindSeries)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := CreateBook(dir, "The Second")
	if err != nil {
		t.Fatal(err)
	}
	b1 := ws.Books[0].ID

	plan := model.SeriesPlan{
		Synopsis: "A two-book cycle.",
		Books: []model.SeriesBookPlan{
			{ID: b1, Synopsis: "The fall.", Status: "drafted", Arcs: []string{"doom"}, TargetWords: 90000},
			{ID: "ghost-book", Synopsis: "stale"},
		},
	}
	if err := SaveSeriesPlan(dir, plan); err != nil {
		t.Fatal(err)
	}
	ws2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws2.SeriesPlan.Synopsis != "A two-book cycle." {
		t.Errorf("series synopsis lost: %+v", ws2.SeriesPlan)
	}
	if len(ws2.SeriesPlan.Books) != 1 || ws2.SeriesPlan.Books[0].TargetWords != 90000 {
		t.Errorf("stale book card should be pruned, real one kept: %+v", ws2.SeriesPlan.Books)
	}

	if err := MoveBook(dir, b2, -1); err != nil {
		t.Fatal(err)
	}
	ws3, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws3.Manifest.Books[0] != b2 || ws3.Manifest.Books[1] != b1 {
		t.Errorf("book order after move: %v", ws3.Manifest.Books)
	}
	// Edge moves are no-ops.
	if err := MoveBook(dir, b2, -1); err != nil {
		t.Fatal(err)
	}
	ws4, _ := Load(dir)
	if ws4.Manifest.Books[0] != b2 {
		t.Errorf("edge move should be a no-op: %v", ws4.Manifest.Books)
	}
}
