package main

import (
	"context"
	"fmt"
	"time"

	"koalmine/internal/providers"
	"koalmine/internal/store"
)

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
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
	cfg, err := store.Load()
	if err != nil {
		return nil, err
	}
	pc := cfg.Providers[providerName]

	resolved := providers.Config{}
	for _, field := range p.ConfigFields() {
		if v, ok := values[field.Key]; ok && v != "" {
			resolved[field.Key] = v
			continue
		}
		if field.Kind == providers.FieldSecret {
			secret, err := store.GetSecret(providerName, field.Key)
			if err != nil {
				return nil, err
			}
			resolved[field.Key] = secret
		} else {
			resolved[field.Key] = pc.Values[field.Key]
		}
	}
	return resolved, nil
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
