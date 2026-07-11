package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"novelide/internal/model"
)

// RenameChapter changes a chapter's filename slug to match a new title,
// keeping its numeric order prefix, and rewrites every codex anchor and plan
// entry that referenced the old filename. Returns the new filename. The
// chapter's prose (including any `# heading`) is left untouched.
func RenameChapter(wsPath, bookID, chapter, newTitle string) (string, error) {
	if err := validateName(bookID); err != nil {
		return "", err
	}
	if err := validateName(chapter); err != nil {
		return "", err
	}
	book, err := loadBook(wsPath, bookID)
	if err != nil {
		return "", err
	}
	found := false
	for _, c := range book.Chapters {
		if c == chapter {
			found = true
		}
	}
	if !found {
		return "", fmt.Errorf("chapter %q not found", chapter)
	}
	newName := chapterPrefix.FindString(chapter) + Slugify(newTitle) + ".md"
	if newName == chapter {
		return chapter, nil
	}
	plan := loadPlan(wsPath, bookID, book.Chapters)
	if err := applyChapterEdits(wsPath, bookID, map[string]string{chapter: newName}, nil, plan); err != nil {
		return "", err
	}
	return newName, nil
}

// DeleteChapter removes a chapter file, renumbers the remaining chapters to
// stay sequential, and rewrites/drops every codex anchor and plan entry that
// referenced a removed or renamed file.
func DeleteChapter(wsPath, bookID, chapter string) error {
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
	var kept []string
	for _, c := range book.Chapters {
		if c != chapter {
			kept = append(kept, c)
		}
	}
	if len(kept) == len(book.Chapters) {
		return fmt.Errorf("chapter %q not found", chapter)
	}
	renames := map[string]string{}
	for i, old := range kept {
		fresh := fmt.Sprintf("%02d-%s", i+1, chapterPrefix.ReplaceAllString(old, ""))
		if fresh != old {
			renames[old] = fresh
		}
	}
	plan := loadPlan(wsPath, bookID, book.Chapters)
	return applyChapterEdits(wsPath, bookID, renames, map[string]bool{chapter: true}, plan)
}

