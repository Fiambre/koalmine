package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"koalmine/internal/providers"
)

const defaultPollIntervalMinutes = 5

// Integration is one configured, named connection to a provider type (e.g.
// two separate Integration values can both have Type "gitlab", pointing at
// different GitLab servers). Secret fields (API keys/tokens) live in the OS
// keychain instead, keyed by ID — see secrets.go.
type Integration struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Name    string            `json:"name"`
	Enabled bool              `json:"enabled"`
	Values  map[string]string `json:"values"`
}

// Panel is a user-defined, filtered view of tasks pinned to the sidebar
// (e.g. "only open issues in project X"). Each field is a filter dimension;
// an empty value means "no restriction" on that dimension.
type Panel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	IntegrationID string `json:"integrationId,omitempty"`
	Project       string `json:"project,omitempty"`
	Type          string `json:"type,omitempty"`
	// Status is "open", "closed", or "" (both).
	Status string `json:"status,omitempty"`
}

// Config is Koalmine's persisted, non-sensitive settings.
type Config struct {
	Integrations        []Integration `json:"integrations"`
	PollIntervalMinutes int           `json:"pollIntervalMinutes"`
	Panels              []Panel       `json:"panels"`
}

// IntegrationByID returns the integration with the given ID, if any. A
// not-found ID (including "") returns the zero value with ok=false — every
// caller treats that as "resolve to blank config" rather than an error, so
// e.g. testing a not-yet-created integration's connection works the same
// way as testing an existing one.
func (c Config) IntegrationByID(id string) (Integration, bool) {
	for _, integ := range c.Integrations {
		if integ.ID == id {
			return integ, true
		}
	}
	return Integration{}, false
}

// SameTypeCount returns how many configured integrations share providerType
// — used to decide whether TaskItem IDs for that type need to be
// disambiguated (see store.NamespaceItem).
func (c Config) SameTypeCount(providerType string) int {
	count := 0
	for _, integ := range c.Integrations {
		if integ.Type == providerType {
			count++
		}
	}
	return count
}

// NewIntegrationID generates a fresh, unique ID for a new integration of
// the given provider type — e.g. "gitlab-a1b2c3d4". The type prefix is
// just for readability in config.json/logs, not parsed anywhere. Migrated
// integrations (see migrateLegacyConfig) instead reuse the bare provider
// type name as their ID, so newly generated IDs always include the random
// suffix to guarantee they never collide with one of those.
func NewIntegrationID(providerType string) string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf) // crypto/rand.Read never fails on supported platforms
	return providerType + "-" + hex.EncodeToString(buf)
}

// NewPanelID generates a fresh, unique ID for a new panel, e.g. "panel-a1b2c3d4".
func NewPanelID() string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf) // crypto/rand.Read never fails on supported platforms
	return "panel-" + hex.EncodeToString(buf)
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
		Integrations:        []Integration{},
		PollIntervalMinutes: defaultPollIntervalMinutes,
		Panels:              []Panel{},
	}
}

// Load reads the persisted config, returning sensible defaults if none
// exists yet. A config.json left over from before multi-integration
// support (one slot per provider type, keyed by name) is transparently
// migrated into today's shape and immediately re-saved — see
// migrateLegacyConfig.
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
	if cfg.Integrations == nil {
		cfg.Integrations = []Integration{}
	}
	if cfg.Panels == nil {
		cfg.Panels = []Panel{}
	}
	if cfg.PollIntervalMinutes <= 0 {
		cfg.PollIntervalMinutes = defaultPollIntervalMinutes
	}

	if len(cfg.Integrations) == 0 {
		if migrated, ok := migrateLegacyConfig(data, cfg.PollIntervalMinutes); ok {
			cfg = migrated
			if err := Save(cfg); err != nil {
				return Config{}, fmt.Errorf("no se pudo guardar la configuración migrada: %w", err)
			}
		}
	}
	return cfg, nil
}

// migrateLegacyConfig upgrades a pre-multi-integration config.json (one
// slot per provider *type*, keyed by its registered name) into today's
// shape (a list of named, independently-configurable integrations). Each
// migrated integration reuses its old provider-type name as its own ID —
// e.g. a pre-existing "gitlab" entry becomes an integration with ID
// "gitlab" — so its already-stored OS-keychain secrets (keyed by
// "gitlab:<field>") keep resolving with no separate secrets migration
// needed. New integrations created after this point always get a
// randomly-suffixed ID (see NewIntegrationID), so they never collide with
// a migrated one. ok is false when data isn't legacy-shaped (a fresh
// install, or a new-shape file that legitimately has zero integrations).
func migrateLegacyConfig(data []byte, pollInterval int) (cfg Config, ok bool) {
	var legacy struct {
		Providers map[string]struct {
			Enabled bool              `json:"enabled"`
			Values  map[string]string `json:"values"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil || len(legacy.Providers) == 0 {
		return Config{}, false
	}

	// Sort names for a deterministic migration order — map iteration order
	// isn't stable, and this only ever runs once per install so it's worth
	// keeping predictable for anyone diffing their config.json.
	names := make([]string, 0, len(legacy.Providers))
	for name := range legacy.Providers {
		names = append(names, name)
	}
	sort.Strings(names)

	integrations := make([]Integration, 0, len(names))
	for _, name := range names {
		pc := legacy.Providers[name]
		integrations = append(integrations, Integration{
			ID:      name,
			Type:    name,
			Name:    displayNameForMigration(name),
			Enabled: pc.Enabled,
			Values:  pc.Values,
		})
	}

	return Config{Integrations: integrations, PollIntervalMinutes: pollInterval}, true
}

func displayNameForMigration(providerType string) string {
	if p, ok := providers.Get(providerType); ok {
		return p.DisplayName()
	}
	return providerType
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
