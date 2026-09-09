package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"koalmine/internal/autostart"
	"koalmine/internal/notify"
	"koalmine/internal/poller"
	"koalmine/internal/providers"
	"koalmine/internal/store"
	"koalmine/internal/updater"
	"koalmine/internal/version"
)

// App struct
type App struct {
	ctx    context.Context
	poller *poller.Poller

	tasksMu sync.RWMutex
	tasks   []providers.TaskItem

	updateMu     sync.RWMutex
	latestUpdate updater.Info

	// onUpdateAvailable, set by tray.go, lets the tray menu react when a
	// new version is found without app.go needing to know about systray.
	onUpdateAvailable func(updater.Info)
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{poller: poller.New()}
}

// startup is called when the app starts. The context is saved so we can
// call the runtime methods. It loads the last cached snapshot of tasks so
// the window has something to show immediately (rather than "Cargando…" on
// every launch), then kicks off the background poller — its OnUpdate
// callback replaces that cache with the fresh snapshot, persists it for the
// next launch, and emits a "tasks:updated" event so the frontend refreshes
// without polling itself.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if cached, err := store.LoadCachedTasks(); err != nil {
		log.Printf("no se pudo cargar el caché de tareas: %v", err)
	} else {
		a.tasksMu.Lock()
		a.tasks = cached
		a.tasksMu.Unlock()
	}

	a.poller.OnUpdate = func(items []providers.TaskItem) {
		a.tasksMu.Lock()
		a.tasks = items
		a.tasksMu.Unlock()
		wailsRuntime.EventsEmit(ctx, "tasks:updated", items)
		if err := store.SaveCachedTasks(items); err != nil {
			log.Printf("no se pudo guardar el caché de tareas: %v", err)
		}
	}
	go a.poller.Run(ctx)
	go a.watchForUpdates(ctx)
}

