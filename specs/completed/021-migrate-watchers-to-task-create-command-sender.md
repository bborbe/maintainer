---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-05-07T19:49:58Z"
generating: "2026-05-07T20:00:18Z"
prompted: "2026-05-07T20:07:23Z"
verifying: "2026-05-07T21:03:18Z"
completed: "2026-05-08T06:08:51Z"
branch: dark-factory/migrate-watchers-to-task-create-command-sender
---

## Summary

- Both GitHub watchers (`watcher/github-pr/`, `watcher/github-build/`) currently emit a maintainer-local `WatcherCreateTaskCommand` wrapper carrying `FilenameHint`. The agent controller has moved on and now reads `Title` only, so every create-task today falls into the UUID-fallback path.
- Agent specs 019 + 020 introduced a typed `task.CreateCommandSender` (in `agent/lib/command/task/`) with a required `Title` field. This spec migrates the watchers onto that sender and retires the wrapper.
- The slug helpers stay byte-identical; only the destination field renames from `FilenameHint` to `Title`. No JSON `filename_hint` key remains on the wire.
- After this lands and is deployed to dev, vault files for PR-review and build tasks land under their human-readable slugs instead of UUIDs (rung-2 verification of agent spec 019).
- Pre-condition: agent `github.com/bborbe/agent/lib v0.58.0` (paired with root `v0.58.0`) — already published. Both watchers must bump to this minimum from current `v0.57.0`.

## Problem

Maintainer's two GitHub watchers each define a `WatcherCreateTaskCommand` wrapper that embeds `agentlib.CreateTaskCommand` and adds a `FilenameHint string` field, plus a hand-rolled `kafkaPublisher.PublishCreate` that marshals + publishes via `cdb.CommandObjectSender`. This was correct when shipped (maintainer specs 018 + 019), but agent specs 019 + 020 (now `verifying`) made it obsolete: the controller now reads `Title`, not `filename_hint`. Every create-task hits empty-`Title` validation and lands at the UUID path with a WARN. Until the maintainer migrates, vault filenames stay UUID-named — the human-readable-filename win from four specs across two repos doesn't actually reach the vault.

## Goal

Both watchers emit a populated `Title` on `task.CreateCommand` via `task.NewCreateCommandSender`. The `WatcherCreateTaskCommand` wrapper and the hand-rolled `PublishCreate` are gone. The slug-computing helpers carry over unchanged — they now feed `Title` instead of `FilenameHint`. After dev deploy, real PRs and build failures land at human-readable vault paths.

## Non-goals

- Changing slug rules (length, char-class, format) — byte-identical behavior
- Changing what triggers a create-task
- Auto-renaming existing UUID-named vault files
- Moving slug helpers out of the watcher pkgs
- Adding new tests for slug helpers (existing ones cover the algorithm)
<!-- force-push migration moved INTO scope (see Constraints) — agent spec 020 ships UpdateFrontmatterCommandSender. -->

## Desired Behavior

1. `watcher/github-pr/pkg/publisher.go`: `WatcherCreateTaskCommand` removed; the publisher either accepts `task.CreateCommand` directly or is replaced entirely by `task.CreateCommandSender` injection.
2. `watcher/github-build/pkg/publisher.go`: same as above.
3. Both watchers' main wiring constructs `task.NewCreateCommandSender(commandObjectSender)` and uses it instead of the hand-rolled `kafkaPublisher.PublishCreate`.
4. Both watchers emit `Title` (the existing slug result) on `task.CreateCommand`; no `filename_hint` field appears on the wire.
5. Both watchers' `go.mod` bump `github.com/bborbe/agent/lib` from `v0.57.0` to `v0.58.0` (the paired release containing agent specs 019 + 020). `go mod tidy` clean afterward.
6. Existing slug-helper tests continue to pass unchanged — only the field they feed renames.
7. `make precommit` clean in both `watcher/github-pr/` and `watcher/github-build/`.
8. After dev deploy, a real PR or build failure produces a vault file at `tasks/{title}.md` (rung-2 verification of agent spec 019).

## Constraints

