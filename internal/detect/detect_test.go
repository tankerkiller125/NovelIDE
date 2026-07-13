package detect

import (
	"testing"

	"novelide/internal/match"
	"novelide/internal/model"
	"novelide/internal/nlp"
)

func testWorkspace() *model.Workspace {
	return &model.Workspace{
		Manifest: model.Manifest{Name: "Test", Kind: model.KindSeries, Books: []string{"book-one", "book-two"}},
		Books: []model.Book{
			{ID: "book-one", Title: "Book One", Chapters: []string{"01-start.md", "02-the-fall.md", "03-after.md"}},
			{ID: "book-two", Title: "Book Two", Chapters: []string{"01-return.md"}},
		},
		Codex: []model.CodexEntry{
			{
				ID: "aria", Name: "Aria", Type: "character",
				Status: []model.StatusChange{
					{State: "alive"},
					{State: "dead", At: model.StoryPoint{Book: "book-one", Chapter: "02-the-fall.md"}},
				},
			},
			{ID: "kael", Name: "Kael", Type: "character"},
		},
	}
}

func scanAndCheck(ws *model.Workspace, book, chapter, text string) []Flag {
	m := match.New(ws.Codex)
	doc, err := nlp.Parse(text)
	if err != nil {
		panic(err)
	}
	return Check(ws, book, chapter, m.Scan(text), doc)
}

func TestDeadCharacterActingIsFlagged(t *testing.T) {
	ws := testWorkspace()
	flags := scanAndCheck(ws, "book-one", "03-after.md", "Aria walked into the room.")
	if len(flags) != 1 {
		t.Fatalf("want 1 flag, got %+v", flags)
	}
	if flags[0].Rule != "dead-entity-agency" || flags[0].Severity != SevError {
		t.Errorf("got %+v", flags[0])
	}
}

func TestDeadCharacterActsInLaterBook(t *testing.T) {
	ws := testWorkspace()
	flags := scanAndCheck(ws, "book-two", "01-return.md", "said Aria")
	if len(flags) != 1 || flags[0].Rule != "dead-entity-agency" {
		t.Fatalf("dialogue attribution after death should flag, got %+v", flags)
	}
}

func TestMentionOfDeadCharacterIsInfoOnly(t *testing.T) {
	ws := testWorkspace()
	flags := scanAndCheck(ws, "book-one", "03-after.md", "Kael missed Aria. Aria's sword hung on the wall. Aria was gone.")
	for _, f := range flags {
		if f.Severity == SevError {
			t.Errorf("pure mention flagged as error: %+v", f)
		}
	}
	if len(flags) == 0 {
		t.Error("expected info-level mentions")
	}
}

func TestDeadBodyPostureIsNotAgency(t *testing.T) {
	ws := testWorkspace()
	// A corpse can lie, sit, or rest somewhere — postural verbs describe a
	// state, not an action, so they must not be flagged as the dead acting.
	for _, text := range []string{
		"They found the spot where Aria lay.",
		"Aria lay still among the reeds.",
		"Aria lay dead on the stones.",
		"The body of Aria rested against the wall.",
		"Aria sat slumped where she had fallen.",
	} {
		flags := scanAndCheck(ws, "book-one", "03-after.md", text)
		for _, f := range flags {
			if f.Severity == SevError {
				t.Errorf("%q wrongly flagged as agency: %+v", text, f)
			}
		}
	}
}

func TestDeadCharacterDeliberateMotionStillFlags(t *testing.T) {
	ws := testWorkspace()
	// "lay down" / "sat up" are deliberate movements a corpse can't make.
	flags := scanAndCheck(ws, "book-one", "03-after.md", "Aria sat up and looked around.")
	if len(flags) == 0 {
		t.Error("a dead character deliberately sitting up should still flag")
	}
}

func TestNoFlagBeforeDeath(t *testing.T) {
	ws := testWorkspace()
	if flags := scanAndCheck(ws, "book-one", "01-start.md", "Aria walked in."); len(flags) != 0 {
		t.Errorf("no flags expected before death, got %+v", flags)
	}
}

func TestNoFlagInDeathChapter(t *testing.T) {
	ws := testWorkspace()
	if flags := scanAndCheck(ws, "book-one", "02-the-fall.md", "Aria fell. Aria died."); len(flags) != 0 {
		t.Errorf("death scene itself should not flag, got %+v", flags)
	}
}

func TestAliveCharacterNeverFlagged(t *testing.T) {
	ws := testWorkspace()
	if flags := scanAndCheck(ws, "book-two", "01-return.md", "Kael walked on alone."); len(flags) != 0 {
		t.Errorf("alive character flagged: %+v", flags)
	}
}
