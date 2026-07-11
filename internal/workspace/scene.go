package workspace

import (
	"fmt"
	"regexp"
	"strings"
)

// Scenes are sections *within* a chapter file, separated by an HTML-comment
// marker that carries an optional title:
//
//	<!-- scene: The Ash Farms -->
//
// Keeping scenes inside the chapter file (rather than as separate files)
// means every chapter reference the app already relies on — codex timeline
// anchors, plans, export, detection — keeps working unchanged. The corkboard
// is a view over these sections.
//
// Scene 0 of a chapter is the "opening": everything before the first marker
// (including the chapter's `# heading`). It stays first and is not movable.
var sceneRe = regexp.MustCompile(`(?m)^[ \t]*<!--[ \t]*scene(?:[ \t]*:[ \t]*(.*?))?[ \t]*-->[ \t]*$`)

// Scene is the display shape for one scene (no raw body — see sceneRaw).
type Scene struct {
	Index   int    `json:"index"`
	Title   string `json:"title"`
	Words   int    `json:"words"`
	Snippet string `json:"snippet"`
}

// ChapterScenes bundles a chapter's scenes for the corkboard.
type ChapterScenes struct {
	Chapter string  `json:"chapter"`
	Title   string  `json:"title"` // prettified chapter name
	Scenes  []Scene `json:"scenes"`
}

type sceneRaw struct {
	title string
	body  string // raw text of the scene (marker line excluded)
}

func parseScenes(text string) []sceneRaw {
	locs := sceneRe.FindAllStringSubmatchIndex(text, -1)
	if len(locs) == 0 {
		return []sceneRaw{{body: text}}
	}
	out := []sceneRaw{{body: text[:locs[0][0]]}} // opening
	for i, loc := range locs {
		title := ""
		if loc[2] >= 0 {
			title = strings.TrimSpace(text[loc[2]:loc[3]])
		}
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		out = append(out, sceneRaw{title: title, body: text[loc[1]:end]})
	}
	return out
}

func markerLine(title string) string {
	title = strings.TrimSpace(strings.ReplaceAll(title, "-->", ""))
	if title == "" {
		return "<!-- scene -->"
	}
	return "<!-- scene: " + title + " -->"
}

func serializeScenes(scenes []sceneRaw) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(scenes[0].body, " \t\r\n"))
	for i := 1; i < len(scenes); i++ {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(markerLine(scenes[i].title))
		if body := strings.Trim(scenes[i].body, " \t\r\n"); body != "" {
			b.WriteString("\n\n")
			b.WriteString(body)
		}
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}

var headingLine = regexp.MustCompile(`(?m)^#{1,6}[ \t]+`)

func sceneSnippet(body string) string {
	// Drop leading markdown headings, take the first prose up to ~140 chars.
	clean := strings.TrimSpace(headingLine.ReplaceAllString(body, ""))
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > 140 {
		clean = strings.TrimSpace(clean[:140]) + "…"
	}
	return clean
}

// ChapterScenesFor parses a chapter's markdown into display scenes.
func ChapterScenesFor(text string) []Scene {
	raw := parseScenes(text)
	out := make([]Scene, len(raw))
	for i, s := range raw {
		out[i] = Scene{
			Index:   i,
			Title:   s.title,
			Words:   WordCount(s.body),
			Snippet: sceneSnippet(s.body),
		}
	}
	return out
}

// BookScenes returns every chapter's scenes, in reading order.
func BookScenes(wsPath, bookID string) ([]ChapterScenes, error) {
	if err := validateName(bookID); err != nil {
		return nil, err
	}
	book, err := loadBook(wsPath, bookID)
	if err != nil {
		return nil, err
	}
	out := make([]ChapterScenes, 0, len(book.Chapters))
	for _, ch := range book.Chapters {
		text, err := ReadChapter(wsPath, bookID, ch)
		if err != nil {
			return nil, err
		}
		out = append(out, ChapterScenes{
			Chapter: ch,
			Title:   prettyChapterName(ch),
			Scenes:  ChapterScenesFor(text),
		})
	}
	return out, nil
}

func prettyChapterName(file string) string {
	name := strings.TrimSuffix(file, ".md")
	name = chapterPrefix.ReplaceAllString(name, "")
	return strings.ReplaceAll(name, "-", " ")
}

var chapterH1 = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)

// ChapterTitle is a chapter's display title: its first `# heading` if present,
// otherwise the prettified filename.
func ChapterTitle(text, file string) string {
	if m := chapterH1.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	return prettyChapterName(file)
}

func removeScene(s []sceneRaw, i int) []sceneRaw {
	return append(append([]sceneRaw{}, s[:i]...), s[i+1:]...)
}

func insertScene(s []sceneRaw, i int, v sceneRaw) []sceneRaw {
	if i < 1 {
		i = 1
	}
	if i > len(s) {
		i = len(s)
	}
	out := append([]sceneRaw{}, s[:i]...)
	out = append(out, v)
	return append(out, s[i:]...)
}

// MoveScene moves a scene (index >= 1; the opening can't move) within a
// chapter or to another chapter of the same book. Chapter files are
// rewritten; no codex anchors or filenames change.
func MoveScene(wsPath, bookID, srcChapter string, sceneIndex int, dstChapter string, dstIndex int) error {
	if err := validateName(bookID); err != nil {
		return err
	}
	if sceneIndex < 1 {
		return fmt.Errorf("the opening scene cannot be moved")
	}
	srcText, err := ReadChapter(wsPath, bookID, srcChapter)
	if err != nil {
		return err
	}
	srcScenes := parseScenes(srcText)
	if sceneIndex >= len(srcScenes) {
		return fmt.Errorf("scene index %d out of range", sceneIndex)
	}
	moved := srcScenes[sceneIndex]

	if srcChapter == dstChapter {
		scenes := removeScene(srcScenes, sceneIndex)
		scenes = insertScene(scenes, dstIndex, moved)
		return WriteChapter(wsPath, bookID, srcChapter, serializeScenes(scenes))
	}
	// Cross-chapter: remove from source, insert into destination.
	srcScenes = removeScene(srcScenes, sceneIndex)
	if err := WriteChapter(wsPath, bookID, srcChapter, serializeScenes(srcScenes)); err != nil {
		return err
	}
	dstText, err := ReadChapter(wsPath, bookID, dstChapter)
	if err != nil {
		return err
	}
	dstScenes := insertScene(parseScenes(dstText), dstIndex, moved)
	return WriteChapter(wsPath, bookID, dstChapter, serializeScenes(dstScenes))
}

// SetSceneTitle renames a scene (index >= 1).
func SetSceneTitle(wsPath, bookID, chapter string, sceneIndex int, title string) error {
	if err := validateName(bookID); err != nil {
		return err
	}
	if sceneIndex < 1 {
		return fmt.Errorf("the opening scene has no title")
	}
	text, err := ReadChapter(wsPath, bookID, chapter)
	if err != nil {
		return err
	}
	scenes := parseScenes(text)
	if sceneIndex >= len(scenes) {
		return fmt.Errorf("scene index %d out of range", sceneIndex)
	}
	scenes[sceneIndex].title = title
	return WriteChapter(wsPath, bookID, chapter, serializeScenes(scenes))
}