// applyChapterEdits performs manuscript file removals and renames (two-phase
// to avoid collisions) and then reconciles all codex anchors and the plan.
func applyChapterEdits(wsPath, bookID string, renames map[string]string, removed map[string]bool, plan []model.ChapterPlan) error {
	dir := filepath.Join(wsPath, BooksDir, bookID, ManuscriptDir)
	for f := range removed {
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for old := range renames {
		if err := os.Rename(filepath.Join(dir, old), filepath.Join(dir, old+".edit~")); err != nil {
			return err
		}
	}
	for old, fresh := range renames {
		if err := os.Rename(filepath.Join(dir, old+".edit~"), filepath.Join(dir, fresh)); err != nil {
			return err
		}
	}
	if err := updateCodexForChapterChanges(wsPath, bookID, renames, removed); err != nil {
		return err
	}
	// Reconcile the plan: drop removed chapters, follow renames.
	if len(plan) > 0 {
		var np []model.ChapterPlan
		for _, cp := range plan {
			if removed[cp.File] {
				continue
			}
			if fresh, ok := renames[cp.File]; ok {
				cp.File = fresh
			}
			np = append(np, cp)
		}
		if err := SavePlan(wsPath, bookID, np); err != nil {
			return err
		}
	}
	return nil
}

// updateCodexForChapterChanges rewrites anchors of renamed chapters and drops
// anchors of removed chapters, across every codex entry.
func updateCodexForChapterChanges(wsPath, bookID string, renames map[string]string, removed map[string]bool) error {
	ws, err := Load(wsPath)
	if err != nil {
		return err
	}
	// fixPoint returns (drop, changed): drop means the containing status /
	// relation bound refers to a chapter that no longer exists.
	fixPoint := func(p *model.StoryPoint) (drop, changed bool) {
		if p == nil || p.Book != bookID || p.Chapter == "" {
			return false, false
		}
		if removed[p.Chapter] {
			return true, true
		}
		if fresh, ok := renames[p.Chapter]; ok {
			p.Chapter = fresh
			return false, true
		}
		return false, false
	}
	for i := range ws.Codex {
		e := &ws.Codex[i]
		changed := false
		var st []model.StatusChange
		for _, s := range e.Status {
			drop, ch := fixPoint(&s.At)
			if drop {
				changed = true
				continue
			}
			if ch {
				changed = true
			}
			st = append(st, s)
		}
		e.Status = st
		for j := range e.Relations {
			if e.Relations[j].From != nil {
				if drop, ch := fixPoint(e.Relations[j].From); drop {
					e.Relations[j].From = nil
					changed = true
				} else if ch {
					changed = true
				}
			}
			if e.Relations[j].Until != nil {
				if drop, ch := fixPoint(e.Relations[j].Until); drop {
					e.Relations[j].Until = nil
					changed = true
				} else if ch {
					changed = true
				}
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

// RenameBook changes a book's display title (book.yaml). The book's
// directory id is a stable structural identifier and is intentionally left
// unchanged, so codex anchors and the reading order stay valid.
func RenameBook(wsPath, bookID, newTitle string) error {
	if err := validateName(bookID); err != nil {
		return err
	}
	p := filepath.Join(wsPath, BooksDir, bookID, BookFile)
	if _, err := os.Stat(p); err != nil {
		return fmt.Errorf("book %q not found", bookID)
	}
	return writeYAML(p, model.Book{Title: newTitle})
}

// DeleteBook removes a book from the series: drops it from the manifest,
// deletes its directory, and cleans every codex anchor and series-plan entry
// that pointed at it. Refuses to delete the last remaining book.
func DeleteBook(wsPath, bookID string) error {
	if err := validateName(bookID); err != nil {
		return err
	}
	var man model.Manifest
	if err := readYAML(filepath.Join(wsPath, ManifestFile), &man); err != nil {
		return err
	}
	if len(man.Books) <= 1 {
		return fmt.Errorf("cannot delete the only book in the workspace")
	}
	found := false
	kept := man.Books[:0]
	for _, b := range man.Books {
		if b == bookID {
			found = true
			continue
		}
		kept = append(kept, b)
	}
	if !found {
		return fmt.Errorf("book %q not found", bookID)
	}
	man.Books = kept
	if err := writeYAML(filepath.Join(wsPath, ManifestFile), man); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(wsPath, BooksDir, bookID)); err != nil {
		return err
	}

	// Clean codex anchors that referenced the deleted book.
	ws, err := Load(wsPath)
	if err != nil {
		return err
	}
	for i := range ws.Codex {
		e := &ws.Codex[i]
		changed := false
		var st []model.StatusChange
		for _, s := range e.Status {
			if s.At.Book == bookID {
				changed = true
				continue
			}
			st = append(st, s)
		}
		e.Status = st
		for j := range e.Relations {
			if e.Relations[j].From != nil && e.Relations[j].From.Book == bookID {
				e.Relations[j].From = nil
				changed = true
			}
			if e.Relations[j].Until != nil && e.Relations[j].Until.Book == bookID {
				e.Relations[j].Until = nil
				changed = true
			}
		}
		if changed {
			if err := SaveEntry(wsPath, e); err != nil {
				return err
			}
		}
	}

	// Clean the series plan.
	var sp model.SeriesPlan
	if readYAML(filepath.Join(wsPath, SeriesPlanFile), &sp) == nil && len(sp.Books) > 0 {
		var keptBooks []model.SeriesBookPlan
		removed := false
		for _, bc := range sp.Books {
			if bc.ID == bookID {
				removed = true
				continue
			}
			keptBooks = append(keptBooks, bc)
		}
		if removed {
			sp.Books = keptBooks
			if err := SaveSeriesPlan(wsPath, sp); err != nil {
				return err
			}
		}
	}
	return nil
}
