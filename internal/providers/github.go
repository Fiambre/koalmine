package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const githubAPIBase = "https://api.github.com"

func init() {
	Register("github", func() Provider {
		return &githubProvider{client: defaultHTTPClient(), baseURL: githubAPIBase}
	})
}

type githubProvider struct {
	client  *http.Client
	baseURL string // overridable in tests; real usage always hits githubAPIBase
}

func (p *githubProvider) Name() string        { return "github" }
func (p *githubProvider) DisplayName() string { return "GitHub" }

func (p *githubProvider) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "token", Label: "Personal Access Token", Kind: FieldSecret, Required: true},
	}
}

func (p *githubProvider) TestConnection(ctx context.Context, cfg Config) error {
	req, err := p.newRequest(ctx, cfg, "/user")
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("no se pudo conectar a GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub respondió %s", resp.Status)
	}
	return nil
}

// FetchItems merges three searches — assigned issues, PRs where the user is
// a requested reviewer, and mentions — de-duplicating items that match more
// than one (an item keeps whichever type it was first seen as, in that
// priority order).
func (p *githubProvider) FetchItems(ctx context.Context, cfg Config) ([]TaskItem, error) {
	assigned, err := p.search(ctx, cfg, "is:open assignee:@me", ItemTypeIssue)
	if err != nil {
		return nil, err
	}
	reviewRequested, err := p.search(ctx, cfg, "is:open is:pr review-requested:@me", ItemTypePR)
	if err != nil {
		return nil, err
	}
	mentioned, err := p.search(ctx, cfg, "is:open mentions:@me", ItemTypeMention)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(assigned)+len(reviewRequested)+len(mentioned))
	items := make([]TaskItem, 0, len(assigned)+len(reviewRequested)+len(mentioned))
	for _, group := range [][]TaskItem{assigned, reviewRequested, mentioned} {
		for _, item := range group {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			items = append(items, item)
		}
	}
	return items, nil
}

func (p *githubProvider) search(ctx context.Context, cfg Config, query string, itemType ItemType) ([]TaskItem, error) {
	path := "/search/issues?q=" + url.QueryEscape(query) + "&per_page=50&sort=updated&order=desc"
	req, err := p.newRequest(ctx, cfg, path)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub respondió %s", resp.Status)
	}

	var parsed githubSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("respuesta inválida de GitHub: %w", err)
	}

	items := make([]TaskItem, 0, len(parsed.Items))
	for _, issue := range parsed.Items {
		updatedAt, _ := time.Parse(time.RFC3339, issue.UpdatedAt)

		resolvedType := itemType
		if issue.PullRequest != nil && itemType == ItemTypeIssue {
			resolvedType = ItemTypePR
		}

		items = append(items, TaskItem{
			ID:        fmt.Sprintf("github:%d", issue.ID),
			Provider:  "github",
			Type:      resolvedType,
			Title:     issue.Title,
			URL:       issue.HTMLURL,
			Project:   repoNameFromURL(issue.RepositoryURL),
			Status:    issue.State,
			Author:    issue.User.Login,
			UpdatedAt: updatedAt,
		})
	}
	return items, nil
}

func (p *githubProvider) newRequest(ctx context.Context, cfg Config, path string) (*http.Request, error) {
	if cfg["token"] == "" {
		return nil, fmt.Errorf("falta el Personal Access Token de GitHub")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg["token"])
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "koalmine")
	return req, nil
}

func repoNameFromURL(repositoryURL string) string {
	parts := strings.Split(strings.TrimRight(repositoryURL, "/"), "/")
	if len(parts) < 2 {
		return repositoryURL
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

type githubSearchResponse struct {
	Items []githubIssue `json:"items"`
}

type githubIssue struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	HTMLURL       string `json:"html_url"`
	State         string `json:"state"`
	UpdatedAt     string `json:"updated_at"`
	RepositoryURL string `json:"repository_url"`
	User          struct {
		Login string `json:"login"`
	} `json:"user"`
	PullRequest *struct{} `json:"pull_request,omitempty"`
}
