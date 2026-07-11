//go:build linux

// Package spell is NovelIDE's own spellchecker: hunspell bound at runtime
// via purego (no C headers or link-time deps — it dlopens the system
// libhunspell and reads the system dictionaries). We run spellcheck
// ourselves because webview-native spellchecking attaches markers to DOM
// text nodes that CodeMirror's model/DOM sync constantly rewrites, which
// makes native squiggles unreliable inside the editor.
package spell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var dictDirs = []string{
	"/app/share/hunspell", // flatpak: dictionaries bundled with the app
	"/usr/share/hunspell",
	"/usr/share/myspell",
	"/usr/local/share/hunspell",
}

var libNames = []string{
	"libhunspell-1.7.so.0",
	"libhunspell-1.7.so",
	"libhunspell-1.6.so.0",
	"libhunspell.so",
}

type hunspellAPI struct {
	create   func(aff, dic string) uintptr
	destroy  func(h uintptr)
	spell    func(h uintptr, word string) int32
	suggest  func(h uintptr, slst *unsafe.Pointer, word string) int32
	freeList func(h uintptr, slst *unsafe.Pointer, n int32)
}

// Engine wraps one loaded dictionary plus the user's personal word list.
type Engine struct {
	mu       sync.Mutex
	api      *hunspellAPI
	handle   uintptr
	lang     string
	personal map[string]bool
	// Err records why the engine is unavailable (lib or dictionary
	// missing) so the UI can say something useful.
	Err error
}

func NewEngine() *Engine {
	return &Engine{personal: map[string]bool{}}
}

func loadAPI() (*hunspellAPI, error) {
	var lib uintptr
	var err error
	for _, name := range libNames {
		lib, err = purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			break
		}
	}
	if lib == 0 {
		return nil, fmt.Errorf("libhunspell not found: %w", err)
	}
	api := &hunspellAPI{}
	purego.RegisterLibFunc(&api.create, lib, "Hunspell_create")
	purego.RegisterLibFunc(&api.destroy, lib, "Hunspell_destroy")
	purego.RegisterLibFunc(&api.spell, lib, "Hunspell_spell")
	purego.RegisterLibFunc(&api.suggest, lib, "Hunspell_suggest")
	purego.RegisterLibFunc(&api.freeList, lib, "Hunspell_free_list")
	return api, nil
}

func findDict(lang string) (aff, dic string, err error) {
	// Try the exact language, then lang with region variants normalized.
	candidates := []string{lang, strings.ReplaceAll(lang, "-", "_")}
	if i := strings.IndexAny(lang, "_-"); i > 0 {
		candidates = append(candidates, lang[:i])
	}
	for _, dir := range dictDirs {
		for _, c := range candidates {
			a := filepath.Join(dir, c+".aff")
			d := filepath.Join(dir, c+".dic")
			if fileExists(a) && fileExists(d) {
				return a, d, nil
			}
		}
		// Last resort: any dictionary matching the base language.
		base := lang
		if i := strings.IndexAny(base, "_-"); i > 0 {
			base = base[:i]
		}
		matches, _ := filepath.Glob(filepath.Join(dir, base+"_*.aff"))
		if len(matches) > 0 {
			a := matches[0]
			d := strings.TrimSuffix(a, ".aff") + ".dic"
			if fileExists(d) {
				return a, d, nil
			}
		}
	}
	return "", "", fmt.Errorf("no hunspell dictionary for %q (looked in %s)", lang, strings.Join(dictDirs, ", "))
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// Load (re)initializes the engine for a dictionary language like "en_US".
func (e *Engine) Load(lang string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle != 0 && e.lang == lang {
		return e.Err
	}
	if e.api == nil {
		api, err := loadAPI()
		if err != nil {
			e.Err = err
			return err
		}
		e.api = api
	}
	aff, dic, err := findDict(lang)
	if err != nil {
		e.Err = err
		return err
	}
	if e.handle != 0 {
		e.api.destroy(e.handle)
		e.handle = 0
	}
	h := e.api.create(aff, dic)
	if h == 0 {
		e.Err = fmt.Errorf("failed to load dictionary %s", dic)
		return e.Err
	}
	e.handle = h
	e.lang = lang
	e.Err = nil
	e.loadPersonal()
	return nil
}

// Ready reports whether the engine can check words.
func (e *Engine) Ready() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.handle != 0
}

// Check returns true when the word is spelled correctly (or in the
// personal dictionary). Unknown-engine state counts as correct — the
// spellchecker must fail open.
func (e *Engine) Check(word string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle == 0 {
		return true
	}
	if e.personal[strings.ToLower(word)] {
		return true
	}
	return e.api.spell(e.handle, word) != 0
}

// Suggest returns up to 8 spelling suggestions for a word.
func (e *Engine) Suggest(word string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle == 0 {
		return nil
	}
	var slst unsafe.Pointer
	n := e.api.suggest(e.handle, &slst, word)
	if n <= 0 || slst == nil {
		return nil
	}
	defer e.api.freeList(e.handle, &slst, n)
	if n > 8 {
		n = 8
	}
	out := make([]string, 0, n)
	ptrs := unsafe.Slice((**byte)(slst), n)
	for _, p := range ptrs {
		if p != nil {
			out = append(out, goString(p))
		}
	}
	return out
}

// goString copies a NUL-terminated C string.
func goString(p *byte) string {
	var b []byte
	for i := 0; ; i++ {
		c := *(*byte)(unsafe.Add(unsafe.Pointer(p), i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

// ---- personal dictionary ----

func personalPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "novelide", "dictionary.txt")
}

func (e *Engine) loadPersonal() {
	e.personal = map[string]bool{}
	p := personalPath()
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	for _, w := range strings.Fields(string(b)) {
		e.personal[strings.ToLower(w)] = true
	}
}

// AddPersonal appends a word to the user's dictionary file.
func (e *Engine) AddPersonal(word string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	w := strings.TrimSpace(word)
	if w == "" {
		return nil
	}
	e.personal[strings.ToLower(w)] = true
	p := personalPath()
	if p == "" {
		return fmt.Errorf("no config directory")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(w + "\n")
	return err
}
