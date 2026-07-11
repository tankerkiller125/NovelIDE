package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"novelide/internal/deep"
	"novelide/internal/detect"
	"novelide/internal/match"
	"novelide/internal/model"
	"novelide/internal/nlp"
	"novelide/internal/settings"
	"novelide/internal/spell"
	"novelide/internal/spellcheck"
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
	return workspace.WriteChapter(path, bookID, chapter, content)
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
