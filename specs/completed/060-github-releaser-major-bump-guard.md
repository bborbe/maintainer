---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-06-03T12:54:47Z"
generating: "2026-06-03T13:05:52Z"
prompted: "2026-06-03T13:11:32Z"
verifying: "2026-06-03T17:37:03Z"
completed: "2026-06-03T17:37:05Z"
branch: dark-factory/github-releaser-major-bump-guard
---

## Summary

- Add a guard that blocks the github-releaser-agent from auto-releasing a **major** semver bump unless the repo has explicitly opted in. Default: blocked.
- New per-repo opt-in: `release.allowMajorBump` in `.maintainer.yaml` (default `false`).
- New per-run override: CLI flag `--allow-major` (env `ALLOW_MAJOR`) on the github-releaser-agent binary, so an operator can re-fire a stuck task without editing the target repo's config.
- Guard fires in the **planning phase** after Claude classifies the bump. Trip condition: `bump=major` AND `allowMajorBump=false` AND `--allow-major` not set. Trip behavior: `Status: NeedsInput`, no silent downgrade to minor, escalation reason cites the offending bullet(s).
- README documents the new field and the new flag. Reference incident in `## Problem`: a `refactor:` bullet that was a breaking lib rename was classified by the prefix-based slash-command as patch — the guard exists so that any future `major` verdict (from any classifier source) requires an explicit human ack before tag + push.

## Problem

The github-releaser-agent currently treats a `major` verdict from the bump classifier the same as `minor` or `patch`: it advances to execution, rewrites the CHANGELOG header to `## vN+1.0.0`, commits, tags, and pushes. There is no human in the loop for a breaking-change release.

The classifier itself is prefix-based (`feat:` → minor, `BREAKING CHANGE` → major, everything else → patch). It can be tricked: a `refactor:` bullet that is semantically a breaking library rename (e.g. `refactor(lib): rename TaskTypeClaude → TaskTypeLLM`) reads as patch under the rules, yet downstream consumers see an incompatible API. Fixing the classifier itself is a moving target — the rules cannot catch every false-negative. What we CAN do cheaply: require an explicit per-repo opt-in for **any** classifier output of `major`. If the classifier is conservative and emits `major` from a hidden BREAKING bullet, the operator confirms once and the release ships. If the classifier is liberal and emits `major` from a `feat:` typo, the operator catches it before tag + push.

Without this guard the cost of a single wrong major release is high: downstream consumers pull a tag that purports to be backwards-compatible (under their semver discipline) and break in surprising ways. The fix-forward is publishing another tag, often with a manual revert — far more expensive than a one-time human ack.

The `--allow-major` CLI flag is the transient ops escape hatch alongside the durable per-repo YAML policy. When the YAML opt-in is committed but a Job is already in flight (cached classification, mid-run, etc.), edit-commit-push round-trip is too slow; the CLI flag lets the operator re-fire the binary with the override without a target-repo commit. The YAML field is the durable per-repo policy; the flag is the transient ops escape hatch.

## Goal

After this spec ships, end state:

1. The github-releaser-agent **never** writes a `major` release tag without an explicit opt-in from one of two sources: (a) the target repo's `.maintainer.yaml` has `release.allowMajorBump: true`, or (b) the agent was invoked with `--allow-major` / `ALLOW_MAJOR=true`.
2. When the guard trips (bump=major, neither opt-in present), the planning phase returns `Status: NeedsInput`, writes a `## Plan` block with `outcome: needs_input` AND `precondition_failed: major_bump_not_allowed`, AND the escalation reason names the bump verdict, the reasoning string from the classifier, and the bullet(s) the classifier cited.
3. `minor` and `patch` verdicts proceed to execution exactly as today — guard is a no-op on those.
4. A `major` verdict with **either** opt-in source proceeds to execution exactly as today (no behavior change for opted-in repos).
5. The maintainer README documents the new `release.allowMajorBump` field and the new `--allow-major` flag with a one-paragraph rationale and a one-line YAML example.
6. The reference scenario — a fixture task where the CHANGELOG's `## Unreleased` contains a single `refactor(lib): rename TaskTypeClaude → TaskTypeLLM` bullet, the classifier returns `major` (forced via mocked Claude runner), the repo's `.maintainer.yaml` has `release.allowMajorBump: false` (or is absent), and `--allow-major` is unset — produces a `NeedsInput` result with the documented escalation shape.

