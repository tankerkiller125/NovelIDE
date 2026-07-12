package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"novelide/internal/deep"
	"novelide/internal/detect"
	"novelide/internal/export"
	"novelide/internal/history"
	"novelide/internal/match"
	"novelide/internal/model"
	"novelide/internal/nlp"
	"novelide/internal/settings"
	"novelide/internal/spell"
	"novelide/internal/spellcheck"
	"novelide/internal/stats"
	"novelide/internal/syncclient"
	"novelide/internal/syncproto"
	"novelide/internal/workspace"
)

// App is the Wails-bound backend API.
type App struct {
	ctx context.Context

	mu       sync.RWMutex
	ws       *model.Workspace
	matcher  *match.Matcher
	settings settings.Settings
	deep     *deep.Engine
	spell    *spell.Engine
}

func NewApp() *App {
	return &App{
		settings: settings.Load(),
		deep:     deep.NewEngine(),
		spell:    spell.NewEngine(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// WebKitGTK ships with spell checking off at the context level; turn it
	// on per the user's settings — this covers plain inputs (codex forms).
	// The manuscript editor uses our own hunspell engine instead, because
	// CodeMirror's DOM syncing erases webview spelling markers.
	spellcheck.Set(a.settings.EditorSpellcheck, a.settings.SpellcheckLang)
	if a.settings.EditorSpellcheck {
		go a.spell.Load(a.settings.SpellcheckLang)
	}
}

// AppVersion returns the running app's version (from the release tag, or
// "dev" for source builds), for display in the UI.
func (a *App) AppVersion() string {
	return Version()
}

// GetSettings returns the persisted application settings.
func (a *App) GetSettings() settings.Settings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settings
}

// SaveSettings persists application settings.
func (a *App) SaveSettings(s settings.Settings) (settings.Settings, error) {
	s = settings.Sanitize(s)
	if err := settings.Save(s); err != nil {
		return s, err
	}
	a.mu.Lock()
	a.settings = s
	a.mu.Unlock()
	spellcheck.Set(s.EditorSpellcheck, s.SpellcheckLang)
	if s.EditorSpellcheck {
		go a.spell.Load(s.SpellcheckLang)
	}
	return s, nil
}

// SpellStatus reports why the spell engine is unavailable ("" = healthy),
// so Settings can explain missing dictionaries instead of failing silently.
func (a *App) SpellStatus() string {
	if a.spell.Ready() {
		return ""
	}
	if err := a.spell.Err; err != nil {
		return err.Error()
	}
	return "spell engine not loaded"
}

// SpellSuggest returns spelling suggestions for a word.
func (a *App) SpellSuggest(word string) []string {
	out := a.spell.Suggest(word)
	if out == nil {
		out = []string{}
	}
	return out
}

// AddToDictionary records a word in the user's personal dictionary.
func (a *App) AddToDictionary(word string) error {
	return a.spell.AddPersonal(word)
}

// CloseWorkspace drops the open workspace so the frontend can return to the
// welcome screen.
func (a *App) CloseWorkspace() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ws = nil
	a.matcher = nil
}

// ScanResult bundles entity mentions, consistency flags, codex-gap
// suggestions, and misspellings for a chapter.
type ScanResult struct {
	Spans        []match.Span        `json:"spans"`
	Flags        []detect.Flag       `json:"flags"`
	Suggestions  []detect.Suggestion `json:"suggestions"`
	Misspellings []spell.Word        `json:"misspellings"`
}

func (a *App) setWorkspace(ws *model.Workspace) *model.Workspace {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ws = ws
	a.matcher = match.New(ws.Codex)
	settings.Touch(&a.settings, ws.Path)
	return ws
}

func (a *App) workspacePath() (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.ws == nil {
		return "", fmt.Errorf("no workspace open")
	}
	return a.ws.Path, nil
}

// SelectFolder opens a native directory picker and returns the chosen path
// (empty string if cancelled).
func (a *App) SelectFolder(title string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title})
}

// ExportThemes lists the available book themes (id/label/description).
func (a *App) ExportThemes() []export.Theme {
	return export.BuiltinThemes()
}

// ExportPreview renders the current workspace to themed HTML for the export
// preview pane. Always HTML regardless of the target format.
func (a *App) ExportPreview(opts export.Options) (string, error) {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return "", fmt.Errorf("no workspace open")
	}
	opts.Format = export.FormatHTML
	res, err := export.Export(ws, opts)
	if err != nil {
		return "", err
	}
	return string(res.Bytes), nil
}

