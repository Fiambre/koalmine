package store

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestSecretRoundTrip(t *testing.T) {
	keyring.MockInit()

	if err := SetSecret("github", "token", "ghp_example"); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	got, err := GetSecret("github", "token")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got != "ghp_example" {
		t.Errorf("expected %q, got %q", "ghp_example", got)
	}

	if err := DeleteSecret("github", "token"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	got, err = GetSecret("github", "token")
	if err != nil {
		t.Fatalf("GetSecret after delete: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string after delete, got %q", got)
	}
}

func TestGetSecretMissingReturnsEmptyNotError(t *testing.T) {
	keyring.MockInit()

	got, err := GetSecret("redmine", "api_key")
	if err != nil {
		t.Fatalf("expected no error for a missing secret, got %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
