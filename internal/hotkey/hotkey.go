// Package hotkey registers a system-wide keyboard shortcut that fires
// regardless of which window has focus. Actual registration is
// OS-specific (see hotkey_windows.go/hotkey_other.go); this file holds the
// platform-independent combo string parsing shared by both, and by
// callers that want to validate a combo before attempting to register it.
package hotkey

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Combo is a parsed hotkey: a modifier bitmask plus one key. Modifiers uses
// the real Win32 MOD_* bit values (1/2/4/8) and Key the real Win32
// virtual-key code directly, rather than an abstracted set of our own —
// there is exactly one real backend today (hotkey_windows.go), and a
// hypothetical future macOS/Linux backend would need its own combo
// semantics anyway, so a translation layer here would have no one to
// serve yet.
type Combo struct {
	Modifiers uint32
	Key       uint32
}

const (
	ModAlt     uint32 = 0x0001
	ModControl uint32 = 0x0002
	ModShift   uint32 = 0x0004
	ModWin     uint32 = 0x0008
)

// ErrEmpty is returned by ParseCombo("") — the sentinel for "no hotkey
// configured", which callers should check explicitly rather than treating
// as a malformed combo.
var ErrEmpty = errors.New("no hay ningún atajo configurado")

// modifierNames maps a lowercased modifier token to its bit. "Win" covers
// both the Windows key and (for a hypothetical macOS backend) Cmd —
// whichever the platform calls its "super" key.
var modifierNames = map[string]uint32{
	"ctrl":    ModControl,
	"control": ModControl,
	"alt":     ModAlt,
	"shift":   ModShift,
	"win":     ModWin,
	"cmd":     ModWin,
	"meta":    ModWin,
}

// keyTokens maps a canonical key token (as produced by Format, and as the
// frontend's capture UI sends) to its virtual-key code. Letters and digits
// are handled separately below since VK codes for 'A'-'Z'/'0'-'9' equal
// their ASCII values — no need to enumerate 36 entries here. Built by a
// function (rather than a func init() appending F1-F12 to a map literal)
// so keyNames below, which is itself built from this map, can't observe a
// half-populated keyTokens — Go runs package-level var initializers before
// func init(), so appending in an init() would run too late for that.
var keyTokens = buildKeyTokens()

func buildKeyTokens() map[string]uint32 {
	m := map[string]uint32{
		"Space":      0x20,
		"Escape":     0x1B,
		"Tab":        0x09,
		"Enter":      0x0D,
		"Backspace":  0x08,
		"Delete":     0x2E,
		"Insert":     0x2D,
		"Home":       0x24,
		"End":        0x23,
		"PageUp":     0x21,
		"PageDown":   0x22,
		"ArrowUp":    0x26,
		"ArrowDown":  0x28,
		"ArrowLeft":  0x25,
		"ArrowRight": 0x27,
	}
	for n := 1; n <= 12; n++ {
		m[fmt.Sprintf("F%d", n)] = 0x6F + uint32(n) // VK_F1 = 0x70
	}
	return m
}

// keyNames is the reverse of keyTokens, built once for Format.
var keyNames = func() map[uint32]string {
	m := make(map[uint32]string, len(keyTokens))
	for name, vk := range keyTokens {
		m[vk] = name
	}
	return m
}()

// ParseCombo parses a canonical combo string like "Ctrl+Shift+K" into a
// Combo. It requires at least one modifier — a bare key would hijack that
// key system-wide (e.g. registering plain "K" breaks typing "k"
// everywhere) — and exactly one non-modifier key token. Modifier names are
// case-insensitive; the key token itself is upper-cased before lookup, so
// "ctrl+k" and "Ctrl+K" both parse.
func ParseCombo(s string) (Combo, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Combo{}, ErrEmpty
	}

	parts := strings.Split(s, "+")
	var mods uint32
	var key uint32
	var keyFound bool

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return Combo{}, fmt.Errorf("combinación inválida: %q", s)
		}
		if mod, ok := modifierNames[strings.ToLower(part)]; ok {
			mods |= mod
			continue
		}
		if keyFound {
			return Combo{}, fmt.Errorf("combinación inválida: %q (más de una tecla no modificadora)", s)
		}
		vk, err := parseKeyToken(part)
		if err != nil {
			return Combo{}, err
		}
		key = vk
		keyFound = true
	}

	if !keyFound {
		return Combo{}, fmt.Errorf("combinación inválida: %q (falta la tecla)", s)
	}
	if mods == 0 {
		return Combo{}, fmt.Errorf("la combinación necesita al menos un modificador (Ctrl/Alt/Shift/Win): %q", s)
	}

	return Combo{Modifiers: mods, Key: key}, nil
}

func parseKeyToken(token string) (uint32, error) {
	upper := strings.ToUpper(token)
	if len(upper) == 1 {
		c := upper[0]
		switch {
		case c >= 'A' && c <= 'Z':
			return uint32(c), nil
		case c >= '0' && c <= '9':
			return uint32(c), nil
		}
	}
	// Named tokens (F1-F12, Space, ArrowUp, ...) are matched against the
	// canonical casing directly (e.g. "ArrowUp", not "arrowup") since
	// that's what both Format and the frontend's capture UI produce.
	if vk, ok := keyTokens[token]; ok {
		return vk, nil
	}
	return 0, fmt.Errorf("tecla no soportada: %q", token)
}

// Format renders a Combo back into its canonical string form, in a fixed
// Ctrl+Alt+Shift+Win order, so a saved combo redisplays the same way it
// was parsed (used by Settings' "current hotkey" display).
func Format(c Combo) string {
	var parts []string
	if c.Modifiers&ModControl != 0 {
		parts = append(parts, "Ctrl")
	}
	if c.Modifiers&ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if c.Modifiers&ModShift != 0 {
		parts = append(parts, "Shift")
	}
	if c.Modifiers&ModWin != 0 {
		parts = append(parts, "Win")
	}
	parts = append(parts, keyName(c.Key))
	return strings.Join(parts, "+")
}

func keyName(vk uint32) string {
	if (vk >= 'A' && vk <= 'Z') || (vk >= '0' && vk <= '9') {
		return string(rune(vk))
	}
	if name, ok := keyNames[vk]; ok {
		return name
	}
	return "0x" + strconv.FormatUint(uint64(vk), 16)
}
