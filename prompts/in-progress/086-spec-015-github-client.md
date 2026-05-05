---
status: committing
spec: [015-github-build-watcher-mvp]
summary: Implemented GitHub Actions API client, task ID derivation, and Kafka publisher for watcher/github-build with full test coverage (86.4%), counterfeiter mocks at pkg/mocks/, and make precommit passing.
container: maintainer-086-spec-015-github-client
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-05T21:00:00Z"
queued: "2026-05-05T21:18:21Z"
started: "2026-05-05T21:22:35Z"
---

<summary>
- The build watcher can fetch failed workflow runs and the default branch for a repo via GitHub Actions
- In-progress runs (no conclusion yet) are filtered out so only completed runs reach state derivation
- Rate-limit and abuse-rate-limit errors surface as a typed sentinel so the watcher can record a discriminating metric
- Task IDs are deterministic across re-polls, distinct across episodes, and isolated from PR-watcher identifiers via a separate UUID namespace
- Kafka publishing mirrors the PR watcher's publisher interface — the watcher passes a complete CreateTaskCommand and the publisher only marshals + sends
- Counterfeiter mocks exist for the GitHub client and the publisher so downstream prompts can write watcher tests without wiring real services
- Coverage includes API field mapping, in-progress filtering, default-branch lookup, task-ID determinism + namespace isolation, and publisher marshalling
- `make precommit` passes in the new module
</summary>

<objective>
Implement the GitHub Actions API client, task ID derivation, and Kafka publisher for the build watcher. These are leaf dependencies consumed by the watcher core (prompt 3) and factory (prompt 4). No poll loop yet — just the primitives with tests.
</objective>

<context>
Read CLAUDE.md for project conventions.

Files to read fully before making any changes — these are canonical patterns:
- `watcher/github-pr/pkg/githubclient.go` — interface + go-github usage
- `watcher/github-pr/pkg/publisher.go` — `CommandPublisher` interface; `PublishCreate(ctx, cmd agentlib.CreateTaskCommand) error` is the exact signature to mirror; `cdb.NewCommandObjectSender` + `base.NewCommandCreator` wiring; `agentlib.CreateTaskCommandOperation`, `agentlib.TaskV1SchemaID`
- `watcher/github-pr/pkg/taskid.go` — `DeriveTaskID(...) uuid.UUID` returning `uuid.UUID`; the build watcher uses a different namespace UUID
- `watcher/github-pr/pkg/publisher_test.go` — mirror this test pattern (counterfeit `CommandObjectSender`, assert on parsed `Event`)
- `watcher/github-pr/pkg/mocks/` — list files; mocks are output to `pkg/mocks/` relative to the source file (`-o mocks/...` from inside `pkg/`)

**Symbol verification before writing any code (MUST run; if any returns nothing, halt and report — do NOT guess type names):**
```bash
# go-github Actions API:
grep -rn "ListRepositoryWorkflowRuns" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_runs.go 2>/dev/null | head -10

# go-github WorkflowRun struct:
grep -A 25 "type WorkflowRun struct" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_runs.go 2>/dev/null | head -30

# go-github typed rate-limit errors:
grep -n "type RateLimitError\|type AbuseRateLimitError" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/github.go 2>/dev/null

# Repository.DefaultBranch field:
grep -n "DefaultBranch" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/repos.go 2>/dev/null | head -5

# agent/lib CreateTaskCommand shape:
grep -B2 -A12 "type CreateTaskCommand struct" \
  $(go env GOPATH)/pkg/mod/github.com/bborbe/agent@*/lib/*.go 2>/dev/null | head -30
```
</context>

<requirements>
**Execute steps in order. Run `make precommit` only at the final step.**

