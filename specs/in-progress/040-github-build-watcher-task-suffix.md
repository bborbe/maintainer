---
status: prompted
tags:
    - dark-factory
    - draft
approved: "2026-05-24T18:51:08Z"
generating: "2026-05-24T18:51:08Z"
prompted: "2026-05-24T18:54:41Z"
branch: dark-factory/github-build-watcher-task-suffix
---

# Add TASK_SUFFIX to github-build-watcher

## Summary

Add a stage-specific suffix to build-failure task filenames so dev and prod `watcher/github-build` instances don't overwrite each other in the shared OpenClaw vault. Mirror the existing `watcher/github-pr` `TaskSuffix` pattern 1:1.

## Problem

Today `watcher/github-build` produces task filenames like `Build Failure github - bborbe-go-skeleton - <sha>.md`. When dev and prod watchers both poll the same repo into the same OpenClaw vault, they generate **identical filenames** — later writes silently overwrite earlier ones.

Concrete overlap today: dev's `REPO_ALLOWLIST=github.com/bborbe/go-skeleton` intersects prod's `REPO_ALLOWLIST=github.com/bborbe/*` on `go-skeleton`. Any red episode on that repo gets emitted by both watchers with the same filename. The bug is latent for everything else but widens the moment dev's allowlist expands.

The PR watcher already solved the same problem with `WATCHER_GITHUB_PR_TASK_SUFFIX`. Build watcher should do the same.

## Goal

`watcher/github-build` reads a new `WATCHER_GITHUB_BUILD_TASK_SUFFIX` env var and appends `' - <suffix>'` to every OpenClaw task filename it produces. Empty suffix produces the legacy filename unchanged.

## Desired Behavior

- New `TaskSuffix` field on the `application` struct in `watcher/github-build/main.go`, declared with env `WATCHER_GITHUB_BUILD_TASK_SUFFIX`, arg `--task-suffix`, usage text matching the PR watcher's wording.
- `TaskSuffix` is plumbed through the same path the PR watcher uses (call sites visible at `watcher/github-pr/main.go:222, :242`) into the filename generator.
- When `TaskSuffix=""` (unset), generated filename is `Build Failure github - <repo> - <sha>.md` (unchanged from today).
- When `TaskSuffix="dev"`, generated filename is `Build Failure github - <repo> - <sha> - dev.md`.
- `k8s/maintainer-watcher-github-build-sts.yaml` declares the new env var, sourced from `WATCHER_GITHUB_BUILD_TASK_SUFFIX` env, mirroring the PR watcher's sts entry.

## Constraints

- Implementation must mirror `watcher/github-pr` exactly — same field shape, same wiring path, same usage text (modulo "PR" → "build-failure"). No novel design.
- Empty suffix must be a valid value producing the legacy filename — required for backward compatibility and single-watcher deployments.
- No changes to existing task files in the vault. Migration is forward-only.

## Acceptance Criteria

1. `grep -n 'TaskSuffix' watcher/github-build/main.go` shows the field declaration with `env:"WATCHER_GITHUB_BUILD_TASK_SUFFIX"` and is referenced at the call site that creates the watcher / filename generator.
2. `grep -n 'WATCHER_GITHUB_BUILD_TASK_SUFFIX' watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml` shows the env var wired through the same templating pattern as `WATCHER_GITHUB_PR_TASK_SUFFIX` in the PR sts.
3. A **unit test** on the filename generator verifies: with `TaskSuffix="dev"`, generated filename ends with ` - dev.md`; with `TaskSuffix=""`, generated filename ends with `.md` without an extra suffix segment.
4. Existing tests for `watcher/github-build` continue to pass.
5. `grep -n 'WATCHER_GITHUB_BUILD_TASK_SUFFIX' CHANGELOG.md` returns ≥1 line, and that line appears under the `## Unreleased` section.

## Failure Modes

| Trigger | Expected | Recovery |
|---|---|---|
| `WATCHER_GITHUB_BUILD_TASK_SUFFIX` not set in deployment | TaskSuffix is empty string; filenames keep legacy shape | None needed (backward-compatible default) |
| Suffix contains characters invalid in filenames (e.g. `/`, `\0`) | Mirror the PR watcher's existing handling exactly — implementing agent reads `watcher/github-pr` to determine the rule and applies the same one here; if PR watcher does no validation, do none. No new policy introduced. | None — operator misconfiguration; mirroring the PR watcher's behavior is the documented contract |
| Two watchers configured with the same suffix | Filename collision returns (the bug this spec fixes) | Operator misconfiguration; not handled here |

## Do-Nothing Option

If this spec is never implemented: the dev and prod `watcher/github-build` instances keep producing identical filenames for any repo in both allowlists. Today that's only `bborbe/go-skeleton`, where the later writer silently overwrites the earlier one. Dev's allowlist will eventually expand (current value `github.com/bborbe/go-skeleton` is intentionally narrow for testing), at which point every overlapping repo's red episodes lose one of the two emissions. Operator loses signal; auto-fix pipeline may miss tasks. Cost: latent silent data loss on a growing surface.

## Non-goals

- Setting `WATCHER_GITHUB_BUILD_TASK_SUFFIX` value in `dev.env` / `prod.env` — operator's responsibility (a follow-up of this code change). `WATCHER_GITHUB_BUILD_TASK_ASSIGNEE` was already set similarly on 2026-05-24; same pattern.
- Renaming existing task files in the OpenClaw vault to add the suffix retroactively — forward-only migration.
- Introducing new validation rules for suffix content beyond what the PR watcher does — match PR watcher exactly (Failure Mode row 2).
- Per-repo or per-task-type suffix overrides — single stage-wide suffix is sufficient.

## Verification

- Unit / integration test added (AC #3) confirms filename shape under both empty and non-empty suffix.
- After deploy: trigger or wait for a real failed build in dev; observe new task file has `- dev.md` suffix. Same in prod produces no suffix.

## Related

- Reference implementation in `watcher/github-pr`. Line numbers may drift; implementing agent should locate via `grep -n 'TaskSuffix' watcher/github-pr/main.go` and `grep -n 'WATCHER_GITHUB_PR_TASK_SUFFIX' watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml`. At spec time (2026-05-24): main.go:127 (field), :222, :242 (call sites); sts.yaml:57-58 (env wiring).
- Companion change already shipped: `dev.env` + `prod.env` updated 2026-05-24 to set `WATCHER_GITHUB_BUILD_TASK_ASSIGNEE=build-fix-agent`. `WATCHER_GITHUB_BUILD_TASK_SUFFIX` env value is NOT yet set in env files — operator will set after this code change lands (per Non-goals).
- Triggering task in operator vault: `[[Watcher Assigns Build-Failure Tasks to build-fix-agent]]`.
