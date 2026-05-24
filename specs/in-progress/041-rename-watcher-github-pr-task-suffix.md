---
status: generating
tags:
    - dark-factory
    - draft
approved: "2026-05-24T19:40:13Z"
generating: "2026-05-24T19:40:13Z"
branch: dark-factory/rename-watcher-github-pr-task-suffix
---

## Summary

Rename the existing PR watcher task-suffix env var from `WATCHER_GITHUB_PR_TASK_SUFFIX` to plain `TASK_SUFFIX`, matching the unified naming introduced by spec 040 for the build watcher. Single shared env var per deployment for stage identity.

## Problem

Spec 040 added `TASK_SUFFIX` to the build watcher. The PR watcher still reads the verbose `WATCHER_GITHUB_PR_TASK_SUFFIX`. With both deployed, `dev.env` and `prod.env` carry two redundant lines that must always hold the same value — a maintenance trap. Drift will eventually happen and one watcher will produce wrong filenames.

Unification: every watcher / agent reads the same `TASK_SUFFIX`. Stage identity is one config knob, not N.

## Goal

`watcher/github-pr` reads `TASK_SUFFIX` (not `WATCHER_GITHUB_PR_TASK_SUFFIX`) for its existing `TaskSuffix` field. Behavior unchanged; env var name only.

## Desired Behavior

- `TaskSuffix` field in `watcher/github-pr/main.go` keeps its name and arg (`--task-suffix`); only the `env:` tag changes from `WATCHER_GITHUB_PR_TASK_SUFFIX` to `TASK_SUFFIX`.
- `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml` env entry changes name + templated value substitution from `WATCHER_GITHUB_PR_TASK_SUFFIX` to `TASK_SUFFIX` on both sides.
- All other wiring (call sites, filename generator) unchanged.
- `CHANGELOG.md` gains an `## Unreleased` entry documenting the breaking env-var rename.

## Constraints

- **Hard cutover, no fallback shim.** Internal-only deployment; one operator owns dev + prod. Reading `TASK_SUFFIX` first with fallback to `WATCHER_GITHUB_PR_TASK_SUFFIX` is unnecessary complexity for an audience of one.
- Go field name `TaskSuffix` stays — only the `env:` struct tag changes.
- Arg flag `--task-suffix` stays — already matches the env var minus underscoring convention.
- Do NOT modify `dev.env` or `prod.env` in this spec — operator-owned, separate change window (Non-goal).
- Do NOT commit — dark-factory handles git.

## Acceptance Criteria

1. `grep -rn 'WATCHER_GITHUB_PR_TASK_SUFFIX' watcher/github-pr/` returns 0 lines (purged from source + manifests).
2. `grep -n 'env:"TASK_SUFFIX"' watcher/github-pr/main.go` returns ≥1 line on the `TaskSuffix` field declaration.
3. `grep -n 'TASK_SUFFIX' watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml` returns ≥1 line for the env name AND ≥1 line for the templated value (both sides of the mapping use the new name).
4. `grep -n 'WATCHER_GITHUB_PR_TASK_SUFFIX' CHANGELOG.md` returns ≥1 line under `## Unreleased` documenting the breaking-rename.
5. `cd watcher/github-pr && go build ./...` exits 0.
6. `cd watcher/github-pr && go test ./...` exits 0 (no test code references the old env var name).
7. `make precommit` (or repo-equivalent project-wide gate) exits 0 if the target exists.

## Failure Modes

| Trigger | Expected | Recovery |
|---|---|---|
| Operator forgets to set `TASK_SUFFIX` in env files at deploy time | TaskSuffix is empty string → legacy filenames (no suffix) — same as PR watcher's pre-suffix behavior | Operator sets the env var; restart pod |
| Stale references to `WATCHER_GITHUB_PR_TASK_SUFFIX` in any non-watcher code (other services, scripts) | AC #1 catches anything in `watcher/github-pr/` only; other repos may still reference the old name | Out of scope; manually grep other repos at deploy time |

## Do-Nothing Option

Two env vars carry the same value forever, drift risk grows, every new watcher repeats the same per-component naming mistake. Cost: latent misconfiguration risk + boilerplate in every env file forever.

## Non-goals

- **Backward-compat fallback** reading both old and new env vars — explicit hard cutover.
- **Modifying `dev.env` / `prod.env`** — operator-owned change, done in same deploy window but outside the dark-factory spec.
- **Renaming the Go field `TaskSuffix`** — only the env tag changes.
- **Renaming the arg flag `--task-suffix`** — already unified-style.
- **Migrating other watchers / agents** beyond `watcher/github-pr` — spec 040 already covered the build watcher; future watchers are written using `TASK_SUFFIX` from the start.

## Verification

- After deploy with `TASK_SUFFIX=dev` set in dev env: trigger or observe a real PR event in dev; PR task filename ends with ` - dev.md`.
- After deploy with `TASK_SUFFIX=` (empty) in prod: PR task filename has no suffix segment (matches legacy behavior).

## Related

- Companion / parent: `specs/in-progress/040-github-build-watcher-task-suffix.md` (introduced `TASK_SUFFIX` for the build watcher)
- Triggering vault task: [[Watcher Assigns Build-Failure Tasks to build-fix-agent]] — broader deployment-config rollout
- Touches: `watcher/github-pr/main.go`, `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml`, `CHANGELOG.md`
