package main

import (
	_ "embed"
	"runtime"

	"github.com/getlantern/systray"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var trayIconWindows []byte

//go:embed build/appicon.png
var trayIconOther []byte

func trayIcon() []byte {
	if runtime.GOOS == "windows" {
		return trayIconWindows
	}
	return trayIconOther
}

func onTrayReady() {
	systray.SetIcon(trayIcon())
	systray.SetTooltip("Koalmine — tus tareas")

	mShow := systray.AddMenuItem("Mostrar", "Mostrar la ventana principal")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Salir", "Cerrar Koalmine")

	go runWails()

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				if app.ctx != nil {
					wailsRuntime.WindowShow(app.ctx)
				}
			case <-mQuit.ClickedCh:
				if app.ctx != nil {
					wailsRuntime.Quit(app.ctx)
				}
				return
			}
		}
	}()
}

func onTrayExit() {
}
