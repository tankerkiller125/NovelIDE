// Package settings persists app-level (not per-workspace) preferences to
// the user's config directory.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"novelide/internal/ai"
)

// Settings are the user's application preferences.
type Settings struct {
	// DeepEnabled turns on the optional Cybertron (transformer) NLP tier.
	DeepEnabled bool `json:"deepEnabled"`
	// DeepModel is the Hugging Face model used for deep NER,
	// e.g. "dbmdz/bert-large-cased-finetuned-conll03-english".
	DeepModel string `json:"deepModel"`
	// ModelsDir is where downloaded models are cached.
	ModelsDir string `json:"modelsDir"`
	// EditorWidth is the manuscript editor's text column width in
	// characters; 0 means use the full pane width.
	EditorWidth int `json:"editorWidth"`
	// EditorFont is "serif", "sans", "mono", or a custom font-family name.
	EditorFont string `json:"editorFont"`
	// EditorFontSize is the editor font size in px.
	EditorFontSize int `json:"editorFontSize"`
	// EditorLineHeight is the editor line spacing (e.g. 1.7).
	EditorLineHeight float64 `json:"editorLineHeight"`
	// EditorLineNumbers shows the line-number gutter (off by default —
	// this is prose, not code).
	EditorLineNumbers bool `json:"editorLineNumbers"`
	// EditorSpellcheck enables the platform spellchecker in the editor.
	EditorSpellcheck bool `json:"editorSpellcheck"`
	// EditorRawMarkup disables live Markdown rendering, showing the raw
	// `**`, `_`, `#`, and `` ` `` markers at all times. Off by default (zero
	// value) so live preview is on out of the box.
	EditorRawMarkup bool `json:"editorRawMarkup"`
	// SpellcheckLang is the dictionary language, e.g. "en_US".
	SpellcheckLang string `json:"spellcheckLang"`

	// Optional sync server. Empty SyncServer means sync is off (the default).
	// SyncToken is the bearer token from the last login; SyncUsername is shown
	// in the UI. These are only set when the user opts into sync.
	SyncServer    string `json:"syncServer"`
	SyncUsername  string `json:"syncUsername"`
	SyncToken     string `json:"syncToken"`
	SyncAccountID string `json:"syncAccountId"`

	// AI holds the optional AI configuration (providers + per-mode models).
	// Empty/disabled by default. API keys live here in plaintext, like the
	// sync token — same local-config trust model.
	AI ai.Config `json:"ai"`

	// Recent lists recently opened workspace paths, most recent first.
	Recent []string `json:"recent"`
}

// DefaultDeepModel is a solid general-purpose English NER model.
const DefaultDeepModel = "dbmdz/bert-large-cased-finetuned-conll03-english"

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "novelide", "settings.json"), nil
}

// Defaults returns settings with sensible defaults filled in.
func Defaults() Settings {
	models := ""
	if cache, err := os.UserCacheDir(); err == nil {
		models = filepath.Join(cache, "novelide", "models")
	}
	return Settings{
		DeepModel:        DefaultDeepModel,
		ModelsDir:        models,
		EditorFont:       "serif",
		EditorFontSize:   15,
		EditorLineHeight: 1.7,
		EditorSpellcheck: true,
		SpellcheckLang:   localeLang(),
		Recent:           []string{},
	}
}

// localeLang derives a dictionary language like "en_US" from the
// environment, falling back to en_US.
func localeLang() string {
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := os.Getenv(env)
		if v == "" || v == "C" || v == "POSIX" {
			continue
		}
		if i := strings.IndexAny(v, ".@"); i > 0 {
			v = v[:i]
		}
		if v != "" {
			return v
		}
	}
	return "en_US"
}

// Sanitize clamps user-supplied values to sane ranges and fills defaults.
func Sanitize(s Settings) Settings {
	if s.DeepModel == "" {
		s.DeepModel = DefaultDeepModel
	}
	if s.EditorFont == "" {
		s.EditorFont = "serif"
	}
	if s.EditorFontSize < 10 || s.EditorFontSize > 32 {
		s.EditorFontSize = 15
	}
	if s.EditorLineHeight < 1.0 || s.EditorLineHeight > 3.0 {
		s.EditorLineHeight = 1.7
	}
	if s.EditorWidth != 0 && (s.EditorWidth < 40 || s.EditorWidth > 160) {
		s.EditorWidth = 80
	}
	if s.SpellcheckLang == "" {
		s.SpellcheckLang = localeLang()
	}
	if s.Recent == nil {
		s.Recent = []string{}
	}
	return s
}

// Load reads settings from disk, returning defaults if none exist yet.
func Load() Settings {
	s := Defaults()
	p, err := configPath()
	if err != nil {
		return s
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s)
	return Sanitize(s)
}

// Save writes settings to disk.
func Save(s Settings) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// Touch records a workspace path as most-recently-used and persists.
func Touch(s *Settings, path string) {
	out := []string{path}
	for _, r := range s.Recent {
		if r != path && len(out) < 8 {
			out = append(out, r)
		}
	}
	s.Recent = out
	_ = Save(*s)
}
