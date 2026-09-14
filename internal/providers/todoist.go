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

const todoistAPIBase = "https://api.todoist.com/api/v1"

func init() {
	Register("todoist", func() Provider {
		return &todoistProvider{client: defaultHTTPClient(), apiBase: todoistAPIBase}
	})
}

// todoistProvider surfaces the user's active Todoist tasks. Todoist is a
// personal task manager, not a code host or issue tracker, so — like
// Redmine's own gap — it never returns PRs or mentions, only
// ItemTypeIssue.
//
// "Assigned to me" has no single equivalent here either: Todoist's own
// "assigned to: me" filter only matches tasks explicitly assigned in a
// shared project, which would return nothing for the (far more common)
// case of a personal account with no shared projects at all. FetchItems
// instead asks for "assigned to: me | !assigned" — explicitly-assigned-to-
// me OR unassigned — which covers both a solo account's ordinary tasks and
// a team account's shared-project assignments, while excluding tasks
// explicitly assigned to someone else.
//
// The API only exposes creator/poster user IDs for tasks and comments, not
// names — resolving those needs a separate per-project collaborators
// lookup. Given personal Todoist usage means the account holder is
// virtually always the task's own creator, CreatedByMe is reported
// unconditionally true rather than adding that lookup; comment Author is
// left blank, which the UI already renders as "Alguien"/"Someone".
type todoistProvider struct {
	client  *http.Client
	apiBase string // overridable in tests; real usage always hits todoistAPIBase
}

func (p *todoistProvider) Name() string        { return "todoist" }
func (p *todoistProvider) DisplayName() string { return "Todoist" }

func (p *todoistProvider) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "token", Label: "provider.field.todoist.token", Kind: FieldSecret, Required: true},
	}
}

func (p *todoistProvider) ProjectHint() string {
	return "provider.hint.todoist"
}

func (p *todoistProvider) TestConnection(ctx context.Context, cfg Config) error {
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/projects?limit=1", nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("no se pudo conectar a Todoist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Todoist respondió %s", resp.Status)
	}
	return nil
}

// FetchItems returns the caller's active (incomplete) tasks — see the type
// doc comment for what "assigned to me" means for this provider.
func (p *todoistProvider) FetchItems(ctx context.Context, cfg Config) ([]TaskItem, error) {
	query := "assigned to: me | !assigned"
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/tasks/filter?query="+url.QueryEscape(query)+"&limit=200", nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a Todoist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Todoist respondió %s", resp.Status)
	}

	var parsed todoistTasksResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("respuesta inválida de Todoist: %w", err)
	}

	// Best-effort: if this fails, Project just stays blank for every item
	// rather than failing the whole fetch over a secondary field.
	projectNames, _ := p.fetchProjectNames(ctx, cfg)

	items := make([]TaskItem, 0, len(parsed.Results))
	for _, task := range parsed.Results {
		items = append(items, todoistToTaskItem(task, projectNames[task.ProjectID]))
	}
	return items, nil
}

// ListProjects returns every project the token's user has access to, for
// the "new task" form's project dropdown. Capped at 200 (Todoist's max
// page size) — the form's free-text fallback covers anything beyond that,
// though a Todoist project ID isn't something a user would type from
// memory, so the dropdown is the practical path here.
func (p *todoistProvider) ListProjects(ctx context.Context, cfg Config) ([]ProjectOption, error) {
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/projects?limit=200", nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a Todoist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Todoist respondió %s", resp.Status)
	}

	var parsed todoistProjectsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("respuesta inválida de Todoist: %w", err)
	}

	options := make([]ProjectOption, 0, len(parsed.Results))
	for _, pr := range parsed.Results {
		options = append(options, ProjectOption{Value: pr.ID, Label: pr.Name})
	}
	return options, nil
}