## Non-goals

- Do NOT change the bump classifier or its prompt. The classifier stays prefix-based. The guard catches the false-negative externally by requiring a human ack on every `major` verdict.
- Do NOT add a `release.allowMinorBump` or `release.allowPatchBump` knob. The guard is specific to `major`; `minor`/`patch` proceed unchanged. If a future consumer demands per-bump gating, that is a separate spec.
- Do NOT add an "auto-downgrade major to minor" fallback. Silent downgrade is the explicit anti-feature the guard exists to prevent.
- Do NOT audit or update `.maintainer.yaml` in agent / dark-factory / trading / other repos. Per-repo opt-in audit is a follow-up task (the spec only ships the mechanism, not the rollout).
- Do NOT change the execution-phase or ai_review-phase behavior. The guard fires before `NextPhase=execution` is returned.
- Do NOT add a "force-major-via-task-frontmatter" path. The override must come from the operator running the binary (CLI flag) OR the repo owner committing the YAML field — not from the task content itself, which is watcher-generated and not human-authored per task.

## Desired Behavior

1. The `.maintainer.yaml` schema gains a new field `release.allowMajorBump` (bool, default `false`). When the field is absent, the parsed config exposes `allowMajorBump = false`. When present and set to `true`, the parsed config exposes `allowMajorBump = true`. Unknown values (non-bool YAML) cause the strict-parse path to return a wrapped error containing the literal `release.allowMajorBump`.
2. The github-releaser-agent binary gains a new CLI flag `--allow-major` (env `ALLOW_MAJOR`, default `false`). When set to true via either source, the planning phase treats it as a per-run opt-in equivalent to the repo's `allowMajorBump: true`.
3. The planning phase, after parsing the bump verdict but before advancing to execution, evaluates the guard. Decision table (the ONLY combinations — no fall-through):

   | classifier `bump` | repo `allowMajorBump` | CLI `--allow-major` | guard outcome |
   |---|---|---|---|
   | `patch` | any | any | proceed (no-op) |
   | `minor` | any | any | proceed (no-op) |
   | `major` | `true` | any | proceed (advance to execution) |
   | `major` | `false` or absent | `true` | proceed (advance to execution) |
   | `major` | `false` or absent | `false` or unset | **TRIP** — NeedsInput |

