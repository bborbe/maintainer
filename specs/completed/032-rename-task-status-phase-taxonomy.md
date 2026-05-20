---
status: completed
approved: "2026-05-20T16:37:06Z"
generating: "2026-05-20T16:40:38Z"
prompted: "2026-05-20T16:51:56Z"
verifying: "2026-05-20T17:56:21Z"
completed: "2026-05-20T18:13:34Z"
branch: dark-factory/rename-task-status-phase-taxonomy
---

## Summary

- Align maintainer repo with vault-cli's renamed task taxonomy: status canonical `next` (was `todo`), phase canonical `execution` (was `in_progress`).
- Bump vault-cli dependency in every maintainer service module that imports `pkg/domain`.
- Flip the build-failure task-status default in `watcher/github-build` from `"todo"` to `"next"` so newly published tasks emit the new canonical.
- Flip the Phase-flag default in `agent/pr-reviewer` (and any sibling main packages) from `"in_progress"` to `"execution"`.
- Existing tasks/goals with old values keep working because vault-cli's normalize functions accept both forms as aliases.

## Problem

vault-cli flipped the canonical status to `next` and phase to `execution`. Maintainer is a multi-module Go repo whose watchers and agents publish task files into the Obsidian vault using literal default strings — `default:"todo"` for status, `default:"in_progress"` for phase. Those literals bypass vault-cli's type system, so every freshly published build-failure task or PR-review task keeps emitting legacy values forever unless we flip the defaults explicitly.

## Goal

After this work, the maintainer repo:

1. Compiles against the vault-cli version that exposes `TaskStatusNext` and `TaskPhaseExecution`.
2. Publishes new tasks into the vault with `status: next` and `phase: execution` in their frontmatter.
3. Continues to load and operate on existing vault tasks that still carry legacy `todo` / `in_progress` values — the agents read via normalize and see the canonical form.

## Non-goals

- Do NOT migrate existing vault frontmatter files in `~/Documents/Obsidian/Personal/` or `~/Documents/Obsidian/Trading/`.
- Do NOT touch GitHub PR or build-status fields — those are GitHub-domain states, not vault task status.
- Do NOT rename `ai_review` / `human_review` phase values.
- Do NOT remove the legacy aliases from vault-cli — both old and new must keep validating.

## Desired Behavior

1. Every `go.mod` under `watcher/`, `agent/`, `lib/` that lists `github.com/bborbe/vault-cli` is bumped to a published version whose `pkg/domain.AvailableTaskStatuses` contains `next` and `pkg/domain.AvailableTaskPhases` contains `execution`.
2. `watcher/github-build/main.go` declares `BuildTaskStatus` with `default:"next"` and an updated usage string.
3. `agent/pr-reviewer/main.go`, `agent/pr-reviewer/cmd/cli/main.go`, and `agent/pr-reviewer/cmd/run-task/main.go` (every file in that service tree declaring a `domain.TaskPhase` flag) declare the Phase flag with `default:"execution"` and an updated usage string mentioning `planning | execution | ai_review`.
4. Any other `*/main.go` in maintainer declaring a `domain.TaskPhase` or `domain.TaskStatus` flag emits new canonical defaults.
5. Test files asserting literal `"todo"` / `"in_progress"` for task status/phase use the new canonical, except for at least one explicit alias-round-trip test per dimension.
6. `make precommit` exits 0 in every service whose `go.mod` was bumped.

## Constraints

- Existing task files written with `status: todo` / `phase: in_progress` MUST continue to load — never reject those values.
- `errors.Errorf` / `errors.Wrapf` from `github.com/bborbe/errors` for all new error wrapping.
- Counterfeiter mocks regenerate cleanly via `go generate ./...` after the dep bump.
- GitHub-domain types (`watcher/github-pr` PR review state, `watcher/github-build` build status) are unrelated to vault task status — leave them untouched.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| vault-cli version with new constants not yet published | `go get github.com/bborbe/vault-cli@vX.Y.Z` fails with "version not found" | Wait for vault-cli release; re-run |
| Counterfeiter regeneration drifts after dep bump | `make precommit` fails in changed service with mock-mismatch diff | Run `go generate ./...`; recommit |
| Default-flip breaks a downstream consumer that pinned the literal `"todo"` value | New build-failure task carries `status: next`; downstream parser that grepped for `status: todo` ignores it | Downstream consumer must read via normalize or accept both — that's the new contract |
| Existing in-flight task with `status: todo` and new code reads it | Normalize converges to canonical `next`; agent processes the task normally | None — by design |
| Old binary still running publishes `status: todo` while new binary already deployed reads it | New binary normalizes the legacy value on read; processing continues | None — explicit cross-version compatibility via aliases |
| Concurrent dep bumps across modules land different vault-cli versions | `go list -m github.com/bborbe/vault-cli` returns different versions per module; runtime mostly fine because both versions accept aliases | Pin all modules to the same vault-cli version in one commit |