1. **Run the symbol verification greps** in `<context>` above. If any returns no results, halt and report — do not guess. Record:
   - Exact method name for listing workflow runs
   - Exact `WorkflowRun` go-github struct fields (especially `WorkflowID`, `HeadSHA`, `Conclusion`, `HTMLURL`, `CreatedAt`, `Name`)
   - Exact typed errors `*github.RateLimitError`, `*github.AbuseRateLimitError`
   - Exact `agentlib.CreateTaskCommand` field set (must contain a TaskIdentifier, Frontmatter map, and Body string — confirm exact field names)

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
   - `GetWorkflowRuns` uses go-github's `Actions.ListRepositoryWorkflowRuns` with `&github.ListWorkflowRunsOptions{Branch: branch, Status: "completed", ListOptions: github.ListOptions{PerPage: 20}}`; maps each go-github `WorkflowRun` to the local `WorkflowRun`
   - `GetDefaultBranch` uses `Repositories.Get` and returns `repo.GetDefaultBranch()`
   - Rate-limit detection MUST use `errors.As` against the typed go-github errors:
     ```go
     var rl *github.RateLimitError
     var arl *github.AbuseRateLimitError
     if stdlibErrors.As(err, &rl) || stdlibErrors.As(err, &arl) {
         return nil, ErrRateLimited
     }
     ```
     (Use the standard library `errors.As` from the `errors` stdlib alias — NOT string matching.)
   - Define `ErrRateLimited = errors.New("github rate limited")` as a package-level sentinel; the watcher will compare via `stdlibErrors.Is(err, ErrRateLimited)`.

   Counterfeiter annotation on the interface (path is relative to this file, which lives in `pkg/`):
   ```go
   //counterfeiter:generate -o mocks/github_client.go --fake-name GitHubClient . GitHubClient
   ```

   `GetWorkflowRuns` MUST filter out runs where `run.GetConclusion() == ""` (still in progress) — only return completed runs.

3. **Create `watcher/github-build/pkg/githubclient_test.go`** with Ginkgo v2 + Gomega tests in package `pkg_test`. Cover:
   - `GetWorkflowRuns` with a fake HTTP server (`net/http/httptest`) serving a JSON fixture of 2 workflow runs returning 2 `WorkflowRun` values with correct field mapping
   - `GetWorkflowRuns` filters out runs with empty `Conclusion` (in-progress runs excluded)
   - `GetDefaultBranch` returns the `default_branch` field from the repository JSON

