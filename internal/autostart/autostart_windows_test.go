//go:build windows

package autostart

import "testing"

func TestEnableDisableRoundTrip(t *testing.T) {
	// Saved and restored as the raw registry value, not just the
	// enabled/disabled bool — re-running Enable() in cleanup would
	// register *this test binary's own* os.Executable() path instead of
	// whatever was really there before (e.g. the installed Koalmine.exe),
	// corrupting a real autostart registration.
	original, err := registeredPath()
	if err != nil {
		t.Fatalf("registeredPath (initial): %v", err)
	}
	t.Cleanup(func() {
		_ = setRegisteredPath(original)
	})

	if err := Enable(); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	enabled, err := IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled (after Enable): %v", err)
	}
	if !enabled {
		t.Error("expected IsEnabled to be true after Enable")
	}

	if err := Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	enabled, err = IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled (after Disable): %v", err)
	}
	if enabled {
		t.Error("expected IsEnabled to be false after Disable")
	}
}

func TestDisableWhenNeverEnabledIsNoop(t *testing.T) {
	original, err := IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled (initial): %v", err)
	}
	if original {
		t.Skip("autostart is currently enabled on this machine; skipping to avoid disturbing it")
	}

	if err := Disable(); err != nil {
		t.Fatalf("Disable on an already-disabled state should be a no-op, got: %v", err)
	}
}
