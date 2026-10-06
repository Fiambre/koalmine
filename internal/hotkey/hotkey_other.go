//go:build !windows

package hotkey

import "fmt"

// Registration exists so callers can compile against the same API on every
// platform; on a non-Windows build Register never actually returns one.
type Registration struct{}

// Close is a no-op — no real Registration is ever produced on this
// platform (Register always errors), but callers may still hold a nil
// *Registration through the same code paths app.go uses on Windows.
func (r *Registration) Close() {}

// Register always fails: global hotkey registration is a genuinely
// OS-specific primitive (Win32 RegisterHotKey vs. Carbon/Cocoa vs.
// X11/Wayland), and only the Windows backend (hotkey_windows.go) is
// implemented — matching this project's other documented per-OS gaps
// (e.g. internal/autostart's three separate implementations, or
// providers/redmine.go's "no PR concept" gap). combo and callback are
// unused on this path but kept so both build's Register has one signature.
func Register(combo Combo, callback func()) (*Registration, error) {
	return nil, fmt.Errorf("los atajos globales todavía no están soportados en este sistema operativo")
}
