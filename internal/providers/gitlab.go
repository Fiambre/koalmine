package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
			items = append(items, item)
		}
	}
	return items, nil
}

type gitlabUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

func (p *gitlabProvider) currentUser(ctx context.Context, cfg Config) (gitlabUser, error) {
	req, err := p.newRequest(ctx, cfg, "/user")
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
	req, err := p.newRequest(ctx, cfg, path)
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
		updatedAt, _ := time.Parse(time.RFC3339, it.UpdatedAt)
		items = append(items, TaskItem{
			// Issues and merge requests have independent ID sequences in
			// GitLab's API, so the item type must be part of the key —
			// otherwise an issue and an MR that happen to share a numeric
			// ID would collide.
			ID:        fmt.Sprintf("gitlab:%s:%d", itemType, it.ID),
			Provider:  "gitlab",
			Type:      itemType,
			Title:     it.Title,
			URL:       it.WebURL,
			Project:   projectFromReference(it.References.Full),
			Status:    it.State,
			Author:    it.Author.Username,
			UpdatedAt: updatedAt,
		})
	}
	return items, nil
}

func (p *gitlabProvider) newRequest(ctx context.Context, cfg Config, path string) (*http.Request, error) {
	if cfg["token"] == "" {
		return nil, fmt.Errorf("falta el Personal Access Token de GitLab")
	}

	baseURL := p.apiBase
	if override := strings.TrimRight(cfg["base_url"], "/"); override != "" {
		baseURL = override + "/api/v4"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", cfg["token"])
	req.Header.Set("Accept", "application/json")
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
	ID        int    `json:"id"`
	Title     string `json:"title"`
	WebURL    string `json:"web_url"`
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
	Author    struct {
		Username string `json:"username"`
	} `json:"author"`
	References struct {
		Full string `json:"full"`
	} `json:"references"`
}
