package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

func (p *redmineProvider) ProjectHint() string {
	return "Identificador del proyecto en Redmine (ej: mi-proyecto)"
}

func (p *redmineProvider) TestConnection(ctx context.Context, cfg Config) error {
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/users/current.json", nil)
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
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/issues.json?assigned_to_id=me&status_id=open&limit=100&sort=updated_on:desc", nil)
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

	// Best-effort: if this fails, CreatedByMe just stays false for every
	// item rather than failing the whole fetch over a secondary field.
	userID, _ := p.currentUserID(ctx, cfg)

	baseURL := strings.TrimRight(cfg["base_url"], "/")
	items := make([]TaskItem, 0, len(parsed.Issues))
	for _, issue := range parsed.Issues {
		item := redmineToTaskItem(issue, baseURL)
		item.CreatedByMe = userID != 0 && issue.Author.ID == userID
		items = append(items, item)
	}
	return items, nil
}

// ListProjects returns every project the API key's user has access to, for
// the "new task" form's project dropdown.
func (p *redmineProvider) ListProjects(ctx context.Context, cfg Config) ([]ProjectOption, error) {
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/projects.json?limit=100", nil)
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

	var parsed struct {
		Projects []struct {
			Identifier string `json:"identifier"`
			Name       string `json:"name"`
		} `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("respuesta inválida de Redmine: %w", err)
	}

	options := make([]ProjectOption, 0, len(parsed.Projects))
	for _, pr := range parsed.Projects {
		options = append(options, ProjectOption{Value: pr.Identifier, Label: pr.Name})
	}
	return options, nil
}

// SearchItems runs a full-text search across every issue the user has
// access to (not just ones assigned to them), via Redmine's own search
// endpoint. Redmine's search results carry only id/title/url/description —
// no project, status or author — so those get backfilled below with one
// bulk issue lookup (matching what FetchItems returns), rather than left
// blank the way they'd otherwise show up in the UI (e.g. the "Seguimiento"
// table's Proyecto/Estado columns).
func (p *redmineProvider) SearchItems(ctx context.Context, cfg Config, query string) ([]TaskItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	// Redmine's full-text search indexes issue subjects/descriptions, not
	// issue numbers — searching for a bare ticket number like "12345"
	// (optionally "#12345") returns nothing unless that digit string also
	// happens to appear as text. Fetch the issue directly by ID first for
	// that case rather than relying on /search.json to find it.
	if id, ok := parseRedmineIssueID(query); ok {
		item, found, err := p.fetchIssueByID(ctx, cfg, id)
		if err != nil {
			return nil, err
		}
		if found {
			return []TaskItem{item}, nil
		}
	}

	path := "/search.json?q=" + url.QueryEscape(query) + "&issues=1&limit=25"
	req, err := p.newRequest(ctx, cfg, http.MethodGet, path, nil)
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

	var parsed struct {
		Results []struct {
			ID          int    `json:"id"`
			Title       string `json:"title"`
			Type        string `json:"type"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Datetime    string `json:"datetime"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("respuesta inválida de Redmine: %w", err)
	}

	var issueIDs []int
	for _, r := range parsed.Results {
		if r.Type == "issue" {
			issueIDs = append(issueIDs, r.ID)
		}
	}
	// Best-effort: if the backfill fails, results still come back with
	// their bare search fields rather than failing the whole search over
	// an enrichment step.
	var full map[int]redmineIssue
	var userID int
	if len(issueIDs) > 0 {
		full, _ = p.fetchIssuesByIDs(ctx, cfg, issueIDs)
		userID, _ = p.currentUserID(ctx, cfg)
	}

	baseURL := strings.TrimRight(cfg["base_url"], "/")
	items := make([]TaskItem, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		if r.Type != "issue" {
			continue
		}
		if issue, ok := full[r.ID]; ok {
			item := redmineToTaskItem(issue, baseURL)
			item.CreatedByMe = userID != 0 && issue.Author.ID == userID
			items = append(items, item)
			continue
		}
		updatedAt, _ := time.Parse(time.RFC3339, r.Datetime)
		items = append(items, TaskItem{
			ID:          fmt.Sprintf("redmine:%d", r.ID),
			Provider:    "redmine",
			Type:        ItemTypeIssue,
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
			UpdatedAt:   updatedAt,
		})
	}
	return items, nil
}

// parseRedmineIssueID recognizes a query that's just a ticket number,
// optionally prefixed with "#" (how users typically write a Redmine
// reference, e.g. "#12345").
func parseRedmineIssueID(query string) (int, bool) {
	digits := strings.TrimPrefix(query, "#")
	if digits == "" {
		return 0, false
	}
	id, err := strconv.Atoi(digits)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// fetchIssueByID looks up a single issue directly. found is false (with a
// nil error) when Redmine returns 404 — an ID that doesn't exist, or one
// outside the API key's access — so the caller can fall back to a regular
// text search instead of treating it as a hard failure.
func (p *redmineProvider) fetchIssueByID(ctx context.Context, cfg Config, id int) (TaskItem, bool, error) {
	req, err := p.newRequest(ctx, cfg, http.MethodGet, fmt.Sprintf("/issues/%d.json", id), nil)
	if err != nil {
		return TaskItem{}, false, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return TaskItem{}, false, fmt.Errorf("no se pudo conectar a Redmine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return TaskItem{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return TaskItem{}, false, fmt.Errorf("Redmine respondió %s", resp.Status)
	}

	var parsed struct {
		Issue redmineIssue `json:"issue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return TaskItem{}, false, fmt.Errorf("respuesta inválida de Redmine: %w", err)
	}

	baseURL := strings.TrimRight(cfg["base_url"], "/")
	return redmineToTaskItem(parsed.Issue, baseURL), true, nil
}

