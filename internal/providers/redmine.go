package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func init() {
	Register("redmine", func() Provider {
		return &redmineProvider{client: defaultHTTPClient()}
	})
}

// redmineProvider only surfaces issues assigned to the current user.
// Redmine's public REST API has no endpoint for "issues mentioning me" and
// no native pull/merge-request concept, so mentions and PR-review items
// (agreed in scope for the app overall) simply don't apply to this
// provider — that's a Redmine limitation, not something left unimplemented.
type redmineProvider struct {
	client *http.Client
}

func (p *redmineProvider) Name() string        { return "redmine" }
func (p *redmineProvider) DisplayName() string { return "Redmine" }

func (p *redmineProvider) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "base_url", Label: "URL del servidor", Kind: FieldURL, Placeholder: "https://redmine.miempresa.com", Required: true},
		{Key: "api_key", Label: "API Key", Kind: FieldSecret, Required: true},
	}
}

func (p *redmineProvider) TestConnection(ctx context.Context, cfg Config) error {
	req, err := p.newRequest(ctx, cfg, "/users/current.json")
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("no se pudo conectar a Redmine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Redmine respondió %s", resp.Status)
	}
	return nil
}

func (p *redmineProvider) FetchItems(ctx context.Context, cfg Config) ([]TaskItem, error) {
	req, err := p.newRequest(ctx, cfg, "/issues.json?assigned_to_id=me&status_id=open&limit=100&sort=updated_on:desc")
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a Redmine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Redmine respondió %s", resp.Status)
	}

	var parsed redmineIssuesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("respuesta inválida de Redmine: %w", err)
	}

	baseURL := strings.TrimRight(cfg["base_url"], "/")
	items := make([]TaskItem, 0, len(parsed.Issues))
	for _, issue := range parsed.Issues {
		updatedAt, _ := time.Parse(time.RFC3339, issue.UpdatedOn)
		items = append(items, TaskItem{
			ID:        fmt.Sprintf("redmine:%d", issue.ID),
			Provider:  "redmine",
			Type:      ItemTypeIssue,
			Title:     issue.Subject,
			URL:       fmt.Sprintf("%s/issues/%d", baseURL, issue.ID),
			Project:   issue.Project.Name,
			Status:    issue.Status.Name,
			Author:    issue.Author.Name,
			UpdatedAt: updatedAt,
		})
	}
	return items, nil
}

func (p *redmineProvider) newRequest(ctx context.Context, cfg Config, path string) (*http.Request, error) {
	baseURL := strings.TrimRight(cfg["base_url"], "/")
	if baseURL == "" {
		return nil, fmt.Errorf("falta la URL del servidor Redmine")
	}
	if cfg["api_key"] == "" {
		return nil, fmt.Errorf("falta la API key de Redmine")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Redmine-API-Key", cfg["api_key"])
	req.Header.Set("Accept", "application/json")
	return req, nil
}

type redmineIssuesResponse struct {
	Issues []redmineIssue `json:"issues"`
}

type redmineIssue struct {
	ID        int    `json:"id"`
	Subject   string `json:"subject"`
	UpdatedOn string `json:"updated_on"`
	Project   struct {
		Name string `json:"name"`
	} `json:"project"`
	Status struct {
		Name string `json:"name"`
	} `json:"status"`
	Author struct {
		Name string `json:"name"`
	} `json:"author"`
}
