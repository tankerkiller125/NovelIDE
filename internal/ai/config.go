package ai

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// NamedProvider is a reusable, named connection the user configures once and
// references from either mode. The model is chosen per mode, so the same
// connection can drive different models.
type NamedProvider struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Kind    ProviderKind `json:"kind"`
	BaseURL string       `json:"baseUrl"`
	APIKey  string       `json:"apiKey"`
	// NoStream disables SSE streaming for this endpoint (send stream:false, get
	// the whole reply at once). Turn it on for gateways whose streaming is broken
	// — e.g. Cloudflare's OpenAI-compat endpoint serving Claude.
	NoStream bool `json:"noStream"`
}

// ModeConfig is one mode's model selection and budgeting. ContextTokens is the
// model's context window and MaxOutputTokens the reserve for the reply — both
// user-set because there's no reliable cross-provider way to discover them.
type ModeConfig struct {
	ProviderID      string  `json:"providerId"`
	Model           string  `json:"model"`
	ContextTokens   int     `json:"contextTokens"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
	Temperature     float64 `json:"temperature"`
}

// Config is the whole optional-AI configuration, persisted in settings.
type Config struct {
	Enabled   bool            `json:"enabled"`
	Providers []NamedProvider `json:"providers"`
	Assistant ModeConfig      `json:"assistant"`
	Planning  ModeConfig      `json:"planning"`
}

// Defaults for budgeting when a mode leaves them zero.
const (
	DefaultContextTokens   = 8192
	DefaultMaxOutputTokens = 2048
)

// ContextTokens returns the configured window or a conservative default.
func (m ModeConfig) ContextWindow() int {
	if m.ContextTokens > 0 {
		return m.ContextTokens
	}
	return DefaultContextTokens
}

// OutputReserve returns the tokens reserved for the model's reply.
func (m ModeConfig) OutputReserve() int {
	if m.MaxOutputTokens > 0 {
		return m.MaxOutputTokens
	}
	return DefaultMaxOutputTokens
}

func (c Config) provider(id string) (NamedProvider, bool) {
	for _, p := range c.Providers {
		if p.ID == id {
			return p, true
		}
	}
	return NamedProvider{}, false
}

// Resolve turns a mode into a runtime Provider (connection + chosen model).
func (c Config) Resolve(m ModeConfig) (Provider, error) {
	p, ok := c.provider(m.ProviderID)
	if !ok {
		return Provider{}, fmt.Errorf("no provider configured for this mode")
	}
	if strings.TrimSpace(m.Model) == "" {
		return Provider{}, fmt.Errorf("no model set for this mode")
	}
	if strings.TrimSpace(p.BaseURL) == "" {
		return Provider{}, fmt.Errorf("provider %q has no base URL", p.Name)
	}
	return Provider{Kind: p.Kind, BaseURL: p.BaseURL, APIKey: p.APIKey, Model: m.Model, NoStream: p.NoStream}, nil
}

// Normalize fills in missing provider IDs, trims URLs, and defaults kinds, so
// the stored config is well-formed.
func Normalize(c Config) Config {
	seen := map[string]bool{}
	for i := range c.Providers {
		p := &c.Providers[i]
		p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
		p.Name = strings.TrimSpace(p.Name)
		if p.Kind != KindOpenAI && p.Kind != KindAnthropic {
			p.Kind = KindOpenAI
		}
		if p.ID == "" || seen[p.ID] {
			p.ID = uuid.NewString()
		}
		seen[p.ID] = true
	}
	c.Assistant.Model = strings.TrimSpace(c.Assistant.Model)
	c.Planning.Model = strings.TrimSpace(c.Planning.Model)
	return c
}
