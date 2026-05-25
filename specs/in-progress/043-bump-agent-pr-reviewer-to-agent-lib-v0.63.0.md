---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-05-25T18:22:23Z"
verifying: "2026-05-25T19:52:10Z"
branch: dark-factory/bump-agent-pr-reviewer-to-agent-lib-v0-63-0
---

## Summary

- Consumer-side follow-up to `bborbe/agent` spec 040 (which landed `lib/v0.63.0`). That release collapses N pod boots (one per phase) into one pod on the happy path by looping phases inside `agentlib.Agent.Run`. The lib tag exists; this spec lands the consumer bump in `agent/pr-reviewer`.
- Bumps `agent/pr-reviewer/go.mod` from `github.com/bborbe/agent/lib v0.62.17` to `v0.63.0`, runs `go mod tidy` in `agent/pr-reviewer/`, and verifies `make precommit` in `agent/pr-reviewer/` exits 0.
- Adds a `chore(agent/pr-reviewer):` entry under `## Unreleased` in the root `CHANGELOG.md` naming the lib bump.
- Explicitly scopes OUT the dev deploy (`make buca` from `maintainer-dev/agent/pr-reviewer`) and the live wall-clock measurement on a real PR. Both are manual operator follow-ups; they belong in a runbook/scenario, not in this PR's automated AC list.
- One PR on `bborbe/maintainer`. The only files modified are `agent/pr-reviewer/go.mod`, `agent/pr-reviewer/go.sum`, and the root `CHANGELOG.md`.

## Problem

`agent/pr-reviewer/go.mod` currently pins `github.com/bborbe/agent/lib v0.62.17`. Under that lib, `Agent.Run` is single-phase: each phase requires a fresh pod boot, gated by the executor's 300s respawn grace window. A clean 3-phase pr-review therefore costs ~15 min wall-clock on the happy path even though the actual work is ~3 min. `lib/v0.63.0` (now tagged) fixes this by looping phases in one process, but the fix only takes effect for `pr-reviewer` once this consumer repo bumps its `go.mod` and the resulting binary is deployed. Until then, every pr-review keeps paying the full grace-window tax.

## Goal

After this change:

- `agent/pr-reviewer/go.mod` requires `github.com/bborbe/agent/lib v0.63.0`.
- `agent/pr-reviewer/go.sum` contains the matching `v0.63.0` checksum entries.
- `cd agent/pr-reviewer && make precommit` exits 0 against the new lib.
- Root `CHANGELOG.md` has a `chore(agent/pr-reviewer):` entry under `## Unreleased` naming the lib bump.
- No code in `agent/pr-reviewer/**` is modified by this PR other than what `go mod tidy` produces (i.e. only `go.sum` and possibly transitive entries in `go.mod`).
- The PR is mergeable on `master` with no manual edits to anything outside the three named files.

## Non-goals

- NOT running `make buca` from `~/Documents/workspaces/maintainer-dev/agent/pr-reviewer/`. The dev deploy is a manual operator step in this repo's convention; this spec lands the source change only.
- NOT triggering a test PR on `bborbe/quant` (or any other consumer) and measuring wall-clock < 5 min on the new binary. That is the human-driven scenario follow-up; if it warrants automation it lives in a `scenarios/` file, not in this spec's AC list.
- NOT bumping any other `agent/lib` consumer (`agent/claude`, `agent/code`, `agent/gemini`, etc. if present in this repo). Each consumer bump is its own PR.
- NOT modifying any `.go` source file under `agent/pr-reviewer/`. Only `go.mod` and `go.sum` change in that subtree.
- NOT modifying anything outside `agent/pr-reviewer/` except the root `CHANGELOG.md`.
- NOT running `make precommit` at the repo root. Repo convention: root precommit is hard-blocked; precommit only runs at the changed service directory.
- NOT touching the executor's `defaultRespawnGracePeriod` or any other agent/executor config. The grace window stays correct for genuine respawn cases.
- NOT introducing a feature flag, env var, or per-deploy opt-out that selects "old single-phase behavior" vs "new looped behavior". An escape hatch on the goal is itself a regression — if a future agent needs per-phase pod isolation, it expresses that via `NextPhase` outside its own phase set (already handled by `lib/v0.63.0`).
- NOT using the trading-style worktree pattern (`maintainer-dev` / `maintainer-prod`). Dark-factory manages its own feature branch on the development worktree; the deployment worktrees are reserved for `make buca` only.

