// Package workspace handles loading, creating, and mutating NovelIDE
// workspaces on disk. All content lives in plain files (YAML + Markdown) so
// projects stay portable, greppable, and git-friendly.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"novelide/internal/model"
)

const (
	ManifestFile  = "novelide.yaml"
	SchemaFile    = "codex-schema.yaml"
	BookFile      = "book.yaml"
	BooksDir      = "books"
	CodexDir      = "codex"
	ManuscriptDir = "manuscript"

	// ScopeSeries marks codex entries stored at the workspace root,
	// shared across every book.
	ScopeSeries = "series"
)

var slugStrip = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify turns a display name into a filesystem/id-safe slug.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugStrip.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "untitled"
	}
	return s
}

// Create initializes a new workspace directory. For a series, one starter
// book is created; for a novel, a single book directory holds everything.
func Create(path, name string, kind model.WorkspaceKind) (*model.Workspace, error) {
	if kind != model.KindNovel && kind != model.KindSeries {
		return nil, fmt.Errorf("invalid workspace kind %q", kind)
	}
	if _, err := os.Stat(filepath.Join(path, ManifestFile)); err == nil {
		return nil, fmt.Errorf("a workspace already exists at %s", path)
	}
	bookID := "book-one"
	bookTitle := "Book One"
	if kind == model.KindNovel {
		bookID = Slugify(name)
		bookTitle = name
	}
	man := model.Manifest{Name: name, Kind: kind, Books: []string{bookID}}

	schema := model.DefaultSchema()
	for _, t := range schema.Types {
		if err := os.MkdirAll(filepath.Join(path, CodexDir, t.ID), 0o755); err != nil {
			return nil, err
		}
	}
	if err := writeYAML(filepath.Join(path, SchemaFile), schema); err != nil {
		return nil, err
	}
	if err := writeYAML(filepath.Join(path, ManifestFile), man); err != nil {
		return nil, err
	}
	if err := createBookDir(path, bookID, bookTitle); err != nil {
		return nil, err
	}
	if err := os.WriteFile(chapterPath(path, bookID, "01-chapter-one.md"), []byte("# Chapter One\n\n"), 0o644); err != nil {
		return nil, err
	}
	return Load(path)
}

// Load reads a full workspace from disk.
func Load(path string) (*model.Workspace, error) {
	var man model.Manifest
	if err := readYAML(filepath.Join(path, ManifestFile), &man); err != nil {
		return nil, fmt.Errorf("not a NovelIDE workspace (missing %s): %w", ManifestFile, err)
	}
	ws := &model.Workspace{Path: path, Manifest: man, Schema: loadSchema(path)}

	for _, bookID := range man.Books {
		book, err := loadBook(path, bookID)
		if err != nil {
			return nil, err
		}
		ws.Books = append(ws.Books, *book)
	}

	ws.SeriesPlan = loadSeriesPlan(path, man.Books)

	entries, err := loadCodexDir(filepath.Join(path, CodexDir), ScopeSeries)
	if err != nil {
		return nil, err
	}
	ws.Codex = entries
	for _, bookID := range man.Books {
		bookEntries, err := loadCodexDir(filepath.Join(path, BooksDir, bookID, CodexDir), bookID)
		if err != nil {
			return nil, err
		}
		ws.Codex = append(ws.Codex, bookEntries...)
	}
	return ws, nil
}

func loadBook(wsPath, bookID string) (*model.Book, error) {
	dir := filepath.Join(wsPath, BooksDir, bookID)
	var book model.Book
	if err := readYAML(filepath.Join(dir, BookFile), &book); err != nil {
		return nil, fmt.Errorf("loading book %q: %w", bookID, err)
	}
	book.ID = bookID
	files, err := os.ReadDir(filepath.Join(dir, ManuscriptDir))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".md") {
			book.Chapters = append(book.Chapters, f.Name())
		}
	}
	sort.Strings(book.Chapters)
	book.Plan = loadPlan(wsPath, bookID, book.Chapters)
	return &book, nil
}

// loadSchema reads codex-schema.yaml, falling back to the default schema for
// workspaces that predate (or deleted) it. Types found on disk but missing
// from the schema are appended so no entries silently disappear from the UI.
func loadSchema(wsPath string) model.Schema {
	var schema model.Schema
	if err := readYAML(filepath.Join(wsPath, SchemaFile), &schema); err != nil || len(schema.Types) == 0 {
		schema = model.DefaultSchema()
	}
	return schema
}

// EnsureSchemaFile writes the default schema file if none exists yet, so
// users always have something concrete to edit.
func EnsureSchemaFile(wsPath string) error {
	p := filepath.Join(wsPath, SchemaFile)
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	return writeYAML(p, model.DefaultSchema())
}

// SaveSchema persists the workspace schema.
func SaveSchema(wsPath string, schema model.Schema) error {
	for i := range schema.Types {
		if schema.Types[i].ID == "" {
			schema.Types[i].ID = Slugify(schema.Types[i].Label)
		}
	}
	for i := range schema.Relations {
		if schema.Relations[i].ID == "" {
			schema.Relations[i].ID = Slugify(schema.Relations[i].Label)
		}
	}
	return writeYAML(filepath.Join(wsPath, SchemaFile), schema)
}