// ExportSave compiles the workspace to the requested format and writes it to
// a location the user picks. Returns the saved path, or "" if cancelled.
func (a *App) ExportSave(opts export.Options) (string, error) {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return "", fmt.Errorf("no workspace open")
	}
	res, err := export.Export(ws, opts)
	if err != nil {
		return "", err
	}
	filter := runtime.FileFilter{DisplayName: "EPUB e-book (*.epub)", Pattern: "*.epub"}
	if opts.Format == export.FormatHTML {
		filter = runtime.FileFilter{DisplayName: "HTML (*.html)", Pattern: "*.html"}
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export book",
		DefaultFilename: res.Filename,
		Filters:         []runtime.FileFilter{filter},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // user cancelled
	}
	if err := os.WriteFile(path, res.Bytes, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// CreateWorkspace initializes a new novel or series at path.
func (a *App) CreateWorkspace(path, name, kind string) (*model.Workspace, error) {
	ws, err := workspace.Create(path, name, model.WorkspaceKind(kind))
	if err != nil {
		return nil, err
	}
	return a.setWorkspace(ws), nil
}

// OpenWorkspace loads an existing workspace from path, materializing the
// schema file for older workspaces so it can be hand-edited.
func (a *App) OpenWorkspace(path string) (*model.Workspace, error) {
	ws, err := workspace.Load(path)
	if err != nil {
		return nil, err
	}
	if err := workspace.EnsureSchemaFile(path); err != nil {
		return nil, err
	}
	return a.setWorkspace(ws), nil
}

// SaveSchema persists the codex schema (types + relationship definitions)
// and returns the refreshed workspace.
func (a *App) SaveSchema(schema model.Schema) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := workspace.SaveSchema(path, schema); err != nil {
		return nil, err
	}
	return a.reload()
}

// reload re-reads the workspace from disk after a mutation.
func (a *App) reload() (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	ws, err := workspace.Load(path)
	if err != nil {
		return nil, err
	}
	return a.setWorkspace(ws), nil
}

// SaveCodexEntry creates or updates a codex entry and returns the refreshed
// workspace. oldType/oldScope identify the previous file location when the
// entry is being moved (pass empty strings for new entries).
func (a *App) SaveCodexEntry(entry model.CodexEntry, oldType, oldScope string) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if oldType != "" && oldScope != "" &&
		(oldType != entry.Type || oldScope != entry.Scope) {
		old := entry
		old.Type = oldType
		old.Scope = oldScope
		if err := workspace.DeleteEntry(path, &old); err != nil {
			return nil, err
		}
	}
	if err := workspace.SaveEntry(path, &entry); err != nil {
		return nil, err
	}
	return a.reload()
}

// DeleteCodexEntry removes an entry and returns the refreshed workspace.
func (a *App) DeleteCodexEntry(entry model.CodexEntry) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := workspace.DeleteEntry(path, &entry); err != nil {
		return nil, err
	}
	return a.reload()
}

// PickEntryImage opens a file dialog, copies the chosen image into the
// workspace, records it on the entry, and returns the refreshed workspace.
// Cancelling leaves the workspace unchanged.
func (a *App) PickEntryImage(entry model.CodexEntry) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	src, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose an image",
		Filters: []runtime.FileFilter{
			{DisplayName: "Images", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp"},
		},
	})
	if err != nil {
		return nil, err
	}
	if src == "" {
		return a.reload() // cancelled — return the current workspace
	}
	if err := workspace.SetEntryImage(path, &entry, src); err != nil {
		return nil, err
	}
	return a.reload()
}

// ClearEntryImage removes an entry's image and returns the refreshed workspace.
func (a *App) ClearEntryImage(entry model.CodexEntry) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := workspace.ClearEntryImage(path, &entry); err != nil {
		return nil, err
	}
	return a.reload()
}