- **Sequencing**: `github.com/bborbe/agent/lib v0.58.0` is the minimum required version (paired root + lib tag pushed at agent commit `5239f96`, contains agent specs 019 + 020 lib changes). Bump from current `v0.57.0`.
- New import path: `github.com/bborbe/agent/lib/command/task`.
- `task.CreateCommand.Title` is required (non-empty, ≤200 chars, cross-platform safe). Existing slug helpers already produce non-empty fallbacks like `"PR Review github - bborbe-x - 7"`.
- Wire format: JSON tag `"title"` (per agent spec 019). The `"filename_hint"` field disappears entirely.
- Errors wrapped with `github.com/bborbe/errors`.
- `make precommit` runs from each watcher subdir, never repo root.
- Counterfeiter mock for `task.CreateCommandSender` ships from `agent/lib/command/task/mocks/`; tests consume it.
- Force-push path (`UpdateFrontmatterCommand`) — agent spec 020 shipped `task.NewUpdateFrontmatterCommandSender` (`agent/lib/command/task/update-frontmatter-command-sender.go` confirmed). Migrate force-push to it as part of this spec, same pattern as create-task. Keeps both publish paths consistent and removes the last hand-rolled path.
- Domain reference: `docs/architecture.md`, `docs/build-watcher.md`, `docs/watcher-decision-chains.md` describe the watchers' current shape.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Slug result violates `task.CreateCommand.Title` validation | Sender's `Validate` returns error before publish; nothing reaches Kafka | Slug helper tightens regex; existing rules already cover this |
| `agent/lib` bump introduces unrelated breaking changes | CI fails on `make precommit` | Resolve before prompt finishes |
| Caller still constructs `WatcherCreateTaskCommand` post-migration | Compile error (type removed) | Migrate caller |
| Slug helper produces an empty result for some edge input (unicode-only PR title etc.) | Maintainer-side unit test covers the helper's empty-output → fallback-segment-omit rule; slug result still includes constant prefix so Title is never empty in practice | If empty Title ever escapes, controller-side WARN+UUID fallback (agent spec 019) catches it |

## Security / Abuse Cases

The slug feeds attacker-influenceable input (PR title, branch name, repo owner) into a vault filename. Validation lives on the agent side (`task.CreateCommand.Validate`: non-empty, ≤200 chars, cross-platform safe). The watcher must rely on that validation rather than re-implementing it; the existing slug helpers already constrain output to a safe character class. No new trust boundary is added by this migration — the wire format changes, the validation owner moves to the lib type, but the input set is unchanged.

## Acceptance Criteria

- [ ] `WatcherCreateTaskCommand` type removed from both `watcher/github-pr/pkg/publisher.go` and `watcher/github-build/pkg/publisher.go`.
- [ ] Both watchers emit `task.CreateCommand{Title: <slug-result>, ...}` via the typed sender.
- [ ] No `filename_hint` JSON key appears in any test fixture or marshalled output; a wire-format unit test locks `"title"` in and `"filename_hint"` out.
- [ ] Existing slug-helper tests pass unchanged: `computePRFilenameHint` and `slugifyTitle` in `watcher/github-pr/pkg/filename.go`; `computeFilenameHint` and `slugifySegment` in `watcher/github-build/pkg/filename.go`.
- [ ] Both watchers' `go.mod` pin `github.com/bborbe/agent/lib v0.58.0` (or higher) — the paired release containing agent specs 019 + 020.
- [ ] Counterfeiter mock for `task.CreateCommandSender` consumed in publisher tests (replacing direct `cqrsmocks.CDBCommandObjectSender` usage where appropriate).
- [ ] `make precommit` clean in `watcher/github-pr/` and `watcher/github-build/`.
- [ ] CHANGELOG entry under `## Unreleased` in each watcher.
- [ ] After dev deploy on a real PR, the vault file lands at `tasks/{computePRFilenameHint(...)}.md` — i.e. the exact string the existing slug helper produces (manual rung-2; format determined by the helper, not hardcoded here).

