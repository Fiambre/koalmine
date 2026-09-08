// Package updater checks GitHub Releases for a newer version of Koalmine
// and, if the user asks for it, downloads and applies it in place.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	update "github.com/inconshreveable/go-update"
	"golang.org/x/mod/semver"

	"koalmine/internal/version"
)

const (
	repoOwner = "Fiambre"
	repoName  = "koalmine"

	// CheckInterval is how often the background check re-runs.
	CheckInterval = 12 * time.Hour
)

// apiBaseURL is overridden in tests to point at an httptest.Server instead
// of the real GitHub API.
var apiBaseURL = "https://api.github.com"

// Info describes the result of a Check.
type Info struct {
	Available   bool   `json:"available"`
	Version     string `json:"version"`
	DownloadURL string `json:"-"` // never sent to the frontend; ApplyUpdate resolves it server-side
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// assetName is the fixed filename the release workflow publishes the raw
// binary under, e.g. "koalmine-windows-amd64.exe".
func assetName() string {
	name := fmt.Sprintf("koalmine-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// Check queries GitHub's latest release and reports whether it's newer than
// the running version. No releases published yet (404) is not an error —
// it just means nothing to report.
func Check(ctx context.Context) (Info, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBaseURL, repoOwner, repoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Info{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Info{}, fmt.Errorf("no se pudo conectar a GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Info{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return Info{}, fmt.Errorf("GitHub respondió %s", resp.Status)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return Info{}, fmt.Errorf("respuesta inválida de GitHub: %w", err)
	}

	return evaluateRelease(release, "v"+version.Current)
}

// evaluateRelease is the pure decision logic behind Check, split out so it
// can be unit-tested without an HTTP round-trip.
func evaluateRelease(release githubRelease, currentVersion string) (Info, error) {
	latest := release.TagName
	if !semver.IsValid(latest) {
		return Info{}, fmt.Errorf("tag de release inválido: %q", latest)
	}

	if semver.Compare(latest, currentVersion) <= 0 {
		return Info{}, nil
	}

	want := assetName()
	for _, asset := range release.Assets {
		if asset.Name == want {
			return Info{Available: true, Version: latest, DownloadURL: asset.BrowserDownloadURL}, nil
		}
	}
	return Info{}, fmt.Errorf("la release %s no incluye el binario %s", latest, want)
}

// Apply downloads the binary at downloadURL and atomically replaces the
// currently running executable with it.
func Apply(ctx context.Context, downloadURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("no se pudo descargar la actualización: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("la descarga falló: %s", resp.Status)
	}

	if err := update.Apply(resp.Body, update.Options{}); err != nil {
		return fmt.Errorf("no se pudo reemplazar el ejecutable: %w", err)
	}
	return nil
}

// Relaunch starts a new instance of the currently running executable. The
// caller is responsible for exiting the current process right after.
func Relaunch() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	return exec.Command(exePath).Start()
}
