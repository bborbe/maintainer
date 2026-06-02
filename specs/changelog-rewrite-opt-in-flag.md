---
tags:
  - dark-factory
  - spec
status: draft
---

## Summary

- Adds a per-repo opt-in flag `release.changelogRewrite` (boolean, default `false`) to `.maintainer.yaml`.
- When `false` (default): planning short-circuits to `rewrite_needed=false` regardless of `## Unreleased` content quality — today's header-rename-only behavior is preserved across the fleet.
- When `true`: planning evaluates `## Unreleased` against the Changelog Quality Guide and may emit `rewrite_needed=true` — the behavior spec 058 introduces.
- Invalid value (non-boolean) → fail-closed at planning entry, task ends in `human_review`, no commit / no tag / no push.
- Missing `release:` block or missing field → treated as `false`. No fleet-wide change of behavior on this spec landing; opt-in is per-repo and explicit.

## Problem

Spec 058 introduces an LLM-driven rewrite of `## Unreleased` body content during release planning. Even with the ai-review faithfulness gate, rolling that behavior change to every repo on day one is a step-function in blast radius — every release across the fleet would see its changelog body potentially mutated before anyone has watched the canaries (`maintainer`, `go-skeleton`) burn in. Operators need a way to keep today's header-rename-only behavior on every repo by default and turn rewrite on per-repo, starting with the canaries, expanding as confidence grows. Without an opt-in, 058 is either landed-and-shipped-everywhere or not landed at all — neither matches how the canary rollout is planned.

## Goal

After this work, every repo currently being released by github-releaser continues to see the exact pre-058 behavior — header rename only, no body changes — unless its `.maintainer.yaml` explicitly sets `release.changelogRewrite: true`. A repo that opts in gets the full 058 rewrite pipeline (planning evaluates quality, may emit `rewritten_unreleased`, execution applies it, ai-review checks faithfulness, push is gated). A repo with an invalid (non-boolean) value for the flag fails fast at planning entry without producing a commit, tag, or push — fail-closed.

## Non-goals

- Do NOT add a per-PR override (CLI flag, env var, frontmatter) — flag is per-repo only. If a future consumer demands per-PR variation, that's a separate spec.
- Do NOT add a global default override at agent-deployment level — source of truth stays at the repo. If a future consumer demands a fleet-wide flip, that's a separate spec.
- Do NOT support runtime mutation (config reload mid-release) — flag is read once at planning entry. If a future consumer demands hot-reload, that's a separate spec.
- Do NOT auto-enable based on changelog-quality heuristics — explicit opt-in only. Heuristic auto-enable defeats the conservative-rollout premise.
- Do NOT replace or modify the spec 058 ai-review faithfulness check — that pipeline runs unchanged when the flag is true.
- Do NOT short-circuit the rewrite-flow when the flag is true but `## Unreleased` is already clean — planning still emits `rewrite_needed=false` (the LLM judges); this spec only gates whether the LLM is asked at all.
- Do NOT change spec 058's behavior on flag-true repos.
- Do NOT migrate any existing repo's `.maintainer.yaml` to set the flag — canary opt-in (`maintainer`, `go-skeleton`) is a per-repo change shipped outside this spec.

## Desired Behavior

1. `.maintainer.yaml` accepts a new boolean field `release.changelogRewrite`, default `false`.
2. When `release.changelogRewrite` is `false` (explicitly or via default), planning's `## Plan` block carries `rewrite_needed=false` and omits `rewritten_unreleased`, regardless of `## Unreleased` content. Execution then performs header-rename only — identical to pre-058 behavior.
3. When `release.changelogRewrite` is `true`, planning evaluates `## Unreleased` against the Changelog Quality Guide (the 058 pipeline) and may emit `rewrite_needed=true` with `rewritten_unreleased`.
4. The flag is read once, at planning entry, from the `.maintainer.yaml` at the cloned ref's tip — the same file and same parsing path the github-releaser already uses for `release.autoRelease`.
5. A non-boolean value (e.g. the string `"yes"`, the number `1`) causes planning to fail-closed at entry with `error_category=invalid_config` surfaced on the task page. No commit, no tag, no push, no LLM call.
6. A missing `.maintainer.yaml`, a missing `release:` block, or a missing `changelogRewrite` field is treated as `false` — clean run, no error block, header-rename-only behavior.
7. The flag's value is recorded on the task page (planning step output) so a reader can tell from the task page alone which mode the run took.

## Constraints