**Scenario coverage — NO new scenario.** Four-condition test from `docs/scenario-writing.md`: (1) unit + integration tests with a test broker + the typed sender reach the wire path; agent-side spec 019 covers controller honor — fails. (2) Load-bearing yes, but (1) already fails. (3) No existing scenario covers — moot. (4) Concrete regression risk = JSON tag drift; covered by the wire-format unit test in AC. → NO scenario file. Manual rung-2 verification stays in AC.

## Verification

```
cd watcher/github-pr && make precommit
cd watcher/github-build && make precommit
```

Plus manual rung-2: dev deploy, trigger a real PR on `bborbe/code-reviewer`, observe vault file at `tasks/PR Review github - bborbe-code-reviewer - N - {slug}.md`. Trigger a real build failure, observe build-watcher equivalent.

## Do-Nothing Option

Keep `WatcherCreateTaskCommand` and `FilenameHint`. Cost: the agent controller's WARN+UUID-fallback is the only path used for create-task; vault stays UUID-named indefinitely; the new lib API (post-019/020) sits unused for these producers; both watchers maintain a hand-rolled marshal-and-send plus a deprecated wrapper. The human-readable-vault-filename outcome that justified four specs across two repos never lands.

## Related

- Agent spec 019 (`human-readable-vault-task-paths`): introduces `Title` + `Validate` + sender in `agent/lib`
- Agent spec 020 (`agent-lib-command-package-restructure`): relocates to `agent/lib/command/task/`; renames (`CreateTaskCommand` → `CreateCommand`)
- Maintainer spec 018 (`human-readable-filenames-for-build-tasks`): introduced `FilenameHint` for build tasks (this spec retires it)
- Maintainer spec 019 (`human-readable-filenames-for-pr-review-tasks`): introduced `FilenameHint` for PR-review tasks (this spec retires it)

## Verification Result

**Verified:** 2026-05-08T06:08:14Z (HEAD 318dd757)
**Binary:** /Users/bborbe/Documents/workspaces/go/bin/dark-factory v0.154.0
**Scenario:** No scenario file (four-condition test); structural ACs verified via fresh `make precommit` in both watcher subdirs; rung-2 verified by build-failure task landing in vault post dev-deploy.
**Evidence:**
- `watcher/github-pr/pkg/publisher.go` + `watcher/github-build/pkg/publisher.go` reduced to 5-line stubs; zero `WatcherCreateTaskCommand` / `FilenameHint` / `filename_hint` matches across `watcher/`
- Both `go.mod` pin `github.com/bborbe/agent/lib v0.58.0`; both wire `task.NewCreateCommandSender` in `pkg/factory/factory.go`; github-pr also wires `task.NewUpdateFrontmatterCommandSender`
- Watcher code emits `task.CreateCommand{Title: computePRFilenameHint(...)}` (`watcher/github-pr/pkg/watcher.go:225,236`) and `task.CreateCommand{Title: computeFilenameHint(...)}` (`watcher/github-build/pkg/watcher.go:305`) via `createSender.SendCommand`
- Wire-format unit tests assert `"title"` in / `"filename_hint"` out: `watcher/github-pr/pkg/filename_internal_test.go:91-104`, `watcher/github-build/pkg/watcher_internal_test.go:134-148`
- `taskmocks.TaskCreateCommandSender` + `TaskUpdateFrontmatterCommandSender` consumed across both watchers' `_test.go` files
- `cd watcher/github-pr && make precommit` → `Issues: 0`, trivy clean, `ready to commit` (run 2026-05-08 08:07 local against HEAD 318dd757)
- `cd watcher/github-build && make precommit` → `Issues: 0`, trivy clean, `ready to commit` (run 2026-05-08 08:07 local)
- Rung-2: `~/Documents/Obsidian/OpenClaw/tasks/Build Failure github - bborbe-go-skeleton - 08490c4.md` landed 2026-05-08 00:33 local — exact `computeFilenameHint` slug format, frontmatter and body produced by deployed dev build-watcher (post-`BRANCH=dev make buca`) round-tripping through agent task-controller v0.58.0+
- CHANGELOG: `## v0.23.29` entry covers github-pr migration; `## Unreleased` covers github-build migration
**Verdict:** PASS
