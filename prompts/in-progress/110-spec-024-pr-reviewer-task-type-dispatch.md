---
status: committing
spec: [024-maintainer-repo-task-type-dispatch]
summary: Replaced CreateAgentForTaskType switch with CreateAgentProvider returning agentlib.AgentProvider; removed AgentRunner interface; updated RunConfig.Agent to *agentlib.Agent; rewrote dispatchAgent to use provider.Get; updated tests and CHANGELOG.
container: maintainer-110-spec-024-pr-reviewer-task-type-dispatch
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-14T17:45:00Z"
queued: "2026-05-14T15:32:21Z"
started: "2026-05-14T15:32:22Z"
branch: dark-factory/maintainer-repo-task-type-dispatch
---

<summary>

- Refactor existing switch-based dispatch in `agent/pr-reviewer` to the canonical `lib.AgentProvider` map-based pattern.
- Replace `factory.CreateAgentForTaskType(ctx, taskType, ...) (*agentlib.Agent, error)` with `factory.CreateAgentProvider(...) agentlib.AgentProvider` — pure plumbing, no switch, no error return. Body builds the PR-review domain agent and a healthcheck liveness agent (via `lib/healthcheck`), then returns `agentlib.NewAgentProvider(serviceName, map[agentlib.TaskType]*agentlib.Agent{...})`.
- Change `factory.CreateAgent`'s return type from the package-local `AgentRunner` interface to the concrete `*agentlib.Agent` (matches canonical `bborbe/agent/agent/claude/pkg/factory`). Remove the now-unused `AgentRunner` interface.
- Update `RunConfig.Agent` type from `AgentRunner` to `*agentlib.Agent` to match.
- Replace `main.go`'s `dispatchAgent` body — instead of calling `CreateAgentForTaskType`, construct the provider once and call `provider.Get(ctx, agentlib.TaskType(a.TaskType))`. Method signature unchanged.
- Replace the `Describe("CreateAgentForTaskType", ...)` block in `factory_test.go` with an equivalent `Describe("CreateAgentProvider", ...)` block — same three test cases (pr-review, healthcheck, unknown), unknown-task assertions adjusted to lib's error format.
- Update the existing `## Unreleased` CHANGELOG bullet to reference `CreateAgentProvider` instead of `CreateAgentForTaskType`.
- `cmd/run-task/main.go` is unchanged. `go.mod` is unchanged (`lib v0.62.16` already pinned).

</summary>

<objective>

Migrate `maintainer-agent-pr-reviewer`'s task-type dispatch from the bespoke `CreateAgentForTaskType` switch (returns `*agentlib.Agent, error`) to the canonical `lib.AgentProvider` registry (constructed via `CreateAgentProvider`, lookup via `provider.Get`). The unknown-task error now comes from `lib.AgentProvider.Get` rather than a hand-rolled `errors.Errorf`. End-user behaviour identical: `task_type: pr-review` → 3-phase domain agent; `task_type: healthcheck` → liveness agent; anything else → wrapped error with both accepted keys listed.

</objective>

<context>

Read `CLAUDE.md` at the repo root for project conventions.

Read these guides before writing code (in `~/.claude/plugins/marketplaces/coding/docs/`):
- `go-factory-pattern.md` — `Create*` prefix, zero-logic factories
- `go-error-wrapping-guide.md` — `bborbe/errors`, never `fmt.Errorf`
- `go-testing-guide.md` — Ginkgo/Gomega, external `_test` package

**Current state — read these files in full before editing:**