4. When the guard trips, the planning phase writes a `## Plan` section with: `outcome: needs_input`, `precondition_failed: "major_bump_not_allowed"`, `reason` containing the literal substring `major bump not allowed`, the classifier's reasoning string verbatim, the cited bullet(s) from `## Unreleased`, the resolved values of `allow_major_bump_config` (the repo YAML value) and `allow_major_bump_flag` (the CLI flag value) so the operator can see which lever to flip. `current_version` is populated; `next_version` / `next_version_header` / `bump` / `bullets` ARE populated (the operator needs to see what would have been released).
5. When the guard trips, the task frontmatter is mutated per the existing escalation contract: `assignee: ""`, `previous_assignee: github-releaser-agent`, `status` UNCHANGED, `phase` UNCHANGED (resume cursor at `planning`). Identical to the existing P1/P2/missing-frontmatter escalation paths — no new mutation rules.
6. When the guard trips, the agent Result is `Status: NeedsInput` (NOT `Done`, NOT `Failed`). The deliverer maps NeedsInput to "no auto-retry, no advance, operator re-delegates by re-setting assignee" — same wiring as existing escalations. The agent emits a `glog.V(2)` line containing the literal `major bump not allowed` so kubectl-logs greps surface trips.
7. README gains a section under the github-releaser docs that documents (a) the new YAML field with a one-line example, (b) the new CLI flag with the env-var name, (c) one-paragraph rationale pointing at the false-negative class of bug, (d) the two ways an operator re-delegates a tripped task (commit YAML opt-in + re-set assignee, OR re-fire the Job with `--allow-major`).
8. The fixture-driven scenario at the agent's existing unit + integration test layer covers four cases: `major + no opt-in` (trip), `major + repo opt-in` (proceed), `major + flag opt-in` (proceed), `minor + no opt-in` (proceed, guard no-op). The `refactor(lib): rename TaskTypeClaude → TaskTypeLLM` bullet is used verbatim as the input for at least the trip case so the regression is named in the test source.

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Add `AllowMajorBump` to ReleaseConfig + strict-parse tests | 1 | 3,4,5,6 + new strict-reject AC | — |
| 2 | Wire `--allow-major` flag through main → BuildEnv → planning step | 2 | 7,8,9 | 1 |
| 3 | Implement guard in planning step + PlanOutput fields + precondition constant | 3,4,5,6 | 10,11,12,13 | 2 |
| 4 | Four Ginkgo decision-table cases + regression-bullet fixture | 8 | 14,15,16,17,18,19 | 3 |
| 5 | README + CHANGELOG | 7 | 20,21 | 3 |

Rationale: schema first (no deps), then flag plumbing (depends on schema for `AllowMajorBump` symbol), then guard logic (depends on both lever sources being readable), then tests (need guard to assert against), README/CHANGELOG last so docs reflect the final names. Splitting test cases from guard logic avoids one mega-prompt that must hold both the implementation and the four-row table at once.

## Constraints

