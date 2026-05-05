---
spec: ["015"]
status: draft
created: "2026-05-05T21:00:00Z"
---

<summary>
- `pkg/githubclient.go` adds a `GitHubClient` interface with `GetWorkflowRuns` and `GetDefaultBranch` methods backed by the GitHub Actions API (`GET /repos/{owner}/{repo}/actions/runs?branch=<default>&per_page=20&status=completed`)
- `WorkflowRun` struct captures the fields needed for state derivation: `WorkflowID`, `Name`, `HeadSHA`, `Conclusion`, `HTMLURL`, `CreatedAt`
- `pkg/taskid.go` adds `DeriveTaskID(owner, repo, episodeSHA string)` producing a deterministic UUID v5 from `"<owner>/<repo>#build-<episodeSHA>"` using a fixed build-watcher-specific namespace UUID
- `pkg/publisher.go` adds a `CommandPublisher` interface with `PublishCreate(ctx, params BuildTaskParams) error`; the Kafka-backed implementation marshals a `CreateTaskCommand` with `assignee: build-fixer-agent`
- Counterfeiter mocks generated for `GitHubClient`, `CommandPublisher` in `pkg/mocks/`
- Unit tests cover: workflow run API mapping, default branch fetch, task ID determinism, publisher marshalling (including idempotency of UUID5)
- `make precommit` passes in `watcher/github-build/`
</summary>

<objective>
Implement the GitHub Actions API client, task ID derivation, and Kafka publisher for the build watcher. These are leaf dependencies consumed by the watcher core (prompt 3) and factory (prompt 4). No poll loop yet — just the primitives with tests.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface→constructor→struct pattern.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo v2/Gomega, counterfeiter mocks.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — bborbe/errors, never fmt.Errorf.
Read `go-security-linting.md` in `~/.claude/plugins/marketplaces/coding/docs/` — gosec rules.

Files to read before making any changes:
- `watcher/github-pr/pkg/githubclient.go` — full file; mirror the client interface pattern and go-github usage
- `watcher/github-pr/pkg/publisher.go` — full file; mirror CreateTaskCommand construction and cdb.CommandObjectSender usage
- `watcher/github-pr/pkg/taskid.go` — full file; mirror UUID v5 derivation pattern; the build watcher uses a DIFFERENT namespace UUID
- `watcher/github-pr/pkg/mocks/` — list files; understand counterfeiter output format

**Symbol verification before writing any code:**
```bash
# Verify go-github Actions API method:
grep -rn "ListRepositoryWorkflowRuns\|WorkflowRuns\|ListWorkflowRuns" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_runs.go 2>/dev/null | head -30

# Verify WorkflowRun struct fields:
grep -A 25 "type WorkflowRun struct" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_runs.go 2>/dev/null | head -30

# Verify ListWorkflowRunsOptions struct:
grep -A 15 "type ListWorkflowRunsOptions struct" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_runs.go 2>/dev/null

# Verify Repository.DefaultBranch field:
grep -n "DefaultBranch" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/github.go \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/repos.go 2>/dev/null | head -10

# Verify CreateTaskCommand type from agent/lib:
grep -rn "CreateTaskCommand\|TaskCommand" \
  $(go env GOPATH)/pkg/mod/github.com/bborbe/agent@*/lib/*.go 2>/dev/null | head -20

# Verify UUID v5 package:
grep -n "uuid" watcher/github-pr/go.mod
grep -rn "uuid.NewSHA1\|uuid.MustParse\|uuid.UUID" watcher/github-pr/pkg/taskid.go
```

Do NOT proceed to implementation until these greps confirm the exact method/field names. Use whatever the grep returns.
</context>

<requirements>
**Execute steps in order. Run `make precommit` only at the final step.**

1. **Run the symbol verification greps** from the context section. Record:
   - Exact method name for listing workflow runs (e.g. `ListRepositoryWorkflowRuns`)
   - Exact option struct name for branch/status/per_page filters
   - Exact `WorkflowRun` struct fields available (especially: `WorkflowID`, `HeadSHA`, `Conclusion`, `HTMLURL`, `CreatedAt`, `RunNumber`, `Name`)
   - Exact `Repository.DefaultBranch` field type (pointer or value)
   - Exact `CreateTaskCommand` struct from agent/lib
   - UUID package used in the PR watcher taskid.go

   If greps return no results, check the module cache at a different path:
   ```bash
   find $(go env GOPATH)/pkg/mod/github.com/google -name "actions_workflow_runs.go" 2>/dev/null | head -5
   ```

