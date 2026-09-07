//go:build windows

package main

import (
	"log"
	"runtime"

	"golang.org/x/sys/windows"
)

// lockRenderThread pins the calling goroutine to its OS thread and
// initialises COM as single-threaded apartment on it. It must run before
// wails.Run() when wails is started from a goroutine other than the
// program's main goroutine (e.g. because the main goroutine is occupied by
// systray's native message loop) — WebView2/COM require initialisation on
// the exact thread that will create and drive the webview.
func lockRenderThread() {
	runtime.LockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		log.Printf("CoInitializeEx: %v", err)
	}
}
