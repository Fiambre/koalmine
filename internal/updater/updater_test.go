package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func release(tag string, assetNames ...string) githubRelease {
	r := githubRelease{TagName: tag}
	for _, name := range assetNames {
		r.Assets = append(r.Assets, struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		}{Name: name, BrowserDownloadURL: "https://example.com/" + name})
	}
	return r
}

func TestEvaluateReleaseNewerVersionAvailable(t *testing.T) {
	r := release("v0.2.0", assetName())
	info, err := evaluateRelease(r, "v0.1.0")
	if err != nil {
		t.Fatalf("evaluateRelease: %v", err)
	}
	if !info.Available || info.Version != "v0.2.0" {
		t.Errorf("expected an available v0.2.0 update, got %+v", info)
	}
	if info.DownloadURL == "" {
		t.Error("expected a non-empty download URL")
	}
}

func TestEvaluateReleaseAlreadyUpToDate(t *testing.T) {
	r := release("v0.1.0", assetName())
	info, err := evaluateRelease(r, "v0.1.0")
	if err != nil {
		t.Fatalf("evaluateRelease: %v", err)
	}
	if info.Available {
		t.Errorf("expected no update when already on the latest version, got %+v", info)
	}
}

func TestEvaluateReleaseOlderThanCurrent(t *testing.T) {
	// Can happen if the user is running a pre-release/dev build ahead of
	// the latest published tag — should never report a "downgrade" as available.
	r := release("v0.1.0", assetName())
	info, err := evaluateRelease(r, "v0.5.0")
	if err != nil {
		t.Fatalf("evaluateRelease: %v", err)
	}
	if info.Available {
		t.Errorf("expected no update when the release is older than current, got %+v", info)
	}
}

func TestEvaluateReleaseSemverPointComparison(t *testing.T) {
	// v0.10.0 must be treated as newer than v0.9.0 (not a lexicographic "0.10 < 0.9").
	r := release("v0.10.0", assetName())
	info, err := evaluateRelease(r, "v0.9.0")
	if err != nil {
		t.Fatalf("evaluateRelease: %v", err)
	}
	if !info.Available {
		t.Error("expected v0.10.0 to be treated as newer than v0.9.0")
	}
}

func TestEvaluateReleaseMissingExpectedAsset(t *testing.T) {
	r := release("v0.2.0", "some-other-file.exe")
	_, err := evaluateRelease(r, "v0.1.0")
	if err == nil {
		t.Error("expected an error when the release doesn't include the expected asset")
	}
}

func TestEvaluateReleaseInvalidTag(t *testing.T) {
	r := release("not-a-version", assetName())
	_, err := evaluateRelease(r, "v0.1.0")
	if err == nil {
		t.Error("expected an error for a non-semver tag")
	}
}

func TestCheckReturnsEmptyOn404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	orig := apiBaseURL
	apiBaseURL = server.URL
	defer func() { apiBaseURL = orig }()

	info, err := Check(context.Background())
	if err != nil {
		t.Fatalf("expected no error when there are no releases yet, got %v", err)
	}
	if info.Available {
		t.Errorf("expected no update available, got %+v", info)
	}
}

func TestCheckParsesRealResponseShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(release("v9.9.9", assetName()))
	}))
	defer server.Close()

	orig := apiBaseURL
	apiBaseURL = server.URL
	defer func() { apiBaseURL = orig }()

	info, err := Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !info.Available || info.Version != "v9.9.9" {
		t.Errorf("expected an available v9.9.9 update, got %+v", info)
	}
}
