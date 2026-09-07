// Package store persists Koalmine's local state: non-sensitive settings in
// a JSON file, and provider secrets (API keys/tokens) in the OS keychain
// (Windows Credential Manager / macOS Keychain / Linux Secret Service via
// go-keyring) so they're never written to disk in plain text.
package store

import "github.com/zalando/go-keyring"

const keyringService = "Koalmine"

// SetSecret stores a provider's secret field (e.g. an API key) in the OS keychain.
func SetSecret(providerName, fieldKey, value string) error {
	return keyring.Set(keyringService, secretName(providerName, fieldKey), value)
}

// GetSecret returns a provider's stored secret, or "" if none has been set.
func GetSecret(providerName, fieldKey string) (string, error) {
	value, err := keyring.Get(keyringService, secretName(providerName, fieldKey))
	if err == keyring.ErrNotFound {
		return "", nil
	}
	return value, err
}

// DeleteSecret removes a provider's stored secret, if any.
func DeleteSecret(providerName, fieldKey string) error {
	err := keyring.Delete(keyringService, secretName(providerName, fieldKey))
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}

func secretName(providerName, fieldKey string) string {
	return providerName + ":" + fieldKey
}
