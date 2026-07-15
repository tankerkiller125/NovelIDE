// Package secrets stores sensitive values (API keys, the sync token) in the
// operating system's credential store — Keychain on macOS, Credential Manager
// on Windows, and the Secret Service (GNOME Keyring / KWallet) on Linux — so
// they never sit in plaintext in the settings file.
//
// Some minimal Linux setups have no running Secret Service; Available() reports
// whether secure storage works, and callers fall back to the settings file
// (still owner-only, 0600) when it doesn't.
package secrets

import (
	"errors"
	"sync"

	keyring "github.com/zalando/go-keyring"
)

// service is the credential-store service/collection name for all NovelIDE
// secrets. Individual secrets are keyed by an id (the "account"/"user").
const service = "NovelIDE"

// Set stores value under id, or deletes the entry when value is empty.
func Set(id, value string) error {
	if value == "" {
		return Delete(id)
	}
	return keyring.Set(service, id, value)
}

// Get returns the value for id (empty string if there is no such entry).
func Get(id string) (string, error) {
	v, err := keyring.Get(service, id)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return v, err
}

// Delete removes id's entry; a missing entry is not an error.
func Delete(id string) error {
	err := keyring.Delete(service, id)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

var (
	probeOnce sync.Once
	probeOK   bool
)

// Available reports whether a working secret store is present. It probes once
// (writing and removing a throwaway entry) and caches the result.
func Available() bool {
	probeOnce.Do(func() { probeOK = probe() })
	return probeOK
}

func probe() bool {
	const id = "__novelide_probe__"
	if keyring.Set(service, id, "1") != nil {
		return false
	}
	_, err := keyring.Get(service, id)
	_ = keyring.Delete(service, id)
	return err == nil
}

// ResetForTest clears the cached Available() probe. Test-only.
func ResetForTest() { probeOnce = sync.Once{} }
