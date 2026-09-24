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
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search.json":
			if r.URL.Query().Get("q") != "build" {
				t.Errorf("unexpected query: %s", r.URL.Query().Get("q"))
			}
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
		case "/issues.json":
			// The backfill lookup: only the issue-type result (42) should
			// be requested, never the project-type one (7).
			if got := r.URL.Query().Get("issue_id"); got != "42" {
				t.Errorf("unexpected issue_id filter: %s", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issues": []map[string]any{
					{
						"id":          42,
						"subject":     "Arreglar el build",
						"description": "El build falla en CI.",
						"updated_on":  "2026-09-01T10:00:00Z",
						"project":     map[string]any{"name": "Koalmine"},
						"status":      map[string]any{"id": 2, "name": "En curso"},
						"author":      map[string]any{"id": 9, "name": "Otra Persona"},
					},
				},
			})
		case "/issue_statuses.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issue_statuses": []map[string]any{
					{"id": 1, "is_closed": true},
					{"id": 2, "is_closed": false},
				},
			})
		case "/users/current.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": 5}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
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
	item := items[0]
	if item.ID != "redmine:42" || item.Title != "Arreglar el build" {
		t.Errorf("unexpected item: %+v", item)
	}
	if item.Project != "Koalmine" || item.Status != "En curso" {
		t.Errorf("expected project/status backfilled from the bulk issue lookup, got: %+v", item)
	}
	if item.Closed {
		t.Errorf("expected Closed false for a status not marked is_closed, got true")
	}
	if item.CreatedByMe {
		t.Errorf("expected CreatedByMe false for an issue authored by someone else, got true")
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

func TestRedmineSearchItemsByTicketNumber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/issues/42.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issue": map[string]any{
					"id":         42,
					"subject":    "Arreglar el build",
					"updated_on": "2026-09-01T10:00:00Z",
					"status":     map[string]any{"id": 1, "name": "Cerrado"},
				},
			})
		case "/issue_statuses.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issue_statuses": []map[string]any{
					{"id": 1, "is_closed": true},
				},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p, _ := Get("redmine")
	cfg := Config{"base_url": server.URL, "api_key": "secret"}

	for _, query := range []string{"42", "#42"} {
		items, err := p.SearchItems(context.Background(), cfg, query)
		if err != nil {
			t.Fatalf("SearchItems(%q): %v", query, err)
		}
		if len(items) != 1 || items[0].ID != "redmine:42" || items[0].Title != "Arreglar el build" {
			t.Errorf("SearchItems(%q): unexpected items: %+v", query, items)
		}
		if !items[0].Closed {
			t.Errorf("SearchItems(%q): expected Closed true for a status marked is_closed, got false", query)
		}
	}
}

func TestRedmineSearchItemsByTicketNumberFallsBackWhenNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/issues/999.json":
			w.WriteHeader(http.StatusNotFound)
		case "/search.json":
			if r.URL.Query().Get("q") != "999" {
				t.Errorf("unexpected query: %s", r.URL.Query().Get("q"))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p, _ := Get("redmine")
	cfg := Config{"base_url": server.URL, "api_key": "secret"}

	items, err := p.SearchItems(context.Background(), cfg, "999")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected no items, got %+v", items)
	}
}

func TestRedmineFetchItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/issues/42.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issue": map[string]any{
					"id":         42,
					"subject":    "Arreglar el build",
					"updated_on": "2026-09-01T10:00:00Z",
					"project":    map[string]any{"name": "Koalmine"},
					"status":     map[string]any{"id": 2, "name": "En curso"},
				},
			})
		case "/issue_statuses.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issue_statuses": []map[string]any{
					{"id": 1, "is_closed": true},
					{"id": 2, "is_closed": false},
				},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p, _ := Get("redmine")
	item := TaskItem{ID: "redmine:42"}
	fresh, err := p.FetchItem(context.Background(), Config{"base_url": server.URL, "api_key": "secret"}, item)
	if err != nil {
		t.Fatalf("FetchItem: %v", err)
	}
	if fresh.Project != "Koalmine" || fresh.Status != "En curso" {
		t.Errorf("unexpected item: %+v", fresh)
	}
	if fresh.Closed {
		t.Errorf("expected Closed false for a status not marked is_closed, got true")
	}
}

func TestRedmineFetchItemNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p, _ := Get("redmine")
	item := TaskItem{ID: "redmine:999"}
	_, err := p.FetchItem(context.Background(), Config{"base_url": server.URL, "api_key": "secret"}, item)
	if err == nil {
		t.Fatal("expected an error for a ticket that no longer exists")
	}
}

func TestRedmineFetchComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/issues/42.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issue": map[string]any{
				"journals": []map[string]any{
					{"notes": "", "created_on": "2026-09-01T09:00:00Z", "user": map[string]any{"name": "Sistema"}},
					{"notes": "Un comentario real", "created_on": "2026-09-01T10:00:00Z", "user": map[string]any{"name": "Rodrigo"}},
				},
			},
		})
	}))
	defer server.Close()

	p, _ := Get("redmine")
	cfg := Config{"base_url": server.URL, "api_key": "secret"}

	comments, err := p.FetchComments(context.Background(), cfg, TaskItem{ID: "redmine:42"})
	if err != nil {
		t.Fatalf("FetchComments: %v", err)
	}
	if len(comments) != 1 || comments[0].Body != "Un comentario real" || comments[0].Author != "Rodrigo" {
		t.Errorf("unexpected comments: %+v", comments)
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

func TestRedmineFetchItemsAssignedTo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/issues.json":
			if got := r.URL.Query().Get("assigned_to_id"); got != "9" {
				t.Errorf("unexpected assigned_to_id: %s", got)
			}
			if got := r.URL.Query().Get("project_id"); got != "koalmine" {
				t.Errorf("unexpected project_id: %s", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issues": []map[string]any{
					{
						"id":         42,
						"subject":    "Tarea de otra persona",
						"updated_on": "2026-09-01T10:00:00Z",
						"project":    map[string]any{"name": "Koalmine"},
						"status":     map[string]any{"name": "Nueva"},
						"author":     map[string]any{"id": 9, "name": "Otra Persona"},
					},
				},
			})
		case "/users/current.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": 5}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p, _ := Get("redmine")
	cfg := Config{"base_url": server.URL, "api_key": "secret"}

	items, err := p.FetchItemsAssignedTo(context.Background(), cfg, "9", "koalmine")
	if err != nil {
		t.Fatalf("FetchItemsAssignedTo: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Tarea de otra persona" {
		t.Errorf("unexpected items: %+v", items)
	}
	if items[0].CreatedByMe {
		t.Errorf("expected CreatedByMe false (item authored by 9, current user is 5)")
	}
}

func TestRedmineFetchItemsAssignedToAll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/issues.json":
			if r.URL.Query().Has("assigned_to_id") {
				t.Errorf("expected no assigned_to_id filter for AssignedToAll, got %s", r.URL.Query().Get("assigned_to_id"))
			}
			if got := r.URL.Query().Get("project_id"); got != "koalmine" {
				t.Errorf("unexpected project_id: %s", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issues": []map[string]any{
					{
						"id":         42,
						"subject":    "Tarea de cualquiera",
						"updated_on": "2026-09-01T10:00:00Z",
						"project":    map[string]any{"name": "Koalmine"},
						"status":     map[string]any{"name": "Nueva"},
						"author":     map[string]any{"id": 9, "name": "Otra Persona"},
					},
				},
			})
		case "/users/current.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": 5}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p, _ := Get("redmine")
	cfg := Config{"base_url": server.URL, "api_key": "secret"}

	items, err := p.FetchItemsAssignedTo(context.Background(), cfg, AssignedToAll, "koalmine")
	if err != nil {
		t.Fatalf("FetchItemsAssignedTo: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Tarea de cualquiera" {
		t.Errorf("unexpected items: %+v", items)
	}
}

func TestRedmineFetchItemsAssignedToRequiresUser(t *testing.T) {
	p, _ := Get("redmine")
	if _, err := p.FetchItemsAssignedTo(context.Background(), Config{"base_url": "http://example.com", "api_key": "secret"}, "", ""); err == nil {
		t.Error("expected an error when assignedTo is blank")
	}
}

func TestRedmineFetchItemsCreatedByMe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/issues.json":
			if got := r.URL.Query().Get("author_id"); got != "me" {
				t.Errorf("unexpected author_id: %s", got)
			}
			if got := r.URL.Query().Get("status_id"); got != "*" {
				t.Errorf("expected status_id=* to include closed issues, got %s", got)
			}
			if got := r.URL.Query().Get("project_id"); got != "koalmine" {
				t.Errorf("unexpected project_id: %s", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issues": []map[string]any{
					{
						"id":         42,
						"subject":    "Una que cerré yo mismo",
						"updated_on": "2026-09-01T10:00:00Z",
						"project":    map[string]any{"name": "Koalmine"},
						"status":     map[string]any{"id": 1, "name": "Cerrado"},
						"author":     map[string]any{"id": 5, "name": "Rodrigo"},
					},
				},
			})
		case "/issue_statuses.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issue_statuses": []map[string]any{
					{"id": 1, "is_closed": true},
				},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p, _ := Get("redmine")
	cfg := Config{"base_url": server.URL, "api_key": "secret"}

	items, err := p.FetchItemsCreatedByMe(context.Background(), cfg, "koalmine")
	if err != nil {
		t.Fatalf("FetchItemsCreatedByMe: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Una que cerré yo mismo" {
		t.Errorf("unexpected items: %+v", items)
	}
	if !items[0].CreatedByMe {
		t.Error("expected CreatedByMe true")
	}
	if !items[0].Closed {
		t.Error("expected the closed issue to be reported as closed")
	}
}

func TestRedmineListAssignableUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/koalmine/memberships.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"memberships": []map[string]any{
				{"id": 1, "user": map[string]any{"id": 5, "name": "Rodrigo"}},
				{"id": 2, "group": map[string]any{"id": 9, "name": "Un Equipo"}},
				{"id": 3, "user": map[string]any{"id": 9, "name": "Otra Persona"}},
			},
		})
	}))
	defer server.Close()

	p, _ := Get("redmine")
	options, err := p.ListAssignableUsers(context.Background(), Config{"base_url": server.URL, "api_key": "secret"}, "koalmine")
	if err != nil {
		t.Fatalf("ListAssignableUsers: %v", err)
	}
	if len(options) != 2 {
		t.Fatalf("expected group membership to be skipped, got %+v", options)
	}
	if options[0].Value != "5" || options[0].Label != "Rodrigo" {
		t.Errorf("unexpected options: %+v", options)
	}
}

func TestRedmineListAssignableUsersRequiresProject(t *testing.T) {
	p, _ := Get("redmine")
	options, err := p.ListAssignableUsers(context.Background(), Config{"base_url": "http://example.com", "api_key": "secret"}, "")
	if err != nil {
		t.Fatalf("ListAssignableUsers: %v", err)
	}
	if options != nil {
		t.Errorf("expected nil options for a blank project, got %+v", options)
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