4. **Create `watcher/github-build/pkg/taskid.go`**:

   Read `watcher/github-pr/pkg/taskid.go` fully. Mirror the pattern with a NEW namespace UUID (different from PR watcher's namespace). `DeriveTaskID` MUST return `uuid.UUID` (matches PR watcher exactly). Callers will `.String()` it themselves.

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg

   import (
   	"github.com/google/uuid"
   )

   // buildWatcherNamespace is the fixed v5 UUID namespace for all build-watcher task identifiers.
   // Distinct from prWatcherNamespace to prevent cross-service ID collisions.
   var buildWatcherNamespace = uuid.MustParse("8e3f5a2c-7b14-4d9e-a017-3c6e8b9f2a1d")

   // DeriveTaskID returns a deterministic task identifier for a build-failure episode.
   // Input: "<owner>/<repo>#build-<episodeSHA>", e.g. "bborbe/maintainer#build-abc123".
   func DeriveTaskID(owner, repo, episodeSHA string) uuid.UUID {
   	key := owner + "/" + repo + "#build-" + episodeSHA
   	return uuid.NewSHA1(buildWatcherNamespace, []byte(key))
   }
   ```

5. **Create `watcher/github-build/pkg/taskid_test.go`** with Ginkgo v2 + Gomega tests in package `pkg_test`. Cover:
   - Same inputs always produce the same UUID (determinism)
   - Different `episodeSHA` values produce different UUIDs (distinct episodes)
   - Different repos produce different UUIDs
   - Build-watcher UUID differs from a UUID5 derived in the PR-watcher namespace for the same repo (namespace isolation)

6. **Create `watcher/github-build/pkg/publisher.go`**:

   Read `watcher/github-pr/pkg/publisher.go` fully. Mirror it nearly verbatim, with these differences:
   - The interface has only `PublishCreate` — no `PublishUpdateFrontmatter` (no force-push concept for builds)
   - Signature MUST be `PublishCreate(ctx context.Context, cmd agentlib.CreateTaskCommand) error` — IDENTICAL to PR watcher. Body construction and `Frontmatter["assignee"] = "build-fixer-agent"` happen at the call site (the watcher in prompt 3), NOT inside the publisher
   - `buildCommandObject` uses producer name `"maintainer-watcher-github-build"` (not `-pr`)
   - Counterfeiter annotation (relative to `pkg/`):
     ```go
     //counterfeiter:generate -o mocks/command_publisher.go --fake-name CommandPublisher . CommandPublisher
     ```

7. **Create `watcher/github-build/pkg/publisher_test.go`** with Ginkgo v2 + Gomega tests. Mirror `watcher/github-pr/pkg/publisher_test.go`. Cover:
   - `PublishCreate` invokes the underlying counterfeit `CommandObjectSender` exactly once
   - The captured `cdb.CommandObject` parses to an event whose JSON contains the input task identifier and `assignee: "build-fixer-agent"` (the test constructs a `CreateTaskCommand` with that frontmatter and asserts round-trip)
   - `SchemaID` on the command object equals `agentlib.TaskV1SchemaID`
   - The producer name in the embedded command equals `"maintainer-watcher-github-build"`

8. **Generate counterfeiter mocks** (mirror PR watcher's convention — likely `make generate`):
   ```bash
   cd watcher/github-build && make generate
   ```
   If the Makefile lacks a `generate` target (compare to `watcher/github-pr/Makefile`), invoke counterfeiter directly:
   ```bash
   cd watcher/github-build/pkg && go generate ./...
   ```
   Mocks land at `watcher/github-build/pkg/mocks/github_client.go` and `pkg/mocks/command_publisher.go`.

9. **Create `watcher/github-build/pkg/suite_test.go`** if not auto-created by ginkgo bootstrap:
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
- MUST grep-verify every go-github + agentlib symbol before using it; if any grep returns nothing, halt and report — do not invent types
- The build watcher namespace UUID `8e3f5a2c-7b14-4d9e-a017-3c6e8b9f2a1d` MUST differ from the PR watcher's namespace UUID
- `DeriveTaskID` returns `uuid.UUID` (NOT a `libkafka.TaskID` — that type does not exist; the PR watcher returns `uuid.UUID` and callers `.String()` it)
- `DeriveTaskID` key format: `"<owner>/<repo>#build-<episodeSHA>"` — exact format
- `PublishCreate` signature MUST be `(ctx, cmd agentlib.CreateTaskCommand) error` — mirror PR watcher; do NOT introduce a `BuildTaskParams` parameter type. Body + frontmatter assembly is the watcher's job (prompt 3)
- `assignee` field flows in the `Frontmatter` map of the `CreateTaskCommand` constructed by the watcher (prompt 3); the publisher MUST be domain-agnostic
- Counterfeiter `-o` path is `mocks/...` relative to `pkg/` (matches PR watcher) — NOT `../mocks/...`
- Rate-limit detection MUST use `errors.As` against `*github.RateLimitError` / `*github.AbuseRateLimitError` typed errors — NOT string matching
- `GetWorkflowRuns` MUST filter out runs with empty `Conclusion`
- Error wrapping uses `github.com/bborbe/errors`; never `fmt.Errorf`
- Producer name in the publisher's command object MUST be `"maintainer-watcher-github-build"`
- `make precommit` runs from `watcher/github-build/`, never at repo root
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm GitHubClient interface and WorkflowRun struct exist:
grep -n "GitHubClient\|WorkflowRun\|GetWorkflowRuns\|GetDefaultBranch" watcher/github-build/pkg/githubclient.go

# Confirm typed rate-limit detection (NOT string matching):
grep -n "RateLimitError\|AbuseRateLimitError\|errors.As" watcher/github-build/pkg/githubclient.go
# Expected: at least one match for each typed error

# Confirm DeriveTaskID returns uuid.UUID (NOT libkafka.TaskID):
grep -n "func DeriveTaskID" watcher/github-build/pkg/taskid.go
# Expected: ...) uuid.UUID

# Confirm build-watcher-specific namespace:
grep -n "8e3f5a2c\|buildWatcherNamespace" watcher/github-build/pkg/taskid.go

# Confirm DeriveTaskID key format:
grep -n "build-" watcher/github-build/pkg/taskid.go
# Expected: owner + "/" + repo + "#build-" + episodeSHA

# Confirm publisher mirrors PR signature (no BuildTaskParams):
grep -n "PublishCreate\|BuildTaskParams" watcher/github-build/pkg/publisher.go
# Expected: PublishCreate(ctx context.Context, cmd agentlib.CreateTaskCommand) error
# Expected: zero matches for BuildTaskParams

# Confirm producer name:
grep -n "maintainer-watcher-github-build" watcher/github-build/pkg/publisher.go

# Confirm counterfeiter mocks were generated to the correct path:
ls watcher/github-build/pkg/mocks/github_client.go watcher/github-build/pkg/mocks/command_publisher.go

# Confirm ErrRateLimited sentinel:
grep -n "ErrRateLimited" watcher/github-build/pkg/githubclient.go
</verification>
