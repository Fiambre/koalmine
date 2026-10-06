package hotkey

import "testing"

func TestParseComboValid(t *testing.T) {
	cases := []struct {
		in   string
		want Combo
	}{
		{"Ctrl+Shift+K", Combo{Modifiers: ModControl | ModShift, Key: 'K'}},
		{"ctrl+k", Combo{Modifiers: ModControl, Key: 'K'}},
		{"Alt+F5", Combo{Modifiers: ModAlt, Key: 0x74}},
		{"Ctrl+Alt+Shift+Win+5", Combo{Modifiers: ModControl | ModAlt | ModShift | ModWin, Key: '5'}},
		{"Ctrl+Space", Combo{Modifiers: ModControl, Key: 0x20}},
		{"Ctrl+ArrowUp", Combo{Modifiers: ModControl, Key: 0x26}},
		{"Cmd+K", Combo{Modifiers: ModWin, Key: 'K'}},
	}
	for _, c := range cases {
		got, err := ParseCombo(c.in)
		if err != nil {
			t.Errorf("ParseCombo(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseCombo(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestParseComboEmpty(t *testing.T) {
	if _, err := ParseCombo(""); err != ErrEmpty {
		t.Errorf("ParseCombo(\"\"): expected ErrEmpty, got %v", err)
	}
	if _, err := ParseCombo("   "); err != ErrEmpty {
		t.Errorf("ParseCombo(whitespace): expected ErrEmpty, got %v", err)
	}
}

func TestParseComboRequiresModifier(t *testing.T) {
	if _, err := ParseCombo("K"); err == nil {
		t.Error("expected an error for a bare key with no modifier")
	}
}

func TestParseComboRequiresExactlyOneKey(t *testing.T) {
	if _, err := ParseCombo("Ctrl+Shift"); err == nil {
		t.Error("expected an error for a combo with only modifiers")
	}
	if _, err := ParseCombo("Ctrl+K+L"); err == nil {
		t.Error("expected an error for a combo with two non-modifier keys")
	}
}

func TestParseComboRejectsUnknownKey(t *testing.T) {
	if _, err := ParseCombo("Ctrl+Frobnicate"); err == nil {
		t.Error("expected an error for an unsupported key token")
	}
}

func TestFormatRoundTrips(t *testing.T) {
	cases := []string{"Ctrl+Shift+K", "Alt+F12", "Ctrl+Alt+Shift+Win+5", "Ctrl+Space", "Ctrl+ArrowLeft"}
	for _, in := range cases {
		combo, err := ParseCombo(in)
		if err != nil {
			t.Fatalf("ParseCombo(%q): %v", in, err)
		}
		if got := Format(combo); got != in {
			t.Errorf("Format(ParseCombo(%q)) = %q, want %q", in, got, in)
		}
	}
}

func TestFormatCanonicalizesModifierOrder(t *testing.T) {
	combo, err := ParseCombo("Shift+Ctrl+K")
	if err != nil {
		t.Fatalf("ParseCombo: %v", err)
	}
	if got, want := Format(combo), "Ctrl+Shift+K"; got != want {
		t.Errorf("Format = %q, want %q (modifiers should canonicalize to Ctrl+Alt+Shift+Win order)", got, want)
	}
}
