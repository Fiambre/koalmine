//go:build windows

// Package autostart manages registering Koalmine to launch when the user
// logs in. Each OS has its own mechanism (Windows: a Run registry value,
// macOS: a LaunchAgent plist, Linux: a .desktop file in ~/.config/autostart)
// behind the same three-function API.
package autostart

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const valueName = "Koalmine"

// IsEnabled reports whether Koalmine is registered to launch at login.
func IsEnabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer key.Close()

	if _, _, err := key.GetStringValue(valueName); err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Enable registers the currently running executable to launch at login.
func Enable() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	return key.SetStringValue(valueName, `"`+exePath+`"`)
}

// Disable removes the login registration, if any.
func Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err == registry.ErrNotExist {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()

	err = key.DeleteValue(valueName)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}