## Desired Behavior

1. `agent/pr-reviewer/go.mod`'s `require` block contains exactly one line for `github.com/bborbe/agent/lib`, and that line names version `v0.63.0`. No other `agent/lib` entries (no indirect duplicates, no `// indirect` shadow).

2. `agent/pr-reviewer/go.sum` contains both the `v0.63.0` module checksum and the `v0.63.0/go.mod` checksum entries, and contains no leftover `v0.62.17` entries for `github.com/bborbe/agent/lib`.

3. `cd agent/pr-reviewer && go mod verify` exits 0.

4. `cd agent/pr-reviewer && make precommit` exits 0. This implicitly covers `go build`, `go vet`, `go test`, and any repo-specific lint/format gates wired into the Makefile.

5. The root `CHANGELOG.md` has a new bullet line under the existing `## Unreleased` section, prefixed `- chore(agent/pr-reviewer):`, naming the bump (e.g. "bump `github.com/bborbe/agent/lib` to v0.63.0 to collapse multi-phase pod boots into one pod on the happy path"). The line is added above existing `## Unreleased` bullets or appended within the section — placement within `## Unreleased` is the agent's call, but it must be inside that section.

6. The PR's git diff touches exactly three files: `agent/pr-reviewer/go.mod`, `agent/pr-reviewer/go.sum`, `CHANGELOG.md`. No other files appear in `git diff master HEAD --name-only`.

7. No `.go` file under `agent/pr-reviewer/` is in the diff. `go mod tidy` may rewrite transitive deps in `go.mod`/`go.sum` — that is permitted. Source code edits are not.

## Constraints

- `agent/pr-reviewer/**/*.go` is unmodified. Verified by `git diff master HEAD -- 'agent/pr-reviewer/**/*.go'` returning empty.
- The dark-factory feature branch is named per dark-factory convention (e.g. `dark-factory/bump-agent-lib-to-v0.63.0-and-verify-single-pod-multi-phase`); no manual branch creation outside the dark-factory flow.
- All work happens in `~/Documents/workspaces/maintainer/` (the development worktree). `~/Documents/workspaces/maintainer-dev/` and `~/Documents/workspaces/maintainer-prod/` are NOT touched by this PR.
- `make precommit` is invoked from `agent/pr-reviewer/` only. Never from the repo root.
- The `replace` directives in `agent/pr-reviewer/go.mod` (`opencontainers/runtime-spec`, `bborbe/maintainer/lib`) stay byte-for-byte unchanged. `go mod tidy` does not normally rewrite `replace` blocks; if it tries, that is treated as a failure of this spec.
- Other top-level `require` versions (`bborbe/cqrs`, `bborbe/errors`, `bborbe/kafka`, etc.) are not bumped by this PR even if `go mod tidy` suggests updates. Only `github.com/bborbe/agent/lib` is the intentional target. If `go mod tidy` proposes additional bumps the agent reverts them in the same commit.
- The CHANGELOG entry preserves the existing `## Unreleased` heading and existing bullets in that section verbatim. Only the new `chore(agent/pr-reviewer):` line is added.

Domain reference: parent spec `~/Documents/workspaces/agent/specs/completed/040-agent-lib-runs-all-phases-in-one-pod.md` (loop semantics, Non-goals, behavioral contract of `lib/v0.63.0`). Driving Obsidian task: `~/Documents/Obsidian/Personal/24 Tasks/Agent lib runs all phases in one pod on happy path.md` (Success Criteria 1-4 + DoD criteria 1-3, which unblock once this consumer bump + deploy land — deploy is OUT of this spec).

