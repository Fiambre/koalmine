package main

import (
	_ "embed"
	"log"
	"runtime"

	"github.com/getlantern/systray"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"koalmine/internal/notify"
	"koalmine/internal/updater"
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
	mRefresh := systray.AddMenuItem("Actualizar ahora", "Buscar tareas nuevas ahora")
	mSettings := systray.AddMenuItem("Configuración", "Abrir la configuración de proveedores")
	mUpdate := systray.AddMenuItem("Actualización disponible", "Descargar e instalar la actualización")
	mUpdate.Hide() // shown once app.onUpdateAvailable fires
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Salir", "Cerrar Koalmine")

	app.onUpdateAvailable = func(info updater.Info) {
		mUpdate.SetTitle("Actualización disponible: " + info.Version)
		mUpdate.Show()
	}

	go runWails()

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				if app.ctx != nil {
					wailsRuntime.WindowShow(app.ctx)
				}
			case <-mRefresh.ClickedCh:
				if app.ctx != nil {
					app.RefreshNow()
				}
			case <-mSettings.ClickedCh:
				if app.ctx != nil {
					wailsRuntime.WindowShow(app.ctx)
					wailsRuntime.EventsEmit(app.ctx, "navigate", "settings")
				}
			case <-mUpdate.ClickedCh:
				if app.ctx != nil {
					if err := app.ApplyUpdate(); err != nil {
						log.Printf("no se pudo aplicar la actualización: %v", err)
						_ = notify.Send("No se pudo actualizar", err.Error())
					}
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
