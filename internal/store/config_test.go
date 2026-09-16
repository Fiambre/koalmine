package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "koalmine/internal/providers" // registers redmine/github/gitlab/todoist for DisplayName lookups
)

func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	original := configDir
	configDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { configDir = original })
	return dir
}

func TestLoadReturnsDefaultsWhenNoFileExists(t *testing.T) {
	withTempConfigDir(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PollIntervalMinutes != defaultPollIntervalMinutes {
		t.Errorf("expected default poll interval %d, got %d", defaultPollIntervalMinutes, cfg.PollIntervalMinutes)
	}
	if cfg.Integrations == nil {
		t.Error("expected Integrations to be a non-nil empty slice")
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	withTempConfigDir(t)

	cfg := Config{
		Integrations: []Integration{
			{ID: "redmine", Type: "redmine", Name: "Redmine", Enabled: true, Values: map[string]string{"base_url": "https://redmine.example.com"}},
		},
		PollIntervalMinutes: 10,
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.PollIntervalMinutes != 10 {
		t.Errorf("expected poll interval 10, got %d", loaded.PollIntervalMinutes)
	}
	integ, ok := loaded.IntegrationByID("redmine")
	if !ok || !integ.Enabled || integ.Values["base_url"] != "https://redmine.example.com" {
		t.Errorf("unexpected redmine integration after round-trip: %+v", integ)
	}
}

func TestSaveThenLoadRoundTripsPanels(t *testing.T) {
	withTempConfigDir(t)

	cfg := Config{
		Panels: []Panel{
			{ID: "panel-1", Name: "Proyecto X abiertas", IntegrationID: "redmine", Project: "proyecto-x", Status: "open"},
		},
		PollIntervalMinutes: defaultPollIntervalMinutes,
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Panels) != 1 {
		t.Fatalf("expected 1 panel, got %+v", loaded.Panels)
	}
	panel := loaded.Panels[0]
	if panel.Name != "Proyecto X abiertas" || panel.IntegrationID != "redmine" || panel.Project != "proyecto-x" || panel.Status != "open" {
		t.Errorf("unexpected panel after round-trip: %+v", panel)
	}
}

// TestLoadMigratesLegacyProvidersShape uses the exact shape a real
// pre-multi-integration config.json had (one slot per provider type, keyed
// by name) to verify Load() transparently upgrades it: each old entry
// becomes an Integration whose ID is the old provider name (so its
// already-stored keychain secrets keep resolving — see
// migrateLegacyConfig), and the migration is persisted back to disk.
func TestLoadMigratesLegacyProvidersShape(t *testing.T) {
	dir := withTempConfigDir(t)

	legacyJSON := `{
		"providers": {
			"gitlab": {"enabled": false, "values": {"base_url": "https://git.example.com/"}},
			"redmine": {"enabled": true, "values": {"base_url": "http://redmine"}},
			"todoist": {"enabled": true, "values": {}}
		},
		"pollIntervalMinutes": 7
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(legacyJSON), 0o600); err != nil {
		t.Fatalf("writing legacy config.json: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PollIntervalMinutes != 7 {
		t.Errorf("expected poll interval 7, got %d", cfg.PollIntervalMinutes)
	}
	if len(cfg.Integrations) != 3 {
		t.Fatalf("expected 3 migrated integrations, got %d: %+v", len(cfg.Integrations), cfg.Integrations)
	}

	gitlab, ok := cfg.IntegrationByID("gitlab")
	if !ok || gitlab.Type != "gitlab" || gitlab.Name != "GitLab" || gitlab.Enabled || gitlab.Values["base_url"] != "https://git.example.com/" {
		t.Errorf("unexpected migrated gitlab integration: %+v", gitlab)
	}
	redmine, ok := cfg.IntegrationByID("redmine")
	if !ok || redmine.Type != "redmine" || redmine.Name != "Redmine" || !redmine.Enabled || redmine.Values["base_url"] != "http://redmine" {
		t.Errorf("unexpected migrated redmine integration: %+v", redmine)
	}
	todoist, ok := cfg.IntegrationByID("todoist")
	if !ok || todoist.Type != "todoist" || todoist.Name != "Todoist" || !todoist.Enabled {
		t.Errorf("unexpected migrated todoist integration: %+v", todoist)
	}

	// The migration must have been persisted, not just returned in-memory.
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("reading config.json after migration: %v", err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("unmarshalling migrated config.json: %v", err)
	}
	if _, ok := onDisk["integrations"]; !ok {
		t.Errorf("expected migrated config.json to have an \"integrations\" key, got: %s", data)
	}
}

// TestLoadDoesNotMigrateFreshNewShapeConfig ensures a legitimately-empty
// new-shape config (e.g. every integration deleted) isn't mistaken for a
// legacy file needing migration.
func TestLoadDoesNotMigrateFreshNewShapeConfig(t *testing.T) {
	dir := withTempConfigDir(t)

	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"integrations":[],"pollIntervalMinutes":5}`), 0o600); err != nil {
		t.Fatalf("writing config.json: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Integrations) != 0 {
		t.Errorf("expected no integrations, got %+v", cfg.Integrations)
	}
}
