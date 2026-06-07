---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-06-06T21:42:28Z"
generating: "2026-06-06T21:42:46Z"
prompted: "2026-06-06T22:03:06Z"
branch: dark-factory/releaser-no-major-bump
---

## Summary

- The github-releaser agent currently halts at planning whenever the bump classifier emits `bump: major`, even for pre-1.0 libraries where semver convention treats breaking changes as `minor`.
- For pre-1.0 projects (`current_version` starts with `0.` or `v0.`), the classifier will be capped at `minor` — a breaking change downgrades to `minor` with an explicit note in the classifier's reasoning.
- For post-1.0 projects (`current_version >= 1.0.0`), behavior is unchanged — `major` verdicts still trip `applyMajorBumpGuard` and require human blessing.
- The pre-1.0 cap is enforced inside the classification step (prompt rule + `current_version` injection into the assembled prompt), NOT by relaxing the existing Go guard, which stays as a safety net.
- Concrete fixture: 2026-06-06 release of `bborbe-vault-cli` at `v0.69.0` (commit `77ba50e`) halted at planning with `precondition_failed: major_bump_not_allowed` for the `/refine-task → /plan-task` rename bullet; operator manually shipped `v0.70.0`. Replaying this exact task after the fix must produce an automatic minor-bump release with no operator intervention.

## Problem

The bump classifier sends Claude only the CHANGELOG bullets, not the current version. When a pre-1.0 project ships a breaking change, Claude correctly classifies it as `major`, the Go guard `applyMajorBumpGuard` trips, planning halts with `outcome: needs_input, precondition_failed: major_bump_not_allowed`, and an operator must intervene. This contradicts semver convention for the 0.x stream, where breaking changes are released as minor bumps. The friction recurs every time a pre-1.0 library (vault-cli, dark-factory plugins, internal tools) ships a renamed flag, removed command, or any reshape. The most recent occurrence was vault-cli on 2026-06-06: a `/refine-task → /plan-task` rename at `v0.69.0` halted the agent; the human-shipped fix was `v0.70.0` — exactly what the agent should have produced unattended.

## Goal

When the github-releaser agent classifies a release whose `current_version` is pre-1.0 (matches `0.` or `v0.`), the classifier never returns `bump: major`. Breaking-change bullets resolve to `minor` and the reasoning string records that the bump was capped due to pre-1.0 status. The downstream guard `applyMajorBumpGuard` therefore never trips for pre-1.0 projects, and the release proceeds end-to-end without operator intervention. Post-1.0 behavior — strict semver, `major` requires opt-in — is unchanged.

## Non-goals

- Do NOT auto-approve `major` bumps for post-1.0 projects — those still halt via `applyMajorBumpGuard`.
- Do NOT modify, weaken, or remove `applyMajorBumpGuard` Go code — it remains the safety net if the classifier ever emits `major` against policy.
- Do NOT add a CLI flag or env var to force pre-1.0-style capping for post-1.0 projects.
- Do NOT change the `BumpVerdict` schema (`bump` field still accepts `patch | minor | major` — the cap is policy, not type-level).
- Do NOT introduce a per-repo opt-out of the pre-1.0 cap — if a future consumer demands variation, that's a separate spec.
- Do NOT change the bump-classification prompt's priority order (major → minor → patch) for post-1.0 inputs.

## Desired Behavior

1. The bump-classification prompt accepts a `current_version` string as input (alongside the bullets) and renders a `## Current version` section before the bullets in the assembled prompt.
2. The prompt instructs Claude that when `current_version` matches the pattern `0.*` or `v0.*` (a pre-1.0 release stream), Claude MUST NOT return `bump: major`. The strongest allowed bump is `minor`, and a breaking-change bullet resolves to `minor` with a reasoning string that names the downgrade (e.g. mentions "pre-1.0"). The match is **literal prefix-based**: the value must start with `0.` or `v0.` exactly. `0.0.0`, `v0.69.0`, and `v0.69.0-rc1` all match; `0` and `v0` (no dot) do not — those are treated as malformed and fall through to the existing `bad_current_version` precondition.
3. The prompt's existing major → minor → patch priority order remains in force for `current_version` that is `1.*`, `v1.*`, or higher.
4. The planning step assembles the prompt by concatenating `BumpClassificationPrompt() + current_version section + bullets`, so the LLM always sees the version context.
5. Cached bump verdicts (M2 cache hit path) are unaffected by this change — only fresh LLM calls receive the new prompt assembly.
6. `applyMajorBumpGuard` Go code is unchanged. For pre-1.0 inputs the classifier never emits `major`, so the guard's `verdict.Bump != "major"` short-circuit fires and the guard is a no-op. For post-1.0 inputs the guard continues to enforce the existing decision table.
7. The fixture release `bborbe-vault-cli` at `v0.69.0` with the bullet `- refactor: rename /refine-task to /plan-task` produces verdict `{bump: "minor", reasoning: <contains "pre-1.0">}` when replayed against the updated prompt, and the planning step writes `outcome: ready` (not `needs_input`).