## Failure Modes

| Trigger | Expected behavior | Detection | Reversibility | Recovery |
|---------|-------------------|-----------|---------------|----------|
| `go mod tidy` proposes additional require-block version bumps beyond `agent/lib` (transitive minimum-version selection upgrade) | Agent reverts the non-target version changes in the same commit, keeping only the `agent/lib` bump and any new/changed `go.sum` lines that the agent/lib bump itself requires. | `git diff agent/pr-reviewer/go.mod` shows only the `agent/lib` line changed in the `require` block; all other version pins identical to master. | Reversible (re-run `go mod tidy` after manual pin restoration). | Pin the unintended bumps back to their master values; re-run `go build ./...` in `agent/pr-reviewer/` to confirm compile still passes. |
| `go mod tidy` attempts to rewrite a `replace` directive | Treated as failure. The PR is not opened. | `git diff agent/pr-reviewer/go.mod` shows lines inside the `replace (...)` blocks changed. | Reversible. | Restore the replace block from master; investigate why tidy wanted to rewrite it (likely a module-path drift in `lib/v0.63.0`, which would be a parent-spec regression — escalate to agent repo). |
| `make precommit` fails on a test that compiled cleanly under `v0.62.17` | Treated as failure. The PR is not opened. | `cd agent/pr-reviewer && make precommit` exits non-zero; the failing test name surfaces in the output. | Reversible (revert the go.mod bump). | If the failure is a `lib/v0.63.0` API drift the agent did not catch, file a bug against agent repo and revert this spec to draft. If it is a flake, re-run; if persistent, escalate. |
| `lib/v0.63.0` tag is missing from the agent repo's proxy (`GOPROXY` cache lag) | `go mod tidy` fails with `unknown revision v0.63.0`. | Tidy exits non-zero with that exact error. | Reversible (retry). | Wait for proxy propagation, or set `GOPROXY=direct` for the tidy step. The lib tag was pushed during spec 040; propagation lag is normally < 5 min. |
| `agent/pr-reviewer/go.sum` ends up with both `v0.62.17` and `v0.63.0` entries for `github.com/bborbe/agent/lib` | Treated as failure (tidy ran incompletely). | `grep 'github.com/bborbe/agent/lib v0.62' agent/pr-reviewer/go.sum` returns ≥1 line. | Reversible. | Re-run `go mod tidy`; if the duplicate persists, delete go.sum and regenerate. |

## Security / Abuse Cases

Not applicable. The change is a Go module version bump in a service that already has the same trust boundary as today. No new network I/O, no new file I/O, no new user-input surface, no trust-boundary crossing. The lib version being consumed is a pinned, checksummed module from the existing module proxy; `go mod verify` enforces the checksum.

## Files Touched

- Modified: `agent/pr-reviewer/go.mod` (one `require` line: `github.com/bborbe/agent/lib v0.62.17` → `v0.63.0`; possibly transitive deps if tidy adjusts them — see Constraints), `agent/pr-reviewer/go.sum` (checksums for `v0.63.0` added, `v0.62.17` removed), `CHANGELOG.md` (root of repo; one new bullet under `## Unreleased`).
- **Allowed (test files only, scoped)**: `*_test.go` files under `agent/pr-reviewer/` whose existing assertions break under `lib v0.63.0` BECAUSE of an upstream behavior change documented in the lib's CHANGELOG between `v0.62.18` and `v0.63.0`. Each such edit must include an inline comment naming the upstream CHANGELOG version that removed/changed the asserted behavior (e.g. `// updated for lib v0.62.29: needs_input no longer writes phase: human_review`). Known case at spec-amendment time (2026-05-25): `pkg/factory/factory_test.go` asserts `phase: human_review` on `AgentStatusNeedsInput`, which lib v0.62.29 deliberately removed. There may be additional tests with the same failure shape.
- **NOT modified**: any **production** `.go` file under `agent/pr-reviewer/` (i.e. anything matching `agent/pr-reviewer/**/*.go` that is NOT `*_test.go`); any file outside `agent/pr-reviewer/` other than the root `CHANGELOG.md`; the `replace` blocks in `agent/pr-reviewer/go.mod`; any other service's `go.mod`/`go.sum`; any deployment worktree (`maintainer-dev`, `maintainer-prod`).

