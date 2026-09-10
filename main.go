package main

import (
	"embed"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"koalmine/internal/notify"
	"koalmine/internal/singleinstance"
)

//go:embed all:frontend/dist
var assets embed.FS

var app *App

func main() {
	app = NewApp()
	notify.Icon = trayIconOther

	// If Koalmine is already running, ask it to show its window instead of
	// starting a second tray icon and hidden window on top of it.
	if !singleinstance.Acquire(showExistingWindow) {
		return
	}

	systray.Run(onTrayReady, onTrayExit)
}

// showExistingWindow is the callback a later launch's singleinstance
// request triggers on the already-running instance. app.ctx is nil until
// app.startup runs; a request arriving in that narrow startup window is
// dropped rather than queued, since by the time anyone could race it the
// window has already finished mounting.
func showExistingWindow() {
	if app.ctx == nil {
		return
	}
	wailsRuntime.WindowShow(app.ctx)
	wailsRuntime.WindowUnminimise(app.ctx)
}

// runWails starts the Wails application. It must run on its own goroutine
// since the main goroutine is parked in systray's native event loop.
func runWails() {
	lockRenderThread()

	err := wails.Run(&options.App{
		Title:             "Koalmine",
		Width:             1024,
		Height:            768,
		StartHidden:       true,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}

	systray.Quit()
}
