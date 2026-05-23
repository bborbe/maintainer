---
status: draft
spec: [037-migrate-pr-watcher-to-github-app]
created: "2026-05-23T21:30:00Z"
branch: dark-factory/migrate-pr-watcher-to-github-app
---

## Summary

- `NewGitHubClient` in `watcher/github-pr/pkg/githubclient.go` now accepts `*http.Client` instead of a token string, enabling it to use the auto-refreshing IAT transport from `lib/githubapp`
- All callers in `watcher/github-pr/pkg/factory/factory.go` updated to pass `*http.Client`

## Objective

Refactor `pkg.NewGitHubClient` to accept an `*http.Client` so the watcher can inject the `ghinstallation/v2` transport that auto-refreshes the IAT.

## Context

Read these files before making changes:
- `/workspace/watcher/github-pr/pkg/githubclient.go`
- `/workspace/watcher/github-pr/pkg/factory/factory.go`

This change is the first step in a larger migration from PAT to GitHub App auth.

## Requirements

1. In `watcher/github-pr/pkg/githubclient.go`, change the `NewGitHubClient` function signature from:
   ```go
   func NewGitHubClient(token string) GitHubClient
   ```
   to:
   ```go
   func NewGitHubClient(httpClient *http.Client) GitHubClient
   ```

2. Inside the `githubClient` struct, keep the existing `client *gogithub.Client` field. The constructor now assigns via `.WithAuthToken` not needed — instead call `gogithub.NewClient(httpClient)` directly since the injected client already carries the auth transport.

3. In `watcher/github-pr/pkg/factory/factory.go`, update the `CreateWatcher` function and its call to `pkg.NewGitHubClient`:
   - Add `httpClient *http.Client` as a parameter to `CreateWatcher`
   - Pass that `httpClient` to `pkg.NewGitHubClient(httpClient)` instead of the token string
   - Do NOT add auth resolution logic here — that lives in a later prompt (`2-037-add-auth-mode-selection.md`). Leave a `// TODO(spec-037): pass authenticated http.Client from auth resolution` comment.

4. Update every caller of `CreateWatcher` in `watcher/github-pr/main.go`:
   - Add `*http.Client` parameter placeholder (use `nil` for now — the auth factory is added in the next prompt)
   - Add a `// TODO(spec-037): inject authenticated http.Client from auth mode selection` comment

5. Run `cd /workspace/watcher/github-pr && make test` to confirm existing tests still pass. If compilation errors appear in test files due to the signature change, fix them by updating `NewForTest` calls if needed (the export test helper in `githubclient_export_test.go` injects `*gogithub.Client` — no change needed there since `*http.Client` implements `.RoundTripper`).

## Constraints

- Do NOT change the `GitHubClient` interface or any other method signatures
- Do NOT add any auth resolution logic in this prompt
- Errors: use `github.com/bborbe/errors` for any new error wrapping
- BSD-style license header on any new files

## Verification

```bash
cd /workspace/watcher/github-pr && make test
```

Expected: all tests pass; `make precommit` exits 0 after all prompts complete.