- Depends on spec 058 landing first — that spec introduces the `PlanOutput.RewriteNeeded` field, `rewritten_unreleased`, and the planning prompt's rewrite-decision logic. This spec only adds the flag-gated short-circuit; it does not introduce those fields.
- The `.maintainer.yaml` reader already exists and is used for `release.autoRelease` — the same parsing path extends to `release.changelogRewrite`. No new yaml-discovery logic, no new file path.
- Default-on-missing semantics for `release.changelogRewrite` MUST be `false` — this is the load-bearing rollout-safety invariant. If a future consumer demands a different default, that's a separate spec.
- The 058 ai-review faithfulness check, the pre-push diff guard, and the structural ai-review checks (`TagExists`, `TagAtExpectedSHA`, `ChangelogHeaderRewritten`) run unchanged when the flag is true.
- The 3-phase lifecycle (`planning → execution → ai_review`) and `human_review` exit point are frozen.
- The flag is read at the cloned ref's tip — ambient changes to `master`'s `.maintainer.yaml` during an in-flight release do NOT retroactively flip the mode.

## Failure Modes

| Trigger | Expected behavior | Detection | Reversibility | Recovery |
|---|---|---|---|---|
| Repo has no `.maintainer.yaml` | Planning proceeds with `rewrite_needed=false` (default); header-rename-only execution | Task page shows clean planning step; no error block | Reversible (no remote side effect from the flag path) | None needed; same as today's autoRelease-less repos |
| Repo has `.maintainer.yaml` but no `release:` block | Same as missing yaml: treated as `false` | Same as above | Reversible | None needed |
| Repo has `release:` block but no `changelogRewrite` field | Treated as `false` | Same as above | Reversible | None needed |
| `release.changelogRewrite: true` and `## Unreleased` is clean | Planning runs 058 pipeline; LLM emits `rewrite_needed=false`; execution performs header-rename only | `## Plan` JSON shows `rewrite_needed=false`; no `rewritten_unreleased` field | Reversible | None |
| `release.changelogRewrite: true` and `## Unreleased` is noisy | Planning runs 058 pipeline; LLM emits `rewrite_needed=true` with `rewritten_unreleased`; downstream 058 phases proceed | `## Plan` JSON shows `rewrite_needed=true` and a `rewritten_unreleased` field | Reversible until push | 058's failure modes apply from this point |
| `release.changelogRewrite: "yes"` (string, not boolean) | Planning fails fast at entry with `error_category=invalid_config`; no commit, no tag, no push, no LLM call | Task page shows `## Error` block naming the field and the invalid value; phase=`human_review` | Reversible (no side effects) | Human fixes `.maintainer.yaml`, re-fires planning |
| `release.changelogRewrite: 1` (number, not boolean) | Same as above: fail-closed with `error_category=invalid_config` | Same as above | Reversible | Human fixes value to `true` or `false` |
| `.maintainer.yaml` itself is malformed yaml (unparseable) | Existing yaml-parse error path fires — same behavior as today for `autoRelease` parse failures; no new behavior from this spec | Existing yaml-parse error surfaces on task page | Reversible | Human fixes yaml syntax (pre-existing recovery path) |
| Ambient `master` `.maintainer.yaml` flips the flag mid-release | No effect on the in-flight release: the value at the cloned ref's tip is what binds | The in-flight task page's planning step shows the value at clone time | N/A — read-once semantics | None; next release picks up the new value |
| Two concurrent releases from same repo, one branch has `true` and one has `false` | Each release reads the flag at its own cloned ref's tip; each behaves per its own ref's value | Each task page records its own flag value independently | N/A | None |

## Security / Abuse Cases

- An attacker who can land a PR into `.maintainer.yaml` can flip `release.changelogRewrite` from `false` to `true` (or vice versa). Mitigation: this is the same trust boundary as today's `release.autoRelease` field — `.maintainer.yaml` changes go through the same PR review path. No new trust boundary is introduced.
- Setting the flag to `true` does NOT bypass the 058 ai-review faithfulness check or the pre-push diff guard. A malicious changelog mutation in a flag-true repo still has to pass faithfulness against the original `## Unreleased`.
- Setting the flag to `false` does NOT unlock any new write path — it short-circuits to the pre-058 behavior, which is the behavior the fleet runs today.
- Invalid-value fail-closed (not fail-open to `true` and not fail-open to `false`) is the intentional posture: a config typo on a high-trust field stops the release rather than silently picking either branch.

## Acceptance Criteria

