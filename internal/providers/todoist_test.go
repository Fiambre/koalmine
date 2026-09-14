package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTodoistFetchItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing/incorrect Authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/tasks/filter":
			if got := r.URL.Query().Get("query"); got != "assigned to: me | !assigned" {
				t.Errorf("unexpected filter query: %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					todoistTaskFixture("1", "Comprar café", "p1", 3, nil),
					todoistTaskFixture("2", "Sin prioridad", "p1", 1, nil),
				},
			})
		case "/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{"id": "p1", "name": "Inbox"}},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p := &todoistProvider{client: server.Client(), apiBase: server.URL}
	items, err := p.FetchItems(context.Background(), Config{"token": "secret"})
	if err != nil {
		t.Fatalf("FetchItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}
	if items[0].ID != "todoist:1" || items[0].Type != ItemTypeIssue || items[0].Project != "Inbox" {
		t.Errorf("unexpected item 1: %+v", items[0])
	}
	// API priority 3 -> UI label P2 (the mapping is inverted).
	if items[0].Status != "P2" {
		t.Errorf("expected priority label P2, got %q", items[0].Status)
	}
	// API priority 1 (no priority set) shouldn't produce a misleading badge.
	if items[1].Status != "" {
		t.Errorf("expected no priority label for the default priority, got %q", items[1].Status)
	}
	if items[0].Closed || items[1].Closed {
		t.Errorf("active tasks from /tasks/filter should never be reported closed: %+v", items)
	}
	if !items[0].CreatedByMe {
		t.Errorf("expected CreatedByMe true (see todoistProvider's doc comment)")
	}
}

func TestTodoistFetchItemsMissingToken(t *testing.T) {
	p := &todoistProvider{client: defaultHTTPClient(), apiBase: todoistAPIBase}
	if _, err := p.FetchItems(context.Background(), Config{}); err == nil {
		t.Error("expected an error when the token is missing")
	}
}

func TestTodoistListProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"id": "p1", "name": "Inbox"},
				{"id": "p2", "name": "Trabajo"},
			},
		})
	}))
	defer server.Close()

	p := &todoistProvider{client: server.Client(), apiBase: server.URL}
	options, err := p.ListProjects(context.Background(), Config{"token": "secret"})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(options) != 2 || options[0].Value != "p1" || options[0].Label != "Inbox" {
		t.Errorf("unexpected options: %+v", options)
	}
}

func TestTodoistSearchItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tasks/filter":
			if got := r.URL.Query().Get("query"); got != "search: build" {
				t.Errorf("unexpected filter query: %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{todoistTaskFixture("5", "Arreglar el build", "p1", 1, nil)},
			})
		case "/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{"id": "p1", "name": "Inbox"}},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p := &todoistProvider{client: server.Client(), apiBase: server.URL}
	items, err := p.SearchItems(context.Background(), Config{"token": "secret"}, "build")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) != 1 || items[0].ID != "todoist:5" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestTodoistSearchItemsEmptyQuery(t *testing.T) {
	p := &todoistProvider{client: defaultHTTPClient(), apiBase: todoistAPIBase}
	items, err := p.SearchItems(context.Background(), Config{"token": "secret"}, "   ")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected no items for a blank query, got %d", len(items))
	}
}

func TestTodoistFetchComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/comments" || r.URL.Query().Get("task_id") != "42" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"content": "Un comentario", "posted_at": "2026-09-01T10:00:00.000000Z"},
			},
		})
	}))
	defer server.Close()

	p := &todoistProvider{client: server.Client(), apiBase: server.URL}
	comments, err := p.FetchComments(context.Background(), Config{"token": "secret"}, TaskItem{ID: "todoist:42"})
	if err != nil {
		t.Fatalf("FetchComments: %v", err)
	}
	// Author is intentionally left blank — see the type doc comment.
	if len(comments) != 1 || comments[0].Body != "Un comentario" || comments[0].Author != "" {
		t.Errorf("unexpected comments: %+v", comments)
	}
}

func TestTodoistFetchItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tasks/42":
			completed := "2026-09-05T10:00:00.000000Z"
			_ = json.NewEncoder(w).Encode(todoistTaskFixture("42", "Arreglar el build", "p1", 1, &completed))
		case "/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{"id": "p1", "name": "Inbox"}},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p := &todoistProvider{client: server.Client(), apiBase: server.URL}
	fresh, err := p.FetchItem(context.Background(), Config{"token": "secret"}, TaskItem{ID: "todoist:42"})
	if err != nil {
		t.Fatalf("FetchItem: %v", err)
	}
	if fresh.Title != "Arreglar el build" || fresh.Project != "Inbox" {
		t.Errorf("unexpected item: %+v", fresh)
	}
	if !fresh.Closed {
		t.Errorf("expected a task with completed_at set to be reported closed")
	}
}

func TestTodoistFetchItemNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := &todoistProvider{client: server.Client(), apiBase: server.URL}
	_, err := p.FetchItem(context.Background(), Config{"token": "secret"}, TaskItem{ID: "todoist:999"})
	if err == nil {
		t.Fatal("expected an error for a task that no longer exists")
	}
}

func TestTodoistCreateItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing/incorrect Authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/tasks":
			var body struct {
				Content     string `json:"content"`
				Description string `json:"description"`
				ProjectID   string `json:"project_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding request body: %v", err)
			}
			if body.Content != "Nueva tarea" || body.ProjectID != "p1" {
				t.Errorf("unexpected request body: %+v", body)
			}
			_ = json.NewEncoder(w).Encode(todoistTaskFixture("99", "Nueva tarea", "p1", 1, nil))
		case r.Method == http.MethodGet && r.URL.Path == "/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{"id": "p1", "name": "Inbox"}},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p := &todoistProvider{client: server.Client(), apiBase: server.URL}
	item, err := p.CreateItem(context.Background(), Config{"token": "secret"}, CreateItemInput{
		Project:     "p1",
		Title:       "Nueva tarea",
		Description: "Detalle",
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if item.ID != "todoist:99" || item.Type != ItemTypeIssue || item.Project != "Inbox" {
		t.Errorf("unexpected item: %+v", item)
	}
}

func TestTodoistCreateItemMissingFields(t *testing.T) {
	p := &todoistProvider{client: defaultHTTPClient(), apiBase: todoistAPIBase}
	if _, err := p.CreateItem(context.Background(), Config{"token": "secret"}, CreateItemInput{Title: "x"}); err == nil {
		t.Error("expected an error when project is missing")
	}
	if _, err := p.CreateItem(context.Background(), Config{"token": "secret"}, CreateItemInput{Project: "p1"}); err == nil {
		t.Error("expected an error when title is missing")
	}
}

func TestTodoistTestConnectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	p := &todoistProvider{client: server.Client(), apiBase: server.URL}
	if err := p.TestConnection(context.Background(), Config{"token": "wrong"}); err == nil {
		t.Error("expected TestConnection to fail on a 401 response")
	}
}

func TestTodoistPriorityLabel(t *testing.T) {
	cases := map[int]string{0: "", 1: "", 2: "P3", 3: "P2", 4: "P1", 5: ""}
	for apiPriority, want := range cases {
		if got := todoistPriorityLabel(apiPriority); got != want {
			t.Errorf("todoistPriorityLabel(%d) = %q, want %q", apiPriority, got, want)
		}
	}
}

func todoistTaskFixture(id, content, projectID string, priority int, completedAt *string) map[string]any {
	f := map[string]any{
		"id":          id,
		"content":     content,
		"description": "Descripción de " + content,
		"project_id":  projectID,
		"priority":    priority,
		"created_at":  "2026-09-01T10:00:00.000000Z",
		"updated_at":  "2026-09-01T10:00:00.000000Z",
	}
	if completedAt != nil {
		f["completed_at"] = *completedAt
	} else {
		f["completed_at"] = nil
	}
	return f
}
