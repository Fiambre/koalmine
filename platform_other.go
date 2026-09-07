//go:build !windows

package main

// lockRenderThread is a no-op outside Windows: macOS/Linux webview backends
// don't need the explicit per-thread COM initialisation Windows/WebView2
// requires here. Their own thread-affinity needs (Cocoa's main thread,
// GTK's main loop) are handled by systray/wails themselves.
func lockRenderThread() {}
