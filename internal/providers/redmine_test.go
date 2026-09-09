package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedmineFetchItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Redmine-API-Key") != "secret" {
			t.Errorf("missing/incorrect API key header: %q", r.Header.Get("X-Redmine-API-Key"))
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/users/current.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": 5}})
		case "/issues.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issues": []map[string]any{
					{
						"id":          42,
						"subject":     "Arreglar el build",
						"description": "El build falla en CI desde el commit abc123.",
						"updated_on":  "2026-09-01T10:00:00Z",
						"project":     map[string]any{"name": "Koalmine"},
						"status":      map[string]any{"name": "En curso"},
						"author":      map[string]any{"id": 5, "name": "Rodrigo"},
					},
				},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p, ok := Get("redmine")
	if !ok {
		t.Fatal("redmine provider not registered")
	}
	cfg := Config{"base_url": server.URL, "api_key": "secret"}

	items, err := p.FetchItems(context.Background(), cfg)
	if err != nil {
		t.Fatalf("FetchItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item.Title != "Arreglar el build" || item.Project != "Koalmine" || item.Type != ItemTypeIssue {
		t.Errorf("unexpected item: %+v", item)
	}
	if item.Description != "El build falla en CI desde el commit abc123." {
		t.Errorf("unexpected description: %q", item.Description)
	}
	if item.URL != server.URL+"/issues/42" {
		t.Errorf("unexpected URL: %s", item.URL)
	}
	if item.ID != "redmine:42" {
		t.Errorf("unexpected ID: %s", item.ID)
	}
	if !item.CreatedByMe {
		t.Errorf("expected item to be marked as created by me (author id matches the current user)")
	}
}

func TestRedmineFetchItemsMissingConfig(t *testing.T) {
	p, _ := Get("redmine")
	if _, err := p.FetchItems(context.Background(), Config{}); err == nil {
		t.Error("expected an error when base_url/api_key are missing")
	}
}

func TestRedmineListProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"projects": []map[string]any{
				{"id": 1, "identifier": "mi-proyecto", "name": "Mi Proyecto"},
				{"id": 2, "identifier": "otro", "name": "Otro Proyecto"},
			},
		})
	}))
	defer server.Close()

	p, _ := Get("redmine")
	options, err := p.ListProjects(context.Background(), Config{"base_url": server.URL, "api_key": "secret"})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(options) != 2 || options[0].Value != "mi-proyecto" || options[0].Label != "Mi Proyecto" {
		t.Errorf("unexpected options: %+v", options)
	}
}

func TestRedmineSearchItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "build" {
			t.Errorf("unexpected query: %s", r.URL.Query().Get("q"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"id":          42,
					"title":       "Arreglar el build",
					"type":        "issue",
					"url":         "https://redmine.example.com/issues/42",
					"description": "El build falla en CI.",
					"datetime":    "2026-09-01T10:00:00Z",
				},
				{
					"id":    7,
					"title": "Un proyecto que contiene 'build' en el nombre",
					"type":  "project",
					"url":   "https://redmine.example.com/projects/7",
				},
			},
		})
	}))
	defer server.Close()

	p, _ := Get("redmine")
	cfg := Config{"base_url": server.URL, "api_key": "secret"}

	items, err := p.SearchItems(context.Background(), cfg, "build")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected non-issue results to be filtered out, got %d: %+v", len(items), items)
	}
	if items[0].ID != "redmine:42" || items[0].Title != "Arreglar el build" {
		t.Errorf("unexpected item: %+v", items[0])
	}
}

func TestRedmineSearchItemsEmptyQuery(t *testing.T) {
	p, _ := Get("redmine")
	items, err := p.SearchItems(context.Background(), Config{"base_url": "http://example.com", "api_key": "secret"}, "  ")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected no items for a blank query, got %d", len(items))
	}
}

func TestRedmineCreateItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/current.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": 5}})
		case r.Method == http.MethodPost && r.URL.Path == "/issues.json":
			var body struct {
				Issue struct {
					ProjectID    string `json:"project_id"`
					Subject      string `json:"subject"`
					Description  string `json:"description"`
					AssignedToID int    `json:"assigned_to_id"`
				} `json:"issue"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding request body: %v", err)
			}
			if body.Issue.ProjectID != "mi-proyecto" || body.Issue.Subject != "Nueva tarea" || body.Issue.AssignedToID != 5 {
				t.Errorf("unexpected request body: %+v", body.Issue)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issue": map[string]any{
					"id":          99,
					"subject":     "Nueva tarea",
					"description": "Detalle",
					"updated_on":  "2026-09-01T10:00:00Z",
					"project":     map[string]any{"name": "Mi Proyecto"},
					"status":      map[string]any{"name": "Nueva"},
					"author":      map[string]any{"name": "Rodrigo"},
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p, _ := Get("redmine")
	cfg := Config{"base_url": server.URL, "api_key": "secret"}

	item, err := p.CreateItem(context.Background(), cfg, CreateItemInput{
		Project:     "mi-proyecto",
		Title:       "Nueva tarea",
		Description: "Detalle",
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if item.ID != "redmine:99" || item.Title != "Nueva tarea" || item.Project != "Mi Proyecto" {
		t.Errorf("unexpected item: %+v", item)
	}
}

func TestRedmineTestConnectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	p, _ := Get("redmine")
	cfg := Config{"base_url": server.URL, "api_key": "wrong"}
	if err := p.TestConnection(context.Background(), cfg); err == nil {
		t.Error("expected TestConnection to fail on a 401 response")
	}
}
