package providers

import "sort"

// Factory constructs a new Provider instance.
type Factory func() Provider

var registry = map[string]Factory{}

// Register adds a provider factory to the catalog. Each provider file
// calls this from its own init().
func Register(name string, factory Factory) {
	registry[name] = factory
}

// Get returns a new instance of the named provider, or ok=false if unknown.
func Get(name string) (provider Provider, ok bool) {
	factory, ok := registry[name]
	if !ok {
		return nil, false
	}
	return factory(), true
}

// List returns a fresh instance of every registered provider, sorted by
// name, for the settings UI to enumerate.
func List() []Provider {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)

	list := make([]Provider, 0, len(names))
	for _, name := range names {
		list = append(list, registry[name]())
	}
	return list
}
