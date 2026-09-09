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

const gitlabAPIBase = "https://gitlab.com/api/v4"

func init() {
	Register("gitlab", func() Provider {
		return &gitlabProvider{client: defaultHTTPClient(), apiBase: gitlabAPIBase}
	})
}

// gitlabProvider covers issues assigned to the user and merge requests
// assigned to them or where they're a requested reviewer. GitLab's REST API
// has no unified "mentioned me" search across issues/MRs (unlike GitHub's
// mentions:@me), so mentions simply don't apply here — the same kind of gap
// as Redmine, for a different reason.
type gitlabProvider struct {
	client  *http.Client
	apiBase string // overridable in tests; real usage always hits gitlabAPIBase (or a self-hosted base_url)
}

func (p *gitlabProvider) Name() string        { return "gitlab" }
func (p *gitlabProvider) DisplayName() string { return "GitLab" }

func (p *gitlabProvider) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "base_url", Label: "URL de la instancia (vacío = gitlab.com)", Kind: FieldURL, Placeholder: "https://gitlab.miempresa.com"},
		{Key: "token", Label: "Personal Access Token", Kind: FieldSecret, Required: true},
	}
}

func (p *gitlabProvider) ProjectHint() string {
	return "namespace/proyecto o ID numérico (ej: grupo/proyecto)"
}

func (p *gitlabProvider) TestConnection(ctx context.Context, cfg Config) error {
	_, err := p.currentUser(ctx, cfg)
	return err
}

// FetchItems merges three lists — issues assigned to me, MRs assigned to
// me, and MRs where I'm a requested reviewer — de-duplicating an MR that
// shows up in both of the latter two.
func (p *gitlabProvider) FetchItems(ctx context.Context, cfg Config) ([]TaskItem, error) {
	me, err := p.currentUser(ctx, cfg)
	if err != nil {
		return nil, err
	}

	issues, err := p.list(ctx, cfg, "/issues?scope=assigned_to_me&state=opened&per_page=100", ItemTypeIssue)
	if err != nil {
		return nil, err
	}
	assignedMRs, err := p.list(ctx, cfg, "/merge_requests?scope=assigned_to_me&state=opened&per_page=100", ItemTypePR)
	if err != nil {
		return nil, err
	}
	reviewMRs, err := p.list(ctx, cfg, fmt.Sprintf("/merge_requests?scope=all&state=opened&reviewer_id=%d&per_page=100", me.ID), ItemTypePR)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(issues)+len(assignedMRs)+len(reviewMRs))
	items := make([]TaskItem, 0, len(issues)+len(assignedMRs)+len(reviewMRs))
	for _, group := range [][]TaskItem{issues, assignedMRs, reviewMRs} {
		for _, item := range group {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			item.CreatedByMe = item.Author == me.Username
			items = append(items, item)
		}
	}
	return items, nil
}

// ListProjects returns the projects the user is a member of, for the "new
// task" form's project dropdown. Capped at the first 100 (sorted by most
// recent activity) — the form's free-text fallback covers anything beyond
// that.
func (p *gitlabProvider) ListProjects(ctx context.Context, cfg Config) ([]ProjectOption, error) {
	path := "/projects?membership=true&per_page=100&order_by=last_activity_at&simple=true"
	req, err := p.newRequest(ctx, cfg, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a GitLab: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab respondió %s", resp.Status)
	}

	var repos []struct {
		PathWithNamespace string `json:"path_with_namespace"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("respuesta inválida de GitLab: %w", err)
	}

	options := make([]ProjectOption, 0, len(repos))
	for _, r := range repos {
		options = append(options, ProjectOption{Value: r.PathWithNamespace, Label: r.PathWithNamespace})
	}
	return options, nil
}

// SearchItems runs a free-text search across every issue and merge request
// the user has access to — open or closed — unlike FetchItems, which only
// covers currently-open items assigned to (or awaiting review from) them.
func (p *gitlabProvider) SearchItems(ctx context.Context, cfg Config, query string) ([]TaskItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	me, err := p.currentUser(ctx, cfg)
	if err != nil {
		return nil, err
	}

	issues, err := p.searchScope(ctx, cfg, "issues", query, ItemTypeIssue)
	if err != nil {
		return nil, err
	}
	mrs, err := p.searchScope(ctx, cfg, "merge_requests", query, ItemTypePR)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(issues)+len(mrs))
	items := make([]TaskItem, 0, len(issues)+len(mrs))
	for _, group := range [][]TaskItem{issues, mrs} {
		for _, item := range group {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			item.CreatedByMe = item.Author == me.Username
			items = append(items, item)
		}
	}
	return items, nil
}

func (p *gitlabProvider) searchScope(ctx context.Context, cfg Config, scope, query string, itemType ItemType) ([]TaskItem, error) {
	path := "/search?scope=" + scope + "&search=" + url.QueryEscape(query)
	req, err := p.newRequest(ctx, cfg, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a GitLab: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab respondió %s", resp.Status)
	}

	var raw []gitlabItem
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("respuesta inválida de GitLab: %w", err)
	}

	items := make([]TaskItem, 0, len(raw))
	for _, it := range raw {
		items = append(items, gitlabToTaskItem(it, itemType))
	}
	return items, nil
}

type gitlabUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

func (p *gitlabProvider) currentUser(ctx context.Context, cfg Config) (gitlabUser, error) {
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/user", nil)
	if err != nil {
		return gitlabUser{}, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return gitlabUser{}, fmt.Errorf("no se pudo conectar a GitLab: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return gitlabUser{}, fmt.Errorf("GitLab respondió %s", resp.Status)
	}

	var user gitlabUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return gitlabUser{}, fmt.Errorf("respuesta inválida de GitLab: %w", err)
	}
	return user, nil
}

func (p *gitlabProvider) list(ctx context.Context, cfg Config, path string, itemType ItemType) ([]TaskItem, error) {
	req, err := p.newRequest(ctx, cfg, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a GitLab: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab respondió %s", resp.Status)
	}

	var raw []gitlabItem
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("respuesta inválida de GitLab: %w", err)
	}

	items := make([]TaskItem, 0, len(raw))
	for _, it := range raw {
		items = append(items, gitlabToTaskItem(it, itemType))
	}
	return items, nil
}

// FetchComments returns an issue or MR's non-system notes — GitLab's notes
// API also includes auto-generated system notes (label changes, status
// changes, ...), which aren't comments and are skipped here.
func (p *gitlabProvider) FetchComments(ctx context.Context, cfg Config, item TaskItem) ([]Comment, error) {
	iid, err := lastURLSegment(item.URL)
	if err != nil {
		return nil, err
	}

	resource := "issues"
	if item.Type == ItemTypePR {
		resource = "merge_requests"
	}

	path := fmt.Sprintf("/projects/%s/%s/%s/notes", url.PathEscape(item.Project), resource, iid)
	req, err := p.newRequest(ctx, cfg, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a GitLab: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab respondió %s", resp.Status)
	}

	var raw []struct {
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		System    bool   `json:"system"`
		Author    struct {
			Username string `json:"username"`
		} `json:"author"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("respuesta inválida de GitLab: %w", err)
	}

	comments := make([]Comment, 0, len(raw))
	for _, n := range raw {
		if n.System {
			continue
		}
		createdAt, _ := time.Parse(time.RFC3339, n.CreatedAt)
		comments = append(comments, Comment{Author: n.Author.Username, Body: n.Body, CreatedAt: createdAt})
	}
	return comments, nil
}

