---
status: verifying
approved: "2026-05-28T21:49:43Z"
verifying: "2026-05-29T14:40:08Z"
branch: dark-factory/github-releaser-execution-phase-direct-push
---

## Summary

- Wires the execution phase of `agent/github-releaser` — direct-push path only. After planning produces a `## Plan` JSON with `outcome: ready`, execution clones the target repo, rewrites `## Unreleased → ## vX.Y.Z` in `CHANGELOG.md`, commits + annotated tag, and direct-pushes commit + tag to the default branch.
- New package `pkg/git/` (interface + impl + counterfeiter mock) wrapping `os/exec` git commands. `pkg/changelog/` extended with `RewriteUnreleasedHeader([]byte, string) ([]byte, error)`. `pkg/steps_execution.go` orchestrates everything as `agentlib.Step`.
- Factory wires planning + execution phases together so the agent advances `planning → execution → done` on the happy path. `ai_review` phase deferred to a separate spec.
- **Out of scope explicit:** PR + auto-merge fallback for branch-protected repos (separate spec — `protected_branch_rejected` error category surfaces here for the fallback spec to consume); `ai_review` phase; CRD update / dev deploy; `--dry-run` flag.
- Mirrors Phase 1 slash-command behavior verbatim per [[GitHub Release Agent Phase 1 Learnings]] § "What carries to Phase 2 verbatim". The direct-push path was validated live in Phase 1 (`docker-utils v1.7.8`).

## Problem

After spec 047, the agent's planning phase produces a structured `## Plan` JSON but no downstream step consumes it. Watcher-emitted tasks land in the vault, get planned, and sit. Without execution, the daily-value loop ("merged feature → released version with tag") is broken at the second link.

The execution phase is also the first time the agent writes to a target repo. Every prior phase was read-only. The git-write surface introduces new failure modes (auth, branch protection, tag collision, race-with-concurrent-commit) that must be classified explicitly so downstream code (PR-fallback spec, retry policy) can branch on them.

Direct-push is the simpler path and ships first. Branch protection — observed live on `disk-status` during Phase 1 — needs a separate PR + auto-merge fallback that consumes the `protected_branch_rejected` error category emitted here.

## Goal

End state: given a task with `phase: execution` + `## Plan` JSON `outcome: ready`, the agent clones the target repo, rewrites the CHANGELOG header in-place, commits + tags, pushes both, writes a `## Result` JSON with `outcome: released, path: direct-push, commit_sha, tag`, and returns `agentlib.Result{Status: Done, NextPhase: "ai_review"}`. The framework advances `phase: ai_review` for the (not-yet-implemented) review step.

On any failure: `## Result` JSON has `outcome: failed`, an `error_category` field from a closed enum (auth, repo_not_found, changelog_missing, unreleased_not_found, tag_collision, protected_branch_rejected, push_non_fast_forward, unknown), and `agentlib.Result{Status: Failed}` triggers controller retry per the existing cap.

`cd agent/github-releaser && make precommit` exits 0 throughout.

## Non-goals

