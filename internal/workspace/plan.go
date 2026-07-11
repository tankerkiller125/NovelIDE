package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"novelide/internal/model"
)

const (
	PlanFile       = "plan.yaml"
	SeriesPlanFile = "series-plan.yaml"
)

// loadSeriesPlan reads series-plan.yaml, dropping book cards whose book no
// longer exists in the manifest.
func loadSeriesPlan(wsPath string, bookIDs []string) model.SeriesPlan {
	var plan model.SeriesPlan
	if err := readYAML(filepath.Join(wsPath, SeriesPlanFile), &plan); err != nil {
		return model.SeriesPlan{}
	}
	exists := map[string]bool{}
	for _, id := range bookIDs {
		exists[id] = true
	}
	kept := plan.Books[:0]
	for _, b := range plan.Books {
		if exists[b.ID] {
			kept = append(kept, b)
		}
	}
	plan.Books = kept
	return plan
}

// SaveSeriesPlan writes the workspace-level series plan.
func SaveSeriesPlan(wsPath string, plan model.SeriesPlan) error {
	return writeYAML(filepath.Join(wsPath, SeriesPlanFile), plan)
}

// MoveBook shifts a book earlier (-1) or later (+1) in the series. Book
// order lives in the manifest, so this is a pure metadata change — no file
// renames, and codex anchors (which reference book ids) stay valid. It DOES
// change the story timeline, which is the point.
func MoveBook(wsPath, bookID string, delta int) error {
	var man model.Manifest
	if err := readYAML(filepath.Join(wsPath, ManifestFile), &man); err != nil {
		return err
	}
	idx := -1
	for i, id := range man.Books {
		if id == bookID {
			idx = i
		}
	}
	if idx == -1 {
		return fmt.Errorf("book %q not found", bookID)
	}
	target := idx + delta
	if target < 0 || target >= len(man.Books) {
		return nil
	}
	man.Books[idx], man.Books[target] = man.Books[target], man.Books[idx]
	return writeYAML(filepath.Join(wsPath, ManifestFile), man)
}

// loadPlan reads a book's plan.yaml, dropping entries whose chapter file no
// longer exists (renamed or deleted outside the app).
func loadPlan(wsPath, bookID string, chapters []string) []model.ChapterPlan {
	var plan model.BookPlan
	p := filepath.Join(wsPath, BooksDir, bookID, PlanFile)
	if err := readYAML(p, &plan); err != nil {
		return nil
	}
	exists := map[string]bool{}
	for _, c := range chapters {
		exists[c] = true
	}
	out := plan.Chapters[:0]
	for _, cp := range plan.Chapters {
		if exists[cp.File] {
			out = append(out, cp)
		}
	}
	return out
}

// SavePlan writes a book's plan.yaml.
func SavePlan(wsPath, bookID string, plan []model.ChapterPlan) error {
	if err := validateName(bookID); err != nil {
		return err
	}
	return writeYAML(filepath.Join(wsPath, BooksDir, bookID, PlanFile), model.BookPlan{Chapters: plan})
}

var chapterPrefix = regexp.MustCompile(`^\d+-`)

// ReorderChapter moves a chapter earlier (delta -1) or later (+1) in its
// book. Chapter order IS filename order, so every chapter is renumbered to
// match the new sequence — and every reference to a renamed file (codex
// status anchors, relation bounds, the plan) is rewritten to follow.
func ReorderChapter(wsPath, bookID, chapter string, delta int) error {
	if err := validateName(bookID); err != nil {
		return err
	}
	if err := validateName(chapter); err != nil {
		return err
	}
	book, err := loadBook(wsPath, bookID)
	if err != nil {
		return err
	}
	idx := -1
	for i, c := range book.Chapters {
		if c == chapter {
			idx = i
		}
	}
	if idx == -1 {
		return fmt.Errorf("chapter %q not found", chapter)
	}
	target := idx + delta
	if target < 0 || target >= len(book.Chapters) {
		return nil // nothing to do at the edges
	}
	order := append([]string{}, book.Chapters...)
	order[idx], order[target] = order[target], order[idx]

	// Capture the plan before renaming: loadPlan prunes entries whose
	// files are missing, which after a rename would be all the moved ones.
	plan := loadPlan(wsPath, bookID, book.Chapters)

	// Renumber everything to the new order: "NN-rest.md".
	renames := map[string]string{}
	for i, old := range order {
		rest := chapterPrefix.ReplaceAllString(old, "")
		fresh := fmt.Sprintf("%02d-%s", i+1, rest)
		if fresh != old {
			renames[old] = fresh
		}
	}
	if len(renames) == 0 {
		return nil
	}

	// Two-phase rename via temp names so 01↔02 swaps can't collide.
	dir := filepath.Join(wsPath, BooksDir, bookID, ManuscriptDir)
	for old := range renames {
		if err := os.Rename(filepath.Join(dir, old), filepath.Join(dir, old+".reorder~")); err != nil {
			return err
		}
	}
	for old, fresh := range renames {
		if err := os.Rename(filepath.Join(dir, old+".reorder~"), filepath.Join(dir, fresh)); err != nil {
			return err
		}
	}

	if err := rewriteCodexRefs(wsPath, bookID, renames); err != nil {
		return err
	}
	if len(plan) > 0 {
		for j := range plan {
			if fresh, ok := renames[plan[j].File]; ok {
				plan[j].File = fresh
			}
		}
		if err := SavePlan(wsPath, bookID, plan); err != nil {
			return err
		}
	}
	return nil
}

// rewriteCodexRefs updates every codex anchor that points at a renamed
// chapter of the given book.
func rewriteCodexRefs(wsPath, bookID string, renames map[string]string) error {
	ws, err := Load(wsPath)
	if err != nil {
		return err
	}
	fix := func(p *model.StoryPoint) bool {
		if p == nil || p.Book != bookID {
			return false
		}
		if fresh, ok := renames[p.Chapter]; ok {
			p.Chapter = fresh
			return true
		}
		return false
	}
	for i := range ws.Codex {
		e := &ws.Codex[i]
		changed := false
		for j := range e.Status {
			if fix(&e.Status[j].At) {
				changed = true
			}
		}
		for j := range e.Relations {
			if fix(e.Relations[j].From) {
				changed = true
			}
			if fix(e.Relations[j].Until) {
				changed = true
			}
		}
		if changed {
			if err := SaveEntry(wsPath, e); err != nil {
				return err
			}
		}
	}
	return nil
}

// WordCount counts prose words: whitespace-separated tokens containing at
// least one letter or digit (markdown markers and bare dashes don't count).
func WordCount(text string) int {
	n := 0
	for _, f := range strings.Fields(text) {
		for _, r := range f {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				n++
				break
			}
		}
	}
	return n
}