// ReadImageDataURL reads a workspace-relative image and returns it as a
// base64 data URL for display in the webview.
func (a *App) ReadImageDataURL(rel string) (string, error) {
	path, err := a.workspacePath()
	if err != nil {
		return "", err
	}
	b, err := workspace.ReadImage(path, rel)
	if err != nil {
		return "", err
	}
	mime := http.DetectContentType(b)
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

// ReadChapter returns a chapter's markdown content.
func (a *App) ReadChapter(bookID, chapter string) (string, error) {
	path, err := a.workspacePath()
	if err != nil {
		return "", err
	}
	return workspace.ReadChapter(path, bookID, chapter)
}

// SaveChapter writes a chapter's markdown content.
func (a *App) SaveChapter(bookID, chapter, content string) error {
	path, err := a.workspacePath()
	if err != nil {
		return err
	}
	if err := workspace.WriteChapter(path, bookID, chapter, content); err != nil {
		return err
	}
	a.maybeAutoSnapshot(path)
	return nil
}

// maybeAutoSnapshot takes an automatic snapshot at most once per day, as a
// safety net beneath the author's manual snapshots. Best-effort: a failure
// here never blocks a save.
func (a *App) maybeAutoSnapshot(wsPath string) {
	now := time.Now()
	if history.LatestIsFromDay(wsPath, now) {
		return
	}
	_, _, _ = history.Create(wsPath, "", true, now)
}

// CreateChapter adds a chapter to a book and returns the refreshed workspace
// plus the new chapter's file name.
type CreateChapterResult struct {
	Workspace *model.Workspace `json:"workspace"`
	Chapter   string           `json:"chapter"`
}

func (a *App) CreateChapter(bookID, title string) (*CreateChapterResult, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	name, err := workspace.CreateChapter(path, bookID, title)
	if err != nil {
		return nil, err
	}
	ws, err := a.reload()
	if err != nil {
		return nil, err
	}
	return &CreateChapterResult{Workspace: ws, Chapter: name}, nil
}

// CreateBook adds a book to the series and returns the refreshed workspace.
func (a *App) CreateBook(title string) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if _, err := workspace.CreateBook(path, title); err != nil {
		return nil, err
	}
	return a.reload()
}

// RenameChapterResult returns the refreshed workspace plus the chapter's new
// filename (so the frontend can re-point an open tab).
type RenameChapterResult struct {
	Workspace *model.Workspace `json:"workspace"`
	Chapter   string           `json:"chapter"`
}

// RenameChapter changes a chapter's filename to match a new title and rewrites
// every codex anchor and plan reference to it.
func (a *App) RenameChapter(bookID, chapter, newTitle string) (*RenameChapterResult, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	name, err := workspace.RenameChapter(path, bookID, chapter, newTitle)
	if err != nil {
		return nil, err
	}
	ws, err := a.reload()
	if err != nil {
		return nil, err
	}
	return &RenameChapterResult{Workspace: ws, Chapter: name}, nil
}

// DeleteChapter removes a chapter, renumbers the rest, and cleans references.
func (a *App) DeleteChapter(bookID, chapter string) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := workspace.DeleteChapter(path, bookID, chapter); err != nil {
		return nil, err
	}
	return a.reload()
}

// RenameBook changes a book's display title.
func (a *App) RenameBook(bookID, newTitle string) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := workspace.RenameBook(path, bookID, newTitle); err != nil {
		return nil, err
	}
	return a.reload()
}

// DeleteBook removes a book from the series and cleans references.
func (a *App) DeleteBook(bookID string) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := workspace.DeleteBook(path, bookID); err != nil {
		return nil, err
	}
	return a.reload()
}

// workspaceWordCount sums the word count of every manuscript chapter.
func (a *App) workspaceWordCount() int {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return 0
	}
	total := 0
	for _, b := range ws.Books {
		for _, ch := range b.Chapters {
			if text, err := workspace.ReadChapter(ws.Path, b.ID, ch); err == nil {
				total += workspace.WordCount(text)
			}
		}
	}
	return total
}

// RecordWritingProgress recomputes the total manuscript word count, credits
// any change since the last check to today, and returns the writing stats.
func (a *App) RecordWritingProgress() (stats.Stats, error) {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return stats.Stats{}, fmt.Errorf("no workspace open")
	}
	return stats.Record(ws.Path, a.workspaceWordCount()), nil
}

// SetDailyGoal sets the workspace's daily word goal (0 = none).
func (a *App) SetDailyGoal(goal int) (stats.Stats, error) {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return stats.Stats{}, fmt.Errorf("no workspace open")
	}
	return stats.SetGoal(ws.Path, goal, a.workspaceWordCount()), nil
}

