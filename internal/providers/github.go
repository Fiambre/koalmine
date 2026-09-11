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

	// Best-effort: if this fails, CreatedByMe just stays false for every
	// item rather than failing the whole fetch over a secondary field.
	login, _ := p.currentLogin(ctx, cfg)
	if login != "" {
		for i := range items {
			items[i].CreatedByMe = items[i].Author == login
		}
	}
	return items, nil
}

func (p *githubProvider) search(ctx context.Context, cfg Config, query string, itemType ItemType) ([]TaskItem, error) {
	issues, err := p.rawSearch(ctx, cfg, query)
	if err != nil {
		return nil, err
	}

	items := make([]TaskItem, 0, len(issues))
	for _, issue := range issues {
		resolvedType := itemType
		if issue.PullRequest != nil && itemType == ItemTypeIssue {
			resolvedType = ItemTypePR
		}
		items = append(items, githubToTaskItem(issue, resolvedType))
	}
	return items, nil
}

// SearchItems runs a free-text search across every issue/PR the user is
// involved in (author, assignee, mentioned, or commenter) — open or closed
// — unlike FetchItems, which only covers currently-open, currently-assigned
// items.
func (p *githubProvider) SearchItems(ctx context.Context, cfg Config, query string) ([]TaskItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	login, _ := p.currentLogin(ctx, cfg)

	issues, err := p.rawSearch(ctx, cfg, query+" involves:@me")
	if err != nil {
		return nil, err
	}

	items := make([]TaskItem, 0, len(issues))
	for _, issue := range issues {
		itemType := ItemTypeIssue
		if issue.PullRequest != nil {
			itemType = ItemTypePR
		}
		item := githubToTaskItem(issue, itemType)
		item.CreatedByMe = login != "" && item.Author == login
		items = append(items, item)
	}
	return items, nil
}

// ListProjects returns the repos the user owns, collaborates on, or belongs
// to via an organization, for the "new task" form's project dropdown. Capped
// at the first 100 (sorted by most recently updated) — the form's free-text
// fallback covers anything beyond that.
func (p *githubProvider) ListProjects(ctx context.Context, cfg Config) ([]ProjectOption, error) {
	path := "/user/repos?per_page=100&sort=updated&affiliation=owner,collaborator,organization_member"
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

	var repos []struct {
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("respuesta inválida de GitHub: %w", err)
	}

	options := make([]ProjectOption, 0, len(repos))
	for _, r := range repos {
		options = append(options, ProjectOption{Value: r.FullName, Label: r.FullName})
	}
	return options, nil
}

// FetchComments returns an issue or PR's conversation comments. GitHub
// serves these through the same /issues/{number}/comments endpoint for
// both (review comments on specific diff lines are a separate endpoint,
// not covered here).
func (p *githubProvider) FetchComments(ctx context.Context, cfg Config, item TaskItem) ([]Comment, error) {
	number, err := lastURLSegment(item.URL)
	if err != nil {
		return nil, err
	}

	req, err := p.newRequest(ctx, cfg, http.MethodGet, fmt.Sprintf("/repos/%s/issues/%s/comments", item.Project, number), nil)
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

	var raw []struct {
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("respuesta inválida de GitHub: %w", err)
	}

	comments := make([]Comment, 0, len(raw))
	for _, c := range raw {
		createdAt, _ := time.Parse(time.RFC3339, c.CreatedAt)
		comments = append(comments, Comment{Author: c.User.Login, Body: c.Body, CreatedAt: createdAt})
	}
	return comments, nil
}

// FetchItem re-fetches a single issue or PR's current data — see
// Provider.FetchItem. Type is kept from the passed-in item rather than
// re-derived (beyond upgrading issue→pr if the API says so): GitHub itself
// has no "mention" concept, so a starred mention would otherwise silently
// flip to a plain issue on refresh.
func (p *githubProvider) FetchItem(ctx context.Context, cfg Config, item TaskItem) (TaskItem, error) {
	number, err := lastURLSegment(item.URL)
	if err != nil {
		return TaskItem{}, err
	}

	req, err := p.newRequest(ctx, cfg, http.MethodGet, fmt.Sprintf("/repos/%s/issues/%s", item.Project, number), nil)
	if err != nil {
		return TaskItem{}, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return TaskItem{}, fmt.Errorf("no se pudo conectar a GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TaskItem{}, fmt.Errorf("GitHub respondió %s", resp.Status)
	}

	var issue githubIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return TaskItem{}, fmt.Errorf("respuesta inválida de GitHub: %w", err)
	}

	itemType := item.Type
	if issue.PullRequest != nil {
		itemType = ItemTypePR
	}
	fresh := githubToTaskItem(issue, itemType)
	login, _ := p.currentLogin(ctx, cfg)
	fresh.CreatedByMe = login != "" && fresh.Author == login
	return fresh, nil
}

func (p *githubProvider) rawSearch(ctx context.Context, cfg Config, query string) ([]githubIssue, error) {
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
	return parsed.Items, nil
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
	item := githubToTaskItem(issue, ItemTypeIssue)
	item.CreatedByMe = true
	return item, nil
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
