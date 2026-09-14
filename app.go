package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
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

	// Koalmine's window may be hidden (or never opened) when an update is
	// applied, so the in-app "Actualizando…" feedback can't be relied on
	// alone — confirm completion via an OS notification instead, which
	// works regardless of window state.
	if v, ok := updater.JustUpdatedTo(os.Args[1:]); ok {
		if err := notify.Send("Koalmine actualizado", "Ahora estás en la versión "+v+"."); err != nil {
			log.Printf("no se pudo enviar la notificación de actualización completa: %v", err)
		}
	}

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

func (a *App) checkForUpdate(ctx context.Context) (updater.Info, error) {
	info, err := updater.Check(ctx)
	if err != nil {
		log.Printf("updater: %v", err)
		return updater.Info{}, err
	}

	a.updateMu.Lock()
	a.latestUpdate = info
	a.updateMu.Unlock()

	if !info.Available {
		return info, nil
	}

	wailsRuntime.EventsEmit(ctx, "update:available", info.Version)
	if err := notify.Send("Actualización disponible", "Koalmine "+info.Version+" está disponible."); err != nil {
		log.Printf("no se pudo enviar la notificación de actualización: %v", err)
	}
	if a.onUpdateAvailable != nil {
		a.onUpdateAvailable(info)
	}
	return info, nil
}

