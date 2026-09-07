package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const defaultPollIntervalMinutes = 5

// ProviderConfig holds one provider's non-secret settings: whether it's
// enabled, and its non-secret field values (e.g. Redmine's base_url).
// Secret fields (API keys/tokens) live in the OS keychain instead — see secrets.go.
type ProviderConfig struct {
	Enabled bool              `json:"enabled"`
	Values  map[string]string `json:"values"`
}

// Config is Koalmine's persisted, non-sensitive settings.
type Config struct {
	Providers           map[string]ProviderConfig `json:"providers"`
	PollIntervalMinutes int                       `json:"pollIntervalMinutes"`
}

// configDir is overridden in tests to avoid touching the real user config directory.
var configDir = defaultConfigDir

func defaultConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Koalmine"), nil
}

func configFilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func newConfig() Config {
	return Config{
		Providers:           map[string]ProviderConfig{},
		PollIntervalMinutes: defaultPollIntervalMinutes,
	}
}

// Load reads the persisted config, returning sensible defaults if none exists yet.
func Load() (Config, error) {
	path, err := configFilePath()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return newConfig(), nil
	}
	if err != nil {
		return Config{}, err
	}

	cfg := newConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	if cfg.PollIntervalMinutes <= 0 {
		cfg.PollIntervalMinutes = defaultPollIntervalMinutes
	}
	return cfg, nil
}

// Save persists cfg, creating the config directory if needed.
func Save(cfg Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	path, err := configFilePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
