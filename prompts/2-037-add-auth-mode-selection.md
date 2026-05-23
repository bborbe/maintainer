---
status: draft
spec: [037-migrate-pr-watcher-to-github-app]
created: "2026-05-23T21:30:00Z"
branch: dark-factory/migrate-pr-watcher-to-github-app
---

## Summary

- Add `ResolveAuthMode` function in `pkg/authmode.go` mirroring the pattern from `agent/pr-reviewer/pkg/authmode.go`
- Add `CreateGitHubHTTPClient` factory function in `pkg/factory/factory.go` that resolves auth mode and returns the appropriate `*http.Client`
- App auth path uses `githubapp.NewClient` (auto-refreshing transport); PAT fallback returns `http.DefaultClient` with bearer token injected
- Unit tests in `pkg/authmode_test.go` cover all four auth mode cases
- Startup log declares chosen auth mode with App ID and Installation ID

## Objective

Add auth mode selection logic to the watcher. When App env vars (`APP_ID`, `INSTALLATION_ID`, `PEM_KEY`) are set, use `lib/githubapp.NewClient` for auto-refreshing IAT. When only `GH_TOKEN` is set, fall back to PAT. When neither is set, refuse to start. At startup, log the chosen mode.

## Context

Read these files before making changes:
- `/workspace/lib/githubapp/githubapp.go` — verify `NewClient` signature and auto-refresh behavior
- `/workspace/agent/pr-reviewer/pkg/authmode.go` — reference implementation for `ResolveAuthMode`
- `/workspace/agent/pr-reviewer/pkg/authmode_test.go` — reference for test patterns
- `/workspace/watcher/github-pr/pkg/factory/factory.go` — current factory that calls `NewGitHubClient`
- `/workspace/watcher/github-pr/main.go` — application struct with env var bindings

## Requirements

### 1. Create `pkg/authmode.go`

Create `watcher/github-pr/pkg/authmode.go` with the same pattern as `agent/pr-reviewer/pkg/authmode.go`:

```go
// AuthMode classifies which credential type is active at pod startup.
type AuthMode int

const (
    AuthModeNone AuthMode = iota
    AuthModeGitHubApp
    AuthModePATFallback
)

// ResolveAuthMode picks the credential type to use at pod startup.
//   - APP_ID > 0 && INSTALLATION_ID > 0 && PEM_KEY != "" → AuthModeGitHubApp
//   - GH_TOKEN != "" → AuthModePATFallback
//   - Otherwise → AuthModeNone (caller MUST refuse to start)
func ResolveAuthMode(appID, installationID int64, pemKey, ghToken string) AuthMode {
    if appID > 0 && installationID > 0 && pemKey != "" {
        return AuthModeGitHubApp
    }
    if ghToken != "" {
        return AuthModePATFallback
    }
    return AuthModeNone
}
```

### 2. Create `pkg/authmode_test.go`

Create `watcher/github-pr/pkg/authmode_test.go` with table-driven tests covering all four cases:

- App auth when all three fields set (APP_ID > 0, INSTALLATION_ID > 0, PEM_KEY != "")
- PAT fallback when App fields missing but GH_TOKEN set
- Both set: App wins, GH_TOKEN ignored
- Neither set: AuthModeNone

Mirror the table structure from `agent/pr-reviewer/pkg/authmode_test.go`.

### 3. Add `CreateGitHubHTTPClient` to `pkg/factory/factory.go`

Add a new factory function:

```go
import (
    "net/http"
    githubapp "github.com/bborbe/maintainer/lib/githubapp"
    // ... existing imports
)

// AuthConfig holds the raw env values for auth resolution.
type AuthConfig struct {
    AppID          int64
    InstallationID int64
    PEMKey         string
    GHToken        string
}

// CreateGitHubHTTPClient resolves the auth mode from AuthConfig and returns a
// configured *http.Client suitable for use with NewGitHubClient.
//   - App auth: returns *http.Client with ghinstallation/v2 transport (auto-refresh)
//   - PAT fallback: returns *http.Client with bearer token injected via WithAuthToken
//   - Neither configured: returns an error naming both env-var sets
//   - Both configured: App wins, logs a warning, returns the App client
func CreateGitHubHTTPClient(ctx context.Context, cfg AuthConfig) (*http.Client, AuthMode, error) {
    mode := ResolveAuthMode(cfg.AppID, cfg.InstallationID, cfg.PEMKey, cfg.GHToken)
    switch mode {
    case AuthModeGitHubApp:
        glog.V(2).Infof("watcher auth mode=github-app app_id=%d installation_id=%d", cfg.AppID, cfg.InstallationID)
        httpClient, err := githubapp.NewClient(ctx, githubapp.Config{
            AppID:          cfg.AppID,
            InstallationID: cfg.InstallationID,
            PEM:            []byte(cfg.PEMKey),
        })
        if err != nil {
            return nil, AuthModeNone, errors.Wrap(ctx, err, "create github app client")
        }
        return httpClient, mode, nil
    case AuthModePATFallback:
        glog.Warningf("watcher auth mode=pat-fallback (legacy GH_TOKEN — migrate to GitHub App)")
        client := gogithub.NewClient(nil).WithAuthToken(cfg.GHToken)
        return client.BaseURL.Client(), mode, nil
    default:
        return nil, AuthModeNone, errors.Errorf(
            ctx,
            "watcher auth: neither App nor PAT configured — set APP_ID+INSTALLATION_ID+PEM_KEY, or set GH_TOKEN",
        )
    }
}
```

Note: `gogithub.BaseURL().Client()` returns the underlying `*http.Client` from the `*github.Client`. Since `gogithub.NewClient(nil).WithAuthToken(token)` wraps a default transport, this gives a working PAT-authenticated client.

### 4. Update `CreateWatcher` signature and body in `pkg/factory/factory.go`

Change the `CreateWatcher` signature to accept `*http.Client` as first parameter (after ctx), replacing the `ghToken string` parameter that will be removed from the caller signature:

```go
func CreateWatcher(
    ctx context.Context,
    ghClient *http.Client,  // replaces ghToken string
    brokers libkafka.Brokers,
    stage string,
    // ... rest unchanged
) (pkg.Watcher, func(), error)
```

Inside the function body, replace:
```go
ghClient := pkg.NewGitHubClient(ghToken)
```
with:
```go
ghClient := pkg.NewGitHubClient(ghClient)
```

Note: `http.Client` from `gogithub.BaseURL().Client()` implements `http.RoundTripper`, so it works with `gogithub.NewClient(httpClient)`.

### 5. Update `pkg/factory/factory_test.go` if needed

If there are existing tests calling `CreateWatcher` with a token string, update them to pass an `*http.Client` instead. Since there are no existing factory tests (the glob showed zero `_test.go` files), this step is likely a no-op — just verify with `make test`.

## Constraints

- MUST use `lib/githubapp.NewClient` for App auth — NOT `MintIAT`
- MUST NOT log PEM bytes or IAT bytes (beyond prefix — the lib already handles this)
- MUST use `github.com/bborbe/errors` for all error wrapping
- BSD-style license header on new files
- Keep `GH_TOKEN` as legacy fallback — do not remove

## Verification

```bash
cd /workspace/watcher/github-pr && make test
```

Expected: auth mode tests pass, all existing tests pass.