// ScanText finds entity mentions and consistency flags in (unsaved) chapter
// text. Offsets are rune offsets; the frontend maps them to editor positions.
func (a *App) ScanText(bookID, chapter, text string) (*ScanResult, error) {
	a.mu.RLock()
	ws, matcher := a.ws, a.matcher
	a.mu.RUnlock()
	if ws == nil {
		return nil, fmt.Errorf("no workspace open")
	}
	spans := matcher.Scan(text)
	// One NLP parse (POS tags, sentences, NER) shared by all three passes.
	doc, err := nlp.Parse(text)
	if err != nil {
		doc = nil // grammar-based detection degrades gracefully; highlighting still works
	}
	flags := detect.Check(ws, bookID, chapter, spans, doc)
	suggestions := detect.Suggest(ws, bookID, chapter, spans, doc)
	extraSuggestions, extraFlags := detect.Extract(ws, bookID, chapter, spans, doc)
	suggestions = append(suggestions, extraSuggestions...)
	flags = append(flags, extraFlags...)
	if spans == nil {
		spans = []match.Span{}
	}
	if flags == nil {
		flags = []detect.Flag{}
	}
	if suggestions == nil {
		suggestions = []detect.Suggestion{}
	}
	misspellings := a.checkSpelling(text, spans)
	return &ScanResult{Spans: spans, Flags: flags, Suggestions: suggestions, Misspellings: misspellings}, nil
}

// checkSpelling runs the hunspell engine over the chapter, skipping words
// inside codex entity mentions — character and place names are never typos.
func (a *App) checkSpelling(text string, spans []match.Span) []spell.Word {
	out := []spell.Word{}
	if !a.settings.EditorSpellcheck || !a.spell.Ready() {
		return out
	}
	for _, w := range spell.Words(text) {
		inEntity := false
		for _, sp := range spans {
			if w.Start < sp.End && w.End > sp.Start {
				inEntity = true
				break
			}
		}
		if inEntity || a.spell.Check(w.Text) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// SavePlan persists a book's planning cards and returns the refreshed
// workspace.
func (a *App) SavePlan(bookID string, plan []model.ChapterPlan) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := workspace.SavePlan(path, bookID, plan); err != nil {
		return nil, err
	}
	return a.reload()
}

// MoveChapter shifts a chapter up (-1) or down (+1) within its book,
// renumbering files and rewriting every codex anchor and plan entry that
// referenced a renamed chapter.
func (a *App) MoveChapter(bookID, chapter string, delta int) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := workspace.ReorderChapter(path, bookID, chapter, delta); err != nil {
		return nil, err
	}
	return a.reload()
}

// SaveSeriesPlan persists the series-level plan and returns the refreshed
// workspace.
func (a *App) SaveSeriesPlan(plan model.SeriesPlan) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := workspace.SaveSeriesPlan(path, plan); err != nil {
		return nil, err
	}
	return a.reload()
}

// MoveBook shifts a book within the series' reading order (manifest only —
// no files move, but the story timeline changes accordingly).
func (a *App) MoveBook(bookID string, delta int) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := workspace.MoveBook(path, bookID, delta); err != nil {
		return nil, err
	}
	return a.reload()
}

// ChapterInsight is derived (not stored) planning data for one chapter.
type ChapterInsight struct {
	// Cast lists codex entry ids actually mentioned in the chapter text.
	Cast  []string `json:"cast"`
	Words int      `json:"words"`
}

// BookInsights scans every chapter of a book and reports who actually
// appears and how long each chapter is — the live half of the plan cards.
func (a *App) BookInsights(bookID string) (map[string]ChapterInsight, error) {
	a.mu.RLock()
	ws, matcher := a.ws, a.matcher
	a.mu.RUnlock()
	if ws == nil {
		return nil, fmt.Errorf("no workspace open")
	}
	var book *model.Book
	for i := range ws.Books {
		if ws.Books[i].ID == bookID {
			book = &ws.Books[i]
		}
	}
	if book == nil {
		return nil, fmt.Errorf("unknown book %q", bookID)
	}
	out := map[string]ChapterInsight{}
	for _, ch := range book.Chapters {
		text, err := workspace.ReadChapter(ws.Path, bookID, ch)
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		cast := []string{}
		for _, sp := range matcher.Scan(text) {
			if !seen[sp.EntryID] {
				seen[sp.EntryID] = true
				cast = append(cast, sp.EntryID)
			}
		}
		out[ch] = ChapterInsight{Cast: cast, Words: workspace.WordCount(text)}
	}
	return out, nil
}

// BookScenes returns every chapter's scenes in reading order for the
// corkboard view.
func (a *App) BookScenes(bookID string) ([]workspace.ChapterScenes, error) {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return nil, fmt.Errorf("no workspace open")
	}
	return workspace.BookScenes(ws.Path, bookID)
}

