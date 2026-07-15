package ai

import "testing"

func TestConfigNormalizeAndResolve(t *testing.T) {
	cfg := Config{
		Providers: []NamedProvider{
			{Name: "OpenAI", Kind: KindOpenAI, BaseURL: "https://api.openai.com/v1/"},
			{Name: "Local", Kind: "weird", BaseURL: "http://localhost:11434/v1"},
		},
		Assistant: ModeConfig{Model: " gpt-4o "},
	}
	cfg = Normalize(cfg)

	// IDs filled, URL trimmed, bad kind defaulted, model trimmed.
	if cfg.Providers[0].ID == "" || cfg.Providers[1].ID == "" {
		t.Fatal("provider IDs not generated")
	}
	if cfg.Providers[0].BaseURL != "https://api.openai.com/v1" {
		t.Errorf("URL not trimmed: %q", cfg.Providers[0].BaseURL)
	}
	if cfg.Providers[1].Kind != KindOpenAI {
		t.Errorf("bad kind not defaulted: %q", cfg.Providers[1].Kind)
	}
	if cfg.Assistant.Model != "gpt-4o" {
		t.Errorf("model not trimmed: %q", cfg.Assistant.Model)
	}

	// Resolve wires the chosen model onto the referenced provider.
	cfg.Assistant.ProviderID = cfg.Providers[0].ID
	p, err := cfg.Resolve(cfg.Assistant)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != KindOpenAI || p.Model != "gpt-4o" || p.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("resolved wrong: %+v", p)
	}

	// Missing provider / model error out.
	if _, err := cfg.Resolve(ModeConfig{ProviderID: "nope", Model: "x"}); err == nil {
		t.Error("unknown provider should error")
	}
	if _, err := cfg.Resolve(ModeConfig{ProviderID: cfg.Providers[0].ID}); err == nil {
		t.Error("missing model should error")
	}

	// Budgeting defaults.
	if cfg.Planning.ContextWindow() != DefaultContextTokens || cfg.Planning.OutputReserve() != DefaultMaxOutputTokens {
		t.Error("mode budgeting defaults wrong")
	}
}
