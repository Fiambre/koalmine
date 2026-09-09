---
name: koalmine-dev
description: Architecture, conventions, and gotchas for developing Koalmine — a Wails v2 (Go) + Svelte 5 desktop tray app that surfaces tasks from Redmine/GitHub/GitLab (repo: https://github.com/Fiambre/koalmine). Load this whenever working anywhere in this repo — adding a task provider, touching the tray/poller/updater/settings code, writing or running tests, building/packaging, or cutting a release — even if the user's request doesn't mention "skill" or "architecture" explicitly. It exists to stop a fresh session from re-deriving (or re-breaking) things already figured out here.
---

# Developing Koalmine

Koalmine is a desktop tray app: it lives in the system tray, polls task
sources (Redmine, GitHub, GitLab today) for issues/PRs/MRs assigned to the
user, and notifies on new ones. Stack: **Wails v2** (Go backend + webview),
**Svelte 5 + TypeScript** frontend, **getlantern/systray** for the tray icon
(Wails v2 has no native tray — that's v3, still alpha). Windows is the only
platform actually tested so far; macOS/Linux code paths exist but are
unverified.

Read this whole file before making non-trivial changes — most of it exists
because something broke in a non-obvious way the first time, and the fix
isn't in the code comments.

## Architecture

### Providers are a self-registering "plugin" pattern, not real plugins

`internal/providers/` holds one file per connector (`redmine.go`,
`github.go`, `gitlab.go`). Each calls `Register(name, factory)` from its own
`init()` into `registry.go`'s catalog. **Adding a new provider is one new
file — nothing else in the app changes**, because:

- `registry.go`'s `List()`/`Get()` discover it automatically.
- The `Settings.svelte` form is generated from each provider's
  `ConfigFields()` metadata (key/label/kind/required) — there's no
  per-provider UI to write.
- `internal/poller` iterates `providers.List()` too.

A real dynamic-plugin system (separate process + RPC, à la HashiCorp's
`go-plugin`) was considered and rejected: Go's native `plugin` package
doesn't work on Windows at all, and RPC-based plugins are a lot of
infrastructure for what's currently 3 connectors. If a future need
outgrows this (third-party connectors, hot-swapping without a rebuild),
that tradeoff is worth revisiting — but don't reach for it by default.

Each provider implements:
```go
Name() string
DisplayName() string
ConfigFields() []ConfigField     // drives the generic Settings form
TestConnection(ctx, cfg) error
FetchItems(ctx, cfg) ([]TaskItem, error)
```

**Every provider has a documented gap, and that's fine — don't try to force
symmetry.** Redmine has no PR/MR concept and no mentions API, so it only
returns issues. GitLab has no unified "mentioned me" search across
issues/MRs the way GitHub's `mentions:@me` does, so GitLab skips mentions
too. When adding a provider, figure out what its API actually supports
rather than assuming it must match the others — and leave a comment
explaining the gap the same way the existing providers do, so the next
person doesn't think it's an oversight.

**Watch for ID collisions across resource types.** GitLab's `gitlab.go` is
the cautionary example: issues and merge requests have independent numeric
ID sequences, so an issue #42 and an MR #42 could collide if `TaskItem.ID`
were just `"gitlab:42"`. It's `"gitlab:issue:42"` / `"gitlab:pr:42"`
instead. Check this for any new provider with more than one resource type.

### Storage is JSON files, deliberately not a database

`internal/store/` persists three things, each a simple `Load()`/`Save()`
pair over a JSON file in `os.UserConfigDir()/Koalmine/`:
- `config.json` — non-secret settings (which providers are enabled, base
  URLs, poll interval).
- `state.json` — which `TaskItem.ID`s have already been notified about
  (dedup for the poller).
- `tasks.json` — the last full snapshot of tasks, so the window has
  something to show immediately on launch instead of "Cargando…".

This was a deliberate choice over SQLite/bbolt: every access pattern here
is "replace the whole thing, read the whole thing back," with no querying
needed server-side (filtering already happens client-side in Svelte). A
real database would add a schema and a CGO/pure-Go driver dependency to
solve a problem this small. If a future need actually requires querying
(e.g. task history, search), that's the point to reconsider — not before.

Secrets (API keys/tokens) **never** touch these JSON files — they go to the
OS keychain via `github.com/zalando/go-keyring` (Windows Credential
Manager / macOS Keychain / Linux Secret Service). `internal/store/secrets.go`
wraps it; `internal/store/resolve.go`'s `ResolveConfig` is the one place
that merges a provider's non-secret values (from config.json) with its
secret values (from the keychain) into the `providers.Config` map a
provider actually receives.

`configDir` in `config.go` is a package-level `var`, not a `const` function
call, specifically so tests can override it to a temp dir
(`withTempConfigDir` in `config_test.go`) instead of touching the real
user config directory. Follow that pattern for any new store file.

### Background work: goroutines with callback hooks, not direct coupling

`internal/poller`, `internal/notify`, and `internal/updater` are started
from `app.go`'s `startup(ctx)` as goroutines. Each exposes a callback
(`Poller.OnUpdate`, `App.onUpdateAvailable`) rather than the caller reaching
into it directly — e.g. `tray.go` sets `app.onUpdateAvailable` so `app.go`
never needs to `import "github.com/getlantern/systray"`. Keep that
direction when adding new background work: the goroutine/package doesn't
know who's listening, it just calls its hook if one is set.

The poller's dedup and the updater's version-comparison logic
(`unseenItems` in `poller.go`, `evaluateRelease` in `updater.go`) are
deliberately split out as pure functions taking plain data, separate from
the HTTP/filesystem code that calls them. That's not incidental style —
it's what makes them unit-testable without mocking I/O. Do this split for
any new decision logic that's more complex than a one-liner.

## Two gotchas that cost real debugging time

**1. systray + WebView2 on Windows need explicit COM initialization.**
`getlantern/systray`'s `init()` calls `runtime.LockOSThread()`, and
`main()` calls `systray.Run(onTrayReady, onTrayExit)` on the main
goroutine — so Wails' `wails.Run()` has to happen on a *different*
goroutine (see `main.go`'s `runWails()`, launched via `go runWails()` from
`tray.go`'s `onTrayReady`). But that goroutine runs on a fresh OS thread
that never had COM initialized, and WebView2's environment creation fails
with `"CoInitialize has not been called"` if you skip this. The fix is
`platform_windows.go`'s `lockRenderThread()` — `runtime.LockOSThread()` +
`windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)` — called at
the top of `runWails()`. `platform_other.go` is the no-op counterpart for
macOS/Linux (untested — their own thread-affinity needs, Cocoa's main
thread and GTK's main loop, are assumed to be handled by systray/wails
themselves, but nobody's verified that on this project).

**2. Svelte 5's legacy component API fails silently.** The Wails
`svelte-ts` scaffold's `main.ts` originally used `new App({target})`, which
Svelte 5 (as resolved by the template's `^5.55.7` range) rejects at
runtime with `component_api_invalid_new` — and the failure is a blank navy
window with *no visible error* unless you open DevTools. `main.ts` now uses
`mount(App, {target})` from `'svelte'` instead. If a blank window ever
comes back after touching frontend entry-point code, check this first
before assuming it's a Go-side problem.

## Testing

`go test ./...` covers everything that can run offline and deterministically:
- Every provider's HTTP calls are mocked with `httptest.Server` (see
  `redmine_test.go`, `github_test.go`, `gitlab_test.go` for the pattern —
  assert on request headers/paths in the handler, return canned JSON).
- `go-keyring` has a `keyring.MockInit()` for exactly this purpose — use it
  in any test touching `store.SetSecret`/`GetSecret` rather than writing to
  the real OS keychain. `autostart_windows_test.go` is the one place that
  *does* touch a real OS resource (HKCU's Run key) — deliberately, since
  the whole point is verifying real Windows registry behavior — and it
  reads the original state first and restores it via `t.Cleanup`, so it
  never leaves the machine in a different state than it found it.
- Pure decision-logic functions (`unseenItems`, `evaluateRelease`,
  `projectFromReference`) get direct unit tests with no I/O at all.

**Not covered by automated tests, and don't try to force it:**
`updater.Apply`/`Relaunch` (real file replacement + real process spawn),
the darwin/linux `autostart` implementations (no machine to test on), and
anything requiring the actual WebView2-hosted frontend (see below). These
get manual verification instead — that's a legitimate choice, not a gap to
apologize for.

## Frontend / Wails bindings

`frontend/wailsjs/` is generated, not hand-written. After adding or
changing any bound `App` method, or any Go struct referenced by one, run:
```
wails generate module
```
then check `frontend/wailsjs/go/main/App.d.ts` and `models.ts` for the
actual generated signatures/field names before writing the corresponding
Svelte code — don't guess at what Wails will call a field. JSON tags on Go
structs control the generated TS field names (camelCase by convention here
— see `ConfigField`/`TaskItem`/`ProviderInfo` in `internal/providers` and
`app.go`); a struct field with no JSON tag comes through as its Go name
verbatim, which breaks the camelCase-everywhere convention.

`Settings.svelte` renders provider forms from `ConfigFields()` metadata via
`ListProviders()` — don't add per-provider markup there. If a provider
needs a form element the generic renderer can't produce, that's a sign the
`ConfigField`/`FieldKind` model needs a new case, not a one-off branch in
the Svelte template.

A known warning during `wails build`/`wails generate module` —
`Not found: time.Time` — is benign; it's Wails' TS generator shrugging at
`time.Time` fields (they come through as `updatedAt: any`, see
`TaskItem.UpdatedAt`). Ignore it.

## Build & verify loop

Go side, run after any Go change:
```
gofmt -l -w .
go vet ./...
go test ./...
go build -o build/bin/koalmine-test.exe .   # then delete it — it's a throwaway smoke build
```

For interactive checks, `wails dev` needs to run detached with output
redirected to files, since this environment's shell isn't interactive:
```
Start-Process -FilePath "wails" -ArgumentList "dev" -PassThru `
  -RedirectStandardOutput "$env:TEMP\wails_dev_out.log" `
  -RedirectStandardError "$env:TEMP\wails_dev_err.log"
```
then poll/wait on those logs for `"Environment created successfully"`
(startup done) or an error pattern, rather than sleeping a fixed amount.
Kill stray `Koalmine-dev`/`wails`/`node` processes before restarting it —
they don't always exit cleanly between runs.

**A plain headless browser cannot fully verify this app.** Wails injects
`window.go` (bound methods) and `window.runtime` (events) into the real
WebView2-hosted page; a bare `msedge --headless=new --dump-dom <devServerURL>`
hit doesn't have that bridge, so any code path that calls a bound method or
`EventsOn`/`EventsEmit` throws immediately in that context. This is still
useful for one thing — confirming the Svelte app *mounts* and renders its
initial DOM without a build/compile error, which is exactly the class of
bug that bit the `new App()` issue above — but a "stuck on Cargando…" or
similar result from a headless check doesn't mean the real app is broken;
it means the check can't exercise that path. Don't chase it further than
that. Real verification of bound methods, events, tray menu actions, and
notifications needs either a human clicking through the actual window, or
running the check *inside* `wails dev`'s hosted webview.

## Release process

Version lives in **two places that must be kept in sync by hand**:
`internal/version/version.go`'s `Current` constant, and `wails.json`'s
`info.productVersion` (NSIS reads the latter for the installer's
metadata — there's no build-time injection wiring it from one source
today). `RELEASING.md` has the exact steps.

The NSIS installer (`build/windows/installer/project.nsi`) installs
per-user (`WAILS_INSTALL_SCOPE`/`REQUEST_EXECUTION_LEVEL` both `"user"`),
specifically so the running app can overwrite its own executable during
auto-update without a UAC prompt. Don't change this to machine-wide scope
without also reworking the updater — it doesn't handle needing elevation.

`.github/workflows/release.yml` builds and publishes on any `v*` tag push,
renaming the build outputs to the fixed names `internal/updater`'s
`assetName()` expects: `koalmine-windows-amd64.exe` (raw binary, what
self-update downloads) and `koalmine-windows-amd64-installer.exe` (NSIS
installer, for first-time installs). If either naming convention changes,
update it in both places — the workflow and `updater.go` — or the
self-update will silently find no matching asset.

## Also worth knowing

- The repo is public: `https://github.com/Fiambre/koalmine`. Pushing
  commits there, and especially pushing release tags (which spends GitHub
  Actions minutes and creates a public release), are visible actions —
  confirm with the user before doing either rather than assuming a standing
  green light.
- No code signing (Authenticode) exists yet — Windows shows an
  "unverified publisher" warning on the installer, and SmartScreen may flag
  the downloaded binary. There's no certificate available; this is a known,
  accepted gap, not a bug to silently work around.
