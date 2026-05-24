---
status: completed
spec: [040-github-build-watcher-task-suffix]
summary: Added TASK_SUFFIX env/arg to github-build watcher, plumbed through factory and watcher, updated computeBuildTitle with suffix truncation logic matching PR watcher, updated StatefulSet YAML and CHANGELOG
container: maintainer-exec-149-spec-040-task-suffix-code
dark-factory-version: v0.169.0
created: "2026-05-24T19:00:00Z"
queued: "2026-05-24T19:05:04Z"
started: "2026-05-24T19:05:06Z"
completed: "2026-05-24T19:09:06Z"
branch: dark-factory/github-build-watcher-task-suffix
---

<summary>
- Add `TaskSuffix` field to the `application` struct in `main.go` with env `TASK_SUFFIX` and arg `--task-suffix`
- Plumb `taskSuffix` through `pkg/factory/factory.go` into `NewWatcher`
- Store `taskSuffix` on the `buildWatcher` struct and pass it to `computeBuildTitle`
- Update `computeBuildTitle` to accept a `taskSuffix` parameter and append `' - <suffix>'` when non-empty (preserving existing title-length cap logic)
- Add `TASK_SUFFIX` env var to the StatefulSet YAML, mirroring `WATCHER_GITHUB_PR_TASK_SUFFIX` in the PR sts
- Append `## Unreleased` changelog entry
</summary>

<objective>
Add stage-specific task suffix to build-failure filenames so dev and prod `watcher/github-build` instances do not overwrite each other in the shared OpenClaw vault. Mirror the existing `watcher/github-pr` `TaskSuffix` pattern exactly.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Read these files before making changes (all verified at spec time 2026-05-24):
- `watcher/github-build/main.go` — application struct (lines 82-101) and Run call (lines 146-158)
- `watcher/github-build/pkg/factory/factory.go` — `CreateWatcher` signature and `NewWatcher` call
- `watcher/github-build/pkg/watcher.go` — `NewWatcher` constructor, `buildWatcher` struct, and call to `computeBuildTitle` at line 331
- `watcher/github-build/pkg/filename.go` — `computeBuildTitle` function signature and implementation
- `watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml` — env var section (lines 44-69)
- `watcher/github-pr/main.go` lines 127, 222, 242 — reference: `TaskSuffix` field, `a.TaskSuffix` at both call sites
- `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml` lines 57-58 — reference: env var wiring pattern
- `CHANGELOG.md` — format and where to append `## Unreleased`
</context>

<requirements>
1. In `watcher/github-build/main.go`, add to the `application` struct after `MaxTitleLen`:
   ```go
   TaskSuffix       string           `required:"false" arg:"task-suffix"     env:"TASK_SUFFIX" usage:"Optional suffix appended to build-failure task filenames as ' - suffix'; empty = no suffix. Use distinct values per stage to prevent task-file collisions when both watchers poll the same repo into the same vault."`
   ```
   Usage text must match the PR watcher's wording except "PR" replaced with "build-failure".

2. In `watcher/github-build/main.go`, update the `factory.CreateWatcher(...)` call inside `Run` (lines 146-158) to append `a.TaskSuffix` as a new final argument after `a.MaxTitleLen`.

3. In `watcher/github-build/pkg/factory/factory.go`, update `CreateWatcher` to accept `taskSuffix string` as the last parameter. Add it to the `NewWatcher` call after `maxTitleLen`.

4. In `watcher/github-build/pkg/watcher.go`, update `NewWatcher` to accept `taskSuffix string` as the last parameter. Store `taskSuffix` on the `buildWatcher` struct and update `buildCreateTaskCommand` to pass `w.taskSuffix` to `computeBuildTitle`.

5. In `watcher/github-build/pkg/filename.go`, update `computeBuildTitle` signature to:
   ```go
   func computeBuildTitle(provider, owner, repo, episodeSHA string, maxTitle int, taskSuffix string) string {
   ```
   After constructing the base title string, append ` - <suffix>` when `taskSuffix != ""`. Apply the same truncation-before-suffix logic that the PR watcher uses — when the final title exceeds `maxTitle`, truncate the base title to make room for the suffix (do NOT drop the suffix; preserving it is the whole point).

   **Read `watcher/github-pr/pkg/filename.go` `computePRTitle` (line 31 onward, ~99 lines total) before implementing** to copy the exact truncation algorithm. Do not invent a different one.

6. In `watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml`, append a new env entry inside the existing `env:` list. Insert **between the `TASK_PHASE` value line (~line 69) and the `image:` line (~line 70)**. Match the existing 12-space indent for `- name:` and 14-space indent for `value:` exactly (see surrounding entries like `TASK_PHASE`):

   ```yaml
               - name: TASK_SUFFIX
                 value: '{{"TASK_SUFFIX" | env}}'
   ```

   Note: surrounding entries use `'{{"VAR" | env}}'` (no spaces inside braces) — match that style exactly.

7. In `CHANGELOG.md`, add `## Unreleased` section at the top (above `## v0.26.6`) with entry:
   ```
   - feat(watcher/github-build): add `TASK_SUFFIX` env var to disambiguate build-failure task filenames per stage, preventing dev/prod filename collisions in the shared vault
   ```

8. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.

9. Run `cd watcher/github-build && go test ./...` to confirm existing tests pass.
</requirements>

<constraints>
- Implementation must mirror `watcher/github-pr` field shape and wiring path. No novel design.
- Two deliberate divergences from a pure mirror:
  - Env var name is `TASK_SUFFIX` (NOT `WATCHER_GITHUB_PR_TASK_SUFFIX`) — forward-looking unified name; PR watcher will migrate to the same name in a follow-up spec.
  - Usage text "PR" → "build-failure".
- Empty suffix must be a valid value producing the legacy filename unchanged.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass after changes.
</constraints>

<verification>
- `grep -n 'TaskSuffix' watcher/github-build/main.go` shows the field declaration with `env:"TASK_SUFFIX"` AND a reference at the `factory.CreateWatcher(...)` call site (`grep -n 'a.TaskSuffix' watcher/github-build/main.go` returns ≥1)
- `grep -n 'taskSuffix' watcher/github-build/pkg/factory/factory.go` and `watcher/github-build/pkg/watcher.go` and `watcher/github-build/pkg/filename.go` each return ≥1 line (parameter plumbed end-to-end)
- `grep -n 'TASK_SUFFIX' watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml` shows the env var wired through the templating pattern
- `grep -n 'TASK_SUFFIX' CHANGELOG.md` returns ≥1 line, and that line appears under `## Unreleased`
- `cd watcher/github-build && go build ./...` exits 0
- `cd watcher/github-build && go test ./...` exits 0
</verification>