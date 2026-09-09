// Package providers defines the pluggable connector interface every task
// source (Redmine, GitHub, ...) implements, plus the unified TaskItem model
// the rest of the app works with regardless of where an item came from.
package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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
	ID          string   `json:"id"`
	Provider    string   `json:"provider"`
	Type        ItemType `json:"type"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Project     string   `json:"project"`
	Status      string   `json:"status"`
	Author      string   `json:"author"`
	Description string   `json:"description"`
	// CreatedByMe reports whether the authenticated user is this item's
	// author/reporter — a best-effort signal computed by comparing the
	// provider's own identity for the item against the current user, not a
	// flag specific to items created through Koalmine's own "new task" form.
	CreatedByMe bool      `json:"createdByMe"`
	UpdatedAt   time.Time `json:"updatedAt"`
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
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	Kind        FieldKind `json:"kind"`
	Placeholder string    `json:"placeholder"`
	Required    bool      `json:"required"`
}

// Config holds the values a provider instance was configured with, keyed by
// ConfigField.Key.
type Config map[string]string

// CreateItemInput holds what's needed to create a new issue on a provider.
// Project identifies where to create it — its exact shape is
// provider-specific (a Redmine project identifier, a GitHub "owner/repo", a
// GitLab project path or numeric ID); ProjectHint on the Provider describes
// the expected shape for the UI.
type CreateItemInput struct {
	Project     string
	Title       string
	Description string
}

// Comment is one comment/note on a TaskItem.
type Comment struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// ProjectOption is one entry in the "new task" form's project dropdown:
// Value is what gets sent back as CreateItemInput.Project, Label is what's
// shown to the user (the same string for GitHub/GitLab, a friendlier
// display name for Redmine).
type ProjectOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

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
	// ProjectHint describes, for the "new task" form, what shape this
	// provider expects CreateItemInput.Project to be in (e.g. "owner/repo").
	ProjectHint() string
	// ListProjects returns the projects/repos the user can create a task in,
	// for the "new task" form's dropdown. May return a short or capped list
	// (e.g. only the user's own repos, or the first page) — the form's free-
	// text fallback covers whatever this doesn't.
	ListProjects(ctx context.Context, cfg Config) ([]ProjectOption, error)
	// CreateItem creates a new issue on the provider, assigned to the
	// authenticated user, and returns it in the unified TaskItem shape.
	CreateItem(ctx context.Context, cfg Config, input CreateItemInput) (TaskItem, error)
	// SearchItems runs a free-text search against the provider itself (not
	// just the locally cached snapshot), so it can surface older or closed
	// items that FetchItems' "currently assigned to me" scope wouldn't.
	SearchItems(ctx context.Context, cfg Config, query string) ([]TaskItem, error)
	// FetchComments returns the comments/notes on the given item, oldest
	// first. It takes the whole TaskItem (not just an id) because different
	// providers need different pieces of it — GitHub needs Project plus the
	// issue number out of URL, GitLab additionally needs Type to know
	// whether it's an issue or a merge request, Redmine only needs ID.
	FetchComments(ctx context.Context, cfg Config, item TaskItem) ([]Comment, error)
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// lastURLSegment returns the final "/"-separated segment of a URL — used to
// pull a numeric issue/MR id out of a TaskItem's web URL when a provider's
// comments API needs it but TaskItem doesn't carry it as its own field.
func lastURLSegment(rawURL string) (string, error) {
	trimmed := strings.TrimRight(rawURL, "/")
	idx := strings.LastIndex(trimmed, "/")
	if idx == -1 || idx == len(trimmed)-1 {
		return "", fmt.Errorf("URL inválida: %q", rawURL)
	}
	return trimmed[idx+1:], nil
}