// MoveScene reorders a scene within a chapter or moves it to another chapter
// of the same book, then returns the book's refreshed scene layout.
func (a *App) MoveScene(bookID, srcChapter string, sceneIndex int, dstChapter string, dstIndex int) ([]workspace.ChapterScenes, error) {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return nil, fmt.Errorf("no workspace open")
	}
	if err := workspace.MoveScene(ws.Path, bookID, srcChapter, sceneIndex, dstChapter, dstIndex); err != nil {
		return nil, err
	}
	return workspace.BookScenes(ws.Path, bookID)
}

// SetSceneTitle renames a scene and returns the refreshed scene layout.
func (a *App) SetSceneTitle(bookID, chapter string, sceneIndex int, title string) ([]workspace.ChapterScenes, error) {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return nil, fmt.Errorf("no workspace open")
	}
	if err := workspace.SetSceneTitle(ws.Path, bookID, chapter, sceneIndex, title); err != nil {
		return nil, err
	}
	return workspace.BookScenes(ws.Path, bookID)
}

// SearchHit groups a chapter's matches for the project-wide search view.
type SearchHit struct {
	BookID       string                `json:"bookId"`
	BookTitle    string                `json:"bookTitle"`
	Chapter      string                `json:"chapter"`
	ChapterTitle string                `json:"chapterTitle"`
	Matches      []workspace.TextMatch `json:"matches"`
}

// SearchResults is the outcome of a project-wide search.
type SearchResults struct {
	Hits  []SearchHit `json:"hits"`
	Total int         `json:"total"` // total match count across all chapters
}

// SearchProject searches every chapter of every book for the query and returns
// matches grouped by chapter, in reading order.
func (a *App) SearchProject(query string, caseSensitive, wholeWord bool) (*SearchResults, error) {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return nil, fmt.Errorf("no workspace open")
	}
	res := &SearchResults{Hits: []SearchHit{}}
	for _, book := range ws.Books {
		for _, ch := range book.Chapters {
			text, err := workspace.ReadChapter(ws.Path, book.ID, ch)
			if err != nil {
				continue
			}
			matches, err := workspace.SearchText(text, query, caseSensitive, wholeWord)
			if err != nil {
				return nil, err
			}
			if len(matches) == 0 {
				continue
			}
			res.Hits = append(res.Hits, SearchHit{
				BookID:       book.ID,
				BookTitle:    book.Title,
				Chapter:      ch,
				ChapterTitle: workspace.ChapterTitle(text, ch),
				Matches:      matches,
			})
			res.Total += len(matches)
		}
	}
	return res, nil
}

// ReplaceAllProject replaces every match of query across the whole workspace,
// rewriting the affected chapter files, and returns how many replacements were
// made. Plain-file storage means each chapter is still individually editable
// and diffable afterwards.
func (a *App) ReplaceAllProject(query, replacement string, caseSensitive, wholeWord bool) (int, error) {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return 0, fmt.Errorf("no workspace open")
	}
	if query == "" {
		return 0, fmt.Errorf("nothing to search for")
	}
	// A bulk replace can't be undone in one step; snapshot first so it can.
	_, _, _ = history.Create(ws.Path, "Before replace-all", true, time.Now())
	total := 0
	for _, book := range ws.Books {
		for _, ch := range book.Chapters {
			text, err := workspace.ReadChapter(ws.Path, book.ID, ch)
			if err != nil {
				continue
			}
			out, n, err := workspace.ReplaceAllText(text, query, replacement, caseSensitive, wholeWord)
			if err != nil {
				return total, err
			}
			if n == 0 {
				continue
			}
			if err := workspace.WriteChapter(ws.Path, book.ID, ch, out); err != nil {
				return total, err
			}
			total += n
		}
	}
	return total, nil
}

// CreateSnapshot captures the workspace's current text as a revision and
// returns the refreshed snapshot list. If nothing changed since the last
// snapshot, no duplicate is created.
func (a *App) CreateSnapshot(label string) ([]history.Snapshot, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if _, _, err := history.Create(path, label, false, time.Now()); err != nil {
		return nil, err
	}
	return history.List(path)
}

// ListSnapshots returns all revisions, newest first.
func (a *App) ListSnapshots() ([]history.Snapshot, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	return history.List(path)
}

// SnapshotChanges lists how the workspace differs from a snapshot.
func (a *App) SnapshotChanges(id string) ([]history.FileChange, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	return history.Changes(path, id)
}

