---
status: completed
spec: [041-rename-watcher-github-pr-task-suffix]
summary: 'Renamed WATCHER_GITHUB_PR_TASK_SUFFIX to TASK_SUFFIX in watcher/github-pr main.go and k8s StatefulSet manifest, added ## Unreleased changelog entry'
container: maintainer-exec-151-spec-041-rename-task-suffix
dark-factory-version: v0.169.0
created: "2026-05-24T19:40:13Z"
queued: "2026-05-24T19:45:04Z"
started: "2026-05-24T19:45:06Z"
completed: "2026-05-24T19:46:54Z"
branch: dark-factory/rename-watcher-github-pr-task-suffix
---

## Summary

- Rename `WATCHER_GITHUB_PR_TASK_SUFFIX` to `TASK_SUFFIX` in `watcher/github-pr/main.go` env tag on the `TaskSuffix` field (line 127)
- Rename the same env var in the Kubernetes StatefulSet manifest at `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml` (lines 57-58)
- Add `## Unreleased` changelog entry documenting the breaking env-var rename
- Verify build and tests pass

## Objective

Unify the PR watcher's task-suffix env var with the build watcher introduced in spec 040. Change the `env:` struct tag on the `TaskSuffix` field from `WATCHER_GITHUB_PR_TASK_SUFFIX` to `TASK_SUFFIX`. Behavior is unchanged — only the env var name changes.

## Context

Read the relevant files before making changes (paths are repo-relative; the container mounts the project at the working directory):
- `watcher/github-pr/main.go` — contains the `TaskSuffix` field declaration. Locate by literal content `env:"WATCHER_GITHUB_PR_TASK_SUFFIX"` rather than line number (line ~127 at spec time but may drift).
- `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml` — locate the env block by content (`- name: WATCHER_GITHUB_PR_TASK_SUFFIX` and the adjacent `value:` line; ~lines 57-58 at spec time).
- `CHANGELOG.md` — top of file. Existing format: `# Changelog` title, then version sections (`## v0.26.7` is currently the first). New entries prepend a new `## Unreleased` section directly under `# Changelog`, above the first `## vX.Y.Z`. Preamble bullets (if any) stay in place.

## Requirements

1. In `watcher/github-pr/main.go`, locate the `TaskSuffix` field on the `application` struct by searching for the literal `env:"WATCHER_GITHUB_PR_TASK_SUFFIX"` (line ~127 at spec time — use the literal as the primary anchor, the line number only as a hint):

   ```go
   TaskSuffix       string           `required:"false" arg:"task-suffix"       env:"WATCHER_GITHUB_PR_TASK_SUFFIX" usage:"..."`
   ```

   Change only the `env:` tag value from `WATCHER_GITHUB_PR_TASK_SUFFIX` to `TASK_SUFFIX`. Leave all other attributes (field name, arg, required, usage) unchanged.

2. In `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml`, locate the env block by searching for `- name: WATCHER_GITHUB_PR_TASK_SUFFIX` (lines ~57-58 at spec time — use the literal as the primary anchor):

   ```yaml
   - name: WATCHER_GITHUB_PR_TASK_SUFFIX
     value: '{{ "WATCHER_GITHUB_PR_TASK_SUFFIX" | env }}'
   ```

   Change both occurrences to `TASK_SUFFIX` — the name field and the value field:

   ```yaml
   - name: TASK_SUFFIX
     value: '{{ "TASK_SUFFIX" | env }}'
   ```

   **Preserve the exact existing template-brace spacing of the line you're editing** (some entries in this file use `{{ "X" | env }}` with spaces, others use `{{"X" | env}}` without — match what's already on the line, do not normalize sibling lines).

3. Add a changelog entry under `## Unreleased` in `CHANGELOG.md`. If `## Unreleased` does not exist, insert it directly under the `# Changelog` title (and any preamble lines that follow it), immediately above the first `## vX.Y.Z` section (currently `## v0.26.7`). Use prefix `chore:` since this is a config-change side-effect of a feat (spec 040 introduced `TASK_SUFFIX`):

   ```
   ## Unreleased

   - chore: Rename `WATCHER_GITHUB_PR_TASK_SUFFIX` to `TASK_SUFFIX` in watcher/github-pr to match build watcher unified naming — breaking change, operator must update env files at deploy time
   ```

   The `# Changelog` title and any preamble lines (links, MAJOR/MINOR/PATCH bullets if present) must remain intact above the new `## Unreleased` section.

## Constraints

- Do NOT rename the Go field `TaskSuffix` — only the `env:` tag changes
- Do NOT rename the arg flag `--task-suffix` — already unified-style
- Do NOT modify `dev.env` or `prod.env` — operator-owned change, outside this spec
- Do NOT add a fallback shim that reads both old and new env var names — hard cutover
- Do NOT commit — dark-factory handles git

## Verification

1. `grep -rn 'WATCHER_GITHUB_PR_TASK_SUFFIX' watcher/github-pr/` returns 0 lines
2. `grep -n 'env:"TASK_SUFFIX"' watcher/github-pr/main.go` returns ≥1 line on the `TaskSuffix` field
3. `grep -n 'TASK_SUFFIX' watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml` returns ≥2 lines (name and value sides of the mapping)
4. `grep -n 'WATCHER_GITHUB_PR_TASK_SUFFIX' CHANGELOG.md` returns ≥1 line under `## Unreleased`
5. `cd watcher/github-pr && go build ./...` exits 0
6. `cd watcher/github-pr && go test ./...` exits 0
7. `cd watcher/github-pr && make precommit` exits 0 (format + generate + test + lint + license; root-level precommit also exists but service-dir precommit is the correct gate per project conventions).

## Notes

- The `--task-suffix` arg flag name already matches the new env var style — no change needed there
- The `TaskSuffix` field is passed to `CreateWatcher` and `CreateSinglePRHandler` in `main.go` — no changes to those call sites
- No test code in `watcher/github-pr/` references the old env var name by string — tests use the field struct directly, so test changes are not needed