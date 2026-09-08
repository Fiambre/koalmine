//go:build linux

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

func desktopFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "koalmine.desktop"), nil
}

// IsEnabled reports whether Koalmine is registered to launch at login.
func IsEnabled() (bool, error) {
	path, err := desktopFilePath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Enable writes a .desktop autostart entry that runs the currently running
// executable at login.
func Enable() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	path, err := desktopFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Koalmine
Exec=%s
X-GNOME-Autostart-enabled=true
`, exePath)

	return os.WriteFile(path, []byte(content), 0o644)
}

// Disable removes the .desktop autostart entry, if any.
func Disable() error {
	path, err := desktopFilePath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
