package store

import "koalmine/internal/providers"

// ResolveConfig builds a fully-resolved providers.Config for the given
// provider from persisted state: non-secret values from the config file,
// secret values from the OS keychain. Shared by the settings screen (which
// layers unsaved form input on top) and the poller (which uses it as-is).
func ResolveConfig(p providers.Provider, providerName string) (providers.Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	pc := cfg.Providers[providerName]

	resolved := providers.Config{}
	for _, field := range p.ConfigFields() {
		if field.Kind == providers.FieldSecret {
			secret, err := GetSecret(providerName, field.Key)
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
