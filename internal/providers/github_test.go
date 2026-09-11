package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestGithubFetchItemsDeduplicatesAcrossQueries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing/incorrect Authorization header: %q", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/user" {
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "rodrigo"})
			return
		}

		q := r.URL.Query().Get("q")
		switch {
		case strings.Contains(q, "assignee:@me"):
			// Item 1: assigned to me.
			_ = json.NewEncoder(w).Encode(githubFixture(issueFixture(1, "Bug asignado", false)))
		case strings.Contains(q, "review-requested:@me"):
			// Item 2: a PR where I'm requested reviewer.
			_ = json.NewEncoder(w).Encode(githubFixture(issueFixture(2, "PR para revisar", true)))
		case strings.Contains(q, "mentions:@me"):
			// Item 1 shows up again here (also mentions me) — must be
			// deduplicated and keep its original "issue" type. Item 3 is a
			// mention-only item.
			_ = json.NewEncoder(w).Encode(githubFixture(
				issueFixture(1, "Bug asignado", false),
				issueFixture(3, "Me mencionaron acá", false),
			))
		default:
			t.Errorf("unexpected query: %s", q)
		}
	}))
	defer server.Close()

	p := &githubProvider{client: server.Client(), baseURL: server.URL}
	items, err := p.FetchItems(context.Background(), Config{"token": "secret"})
	if err != nil {
		t.Fatalf("FetchItems: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 deduplicated items, got %d: %+v", len(items), items)
	}

	if items[0].ID != "github:1" || items[0].Type != ItemTypeIssue {
		t.Errorf("item 1 should keep its original type (issue): %+v", items[0])
	}
	if items[1].ID != "github:2" || items[1].Type != ItemTypePR {
		t.Errorf("item 2 should be a PR: %+v", items[1])
	}
	if items[2].ID != "github:3" || items[2].Type != ItemTypeMention {
		t.Errorf("item 3 should be a mention: %+v", items[2])
	}
	if items[1].Project != "acme/repo" {
		t.Errorf("unexpected project name: %s", items[1].Project)
	}
	if items[1].Description != "Descripción de PR para revisar" {
		t.Errorf("unexpected description: %q", items[1].Description)
	}
	if !items[0].CreatedByMe {
		t.Errorf("expected item 1 to be marked as created by me (author matches the authenticated login)")
	}
}

func TestGithubListProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"full_name": "acme/repo"},
			{"full_name": "acme/other"},
		})
	}))
	defer server.Close()

	p := &githubProvider{client: server.Client(), baseURL: server.URL}
	options, err := p.ListProjects(context.Background(), Config{"token": "secret"})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(options) != 2 || options[0].Value != "acme/repo" {
		t.Errorf("unexpected options: %+v", options)
	}
}

func TestGithubSearchItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/user" {
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "rodrigo"})
			return
		}

		q := r.URL.Query().Get("q")
		if !strings.Contains(q, "build") || !strings.Contains(q, "involves:@me") {
			t.Errorf("unexpected query: %s", q)
		}
		_ = json.NewEncoder(w).Encode(githubFixture(issueFixture(5, "Arreglar el build", false)))
	}))
	defer server.Close()

	p := &githubProvider{client: server.Client(), baseURL: server.URL}
	items, err := p.SearchItems(context.Background(), Config{"token": "secret"}, "build")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) != 1 || items[0].ID != "github:5" {
		t.Fatalf("unexpected items: %+v", items)
	}
	if !items[0].CreatedByMe {
		t.Errorf("expected item to be marked as created by me")
	}
}

func TestGithubSearchItemsEmptyQuery(t *testing.T) {
	p := &githubProvider{client: defaultHTTPClient(), baseURL: githubAPIBase}
	items, err := p.SearchItems(context.Background(), Config{"token": "secret"}, "   ")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected no items for a blank query, got %d", len(items))
	}
}

func TestGithubFetchComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/repo/issues/42/comments" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"body": "Un comentario", "created_at": "2026-09-01T10:00:00Z", "user": map[string]any{"login": "rodrigo"}},
		})
	}))
	defer server.Close()

	p := &githubProvider{client: server.Client(), baseURL: server.URL}
	item := TaskItem{Project: "acme/repo", URL: "https://github.com/acme/repo/issues/42"}
	comments, err := p.FetchComments(context.Background(), Config{"token": "secret"}, item)
	if err != nil {
		t.Fatalf("FetchComments: %v", err)
	}
	if len(comments) != 1 || comments[0].Body != "Un comentario" || comments[0].Author != "rodrigo" {
		t.Errorf("unexpected comments: %+v", comments)
	}
}

func TestGithubFetchItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/repo/issues/42":
			_ = json.NewEncoder(w).Encode(issueFixture(42, "Arreglar el build", false))
		case "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "rodrigo"})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p := &githubProvider{client: server.Client(), baseURL: server.URL}
	item := TaskItem{ID: "github:42", Type: ItemTypeMention, Project: "acme/repo", URL: "https://github.com/acme/repo/issues/42"}
	fresh, err := p.FetchItem(context.Background(), Config{"token": "secret"}, item)
	if err != nil {
		t.Fatalf("FetchItem: %v", err)
	}
	if fresh.Title != "Arreglar el build" {
		t.Errorf("unexpected item: %+v", fresh)
	}
	// A non-PR issue keeps whatever type the caller passed in (GitHub has no
	// "mention" concept of its own to re-derive from the API response).
	if fresh.Type != ItemTypeMention {
		t.Errorf("expected type to stay %q, got %q", ItemTypeMention, fresh.Type)
	}
}

func TestGithubCreateItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing/incorrect Authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "rodrigo"})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/repo/issues":
			var body struct {
				Title     string   `json:"title"`
				Body      string   `json:"body"`
				Assignees []string `json:"assignees"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding request body: %v", err)
			}
			if body.Title != "Nueva tarea" || len(body.Assignees) != 1 || body.Assignees[0] != "rodrigo" {
				t.Errorf("unexpected request body: %+v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(issueFixture(99, "Nueva tarea", false))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p := &githubProvider{client: server.Client(), baseURL: server.URL}
	item, err := p.CreateItem(context.Background(), Config{"token": "secret"}, CreateItemInput{
		Project:     "acme/repo",
		Title:       "Nueva tarea",
		Description: "Detalle",
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if item.ID != "github:99" || item.Type != ItemTypeIssue || item.Project != "acme/repo" {
		t.Errorf("unexpected item: %+v", item)
	}
}

func TestGithubCreateItemInvalidProject(t *testing.T) {
	p := &githubProvider{client: defaultHTTPClient(), baseURL: githubAPIBase}
	_, err := p.CreateItem(context.Background(), Config{"token": "secret"}, CreateItemInput{Project: "not-a-repo", Title: "x"})
	if err == nil {
		t.Error("expected an error when project isn't in owner/repo form")
	}
}

func TestGithubFetchItemsMissingToken(t *testing.T) {
	p := &githubProvider{client: defaultHTTPClient(), baseURL: githubAPIBase}
	if _, err := p.FetchItems(context.Background(), Config{}); err == nil {
		t.Error("expected an error when the token is missing")
	}
}

func issueFixture(id int, title string, isPR bool) map[string]any {
	f := map[string]any{
		"id":             id,
		"title":          title,
		"body":           "Descripción de " + title,
		"html_url":       "https://github.com/acme/repo/issues/" + strconv.Itoa(id),
		"state":          "open",
		"updated_at":     "2026-09-01T10:00:00Z",
		"repository_url": "https://api.github.com/repos/acme/repo",
		"user":           map[string]any{"login": "rodrigo"},
	}
	if isPR {
		f["pull_request"] = map[string]any{}
	}
	return f
}

func githubFixture(issues ...map[string]any) map[string]any {
	return map[string]any{"items": issues}
}