// CreateItem creates a GitLab issue under the given project path/ID,
// assigned to the authenticated user so it shows up on the next poll.
func (p *gitlabProvider) CreateItem(ctx context.Context, cfg Config, input CreateItemInput) (TaskItem, error) {
	if input.Project == "" {
		return TaskItem{}, fmt.Errorf("falta el proyecto")
	}
	if input.Title == "" {
		return TaskItem{}, fmt.Errorf("falta el título")
	}

	me, err := p.currentUser(ctx, cfg)
	if err != nil {
		return TaskItem{}, err
	}

	payload, err := json.Marshal(map[string]any{
		"title":        input.Title,
		"description":  input.Description,
		"assignee_ids": []int{me.ID},
	})
	if err != nil {
		return TaskItem{}, err
	}

	path := "/projects/" + url.PathEscape(input.Project) + "/issues"
	req, err := p.newRequest(ctx, cfg, http.MethodPost, path, bytes.NewReader(payload))
	if err != nil {
		return TaskItem{}, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return TaskItem{}, fmt.Errorf("no se pudo conectar a GitLab: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return TaskItem{}, fmt.Errorf("GitLab respondió %s al crear la tarea", resp.Status)
	}

	var it gitlabItem
	if err := json.NewDecoder(resp.Body).Decode(&it); err != nil {
		return TaskItem{}, fmt.Errorf("respuesta inválida de GitLab: %w", err)
	}
	item := gitlabToTaskItem(it, ItemTypeIssue)
	item.CreatedByMe = true
	return item, nil
}

func gitlabToTaskItem(it gitlabItem, itemType ItemType) TaskItem {
	updatedAt, _ := time.Parse(time.RFC3339, it.UpdatedAt)
	return TaskItem{
		// Issues and merge requests have independent ID sequences in
		// GitLab's API, so the item type must be part of the key —
		// otherwise an issue and an MR that happen to share a numeric
		// ID would collide.
		ID:          fmt.Sprintf("gitlab:%s:%d", itemType, it.ID),
		Provider:    "gitlab",
		Type:        itemType,
		Title:       it.Title,
		URL:         it.WebURL,
		Project:     projectFromReference(it.References.Full),
		Status:      it.State,
		Author:      it.Author.Username,
		Description: it.Description,
		UpdatedAt:   updatedAt,
	}
}

func (p *gitlabProvider) newRequest(ctx context.Context, cfg Config, method, path string, body io.Reader) (*http.Request, error) {
	if cfg["token"] == "" {
		return nil, fmt.Errorf("falta el Personal Access Token de GitLab")
	}

	baseURL := p.apiBase
	if override := strings.TrimRight(cfg["base_url"], "/"); override != "" {
		baseURL = override + "/api/v4"
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", cfg["token"])
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// projectFromReference turns a GitLab "full" reference like
// "group/project#42" or "group/project!7" into just "group/project".
func projectFromReference(full string) string {
	if idx := strings.LastIndexAny(full, "#!"); idx != -1 {
		return full[:idx]
	}
	return full
}

type gitlabItem struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	WebURL      string `json:"web_url"`
	State       string `json:"state"`
	UpdatedAt   string `json:"updated_at"`
	Author      struct {
		Username string `json:"username"`
	} `json:"author"`
	References struct {
		Full string `json:"full"`
	} `json:"references"`
}