// SearchItems runs a full-text search across the user's active tasks via
// Todoist's own filter query language ("search: <text>") — accessible to
// every task the token's user can see, not just the FetchItems scope.
// Unlike Redmine/GitHub/GitLab, this can't reach completed tasks: Todoist
// only exposes those through a separate date-range-based endpoint, not a
// searchable one, so a completed task simply won't show up here — a
// documented gap, not an oversight.
func (p *todoistProvider) SearchItems(ctx context.Context, cfg Config, query string) ([]TaskItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	filterQuery := "search: " + query
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/tasks/filter?query="+url.QueryEscape(filterQuery)+"&limit=25", nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a Todoist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Todoist respondió %s", resp.Status)
	}

	var parsed todoistTasksResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("respuesta inválida de Todoist: %w", err)
	}

	// Best-effort: if this fails, Project just stays blank for every item.
	projectNames, _ := p.fetchProjectNames(ctx, cfg)

	items := make([]TaskItem, 0, len(parsed.Results))
	for _, task := range parsed.Results {
		items = append(items, todoistToTaskItem(task, projectNames[task.ProjectID]))
	}
	return items, nil
}

// FetchComments returns a task's comments, oldest first. Comment.Author is
// left blank — see the type doc comment.
func (p *todoistProvider) FetchComments(ctx context.Context, cfg Config, item TaskItem) ([]Comment, error) {
	id := strings.TrimPrefix(item.ID, "todoist:")

	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/comments?task_id="+url.QueryEscape(id)+"&limit=200", nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a Todoist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Todoist respondió %s", resp.Status)
	}

	var parsed struct {
		Results []struct {
			Content  string `json:"content"`
			PostedAt string `json:"posted_at"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("respuesta inválida de Todoist: %w", err)
	}

	comments := make([]Comment, 0, len(parsed.Results))
	for _, c := range parsed.Results {
		postedAt, _ := time.Parse(time.RFC3339Nano, c.PostedAt)
		comments = append(comments, Comment{Body: c.Content, CreatedAt: postedAt})
	}
	return comments, nil
}

// CreateItem creates a Todoist task in the given project (a Todoist
// project ID — see ProjectHint), assigned to nobody in particular since a
// personal task simply belongs to the account it was created in.
func (p *todoistProvider) CreateItem(ctx context.Context, cfg Config, input CreateItemInput) (TaskItem, error) {
	if input.Project == "" {
		return TaskItem{}, fmt.Errorf("falta el proyecto")
	}
	if input.Title == "" {
		return TaskItem{}, fmt.Errorf("falta el título")
	}

	payload, err := json.Marshal(map[string]any{
		"content":     input.Title,
		"description": input.Description,
		"project_id":  input.Project,
	})
	if err != nil {
		return TaskItem{}, err
	}

	req, err := p.newRequest(ctx, cfg, http.MethodPost, "/tasks", bytes.NewReader(payload))
	if err != nil {
		return TaskItem{}, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return TaskItem{}, fmt.Errorf("no se pudo conectar a Todoist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TaskItem{}, fmt.Errorf("Todoist respondió %s al crear la tarea", resp.Status)
	}

	var task todoistTask
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return TaskItem{}, fmt.Errorf("respuesta inválida de Todoist: %w", err)
	}

	item := todoistToTaskItem(task, "")
	// Best-effort: resolve the project name for immediate display instead
	// of leaving it blank until the next full refresh.
	if names, err := p.fetchProjectNames(ctx, cfg); err == nil {
		item.Project = names[task.ProjectID]
	}
	return item, nil
}

// FetchItem re-fetches a single task by ID — see Provider.FetchItem. Kept
// working for a completed task too (unlike the list endpoints, GET
// /tasks/{id} still returns it with completed_at set), which is what lets
// a starred task in Seguimiento self-heal into "closed" instead of
// erroring once it's checked off.
func (p *todoistProvider) FetchItem(ctx context.Context, cfg Config, item TaskItem) (TaskItem, error) {
	id := strings.TrimPrefix(item.ID, "todoist:")

	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/tasks/"+url.PathEscape(id), nil)
	if err != nil {
		return TaskItem{}, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return TaskItem{}, fmt.Errorf("no se pudo conectar a Todoist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return TaskItem{}, fmt.Errorf("la tarea %q ya no existe o no es accesible con este token", id)
	}
	if resp.StatusCode != http.StatusOK {
		return TaskItem{}, fmt.Errorf("Todoist respondió %s", resp.Status)
	}

	var task todoistTask
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return TaskItem{}, fmt.Errorf("respuesta inválida de Todoist: %w", err)
	}

	fresh := todoistToTaskItem(task, "")
	// Best-effort: if this fails, Project just stays blank rather than
	// failing the whole refresh over a secondary field.
	if names, err := p.fetchProjectNames(ctx, cfg); err == nil {
		fresh.Project = names[task.ProjectID]
	}
	return fresh, nil
}

// fetchProjectNames returns every accessible project's name keyed by ID —
// tasks only carry a project_id, so this backfills the human-readable
// Project field the same way Redmine's SearchItems backfills project/status.
func (p *todoistProvider) fetchProjectNames(ctx context.Context, cfg Config) (map[string]string, error) {
	req, err := p.newRequest(ctx, cfg, http.MethodGet, "/projects?limit=200", nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a Todoist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Todoist respondió %s", resp.Status)
	}

	var parsed todoistProjectsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("respuesta inválida de Todoist: %w", err)
	}

	names := make(map[string]string, len(parsed.Results))
	for _, pr := range parsed.Results {
		names[pr.ID] = pr.Name
	}
	return names, nil
}

type todoistTasksResponse struct {
	Results []todoistTask `json:"results"`
}

type todoistProjectsResponse struct {
	Results []todoistProject `json:"results"`
}

type todoistTask struct {
	ID          string  `json:"id"`
	Content     string  `json:"content"`
	Description string  `json:"description"`
	ProjectID   string  `json:"project_id"`
	Priority    int     `json:"priority"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	CompletedAt *string `json:"completed_at"`
}

type todoistProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func todoistToTaskItem(task todoistTask, projectName string) TaskItem {
	updatedAt, err := time.Parse(time.RFC3339Nano, task.UpdatedAt)
	if err != nil {
		// Fall back to CreatedAt rather than leaving a zero-value time —
		// still best-effort, same spirit as the other providers' parsing.
		updatedAt, _ = time.Parse(time.RFC3339Nano, task.CreatedAt)
	}

	return TaskItem{
		ID:          fmt.Sprintf("todoist:%s", task.ID),
		Provider:    "todoist",
		Type:        ItemTypeIssue,
		Title:       task.Content,
		URL:         fmt.Sprintf("https://app.todoist.com/app/task/%s", task.ID),
		Project:     projectName,
		Status:      todoistPriorityLabel(task.Priority),
		Closed:      task.CompletedAt != nil && *task.CompletedAt != "",
		Description: task.Description,
		// See the type doc comment on why this is unconditional.
		CreatedByMe: true,
		UpdatedAt:   updatedAt,
	}
}

// todoistPriorityLabel converts Todoist's API priority (1 = normal/no
// priority set, 4 = most urgent) to the P1..P3 label shown in Todoist's own
// UI — which numbers priority the other way around (P1 = most urgent) and,
// like this function, shows no flag at all for the default priority-1
// case rather than an uninformative "P4" on nearly every task.
func todoistPriorityLabel(apiPriority int) string {
	if apiPriority <= 1 || apiPriority > 4 {
		return ""
	}
	return fmt.Sprintf("P%d", 5-apiPriority)
}

func (p *todoistProvider) newRequest(ctx context.Context, cfg Config, method, path string, body io.Reader) (*http.Request, error) {
	if cfg["token"] == "" {
		return nil, fmt.Errorf("falta el token de Todoist")
	}

	req, err := http.NewRequestWithContext(ctx, method, p.apiBase+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg["token"])
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}
