---
status: rejected
spec: [037-migrate-pr-watcher-to-github-app]
created: "2026-05-23T21:30:00Z"
branch: dark-factory/migrate-pr-watcher-to-github-app
rejected: "2026-05-23T21:35:35Z"
rejected_reason: Generator produced compile-breakers + cross-prompt overlap. Hand-writing leaner replacements.
---

## Summary

- Add `APP_ID`, `INSTALLATION_ID`, `PEM_KEY` fields to the `application` struct in `main.go`
- Change `GH_TOKEN` from `required:"true"` to `required:"false"` with usage string mentioning "legacy fallback"
- Wire `CreateGitHubHTTPClient` at the start of `Run()` and pass the `*http.Client` to `CreateWatcher`
- Startup log from `CreateGitHubHTTPClient` is already emitted there — no duplicate logging needed

## Objective

Wire new GitHub App env vars into the watcher binary's main entry point. When App credentials are present, use them (auto-refreshing IAT via `ghinstallation/v2`). When only `GH_TOKEN` is present, fall back to PAT. When neither is present, refuse to start.

## Context

Read these files before making changes:
- `/workspace/watcher/github-pr/main.go` — current main entry point
- `/workspace/watcher/github-pr/pkg/factory/factory.go` — updated `CreateGitHubHTTPClient` and `CreateWatcher`
- `/workspace/agent/pr-reviewer/main.go` — reference for env var patterns (APP_ID, INSTALLATION_ID, PEM_KEY fields)
- `/workspace/lib/githubapp/githubapp.go` — Config struct and NewClient signature

## Requirements

### 1. Add new fields to `application` struct in `main.go`

Add after the existing `GHToken` field:

```go
// GitHub App authentication. When AppID + InstallationID + PEMKey are all set,
// the watcher uses GitHub App auth with auto-refreshing IAT transport.
// Legacy GH_TOKEN env stays accepted as a fallback (see Run() for resolution order).
AppID          int64  `required:"false" arg:"app-id"          env:"APP_ID"           usage:"GitHub App ID (numeric); when set, App auth is used instead of GH_TOKEN"`
InstallationID int64  `required:"false" arg:"installation-id" env:"INSTALLATION_ID"  usage:"GitHub App Installation ID (numeric)"`
PEMKey         string `required:"false" arg:"pem-key"         env:"PEM_KEY"          usage:"GitHub App private key (PEM) as env var content; mutually exclusive with PEM_KEY_FILE" display:"length"`
```

### 2. Update `GHToken` field to `required:"false"` with legacy fallback usage

Change the existing `GHToken` field:

```go
GHToken string `required:"false" arg:"gh-token" env:"GH_TOKEN" usage:"GitHub token (legacy fallback; migrate to GitHub App via APP_ID+INSTALLATION_ID+PEM_KEY)" display:"length"`
```

### 3. Wire auth resolution and pass `*http.Client` to `CreateWatcher`

In the `Run()` method, after `validateConfig()` and before `CreateWatcher`:

```go
// Resolve auth mode: prefer App auth, fall back to PAT, refuse if neither set.
httpClient, _, err := factory.CreateGitHubHTTPClient(ctx, factory.AuthConfig{
    AppID:          a.AppID,
    InstallationID: a.InstallationID,
    PEMKey:         a.PEMKey,
    GHToken:        a.GHToken,
})
if err != nil {
    return err
}
```

Note: the second return value (`AuthMode`) is ignored here — the log line is already emitted inside `CreateGitHubHTTPClient`.

### 4. Update `CreateWatcher` call signature

In `main.go`, find the `factory.CreateWatcher` call and update parameters:

- First argument after `ctx`: replace `a.GHToken` with the resolved `httpClient`
- All other arguments remain the same

Before:
```go
w, cleanup, err := factory.CreateWatcher(
    ctx,
    a.GHToken,
    // ...
)
```

After:
```go
w, cleanup, err := factory.CreateWatcher(
    ctx,
    httpClient,
    // ...
)
```

### 5. Add `"net/http"` import if not present

Verify `net/http` is imported. If not, add it:
```go
import (
    "net/http"
    // ...
)
```

## Constraints

- Do NOT use `MintIAT` — use `NewClient` only (auto-refresh transport)
- Do NOT make `APP_ID`, `INSTALLATION_ID`, `PEM_KEY` required — they are optional (PAT fallback must work)
- Do NOT log PEM bytes or full IAT bytes
- All errors use `github.com/bborbe/errors`
- BSD-style license header on any new files

## Verification

```bash
cd /workspace/watcher/github-pr && make test
```

Run the binary with `--help` and verify new flags appear:
```bash
cd /workspace/watcher/github-pr && go run . --help | grep -E 'APP_ID|INSTALLATION_ID|PEM_KEY'
```

Expected: all three flags listed with their usage strings.