## Constraints

- `BumpVerdict` struct and `ParseBumpVerdict` validation rules (`bump` ∈ {patch, minor, major}, `reasoning` non-empty) MUST NOT change.
- `applyMajorBumpGuard` signature and decision table MUST NOT change.
- The `BumpClassificationPrompt()` Go function continues to return the embedded prompt text via `//go:embed`. Adding a separate `BumpClassificationPromptFor(currentVersion)` helper is acceptable; replacing the embedded-string export is not.
- All existing prompt assertions in `prompts_test.go` (presence of `patch | minor | major`, `BREAKING CHANGE`, `feat:`, `"bump":`, `major → minor → patch`) MUST continue to pass.
- The fixture path through `steps_planning.go` must continue to use `runner.Run(ctx, fullPrompt)` — no new runner method or context plumbing.
- Reference: existing spec [`060-github-releaser-major-bump-guard.md`](../specs/completed/060-github-releaser-major-bump-guard.md) defines the guard semantics that remain authoritative for post-1.0.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Pre-1.0 input, Claude ignores the cap rule and returns `bump: major` | `applyMajorBumpGuard` trips as today: planning writes `outcome: needs_input, precondition_failed: major_bump_not_allowed`. Operator sees the same halt as pre-fix. | Operator either ships manually OR re-runs; planning is idempotent. The Go guard remains the durable safety net. |
| `current_version` is missing or malformed (does not match `0.*`, `v0.*`, or a SemVer-shaped `N.M.P`) | The existing `bad_current_version` precondition path in planning fires before classification — current behavior, no new failure surface. | Operator fixes the `version` field in the task frontmatter and re-runs. |
| Cached `## Plan` carries a pre-fix `bump: major` verdict for a pre-1.0 project | Cache-hit path is taken and the old verdict is used. Guard still trips. | Operator deletes the cached `## Plan` block (or re-creates the task). Recovery observable: after deletion, re-running planning produces a fresh `## Plan` block whose `bump` field equals `minor` and whose `outcome` field equals `ready` — verifiable via `grep -E '^bump: minor$' <task>` returning 1 line and `grep -E '^outcome: ready$' <task>` returning 1 line. |
| Claude renders the reasoning string without naming the downgrade | Verdict still parses (reasoning is non-empty); release proceeds as `minor`. Audit trail is weaker but correct. | None required — reasoning quality is best-effort. |
| Post-1.0 project ships a breaking change | Classifier returns `bump: major`; guard trips as today; operator opt-in flips `release.allowMajorBump` in `.maintainer.yaml` or passes `--allow-major`. | Unchanged from spec 060. |

## Security / Abuse Cases

Not applicable — no new HTTP, file, or user-input surface. The `current_version` value is read from the task frontmatter (already trusted input) and concatenated into a prompt sent to the agent's own Claude runner. No new trust boundary is crossed.

## Acceptance Criteria

