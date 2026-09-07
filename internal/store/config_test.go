package store

import "testing"

func withTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	original := configDir
	configDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { configDir = original })
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
	if cfg.Providers == nil {
		t.Error("expected Providers to be a non-nil empty map")
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	withTempConfigDir(t)

	cfg := Config{
		Providers: map[string]ProviderConfig{
			"redmine": {Enabled: true, Values: map[string]string{"base_url": "https://redmine.example.com"}},
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
	pc, ok := loaded.Providers["redmine"]
	if !ok || !pc.Enabled || pc.Values["base_url"] != "https://redmine.example.com" {
		t.Errorf("unexpected redmine config after round-trip: %+v", pc)
	}
}