// CheckForUpdateNow triggers an immediate, out-of-cycle update check and
// returns its result — used by the "Buscar actualizaciones" button in
// Settings, so it gets synchronous feedback instead of relying on the
// background watcher's next tick.
func (a *App) CheckForUpdateNow() (updater.Info, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return a.checkForUpdate(ctx)
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

	// Feedback for the case the Settings window isn't visible to show the
	// "Actualizando…" button state — the download can take a few seconds.
	if err := notify.Send("Actualizando Koalmine", "Descargando la versión "+info.Version+"…"); err != nil {
		log.Printf("no se pudo enviar la notificación de inicio de actualización: %v", err)
	}

	if err := updater.Apply(a.ctx, info.DownloadURL); err != nil {
		return err
	}
	if err := updater.Relaunch(info.Version); err != nil {
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

// resolvedIntegration bundles what every method below needs: the
// integration itself (for its Name/Type/ID), the live Provider instance
// for its Type, and its fully-resolved Config.
type resolvedIntegration struct {
	integration   store.Integration
	provider      providers.Provider
	config        providers.Config
	sameTypeCount int
}

// resolveIntegration loads the current config and looks up integrationID,
// returning everything needed to call into its provider. Used by every
// method that acts on one specific configured connection.
func (a *App) resolveIntegration(integrationID string) (resolvedIntegration, error) {
	cfg, err := store.Load()
	if err != nil {
		return resolvedIntegration{}, err
	}
	integ, ok := cfg.IntegrationByID(integrationID)
	if !ok {
		return resolvedIntegration{}, fmt.Errorf("integración desconocida: %s", integrationID)
	}
	p, ok := providers.Get(integ.Type)
	if !ok {
		return resolvedIntegration{}, fmt.Errorf("tipo de proveedor desconocido: %s", integ.Type)
	}
	resolved, err := store.ResolveConfig(p, integ.ID)
	if err != nil {
		return resolvedIntegration{}, err
	}
	return resolvedIntegration{
		integration:   integ,
		provider:      p,
		config:        resolved,
		sameTypeCount: cfg.SameTypeCount(integ.Type),
	}, nil
}

// ListProjects returns the projects/repos the given integration's
// authenticated user can create a task in, for the "new task" form's
// project dropdown.
func (a *App) ListProjects(integrationID string) ([]providers.ProjectOption, error) {
	ri, err := a.resolveIntegration(integrationID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return ri.provider.ListProjects(ctx, ri.config)
}

// GetComments returns the comments/notes on the given task item.
func (a *App) GetComments(item providers.TaskItem) ([]providers.Comment, error) {
	ri, err := a.resolveIntegration(store.IntegrationIDFor(item))
	if err != nil {
		return nil, err
	}
	item.ID = store.DenamespaceID(item.ID, ri.integration.ID)

	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return ri.provider.FetchComments(ctx, ri.config, item)
}

// RefreshTaskItem re-fetches one item's current data from its provider —
// used to heal a starred ("Seguimiento") item whose locally-saved snapshot
// is stale or was incomplete to begin with, since starred items otherwise
// only get refreshed when they happen to still be in scope for the regular
// poll (see providers.Provider.FetchItem).
func (a *App) RefreshTaskItem(item providers.TaskItem) (providers.TaskItem, error) {
	ri, err := a.resolveIntegration(store.IntegrationIDFor(item))
	if err != nil {
		return providers.TaskItem{}, err
	}
	item.ID = store.DenamespaceID(item.ID, ri.integration.ID)

	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	fresh, err := ri.provider.FetchItem(ctx, ri.config, item)
	if err != nil {
		return providers.TaskItem{}, err
	}
	return store.NamespaceItem(fresh, ri.integration, ri.sameTypeCount), nil
}

// CreateTaskInput is what the "new task" form in the frontend submits.
type CreateTaskInput struct {
	Integration string `json:"integration"`
	Project     string `json:"project"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// CreateTask creates a new issue on the given integration, assigned to the
// authenticated user. It's merged into the current in-memory snapshot and
// broadcast immediately, rather than waiting for the next poll cycle, so it
// shows up in the list right away.
func (a *App) CreateTask(input CreateTaskInput) (providers.TaskItem, error) {
	ri, err := a.resolveIntegration(input.Integration)
	if err != nil {
		return providers.TaskItem{}, err
	}

	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()

	item, err := ri.provider.CreateItem(ctx, ri.config, providers.CreateItemInput{
		Project:     input.Project,
		Title:       input.Title,
		Description: input.Description,
	})
	if err != nil {
		return providers.TaskItem{}, err
	}
	item = store.NamespaceItem(item, ri.integration, ri.sameTypeCount)

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

// SearchTasks runs a free-text search against every enabled integration
// (not just the cached snapshot), merging and sorting the results the same
// way the poller does. Errors from individual integrations are logged and
// skipped rather than failing the whole search, so one misbehaving
// connection doesn't block results from the others.
func (a *App) SearchTasks(query string) ([]providers.TaskItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []providers.TaskItem{}, nil
	}

	cfg, err := store.Load()
	if err != nil {
		return nil, err
	}

	var all []providers.TaskItem
	for _, integ := range cfg.Integrations {
		if !integ.Enabled {
			continue
		}

		p, ok := providers.Get(integ.Type)
		if !ok {
			log.Printf("search: %s: tipo de proveedor desconocido: %s", integ.Name, integ.Type)
			continue
		}

		resolved, err := store.ResolveConfig(p, integ.ID)
		if err != nil {
			log.Printf("search: %s: %v", integ.Name, err)
			continue
		}

		items, err := p.SearchItems(a.ctx, resolved, query)
		if err != nil {
			log.Printf("search: %s: %v", integ.Name, err)
			continue
		}
		all = append(all, store.NamespaceItems(items, integ, cfg.SameTypeCount(integ.Type))...)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })
	return all, nil
}

// ProviderTypeInfo describes one registered connector type: the catalog
// shown when picking a system type for a new integration. Unlike
// IntegrationInfo, this carries no configuration state — it's the same for
// everyone regardless of what they've configured.
type ProviderTypeInfo struct {
	Type        string                  `json:"type"`
	DisplayName string                  `json:"displayName"`
	Fields      []providers.ConfigField `json:"fields"`
	ProjectHint string                  `json:"projectHint"`
}

// ListProviderTypes returns every registered connector type, for the "add
// integration" form's system-type dropdown.
func (a *App) ListProviderTypes() []ProviderTypeInfo {
	list := providers.List()
	result := make([]ProviderTypeInfo, 0, len(list))
	for _, p := range list {
		result = append(result, ProviderTypeInfo{
			Type:        p.Name(),
			DisplayName: p.DisplayName(),
			Fields:      p.ConfigFields(),
			ProjectHint: p.ProjectHint(),
		})
	}
	return result
}

// IntegrationInfo describes one configured integration for the settings
// screen: its identity (id/type/name), its type's static metadata (fields,
// project hint), and its current configuration state. Secret field values
// are never sent to the frontend — only whether one has been set — so
// tokens/API keys never round-trip through the webview.
type IntegrationInfo struct {
	ID              string                  `json:"id"`
	Type            string                  `json:"type"`
	TypeDisplayName string                  `json:"typeDisplayName"`
	Name            string                  `json:"name"`
	Enabled         bool                    `json:"enabled"`
	Fields          []providers.ConfigField `json:"fields"`
	Values          map[string]string       `json:"values"`
	SecretsSet      map[string]bool         `json:"secretsSet"`
	ProjectHint     string                  `json:"projectHint"`
}

func integrationInfo(integ store.Integration, p providers.Provider) (IntegrationInfo, error) {
	fields := p.ConfigFields()
	info := IntegrationInfo{
		ID:              integ.ID,
		Type:            integ.Type,
		TypeDisplayName: p.DisplayName(),
		Name:            integ.Name,
		Enabled:         integ.Enabled,
		Fields:          fields,
		Values:          map[string]string{},
		SecretsSet:      map[string]bool{},
		ProjectHint:     p.ProjectHint(),
	}
	for _, field := range fields {
		if field.Kind == providers.FieldSecret {
			secret, err := store.GetSecret(integ.ID, field.Key)
			if err != nil {
				return IntegrationInfo{}, fmt.Errorf("no se pudo leer el secreto de %s: %w", integ.Name, err)
			}
			info.SecretsSet[field.Key] = secret != ""
		} else {
			info.Values[field.Key] = integ.Values[field.Key]
		}
	}
	return info, nil
}

// ListIntegrations returns every configured integration, for the settings
// screen to render. An integration whose provider type is no longer
// registered is skipped rather than crashing the settings screen.
func (a *App) ListIntegrations() ([]IntegrationInfo, error) {
	cfg, err := store.Load()
	if err != nil {
		return nil, err
	}

	result := make([]IntegrationInfo, 0, len(cfg.Integrations))
	for _, integ := range cfg.Integrations {
		p, ok := providers.Get(integ.Type)
		if !ok {
			continue
		}
		info, err := integrationInfo(integ, p)
		if err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, nil
}

// defaultIntegrationName suggests a name for a new integration when the
// user leaves it blank: the type's own display name, disambiguated with a
// counter if one of that type already exists ("GitLab", then "GitLab (2)",
// "GitLab (3)", ...).
func defaultIntegrationName(existing []store.Integration, displayName, providerType string) string {
	count := 0
	for _, integ := range existing {
		if integ.Type == providerType {
			count++
		}
	}
	if count == 0 {
		return displayName
	}
	return fmt.Sprintf("%s (%d)", displayName, count+1)
}

// CreateIntegration adds a new named connection of the given provider type,
// enabled by default. See defaultIntegrationName for what a blank name
// becomes.
func (a *App) CreateIntegration(providerType, name string, values map[string]string) (IntegrationInfo, error) {
	p, ok := providers.Get(providerType)
	if !ok {
		return IntegrationInfo{}, fmt.Errorf("tipo de proveedor desconocido: %s", providerType)
	}

	cfg, err := store.Load()
	if err != nil {
		return IntegrationInfo{}, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultIntegrationName(cfg.Integrations, p.DisplayName(), providerType)
	}

	integ := store.Integration{
		ID:      store.NewIntegrationID(providerType),
		Type:    providerType,
		Name:    name,
		Enabled: true,
		Values:  map[string]string{},
	}

	for _, field := range p.ConfigFields() {
		value, provided := values[field.Key]
		if !provided || value == "" {
			continue
		}
		if field.Kind == providers.FieldSecret {
			if err := store.SetSecret(integ.ID, field.Key, value); err != nil {
				return IntegrationInfo{}, fmt.Errorf("no se pudo guardar el secreto: %w", err)
			}
		} else {
			integ.Values[field.Key] = value
		}
	}

	cfg.Integrations = append(cfg.Integrations, integ)
	if err := store.Save(cfg); err != nil {
		return IntegrationInfo{}, err
	}

	return integrationInfo(integ, p)
}

// UpdateIntegration persists one integration's settings: non-secret values
// go to the config file, secret values go to the OS keychain. A blank
// secret value leaves any previously stored secret untouched (the settings
// form never pre-fills secrets, so an empty field means "unchanged", not
// "clear"). A blank name leaves the integration's existing name untouched.
func (a *App) UpdateIntegration(id, name string, enabled bool, values map[string]string) error {
	cfg, err := store.Load()
	if err != nil {
		return err
	}

	idx := -1
	for i, integ := range cfg.Integrations {
		if integ.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("integración desconocida: %s", id)
	}

	integ := cfg.Integrations[idx]
	p, ok := providers.Get(integ.Type)
	if !ok {
		return fmt.Errorf("tipo de proveedor desconocido: %s", integ.Type)
	}

	if name = strings.TrimSpace(name); name != "" {
		integ.Name = name
	}
	integ.Enabled = enabled
	if integ.Values == nil {
		integ.Values = map[string]string{}
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
			if err := store.SetSecret(integ.ID, field.Key, value); err != nil {
				return fmt.Errorf("no se pudo guardar el secreto: %w", err)
			}
		} else {
			integ.Values[field.Key] = value
		}
	}

	cfg.Integrations[idx] = integ
	return store.Save(cfg)
}

// DeleteIntegration removes one integration and every secret it had stored
// in the OS keychain. A no-op if the ID isn't found (already gone).
func (a *App) DeleteIntegration(id string) error {
	cfg, err := store.Load()
	if err != nil {
		return err
	}

	idx := -1
	for i, integ := range cfg.Integrations {
		if integ.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil
	}

	integ := cfg.Integrations[idx]
	if p, ok := providers.Get(integ.Type); ok {
		for _, field := range p.ConfigFields() {
			if field.Kind != providers.FieldSecret {
				continue
			}
			if err := store.DeleteSecret(integ.ID, field.Key); err != nil {
				log.Printf("no se pudo borrar el secreto %s de %s: %v", field.Key, integ.Name, err)
			}
		}
	}

	cfg.Integrations = append(cfg.Integrations[:idx], cfg.Integrations[idx+1:]...)
	return store.Save(cfg)
}

// TestConnection verifies connectivity for a provider type using the given
// (possibly unsaved) form values, falling back to integrationID's
// already-stored values for any field left blank — so "Probar conexión"
// works before hitting Guardar. integrationID is "" when testing a
// not-yet-created integration (ResolveConfig degrades to an all-blank
// config for an unknown ID, which the draft values then fully cover).
func (a *App) TestConnection(providerType, integrationID string, values map[string]string) error {
	p, ok := providers.Get(providerType)
	if !ok {
		return fmt.Errorf("tipo de proveedor desconocido: %s", providerType)
	}

	resolved, err := a.resolveDraftConfig(p, integrationID, values)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return p.TestConnection(ctx, resolved)
}

func (a *App) resolveDraftConfig(p providers.Provider, integrationID string, values map[string]string) (providers.Config, error) {
	resolved, err := store.ResolveConfig(p, integrationID)
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