- `agent/pr-reviewer/pkg/factory/factory.go` — already contains `serviceName`, `AgentRunner` interface (lines 32-40), `CreateAgent` returning `AgentRunner` (lines 147-184), `CreateClaudeRunner`, and the **switch-based** `CreateAgentForTaskType` (lines 186-223) that this prompt replaces. `healthcheck` is already imported.
- `agent/pr-reviewer/pkg/factory/runner.go` — `RunConfig.Agent AgentRunner` (line 37) and the `cfg.Agent` fallback in `RunAgent` (lines 74-86) already exist. Only the field's type changes in this prompt.
- `agent/pr-reviewer/pkg/factory/factory_test.go` — already has `Describe("CreateAgentForTaskType", ...)` (lines 180-246) which this prompt replaces verbatim; `Describe("CreateAgent", ...)` and other blocks are untouched.
- `agent/pr-reviewer/main.go` — `Run()` has 5 `RecordRun` calls (allowlist-parse fail, deliverer-create fail, dispatch fail, agent-run fail, success-path `RecordRun(result.Status)`). The `dispatchAgent` helper (lines 156-180) currently calls `CreateAgentForTaskType`; this prompt rewrites its body but keeps the method signature `(ctx, repoAllowlist) (*agentlib.Agent, error)`. **`RecordRun` count remains 5 after this prompt — no new return paths are added.**
- `agent/pr-reviewer/cmd/run-task/main.go` — MUST remain unchanged. It builds a zero-value `RunConfig` (no `Agent` field set), so the `cfg.Agent == nil` fallback in `RunAgent` keeps it working.
- `agent/pr-reviewer/go.mod` — `github.com/bborbe/agent/lib v0.62.16` already pinned; do not modify.
- `CHANGELOG.md` (repo root) — already has an `## Unreleased` bullet that names `CreateAgentForTaskType`; update its function name and rationale rather than appending a new bullet.

**Canonical reference (read before writing the refactor):**

```bash
cat ~/Documents/workspaces/agent/agent/claude/pkg/factory/factory.go
```

In particular study how `CreateAgent` returns `*agentlib.Agent` directly (no interface), and how `CreateAgentProvider` returns `agentlib.AgentProvider` by mapping `agentlib.TaskType*` constants to `*agentlib.Agent` values. The maintainer pr-reviewer implementation mirrors this surface.

**Symbol verification (lib already at v0.62.16):**

```bash
grep -n "AgentProvider\|NewAgentProvider\|TaskTypePRReview\|TaskTypeHealthcheck" \
  $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@v0.62.16/*.go | head -20

grep -n "^func " \
  $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@v0.62.16/healthcheck/*.go
```

Expected: `AgentProvider` interface + `NewAgentProvider` constructor present; `TaskTypePRReview = "pr-review"`, `TaskTypeHealthcheck = "healthcheck"` defined; `healthcheck.NewAgent`, `healthcheck.NewClaudeStep` present.

If any symbol is missing at `v0.62.16`, STOP and report `{"status":"failed","blockers":["lib v0.62.16 symbols absent"]}`.

</context>

<requirements>

Execute steps in order. Run `make test` after step 4 for fast feedback. Run `make precommit` only at the end.

## 1. Refactor `agent/pr-reviewer/pkg/factory/factory.go`

Read the full file first.

**1a. Change `CreateAgent` return type from `AgentRunner` to `*agentlib.Agent`.**

Edit only the return-type token on the signature line and remove the now-unused `AgentRunner` interface block (lines ~32-40). The body of `CreateAgent` already returns `agentlib.NewAgent(...)` — no body change needed.

After edit:
```go
func CreateAgent(
    claudeConfigDir claudelib.ClaudeConfigDir,
    agentDir claudelib.AgentDir,
    model claudelib.ClaudeModel,
    ghToken string,
    env map[string]string,
    repoManager git.RepoManager,
    reviewMode string,
    repoAllowlist []string,
) *agentlib.Agent {
    // body unchanged
}
```

**1b. Replace `CreateAgentForTaskType` with `CreateAgentProvider`.**

Delete the entire `CreateAgentForTaskType` function (current lines ~186-223). Add `CreateAgentProvider` in its place:

```go
// CreateAgentProvider wires the per-task-type dispatch table for maintainer-agent-pr-reviewer.
// TaskTypePRReview routes to the 3-phase domain agent built by CreateAgent.
// TaskTypeHealthcheck routes to a liveness agent that reuses the Claude runner factory.
// Pure plumbing; no conditional, no error.
func CreateAgentProvider(
    claudeConfigDir claudelib.ClaudeConfigDir,
    agentDir claudelib.AgentDir,
    model claudelib.ClaudeModel,
    ghToken string,
    env map[string]string,
    repoManager git.RepoManager,
    reviewMode string,
    repoAllowlist []string,
) agentlib.AgentProvider {
    domainAgent := CreateAgent(claudeConfigDir, agentDir, model, ghToken, env, repoManager, reviewMode, repoAllowlist)
    healthcheckRunner := CreateClaudeRunner(claudeConfigDir, agentDir, model, env, claudelib.AllowedTools{})
    livenessAgent := healthcheck.NewAgent(healthcheck.NewClaudeStep(healthcheckRunner))
    return agentlib.NewAgentProvider(serviceName, map[agentlib.TaskType]*agentlib.Agent{
        agentlib.TaskTypePRReview:    domainAgent,
        agentlib.TaskTypeHealthcheck: livenessAgent,
    })
}
```

No type assertion needed because `CreateAgent` now returns `*agentlib.Agent` directly (per step 1a).

**1c. Remove unused imports if any.** `context` is no longer used inside `factory.go` *if* no other function in the file uses it — verify with `grep -n "context\." agent/pr-reviewer/pkg/factory/factory.go` before removing. (Spoiler: `CreateSyncProducer` and `CreateDeliverer` still use `context.Context`, so `context` import stays.)

If `healthcheck` was used only by the removed `CreateAgentForTaskType`, it now serves `CreateAgentProvider` — keep the import.

## 2. Update `agent/pr-reviewer/pkg/factory/runner.go`

Read the full file first.

Change the `RunConfig.Agent` field type from `AgentRunner` to `*agentlib.Agent`:

```go
// Agent overrides the agent used for execution. If nil, CreateAgent is called.
// Set by main.go after dispatching via CreateAgentProvider. cmd/run-task leaves
// this nil so CreateAgent is used for backward compatibility.
Agent *agentlib.Agent
```

Update the comment to say `CreateAgentProvider` (was `CreateAgentForTaskType`).

The `RunAgent` body needs no change — `cfg.Agent` is still nil-checkable, and `agent.Run(...)` works on the concrete type identically. The `CreateAgent` call inside the fallback now returns `*agentlib.Agent` too, so the local `agent` variable type infers correctly.

## 3. Update `agent/pr-reviewer/main.go`

Read the full file first.

Rewrite **only** the body of `dispatchAgent` (lines ~156-180). The method signature `func (a *application) dispatchAgent(ctx context.Context, repoAllowlist []string) (*agentlib.Agent, error)` stays. Replace the `factory.CreateAgentForTaskType(...)` call with provider construction + `Get`:

```go
func (a *application) dispatchAgent(
    ctx context.Context,
    repoAllowlist []string,
) (*agentlib.Agent, error) {
    env := map[string]string{}
    if a.GHToken != "" {
        env["GH_TOKEN"] = a.GHToken
    }
    repoManager := git.NewRepoManager(git.WorkdirConfig{
        ReposPath: a.ReposPath,
        WorkPath:  a.WorkPath,
    })
    provider := factory.CreateAgentProvider(
        a.ClaudeConfigDir,
        a.AgentDir,
        a.Model,
        a.GHToken,
        env,
        repoManager,
        a.ReviewMode,
        repoAllowlist,
    )
    agent, err := provider.Get(ctx, agentlib.TaskType(a.TaskType))
    if err != nil {
        return nil, errors.Wrap(ctx, err, "select agent for task_type")
    }
    return agent, nil
}
```

The caller (`Run()`) already wraps a returned error with `"task type dispatch"` and records a failed-run metric. Do not touch `Run()`. **`RecordRun` count stays at 5** — no new return paths.

All imports in `main.go` already cover the change (`agentlib`, `errors`, `factory`, `git`).

## 4. Run `make test`

```bash
cd agent/pr-reviewer && make test
```

Compile errors at this point usually mean a stale `AgentRunner` reference somewhere — `grep -rn "AgentRunner" agent/pr-reviewer` to locate.

