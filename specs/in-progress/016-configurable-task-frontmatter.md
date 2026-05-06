---
status: verifying
approved: "2026-05-06T17:43:51Z"
generating: "2026-05-06T17:43:52Z"
prompted: "2026-05-06T17:48:00Z"
verifying: "2026-05-06T17:56:18Z"
branch: dark-factory/configurable-task-frontmatter
---

## Summary

- The build watcher currently hard-codes the published task's `assignee`, `status`, and (omitted) `phase` in source code
- Operators cannot adjust these without a code change + rebuild + redeploy
- This spec adds three CLI args / env vars to `watcher/github-build/main.go` so operators can override the defaults at deploy time
- The hard-coded defaults move into the args' `default` tag values, preserving today's behavior when the operator sets nothing
- The args flow through the factory and watcher into `buildCreateTaskCommand` as parameters; no other behavior changes

## Problem

`buildCreateTaskCommand` in `watcher/github-build/pkg/watcher.go` writes:

```go
Frontmatter: agentlib.TaskFrontmatter{
    "assignee": "build-fixer-agent",
    "status":   "todo",
    ...
}
```

Both string literals. If a future fixer agent is renamed, or the operator wants tasks to start in `backlog` or carry an explicit `phase: planning`, today the only path is patch-and-redeploy. That's needless friction for what is purely operational config.

## Goal

`main.go` exposes three new CLI args / env vars on the `application` struct:

| Arg | Env | Default | Purpose |
|---|---|---|---|
| `--build-assignee` | `WATCHER_GITHUB_BUILD_TASK_ASSIGNEE` | `build-fixer-agent` | Value written to `Frontmatter["assignee"]` |
| `--build-task-status` | `WATCHER_GITHUB_BUILD_TASK_STATUS` | `todo` | Value written to `Frontmatter["status"]` |
| `--build-task-phase` | `WATCHER_GITHUB_BUILD_TASK_PHASE` | (empty string — omit field) | Value written to `Frontmatter["phase"]`; if empty, the field is NOT added to frontmatter (preserves today's behavior of omitting `phase`) |

The values flow through `factory.CreateWatcher` into `pkg.NewWatcher`, are stored on `buildWatcher`, and `buildCreateTaskCommand` reads them via parameters (not package-level state).

## Non-goals

- Per-repo overrides — that's a follow-up spec (`per-repo-maintenance-yaml`)
- Validation that `assignee` matches a known consumer in the controller — out of scope (typo blast radius is "task sits in todo with no consumer", not catastrophic)
- Validation of `status` against a known set — `agentlib.TaskFrontmatter` is a `map[string]string`; the controller and downstream consumers ultimately decide what's valid
- Configurability of the body markdown template (title, "Failing Workflows" section, etc.)
- Adding `phase` to the PR watcher

## Desired Behavior

1. Setting `WATCHER_GITHUB_BUILD_TASK_ASSIGNEE=other-agent` in the StatefulSet env results in published `Frontmatter["assignee"] = "other-agent"`
2. Setting `WATCHER_GITHUB_BUILD_TASK_STATUS=backlog` results in `Frontmatter["status"] = "backlog"`
3. Setting `WATCHER_GITHUB_BUILD_TASK_PHASE=planning` results in `Frontmatter["phase"] = "planning"` appearing in the materialized vault file
4. Leaving `WATCHER_GITHUB_BUILD_TASK_PHASE` unset (or set to empty string) results in NO `phase:` key in the materialized vault file (matches today's behavior — verified against `OpenClaw/tasks/710db30e-86e2-59a6-8ac1-9c7718122d4e.md`)
5. Setting nothing produces today's exact output (`assignee: build-fixer-agent`, `status: todo`, no `phase` field)

## Constraints

- `Frontmatter["phase"]` MUST be omitted (not set to empty string) when the env is empty — empty `phase: ""` in YAML is observably different from no `phase` key
- `assignee` and `status` are required-non-empty (defaults guarantee non-empty; explicit empty CLI/env override should be rejected at startup with a clear error). Reusing the existing `service.Main` validation framework via the `required:"true"` struct tag is the path of least surprise — but defaults satisfy the requirement, so this only catches operators who pass an explicit empty string
- `pkg.NewWatcher` signature changes — callers (factory + tests) update accordingly
- `buildWatcher` stores the three values; `buildCreateTaskCommand` takes them as parameters (or reads from the struct receiver). NO package-level mutable state, NO globals. Mirror `go-time-injection.md` discipline
- Existing tests in `pkg/watcher_test.go` MUST pass — they don't currently exercise this codepath but should be updated to construct `NewWatcher` with the new params and to assert frontmatter values when relevant
- Error wrapping: `github.com/bborbe/errors`, never `fmt.Errorf`
- The `cmd/run-once` binary mirrors the same args/env so smoke tests can exercise non-default values
- The default deploy (no env set in `dev.env` / `prod.env` / StatefulSet) MUST still work — the args' defaults cover the no-override case

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| All three env vars unset | Defaults applied; today's output unchanged | none |
| `WATCHER_GITHUB_BUILD_TASK_ASSIGNEE=`	(explicit empty) | Startup fails with `assignee must not be empty` (or equivalent service-framework validation) | operator sets a non-empty value |
| `WATCHER_GITHUB_BUILD_TASK_STATUS=` (explicit empty) | Same as above | same |
| `WATCHER_GITHUB_BUILD_TASK_PHASE=` (explicit empty) | Treated as "omit field", not error; phase key NOT written to frontmatter | by design — empty disables the field |
| `WATCHER_GITHUB_BUILD_TASK_ASSIGNEE=build-fixer-agent` (explicit, matches default) | Same output as unset; logged | none |

## Acceptance Criteria

- [ ] Setting `WATCHER_GITHUB_BUILD_TASK_ASSIGNEE=foo` at startup results in published vault tasks carrying `assignee: foo`
- [ ] Setting `WATCHER_GITHUB_BUILD_TASK_STATUS=backlog` results in `status: backlog` in published tasks
- [ ] Setting `WATCHER_GITHUB_BUILD_TASK_PHASE=planning` results in `phase: planning` appearing in published tasks
- [ ] Leaving all three unset reproduces today's exact published frontmatter (`assignee: build-fixer-agent`, `status: todo`, no `phase` key)
- [ ] `WATCHER_GITHUB_BUILD_TASK_PHASE=` (explicit empty) results in NO `phase` key in published frontmatter (NOT `phase: ""`)
- [ ] The long-running watcher binary AND the one-shot `cmd/run-once` binary both honor all three overrides identically
- [ ] `cmd/run-once/Makefile`'s `run-once` target lets the operator override any of the three on the make command line
- [ ] README (`watcher/github-build/README.md`) Environment Variables table includes the three new entries with defaults
- [ ] CHANGELOG entry under `## Unreleased`
- [ ] `make precommit` clean from `watcher/github-build/`

## Verification

```bash
cd watcher/github-build && make precommit
```

Smoke test the override path locally (port-forward Kafka first):

```bash
cd watcher/github-build/cmd/run-once
make run-once WATCHER_GITHUB_BUILD_TASK_ASSIGNEE=test-agent WATCHER_GITHUB_BUILD_TASK_STATUS=backlog WATCHER_GITHUB_BUILD_TASK_PHASE=planning REPO_ALLOWLIST=github.com/bborbe/maintainer

# Expected: a vault task with frontmatter
#   assignee: test-agent
#   status: backlog
#   phase: planning
# (assuming the repo is currently red and a publish is produced)
```

## Do-Nothing Option

Leave the literals hard-coded. Cost: every operator-level frontmatter change requires a code patch + rebuild + redeploy cycle. That's tolerable today (rare event), but it blocks the more useful per-repo override (`per-repo-maintenance-yaml`), which depends on having configurable defaults to fall back to. So this spec is also a structural prerequisite for the next one.
