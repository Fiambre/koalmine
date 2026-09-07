// Package notify wraps beeep to send native OS notifications (Windows
// toast, macOS Notification Center, Linux libnotify) behind one function,
// so the rest of the app doesn't depend on beeep directly.
package notify

import "github.com/gen2brain/beeep"

// Send shows a native notification with the given title and message.
func Send(title, message string) error {
	return beeep.Notify(title, message, "")
}