// SnapshotFileDiff returns a line diff of one file between a snapshot and now.
func (a *App) SnapshotFileDiff(id, rel string) (*history.DiffResult, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	res, err := history.FileDiff(path, id, rel)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// RestoreSnapshotFile reverts a single file to its snapshot version and returns
// the refreshed workspace.
func (a *App) RestoreSnapshotFile(id, rel string) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := history.RestoreFile(path, id, rel); err != nil {
		return nil, err
	}
	return a.reload()
}

// RestoreSnapshot rolls the whole workspace back to a snapshot. It takes a
// safety snapshot of the current state first, so the rollback is itself
// reversible, then returns the refreshed workspace.
func (a *App) RestoreSnapshot(id string) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	_, _, _ = history.Create(path, "Before restore", true, time.Now())
	if _, err := history.Restore(path, id); err != nil {
		return nil, err
	}
	return a.reload()
}

// DeleteSnapshot removes a revision and returns the refreshed list.
func (a *App) DeleteSnapshot(id string) ([]history.Snapshot, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	if err := history.Delete(path, id); err != nil {
		return nil, err
	}
	return history.List(path)
}

// --- Optional sync ---
//
// All sync is opt-in: with no server configured the app is fully offline and
// none of this runs. Credentials live in the app settings; per-workspace link
// state lives in the workspace's .novelide/sync.json.

// SyncStatus reports the app's sync configuration for the UI.
type SyncStatus struct {
	Configured bool   `json:"configured"` // a server URL is set
	LoggedIn   bool   `json:"loggedIn"`   // a token is held
	Server     string `json:"server"`
	Username   string `json:"username"`
	Linked     bool   `json:"linked"`   // the open workspace is linked to a remote
	RemoteID   string `json:"remoteId"` // that remote's id
}

// SyncOutcome is returned by operations that may change local files, carrying
// both what happened and the refreshed workspace to re-render.
type SyncOutcome struct {
	Result    syncclient.Result `json:"result"`
	Workspace *model.Workspace  `json:"workspace"`
}

// syncClient builds an authenticated client and returns the open workspace path
// and the signed-in account id (used to scope per-workspace sync state).
func (a *App) syncClient() (client *syncclient.Client, wsPath, account string, err error) {
	a.mu.RLock()
	s, ws := a.settings, a.ws
	a.mu.RUnlock()
	if s.SyncServer == "" {
		return nil, "", "", fmt.Errorf("sync is not configured")
	}
	if s.SyncToken == "" {
		return nil, "", "", fmt.Errorf("not logged in to the sync server")
	}
	if ws != nil {
		wsPath = ws.Path
	}
	return syncclient.New(s.SyncServer, s.SyncToken), wsPath, s.SyncAccountID, nil
}

func (a *App) saveSyncSettings(server, username, token, accountID string) error {
	a.mu.Lock()
	a.settings.SyncServer = server
	a.settings.SyncUsername = username
	a.settings.SyncToken = token
	a.settings.SyncAccountID = accountID
	s := a.settings
	a.mu.Unlock()
	return settings.Save(s)
}

// SyncStatus returns the current sync state.
func (a *App) SyncStatus() SyncStatus {
	a.mu.RLock()
	s, ws := a.settings, a.ws
	a.mu.RUnlock()
	st := SyncStatus{
		Configured: s.SyncServer != "",
		LoggedIn:   s.SyncServer != "" && s.SyncToken != "",
		Server:     s.SyncServer,
		Username:   s.SyncUsername,
	}
	if ws != nil {
		if rid := syncclient.LinkedRemoteID(ws.Path); rid != "" {
			st.Linked = true
			st.RemoteID = rid
		}
	}
	return st
}

func normalizeServer(server string) string {
	return strings.TrimRight(strings.TrimSpace(server), "/")
}

// SyncRegister creates an account on the given server and logs in.
func (a *App) SyncRegister(server, username, password string) (SyncStatus, error) {
	server = normalizeServer(server)
	if server == "" {
		return SyncStatus{}, fmt.Errorf("a server URL is required")
	}
	c := syncclient.New(server, "")
	auth, err := c.Register(username, password)
	if err != nil {
		return SyncStatus{}, err
	}
	if err := a.saveSyncSettings(server, auth.Username, auth.Token, auth.AccountID); err != nil {
		return SyncStatus{}, err
	}
	return a.SyncStatus(), nil
}

// SyncLogin authenticates against the given server.
func (a *App) SyncLogin(server, username, password string) (SyncStatus, error) {
	server = normalizeServer(server)
	if server == "" {
		return SyncStatus{}, fmt.Errorf("a server URL is required")
	}
	c := syncclient.New(server, "")
	auth, err := c.Login(username, password)
	if err != nil {
		return SyncStatus{}, err
	}
	if err := a.saveSyncSettings(server, auth.Username, auth.Token, auth.AccountID); err != nil {
		return SyncStatus{}, err
	}
	return a.SyncStatus(), nil
}

