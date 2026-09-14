package store

import "koalmine/internal/providers"

// ResolveConfig builds a fully-resolved providers.Config for one configured
// integration: non-secret values from the config file, secret values from
// the OS keychain. Shared by the settings screen (which layers unsaved
// form input on top) and the poller/app (which use it as-is). An unknown
// integrationID (including "", for a not-yet-created integration being
// tested) resolves to an all-blank Config rather than an error — the
// caller is expected to overlay real values on top either way.
func ResolveConfig(p providers.Provider, integrationID string) (providers.Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	integ, _ := cfg.IntegrationByID(integrationID)

	resolved := providers.Config{}
	for _, field := range p.ConfigFields() {
		if field.Kind == providers.FieldSecret {
			secret, err := GetSecret(integrationID, field.Key)
			if err != nil {
				return nil, err
			}
			resolved[field.Key] = secret
		} else {
			resolved[field.Key] = integ.Values[field.Key]
		}
	}
	return resolved, nil
}