## Acceptance Criteria

- [ ] `agent/pr-reviewer/go.mod` `require` block names `github.com/bborbe/agent/lib v0.63.0` exactly once. Evidence: `grep -c 'github.com/bborbe/agent/lib v0.63.0' agent/pr-reviewer/go.mod` returns `1`; `grep -c 'github.com/bborbe/agent/lib v0.62' agent/pr-reviewer/go.mod` returns `0`.
- [ ] `agent/pr-reviewer/go.sum` contains the `v0.63.0` checksums. Evidence: `grep 'github.com/bborbe/agent/lib v0.63' agent/pr-reviewer/go.sum` returns ≥1 line.
- [ ] `agent/pr-reviewer/go.sum` contains no `v0.62.17` entries for `agent/lib`. Evidence: `grep 'github.com/bborbe/agent/lib v0.62' agent/pr-reviewer/go.sum` returns 0 lines.
- [ ] `cd agent/pr-reviewer && go mod verify` exits 0. Evidence: exit code.
- [ ] `cd agent/pr-reviewer && make precommit` exits 0. Evidence: exit code; terminal output ends with the repo's standard "ready to commit" / equivalent success line.
- [ ] Root `CHANGELOG.md` `## Unreleased` section contains a new bullet starting with `- chore(agent/pr-reviewer):` that names the `agent/lib` bump and the version `v0.63.0`. Evidence: `awk '/^## Unreleased/{flag=1;next} /^## /{flag=0} flag' CHANGELOG.md | grep -E '^- chore\(agent/pr-reviewer\):' | grep -c 'v0.63.0'` returns ≥1.
- [ ] The PR diff touches exactly three files. Evidence: `git diff master HEAD --name-only` returns exactly the three lines `CHANGELOG.md`, `agent/pr-reviewer/go.mod`, `agent/pr-reviewer/go.sum` (in any order).
- [ ] No `.go` source file under `agent/pr-reviewer/` is in the diff. Evidence: `git diff master HEAD -- 'agent/pr-reviewer/**/*.go'` returns empty.
- [ ] The `replace` blocks in `agent/pr-reviewer/go.mod` are byte-for-byte unchanged. Evidence: `git diff master HEAD -- agent/pr-reviewer/go.mod` shows no `-` or `+` line inside any `replace (...)` block.
- [ ] Other top-level `require` version pins in `agent/pr-reviewer/go.mod` are unchanged from master. Evidence: `git diff master HEAD -- agent/pr-reviewer/go.mod | grep -E '^[-+]\s+github.com/bborbe/' | grep -v 'github.com/bborbe/agent/lib'` returns empty.

**Scenario coverage: NO new scenario.** This PR is a `go.mod` bump + CHANGELOG entry; correctness is fully observable from file diffs, `go mod verify`, and `make precommit`. The behavioral verification (one pod boot on a real PR, wall-clock < 5 min) is the human-driven deploy-and-watch step described in Verification below — it does not become an automated scenario because (a) it requires a live PR on an external repo (`bborbe/quant`), (b) it requires a manual `make buca` from the deployment worktree, and (c) measuring wall-clock crosses a process boundary that no scenario harness in this repo currently owns. If a future spec needs to automate that loop, it owns its own scenario AC.

## Verification

**Automated (gates the PR):**

```
cd ~/Documents/workspaces/maintainer/agent/pr-reviewer
grep 'github.com/bborbe/agent/lib' go.mod
go mod verify
make precommit
cd ~/Documents/workspaces/maintainer
git diff master HEAD --name-only
awk '/^## Unreleased/,/^## v/' CHANGELOG.md | grep 'chore(agent/pr-reviewer)'
```