## Do-Nothing Option

Acceptable short-term. Vault-cli normalize already accepts both old and new values, so maintainer continues to function with legacy defaults. Cost: every new build-failure task or PR-review task keeps emitting old canonical, slowly multiplying grep noise across the vault. Fix is small but never gets cheaper, and the vault-side audits will start flagging maintainer-published tasks as inconsistent.

## Acceptance Criteria

- [ ] `grep -rn 'github.com/bborbe/vault-cli' watcher/ agent/ lib/ --include='go.mod'` lists every maintainer module on the chosen vault-cli version; `go list -m github.com/bborbe/vault-cli` in each module prints that version exactly once. The chosen version is the smallest published vault-cli release that contains `domain.TaskStatusNext` and `domain.TaskPhaseExecution` — agent decides the exact tag at impl time, verified by `go doc github.com/bborbe/vault-cli/pkg/domain TaskStatusNext` succeeding.
- [ ] `grep -n 'default:"todo"' watcher/github-build/main.go` returns 0 lines; `grep -n 'default:"next"' watcher/github-build/main.go` returns ≥1 line.
- [ ] `grep -n 'default:"in_progress"' agent/pr-reviewer/main.go agent/pr-reviewer/cmd/cli/main.go agent/pr-reviewer/cmd/run-task/main.go` returns 0 lines; `grep -n 'default:"execution"' agent/pr-reviewer/main.go agent/pr-reviewer/cmd/cli/main.go agent/pr-reviewer/cmd/run-task/main.go` returns ≥1 line in each file that declares a Phase flag.
- [ ] `grep -rn 'default:"todo"\|default:"in_progress"' watcher/ agent/ --include='*.go' --exclude-dir=vendor` returns 0 lines for `TaskStatus`/`TaskPhase`-typed flags (visual confirmation that no surviving hit is a vault-task-domain default — GitHub-domain build/PR status hits are out of scope).
- [ ] `grep -rn 'planning | in_progress | ai_review' agent/ --include='*.go' --exclude-dir=vendor` returns 0 lines; equivalent usage strings reference `planning | execution | ai_review`.
- [ ] Running `make precommit` in `lib`, `watcher/github-build`, `watcher/github-pr`, `agent/pr-reviewer` exits 0.
- [ ] At least one test invokes `domain.NormalizeTaskStatus("todo")` and asserts the return value equals `domain.TaskStatusNext`; at least one test invokes `domain.NormalizeTaskPhase("in_progress")` and asserts the return value equals `domain.TaskPhaseExecution`. Verify with `grep -rn 'NormalizeTaskStatus("todo")\|NormalizeTaskPhase("in_progress")' --include='*_test.go' --exclude-dir=vendor` returning ≥2 lines (one per dimension) with the expected constant asserted within 4 lines below each match.

## Verification

```bash
# Each module verified in its own subshell — independent of prior step success
(cd lib                    && make precommit)
(cd watcher/github-build   && make precommit)
(cd watcher/github-pr      && make precommit)
(cd agent/pr-reviewer      && make precommit)
```

Sanity-check the binaries emit new canonical:

```bash
# In watcher/github-build
go run ./ --help 2>&1 | grep 'build-task-status'
# Expected: usage line mentions "next" (or shows default "next")

# In agent/pr-reviewer
go run ./ --help 2>&1 | grep -A1 'Agent phase'
# Expected: "planning | execution | ai_review" in usage
```

## Verification Result

**Verified:** 2026-05-20T18:13:10Z (HEAD 25c0a37)
**Binary:** installed `dark-factory` (maintainer is not the dark-factory repo)
**Scenario:** No scenario file; ACs are grep/build-time evidence captured against worktree at HEAD 25c0a37.
**Evidence:**
- All 3 modules pin `github.com/bborbe/vault-cli v0.64.3`; `go doc TaskStatusNext` + `TaskPhaseExecution` both succeed
- `watcher/github-build/main.go:57` carries `default:"next"`; 0 hits for `default:"todo"`
- `agent/pr-reviewer/main.go:69` and `agent/pr-reviewer/cmd/run-task/main.go:58` carry `default:"execution"` and usage `planning | execution | ai_review`; 0 hits for `in_progress` defaults or legacy usage strings
- `grep -rn 'default:"todo"\|default:"in_progress"' watcher/ agent/` returns 0 lines
- `make precommit` exits 0 in `lib`, `watcher/github-build`, `watcher/github-pr`, `agent/pr-reviewer`
- `agent/pr-reviewer/domain_normalize_test.go:16-18,24-26` asserts both alias roundtrips against `TaskStatusNext` / `TaskPhaseExecution`
**Verdict:** PASS
