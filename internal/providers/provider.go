// Package providers defines the pluggable connector interface every task
// source (Redmine, GitHub, ...) implements, plus the unified TaskItem model
// the rest of the app works with regardless of where an item came from.
package providers

import (
	"context"
	"net/http"
	"time"
)

// ItemType categorizes a TaskItem.
type ItemType string

const (
	ItemTypeIssue   ItemType = "issue"
	ItemTypePR      ItemType = "pr"
	ItemTypeMention ItemType = "mention"
)

// TaskItem is the unified representation of a task/issue/PR/mention across
// every provider.
type TaskItem struct {
	// ID is stable and unique within its provider; used for notification dedup.
	ID        string
	Provider  string
	Type      ItemType
	Title     string
	URL       string
	Project   string
	Status    string
	Author    string
	UpdatedAt time.Time
}

// FieldKind describes how a ConfigField's value should be captured and rendered.
type FieldKind string

const (
	FieldText   FieldKind = "text"
	FieldURL    FieldKind = "url"
	FieldSecret FieldKind = "secret" // rendered as a password input, stored in the OS keychain
)

// ConfigField describes one configuration value a provider needs (e.g. base
// URL, API key), so the settings UI can render a form for it generically
// instead of every provider needing bespoke UI.
type ConfigField struct {
	Key         string
	Label       string
	Kind        FieldKind
	Placeholder string
	Required    bool
}

// Config holds the values a provider instance was configured with, keyed by
// ConfigField.Key.
type Config map[string]string

// Provider is implemented by every task-source connector. Each
// implementation self-registers into the package Registry from its own
// init(), so adding a new connector never requires touching this file or
// the rest of the app.
type Provider interface {
	// Name is the stable identifier used in config/registry/UI, e.g. "redmine".
	Name() string
	// DisplayName is the human-readable name shown in the UI, e.g. "Redmine".
	DisplayName() string
	// ConfigFields describes what this provider needs to be configured.
	ConfigFields() []ConfigField
	// TestConnection verifies the given config can authenticate against the provider.
	TestConnection(ctx context.Context, cfg Config) error
	// FetchItems returns the caller's current assigned issues/work items,
	// PRs/MRs where they're reviewer, and mentions.
	FetchItems(ctx context.Context, cfg Config) ([]TaskItem, error)
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}