## 5. Replace the `CreateAgentForTaskType` test block in `factory_test.go`

Read the full file first.

Delete the entire `Describe("CreateAgentForTaskType", func() { ... })` block (current lines ~180-246). Add `Describe("CreateAgentProvider", ...)` in its place — three test cases mirroring the originals but adapted to the provider API:

```go
Describe("CreateAgentProvider", func() {
    var (
        ctx         context.Context
        repoManager git.RepoManager
        provider    agentlib.AgentProvider
    )
    BeforeEach(func() {
        ctx = context.Background()
        provider = factory.CreateAgentProvider(
            "",
            "agent",
            "sonnet",
            "",
            map[string]string{},
            repoManager,
            "standard",
            nil,
        )
        Expect(provider).NotTo(BeNil())
    })

    It("returns a non-nil agent for pr-review task type", func() {
        agent, err := provider.Get(ctx, agentlib.TaskTypePRReview)
        Expect(err).NotTo(HaveOccurred())
        Expect(agent).NotTo(BeNil())
    })

    It("returns a non-nil agent for healthcheck task type", func() {
        agent, err := provider.Get(ctx, agentlib.TaskTypeHealthcheck)
        Expect(err).NotTo(HaveOccurred())
        Expect(agent).NotTo(BeNil())
    })

    It("returns an error naming the bogus value and both accepted task types", func() {
        agent, err := provider.Get(ctx, agentlib.TaskType("bogus"))
        Expect(err).To(HaveOccurred())
        Expect(agent).To(BeNil())
        Expect(err.Error()).To(ContainSubstring("unknown task_type"))
        Expect(err.Error()).To(ContainSubstring("bogus"))
        Expect(err.Error()).To(ContainSubstring("pr-review"))
        Expect(err.Error()).To(ContainSubstring("healthcheck"))
    })
})
```

The two earlier `Describe("CreateAgent", ...)` test cases assert `Expect(agent).NotTo(BeNil())`; since `CreateAgent` now returns `*agentlib.Agent`, those still pass without modification.

Run:
```bash
cd agent/pr-reviewer && make test
```

All tests must pass.

## 6. Update the CHANGELOG bullet

`CHANGELOG.md` (repo root) already has this `## Unreleased` line:

```
- feat(agent/pr-reviewer): per-task-type dispatch via factory.CreateAgentForTaskType — healthcheck task type now routes to a dedicated liveness agent; unknown task_type values fail fast with an explicit error listing accepted types; bumps agent/lib v0.62.5 → v0.62.16
```

Replace it with:

```
- feat(agent/pr-reviewer): per-task-type dispatch via factory.CreateAgentProvider — healthcheck task type now routes to a dedicated liveness agent built from lib/healthcheck; unknown task_type values fail fast via lib.AgentProvider.Get; bumps agent/lib v0.62.5 → v0.62.16
```

Verify:
```bash
grep -n "CreateAgentProvider\|CreateAgentForTaskType" CHANGELOG.md
```
Expected: exactly one match for `CreateAgentProvider` under `## Unreleased`; zero matches for `CreateAgentForTaskType`.

## 7. Run `make precommit`

```bash
cd agent/pr-reviewer && make precommit
```

Must exit 0. Fix any lint, gosec, or errcheck findings.

</requirements>

<constraints>

