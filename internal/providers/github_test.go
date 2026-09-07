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

		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")

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
