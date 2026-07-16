package ai

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// NamedProvider is a reusable, named connection the user configures once. The
// model to run is chosen per chat turn from the provider's Models list.
type NamedProvider struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Kind    ProviderKind `json:"kind"`
	BaseURL string       `json:"baseUrl"`
	APIKey  string       `json:"apiKey"`
	// Models is the list of model ids the user makes available for this provider,
	// shown in the chat model picker. Ignored for acp/a2a (no model to choose).
	Models []string `json:"models,omitempty"`
}

// Config is the whole optional-AI configuration, persisted in settings. There is
// no fixed per-mode model any more: the user configures providers + their models
// and picks which model to run for each chat turn.
type Config struct {
	Enabled   bool            `json:"enabled"`
	Providers []NamedProvider `json:"providers"`
}

// DefaultContextTokens sizes how much Codex/chapter context is injected into a
// turn (there's no reliable cross-provider way to discover a model's window).
const DefaultContextTokens = 8192

func (c Config) provider(id string) (NamedProvider, bool) {
	for _, p := range c.Providers {
		if p.ID == id {
			return p, true
		}
	}
	return NamedProvider{}, false
}

// ResolveModel turns a provider id + chosen model into a runtime Provider,
// validating per the provider kind. ACP and A2A providers take no model (an
// agent id / card URL lives in BaseURL); Gemini's base URL is optional;
// OpenAI/Anthropic need both a model and a base URL.
func (c Config) ResolveModel(providerID, model string) (Provider, error) {
	p, ok := c.provider(providerID)
	if !ok {
		return Provider{}, fmt.Errorf("pick a model to chat with (none is configured)")
	}
	switch p.Kind {
	case KindACP:
		if strings.TrimSpace(p.BaseURL) == "" {
			return Provider{}, fmt.Errorf("choose a local agent for provider %q", p.Name)
		}
		return Provider{Kind: p.Kind, BaseURL: p.BaseURL}, nil
	case KindA2A:
		if strings.TrimSpace(p.BaseURL) == "" {
			return Provider{}, fmt.Errorf("provider %q needs the agent card URL", p.Name)
		}
		return Provider{Kind: p.Kind, BaseURL: p.BaseURL}, nil
	case KindGemini:
		if strings.TrimSpace(model) == "" {
			return Provider{}, fmt.Errorf("pick a model for %q", p.Name)
		}
		return Provider{Kind: p.Kind, BaseURL: p.BaseURL, APIKey: p.APIKey, Model: model}, nil
	default: // openai, anthropic
		if strings.TrimSpace(model) == "" {
			return Provider{}, fmt.Errorf("pick a model for %q", p.Name)
		}
		if strings.TrimSpace(p.BaseURL) == "" {
			return Provider{}, fmt.Errorf("provider %q has no base URL", p.Name)
		}
		return Provider{Kind: p.Kind, BaseURL: p.BaseURL, APIKey: p.APIKey, Model: model}, nil
	}
}

// Normalize fills in missing provider IDs, trims URLs, and defaults kinds, so
// the stored config is well-formed.
func Normalize(c Config) Config {
	seen := map[string]bool{}
	for i := range c.Providers {
		p := &c.Providers[i]
		p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
		p.Name = strings.TrimSpace(p.Name)
		if !ValidKind(p.Kind) {
			p.Kind = KindOpenAI
		}
		if p.ID == "" || seen[p.ID] {
			p.ID = uuid.NewString()
		}
		seen[p.ID] = true
	}
	return c
}
