package main

import (
	"embed"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

var app *App

func main() {
	app = NewApp()
	systray.Run(onTrayReady, onTrayExit)
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
