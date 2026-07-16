package ai

import "testing"

func TestConfigNormalize(t *testing.T) {
	cfg := Config{
		Providers: []NamedProvider{
			{Name: "OpenAI", Kind: KindOpenAI, BaseURL: "https://api.openai.com/v1/"},
			{Name: "Local", Kind: "weird", BaseURL: "http://localhost:11434/v1"},
		},
	}
	cfg = Normalize(cfg)

	// IDs filled, URL trimmed, bad kind defaulted.
	if cfg.Providers[0].ID == "" || cfg.Providers[1].ID == "" {
		t.Fatal("provider IDs not generated")
	}
	if cfg.Providers[0].BaseURL != "https://api.openai.com/v1" {
		t.Errorf("URL not trimmed: %q", cfg.Providers[0].BaseURL)
	}
	if cfg.Providers[1].Kind != KindOpenAI {
		t.Errorf("bad kind not defaulted: %q", cfg.Providers[1].Kind)
	}
}

func TestResolveModelByKind(t *testing.T) {
	cfg := Config{Providers: []NamedProvider{
		{ID: "oa", Name: "OpenAI", Kind: KindOpenAI, BaseURL: "https://api.openai.com/v1"},
		{ID: "cc", Name: "Claude Code", Kind: KindACP, BaseURL: "claude-code"},
	}}

	// OpenAI wires the chosen model onto the provider.
	p, err := cfg.ResolveModel("oa", "gpt-4o-mini")
	if err != nil || p.Kind != KindOpenAI || p.Model != "gpt-4o-mini" || p.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("openai resolve: %+v err=%v", p, err)
	}

	// OpenAI needs a model; ACP resolves with none (agent id lives in BaseURL).
	if _, err := cfg.ResolveModel("oa", ""); err == nil {
		t.Error("openai without a model should error")
	}
	p, err = cfg.ResolveModel("cc", "")
	if err != nil || p.Kind != KindACP || p.BaseURL != "claude-code" {
		t.Errorf("acp resolve: %+v err=%v", p, err)
	}

	// Unknown provider errors.
	if _, err := cfg.ResolveModel("nope", "x"); err == nil {
		t.Error("unknown provider should error")
	}
}