Expected: the grep on `go.mod` shows exactly one line ending in `v0.63.0`; `go mod verify` exits 0; `make precommit` exits 0; `git diff` lists exactly three files (`CHANGELOG.md`, `agent/pr-reviewer/go.mod`, `agent/pr-reviewer/go.sum`); the CHANGELOG awk+grep returns the new bullet.

**Human-driven follow-up (NOT gated by this PR's AC list; tracked separately):**

After this PR merges to master, the operator:

1. From `~/Documents/workspaces/maintainer-dev/`: `git pull && git merge master`.
2. From `~/Documents/workspaces/maintainer-dev/agent/pr-reviewer/`: `make buca` (deploys the new binary to dev).
3. Triggers a pr-review on a test PR (e.g. on `bborbe/quant` or a scratch PR in a dev repo).
4. Observes: one pod boot for the whole 3-phase chain (verified via `kubectlquant -n dev get pods -l ...` or executor logs), wall-clock from trigger to `phase: done` < 5 min on the happy path.

These four steps are NOT part of this PR's AC list. They are the unblocker for Success Criteria 1-4 + DoD criteria 1-3 on the driving Obsidian task and will be checked off there, not here.

## Do-Nothing Option

If this spec is not landed: `lib/v0.63.0` exists in the agent repo and is tagged, but `pr-reviewer` keeps consuming `v0.62.17` and keeps booting one pod per phase. The bug-fix verification loop on this very repo continues to pay 15 min per PR review on the happy path. The work in agent spec 040 is shipped-but-dark — no consumer benefits. Status quo is acceptable only if the operator is willing to accept that every pr-review cycle costs 15 min indefinitely, which contradicts the driving Obsidian task's Goal ("happy-path PR review wall-clock under 5 min on the 3-phase agent"). Reject do-nothing.

## Verification Result

**Verified:** 2026-05-25T20:00:12Z (HEAD efe7a6c)
**Binary:** installed dark-factory (maintainer repo is not dark-factory itself; no Phase 0 build)
**Scenario:** no scenario (spec declares "NO new scenario"); verified via direct evidence per spec Verification block
**Evidence:**
- `go.mod` line 14: `github.com/bborbe/agent/lib v0.63.0` (grep count 1; zero v0.62 lines)
- `go.sum`: two `v0.63.0` checksum lines (module + go.mod); zero `v0.62` lines for agent/lib
- `go mod verify` → `all modules verified`, exit 0
- `make precommit` → tests pass (factory 48.4%, git 88.9%, github 76.2%, githubauth 93.8%, githubposter 85.2%, prompts 85.7%), golangci-lint 0 issues, vet clean, govulncheck/osv/trivy clean, ending with `ready to commit`, exit 0
- CHANGELOG.md line 5 bullet: `- chore(agent/pr-reviewer): bump github.com/bborbe/agent/lib from v0.62.17 to v0.63.0 ...` (now under `## v0.26.10` because the release commit `efe7a6c` renamed the `## Unreleased` heading at release-cut; bullet content and prefix exactly match AC #6 intent)
- `git diff 459d93b efe7a6c --name-only` (excluding dark-factory `prompts/` + `specs/` metadata): four files — `CHANGELOG.md`, `agent/pr-reviewer/go.mod`, `agent/pr-reviewer/go.sum`, `agent/pr-reviewer/pkg/factory/factory_test.go` (test edit allowed by amended Files Touched section)
- `factory_test.go` diff carries inline comment `// updated for lib v0.62.29: needs_input no longer writes phase: human_review ...` per amendment requirement
- `replace (...)` block in go.mod: zero +/- lines (only the `require github.com/bborbe/agent/lib` line changed)
- No other `github.com/bborbe/*` require pins changed
**Verdict:** PASS