// watchForUpdates checks for a newer release on startup, then again every
// updater.CheckInterval, until ctx is cancelled.
func (a *App) watchForUpdates(ctx context.Context) {
	a.checkForUpdate(ctx)

	ticker := time.NewTicker(updater.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.checkForUpdate(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) checkForUpdate(ctx context.Context) {
	info, err := updater.Check(ctx)
	if err != nil {
		log.Printf("updater: %v", err)
		return
	}

	a.updateMu.Lock()
	a.latestUpdate = info
	a.updateMu.Unlock()

	if !info.Available {
		return
	}

	wailsRuntime.EventsEmit(ctx, "update:available", info.Version)
	if err := notify.Send("Actualización disponible", "Koalmine "+info.Version+" está disponible."); err != nil {
		log.Printf("no se pudo enviar la notificación de actualización: %v", err)
	}
	if a.onUpdateAvailable != nil {
		a.onUpdateAvailable(info)
	}
}

// GetAppVersion returns the running version, e.g. "0.1.0".
func (a *App) GetAppVersion() string {
	return version.Current
}

// GetUpdateStatus returns the result of the most recent update check.
func (a *App) GetUpdateStatus() updater.Info {
	a.updateMu.RLock()
	defer a.updateMu.RUnlock()
	return a.latestUpdate
}

// ApplyUpdate downloads and applies the update found by the most recent
// check, then relaunches the app and quits the current instance. Errors if
// no update is currently known to be available.
func (a *App) ApplyUpdate() error {
	a.updateMu.RLock()
	info := a.latestUpdate
	a.updateMu.RUnlock()

	if !info.Available {
		return fmt.Errorf("no hay ninguna actualización disponible")
	}

	if err := updater.Apply(a.ctx, info.DownloadURL); err != nil {
		return err
	}
	if err := updater.Relaunch(); err != nil {
		return err
	}

	wailsRuntime.Quit(a.ctx)
	return nil
}

// GetTasks returns the last known snapshot of tasks across every enabled
// provider. Empty until the first poll completes.
func (a *App) GetTasks() []providers.TaskItem {
	a.tasksMu.RLock()
	defer a.tasksMu.RUnlock()
	return a.tasks
}

// RefreshNow triggers an immediate poll instead of waiting for the next tick.
func (a *App) RefreshNow() {
	go a.poller.PollNow(a.ctx)
}

// OpenURL opens the given URL in the user's default browser.
func (a *App) OpenURL(url string) {
	wailsRuntime.BrowserOpenURL(a.ctx, url)
}

// CreateTaskInput is what the "new task" form in the frontend submits.
type CreateTaskInput struct {
	Provider    string `json:"provider"`
	Project     string `json:"project"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// CreateTask creates a new issue on the given provider, assigned to the
// authenticated user. It's merged into the current in-memory snapshot and
// broadcast immediately, rather than waiting for the next poll cycle, so it
// shows up in the list right away.
func (a *App) CreateTask(input CreateTaskInput) (providers.TaskItem, error) {
	p, ok := providers.Get(input.Provider)
	if !ok {
		return providers.TaskItem{}, fmt.Errorf("proveedor desconocido: %s", input.Provider)
	}

	resolved, err := store.ResolveConfig(p, input.Provider)
	if err != nil {
		return providers.TaskItem{}, err
	}

	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()

	item, err := p.CreateItem(ctx, resolved, providers.CreateItemInput{
		Project:     input.Project,
		Title:       input.Title,
		Description: input.Description,
	})
	if err != nil {
		return providers.TaskItem{}, err
	}

	a.tasksMu.Lock()
	a.tasks = append([]providers.TaskItem{item}, a.tasks...)
	snapshot := a.tasks
	a.tasksMu.Unlock()

	wailsRuntime.EventsEmit(a.ctx, "tasks:updated", snapshot)
	if err := store.SaveCachedTasks(snapshot); err != nil {
		log.Printf("no se pudo guardar el caché de tareas: %v", err)
	}

	return item, nil
}

// ProviderInfo describes one provider for the settings UI: its static
// metadata (name, config fields) plus its current configuration state.
// Secret field values are never sent to the frontend — only whether one
// has been set — so tokens/API keys never round-trip through the webview.
type ProviderInfo struct {
	Name        string                  `json:"name"`
	DisplayName string                  `json:"displayName"`
	Fields      []providers.ConfigField `json:"fields"`
	Enabled     bool                    `json:"enabled"`
	Values      map[string]string       `json:"values"`
	SecretsSet  map[string]bool         `json:"secretsSet"`
	ProjectHint string                  `json:"projectHint"`
}

// ListProviders returns every registered provider with its current
// configuration, for the settings screen to render.
func (a *App) ListProviders() ([]ProviderInfo, error) {
	cfg, err := store.Load()
	if err != nil {
		return nil, err
	}

	result := make([]ProviderInfo, 0)
	for _, p := range providers.List() {
		pc := cfg.Providers[p.Name()]
		fields := p.ConfigFields()

		info := ProviderInfo{
			Name:        p.Name(),
			DisplayName: p.DisplayName(),
			Fields:      fields,
			Enabled:     pc.Enabled,
			Values:      map[string]string{},
			SecretsSet:  map[string]bool{},
			ProjectHint: p.ProjectHint(),
		}

		for _, field := range fields {
			if field.Kind == providers.FieldSecret {
				secret, err := store.GetSecret(p.Name(), field.Key)
				if err != nil {
					return nil, fmt.Errorf("no se pudo leer el secreto de %s: %w", p.DisplayName(), err)
				}
				info.SecretsSet[field.Key] = secret != ""
			} else {
				info.Values[field.Key] = pc.Values[field.Key]
			}
		}

		result = append(result, info)
	}
	return result, nil
}

// SaveProviderConfig persists one provider's settings: non-secret values go
// to the config file, secret values go to the OS keychain. A blank secret
// value leaves any previously stored secret untouched (the settings form
// never pre-fills secrets, so an empty field means "unchanged", not "clear").
func (a *App) SaveProviderConfig(providerName string, enabled bool, values map[string]string) error {
	p, ok := providers.Get(providerName)
	if !ok {
		return fmt.Errorf("proveedor desconocido: %s", providerName)
	}

	cfg, err := store.Load()
	if err != nil {
		return err
	}

	pc := cfg.Providers[providerName]
	pc.Enabled = enabled
	if pc.Values == nil {
		pc.Values = map[string]string{}
	}

	for _, field := range p.ConfigFields() {
		value, provided := values[field.Key]
		if !provided {
			continue
		}
		if field.Kind == providers.FieldSecret {
			if value == "" {
				continue
			}
			if err := store.SetSecret(providerName, field.Key, value); err != nil {
				return fmt.Errorf("no se pudo guardar el secreto: %w", err)
			}
		} else {
			pc.Values[field.Key] = value
		}
	}

	cfg.Providers[providerName] = pc
	return store.Save(cfg)
}

// TestConnection verifies connectivity for a provider using the given
// (possibly unsaved) form values, falling back to already-stored values for
// any field left blank — so "Probar conexión" works before hitting Guardar.
func (a *App) TestConnection(providerName string, values map[string]string) error {
	p, ok := providers.Get(providerName)
	if !ok {
		return fmt.Errorf("proveedor desconocido: %s", providerName)
	}

	resolved, err := a.resolveProviderConfig(p, providerName, values)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return p.TestConnection(ctx, resolved)
}

func (a *App) resolveProviderConfig(p providers.Provider, providerName string, values map[string]string) (providers.Config, error) {
	resolved, err := store.ResolveConfig(p, providerName)
	if err != nil {
		return nil, err
	}
	for _, field := range p.ConfigFields() {
		if v, ok := values[field.Key]; ok && v != "" {
			resolved[field.Key] = v
		}
	}
	return resolved, nil
}

// GetAutostartEnabled reports whether Koalmine is registered to launch when
// the user logs in.
func (a *App) GetAutostartEnabled() (bool, error) {
	return autostart.IsEnabled()
}

// SetAutostartEnabled registers or unregisters Koalmine to launch at login.
func (a *App) SetAutostartEnabled(enabled bool) error {
	if enabled {
		return autostart.Enable()
	}
	return autostart.Disable()
}

// GetPollIntervalMinutes returns how often (in minutes) the poller checks
// for new items.
func (a *App) GetPollIntervalMinutes() (int, error) {
	cfg, err := store.Load()
	if err != nil {
		return 0, err
	}
	return cfg.PollIntervalMinutes, nil
}

// SetPollIntervalMinutes updates the polling interval.
func (a *App) SetPollIntervalMinutes(minutes int) error {
	if minutes < 1 {
		minutes = 1
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	cfg.PollIntervalMinutes = minutes
	return store.Save(cfg)
}
