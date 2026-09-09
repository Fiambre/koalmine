package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func gitlabItemFixture(id int, title, refFull string) map[string]any {
	return map[string]any{
		"id":          id,
		"title":       title,
		"description": "Descripción de " + title,
		"web_url":     "https://gitlab.com/acme/repo/-/issues/" + refFull,
		"state":       "opened",
		"updated_at":  "2026-09-01T10:00:00Z",
		"author":      map[string]any{"username": "rodrigo"},
		"references":  map[string]any{"full": refFull},
	}
}

func TestGitlabFetchItemsDedupesReviewerAndAssigned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "secret" {
			t.Errorf("missing/incorrect PRIVATE-TOKEN header: %q", r.Header.Get("PRIVATE-TOKEN"))
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "username": "rodrigo"})
		case r.URL.Path == "/issues":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				gitlabItemFixture(1, "Bug asignado", "acme/repo#1"),
			})
		case r.URL.Path == "/merge_requests" && r.URL.Query().Get("scope") == "assigned_to_me":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				gitlabItemFixture(2, "MR asignado", "acme/repo!2"),
			})
		case r.URL.Path == "/merge_requests" && r.URL.Query().Get("reviewer_id") == "7":
			// MR 2 shows up again here (also has me as reviewer): must be
			// deduplicated. MR 3 is reviewer-only.
			_ = json.NewEncoder(w).Encode([]map[string]any{
				gitlabItemFixture(2, "MR asignado", "acme/repo!2"),
				gitlabItemFixture(3, "MR para revisar", "acme/repo!3"),
			})
		default:
			t.Errorf("unexpected request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()

	p := &gitlabProvider{client: server.Client(), apiBase: server.URL}
	items, err := p.FetchItems(context.Background(), Config{"token": "secret"})
	if err != nil {
		t.Fatalf("FetchItems: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 deduplicated items, got %d: %+v", len(items), items)
	}

	if items[0].ID != "gitlab:issue:1" || items[0].Type != ItemTypeIssue || items[0].Project != "acme/repo" {
		t.Errorf("unexpected issue item: %+v", items[0])
	}
	if items[0].Description != "Descripción de Bug asignado" {
		t.Errorf("unexpected description: %q", items[0].Description)
	}
	if items[1].ID != "gitlab:pr:2" || items[1].Type != ItemTypePR {
		t.Errorf("unexpected assigned-MR item: %+v", items[1])
	}
	if items[2].ID != "gitlab:pr:3" || items[2].Type != ItemTypePR {
		t.Errorf("unexpected reviewer-MR item: %+v", items[2])
	}
	if !items[0].CreatedByMe {
		t.Errorf("expected item 1 to be marked as created by me (author matches the authenticated username)")
	}
}

func TestGitlabListProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"path_with_namespace": "grupo/proyecto"},
			{"path_with_namespace": "grupo/otro"},
		})
	}))
	defer server.Close()

	p := &gitlabProvider{client: server.Client(), apiBase: server.URL}
	options, err := p.ListProjects(context.Background(), Config{"token": "secret"})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(options) != 2 || options[0].Value != "grupo/proyecto" {
		t.Errorf("unexpected options: %+v", options)
	}
}

func TestGitlabSearchItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "username": "rodrigo"})
		case r.URL.Path == "/search" && r.URL.Query().Get("scope") == "issues":
			if r.URL.Query().Get("search") != "build" {
				t.Errorf("unexpected search term: %s", r.URL.Query().Get("search"))
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{
				gitlabItemFixture(1, "Arreglar el build", "acme/repo#1"),
			})
		case r.URL.Path == "/search" && r.URL.Query().Get("scope") == "merge_requests":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Errorf("unexpected request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()

	p := &gitlabProvider{client: server.Client(), apiBase: server.URL}
	items, err := p.SearchItems(context.Background(), Config{"token": "secret"}, "build")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) != 1 || items[0].ID != "gitlab:issue:1" {
		t.Fatalf("unexpected items: %+v", items)
	}
	if !items[0].CreatedByMe {
		t.Errorf("expected item to be marked as created by me")
	}
}

func TestGitlabSearchItemsEmptyQuery(t *testing.T) {
	p := &gitlabProvider{client: defaultHTTPClient(), apiBase: gitlabAPIBase}
	items, err := p.SearchItems(context.Background(), Config{"token": "secret"}, "  ")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected no items for a blank query, got %d", len(items))
	}
}

func TestGitlabCreateItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "secret" {
			t.Errorf("missing/incorrect PRIVATE-TOKEN header: %q", r.Header.Get("PRIVATE-TOKEN"))
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "username": "rodrigo"})
		case r.Method == http.MethodPost && r.URL.Path == "/projects/grupo/proyecto/issues":
			var body struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				AssigneeIDs []int  `json:"assignee_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding request body: %v", err)
			}
			if body.Title != "Nueva tarea" || len(body.AssigneeIDs) != 1 || body.AssigneeIDs[0] != 7 {
				t.Errorf("unexpected request body: %+v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(gitlabItemFixture(99, "Nueva tarea", "grupo/proyecto#99"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p := &gitlabProvider{client: server.Client(), apiBase: server.URL}
	item, err := p.CreateItem(context.Background(), Config{"token": "secret"}, CreateItemInput{
		Project:     "grupo/proyecto",
		Title:       "Nueva tarea",
		Description: "Detalle",
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if item.ID != "gitlab:issue:99" || item.Type != ItemTypeIssue || item.Project != "grupo/proyecto" {
		t.Errorf("unexpected item: %+v", item)
	}
}

func TestGitlabFetchItemsMissingToken(t *testing.T) {
	p := &gitlabProvider{client: defaultHTTPClient(), apiBase: gitlabAPIBase}
	if _, err := p.FetchItems(context.Background(), Config{}); err == nil {
		t.Error("expected an error when the token is missing")
	}
}

func TestGitlabTestConnectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	p := &gitlabProvider{client: server.Client(), apiBase: server.URL}
	if err := p.TestConnection(context.Background(), Config{"token": "wrong"}); err == nil {
		t.Error("expected TestConnection to fail on a 401 response")
	}
}

func TestProjectFromReference(t *testing.T) {
	cases := map[string]string{
		"acme/repo#42":          "acme/repo",
		"acme/repo!7":           "acme/repo",
		"group/subgroup/repo#3": "group/subgroup/repo",
		"no-separator-here":     "no-separator-here",
	}
	for input, want := range cases {
		if got := projectFromReference(input); got != want {
			t.Errorf("projectFromReference(%q) = %q, want %q", input, got, want)
		}
	}
}