- **PR + auto-merge fallback** — when push fails with `protected_branch_rejected`, this spec stops and surfaces the error category; a separate spec consumes it.
- **ai_review phase** — remote verification of the tag + CHANGELOG state after push. Separate spec.
- **CRD update** (`trigger.phases: [planning, execution]` instead of just `[planning]`) — defer to dev-deploy spec; agent works locally without it.
- **`--dry-run` flag** — deferred.
- **Mono-repo support** (multiple CHANGELOGs in one repo).
- **GitHub Releases page creation** — tag-only.
- **Backfilling already-released sections.**
- **Per-step model selection** — execution uses whatever `ANTHROPIC_MODEL` resolves to (irrelevant — execution doesn't call Claude).
- **Cosmetic error-message double-prefix cleanup** — separate small fix.

## Desired Behavior

1. `pkg/git/` exports a `GitOps` interface with methods `Clone(ctx, cloneURL, ref, workdir) error`, `Commit(ctx, workdir, message, paths...) (sha string, err error)`, `Tag(ctx, workdir, tag, message) error`, `Push(ctx, workdir, refs...) error`. Concrete implementation `osExecGitOps` shells out to `git` via `os/exec`; counterfeiter mock in `pkg/git/mocks/git_ops.go`.
2. `pkg/changelog/` exports a new pure function `RewriteUnreleasedHeader(content []byte, newHeader string) ([]byte, error)` that locates the line `## Unreleased` (whitespace-tolerant, same as `InferHeaderPrefixStyle`) and replaces it with `newHeader` (e.g. `"## v1.2.7"`). Leaves all bullets + other lines untouched. Returns wrapped error if `## Unreleased` not found.
3. `pkg/steps_execution.go` exports an `ExecutionStep` struct implementing `agentlib.Step`. Its `Run` method: (a) reads `## Plan` via `ExtractSection[PlanOutput]`; (b) validates `Outcome == "ready"` + `NextVersion != ""` + `NextVersionHeader != ""`; (c) builds an ephemeral workdir; (d) calls `GitOps.Clone`; (e) reads CHANGELOG.md from clone, applies `RewriteUnreleasedHeader`, writes back; (f) calls `GitOps.Commit` with message `"release " + NextVersionHeader[3:]` (strip `## ` prefix → `vX.Y.Z`); (g) calls `GitOps.Tag` annotated with same name + message; (h) calls `GitOps.Push` of commit + tag.
4. On success: `pkg/steps_execution.go` writes a `## Result` JSON via `MarshalSectionTyped` with `{outcome: "released", path: "direct-push", commit_sha: <SHA from Commit>, tag: <vX.Y.Z>, error_category: "", error: ""}`; returns `agentlib.Result{Status: Done, NextPhase: string(domain.TaskPhaseAIReview)}`.
5. On any failure: `pkg/steps_execution.go` writes a `## Result` JSON with `{outcome: "failed", path: "direct-push", commit_sha: "", tag: "", error_category: <closed-enum>, error: <wrapped error string>}`; returns `agentlib.Result{Status: Failed, Message: <error string>}`. Error category enum: `auth | repo_not_found | changelog_missing | unreleased_not_found | tag_collision | protected_branch_rejected | push_non_fast_forward | unknown`. Classifier inspects the underlying error message (substring match like pr-reviewer's `IsGitAuthFailure`); unknown messages map to `"unknown"`.
6. `pkg/factory/factory.go` `CreateAgent` is extended to wire BOTH planning + execution phases: `agentlib.NewAgent(agentlib.NewPhase(domain.TaskPhasePlanning, planningStep), agentlib.NewPhase(domain.TaskPhaseExecution, executionStep))`. Typed phase constants only — no string literals. Factory takes a new `GitOps` dependency (constructed once via `CreateGitOps()`).
7. Ephemeral workdir is created under `os.TempDir()` (e.g. `/tmp/github-releaser-<task_identifier>/`) and removed via `defer` on every exit path (success, failure, panic). Cleanup is observable via test against `os.Stat` of the workdir post-Run.
8. Workdir is per-task-identifier so concurrent agents on different tasks don't collide. Re-running the same task (same task_identifier) reuses the path; existing dir is removed before clone (idempotent under replay — Constraint, not Behavior).

## Constraints

- Package path: `github.com/bborbe/maintainer/agent/github-releaser/pkg/git` (new package). Layout: `git.go` (interface), `os_exec_git_ops.go` (impl), `error_classifier.go` (classify err → enum), `git_suite_test.go`, plus mocks at `pkg/git/mocks/`.
- `pkg/git/` shells out to `git` binary via `os/exec`. Container image must include `git` (already true — pr-reviewer's container ships it). No CGo, no go-git library — keeps it simple.
- `RewriteUnreleasedHeader` lives in `pkg/changelog/changelog.go` next to its siblings (single-file pkg). New tests in `pkg/changelog/changelog_test.go` via `DescribeTable`.
- `pkg/steps_execution.go` flat at `pkg/` root, NOT `pkg/steps/`. Mirrors `pkg/steps_planning.go`.
- Phase constants typed only: `domain.TaskPhaseExecution`, `domain.TaskPhaseAIReview`. String literal `"execution"` banned in production code (factory + steps).
- Error wrapping via `github.com/bborbe/errors` everywhere. NO `fmt.Errorf`.
- Auth: HTTPS clone with `GH_TOKEN` env via `https://x-access-token:${GH_TOKEN}@github.com/<owner>/<repo>.git` URL transformation (same pattern pr-reviewer uses for App-token auth). App-token vs PAT resolution lives in `main.go` / factory, NOT in `pkg/git/`.
- Bot identity for commit: `bborbe@users.noreply.github.com` + `Benjamin Borbe`. Same identity used by Phase 1 slash command — verbatim from [[GitHub Release Agent Phase 1 Learnings]].
- Tag is **annotated** (`git tag -a vX.Y.Z -m "release vX.Y.Z"`), not lightweight. Annotated tags carry author + date; lightweight tags don't and look unprofessional.
- Workdir lifetime: created at start of `ExecutionStep.Run`, removed on defer. Concurrent runs on different `task_identifier` use distinct paths; concurrent runs on the SAME `task_identifier` are prevented by the controller's per-task lock (out of scope here).
- Coverage targets: `pkg/git/` ≥ 75% (shelling out is hard to test deeply offline; the error classifier + URL transformation are the unit-testable parts), `pkg/changelog/` for the new function ≥ 90%, `pkg/steps_execution.go` ≥ 75% via mocked GitOps.
- Counterfeiter mock at `pkg/git/mocks/git_ops.go` regenerated via `//counterfeiter:generate -o mocks/git_ops.go --fake-name GitOps . GitOps`.
- All file edits inside the clone use the agent's process; no shell injection (use `exec.CommandContext` with explicit arg slice, never `sh -c`).

## Failure Modes

| Trigger | Detection | Expected behavior | Reversibility | Recovery |
|---|---|---|---|---|
| `## Plan` missing or `outcome != "ready"` | `ExtractSection[PlanOutput]` returns nil/error, or struct fields not `ready` | Write `## Result` with `error_category: unknown` + reason "execution invoked but planning did not complete"; return `Status: Failed` | Reversible | Controller retry; if cap exhausted, operator inbox + manual investigation |
| GitHub clone fails — auth (401/403, "Authentication failed", "could not read Username") | `error_classifier` substring match on git stderr | `error_category: auth`; `Status: Failed` | Reversible | Operator checks `GH_TOKEN` / App auth in Config CRD secret; retry |
| GitHub clone fails — repo not found (404, "Repository not found") | `error_classifier` substring match | `error_category: repo_not_found`; `Status: Failed`; clear assignee + previous_assignee (operator should investigate watcher emission) | Reversible | Operator deletes vault task or fixes watcher's repo allowlist |
| Clone succeeds; `CHANGELOG.md` absent in clone | `os.ReadFile` returns ENOENT | `error_category: changelog_missing`; `Status: Failed`; clear assignee + previous_assignee (race with concurrent merge that removed CHANGELOG?) | Reversible | Operator investigates target-repo state |
| `RewriteUnreleasedHeader` returns "Unreleased not found" | Function error | `error_category: unreleased_not_found`; `Status: Failed`; clear assignee + previous_assignee (race with another release between planning and execution) | Reversible | Operator restarts pipeline (watcher will re-emit a fresh task) |
| `GitOps.Tag` fails with "already exists" / "fatal: tag 'vX.Y.Z' already exists" | `error_classifier` substring match | `error_category: tag_collision`; `Status: Failed`; clear assignee + previous_assignee (concurrent release pipeline ran?) | Irreversible (tag exists on remote) | Operator investigates; either accept the existing tag (mark task aborted) or delete tag + retry |
| `GitOps.Push` fails with protected-branch reject ("protected branch", "required status checks", "Required reviews") | `error_classifier` substring match | `error_category: protected_branch_rejected`; `Status: Failed` (NOT retried by controller — PR fallback spec will consume this category) | Reversible | PR + auto-merge fallback spec (separate) picks up |
| `GitOps.Push` fails with non-fast-forward ("non-fast-forward", "Updates were rejected because the remote contains work") | `error_classifier` substring match | `error_category: push_non_fast_forward`; `Status: Failed` (retry — planning will re-fetch with newer `ref`) | Reversible | Controller retry; new task gets new `ref`; cycle repeats |
| Workdir cleanup fails (permission denied, file in use) | `os.RemoveAll` returns error | Log warning via `glog`; do not block return; `## Result` still written; ops can manually clean `/tmp/github-releaser-*` | Reversible | Manual cleanup or restart pod |

## Do-Nothing Option

Cost of NOT building the execution phase:

- `## Plan` JSON sections accumulate in vault tasks with no downstream consumer. Watcher → planning is a one-way pipe.
- BRO-20203 fleet release stays manual via `/github-release-repo` slash command (Phase 1 prototype). ~5 min/repo × 30 = 2.5h of toil per pass.
- The Phase 1-validated direct-push behavior never graduates to Go. The slash command works but bypasses metrics, observability, k8s scheduling.
- The PR-fallback spec (next after this) can't be written without the direct-push error categories defined here.

## Security / Abuse

- The agent receives `clone_url` from watcher-emitted task frontmatter (trusted producer, allowlist filtered).
- HTTPS auth uses `GH_TOKEN` (or App installation token, future). Token is stored in k8s Secret, mounted as env. Never logged. `display:"length"` tag on the application struct field per [[Go Agent Implementation Guide]].
- Commit identity is a fixed bot identity (`bborbe@users.noreply.github.com`). No user-controlled email injection.
- Git command args are passed as explicit arg slices to `exec.CommandContext` — no shell interpolation, no `sh -c`. CHANGELOG bytes are passed via file write (not shell arg).
- Workdir lives under `/tmp/` and is removed on defer. Sensitive content (cloned repo source) is removed even on crash thanks to the Linux tmpfs cleanup at pod terminate.
- No write access to repos beyond the cloned ephemeral copy (which gets removed). Push targets are constrained to the cloned `clone_url`'s remote — git's own enforcement.

## Acceptance Criteria

- [ ] `cd agent/github-releaser && make precommit` exits 0.
- [ ] `ls agent/github-releaser/pkg/git/` returns: `git.go`, `os_exec_git_ops.go`, `error_classifier.go`, `git_suite_test.go`, plus a `mocks/` subdir with at least one file — evidence: `ls pkg/git/ | wc -l` ≥ 4 + `ls pkg/git/mocks/ | wc -l` ≥ 1.
- [ ] `grep -c '^type GitOps interface' agent/github-releaser/pkg/git/git.go` returns 1 — frozen interface.
- [ ] GitOps interface has the 4 methods Clone/Commit/Tag/Push — evidence: `grep -cE 'Clone\(.*\).*error|Commit\(.*\).*\(string, error\)|Tag\(.*\).*error|Push\(.*\).*error' pkg/git/git.go` returns 4.
- [ ] `grep -c '^func RewriteUnreleasedHeader(' agent/github-releaser/pkg/changelog/changelog.go` returns 1.
- [ ] `grep -c '^func NewExecutionStep(' agent/github-releaser/pkg/steps_execution.go` returns 1.
- [ ] `grep -c 'agentlib.NewPhase(domain.TaskPhaseExecution' agent/github-releaser/pkg/factory/factory.go` returns 1.
- [ ] `grep -c '"execution"' agent/github-releaser/pkg/factory/factory.go agent/github-releaser/pkg/steps_execution.go` returns 0 — typed constants only.
- [ ] `grep -c 'fmt.Errorf' agent/github-releaser/pkg/git/ agent/github-releaser/pkg/steps_execution.go` returns 0 — bborbe/errors only.
- [ ] Coverage `pkg/git/`: `go test -cover ./pkg/git/...` reports `coverage: (7[5-9]|[89][0-9]|100)\.[0-9]%`.
- [ ] Coverage `pkg/changelog/`: `go test -cover ./pkg/changelog/...` reports `coverage: (9[0-9]|100)\.[0-9]%` — bound to ≥ 90% per Constraints (pure-Go pkg).
- [ ] Coverage `pkg/steps_execution/`: `go test -cover ./pkg/steps_execution/...` reports `coverage: (7[5-9]|[89][0-9]|100)\.[0-9]%`.
- [ ] **Mocked happy-path integration test:** Ginkgo case in `pkg/steps_execution_test.go` builds an `ExecutionStep` with a counterfeiter-mocked `GitOps` (all 4 methods return success; `Commit` returns `"abc1234"`), feeds a task content containing `## Plan` JSON `{outcome: "ready", next_version: "1.2.8", next_version_header: "## v1.2.8", ...}`, runs `step.Run(ctx, md)`, asserts: (a) `Result.Status == Done`; (b) `Result.NextPhase == string(domain.TaskPhaseAIReview)`; (c) the `## Result` section has `outcome: "released"`, `commit_sha: "abc1234"`, `tag: "v1.2.8"`; (d) `fakeGitOps.CloneCallCount() == 1 && CommitCallCount() == 1 && TagCallCount() == 1 && PushCallCount() == 1`; (e) the bytes passed to `Commit` contain the literal substring `## v1.2.8` AND do NOT contain `## Unreleased` (proves rewrite ran before commit, not after). Evidence: `grep -c 'CommitCallCount' pkg/steps_execution_test.go` returns ≥ 1; `grep -c '"v1.2.8"' pkg/steps_execution_test.go` returns ≥ 1.
- [ ] **Mocked failure-path integration test:** Same setup but `GitOps.Push` returns `errors.Errorf(ctx, "remote: error: GH006: Protected branch update failed for refs/heads/master")`. Test asserts: (a) `Result.Status == Failed`; (b) `## Result` has `outcome: "failed"`, `error_category: "protected_branch_rejected"`; (c) `fakeGitOps.TagCallCount() == 1 && PushCallCount() == 1` (proves failure surfaces post-tag, not pre-commit). Evidence: `grep -c 'protected_branch_rejected' pkg/steps_execution_test.go` returns ≥ 1; `grep -c 'TagCallCount' pkg/steps_execution_test.go` returns ≥ 1.
- [ ] **Error-classifier unit test** exercises all 8 categories via `DescribeTable` against realistic git stderr samples. Each entry maps a distinct stderr fragment to a distinct enum value — verified by the literal-category grep below: `grep -cE 'auth|repo_not_found|changelog_missing|unreleased_not_found|tag_collision|protected_branch_rejected|push_non_fast_forward|unknown' pkg/git/error_classifier_test.go` returns ≥ 8 (one literal occurrence per category).
- [ ] **`RewriteUnreleasedHeader` DescribeTable** in `pkg/changelog/changelog_test.go` with ≥ 3 entries: (a) happy-path replaces `## Unreleased` with `## v1.2.8`, output contains `## v1.2.8` AND no `## Unreleased`; (b) tolerates trailing whitespace on the heading line; (c) returns wrapped error when `## Unreleased` not present. Evidence: `grep -c '"rewrite unreleased' pkg/changelog/changelog_test.go` returns ≥ 3 (one per entry name starting with the literal string).
- [ ] **Workdir cleanup observability** — when `os.RemoveAll` returns a non-nil error, a `glog.Warningf` line matching `workdir cleanup failed` is emitted. Evidence: `grep -c 'workdir cleanup failed' pkg/steps_execution.go` returns ≥ 1; a unit test triggers the failure path and captures the log line.
- [ ] Root `CHANGELOG.md` `## Unreleased` section gains a `feat:` bullet referencing execution-phase direct-push — evidence: `awk '/^## Unreleased$/,/^## v/' CHANGELOG.md | grep -c 'execution phase'` returns ≥ 1 (scoped to the Unreleased block; rejects misplaced bullets).

## Verification

```bash
cd agent/github-releaser
make precommit                                                            # exit 0

# Per-package coverage floors
go test -cover ./pkg/git/...              # ≥ 75%
go test -cover ./pkg/changelog/...        # ≥ 90%
go test -cover ./pkg/steps_execution/...  # ≥ 75%

# Signatures + factory wiring
grep -c '^type GitOps interface'                            pkg/git/git.go                   # =1
grep -c '^func RewriteUnreleasedHeader('                    pkg/changelog/changelog.go       # =1
grep -c '^func NewExecutionStep('                           pkg/steps_execution.go           # =1
grep -c 'agentlib.NewPhase(domain.TaskPhaseExecution'       pkg/factory/factory.go           # =1

# Cleanliness gates
grep -c '"execution"' pkg/factory/factory.go pkg/steps_execution.go           # =0
grep -c 'fmt.Errorf' pkg/git/ pkg/steps_execution.go                          # =0

# Mock-driven test markers — call-count + arg-content assertions
grep -c 'CommitCallCount'                       pkg/steps_execution_test.go   # ≥1
grep -c 'TagCallCount'                          pkg/steps_execution_test.go   # ≥1
grep -c '"v1.2.8"'                              pkg/steps_execution_test.go   # ≥1
grep -c 'protected_branch_rejected'             pkg/steps_execution_test.go   # ≥1

# Error-classifier — distinct categories
grep -cE 'auth|repo_not_found|changelog_missing|unreleased_not_found|tag_collision|protected_branch_rejected|push_non_fast_forward|unknown' pkg/git/error_classifier_test.go  # ≥8

# RewriteUnreleasedHeader content + boundary tests
grep -c '"rewrite unreleased'                   pkg/changelog/changelog_test.go  # ≥3

# Workdir cleanup observability
grep -c 'workdir cleanup failed'                pkg/steps_execution.go        # ≥1

# Root CHANGELOG (scoped to Unreleased section only)
awk '/^## Unreleased$/,/^## v/' CHANGELOG.md | grep -c 'execution phase'      # ≥1
```

No scenario justified — integration tests with mocked `GitOps` reach every behavior; the only thing they can't cover (real `git push` against a real remote) is the same offline-vs-online tension as spec 047's planning step and is deferred to dev-cluster smoke per the same rationale ([[spec-writing]] § Test-layer responsibilities).
