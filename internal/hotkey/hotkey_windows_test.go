//go:build windows

package hotkey

import "testing"

// TestRegisterAndClose touches a real OS resource (the system-wide hotkey
// table) deliberately — same spirit as autostart_windows_test.go's own
// real-registry test — since that's the only way to verify RegisterHotKey/
// Close actually work. Uses an obscure combo unlikely to collide with
// anything already registered on the dev machine, and always releases it.
func TestRegisterAndClose(t *testing.T) {
	combo := Combo{Modifiers: ModControl | ModAlt | ModShift, Key: 0x7B} // Ctrl+Alt+Shift+F12

	reg, err := Register(combo, func() {})
	if err != nil {
		t.Fatalf("Register: %v (another app may already own Ctrl+Alt+Shift+F12 on this machine)", err)
	}
	reg.Close()

	// If Close actually unregistered it, registering the same combo again
	// must succeed — a leak would make this second Register fail.
	reg2, err := Register(combo, func() {})
	if err != nil {
		t.Fatalf("Register after Close: %v (hotkey wasn't released)", err)
	}
	reg2.Close()
}

func TestCloseIsIdempotent(t *testing.T) {
	combo := Combo{Modifiers: ModControl | ModAlt | ModShift, Key: 0x7B}
	reg, err := Register(combo, func() {})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	reg.Close()
	reg.Close() // must not hang or panic
}
