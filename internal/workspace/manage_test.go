package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"novelide/internal/model"
)

func manageWS(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	ws, err := Create(dir, "Cycle", model.KindSeries)
	if err != nil {
		t.Fatal(err)
	}
	b := ws.Books[0].ID // has 01-chapter-one.md
	if _, err := CreateChapter(dir, b, "The Fall"); err != nil {
		t.Fatal(err) // 02-the-fall.md
	}
	if _, err := CreateChapter(dir, b, "After"); err != nil {
		t.Fatal(err) // 03-after.md
	}
	return dir, b
}

func chapterSet(t *testing.T, dir, book string) map[string]bool {
	t.Helper()
	ws, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, bk := range ws.Books {
		if bk.ID == book {
			out := map[string]bool{}
			for _, c := range bk.Chapters {
				out[c] = true
			}
			return out
		}
	}
	return nil
}

func TestRenameChapterRewritesAnchors(t *testing.T) {
	dir, b := manageWS(t)
	e := &model.CodexEntry{
		Name: "Aria", Type: "character", Scope: ScopeSeries,
		Status:    []model.StatusChange{{State: "dead", At: model.StoryPoint{Book: b, Chapter: "02-the-fall.md"}}},
		Relations: []model.Relation{{Type: "owns", To: "sword", Until: &model.StoryPoint{Book: b, Chapter: "02-the-fall.md"}}},
	}
	if err := SaveEntry(dir, e); err != nil {
		t.Fatal(err)
	}
	newName, err := RenameChapter(dir, b, "02-the-fall.md", "The Great Fall")
	if err != nil {
		t.Fatal(err)
	}
	if newName != "02-the-great-fall.md" {
		t.Fatalf("new name = %q", newName)
	}
	if chapterSet(t, dir, b)["02-the-fall.md"] || !chapterSet(t, dir, b)["02-the-great-fall.md"] {
		t.Error("file not renamed on disk")
	}
	ws, _ := Load(dir)
	for _, ce := range ws.Codex {
		if ce.ID != "aria" {
			continue
		}
		if ce.Status[0].At.Chapter != "02-the-great-fall.md" {
			t.Errorf("status anchor not rewritten: %+v", ce.Status[0].At)
		}
		if ce.Relations[0].Until.Chapter != "02-the-great-fall.md" {
			t.Errorf("relation anchor not rewritten: %+v", ce.Relations[0].Until)
		}
	}
}

func TestDeleteChapterRenumbersAndDropsAnchors(t *testing.T) {
	dir, b := manageWS(t)
	e := &model.CodexEntry{
		Name: "Aria", Type: "character", Scope: ScopeSeries,
		Status: []model.StatusChange{
			{State: "alive"},
			{State: "dead", At: model.StoryPoint{Book: b, Chapter: "02-the-fall.md"}}, // will be deleted
		},
		Relations: []model.Relation{
			{Type: "owns", To: "sword", From: &model.StoryPoint{Book: b, Chapter: "03-after.md"}}, // shifts to 02
		},
	}
	if err := SaveEntry(dir, e); err != nil {
		t.Fatal(err)
	}
	if err := DeleteChapter(dir, b, "02-the-fall.md"); err != nil {
		t.Fatal(err)
	}
	set := chapterSet(t, dir, b)
	if set["02-the-fall.md"] {
		t.Error("deleted chapter still present")
	}
	if !set["01-chapter-one.md"] || !set["02-after.md"] || len(set) != 2 {
		t.Errorf("chapters not renumbered to 01/02: %v", set)
	}
	ws, _ := Load(dir)
	for _, ce := range ws.Codex {
		if ce.ID != "aria" {
			continue
		}
		// the dead-at-02-the-fall status is dropped; only "alive" remains
		if len(ce.Status) != 1 || ce.Status[0].State != "alive" {
			t.Errorf("removed-chapter status not dropped: %+v", ce.Status)
		}
		// relation from 03-after.md follows the renumber to 02-after.md
		if ce.Relations[0].From == nil || ce.Relations[0].From.Chapter != "02-after.md" {
			t.Errorf("shifted relation anchor wrong: %+v", ce.Relations[0].From)
		}
	}
}

func TestRenameBook(t *testing.T) {
	dir, b := manageWS(t)
	if err := RenameBook(dir, b, "The Ashen Crown"); err != nil {
		t.Fatal(err)
	}
	ws, _ := Load(dir)
	if ws.Books[0].Title != "The Ashen Crown" {
		t.Errorf("title = %q", ws.Books[0].Title)
	}
	// id (directory) is unchanged
	if ws.Books[0].ID != b {
		t.Errorf("book id changed to %q", ws.Books[0].ID)
	}
}

func TestDeleteBookCleansRefs(t *testing.T) {
	dir, b1 := manageWS(t)
	b2, err := CreateBook(dir, "Second")
	if err != nil {
		t.Fatal(err)
	}
	// a codex entry anchored into the book we will delete
	e := &model.CodexEntry{
		Name: "Aria", Type: "character", Scope: ScopeSeries,
		Status: []model.StatusChange{
			{State: "alive"},
			{State: "dead", At: model.StoryPoint{Book: b2}},
		},
		Relations: []model.Relation{
			{Type: "owns", To: "sword", Until: &model.StoryPoint{Book: b2}},
		},
	}
	if err := SaveEntry(dir, e); err != nil {
		t.Fatal(err)
	}
	if err := SaveSeriesPlan(dir, model.SeriesPlan{Books: []model.SeriesBookPlan{{ID: b1}, {ID: b2}}}); err != nil {
		t.Fatal(err)
	}

	if err := DeleteBook(dir, b2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, BooksDir, b2)); !os.IsNotExist(err) {
		t.Error("book directory not removed")
	}
	ws, _ := Load(dir)
	if len(ws.Books) != 1 || ws.Books[0].ID != b1 {
		t.Errorf("manifest not updated: %+v", ws.Manifest.Books)
	}
	for _, ce := range ws.Codex {
		if ce.ID != "aria" {
			continue
		}
		if len(ce.Status) != 1 || ce.Status[0].State != "alive" {
			t.Errorf("status anchored to deleted book not dropped: %+v", ce.Status)
		}
		if ce.Relations[0].Until != nil {
			t.Errorf("relation bound to deleted book not cleared: %+v", ce.Relations[0].Until)
		}
	}
	if len(ws.SeriesPlan.Books) != 1 || ws.SeriesPlan.Books[0].ID != b1 {
		t.Errorf("series plan not cleaned: %+v", ws.SeriesPlan.Books)
	}

	// cannot delete the last book
	if err := DeleteBook(dir, b1); err == nil {
		t.Error("deleting the only book should fail")
	}
}