// SyncLogout clears the stored token (keeping the server URL for convenience).
func (a *App) SyncLogout() (SyncStatus, error) {
	a.mu.RLock()
	server := a.settings.SyncServer
	a.mu.RUnlock()
	if err := a.saveSyncSettings(server, "", "", ""); err != nil {
		return SyncStatus{}, err
	}
	return a.SyncStatus(), nil
}

// SyncAuthConfig reports which sign-in methods a server offers, so the UI can
// show a password form, an SSO button, or both.
func (a *App) SyncAuthConfig(server string) (syncproto.AuthConfig, error) {
	server = normalizeServer(server)
	if server == "" {
		return syncproto.AuthConfig{}, fmt.Errorf("a server URL is required")
	}
	return syncclient.New(server, "").AuthConfig()
}

// SyncLoginSSO signs in via the server's OIDC provider. It opens the system
// browser at the server's SSO endpoint and listens on a loopback port for the
// server to hand back a session token once sign-in completes.
func (a *App) SyncLoginSSO(server string) (SyncStatus, error) {
	server = normalizeServer(server)
	if server == "" {
		return SyncStatus{}, fmt.Errorf("a server URL is required")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return SyncStatus{}, fmt.Errorf("could not open a local callback port: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	appState := randomToken()

	type result struct {
		token string
		err   error
	}
	done := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if q.Get("state") != appState || q.Get("token") == "" {
			fmt.Fprint(w, ssoBrowserPage("Sign-in failed. You can close this window."))
			select {
			case done <- result{err: fmt.Errorf("sign-in did not complete")}:
			default:
			}
			return
		}
		fmt.Fprint(w, ssoBrowserPage("Signed in. You can close this window and return to NovelIDE."))
		select {
		case done <- result{token: q.Get("token")}:
		default:
		}
	})
	httpSrv := &http.Server{Handler: mux}
	go httpSrv.Serve(ln)
	defer httpSrv.Close()

	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	startURL := server + "/api/sso/start?app_redirect=" + url.QueryEscape(redirect) + "&app_state=" + url.QueryEscape(appState)
	runtime.BrowserOpenURL(a.ctx, startURL)

	var token string
	select {
	case res := <-done:
		if res.err != nil {
			return SyncStatus{}, res.err
		}
		token = res.token
	case <-time.After(5 * time.Minute):
		return SyncStatus{}, fmt.Errorf("timed out waiting for browser sign-in")
	case <-a.ctx.Done():
		return SyncStatus{}, fmt.Errorf("cancelled")
	}

	// Learn the account the server provisioned for this identity.
	me, err := syncclient.New(server, token).Me()
	if err != nil {
		return SyncStatus{}, err
	}
	if err := a.saveSyncSettings(server, me.Username, token, me.AccountID); err != nil {
		return SyncStatus{}, err
	}
	return a.SyncStatus(), nil
}

func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func ssoBrowserPage(msg string) string {
	return "<!doctype html><meta charset=utf-8><title>NovelIDE sign-in</title>" +
		"<body style=\"font-family:system-ui;padding:3rem;text-align:center\">" +
		"<h2>NovelIDE</h2><p>" + msg + "</p></body>"
}

// RemoteWorkspaces lists the account's workspaces on the server.
func (a *App) RemoteWorkspaces() ([]syncproto.WorkspaceMeta, error) {
	c, _, _, err := a.syncClient()
	if err != nil {
		return nil, err
	}
	return c.Workspaces()
}

// SyncNow syncs the open workspace, auto-linking it (by its folder name) on the
// first run, then reloads it so any pulled changes appear.
func (a *App) SyncNow() (*SyncOutcome, error) {
	c, wsPath, account, err := a.syncClient()
	if err != nil {
		return nil, err
	}
	if wsPath == "" {
		return nil, fmt.Errorf("no workspace open")
	}
	if syncclient.LinkedRemoteID(wsPath) == "" {
		if err := syncclient.Link(wsPath, syncclient.DeriveRemoteID(wsPath), c.BaseURL, account); err != nil {
			return nil, err
		}
	}
	res, err := syncclient.Sync(wsPath, c, account)
	if err != nil {
		return nil, err
	}
	return a.syncOutcome(res)
}