2. **Create `watcher/github-build/pkg/githubclient.go`**:

   Mirror `watcher/github-pr/pkg/githubclient.go` in structure. Key differences:
   - The interface is `GitHubClient` with two methods:
     - `GetWorkflowRuns(ctx context.Context, owner, repo, branch string) ([]WorkflowRun, error)`
     - `GetDefaultBranch(ctx context.Context, owner, repo string) (string, error)`
   - `WorkflowRun` is a local struct (not the go-github type), containing the fields needed for state derivation:
     - `WorkflowID int64` — for dedup (one entry per workflow, latest wins)
     - `Name string` — workflow name for the task body
     - `HeadSHA string` — used as episode_sha
     - `Conclusion string` — "failure", "success", etc.
     - `HTMLURL string` — run URL for task body
     - `CreatedAt time.Time` — for selecting "earliest red" run
   - `GetWorkflowRuns` fetches `GET /repos/{owner}/{repo}/actions/runs?branch={branch}&per_page=20&status=completed`; maps go-github response to `[]WorkflowRun`
   - `GetDefaultBranch` fetches `GET /repos/{owner}/{repo}` and returns the `DefaultBranch` field
   - Rate-limit detection: if the GitHub API response status is 403 or the error contains rate-limit language, return a typed error `ErrRateLimited` so the watcher can record `poll_errors_total{reason="rate_limited"}`

   Counter-feiter annotation on the interface:
   ```go
   //counterfeiter:generate -o ../mocks/github_client.go --fake-name GitHubClient . GitHubClient
   ```

   Note: `GetWorkflowRuns` must filter out runs whose `Conclusion` is empty/null (still in progress) — only return runs where `Conclusion != ""`.

3. **Create `watcher/github-build/pkg/githubclient_test.go`** with Ginkgo v2 + Gomega tests. Cover:
   - `GetWorkflowRuns` with a fake HTTP server (use `net/http/httptest`) serving a JSON fixture with 2 workflows, returning 2 `WorkflowRun` values with correct field mapping
   - `GetWorkflowRuns` filters out runs with empty conclusion (in-progress runs excluded)
   - `GetDefaultBranch` returns the `default_branch` field from the repository JSON

