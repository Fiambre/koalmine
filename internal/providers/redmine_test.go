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
		if r.URL.Path != "/issues.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issues": []map[string]any{
				{
					"id":          42,
					"subject":     "Arreglar el build",
					"description": "El build falla en CI desde el commit abc123.",
					"updated_on":  "2026-09-01T10:00:00Z",
					"project":     map[string]any{"name": "Koalmine"},
					"status":      map[string]any{"name": "En curso"},
					"author":      map[string]any{"name": "Rodrigo"},
				},
			},
		})
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
}

func TestRedmineFetchItemsMissingConfig(t *testing.T) {
	p, _ := Get("redmine")
	if _, err := p.FetchItems(context.Background(), Config{}); err == nil {
		t.Error("expected an error when base_url/api_key are missing")
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
