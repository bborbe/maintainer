---
status: draft
spec: [037-migrate-pr-watcher-to-github-app]
created: "2026-05-23T21:30:00Z"
branch: dark-factory/migrate-pr-watcher-to-github-app
---

## Summary

- Add ginkgo unit tests for `CreateGitHubHTTPClient` in `pkg/factory/factory_test.go` covering all four auth mode cases with a mock GitHub API server
- Add `## Unreleased` entry to `CHANGELOG.md` describing the migration
- Run `make precommit` to verify all tests pass and code is linted

## Objective

Add unit tests for the auth mode selection factory function and write the CHANGELOG entry. The auth mode selection is a critical behavior that must be tested. This prompt also serves as the final validation step — `make precommit` must exit 0.

## Context

Read these files before making changes:
- `/workspace/watcher/github-pr/pkg/factory/factory.go` — the factory with `CreateGitHubHTTPClient`
- `/workspace/watcher/github-pr/pkg/suite_test.go` — test suite pattern
- `/workspace/watcher/github-pr/pkg/githubclient_test.go` — reference for httptest.Server patterns in this package
- `/workspace/CHANGELOG.md` — existing changelog format

## Requirements

### 1. Create `pkg/factory/factory_test.go`

Create a ginkgo test file that tests `CreateGitHubHTTPClient`. Use `httptest.Server` with a mock handler that returns JSON responses to verify the HTTP client is properly configured.

Test cases:
- **App auth happy path**: all three App fields set, `ghinstallation/v2` transport mints IAT, server returns 200
- **PAT fallback happy path**: App fields empty, GH_TOKEN set, client authenticates with bearer token
- **Both set (App wins)**: all fields set, App path wins, warning log emitted (check via captured log or by verifying App client returned)
- **Neither set (error)**: all fields empty, error returned naming both env-var sets
- **Partial App config (error)**: e.g. only APP_ID set, error naming the missing field

For the App auth path with `ghinstallation/v2`, the library internally calls GitHub's API to mint the IAT. Since the test must not call real GitHub, use a mock server that:
1. For `POST /app/installations/{id}/access_tokens` — returns a fake IAT token
2. For any subsequent `/search/issues` — returns empty results

Use `libtime.DateTime` for any time fields as done elsewhere in the codebase.

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

// (tests go here)
```

Use the pattern from `watcher/github-pr/pkg/githubclient_test.go` for the httptest setup, and mirror the table structure from `agent/pr-reviewer/pkg/authmode_test.go` for the parameterised cases.

### 2. Add `## Unreleased` entry to `CHANGELOG.md`

After the existing `## v0.25.12` heading (or wherever `## Unreleased` currently is), add:

```markdown
## Unreleased

- feat(watcher/github-pr): migrate from PAT auth (`GH_TOKEN`) to GitHub App auth using `lib/githubapp.NewClient` with auto-refreshing IAT transport; legacy `GH_TOKEN` retained as fallback; dev uses App 3800041 / Install 134435225, prod uses App 3798945 / Install 134414316 (spec 037)
```

Use `feat:` prefix since this adds new functionality. Follow the existing changelog style — one bullet per logical change.

### 3. Run precommit

```bash
cd /workspace/watcher/github-pr && make precommit
```

Expected: exit 0, final output line `ready to commit`.

If lint fails, fix only the failing lint issue (do not re-run full precommit until all individual targets pass). Then re-run the failing target, then `make precommit`.

If test coverage is below 80% for changed packages, add missing tests for:
- Auth mode selection logic (covered above)
- `NewGitHubClient` with `*http.Client` input (may already be covered by existing tests via `NewForTest` — verify with `make test -coverprofile`)

## Constraints

- Tests must cover all four auth mode cases
- Tests must not call the real GitHub API — use `httptest.Server`
- Changelog entry must use `feat:` prefix
- `make precommit` must exit 0 before declaring complete

## Verification

```bash
cd /workspace/watcher/github-pr && make precommit
```

Expected: all tests pass, lint passes, license check passes, final output `ready to commit`.