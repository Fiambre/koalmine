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
					"id":         42,
					"subject":    "Arreglar el build",
					"updated_on": "2026-09-01T10:00:00Z",
					"project":    map[string]any{"name": "Koalmine"},
					"status":     map[string]any{"name": "En curso"},
					"author":     map[string]any{"name": "Rodrigo"},
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