// SyncLinkPull links the open workspace to an existing remote and pulls it —
// used to join a workspace already synced from another device.
func (a *App) SyncLinkPull(remoteID string) (*SyncOutcome, error) {
	c, wsPath, account, err := a.syncClient()
	if err != nil {
		return nil, err
	}
	if wsPath == "" {
		return nil, fmt.Errorf("no workspace open")
	}
	res, err := syncclient.LinkPull(wsPath, remoteID, c, account)
	if err != nil {
		return nil, err
	}
	return a.syncOutcome(res)
}

// syncOutcome reloads the workspace from disk (sync may have changed files) and
// packages the result for the frontend.
func (a *App) syncOutcome(res syncclient.Result) (*SyncOutcome, error) {
	ws, err := a.reload()
	if err != nil {
		return nil, err
	}
	return &SyncOutcome{Result: res, Workspace: ws}, nil
}

// Backlink is one chapter that mentions a codex entity.
type Backlink struct {
	BookID       string `json:"bookId"`
	BookTitle    string `json:"bookTitle"`
	Chapter      string `json:"chapter"`
	ChapterTitle string `json:"chapterTitle"`
	Count        int    `json:"count"`
	Snippet      string `json:"snippet"` // context around the first mention
}

// Backlinks scans every chapter of every book and reports where the given
// codex entity (by name/alias) is mentioned — the manuscript half of an
// entry's backlinks. Codex-to-codex references come from the relation graph
// on the frontend; this surfaces the prose appearances the graph can't.
func (a *App) Backlinks(entryID string) ([]Backlink, error) {
	a.mu.RLock()
	ws, matcher := a.ws, a.matcher
	a.mu.RUnlock()
	if ws == nil {
		return nil, fmt.Errorf("no workspace open")
	}
	var out []Backlink
	for _, book := range ws.Books {
		for _, ch := range book.Chapters {
			text, err := workspace.ReadChapter(ws.Path, book.ID, ch)
			if err != nil {
				continue
			}
			count := 0
			first := -1
			for _, sp := range matcher.Scan(text) {
				if sp.EntryID != entryID {
					continue
				}
				count++
				if first < 0 {
					first = sp.Start
				}
			}
			if count == 0 {
				continue
			}
			out = append(out, Backlink{
				BookID:       book.ID,
				BookTitle:    book.Title,
				Chapter:      ch,
				ChapterTitle: workspace.ChapterTitle(text, ch),
				Count:        count,
				Snippet:      mentionSnippet(text, first),
			})
		}
	}
	return out, nil
}

// mentionSnippet returns a short window of prose around a rune offset, with an
// ellipsis on either side when it's clipped from a longer chapter.
func mentionSnippet(text string, runeStart int) string {
	if runeStart < 0 {
		return ""
	}
	runes := []rune(text)
	const pad = 60
	from := runeStart - pad
	if from < 0 {
		from = 0
	}
	to := runeStart + pad
	if to > len(runes) {
		to = len(runes)
	}
	s := strings.TrimSpace(strings.Join(strings.Fields(string(runes[from:to])), " "))
	if from > 0 {
		s = "… " + s
	}
	if to < len(runes) {
		s = s + " …"
	}
	return s
}

// DeepScan runs the optional Cybertron transformer pass over a chapter and
// returns new-entity suggestions the fast NER may have missed. The first
// call downloads the model (hundreds of MB) into the configured models
// directory, so this is user-triggered only.
func (a *App) DeepScan(bookID, chapter, text string) ([]detect.Suggestion, error) {
	a.mu.RLock()
	ws := a.ws
	cfg := a.settings
	a.mu.RUnlock()
	if ws == nil {
		return nil, fmt.Errorf("no workspace open")
	}
	if !cfg.DeepEnabled {
		return nil, fmt.Errorf("deep NLP is disabled — enable it in Settings")
	}
	ents, err := a.deep.FindEntities(cfg.ModelsDir, cfg.DeepModel, text)
	if err != nil {
		return nil, err
	}
	var out []detect.Suggestion
	for _, e := range deep.SuggestEntities(ws, ents) {
		out = append(out, detect.Suggestion{
			Kind: "entity", Name: e.Text,
			Start: e.Start, End: e.End,
			Message: fmt.Sprintf("Deep scan: %q (%s, %.0f%% confidence) has no Codex entry. Create one?", e.Text, e.Label, e.Score*100),
			Key:     "entity|" + e.Text,
		})
	}
	if out == nil {
		out = []detect.Suggestion{}
	}
	return out, nil
}
