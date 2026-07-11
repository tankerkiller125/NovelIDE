//go:build !linux

package spell

import "errors"

// Engine is unavailable on this platform (hunspell binding is Linux-only
// for now; Windows/macOS would bind their platform spellcheckers here).
type Engine struct {
	Err error
}

func NewEngine() *Engine {
	return &Engine{Err: errors.New("spellcheck engine not available on this platform")}
}

func (e *Engine) Load(lang string) error       { return e.Err }
func (e *Engine) Ready() bool                  { return false }
func (e *Engine) Check(word string) bool       { return true }
func (e *Engine) Suggest(word string) []string { return nil }
func (e *Engine) AddPersonal(word string) error {
	return e.Err
}
