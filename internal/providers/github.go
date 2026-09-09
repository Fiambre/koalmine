package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func (p *githubProvider) ProjectHint() string {
	return "owner/repo (ej: octocat/Hello-World)"
}

func (p *githubProvider) TestConnection(ctx context.Context, cfg Config) error {
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/user", nil)
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
	req, err := p.newRequest(ctx, cfg, http.MethodGet, path, nil)
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
		resolvedType := itemType
		if issue.PullRequest != nil && itemType == ItemTypeIssue {
			resolvedType = ItemTypePR
		}
		items = append(items, githubToTaskItem(issue, resolvedType))
	}
	return items, nil
}

// CreateItem creates a GitHub issue under the given "owner/repo", assigned
// to the authenticated user so it shows up on the next poll.
func (p *githubProvider) CreateItem(ctx context.Context, cfg Config, input CreateItemInput) (TaskItem, error) {
	owner, repo, ok := strings.Cut(input.Project, "/")
	if !ok || owner == "" || repo == "" {
		return TaskItem{}, fmt.Errorf("el proyecto debe tener el formato owner/repo")
	}
	if input.Title == "" {
		return TaskItem{}, fmt.Errorf("falta el título")
	}

	login, err := p.currentLogin(ctx, cfg)
	if err != nil {
		return TaskItem{}, err
	}

	payload, err := json.Marshal(map[string]any{
		"title":     input.Title,
		"body":      input.Description,
		"assignees": []string{login},
	})
	if err != nil {
		return TaskItem{}, err
	}

	req, err := p.newRequest(ctx, cfg, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues", owner, repo), bytes.NewReader(payload))
	if err != nil {
		return TaskItem{}, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return TaskItem{}, fmt.Errorf("no se pudo conectar a GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return TaskItem{}, fmt.Errorf("GitHub respondió %s al crear la tarea", resp.Status)
	}

	var issue githubIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return TaskItem{}, fmt.Errorf("respuesta inválida de GitHub: %w", err)
	}
	return githubToTaskItem(issue, ItemTypeIssue), nil
}

func (p *githubProvider) currentLogin(ctx context.Context, cfg Config) (string, error) {
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/user", nil)
	if err != nil {
		return "", err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("no se pudo conectar a GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub respondió %s", resp.Status)
	}

	var parsed struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("respuesta inválida de GitHub: %w", err)
	}
	return parsed.Login, nil
}

func githubToTaskItem(issue githubIssue, itemType ItemType) TaskItem {
	updatedAt, _ := time.Parse(time.RFC3339, issue.UpdatedAt)
	return TaskItem{
		ID:          fmt.Sprintf("github:%d", issue.ID),
		Provider:    "github",
		Type:        itemType,
		Title:       issue.Title,
		URL:         issue.HTMLURL,
		Project:     repoNameFromURL(issue.RepositoryURL),
		Status:      issue.State,
		Author:      issue.User.Login,
		Description: issue.Body,
		UpdatedAt:   updatedAt,
	}
}

func (p *githubProvider) newRequest(ctx context.Context, cfg Config, method, path string, body io.Reader) (*http.Request, error) {
	if cfg["token"] == "" {
		return nil, fmt.Errorf("falta el Personal Access Token de GitHub")
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg["token"])
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "koalmine")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
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
	Body          string `json:"body"`
	HTMLURL       string `json:"html_url"`
	State         string `json:"state"`
	UpdatedAt     string `json:"updated_at"`
	RepositoryURL string `json:"repository_url"`
	User          struct {
		Login string `json:"login"`
	} `json:"user"`
	PullRequest *struct{} `json:"pull_request,omitempty"`
}
