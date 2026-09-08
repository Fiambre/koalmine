package providers

import "testing"

func TestRegistryListIncludesBuiltinProviders(t *testing.T) {
	names := map[string]bool{}
	for _, p := range List() {
		names[p.Name()] = true
	}
	for _, want := range []string{"redmine", "github", "gitlab"} {
		if !names[want] {
			t.Errorf("expected provider %q to be registered", want)
		}
	}
}

func TestGetUnknownProvider(t *testing.T) {
	if _, ok := Get("does-not-exist"); ok {
		t.Error("expected ok=false for an unknown provider")
	}
}

func TestGetReturnsFreshInstances(t *testing.T) {
	a, _ := Get("redmine")
	b, _ := Get("redmine")
	if a == b {
		t.Error("expected Get to return distinct instances, not a shared singleton")
	}
}