// fetchIssuesByIDs fetches full issue data for a batch of IDs in one
// request, keyed by ID. status_id=* is required because /issues.json only
// returns open issues by default, and a matched issue may be closed.
func (p *redmineProvider) fetchIssuesByIDs(ctx context.Context, cfg Config, ids []int) (map[int]redmineIssue, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	strIDs := make([]string, len(ids))
	for i, id := range ids {
		strIDs[i] = strconv.Itoa(id)
	}
	path := fmt.Sprintf("/issues.json?issue_id=%s&status_id=*&limit=%d", strings.Join(strIDs, ","), len(ids))

	req, err := p.newRequest(ctx, cfg, http.MethodGet, path, nil)
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

	byID := make(map[int]redmineIssue, len(parsed.Issues))
	for _, issue := range parsed.Issues {
		byID[issue.ID] = issue
	}
	return byID, nil
}

// FetchComments returns an issue's journal entries that have actual notes
// text — Redmine's journals also include pure field-change entries (status
// changed, assignee changed, ...) with empty notes, which aren't comments
// and are skipped here.
func (p *redmineProvider) FetchComments(ctx context.Context, cfg Config, item TaskItem) ([]Comment, error) {
	id := strings.TrimPrefix(item.ID, "redmine:")

	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/issues/"+id+".json?include=journals", nil)
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

	var parsed struct {
		Issue struct {
			Journals []struct {
				Notes     string `json:"notes"`
				CreatedOn string `json:"created_on"`
				User      struct {
					Name string `json:"name"`
				} `json:"user"`
			} `json:"journals"`
		} `json:"issue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("respuesta inválida de Redmine: %w", err)
	}

	comments := make([]Comment, 0, len(parsed.Issue.Journals))
	for _, j := range parsed.Issue.Journals {
		if strings.TrimSpace(j.Notes) == "" {
			continue
		}
		createdAt, _ := time.Parse(time.RFC3339, j.CreatedOn)
		comments = append(comments, Comment{Author: j.User.Name, Body: j.Notes, CreatedAt: createdAt})
	}
	return comments, nil
}

// CreateItem creates a Redmine issue under the given project identifier,
// assigned to the authenticated user so it shows up on the next poll.
func (p *redmineProvider) CreateItem(ctx context.Context, cfg Config, input CreateItemInput) (TaskItem, error) {
	if input.Project == "" {
		return TaskItem{}, fmt.Errorf("falta el proyecto")
	}
	if input.Title == "" {
		return TaskItem{}, fmt.Errorf("falta el título")
	}

	userID, err := p.currentUserID(ctx, cfg)
	if err != nil {
		return TaskItem{}, err
	}

	payload, err := json.Marshal(map[string]any{
		"issue": map[string]any{
			"project_id":     input.Project,
			"subject":        input.Title,
			"description":    input.Description,
			"assigned_to_id": userID,
		},
	})
	if err != nil {
		return TaskItem{}, err
	}

	req, err := p.newRequest(ctx, cfg, http.MethodPost, "/issues.json", bytes.NewReader(payload))
	if err != nil {
		return TaskItem{}, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return TaskItem{}, fmt.Errorf("no se pudo conectar a Redmine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return TaskItem{}, fmt.Errorf("Redmine respondió %s al crear la tarea", resp.Status)
	}

	var parsed struct {
		Issue redmineIssue `json:"issue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return TaskItem{}, fmt.Errorf("respuesta inválida de Redmine: %w", err)
	}

	item := redmineToTaskItem(parsed.Issue, strings.TrimRight(cfg["base_url"], "/"))
	item.CreatedByMe = true
	return item, nil
}

func (p *redmineProvider) currentUserID(ctx context.Context, cfg Config) (int, error) {
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/users/current.json", nil)
	if err != nil {
		return 0, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("no se pudo conectar a Redmine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Redmine respondió %s", resp.Status)
	}

	var parsed struct {
		User struct {
			ID int `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, fmt.Errorf("respuesta inválida de Redmine: %w", err)
	}
	return parsed.User.ID, nil
}

func redmineToTaskItem(issue redmineIssue, baseURL string) TaskItem {
	updatedAt, _ := time.Parse(time.RFC3339, issue.UpdatedOn)
	return TaskItem{
		ID:          fmt.Sprintf("redmine:%d", issue.ID),
		Provider:    "redmine",
		Type:        ItemTypeIssue,
		Title:       issue.Subject,
		URL:         fmt.Sprintf("%s/issues/%d", baseURL, issue.ID),
		Project:     issue.Project.Name,
		Status:      issue.Status.Name,
		Author:      issue.Author.Name,
		Description: issue.Description,
		UpdatedAt:   updatedAt,
	}
}

func (p *redmineProvider) newRequest(ctx context.Context, cfg Config, method, path string, body io.Reader) (*http.Request, error) {
	baseURL := strings.TrimRight(cfg["base_url"], "/")
	if baseURL == "" {
		return nil, fmt.Errorf("falta la URL del servidor Redmine")
	}
	if cfg["api_key"] == "" {
		return nil, fmt.Errorf("falta la API key de Redmine")
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Redmine-API-Key", cfg["api_key"])
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

type redmineIssuesResponse struct {
	Issues []redmineIssue `json:"issues"`
}

type redmineIssue struct {
	ID          int    `json:"id"`
	Subject     string `json:"subject"`
	Description string `json:"description"`
	UpdatedOn   string `json:"updated_on"`
	Project     struct {
		Name string `json:"name"`
	} `json:"project"`
	Status struct {
		Name string `json:"name"`
	} `json:"status"`
	Author struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"author"`
}