- Schema location: the existing `ReleaseConfig` struct in `lib/maintainerconfig/maintainerconfig.go` (shared with watcher + pr-reviewer agent). Field name `AllowMajorBump` with `yaml:"allowMajorBump"`. Strict parser (`ParseStrict`) is the consumer; lenient parser path must also accept the new field. Existing fields `AutoRelease` and `ChangelogRewrite` are unchanged.
- The CLI flag MUST follow the existing `libargument` struct-tag pattern in `agent/github-releaser/main.go` (`required:"false" arg:"allow-major" env:"ALLOW_MAJOR" usage:"..." default:"false"`). The flag value MUST propagate from `application` → `BuildEnv` → the planning step constructor via the existing factory wiring (no global / package-level state).
- The guard MUST fire in the planning phase, AFTER `prompts.ParseBumpVerdict` returns and AFTER `semver.BumpVersion` computes the next version (so the escalation block can name the would-be next_version), but BEFORE `publishPlan` returns `NextPhase: execution`. Execution and ai_review phases stay untouched.
- The escalation contract is FROZEN per spec 047: `assignee: ""`, `previous_assignee: github-releaser-agent`, `status` + `phase` unchanged, Result `Status: AgentStatusNeedsInput`. This spec adds a new trigger to that contract; it does NOT add a new contract.
- The new `precondition_failed` value is `major_bump_not_allowed`. This is a new string constant in the planning-step's precondition enum (alongside `P1_unreleased_not_first`, `P2_unreleased_empty`, `missing_frontmatter_<field>`, `bad_current_version`).
- The `## Plan` `PlanOutput` struct gains two new optional fields: `AllowMajorBumpConfig bool` (yaml-source value at planning time) and `AllowMajorBumpFlag bool` (CLI-source value at planning time), both with `omitempty`. These are populated on the trip case so the operator can see which lever to flip. Other fields are unchanged.
- Errors via `github.com/bborbe/errors` only — no `fmt.Errorf` in the planning step or maintainerconfig parse path. The trip is NOT an error (it's a NeedsInput); only YAML parse failures on the new field surface as errors.
- The fixture/test layer uses Ginkgo v2 + Gomega + counterfeiter mocks per existing pattern. The mocked `claudelib.ClaudeRunner` returns the JSON verdict verbatim — no real Claude call. Coverage on planning-step changes ≥ 75% (per spec 049's existing target; this spec inherits the threshold).
- README documentation lives in `README.md` at repo root, in a new sub-section under the github-releaser-agent docs (or extension of an existing sub-section). NOT a new file.
- The CHANGELOG `## Unreleased` block gains a single `feat:` bullet naming `release.allowMajorBump` and `--allow-major`. Verified by grep in AC walk.

## Failure Modes

| Trigger | Detection | Expected behavior | Reversibility | Recovery |
|---|---|---|---|---|
| Classifier returns `major`, neither opt-in present | Planning step's guard check after `ParseBumpVerdict` | Write `## Plan` with `outcome: needs_input`, `precondition_failed: major_bump_not_allowed`, populate `next_version` + `bullets` so operator sees the would-be release; clear assignee; set previous_assignee; return `Status: NeedsInput` | Reversible | Operator either (a) sets `release.allowMajorBump: true` in target repo's `.maintainer.yaml`, commits, pushes, re-sets `assignee: github-releaser-agent`, OR (b) re-fires the Job with `--allow-major=true` |
| Classifier returns `major`, repo opt-in true, flag unset | Guard reads parsed `Config.Release.AllowMajorBump` | Proceed to execution (no guard action) | n/a | n/a — happy path for opted-in repos |
| Classifier returns `major`, repo opt-in false, flag true | Guard reads CLI/env flag | Proceed to execution; emit `glog.V(2)` line containing literal `--allow-major override` so the override is auditable in kubectl logs | n/a | n/a — explicit operator override |
| `.maintainer.yaml` has `allowMajorBump: yes` (non-bool) | Strict parser rejects | Plan written with `outcome: failed`, `error_category: invalid_config`, `invalid_field: release.allowMajorBump` (existing fail-closed path from spec 059) | Reversible | Operator fixes YAML, re-delegates |
| `.maintainer.yaml` absent at the ref's tip | Existing fetcher returns `ErrFileNotFound` | Treated as `allowMajorBump: false` (default). If classifier returns `major` and flag unset → trip as the primary failure mode above | Reversible | Same recovery as primary trip |
| Classifier returns `major`, but transient Claude error caused re-fire | M2 bump cache reads prior `## Plan` and skips re-running the bump LLM; `glog.V(2) "major bump not allowed"` line appears on every cache-hit re-fire | Cached `bump=major` still re-evaluates the guard on every re-fire — a stale `## Plan` with `outcome=needs_input` does NOT itself bypass the guard. Cache only covers the LLM call; the guard runs again from cached verdict | Reversible | Same recovery as primary trip |
| CLI flag `--allow-major=true` set, classifier returns `patch` or `minor` | Guard no-op branch | Flag is silently inert (no harm, no log) | n/a | n/a |

## Security / Abuse Cases

- The `allowMajorBump` field is committed to the target repo by anyone with write access to that repo's master branch. A malicious / careless commit setting `allowMajorBump: true` to a repo the actor doesn't own is a repo-protection concern, not a maintainer concern — the maintainer agent treats the YAML as authoritative. README MUST note that flipping this field is equivalent to pre-approving any future major releases on that repo until reverted.
- The `--allow-major` flag is set at Job-spawn time by the operator running `kubectl create job` or via the agent task controller's CRD trigger. There is no per-task-frontmatter override. This is deliberate — task content is watcher-generated, and allowing the flag to be set from task content would let any commit on master bypass the guard.
- The trip-case `## Plan` block contains the bullet text from `## Unreleased` verbatim. The bullet content comes from a committed `CHANGELOG.md` (already trusted-source) so there is no new injection surface. The classifier's reasoning string is LLM-generated and is also placed verbatim into the escalation block — same as the existing happy-path `## Plan` block (no new exposure).
- The override-path `glog.V(2)` log line names the operator-set flag value but does NOT log secrets, tokens, or repo content beyond what already appears in planning-step logs.

## Acceptance Criteria

- [ ] `cd agent/github-releaser && make precommit` exits 0 — evidence: exit code 0
- [ ] `cd lib/maintainerconfig && go test ./...` exits 0 — evidence: exit code 0
- [ ] `grep -c 'AllowMajorBump' lib/maintainerconfig/maintainerconfig.go` returns ≥ 1 — evidence: grep count
- [ ] `grep -c 'yaml:"allowMajorBump"' lib/maintainerconfig/maintainerconfig.go` returns 1 — evidence: grep count
- [ ] A `DescribeTable` entry in `lib/maintainerconfig/maintainerconfig_test.go` named `release.allowMajorBump: true -> AllowMajorBump true` exists — evidence: `grep -c 'release.allowMajorBump: true -> AllowMajorBump true' lib/maintainerconfig/maintainerconfig_test.go` returns 1
- [ ] A `DescribeTable` entry named `release: present but no allowMajorBump field -> AllowMajorBump false (default)` exists — evidence: `grep -c 'present but no allowMajorBump field' lib/maintainerconfig/maintainerconfig_test.go` returns 1
- [ ] Strict parser rejects non-bool `allowMajorBump` value — evidence: `grep -c 'allowMajorBump: non-bool -> strict error' lib/maintainerconfig/maintainerconfig_test.go` returns 1
- [ ] `grep -c 'allow-major' agent/github-releaser/main.go` returns ≥ 1 (CLI flag wired into application struct) — evidence: grep count
- [ ] `grep -c 'ALLOW_MAJOR' agent/github-releaser/main.go` returns ≥ 1 (env var wired into application struct) — evidence: grep count
- [ ] `grep -ci 'allowMajor' agent/github-releaser/pkg/buildenv.go` returns ≥ 1 (flag propagates through BuildEnv to planning step; case-insensitive because BuildEnv is a function with parameter `allowMajor`, not a struct with field `AllowMajor`) — evidence: grep count
- [ ] `grep -c 'major_bump_not_allowed' agent/github-releaser/pkg/steps_planning.go` returns ≥ 1 — evidence: grep count
- [ ] `grep -c 'PreconditionMajorBumpNotAllowed' agent/github-releaser/pkg/steps_planning.go` returns ≥ 1 (typed constant exists, not a string-literal-only path) — evidence: grep count
- [ ] `grep -c 'AllowMajorBumpConfig\s\+bool' agent/github-releaser/pkg/plan_output.go` returns 1 (struct field declared) — evidence: grep count
- [ ] `grep -c 'AllowMajorBumpFlag\s\+bool' agent/github-releaser/pkg/plan_output.go` returns 1 (struct field declared) — evidence: grep count
- [ ] Ginkgo case `major bump trips guard when neither opt-in present` exists — evidence: `grep -c 'major bump trips guard when neither opt-in present' agent/github-releaser/pkg/steps_planning_test.go` returns 1. Test body asserts: returned `Result.Status == AgentStatusNeedsInput`, the mutated content's `## Plan` contains `outcome: needs_input` AND `precondition_failed: major_bump_not_allowed`, frontmatter has `assignee: ""` AND `previous_assignee: github-releaser-agent` AND `status: in_progress` AND `phase: planning`. Five separate Gomega expectations.
- [ ] Ginkgo case `major bump proceeds when repo opt-in true` exists — evidence: `grep -c 'major bump proceeds when repo opt-in true' agent/github-releaser/pkg/steps_planning_test.go` returns 1. Test body asserts: returned `Result.Status == AgentStatusDone` AND `NextPhase == "execution"`, `## Plan` `outcome: ready`.
- [ ] Ginkgo case `major bump proceeds when CLI flag set` exists — evidence: `grep -c 'major bump proceeds when CLI flag set' agent/github-releaser/pkg/steps_planning_test.go` returns 1. Test body asserts: `Result.Status == AgentStatusDone` AND `NextPhase == "execution"`.
- [ ] Ginkgo case `minor bump unaffected by guard` exists — evidence: `grep -c 'minor bump unaffected by guard' agent/github-releaser/pkg/steps_planning_test.go` returns 1. Test body asserts: `Result.Status == AgentStatusDone` AND `NextPhase == "execution"` for a fixture where neither opt-in is set.
- [ ] Reference regression bullet present in trip-case test fixture — evidence: `grep -c 'refactor(lib): rename TaskTypeClaude' agent/github-releaser/pkg/steps_planning_test.go` returns ≥ 1
- [ ] `go test -cover ./pkg/...` in `agent/github-releaser` reports coverage matching regex `coverage: ([7-9][0-9]|100)\.?[0-9]*%` on the planning-step package — evidence: stdout regex match
- [ ] README documents the new field and flag — evidence: `grep -c 'allowMajorBump' README.md` returns ≥ 1 AND `grep -c '\-\-allow-major' README.md` returns ≥ 1 AND `grep -c 'ALLOW_MAJOR' README.md` returns ≥ 1
- [ ] Root `CHANGELOG.md` `## Unreleased` section gains a single `feat:` bullet naming the new field and flag — evidence: `grep -c 'allowMajorBump' CHANGELOG.md` returns ≥ 1
- [ ] No `fmt.Errorf` introduced in the touched files — evidence: `git diff master -- agent/github-releaser/pkg/steps_planning.go lib/maintainerconfig/maintainerconfig.go agent/github-releaser/main.go agent/github-releaser/pkg/buildenv.go agent/github-releaser/pkg/plan_output.go | grep -c '^+.*fmt\.Errorf'` returns 0

**Scenario coverage**: NO new scenario. The four mock-Claude Ginkgo cases above are integration-layer tests that exercise the guard end-to-end through the planning step (factory wiring, frontmatter mutation, `## Plan` section marshal, NeedsInput result mapping). The guard's logic is a pure decision-table evaluation with no I/O of its own — the YAML fetch and Claude call are existing seams already covered by mocks. No E2E scenario reaches anything the unit + integration layer cannot.

## Verification

```bash
cd agent/github-releaser
make precommit                                                                   # exit 0
go test -cover ./pkg/...                                                         # ≥ 75% on planning step

cd ../../lib/maintainerconfig
go test ./...                                                                    # exit 0

# Schema
grep -c 'AllowMajorBump'                          lib/maintainerconfig/maintainerconfig.go  # ≥1
grep -c 'yaml:"allowMajorBump"'                   lib/maintainerconfig/maintainerconfig.go  # =1
grep -c 'release.allowMajorBump: true -> AllowMajorBump true'  lib/maintainerconfig/maintainerconfig_test.go  # =1
grep -c 'present but no allowMajorBump field'     lib/maintainerconfig/maintainerconfig_test.go  # =1
grep -c 'allowMajorBump: non-bool -> strict error' lib/maintainerconfig/maintainerconfig_test.go  # =1

# CLI flag + env propagation
grep -c 'allow-major'                             agent/github-releaser/main.go        # ≥1
grep -c 'ALLOW_MAJOR'                             agent/github-releaser/main.go        # ≥1
grep -ci 'allowMajor'                             agent/github-releaser/pkg/buildenv.go  # ≥1

# Guard inside planning step
grep -c 'major_bump_not_allowed'                  agent/github-releaser/pkg/steps_planning.go  # ≥1
grep -c 'PreconditionMajorBumpNotAllowed'         agent/github-releaser/pkg/steps_planning.go  # ≥1
grep -c 'AllowMajorBumpConfig\s\+bool'            agent/github-releaser/pkg/plan_output.go     # =1
grep -c 'AllowMajorBumpFlag\s\+bool'              agent/github-releaser/pkg/plan_output.go     # =1

# Four Ginkgo cases — guard decision table
grep -c 'major bump trips guard when neither opt-in present'   agent/github-releaser/pkg/steps_planning_test.go  # =1
grep -c 'major bump proceeds when repo opt-in true'            agent/github-releaser/pkg/steps_planning_test.go  # =1
grep -c 'major bump proceeds when CLI flag set'                agent/github-releaser/pkg/steps_planning_test.go  # =1
grep -c 'minor bump unaffected by guard'                       agent/github-releaser/pkg/steps_planning_test.go  # =1

# Reference regression named in test fixture
grep -c 'refactor(lib): rename TaskTypeClaude'    agent/github-releaser/pkg/steps_planning_test.go  # ≥1

# README documents new field + flag
grep -c 'allowMajorBump'                          README.md  # ≥1
grep -c '\-\-allow-major'                         README.md  # ≥1
grep -c 'ALLOW_MAJOR'                             README.md  # ≥1

# CHANGELOG entry
grep -c 'allowMajorBump'                          CHANGELOG.md  # ≥1
```

## Do-Nothing Option

Cost of NOT building this guard:

- The github-releaser-agent will silently tag a `major` release whenever the classifier emits `major` — including the (rare-but-real) cases where Claude reads a `BREAKING CHANGE:` body line in a `refactor:`-prefixed bullet and correctly classifies as major. Operator has no opt-out short of disabling `release.autoRelease` for the whole repo, which loses the auto-tag benefit for all minor/patch releases too.
- Conversely, when the classifier mis-classifies a breaking change as patch (the originating incident: `refactor(lib): rename TaskTypeClaude → TaskTypeLLM`), the agent tags `vN.Y.Z+1` and downstream consumers break under semver discipline. The guard does NOT fix this case directly — it can only catch the symmetric `major` false-positive — BUT shipping the guard establishes the per-repo opt-in scaffolding that a future stricter classifier (LLM-based, audited) could rely on without needing a fleet-wide migration.
- Without the per-repo opt-in field, the fleet rollout of a stricter classifier becomes all-or-nothing: every repo gets stricter rules at once, and the inevitable false-positives block every release until manually overridden. With the field shipped now (default false), a stricter classifier can be rolled out repo-by-repo by flipping `allowMajorBump: true` on opted-in repos that have human reviewers ready.
- A do-nothing alternative is "rely on PR review of the release commit" — but the agent commits direct-to-master via App bypass (no PR), so there is no review surface. The guard IS the review surface.

## Verification Result

**Verified:** 2026-06-03T17:15:00Z (HEAD d25e50d)
**Binary:** installed dark-factory v0.175.0
**Scenario:** Static-evidence verification (no scenario per spec); 23 ACs walked against merged implementation (PR #42, commit 7c69a78, released v0.33.0 by github-releaser-agent itself).
**Evidence:**
- AC1 `cd agent/github-releaser && make precommit` → "ready to commit" (exit 0)
- AC2 `cd lib/maintainerconfig && go test ./...` → ok (exit 0)
- AC3-22 all grep counts match spec thresholds (incl. AC10 case-insensitive `grep -ci 'allowMajor' .../buildenv.go` = 4)
- AC20 `go test -cover ./pkg/...` → planning-step pkg coverage 86.7% (matches `[7-9][0-9]%`)
- AC23 `git diff` introduced 0 new `fmt.Errorf` in touched files
- Shipped end-to-end: v0.33.0 released by github-releaser-agent on its own code (minor-classified bump, guard no-op path exercised in prod)
**Verdict:** PASS