- [ ] yaml-parse boundary: `.maintainer.yaml` containing `release.changelogRewrite: true` parses into a config struct whose corresponding field is the boolean `true` — evidence: Ginkgo unit spec on the yaml-parsing function asserts the parsed struct field equals `true`.
- [ ] yaml-parse boundary: `.maintainer.yaml` containing `release.changelogRewrite: false` parses to the boolean `false` — evidence: Ginkgo unit spec assertion.
- [ ] yaml-parse boundary: `.maintainer.yaml` with no `release:` block parses into a config whose `release.changelogRewrite` equivalent equals `false` (default) — evidence: Ginkgo unit spec assertion.
- [ ] yaml-parse boundary: `.maintainer.yaml` with `release:` block but no `changelogRewrite` field parses to `false` (default) — evidence: Ginkgo unit spec assertion.
- [ ] yaml-parse boundary: completely-absent `.maintainer.yaml` produces a config whose `release.changelogRewrite` equivalent equals `false` — evidence: Ginkgo unit spec assertion (existing missing-yaml code path returns default-valued config).
- [ ] yaml-parse boundary: `.maintainer.yaml` containing `release.changelogRewrite: "yes"` (string) returns a parse-or-validation error tagged `invalid_config` — evidence: Ginkgo unit spec asserts the error type / category constant.
- [ ] yaml-parse boundary: `.maintainer.yaml` containing `release.changelogRewrite: 1` (number) returns the same `invalid_config` error — evidence: Ginkgo unit spec assertion.
- [ ] Planning short-circuit, flag=false: against a fixture `## Unreleased` containing a raw `git log` dump (the same noisy fixture spec 058's planning suite uses), planning emits `## Plan` JSON with `rewrite_needed=false` and no `rewritten_unreleased` field, and the planning prompt is NOT sent to the LLM (or the LLM call is verifiably skipped) — evidence: Ginkgo integration spec assertion on the parsed `## Plan` JSON plus an assertion that the LLM client mock recorded zero rewrite-prompt calls.
- [ ] Planning rewrite enabled, flag=true, noisy `## Unreleased`: planning emits `## Plan` JSON with `rewrite_needed=true` and a `rewritten_unreleased` field — evidence: Ginkgo integration spec assertion on parsed `## Plan` JSON.
- [ ] Planning rewrite enabled, flag=true, already-clean `## Unreleased`: planning emits `## Plan` JSON with `rewrite_needed=false` and no `rewritten_unreleased` field (the LLM judged the content clean) — evidence: Ginkgo integration spec assertion.
- [ ] Invalid-value fail-closed: a release fired against a repo whose `.maintainer.yaml` has `release.changelogRewrite: "yes"` ends in `human_review` at the planning step, the task page contains an `## Error` block naming the field and the invalid value, no commit lands on the local clone, no tag is created locally, and `git ls-remote` against the upstream shows no new tag and no branch tip advance — evidence: task phase string == `human_review`, `## Error` block text assertion, `git rev-list --count HEAD ^origin/master` output == `0`, `git tag -l vX.Y.Z | wc -l` output == `0`, `git ls-remote` assertion.
- [ ] Missing-yaml clean run: a release fired against a repo with no `.maintainer.yaml` produces a clean planning step (no `## Error` block) and execution performs header-rename only (single commit modifies only `CHANGELOG.md`, no body changes) — evidence: task page assertion (no `## Error` block) plus `git show HEAD -- CHANGELOG.md` diff assertion showing only the `## Unreleased` → `## vX.Y.Z` header rename.
- [ ] Task-page audit trail: the planning step's recorded output on the task page includes the resolved `release.changelogRewrite` value (`true` / `false`) used for the run — evidence: Ginkgo spec parses the planning step's task-page block and asserts the flag value field is present and matches the fixture's `.maintainer.yaml`.
- [ ] Flag-read-once semantics: an integration spec mutates `.maintainer.yaml` on the source `master` ref AFTER the release has cloned the working ref; the in-flight release's planning step still records the value from the cloned ref's tip — evidence: Ginkgo integration spec assertion on the task page's recorded flag value vs. the post-mutation file content.
- [ ] `make precommit` exits 0 in `agent/github-releaser` — evidence: exit code 0.

## Verification

```
cd ~/Documents/workspaces/maintainer-changelog-rewrite/agent/github-releaser
make precommit
```

Expected: exit code 0; all Ginkgo specs above pass.

After unit + integration coverage is green, the canary opt-in (per-repo `.maintainer.yaml` setting `release.changelogRewrite: true` for `maintainer` and `go-skeleton`) happens outside this spec's scope.

## Do-Nothing Option

If we don't ship this: spec 058 either lands and immediately changes behavior for every repo on the next release across the fleet, or it doesn't land. Neither matches the planned canary rollout. The first option blows blast radius on day one — every release across every repo using github-releaser sees its `## Unreleased` body potentially mutated, with the ai-review faithfulness gate as the only catcher. The second option leaves 058's payoff stranded — the rewrite pipeline exists but cannot be exercised in production without a behavior flip everywhere. Not acceptable: the canary rollout is the whole reason 058 was scoped to be a planning-only addition that can be gated.
