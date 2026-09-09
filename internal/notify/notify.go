// Package notify wraps beeep to send native OS notifications (Windows
// toast, macOS Notification Center, Linux libnotify) behind one function,
// so the rest of the app doesn't depend on beeep directly.
package notify

import "github.com/gen2brain/beeep"

func init() {
	// beeep.AppName defaults to the literal string "DefaultAppName", which
	// is exactly what Windows shows as the toast's app attribution (and
	// what it uses as the AUMID to key the registered icon) if left unset.
	beeep.AppName = "Koalmine"
}

// Icon is the PNG shown next to notifications, set once at startup from the
// embedded app icon (see main.go). Left nil, notifications just show with
// no icon rather than failing.
var Icon []byte

// Send shows a native notification with the given title and message.
func Send(title, message string) error {
	if Icon != nil {
		return beeep.Notify(title, message, Icon)
	}
	return beeep.Notify(title, message, "")
}