4. **Create `watcher/github-build/pkg/taskid.go`**:

   Read `watcher/github-pr/pkg/taskid.go` fully. Mirror the pattern with a NEW namespace UUID (different from the PR watcher's namespace to avoid collisions):

   ```go
   var buildWatcherNamespace = uuid.MustParse("8e3f5a2c-7b14-4d9e-a017-3c6e8b9f2a1d")

   // DeriveTaskID produces a deterministic UUID v5 task ID for a build episode.
   // The input key "<owner>/<repo>#build-<episodeSHA>" ensures:
   //   - Same red episode on the same repo always produces the same task ID (idempotent)
   //   - Different episode SHAs produce different task IDs (distinct episodes)
   func DeriveTaskID(owner, repo, episodeSHA string) libkafka.TaskID {
       key := owner + "/" + repo + "#build-" + episodeSHA
       id := uuid.NewSHA1(buildWatcherNamespace, []byte(key))
       return libkafka.TaskID(id.String())
   }
   ```

   Verify `libkafka.TaskID` type before using:
   ```bash
   grep -rn "type TaskID" $(go env GOPATH)/pkg/mod/github.com/bborbe/kafka@*/... 2>/dev/null | head -5
   ```
   Use whatever the grep returns. If `TaskID` is a string alias, `libkafka.TaskID(id.String())` is correct.

5. **Create `watcher/github-build/pkg/taskid_test.go`** with Ginkgo v2 + Gomega tests. Cover:
   - Same inputs always produce the same UUID (determinism / idempotency)
   - Different `episodeSHA` values produce different UUIDs (distinct episodes)
   - Different repos produce different UUIDs
   - The build watcher's UUID differs from a PR watcher UUID for the same repo (namespace isolation)

6. **Create `watcher/github-build/pkg/publisher.go`**:

   Read `watcher/github-pr/pkg/publisher.go` fully. Mirror the Kafka wiring. Differences:
   - Interface has only `PublishCreate` — no `PublishUpdateFrontmatter` (no force-push concept for builds)
   - Input params struct for `PublishCreate`:

   ```go
   type BuildTaskParams struct {
       Owner       string
       Repo        string
       EpisodeSHA  string
       FailingRuns []WorkflowRun // for listing names + URLs in task body
   }
   ```

   - `assignee` value MUST be `"build-fixer-agent"` (hard-coded string constant)
   - Task body is markdown-formatted:
     ```
     # Build Failure: <owner>/<repo>

     **Episode SHA:** <episodeSHA>

     **Failing Workflows:**
     - [<workflow-name>](<html-url>)
     ```

   Counter-feiter annotation on the interface:
   ```go
   //counterfeiter:generate -o ../mocks/command_publisher.go --fake-name CommandPublisher . CommandPublisher
   ```

7. **Create `watcher/github-build/pkg/publisher_test.go`** with Ginkgo v2 + Gomega tests. Cover:
   - `PublishCreate` calls the underlying `CommandObjectSender` with the correct task ID, assignee, and body
   - Body contains `owner/repo`, the episode SHA, and failing workflow names
   - `assignee` is exactly `"build-fixer-agent"`

8. **Generate counterfeiter mocks:**
   ```bash
   cd watcher/github-build && go generate ./pkg/...
   ```
   If `go generate` is not configured (no `//go:generate` directives in pkg files), run counterfeiter directly:
   ```bash
   cd watcher/github-build
   go run github.com/maxbrunsfeld/counterfeiter/v6 \
     -o pkg/mocks/github_client.go --fake-name GitHubClient \
     ./pkg GitHubClient
   go run github.com/maxbrunsfeld/counterfeiter/v6 \
     -o pkg/mocks/command_publisher.go --fake-name CommandPublisher \
     ./pkg CommandPublisher
   ```

9. **Create `watcher/github-build/pkg/suite_test.go`** (if not auto-created by ginkgo bootstrap):
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg_test

   import (
   	"testing"

   	. "github.com/onsi/ginkgo/v2"
   	. "github.com/onsi/gomega"
   )

   func TestPkg(t *testing.T) {
   	RegisterFailHandler(Fail)
   	RunSpecs(t, "Pkg Suite")
   }
   ```

10. **Run `make precommit`** in `watcher/github-build/`:
    ```bash
    cd watcher/github-build && make precommit
    ```
</requirements>

<constraints>
- Only edit files under `watcher/github-build/` and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- MUST grep-verify every go-github symbol before using it (no hallucinated field/method names)
- The build watcher namespace UUID (`8e3f5a2c-7b14-4d9e-a017-3c6e8b9f2a1d`) MUST differ from the PR watcher's namespace UUID — prevents cross-service task ID collisions
- `DeriveTaskID` key format: `"<owner>/<repo>#build-<episodeSHA>"` — exact format, no deviation
- `assignee` in `CreateTaskCommand` MUST be exactly the string `"build-fixer-agent"`
- `GetWorkflowRuns` MUST filter out runs with empty/null `Conclusion` (in-progress runs)
- Error wrapping uses `github.com/bborbe/errors`; never `fmt.Errorf`
- Counterfeiter annotations must follow the exact format from `watcher/github-pr/pkg/`
- `make precommit` runs from `watcher/github-build/`, never at repo root
- Existing tests must still pass
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm GitHubClient interface and WorkflowRun struct exist:
grep -n "GitHubClient\|WorkflowRun\|GetWorkflowRuns\|GetDefaultBranch" watcher/github-build/pkg/githubclient.go

# Confirm task ID uses build-watcher-specific namespace:
grep -n "buildWatcherNamespace\|8e3f5a2c" watcher/github-build/pkg/taskid.go

# Confirm DeriveTaskID key format:
grep -n "build-" watcher/github-build/pkg/taskid.go
# Expected: owner + "/" + repo + "#build-" + episodeSHA

# Confirm assignee constant:
grep -n "build-fixer-agent" watcher/github-build/pkg/publisher.go

# Confirm counterfeiter mocks were generated:
ls watcher/github-build/pkg/mocks/github_client.go watcher/github-build/pkg/mocks/command_publisher.go

# Confirm ErrRateLimited sentinel:
grep -n "ErrRateLimited\|RateLimited" watcher/github-build/pkg/githubclient.go
</verification>