// loadCodexDir loads every codex entry under dir. Each subdirectory is a
// type id — including ones not (yet) declared in the schema.
func loadCodexDir(dir, scope string) ([]model.CodexEntry, error) {
	typeDirs, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []model.CodexEntry
	for _, td := range typeDirs {
		if !td.IsDir() {
			continue
		}
		t := td.Name()
		typeDir := filepath.Join(dir, t)
		files, err := os.ReadDir(typeDir)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".yaml") {
				continue
			}
			var e model.CodexEntry
			p := filepath.Join(typeDir, f.Name())
			if err := readYAML(p, &e); err != nil {
				return nil, fmt.Errorf("parsing codex entry %s: %w", p, err)
			}
			if e.Type == "" {
				e.Type = t
			}
			if e.ID == "" {
				e.ID = strings.TrimSuffix(f.Name(), ".yaml")
			}
			e.Scope = scope
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// entryPath returns the YAML file path for an entry given its scope.
func entryPath(wsPath, scope string, e *model.CodexEntry) string {
	base := filepath.Join(wsPath, CodexDir)
	if scope != ScopeSeries {
		base = filepath.Join(wsPath, BooksDir, scope, CodexDir)
	}
	return filepath.Join(base, Slugify(e.Type), e.ID+".yaml")
}

// SaveEntry writes a codex entry to disk, creating directories as needed.
// If the entry previously lived at a different path (type or scope changed),
// the caller should delete the old file via DeleteEntry first.
func SaveEntry(wsPath string, e *model.CodexEntry) error {
	if e.ID == "" {
		e.ID = Slugify(e.Name)
	}
	if e.Type == "" {
		e.Type = "concept"
	}
	e.Type = Slugify(e.Type)
	scope := e.Scope
	if scope == "" {
		scope = ScopeSeries
	}
	p := entryPath(wsPath, scope, e)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return writeYAML(p, e)
}

// DeleteEntry removes a codex entry's file.
func DeleteEntry(wsPath string, e *model.CodexEntry) error {
	scope := e.Scope
	if scope == "" {
		scope = ScopeSeries
	}
	err := os.Remove(entryPath(wsPath, scope, e))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func chapterPath(wsPath, bookID, chapter string) string {
	return filepath.Join(wsPath, BooksDir, bookID, ManuscriptDir, chapter)
}

// ReadChapter returns the markdown content of a chapter.
func ReadChapter(wsPath, bookID, chapter string) (string, error) {
	if err := validateName(bookID); err != nil {
		return "", err
	}
	if err := validateName(chapter); err != nil {
		return "", err
	}
	b, err := os.ReadFile(chapterPath(wsPath, bookID, chapter))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteChapter saves markdown content to a chapter file.
func WriteChapter(wsPath, bookID, chapter, content string) error {
	if err := validateName(bookID); err != nil {
		return err
	}
	if err := validateName(chapter); err != nil {
		return err
	}
	return os.WriteFile(chapterPath(wsPath, bookID, chapter), []byte(content), 0o644)
}

// CreateChapter adds a new markdown file to a book, auto-numbering it after
// the existing chapters. Returns the new file name.
func CreateChapter(wsPath, bookID, title string) (string, error) {
	if err := validateName(bookID); err != nil {
		return "", err
	}
	book, err := loadBook(wsPath, bookID)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%02d-%s.md", len(book.Chapters)+1, Slugify(title))
	p := chapterPath(wsPath, bookID, name)
	if _, err := os.Stat(p); err == nil {
		return "", fmt.Errorf("chapter file %s already exists", name)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte("# "+title+"\n\n"), 0o644); err != nil {
		return "", err
	}
	return name, nil
}

// CreateBook adds a new book directory and registers it in the manifest.
// Returns the new book ID.
func CreateBook(wsPath, title string) (string, error) {
	var man model.Manifest
	if err := readYAML(filepath.Join(wsPath, ManifestFile), &man); err != nil {
		return "", err
	}
	id := fmt.Sprintf("%02d-%s", len(man.Books)+1, Slugify(title))
	if err := createBookDir(wsPath, id, title); err != nil {
		return "", err
	}
	man.Books = append(man.Books, id)
	if man.Kind == model.KindNovel {
		man.Kind = model.KindSeries // adding a second book promotes to series
	}
	if err := writeYAML(filepath.Join(wsPath, ManifestFile), man); err != nil {
		return "", err
	}
	return id, nil
}

func createBookDir(wsPath, id, title string) error {
	dir := filepath.Join(wsPath, BooksDir, id)
	if err := os.MkdirAll(filepath.Join(dir, ManuscriptDir), 0o755); err != nil {
		return err
	}
	return writeYAML(filepath.Join(dir, BookFile), model.Book{Title: title})
}

// validateName rejects path components that could escape the workspace.
func validateName(name string) error {
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid name %q", name)
	}
	return nil
}

func readYAML(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, out)
}

func writeYAML(path string, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