- `github.com/bborbe/agent/lib` stays pinned at `v0.62.16`. Do NOT run `go get` or `go mod tidy`. `go.mod` and `go.sum` must not be modified.
- `CreateAgent`'s signature changes only its return type (`AgentRunner` → `*agentlib.Agent`). The body and parameter list are unchanged.
- The `AgentRunner` interface is removed from `factory.go`. Verify no other file references it: `grep -rn "AgentRunner" agent/pr-reviewer` → 0 matches after edits.
- `CreateAgentForTaskType` is removed entirely from `factory.go`. `grep -n "CreateAgentForTaskType" agent/pr-reviewer` → 0 matches after edits.
- `cmd/run-task/main.go` is unchanged and must still build with the new `RunConfig.Agent *agentlib.Agent` (zero-value `nil` still triggers the `CreateAgent` fallback).
- `CreateAgentProvider` is pure plumbing: no `ctx context.Context` parameter, no error return, no `switch`, no `if`/`for`. The dispatch map is the only branch point; `lib.AgentProvider.Get` handles lookup.
- Unknown-task-type errors come from `lib.AgentProvider.Get` — format is `"unknown task_type %q for %s; accepted: %v"`. Do NOT hand-roll a separate error. Tests assert against lib-supplied substrings (`unknown task_type`, `bogus`, both registered keys).
- All errors via `github.com/bborbe/errors` — never `fmt.Errorf`. The `provider.Get` error is wrapped via `errors.Wrap(ctx, err, "select agent for task_type")` inside `dispatchAgent`.
- The Kafka entry point's metrics wiring already fires on every return path (5 total: allowlist-parse error, deliverer-create error, dispatch error, agent-run error, success). Number stays 5. Verify: `grep -c "RecordRun" agent/pr-reviewer/main.go` → 5.
- The deferred `pusher.PushContext` continues to fire on all paths — no change to the defer.
- Do NOT touch `cmd/run-task/main.go`, `cmd/cli/main.go`, any `watcher/` directory, any `k8s/` YAML, or any `Dockerfile`.
- Only these files may be modified: `agent/pr-reviewer/pkg/factory/factory.go`, `agent/pr-reviewer/pkg/factory/factory_test.go`, `agent/pr-reviewer/pkg/factory/runner.go`, `agent/pr-reviewer/main.go`, `CHANGELOG.md`.
- Do NOT commit — dark-factory handles git.
- `make precommit` must exit 0 in `agent/pr-reviewer/`.

</constraints>

<verification>

Confirm lib version is unchanged:
```bash
grep "github.com/bborbe/agent/lib v0.62.16" agent/pr-reviewer/go.mod
```
Expected: one match.

Confirm `CreateAgentForTaskType` is gone:
```bash
grep -rn "CreateAgentForTaskType" agent/pr-reviewer
```
Expected: zero matches.

Confirm `AgentRunner` is gone:
```bash
grep -rn "AgentRunner" agent/pr-reviewer
```
Expected: zero matches.

Confirm `CreateAgentProvider` exists with the right return type:
```bash
grep -n "func CreateAgentProvider" agent/pr-reviewer/pkg/factory/factory.go
```
Expected: one match returning `agentlib.AgentProvider`.

Confirm `CreateAgent` now returns `*agentlib.Agent`:
```bash
grep -A1 "^func CreateAgent\b" agent/pr-reviewer/pkg/factory/factory.go | tail -20
```
Expected: signature ends with `) *agentlib.Agent {`.

Confirm `RunConfig.Agent` is `*agentlib.Agent`:
```bash
grep -n "Agent \*agentlib.Agent" agent/pr-reviewer/pkg/factory/runner.go
```
Expected: one match.

Confirm `main.go` uses the provider and still has 5 `RecordRun` calls:
```bash
grep -n "CreateAgentProvider\|provider.Get" agent/pr-reviewer/main.go
grep -c "RecordRun" agent/pr-reviewer/main.go
```
Expected: provider construction + `provider.Get` call inside `dispatchAgent`; `RecordRun` count = 5.

Confirm `cmd/run-task/main.go` is unchanged:
```bash
git diff agent/pr-reviewer/cmd/run-task/main.go
```
Expected: empty.

Confirm no go.mod/go.sum changes:
```bash
git diff agent/pr-reviewer/go.mod agent/pr-reviewer/go.sum
```
Expected: empty.

Confirm CHANGELOG entry:
```bash
grep -n "CreateAgentProvider" CHANGELOG.md
grep -n "CreateAgentForTaskType" CHANGELOG.md
```
Expected: one match for `CreateAgentProvider` under `## Unreleased`; zero matches for `CreateAgentForTaskType`.

Run precommit:
```bash
cd agent/pr-reviewer && make precommit
```
Expected: exit 0.

</verification>