- [ ] `BumpClassificationPrompt()` embedded text contains a rule capping pre-1.0 inputs at `minor` — evidence: `grep -nE 'pre-1\.0|0\.\*|v0\.\*' agent/github-releaser/pkg/prompts/bump_classification.md` returns ≥1 line.
- [ ] The pre-1.0 cap rule names both `0.x` and `v0.x` patterns — evidence: prompt contains both literal forms (grep returns each).
- [ ] The prompt's existing major → minor → patch priority order text is still present — evidence: existing test `contains major → minor → patch priority order` still passes (`go test ./agent/github-releaser/pkg/prompts/...` exit 0).
- [ ] The planning step assembles the classification prompt with a `## Current version` section containing the resolved `current_version` value before the bullets — evidence: a unit test in `agent/github-releaser/pkg/steps_planning_test.go` captures the prompt string sent to the runner mock and asserts both `## Current version` and the literal version string are present and appear before the `## Bullets to classify` heading.
- [ ] The cached-verdict short-circuit in `resolveBumpVerdict` is unchanged — evidence: `git diff HEAD -- agent/github-releaser/pkg/steps_planning.go` shows the only changes inside `resolveBumpVerdict` are within the `cachedBump == ""` branch (the cache-hit early return is untouched). Verifier confirms via reading the diff context lines around the existing `if cachedBump != ""` block.
- [ ] A new `prompts_test.go` table entry asserts a fixture pre-1.0 breaking-change scenario yields `bump: minor` — evidence: the test invokes `ParseBumpVerdict` (or a new helper) on a stubbed Claude response and asserts `verdict.Bump == "minor"` and `verdict.Reasoning` contains the substring `pre-1.0`. Test passes (`go test -run Pre10 ./agent/github-releaser/pkg/prompts/...` exit 0).
- [ ] A planning-step test replays the vault-cli fixture (current_version `0.69.0`, one bullet `- refactor: rename /refine-task to /plan-task`) using a stub runner that returns `{"bump":"minor","reasoning":"breaking change capped to minor due to pre-1.0 stream"}` and asserts the resulting `## Plan` outcome is `ready`, `bump` is `minor`, `next_version` is `0.70.0` — evidence: the test asserts `outcome: ready` and `next_version: 0.70.0` in the published markdown.
- [ ] A planning-step test confirms a post-1.0 input (current_version `1.2.3`, breaking-change bullet) with stub runner returning `bump: major` still trips the guard — evidence: published markdown contains `outcome: needs_input` and `precondition_failed: major_bump_not_allowed`.
- [ ] `applyMajorBumpGuard` source has no diff outside whitespace/comments — evidence: `git diff HEAD -- agent/github-releaser/pkg/steps_planning.go` shows zero changed lines inside the `func (s *planningStep) applyMajorBumpGuard` body (only the prompt-assembly call site upstream changes).
- [ ] `make precommit` in the changed module exits 0 — evidence: exit code 0.

## Verification

Run from the repo root:

```
cd agent/github-releaser && make precommit
```

Then manual fixture replay (developer machine, against a scratch task):

```
# 1. Stage a task page with frontmatter version=0.69.0 and one ## Unreleased bullet:
#    - refactor: rename /refine-task to /plan-task
# 2. Invoke the planning step via the existing controller test harness (or a manual
#    `go run` of the planning step with a Claude runner that returns the capped verdict).
# 3. Assert the resulting ## Plan block has outcome: ready, bump: minor, next_version: 0.70.0.
```

Production smoke (post-merge, NOT a spec-completion gate): after the maintainer image lands on prod, re-fire the original 2026-06-06 vault-cli release task (or any subsequent pre-1.0 breaking-change release) and assert it reaches `outcome: ready` with no operator intervention. This is operator-confirmation only — the spec's verification cannot satisfy it because it crosses repos (vault-cli is not the spec's host). Spec completion gates exclusively on the AC list above.

## Suggested Decomposition

Prompts should be generated in this order — each row is a single prompt with a clear scope.

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Add pre-1.0 cap rule to `bump_classification.md` prompt + assert via prompt-level tests | 2, 3 | 1, 2, 3 | — |
| 2 | Inject `current_version` section into the assembled prompt in `steps_planning.go` + planning-step unit test capturing the assembled prompt string | 1, 4 | 4 | prompt 1 (rule must exist for assembly to reference) |
| 3 | Add fixture tests: pre-1.0 cap (DB 7), post-1.0 unchanged (negative), guard untouched | 5, 6, 7 | 5, 6, 7, 8, 9 | prompts 1 + 2 |

Rationale: prompt 1 is the smallest standalone change — prompt-content edit + assertions. Prompt 2 adds the Go-side assembly so the rule reaches Claude. Prompt 3 locks down the behavioral envelope (positive, negative, and "guard unchanged") in tests. Final `make precommit` (AC 10) is the daemon's standard tail and not a separate prompt.

## Do-Nothing Option

If we do nothing, every breaking change in a pre-1.0 bborbe library continues to halt the github-releaser agent and require operator intervention. The friction is small per-occurrence (one manual release) but recurring — vault-cli alone has hit it once already, and any 0.x library refactor will trigger it. The `applyMajorBumpGuard` already does the right thing for post-1.0; the cost of teaching the classifier about pre-1.0 semver convention is a single prompt rule plus passing one extra string into the assembled prompt. The do-nothing cost grows linearly with the number of pre-1.0 libraries the agent owns; the fix cost is paid once.
