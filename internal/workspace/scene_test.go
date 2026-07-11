package workspace

import (
	"strings"
	"testing"

	"novelide/internal/model"
)

func TestParseScenes(t *testing.T) {
	text := "# Chapter One\n\nOpening prose.\n\n<!-- scene: The Gate -->\n\nGate prose.\n\n<!-- scene -->\n\nUntitled scene.\n"
	sc := ChapterScenesFor(text)
	if len(sc) != 3 {
		t.Fatalf("want 3 scenes (opening + 2), got %d: %+v", len(sc), sc)
	}
	if sc[0].Title != "" || sc[1].Title != "The Gate" || sc[2].Title != "" {
		t.Errorf("titles wrong: %q %q %q", sc[0].Title, sc[1].Title, sc[2].Title)
	}
	if !strings.Contains(sc[0].Snippet, "Opening prose") {
		t.Errorf("opening snippet dropped heading? %q", sc[0].Snippet)
	}
	// no markers → single opening scene
	if s := ChapterScenesFor("Just prose, no scenes."); len(s) != 1 {
		t.Errorf("markerless chapter should be one scene, got %d", len(s))
	}
}

func sceneWS(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	ws, err := Create(dir, "Book", model.KindNovel)
	if err != nil {
		t.Fatal(err)
	}
	b := ws.Books[0].ID
	body := "# Chapter One\n\nOpening.\n\n<!-- scene: Alpha -->\n\nAlpha body.\n\n<!-- scene: Beta -->\n\nBeta body.\n"
	if err := WriteChapter(dir, b, "01-chapter-one.md", body); err != nil {
		t.Fatal(err)
	}
	return dir, b
}

func TestMoveSceneWithinChapter(t *testing.T) {
	dir, b := sceneWS(t)
	// swap Alpha (1) and Beta (2): move scene 2 to index 1
	if err := MoveScene(dir, b, "01-chapter-one.md", 2, "01-chapter-one.md", 1); err != nil {
		t.Fatal(err)
	}
	sc := mustScenes(t, dir, b, "01-chapter-one.md")
	if sc[1].Title != "Beta" || sc[2].Title != "Alpha" {
		t.Errorf("scenes not reordered: %q %q", sc[1].Title, sc[2].Title)
	}
	// opening still first and intact
	if sc[0].Title != "" || !strings.Contains(sc[0].Snippet, "Opening") {
		t.Errorf("opening disturbed: %+v", sc[0])
	}
}

func TestMoveSceneAcrossChapters(t *testing.T) {
	dir, b := sceneWS(t)
	if _, err := CreateChapter(dir, b, "Two"); err != nil {
		t.Fatal(err)
	}
	// move Beta (scene 2 of ch1) into ch2 at index 1
	if err := MoveScene(dir, b, "01-chapter-one.md", 2, "02-two.md", 1); err != nil {
		t.Fatal(err)
	}
	c1 := mustScenes(t, dir, b, "01-chapter-one.md")
	if len(c1) != 2 || c1[1].Title != "Alpha" {
		t.Errorf("source not reduced correctly: %+v", c1)
	}
	c2 := mustScenes(t, dir, b, "02-two.md")
	if len(c2) != 2 || c2[1].Title != "Beta" {
		t.Errorf("destination missing moved scene: %+v", c2)
	}
	// the moved prose came along
	txt, _ := ReadChapter(dir, b, "02-two.md")
	if !strings.Contains(txt, "Beta body") {
		t.Error("scene body did not move with the scene")
	}

	// the opening cannot be moved
	if err := MoveScene(dir, b, "01-chapter-one.md", 0, "02-two.md", 1); err == nil {
		t.Error("moving the opening scene should fail")
	}
}

func TestSetSceneTitle(t *testing.T) {
	dir, b := sceneWS(t)
	if err := SetSceneTitle(dir, b, "01-chapter-one.md", 1, "The Beginning"); err != nil {
		t.Fatal(err)
	}
	sc := mustScenes(t, dir, b, "01-chapter-one.md")
	if sc[1].Title != "The Beginning" {
		t.Errorf("title not set: %q", sc[1].Title)
	}
	txt, _ := ReadChapter(dir, b, "01-chapter-one.md")
	if !strings.Contains(txt, "<!-- scene: The Beginning -->") {
		t.Errorf("marker not rewritten: %q", txt)
	}
}

func mustScenes(t *testing.T, dir, book, chapter string) []Scene {
	t.Helper()
	txt, err := ReadChapter(dir, book, chapter)
	if err != nil {
		t.Fatal(err)
	}
	return ChapterScenesFor(txt)
}
