package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"novelide/internal/ai"
	"novelide/internal/secrets"

	keyring "github.com/zalando/go-keyring"
)

// point configPath at a temp dir and use the in-memory keyring.
func setup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir) // os.UserConfigDir honors this on Linux
	keyring.MockInit()
	secrets.ResetForTest()
	return filepath.Join(dir, "novelide", "settings.json")
}

func readFile(t *testing.T, p string) Settings {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSecretsMovedToKeyring(t *testing.T) {
	p := setup(t)
	s := Defaults()
	s.SyncToken = "tok-123"
	s.AI.Providers = []ai.NamedProvider{{ID: "prov-a", Name: "A", Kind: ai.KindOpenAI, APIKey: "sk-secret"}}
	if err := Save(s); err != nil {
		t.Fatal(err)
	}

	// The on-disk file must NOT contain the plaintext secrets.
	raw, _ := os.ReadFile(p)
	if strings.Contains(string(raw), "sk-secret") || strings.Contains(string(raw), "tok-123") {
		t.Fatalf("secrets leaked into settings file:\n%s", raw)
	}
	onDisk := readFile(t, p)
	if onDisk.SyncToken != "" || onDisk.AI.Providers[0].APIKey != "" {
		t.Errorf("secrets not blanked in file: %+v", onDisk)
	}

	// The keyring holds them, and Load restores them.
	if v, _ := secrets.Get(providerSecretID("prov-a")); v != "sk-secret" {
		t.Errorf("provider key not in keyring: %q", v)
	}
	got := Load()
	if got.SyncToken != "tok-123" || got.AI.Providers[0].APIKey != "sk-secret" {
		t.Errorf("secrets not restored on load: %+v", got.AI.Providers[0])
	}

	// The file is owner-only.
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("settings file mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestMigratesExistingPlaintext(t *testing.T) {
	p := setup(t)
	// Simulate a pre-encryption file: plaintext key on disk, keyring empty.
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := Defaults()
	legacy.AI.Providers = []ai.NamedProvider{{ID: "prov-b", Name: "B", Kind: ai.KindOpenAI, APIKey: "sk-legacy"}}
	b, _ := json.MarshalIndent(legacy, "", "  ")
	os.WriteFile(p, b, 0o644)

	got := Load() // should migrate
	if got.AI.Providers[0].APIKey != "sk-legacy" {
		t.Errorf("legacy key lost: %+v", got.AI.Providers[0])
	}
	if v, _ := secrets.Get(providerSecretID("prov-b")); v != "sk-legacy" {
		t.Errorf("legacy key not migrated to keyring: %q", v)
	}
	// The file no longer holds the plaintext.
	raw, _ := os.ReadFile(p)
	if strings.Contains(string(raw), "sk-legacy") {
		t.Errorf("plaintext not scrubbed after migration:\n%s", raw)
	}
}

func TestRemovedProviderSecretDeleted(t *testing.T) {
	setup(t)
	s := Defaults()
	s.AI.Providers = []ai.NamedProvider{
		{ID: "keep", Name: "K", Kind: ai.KindOpenAI, APIKey: "sk-keep"},
		{ID: "drop", Name: "D", Kind: ai.KindOpenAI, APIKey: "sk-drop"},
	}
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	// Remove one provider and save again.
	s.AI.Providers = s.AI.Providers[:1]
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	if v, _ := secrets.Get(providerSecretID("drop")); v != "" {
		t.Errorf("removed provider's secret should be deleted, got %q", v)
	}
	if v, _ := secrets.Get(providerSecretID("keep")); v != "sk-keep" {
		t.Errorf("kept provider's secret missing: %q", v)
	}
}
